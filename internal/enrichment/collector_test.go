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
	if got := string(log[item.ByteStart:item.ByteEnd]); got != "synchronize inventory: GET https://inventory.internal/snapshot: context deadline exceeded\n" {
		t.Fatalf("causal-message range = %q", got)
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
