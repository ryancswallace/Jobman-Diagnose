package enrichment

import (
	"context"
	"slices"
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
