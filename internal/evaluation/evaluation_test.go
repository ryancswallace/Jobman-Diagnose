package evaluation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/internal/engine"
)

func TestDeterministicCorpus(t *testing.T) {
	t.Parallel()

	corpus, err := Load(filepath.Join("..", "..", "testdata", "evaluation", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	diagnostician, err := engine.New("evaluation-test", func() time.Time {
		return time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := Run(t.Context(), corpus, diagnostician, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != summary.Cases || summary.Cases != 19 ||
		summary.Metrics.PrimaryCodePrecision != 1 || summary.Metrics.UnsupportedClaimRate != 0 ||
		summary.Metrics.CitationValidity != 1 || summary.Metrics.SafeActionRate != 1 ||
		summary.Metrics.RetryAdviceAccuracy != 1 || summary.Metrics.DeterministicStability != 1 ||
		summary.Metrics.ProviderFallbackRate != 0 || summary.Metrics.GeneratedSpecificity != 1 ||
		summary.Metrics.GeneratedSpecificityCases != 0 {
		t.Fatalf("evaluation summary = %#v", summary)
	}
}

func TestLoadRejectsUnknownAndEscapingInput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "evaluation", "manifest.json")
	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	encoded := []byte(`{
  "kind":"jobman.diagnosis_evaluation_corpus",
  "schema_version":2,
  "cases":[{
    "name":"escape",
    "evidence":"../../outside.json",
    "accepted_primary_codes":["core.failure"],
    "allowed_generated_codes":[],
    "required_finding_codes":[],
    "forbidden_finding_codes":[],
    "required_action_codes":[],
    "forbidden_action_codes":[],
    "expected_retry":"unknown",
    "expected_existing_policy":"unknown",
    "minimum_confidence":0,
    "maximum_confidence":100
  }]
}`)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load(escaping evidence) error = nil")
	}
}
