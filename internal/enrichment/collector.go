// Package enrichment derives bounded, attributed structure from artifacts that
// Jobman already selected and sealed. It never reads another file or executes a
// command.
package enrichment

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

const (
	collectorName    = "builtin.log-structure"
	collectorVersion = "1.0.0"
	maximumItems     = 32
	maximumRange     = 64 * 1024
	maximumLines     = 256
)

// Stable enrichment codes.
const (
	CodePythonTraceback    = "enrichment.traceback.python"
	CodeGoPanic            = "enrichment.traceback.go_panic"
	CodeJVMException       = "enrichment.traceback.jvm"
	CodeCompilerDiagnostic = "enrichment.compiler.diagnostic"
)

type byteRange struct {
	start  int
	end    int
	code   string
	format string
}

type lineRange struct {
	start int
	end   int
}

// Collect verifies core evidence and returns a sealed companion wrapper with
// exact ranges for recognized structured output. Collection is deterministic
// and bounded by the already selected artifact bytes, item count, range size,
// and line count.
func Collect(ctx context.Context, core diagnostic.Evidence) (diagnosis.FailureEvidence, error) {
	if ctx == nil {
		return diagnosis.FailureEvidence{}, fmt.Errorf("collect enrichment: nil context")
	}
	if err := diagnostic.Verify(core); err != nil {
		return diagnosis.FailureEvidence{}, fmt.Errorf("collect enrichment: verify core evidence: %w", err)
	}
	items := make([]diagnosis.EnrichmentItem, 0)
	for _, artifact := range core.Artifacts {
		if err := ctx.Err(); err != nil {
			return diagnosis.FailureEvidence{}, fmt.Errorf("collect enrichment: %w", err)
		}
		for _, selected := range structuredRanges(artifact.Data) {
			if len(items) == maximumItems {
				break
			}
			// #nosec G115 -- structuredRanges produces nonnegative indexes into this exact artifact.
			items = append(items, diagnosis.EnrichmentItem{
				ID:   fmt.Sprintf("analysis:%06d:%s", len(items)+1, strings.ReplaceAll(selected.format, "_", "-")),
				Code: selected.code, Format: selected.format, SourceArtifactID: artifact.ID,
				ByteStart: uint64(selected.start), ByteEnd: uint64(selected.end),
				ObservedAt: artifact.CapturedAt.UTC().Round(0),
				Collector:  diagnosis.AnalyzerDescriptor{Name: collectorName, Version: collectorVersion},
				Quality:    diagnostic.QualityDerivedExact, Disclosure: diagnostic.DisclosureLocalOnly,
			})
		}
		if len(items) == maximumItems {
			break
		}
	}

	return diagnosis.SealFailureEvidence(core, items)
}

func structuredRanges(data []byte) []byteRange {
	lines := splitLines(data)
	result := make([]byteRange, 0, 4)
	if selected, ok := pythonTraceback(data, lines); ok {
		result = append(result, selected)
	}
	if selected, ok := goPanic(data, lines); ok {
		result = append(result, selected)
	}
	if selected, ok := jvmException(data, lines); ok {
		result = append(result, selected)
	}
	for _, selected := range compilerDiagnostics(data, lines) {
		result = append(result, selected)
		if len(result) == 8 {
			break
		}
	}
	slices.SortFunc(result, func(left, right byteRange) int {
		if left.start != right.start {
			return left.start - right.start
		}

		return strings.Compare(left.code, right.code)
	})

	return result
}

func splitLines(data []byte) []lineRange {
	result := make([]lineRange, 0, min(bytes.Count(data, []byte{'\n'})+1, maximumLines*4))
	start := 0
	for start < len(data) && len(result) < maximumLines*4 {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			result = append(result, lineRange{start: start, end: len(data)})
			break
		}
		end += start + 1
		result = append(result, lineRange{start: start, end: end})
		start = end
	}

	return result
}

