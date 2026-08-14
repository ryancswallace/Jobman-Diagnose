package generation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

const minimumUsefulLogContextBytes = 1024

type logContextCandidate struct {
	artifact    diagnostic.Artifact
	enrichment  []diagnosis.EnrichmentItem
	anchor      *diagnosis.EnrichmentItem
	anchorScore int
}

type projectedLogContext struct {
	artifact   provider.ProjectedArtifact
	enrichment []provider.ProjectedEnrichment
}

// selectLogContexts ranks exact diagnostic ranges ahead of generic terminal
// output, then spends the profile's existing log-content budget across the
// strongest distinct streams. It never reads bytes outside the sealed core
// artifacts supplied by Jobman.
//
//nolint:cyclop,gocognit // Candidate ranking and bounded budget allocation form one selection policy.
func selectLogContexts(
	artifacts []diagnostic.Artifact,
	enrichmentItems []diagnosis.EnrichmentItem,
	limits config.ClassLimits,
) []projectedLogContext {
	if limits.MaximumArtifacts == 0 || limits.MaximumBytes == 0 {
		return nil
	}
	candidates := make([]logContextCandidate, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Disclosure != diagnostic.DisclosureLogContent || len(artifact.Data) == 0 {
			continue
		}
		candidate := logContextCandidate{artifact: artifact, enrichment: []diagnosis.EnrichmentItem{}}
		for _, item := range enrichmentItems {
			if item.SourceArtifactID != artifact.ID || item.ByteStart >= item.ByteEnd ||
				item.ByteEnd > uint64(len(artifact.Data)) {
				continue
			}
			candidate.enrichment = append(candidate.enrichment, item)
			score := diagnosticAnchorScore(item)
			if candidate.anchor == nil || score > candidate.anchorScore ||
				score == candidate.anchorScore && item.ByteEnd > candidate.anchor.ByteEnd {
				copyItem := item
				candidate.anchor = &copyItem
				candidate.anchorScore = score
			}
		}
		candidates = append(candidates, candidate)
	}
	slices.SortStableFunc(candidates, compareLogContextCandidates)
	if len(candidates) != 0 && candidates[0].anchor != nil {
		causalCandidates := candidates[:0]
		for _, candidate := range candidates {
			if candidate.anchor != nil {
				causalCandidates = append(causalCandidates, candidate)
			}
		}
		candidates = causalCandidates
	}
	if uint64(len(candidates)) > limits.MaximumArtifacts {
		candidates = candidates[:limits.MaximumArtifacts]
	}

	result := make([]projectedLogContext, 0, len(candidates))
	remainingBytes := limits.MaximumBytes
	for index, candidate := range candidates {
		// #nosec G115 -- a nonnegative slice length always fits uint64.
		remainingCandidates := uint64(len(candidates) - index)
		if remainingBytes == 0 {
			break
		}
		budget := remainingBytes / remainingCandidates
		if budget < minimumUsefulLogContextBytes && remainingBytes >= minimumUsefulLogContextBytes {
			budget = minimumUsefulLogContextBytes
		}
		budget = min(budget, remainingBytes)
		selected := projectLogContext(candidate, budget)
		if selected.artifact.ContentBytes == 0 || selected.artifact.ContentBytes > remainingBytes {
			continue
		}
		remainingBytes -= selected.artifact.ContentBytes
		result = append(result, selected)
	}

	return result
}

func compareLogContextCandidates(left, right logContextCandidate) int {
	if left.anchorScore != right.anchorScore {
		return right.anchorScore - left.anchorScore
	}
	if left.artifact.Run != right.artifact.Run {
		if left.artifact.Run > right.artifact.Run {
			return -1
		}
		return 1
	}
	if left.artifact.Stream != right.artifact.Stream {
		if left.artifact.Stream == "stderr" {
			return -1
		}
		if right.artifact.Stream == "stderr" {
			return 1
		}
	}

	return strings.Compare(left.artifact.ID, right.artifact.ID)
}

func diagnosticAnchorScore(item diagnosis.EnrichmentItem) int {
	switch {
	case causalDiagnosticFormat(item.Format):
		return 300
	case item.Format == "python_traceback" || item.Format == "go_panic" || item.Format == "jvm_exception" ||
		item.Format == "compiler_diagnostic" || item.Format == "python_syntax":
		return 250
	default:
		return 200
	}
}

