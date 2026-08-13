package diagnosis

import (
	"bytes"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestReportCodecRoundTripAndEvidenceValidation(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	failureEvidence, err := CoreFailureEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	confidence, err := NewConfidence(82, "The exit status was observed directly.")
	if err != nil {
		t.Fatal(err)
	}
	report, err := Seal(Report{
		GeneratedAt:        time.Date(2026, 8, 8, 13, 0, 0, 0, time.UTC),
		AnalysisEvidenceID: failureEvidence.AnalysisEvidenceID, CoreEvidenceID: evidence.EvidenceID,
		Versions: Versions{
			CompanionVersion: "test", EngineVersion: EngineVersion, JobmanVersion: evidence.Source.JobmanVersion,
			EvidenceSchemaVersion: evidence.SchemaVersion, ReportSchemaVersion: SchemaVersion,
		},
		Subject: Subject{
			JobID: evidence.Subject.JobID, JobRevision: evidence.Subject.JobRevision,
			SelectedRuns: evidence.Subject.SelectedRuns, Phase: evidence.Subject.Phase, Outcome: evidence.Subject.Outcome,
		},
		Mode: ModeDeterministic, PrimaryFindingID: "finding:1",
		Findings: []Finding{{
			ID: "finding:1", Code: "core.nonzero_exit", Category: "process", Severity: SeverityError,
			Summary: "The target exited unsuccessfully", Explanation: "The exit code is nonzero.", Confidence: confidence,
			SupportingEvidence:    []string{"ev:run:00000000000000000001:exit:code"},
			ContradictingEvidence: []string{}, Analyzer: "test",
		}},
		Actions: []Action{},
		Retry: RetryAdvice{
			Verdict: RetryAfterChange, Confidence: confidence, Rationale: "Change the target before retrying.",
			SupportingEvidence: []string{"ev:run:00000000000000000001:exit:code"},
		},
		Citations: []Citation{{
			EvidenceID: "ev:run:00000000000000000001:exit:code", Code: "jobman.run.exit.code",
			Summary: "The observed exit code.", Kind: "item",
		}},
		MissingEvidence: []MissingEvidence{}, Warnings: []Warning{},
		Disclosure: DisclosureManifest{Locality: ProviderNotUsed},
	})
	if err != nil {
		t.Fatal(err)
	}
	const expectedReportID = "sha256:110f91a253824f18a8b4ffc6b678fd3745eff8d5a6bbd08de55829ce75751b2d"
	if report.ReportID != expectedReportID {
		t.Fatalf("report ID = %q, want stable contract digest %q", report.ReportID, expectedReportID)
	}
	if validationErr := ValidateAgainstEvidence(report, failureEvidence); validationErr != nil {
		t.Fatal(validationErr)
	}
	var encoded bytes.Buffer
	if encodeErr := Encode(&encoded, report); encodeErr != nil {
		t.Fatal(encodeErr)
	}
	decoded, err := Decode(&encoded, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ReportID != report.ReportID {
		t.Fatalf("decoded report ID = %q, want %q", decoded.ReportID, report.ReportID)
	}

	wrongTarget := report
	wrongTarget.Actions = []Action{{
		ID: "action:1", Code: "inspect_job", Kind: ActionInspect,
		Summary: "Inspect the job", Description: "Inspect only the selected job.",
		SupportingEvidence: []string{"ev:run:00000000000000000001:exit:code"},
		Execution:          ActionExecutionReadOnly,
		Arguments:          []string{"jobman", "show", "job", "01980f4c-7b2a-7a6f-8c10-ffffffffffff"},
	}}
	wrongTarget, err = Seal(wrongTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgainstEvidence(wrongTarget, failureEvidence); err == nil {
		t.Fatal("ValidateAgainstEvidence(action for another job) error = nil")
	}
}

func TestDecodeRejectsDuplicateKeys(t *testing.T) {
	t.Parallel()

	if _, err := Decode(bytes.NewBufferString(`{"kind":"jobman.diagnosis_report","kind":"duplicate"}`), DecodeLimits{}); err == nil {
		t.Fatal("Decode(duplicate key) error = nil")
	}
}