func pythonTraceback(data []byte, lines []lineRange) (byteRange, bool) {
	const marker = "traceback (most recent call last):"
	for index, line := range lines {
		if !bytes.Contains(bytes.ToLower(data[line.start:line.end]), []byte(marker)) {
			continue
		}
		end := boundedEnd(line.start, line.end, data, lines[index+1:])
		for _, following := range lines[index+1:] {
			trimmed := bytes.TrimSpace(data[following.start:following.end])
			if len(trimmed) != 0 && !leadingSpace(data[following.start:following.end]) &&
				bytes.Contains(trimmed, []byte{':'}) {
				end = min(following.end, line.start+maximumRange)
				break
			}
		}

		return byteRange{start: line.start, end: end, code: CodePythonTraceback, format: "python_traceback"}, true
	}

	return byteRange{}, false
}

func goPanic(data []byte, lines []lineRange) (byteRange, bool) {
	for index, line := range lines {
		if !bytes.HasPrefix(bytes.TrimSpace(data[line.start:line.end]), []byte("panic:")) {
			continue
		}
		end := boundedEnd(line.start, line.end, data, lines[index+1:])

		return byteRange{start: line.start, end: end, code: CodeGoPanic, format: "go_panic"}, true
	}

	return byteRange{}, false
}

func jvmException(data []byte, lines []lineRange) (byteRange, bool) {
	for index, line := range lines {
		current := bytes.TrimSpace(data[line.start:line.end])
		if !bytes.Contains(current, []byte("Exception")) && !bytes.Contains(current, []byte("Error:")) {
			continue
		}
		foundFrame := false
		for _, following := range lines[index+1 : min(len(lines), index+8)] {
			trimmed := bytes.TrimSpace(data[following.start:following.end])
			if bytes.HasPrefix(trimmed, []byte("at ")) || bytes.HasPrefix(trimmed, []byte("Caused by:")) {
				foundFrame = true
				break
			}
		}
		if !foundFrame {
			continue
		}
		end := boundedEnd(line.start, line.end, data, lines[index+1:])

		return byteRange{start: line.start, end: end, code: CodeJVMException, format: "jvm_exception"}, true
	}

	return byteRange{}, false
}

func compilerDiagnostics(data []byte, lines []lineRange) []byteRange {
	result := make([]byteRange, 0, 4)
	for _, line := range lines {
		value := bytes.TrimSpace(data[line.start:line.end])
		if !bytes.Contains(bytes.ToLower(value), []byte(": error:")) || !hasLineAndColumn(value) {
			continue
		}
		result = append(result, byteRange{
			start: line.start, end: line.end, code: CodeCompilerDiagnostic, format: "compiler_diagnostic",
		})
		if len(result) == 4 {
			break
		}
	}

	return result
}

func hasLineAndColumn(value []byte) bool {
	colon := bytes.IndexByte(value, ':')
	for colon >= 0 && colon+1 < len(value) {
		lineEnd := bytes.IndexByte(value[colon+1:], ':')
		if lineEnd < 0 {
			return false
		}
		lineEnd += colon + 1
		columnEnd := bytes.IndexByte(value[lineEnd+1:], ':')
		if columnEnd >= 0 {
			columnEnd += lineEnd + 1
			if digits(value[colon+1:lineEnd]) && digits(value[lineEnd+1:columnEnd]) {
				return true
			}
		}
		next := bytes.IndexByte(value[colon+1:], ':')
		if next < 0 {
			return false
		}
		colon += next + 1
	}

	return false
}

func digits(value []byte) bool {
	if len(value) == 0 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}

	return true
}

func boundedEnd(start, initialEnd int, data []byte, following []lineRange) int {
	end := initialEnd
	for index, line := range following {
		if index >= maximumLines-1 || line.end-start > maximumRange {
			break
		}
		if len(bytes.TrimSpace(data[line.start:line.end])) == 0 && index > 0 {
			break
		}
		end = line.end
	}

	return min(end, start+maximumRange)
}

func leadingSpace(value []byte) bool {
	return len(value) != 0 && (value[0] == ' ' || value[0] == '\t')
}
