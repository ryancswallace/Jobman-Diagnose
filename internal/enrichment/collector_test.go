package enrichment

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestCollectExtractsBoundedAttributedStructures(t *testing.T) {
	t.Parallel()

	log := []byte("prefix\n" +
		"Traceback (most recent call last):\n  File \"app.py\", line 1, in <module>\nValueError: bad\n\n" +
		"panic: broken\n\ngoroutine 1 [running]:\nmain.main()\n\n" +
		"java.lang.IllegalStateException: bad\n\tat example.Main.main(Main.java:1)\n\n" +
		"src/main.c:12:7: error: expected expression\n")
	core, err := testevidence.Failed("nonzero_exit", log)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	if first.AnalysisEvidenceID != second.AnalysisEvidenceID {
		t.Fatalf("analysis evidence IDs differ: %q != %q", first.AnalysisEvidenceID, second.AnalysisEvidenceID)
	}
	wantCodes := []string{CodePythonTraceback, CodeGoPanic, CodeJVMException, CodeCompilerDiagnostic}
	gotCodes := make([]string, 0, len(first.Enrichment))
	for _, item := range first.Enrichment {
		gotCodes = append(gotCodes, item.Code)
		if item.ByteStart >= item.ByteEnd || item.ByteEnd > uint64(len(log)) ||
			item.SourceArtifactID != core.Artifacts[0].ID {
			t.Fatalf("invalid enrichment range: %#v", item)
		}
	}
	for _, code := range wantCodes {
		if !slices.Contains(gotCodes, code) {
			t.Fatalf("enrichment codes = %v, missing %q", gotCodes, code)
		}
	}
	if err := diagnosis.VerifyFailureEvidence(first); err != nil {
		t.Fatalf("VerifyFailureEvidence() error = %v", err)
	}
}

func TestCollectHonorsCancellationAndRejectsNilContext(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", []byte("panic: bad\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(nil, core); err == nil { //nolint:staticcheck // Explicitly verifies nil-context rejection.
		t.Fatal("Collect(nil) error = nil")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Collect(ctx, core); err == nil {
		t.Fatal("Collect(cancelled) error = nil")
	}
}

func TestCollectAttributesBoundedCausalMessageLines(t *testing.T) {
	t.Parallel()

	log := []byte("starting request\nsynchronize inventory: GET https://inventory.internal/snapshot: context deadline exceeded\n")
	core, err := testevidence.Failed("nonzero_exit", log)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(evidence.Enrichment, func(item diagnosis.EnrichmentItem) bool {
		return item.Code == CodeCausalMessage
	})
	if index < 0 {
		t.Fatalf("causal-message enrichment missing: %#v", evidence.Enrichment)
	}
	item := evidence.Enrichment[index]
	if item.Format != "deadline_exceeded" {
		t.Fatalf("causal-message format = %q", item.Format)
	}
	if got := string(log[item.ByteStart:item.ByteEnd]); got != "synchronize inventory: GET https://inventory.internal/snapshot: context deadline exceeded\n" {
		t.Fatalf("causal-message range = %q", got)
	}
}

func TestClassifyDiagnosticUsesControlledSpecificFormats(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"listen tcp: address already in use":               "address_in_use",
		"HTTP 401 Unauthorized":                            "authentication_denied",
		"required environment variable API_URL is missing": "configuration_missing",
		"connect ECONNREFUSED 127.0.0.1:5432":              "connection_refused",
		"invalid decimal amount 1.2O":                      "data_validation",
		"ERROR: deadlock detected":                         "database_deadlock",
		"duplicate key violates unique constraint":         "database_unique_violation",
		"request: context deadline exceeded":               "deadline_exceeded",
		"Could not find artifact example:rules:jar:1":      "dependency_missing",
		"lookup inventory.internal: no such host":          "dns_resolution_failed",
		"accept4: too many open files":                     "file_descriptor_exhausted",
		"ld: undefined reference to symbol":                "linker_undefined_reference",
		"apply migrations 009 through 011":                 "migration_required",
		"open input.csv: no such file or directory":        "missing_file",
		"helper: command not found":                        "nested_command_missing",
		"open output.csv: permission denied":               "permission_denied",
		"HTTP 429 Too Many Requests":                       "rate_limited",
		"write config: read-only file system":              "read_only_filesystem",
		"HTTP 503 Service Unavailable":                     "service_unavailable",
		"write output: no space left on device":            "storage_exhausted",
		"x509: certificate signed by unknown authority":    "tls_verification_failed",
	}
	for message, expected := range tests {
		if actual := ClassifyDiagnostic([]byte(message)); actual != expected {
			t.Errorf("ClassifyDiagnostic(%q) = %q, want %q", message, actual, expected)
		}
	}
	if actual := ClassifyDiagnostic([]byte("worker stopped unexpectedly")); actual != "" {
		t.Fatalf("ClassifyDiagnostic(ambiguous) = %q", actual)
	}
}

func TestCollectExtractsEveryPythonCauseChainMember(t *testing.T) {
	t.Parallel()

	log := []byte("Traceback (most recent call last):\n" +
		"  File \"pipeline.py\", line 8, in parse\n" +
		"decimal.InvalidOperation: ConversionSyntax\n\n" +
		"The above exception was the direct cause of the following exception:\n\n" +
		"Traceback (most recent call last):\n" +
		"  File \"pipeline.py\", line 20, in run\n" +
		"RecordTransformError: record 42 has invalid decimal amount '1,2O'\n")
	core, err := testevidence.Failed("nonzero_exit", log)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Collect(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	var tracebacks []diagnosis.EnrichmentItem
	for _, item := range evidence.Enrichment {
		if item.Code == CodePythonTraceback {
			tracebacks = append(tracebacks, item)
		}
	}
	if len(tracebacks) != 2 {
		t.Fatalf("python traceback enrichments = %#v", tracebacks)
	}
	if first := string(log[tracebacks[0].ByteStart:tracebacks[0].ByteEnd]); !strings.Contains(first, "InvalidOperation") {
		t.Fatalf("first traceback = %q", first)
	}
	if second := string(log[tracebacks[1].ByteStart:tracebacks[1].ByteEnd]); !strings.Contains(second, "RecordTransformError") {
		t.Fatalf("second traceback = %q", second)
	}
}
