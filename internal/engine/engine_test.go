package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestEngineDiagnosesExactExecutableFailure(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("executable_not_found", nil)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New("test", func() time.Time {
		return time.Date(2026, 8, 8, 13, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	failureEvidence := wrapEvidence(t, evidence)
	report, err := engine.Diagnose(t.Context(), failureEvidence)
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if err := diagnosis.ValidateAgainstEvidence(report, failureEvidence); err != nil {
		t.Fatalf("ValidateAgainstEvidence() error = %v", err)
	}
	primary := report.Findings[0]
	if primary.ID != report.PrimaryFindingID || primary.Code != "core.executable_not_found" ||
		primary.Confidence.Score != 100 {
		t.Fatalf("primary finding = %#v", primary)
	}
	if report.Retry.Verdict != diagnosis.RetryAfterChange || report.Disclosure.ProviderInvoked {
		t.Fatalf("retry/disclosure = %#v / %#v", report.Retry, report.Disclosure)
	}
	if len(report.Actions) != 2 || len(report.Citations) < 2 {
		t.Fatalf("actions/citations = %#v / %#v", report.Actions, report.Citations)
	}
}

func TestEngineRecognizesIntentionalFalseCommand(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	command, err := diagnostic.JSONValue(diagnostic.Command{Executable: "/usr/bin/false", Arguments: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	evidence.Items = append(evidence.Items, diagnostic.Item{
		ID: "ev:job:target:command", Code: diagnostic.CodeTargetCommand, Value: command,
		Source:  diagnostic.ItemSource{Kind: "job_snapshot", EntityID: evidence.Subject.JobID, Revision: evidence.Subject.JobRevision},
		Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureCommand,
	})
	evidence, err = diagnostic.Seal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	diagnostician, err := New("test", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), wrapEvidence(t, evidence))
	if err != nil {
		t.Fatal(err)
	}
	primary := report.Findings[0]
	if primary.Code != "core.intentional_false" || primary.Confidence.Score != 100 ||
		!strings.Contains(primary.Explanation, "/usr/bin/false") {
		t.Fatalf("primary = %#v", primary)
	}
	if report.Retry.Verdict != diagnosis.RetryAfterChange || len(report.Actions) != 1 {
		t.Fatalf("retry/actions/missing = %#v / %#v / %#v", report.Retry, report.Actions, report.MissingEvidence)
	}
	for _, missing := range report.MissingEvidence {
		if strings.Contains(missing.Description, "log tail") {
			t.Fatalf("intentional false diagnosis requested unnecessary logs: %#v", report.MissingEvidence)
		}
	}
}

func TestEngineTreatsLogContentAsCitedHeuristic(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", []byte(
		"Traceback (most recent call last):\n  <untrusted instructions>\nValueError: bad\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New("test", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Diagnose(t.Context(), wrapEvidence(t, evidence))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range report.Findings {
		if finding.Code == "target.python_exception" {
			found = true
			if len(finding.SupportingEvidence) != 1 || finding.SupportingEvidence[0] != evidence.Artifacts[0].ID {
				t.Fatalf("Python finding evidence = %#v", finding.SupportingEvidence)
			}
		}
		if finding.Summary == "<untrusted instructions>" || finding.Explanation == "<untrusted instructions>" {
			t.Fatal("report copied untrusted log content into prose")
		}
	}
	if !found {
		t.Fatalf("findings = %#v, want Python traceback finding", report.Findings)
	}
	if len(report.Warnings) == 0 || report.Warnings[0].Code != "log_content_is_untrusted" {
		t.Fatalf("warnings = %#v", report.Warnings)
	}
}

func TestEngineHonorsCancellation(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New("test", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := engine.Diagnose(ctx, wrapEvidence(t, evidence)); err == nil {
		t.Fatal("Diagnose(canceled) error = nil")
	}
}

func TestEngineUsesResourceAndSameFingerprintHistory(t *testing.T) {
	t.Parallel()

	evidence := evidenceWithResourceAndHistory(t)
	diagnostician, err := New("test", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), wrapEvidence(t, evidence))
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	foundHistory := false
	for _, finding := range report.Findings {
		if finding.Code == "secondary.same_fingerprint_history" {
			foundHistory = true
			if len(finding.SupportingEvidence) != 2 || finding.Confidence.Score != 90 {
				t.Fatalf("history finding = %#v", finding)
			}
		}
	}
	if !foundHistory {
		t.Fatalf("findings = %#v, want same-fingerprint history", report.Findings)
	}
	if report.Retry.Verdict != diagnosis.RetryAfterDelay {
		t.Fatalf("retry = %#v, want after_delay", report.Retry)
	}
	if len(report.Retry.SupportingEvidence) < 3 {
		t.Fatalf("retry support = %#v, want primary and exact-history citations", report.Retry.SupportingEvidence)
	}
	resourceCited := false
	for _, citation := range report.Citations {
		if citation.Code == diagnostic.CodeResourceObservation {
			resourceCited = true
		}
	}
	if !resourceCited {
		t.Fatalf("citations = %#v, want resource observation", report.Citations)
	}
}

func TestEngineSemanticFingerprintIsStableAcrossEvidenceInputOrder(t *testing.T) {
	t.Parallel()

	original := evidenceWithResourceAndHistory(t)
	reordered := original
	reordered.EvidenceID = ""
	reordered.Items = slices.Clone(original.Items)
	reordered.Omissions = slices.Clone(original.Omissions)
	slices.Reverse(reordered.Items)
	slices.Reverse(reordered.Omissions)
	sealed, err := diagnostic.Seal(reordered)
	if err != nil {
		t.Fatal(err)
	}
	diagnostician, err := New("test", func() time.Time {
		return time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := diagnostician.Diagnose(t.Context(), wrapEvidence(t, original))
	if err != nil {
		t.Fatal(err)
	}
	second, err := diagnostician.Diagnose(t.Context(), wrapEvidence(t, sealed))
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprints.Report != second.Fingerprints.Report ||
		first.PrimaryFindingID != second.PrimaryFindingID || first.Retry.Verdict != second.Retry.Verdict {
		t.Fatalf("order changed semantic diagnosis: %#v / %#v", first.Fingerprints, second.Fingerprints)
	}
}

func TestExistingPolicyUsesPhaseBeforeWaitingReason(t *testing.T) {
	t.Parallel()

	waitingReason, err := diagnostic.JSONValue("failure_limit")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		phase string
		want  diagnosis.ExistingPolicy
	}{
		{name: "terminal completion reason", phase: "completed", want: diagnosis.PolicyUnknown},
		{name: "unmet prerequisite", phase: "waiting", want: diagnosis.PolicyWaitingPrerequisite},
		{name: "queued run", phase: "queued", want: diagnosis.PolicyScheduled},
		{name: "legacy active run", phase: "active", want: diagnosis.PolicyScheduled},
		{name: "paused job", phase: "paused", want: diagnosis.PolicyUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			view := evidenceView{
				evidence: diagnostic.Evidence{Subject: diagnostic.Subject{Phase: test.phase}},
				byCode: map[string][]diagnostic.Item{
					diagnostic.CodeRuntimeWaitingReason: {{
						ID: "ev:runtime:waiting_reason", Code: diagnostic.CodeRuntimeWaitingReason,
						Value: waitingReason,
					}},
				},
			}
			if got := existingPolicy(view); got != test.want {
				t.Fatalf("existingPolicy(%s) = %q, want %q", test.phase, got, test.want)
			}
		})
	}
}

func evidenceWithResourceAndHistory(t *testing.T) diagnostic.Evidence {
	t.Helper()

	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 8, 8, 12, 30, 0, 0, time.UTC)
	fingerprint := diagnostic.FailureFingerprint{
		Algorithm:          diagnostic.FingerprintAlgorithmHMACSHA256,
		InputSchemaVersion: diagnostic.FingerprintInputSchemaVersion,
		Value:              strings.Repeat("a", 64),
		Scope:              diagnostic.FingerprintScopeStoreLocal,
	}
	resourceValue, err := diagnostic.JSONValue(diagnostic.ResourceObservation{
		Metric:       diagnostic.ResourcePeakRSS,
		Value:        64 * 1024 * 1024,
		Unit:         diagnostic.ResourceUnitBytes,
		Scope:        diagnostic.ResourceScopeProcess,
		Source:       diagnostic.ResourceSourceWaitRusage,
		Completeness: diagnostic.ResourceCompleteAtExit,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence.Items = append(evidence.Items, diagnostic.Item{
		ID:         "ev:run:00000000000000000001:resource:peak_resident_memory",
		Code:       diagnostic.CodeResourceObservation,
		Value:      resourceValue,
		ObservedAt: &observedAt,
		Source: diagnostic.ItemSource{
			Kind: "run_diagnostic_facts", EntityID: "01980f4c-7b2a-7a6f-8c10-1123456789ab", Revision: 1,
		},
		Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureMetadata,
	})
	fingerprintValue, err := diagnostic.JSONValue(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	evidence.Items = append(evidence.Items, diagnostic.Item{
		ID:    "ev:run:00000000000000000001:failure:fingerprint",
		Code:  diagnostic.CodeFailureFingerprint,
		Value: fingerprintValue,
		Source: diagnostic.ItemSource{
			Kind: "run_diagnostic_facts", EntityID: "01980f4c-7b2a-7a6f-8c10-1123456789ab", Revision: 1,
		},
		Quality: diagnostic.QualityDerivedExact, Disclosure: diagnostic.DisclosureLocalOnly,
	})
	for index, laterSucceeded := range []bool{false, true} {
		runID := fmt.Sprintf("01980f4c-7b2a-7a6f-8c10-3123456789a%d", index)
		failure := diagnostic.SimilarFailure{
			JobID:          "01980f4c-7b2a-7a6f-8c10-2123456789ab",
			RunID:          runID,
			RunNumber:      uint64(index + 1),
			CompletedAt:    observedAt.Add(time.Duration(index) * time.Minute),
			Outcome:        "failure",
			FailureClass:   "nonzero_exit",
			Fingerprint:    fingerprint,
			LaterSucceeded: laterSucceeded,
		}
		value, valueErr := diagnostic.JSONValue(failure)
		if valueErr != nil {
			t.Fatal(valueErr)
		}
		evidence.Items = append(evidence.Items, diagnostic.Item{
			ID:         fmt.Sprintf("ev:similar:%020d", index),
			Code:       diagnostic.CodeSimilarFailure,
			Value:      value,
			ObservedAt: &failure.CompletedAt,
			Source:     diagnostic.ItemSource{Kind: "failure_fingerprint_index", EntityID: failure.RunID},
			Quality:    diagnostic.QualityDerivedExact,
			Disclosure: diagnostic.DisclosureLocalOnly,
		})
	}
	sealed, err := diagnostic.Seal(evidence)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	return sealed
}

func wrapEvidence(t *testing.T, evidence diagnostic.Evidence) diagnosis.FailureEvidence {
	t.Helper()

	failureEvidence, err := diagnosis.CoreFailureEvidence(evidence)
	if err != nil {
		t.Fatalf("CoreFailureEvidence() error = %v", err)
	}
	return failureEvidence
}