func projectLogContext(candidate logContextCandidate, maximumBytes uint64) projectedLogContext {
	data := candidate.artifact.Data
	budget := maximumBytes
	var start, end int
	var content string
	for attempts := 0; attempts < 4; attempts++ {
		start, end = selectedLogByteRange(data, candidate.anchor, budget)
		if start >= end {
			return projectedLogContext{}
		}
		content = strings.ToValidUTF8(string(data[start:end]), "�")
		if uint64(len(content)) <= maximumBytes {
			break
		}
		excess := uint64(len(content)) - maximumBytes
		if excess >= budget {
			budget = max(1, budget/2)
		} else {
			budget -= excess
		}
	}
	if start >= end {
		return projectedLogContext{}
	}
	selectedRaw := data[start:end]
	if uint64(len(content)) > maximumBytes {
		return projectedLogContext{}
	}
	selection := "tail"
	anchorReason := "terminal_output"
	anchorOffset := end - 1
	if candidate.anchor != nil {
		selection = "causal_context"
		anchorReason = "structured_diagnostic"
		if causalDiagnosticFormat(candidate.anchor.Format) {
			anchorReason = "causal_diagnostic"
		}
		// Structured ranges can be larger than the selected context. Their end is
		// the terminal diagnostic and is guaranteed to remain in the window.
		// #nosec G115 -- candidate ranges were checked against this in-memory artifact above.
		anchorOffset = min(end-1, int(candidate.anchor.ByteEnd)-1)
	}
	totalLines := logLineCount(data)
	startLine := logLineAt(data, start)
	endLine := logLineAt(data, end-1)
	anchorLine := logLineAt(data, anchorOffset)
	capturedAt := candidate.artifact.CapturedAt.UTC().Round(0)
	projected := provider.ProjectedArtifact{
		ID: candidate.artifact.ID, Role: candidate.artifact.Role, Run: candidate.artifact.Run,
		Stream: candidate.artifact.Stream, Selection: selection, AnchorLine: anchorLine,
		AnchorReason: anchorReason, StartLine: startLine, EndLine: endLine, TotalLines: totalLines,
		ByteStart: uint64(start), ByteEnd: uint64(end), FileBytes: uint64(len(data)),
		Content: content, Encoding: "utf-8-lossy", Digest: candidate.artifact.Digest,
		ContentDigest: logContentDigest([]byte(content)), CapturedAt: &capturedAt,
		Quality:       string(candidate.artifact.Quality),
		Truncated:     candidate.artifact.Truncated || start != 0 || end != len(data),
		SelectedBytes: uint64(len(selectedRaw)), ContentBytes: uint64(len(content)),
		Disclosure: string(candidate.artifact.Disclosure),
	}
	projectedEnrichment := make([]provider.ProjectedEnrichment, 0, len(candidate.enrichment))
	for _, item := range candidate.enrichment {
		if item.ByteStart < uint64(start) || item.ByteEnd > uint64(end) {
			continue
		}
		byteStart := len(strings.ToValidUTF8(string(data[start:item.ByteStart]), "�"))
		byteEnd := len(strings.ToValidUTF8(string(data[start:item.ByteEnd]), "�"))
		projectedEnrichment = append(projectedEnrichment, provider.ProjectedEnrichment{
			ID: item.ID, Code: item.Code, Format: item.Format, SourceArtifactID: item.SourceArtifactID,
			ByteStart: uint64(byteStart), ByteEnd: uint64(byteEnd),
			Collector: item.Collector.Name, CollectorVersion: item.Collector.Version,
			Quality: string(item.Quality), Disclosure: string(diagnostic.DisclosureLogContent),
			DiagnosticLines: projectedDiagnosticLines([]diagnostic.Artifact{candidate.artifact}, item),
		})
	}

	return projectedLogContext{artifact: projected, enrichment: projectedEnrichment}
}

func selectedLogByteRange(data []byte, anchor *diagnosis.EnrichmentItem, maximumBytes uint64) (int, int) {
	if len(data) == 0 || maximumBytes == 0 {
		return 0, 0
	}
	budget := len(data)
	// #nosec G115 -- the comparison makes the uint64-to-int conversion bounded by len(data).
	if maximumBytes < uint64(len(data)) {
		budget = int(maximumBytes)
	}
	if len(data) <= budget {
		return 0, len(data)
	}
	anchorStart, anchorEnd := len(data)-1, len(data)
	if anchor != nil {
		// #nosec G115 -- callers validate both offsets against this in-memory artifact.
		anchorStart, anchorEnd = int(anchor.ByteStart), int(anchor.ByteEnd)
	}
	if anchorEnd-anchorStart >= budget {
		start := anchorEnd - budget
		return nextLineStart(data, start), anchorEnd
	}
	contextBefore := budget * 2 / 3
	start := max(0, anchorStart-contextBefore)
	end := min(len(data), start+budget)
	if end < anchorEnd {
		end = anchorEnd
		start = max(0, end-budget)
	}
	start = nextLineStart(data, start)
	end = previousLineEnd(data, end)
	if end <= start || anchorStart < start || anchorEnd > end {
		start = max(0, anchorEnd-budget)
		end = anchorEnd
	}

	return start, end
}

func nextLineStart(data []byte, offset int) int {
	if offset <= 0 {
		return 0
	}
	if index := bytes.IndexByte(data[offset:], '\n'); index >= 0 && offset+index+1 < len(data) {
		return offset + index + 1
	}

	return offset
}

func previousLineEnd(data []byte, offset int) int {
	if offset >= len(data) {
		return len(data)
	}
	if index := bytes.LastIndexByte(data[:offset], '\n'); index >= 0 {
		return index + 1
	}

	return offset
}

func logLineCount(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	count := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		count++
	}

	// #nosec G115 -- a nonnegative byte-slice count always fits uint64.
	return uint64(count)
}

func logLineAt(data []byte, offset int) uint64 {
	offset = min(max(offset, 0), len(data))

	// #nosec G115 -- a nonnegative byte-slice count always fits uint64.
	return uint64(bytes.Count(data[:offset], []byte{'\n'})) + 1
}

func logContentDigest(data []byte) string {
	digest := sha256.Sum256(data)

	return "sha256:" + hex.EncodeToString(digest[:])
}
