package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

func TestNewAndDiagnoseRejectInvalidBoundaries(t *testing.T) {
	t.Parallel()

	if _, err := New(" ", time.Now); err == nil {
		t.Fatal("New(empty version) error = nil")
	}
	if _, err := New("test", nil); err == nil {
		t.Fatal("New(nil clock) error = nil")
	}
	diagnostician, err := New("test", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostician.Diagnose(nil, diagnosis.FailureEvidence{}); err == nil { //nolint:staticcheck // Explicit nil-context contract.
		t.Fatal("Diagnose(nil context) error = nil")
	}
	if _, err := diagnostician.Diagnose(t.Context(), diagnosis.FailureEvidence{}); err == nil {
		t.Fatal("Diagnose(invalid evidence) error = nil")
	}
}

func TestCoreFailureCandidateCatalog(t *testing.T) {
	t.Parallel()

	classes := []string{
		"executable_not_found", "working_directory_missing", "permission_denied", "job_timeout",
		"run_timeout", "timeout", "user_cancellation", "ownership_lost", "supervisor_claim_expired",
		"signal_termination", "nonzero_exit", "target_start_failed", "submission_failed",
		"wait_evaluation_error", "log_recording_degraded", "future_failure",
	}
	for _, class := range classes {
		candidate := coreFailureCandidate(class, "ev:class", "ev:related")
		if candidate.finding.Code == "" || len(candidate.finding.SupportingEvidence) != 2 {
			t.Fatalf("coreFailureCandidate(%q) = %#v", class, candidate)
		}
	}
	if exactCandidate(101, "code", "category", diagnosis.SeverityError, "summary", "explanation", nil).finding.Confidence.Score != 100 {
		t.Fatal("exactCandidate() did not cap confidence")
	}
	if observedCandidate(100, "code", "category", diagnosis.SeverityError, "summary", "explanation", nil).finding.Confidence.Score != 85 {
		t.Fatal("observedCandidate() did not cap confidence")
	}
}

func TestActionAndRetryCatalogs(t *testing.T) {
	t.Parallel()

	view := evidenceView{
		evidence: diagnostic.Evidence{Subject: diagnostic.Subject{JobID: "job", SelectedRuns: []uint64{7}, Phase: "paused"}},
		byCode:   map[string][]diagnostic.Item{},
	}
	codes := []string{
		"core.intentional_false", "core.executable_not_found", "core.working_directory_missing",
		"core.permission_denied", "target.permission_message", "core.timeout",
		"core.supervisor_ownership_lost", "core.user_cancellation", "target.python_exception",
		"target.go_panic", "target.jvm_exception", "target.compiler_error",
		"secondary.notification_failed", "core.signal_termination", "core.no_target_failure",
		"core.insufficient_structured_evidence", "future.code",
	}
	for _, code := range codes {
		primary := observedCandidate(60, code, "process", diagnosis.SeverityError, "Summary", "Explanation", []string{"ev:item"})
		actions := actionsFor(primary, view)
		if len(actions) == 0 {
			t.Fatalf("actionsFor(%q) was empty", code)
		}
		for _, action := range actions {
			if action.SafeToAutomate {
				t.Fatalf("actionsFor(%q) returned automatic action", code)
			}
		}
		retry := retryFor(primary, view)
		if retry.Verdict == "" || len(retry.Reasons) < 2 {
			t.Fatalf("retryFor(%q) = %#v", code, retry)
		}
	}
	view.evidence.Subject.SelectedRuns = nil
	if selectedRun(view) != 1 {
		t.Fatal("selectedRun(empty) != 1")
	}
}

func TestArtifactAndEnrichmentCandidateCatalogs(t *testing.T) {
	t.Parallel()

	messages := []string{
		"Traceback (most recent call last):\nValueError: bad",
		"write failed: no space left on device",
		"helper: command not found",
		"open file: permission denied",
		"dial: connection refused",
	}
	artifacts := make([]diagnostic.Artifact, len(messages))
	for index, message := range messages {
		artifacts[index] = diagnostic.Artifact{ID: "artifact:" + string(rune('a'+index)), Data: []byte(message)}
	}
	view := evidenceView{
		evidence:         diagnostic.Evidence{Artifacts: artifacts},
		byEnrichmentCode: map[string][]diagnosis.EnrichmentItem{},
	}
	if candidates := artifactCandidates(t.Context(), view); len(candidates) != len(messages) {
		t.Fatalf("artifactCandidates() = %#v", candidates)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if candidates := artifactCandidates(canceled, view); len(candidates) != 0 {
		t.Fatalf("artifactCandidates(canceled) = %#v", candidates)
	}
	items := []diagnosis.EnrichmentItem{
		{ID: "analysis:1", Code: "enrichment.traceback.python"},
		{ID: "analysis:2", Code: "enrichment.traceback.go_panic"},
		{ID: "analysis:3", Code: "enrichment.traceback.jvm"},
		{ID: "analysis:4", Code: "enrichment.compiler.diagnostic"},
		{ID: "analysis:5", Code: "enrichment.future"},
	}
	view.failure.Enrichment = items
	view.enrichment = map[string]diagnosis.EnrichmentItem{}
	if candidates := enrichmentCandidates(view); len(candidates) != 4 {
		t.Fatalf("enrichmentCandidates() = %#v", candidates)
	}
}

func TestLimitationsAndCitationHelpers(t *testing.T) {
	t.Parallel()

	view := evidenceView{
		evidence: diagnostic.Evidence{
			Artifacts:   []diagnostic.Artifact{{ID: "artifact", Role: "target_stderr", Stream: "stderr", Run: 1}},
			Consistency: diagnostic.Consistency{ActiveStateMayHaveAdvanced: true, Artifacts: diagnostic.ArtifactsPointInTime},
		},
		omissions: map[string]diagnostic.Omission{}, itemsByID: map[string]diagnostic.Item{},
		artifacts: map[string]diagnostic.Artifact{"artifact": {ID: "artifact", Role: "target_stderr", Stream: "stderr", Run: 1}},
		enrichment: map[string]diagnosis.EnrichmentItem{"analysis": {
			ID: "analysis", Code: "enrichment.future", SourceArtifactID: "artifact", ByteStart: 1, ByteEnd: 2,
		}},
	}
	for _, code := range []string{
		diagnostic.OmissionCommandNotRequested, diagnostic.OmissionPathsNotRequested,
		diagnostic.OmissionEnvironmentNamesNotRequested, coreSystemUnavailableOmission,
		diagnostic.OmissionLogContentNotRequested, diagnostic.OmissionLogsPruned,
		diagnostic.OmissionResourceUnavailable, diagnostic.OmissionSimilarNotRequested,
		diagnostic.OmissionSimilarUnavailable, diagnostic.OmissionSimilarPartiallyIndexed,
		diagnostic.OmissionSimilarTruncated,
	} {
		view.omissions[code] = diagnostic.Omission{Code: code}
	}
	primary := observedCandidate(60, "core.nonzero_exit", "process", diagnosis.SeverityError, "Summary", "Explanation", []string{"item"})
	missing, warnings := limitations(view, primary)
	if len(missing) < 7 || len(warnings) < 4 {
		t.Fatalf("limitations() = %#v / %#v", missing, warnings)
	}
	view.omissions = map[string]diagnostic.Omission{diagnostic.OmissionCommandLimitExceeded: {Code: diagnostic.OmissionCommandLimitExceeded}}
	if missing := commandLimitations(view); len(missing) != 1 || !strings.Contains(missing[0].Description, "exceeded") {
		t.Fatalf("commandLimitations(limit) = %#v", missing)
	}

	view.itemsByID["item"] = diagnostic.Item{ID: "item", Code: diagnostic.CodeRunExitCode}
	citations, err := buildCitations(view, []string{"item", "artifact", "analysis"})
	if err != nil || len(citations) != 3 || citations[2].Kind != "enrichment" {
		t.Fatalf("buildCitations() = %#v, %v", citations, err)
	}
	if _, err := buildCitations(view, []string{"missing"}); err == nil {
		t.Fatal("buildCitations(missing) error = nil")
	}
	for _, code := range []string{
		"enrichment.traceback.python", "enrichment.traceback.go_panic", "enrichment.traceback.jvm",
		"enrichment.compiler.diagnostic", "enrichment.future",
	} {
		if enrichmentSummary(code) == "" {
			t.Fatalf("enrichmentSummary(%q) was empty", code)
		}
	}
	if itemSummary("future.code") != "A structured Jobman evidence item." {
		t.Fatal("itemSummary(default) changed")
	}
}

func TestPolicyHelpersHandleInvalidAndValidValues(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("offset", 3600))
	validTime, err := json.Marshal(when)
	if err != nil {
		t.Fatal(err)
	}
	view := evidenceView{byCode: map[string][]diagnostic.Item{
		diagnostic.CodeRuntimeNextRunAt: {
			{ID: "invalid", Value: json.RawMessage(`"bad"`)},
			{ID: "valid", Value: validTime},
		},
	}}
	value, id := nextRunAt(view)
	if value == nil || id != "valid" || value.Location() != time.UTC {
		t.Fatalf("nextRunAt() = %v, %q", value, id)
	}
	view.byCode[diagnostic.CodeRuntimeNextRunAt] = nil
	if value, id := nextRunAt(view); value != nil || id != "" {
		t.Fatalf("nextRunAt(empty) = %v, %q", value, id)
	}
	items := []diagnostic.Item{{Value: json.RawMessage(`"bad"`)}, {Value: json.RawMessage(`4`)}}
	if lastCounter(items) != 4 {
		t.Fatal("lastCounter() did not skip invalid data")
	}
}
