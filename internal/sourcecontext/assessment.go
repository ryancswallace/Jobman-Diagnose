package sourcecontext

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

// AssessmentStatus describes whether current source can be related to a
// location recorded in target output. Consistent means only that no mismatch
// was detected; it does not prove that the snapshot is the executed revision.
type AssessmentStatus string

// Controlled source-context assessment statuses.
const (
	AssessmentConsistent AssessmentStatus = "consistent"
	AssessmentMismatch   AssessmentStatus = "mismatch"
	AssessmentUnverified AssessmentStatus = "unverified"
)

// Controlled source-context assessment reasons.
const (
	ReasonRuntimeContentMismatch = "runtime_content_mismatch"
	ReasonRuntimeContentMatch    = "runtime_content_match"
	ReasonRuntimeLineOutOfRange  = "runtime_line_out_of_range"
	ReasonRuntimeLocationMatch   = "runtime_location_match"
	ReasonRuntimeLocationMissing = "runtime_location_missing"
	ReasonRuntimePathMismatch    = "runtime_path_mismatch"
)

// Assessment records one bounded, non-content-bearing comparison result.
type Assessment struct {
	SourceID          string
	Status            AssessmentStatus
	Reason            string
	RuntimeArtifactID string
	RuntimeLine       uint64
}

type sourceLocation struct {
	artifactID string
	path       string
	line       uint64
	snippet    string
}

type locationPattern struct {
	expression      *regexp.Regexp
	includesSnippet bool
}

var sourceLocationPatterns = []locationPattern{
	{expression: pythonLocation, includesSnippet: true},
	{expression: pathLocation},
	{expression: frameLocation},
}

// Assess compares one current source snapshot with source locations already
// present in sealed target logs. It never reads a file or treats a matching
// path and line as proof that the source revision executed.
func Assess(artifacts []diagnostic.Artifact, source diagnosis.SourceContext) Assessment {
	result := Assessment{
		SourceID: source.ID, Status: AssessmentUnverified, Reason: ReasonRuntimeLocationMissing,
	}
	locations := sourceLocations(artifacts)
	if len(locations) == 0 {
		return result
	}
	matches := make([]sourceLocation, 0, len(locations))
	for _, location := range locations {
		if sameLogicalSourcePath(source.Path, location.path) {
			matches = append(matches, location)
		}
	}
	if len(matches) == 0 {
		result.Status = AssessmentMismatch
		result.Reason = ReasonRuntimePathMismatch
		result.RuntimeArtifactID = locations[len(locations)-1].artifactID
		result.RuntimeLine = locations[len(locations)-1].line

		return result
	}
	result.Status = AssessmentConsistent
	result.Reason = ReasonRuntimeLocationMatch
	for _, location := range matches {
		result.RuntimeArtifactID = location.artifactID
		result.RuntimeLine = location.line
		if location.line > source.TotalLines {
			result.Status = AssessmentMismatch
			result.Reason = ReasonRuntimeLineOutOfRange

			return result
		}
		if location.snippet == "" {
			continue
		}
		current, ok := selectedSourceLine(source, location.line)
		if !ok {
			continue
		}
		if normalizeSourceLine(current) != normalizeSourceLine(location.snippet) {
			result.Status = AssessmentMismatch
			result.Reason = ReasonRuntimeContentMismatch

			return result
		}
		result.Reason = ReasonRuntimeContentMatch
	}

	return result
}

// MismatchMessage returns controlled prose without copying paths, source, or
// target output into a report.
func MismatchMessage(assessment Assessment) string {
	switch assessment.Reason {
	case ReasonRuntimeContentMismatch:
		return "Current source context was withheld because its selected line differs from the source line recorded in target output."
	case ReasonRuntimeLineOutOfRange:
		return "Current source context was withheld because a recorded runtime line is outside the current file."
	default:
		return "Current source context was withheld because its file does not match the source locations recorded in target output."
	}
}

func sourceLocations(artifacts []diagnostic.Artifact) []sourceLocation {
	result := make([]sourceLocation, 0, 8)
	for _, artifact := range artifacts {
		if artifact.Disclosure != diagnostic.DisclosureLogContent {
			continue
		}
		result = append(result, artifactSourceLocations(artifact)...)
	}

	return result
}

func artifactSourceLocations(artifact diagnostic.Artifact) []sourceLocation {
	text := strings.ToValidUTF8(string(artifact.Data), "")
	result := make([]sourceLocation, 0, 4)
	seen := make(map[string]struct{})
	for _, pattern := range sourceLocationPatterns {
		for _, match := range pattern.expression.FindAllStringSubmatchIndex(text, -1) {
			location, ok := matchedSourceLocation(artifact.ID, text, match, pattern.includesSnippet)
			if !ok {
				continue
			}
			key := location.path + "\x00" + strconv.FormatUint(location.line, 10)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, location)
		}
	}

	return result
}

func matchedSourceLocation(artifactID, text string, match []int, includesSnippet bool) (sourceLocation, bool) {
	if len(match) != 6 || match[2] < 0 || match[4] < 0 {
		return sourceLocation{}, false
	}
	path := text[match[2]:match[3]]
	if _, _, ok := sourceType(path); !ok {
		return sourceLocation{}, false
	}
	line, err := strconv.ParseUint(text[match[4]:match[5]], 10, 64)
	if err != nil || line == 0 {
		return sourceLocation{}, false
	}
	location := sourceLocation{artifactID: artifactID, path: path, line: line}
	if includesSnippet {
		location.snippet = nextIndentedLine(text, match[1])
	}

	return location, true
}

func nextIndentedLine(text string, locationEnd int) string {
	lineEnd := strings.IndexByte(text[locationEnd:], '\n')
	if lineEnd < 0 {
		return ""
	}
	start := locationEnd + lineEnd + 1
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		end = len(text)
	} else {
		end += start
	}
	line := text[start:end]
	if line == "" || line[0] != ' ' && line[0] != '\t' {
		return ""
	}

	return strings.TrimSpace(line)
}

func selectedSourceLine(source diagnosis.SourceContext, line uint64) (string, bool) {
	if line < source.StartLine || line > source.EndLine {
		return "", false
	}
	lines := strings.Split(string(source.Data), "\n")
	index := line - source.StartLine
	if index >= uint64(len(lines)) {
		return "", false
	}

	return lines[index], true
}

func normalizeSourceLine(value string) string {
	return strings.TrimSpace(value)
}

func sameLogicalSourcePath(sourcePath, reference string) bool {
	if sameSourcePath(sourcePath, reference) {
		return true
	}

	return portableBase(reference) == portableBase(sourcePath)
}

func portableBase(value string) string {
	return path.Base(strings.ReplaceAll(value, `\`, "/"))
}
