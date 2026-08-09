package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

func TestHumanIncludesSecondaryFindingsAndTheirEvidence(t *testing.T) {
	t.Parallel()

	report := diagnosis.Report{
		ReportID: "sha256:test", Subject: diagnosis.Subject{JobID: "job", Phase: "completed"},
		PrimaryFindingID: "finding:primary",
		Findings: []diagnosis.Finding{
			{ID: "finding:primary", Summary: "Primary", Explanation: "Primary explanation", Confidence: diagnosis.Confidence{Score: 80, Band: "high"}},
			{
				ID: "finding:history", Summary: "Matching history", Explanation: "It happened before",
				Confidence:         diagnosis.Confidence{Score: 90, Band: "very_high"},
				SupportingEvidence: []string{"ev:similar:000000"},
			},
		},
		Retry: diagnosis.RetryAdvice{Verdict: diagnosis.RetryAfterChange, Rationale: "Change it"},
		Citations: []diagnosis.Citation{{
			EvidenceID: "ev:similar:000000", Summary: "An exact matching failure.",
		}},
	}
	var output bytes.Buffer
	if err := Human(&output, report); err != nil {
		t.Fatalf("Human() error = %v", err)
	}
	for _, wanted := range []string{"ADDITIONAL FINDINGS", "Matching history", "ev:similar:000000"} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("Human() output = %q, want %q", output.String(), wanted)
		}
	}
}
