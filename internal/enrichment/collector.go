// Package enrichment derives bounded, attributed structure from artifacts that
// Jobman already selected and sealed. It never reads another file or executes a
// command.
package enrichment

// cspell:ignore connectionrefusederror parseint permissionerror timeouterror

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
	collectorVersion = "1.1.0"
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
	CodeCausalMessage      = "enrichment.diagnostic.causal_message"
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

type diagnosticSignal struct {
	format string
	needle []byte
}

var diagnosticSignals = []diagnosticSignal{
	{format: "address_in_use", needle: []byte("address already in use")},
	{format: "dependency_missing", needle: []byte("cannot find module")},
	{format: "dependency_missing", needle: []byte("no module named")},
	{format: "dependency_missing", needle: []byte("could not resolve dependencies")},
	{format: "dependency_missing", needle: []byte("could not find artifact")},
	{format: "dependency_missing", needle: []byte("cannot open shared object file")},
	{format: "tls_verification_failed", needle: []byte("certificate signed by unknown authority")},
	{format: "tls_verification_failed", needle: []byte("certificate verify failed")},
	{format: "nested_command_missing", needle: []byte("command not found")},
	{format: "connection_refused", needle: []byte("connection refused")},
	{format: "connection_refused", needle: []byte("connectionrefusederror")},
	{format: "connection_refused", needle: []byte("econnrefused")},
	{format: "deadline_exceeded", needle: []byte("context deadline exceeded")},
	{format: "deadline_exceeded", needle: []byte("timeouterror")},
	{format: "database_deadlock", needle: []byte("deadlock detected")},
	{format: "database_unique_violation", needle: []byte("duplicate key")},
	{format: "data_validation", needle: []byte("invalid decimal")},
	{format: "data_validation", needle: []byte("parseint")},
	{format: "migration_rejected", needle: []byte("migration rejected")},
	{format: "configuration_missing", needle: []byte("missing setting")},
	{format: "configuration_missing", needle: []byte("parameter not set")},
	{format: "configuration_missing", needle: []byte("required environment variable")},
	{format: "storage_exhausted", needle: []byte("no space left on device")},
	{format: "dns_resolution_failed", needle: []byte("no such host")},
	{format: "missing_file", needle: []byte("no such file or directory")},
	{format: "permission_denied", needle: []byte("permission denied")},
	{format: "permission_denied", needle: []byte("permissionerror")},
	{format: "read_only_filesystem", needle: []byte("read-only file system")},
	{format: "service_unavailable", needle: []byte("service unavailable")},
	{format: "file_descriptor_exhausted", needle: []byte("too many open files")},
	{format: "rate_limited", needle: []byte("too many requests")},
	{format: "linker_undefined_reference", needle: []byte("undefined reference")},
	{format: "migration_required", needle: []byte("apply migrations")},
	{format: "authentication_denied", needle: []byte("unauthorized")},
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
	result = append(result, pythonTracebacks(data, lines)...)
	if selected, ok := goPanic(data, lines); ok {
		result = append(result, selected)
	}
	if selected, ok := jvmException(data, lines); ok {
		result = append(result, selected)
	}
	result = append(result, pythonSyntaxDiagnostics(data, lines)...)
	for _, selected := range compilerDiagnostics(data, lines) {
		result = append(result, selected)
		if len(result) == 8 {
			break
		}
	}
	for _, selected := range causalMessages(data, lines) {
		result = append(result, selected)
		if len(result) == 12 {
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

func causalMessages(data []byte, lines []lineRange) []byteRange {
	result := make([]byteRange, 0, 4)
	for _, line := range lines {
		format := ClassifyDiagnostic(data[line.start:line.end])
		if format == "" {
			continue
		}
		result = append(result, byteRange{
			start: line.start, end: line.end, code: CodeCausalMessage, format: format,
		})
		if len(result) == 4 {
			break
		}
	}

	return result
}

// ClassifyDiagnostic returns the controlled subtype for one untrusted target
// line. The empty string means that no conservative signature matched.
func ClassifyDiagnostic(value []byte) string {
	lower := bytes.ToLower(bytes.TrimSpace(value))
	for _, signal := range diagnosticSignals {
		if bytes.Contains(lower, signal.needle) {
			return signal.format
		}
	}

	return ""
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

func pythonTracebacks(data []byte, lines []lineRange) []byteRange {
	const marker = "traceback (most recent call last):"
	result := make([]byteRange, 0, 2)
	for index, line := range lines {
		if !bytes.Contains(bytes.ToLower(data[line.start:line.end]), []byte(marker)) {
			continue
		}
		end := boundedEnd(line.start, line.end, data, lines[index+1:])
		result = append(result, byteRange{
			start: line.start, end: end, code: CodePythonTraceback, format: "python_traceback",
		})
		if len(result) == 4 {
			break
		}
	}

	return result
}

func pythonSyntaxDiagnostics(data []byte, lines []lineRange) []byteRange {
	result := make([]byteRange, 0, 2)
	for index, line := range lines {
		if !bytes.Contains(bytes.TrimSpace(data[line.start:line.end]), []byte("SyntaxError:")) {
			continue
		}
		startIndex := max(0, index-3)
		start := lines[startIndex].start
		result = append(result, byteRange{
			start: start, end: line.end, code: CodeCompilerDiagnostic, format: "python_syntax",
		})
		if len(result) == 4 {
			break
		}
	}

	return result
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
