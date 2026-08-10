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
		current := coreFailureCandidate(class, "ev:class", "ev:related")
		if current.finding.Code == "" || len(current.finding.SupportingEvidence) != 2 {
			t.Fatalf("coreFailureCandidate(%q) = %#v", class, current)
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

func TestEvidenceViewSelectionAndAnalyzerCatalog(t *testing.T) {
	t.Parallel()

	collector1 := diagnosis.AnalyzerDescriptor{Name: "collector", Version: "1"}
	collector2 := diagnosis.AnalyzerDescriptor{Name: "collector", Version: "2"}
	evidence := diagnosis.FailureEvidence{
		Core: diagnostic.Evidence{
			Subject: diagnostic.Subject{SelectedRuns: []uint64{2}},
			Items: []diagnostic.Item{
				{ID: "ev:run:00000000000000000001:value", Code: "code"},
				{ID: "ev:job:value", Code: "code"},
			},
			Artifacts: []diagnostic.Artifact{{ID: "artifact"}},
			Omissions: []diagnostic.Omission{{Code: "omission"}},
		},
		Enrichment: []diagnosis.EnrichmentItem{
			{ID: "one", Code: "enrichment", Collector: collector1},
			{ID: "two", Code: "enrichment", Collector: collector1},
			{ID: "three", Code: "enrichment", Collector: collector2},
		},
	}
	view := newEvidenceView(evidence)
	if selected := view.primaryItems("code"); len(selected) != 1 || selected[0].ID != "ev:job:value" {
		t.Fatalf("primaryItems(selected run fallback) = %#v", selected)
	}
	view.evidence.Subject.SelectedRuns = nil
	if selected := view.primaryItems("code"); len(selected) != 2 {
		t.Fatalf("primaryItems(all runs) = %#v", selected)
	}
	descriptors := analyzerDescriptors(evidence)
	if len(descriptors) != 3 || descriptors[1] != collector1 || descriptors[2] != collector2 {
		t.Fatalf("analyzerDescriptors() = %#v", descriptors)
	}
}

func TestCandidateSelectionHelperBranches(t *testing.T) {
	t.Parallel()

	view := evidenceView{evidence: diagnostic.Evidence{Subject: diagnostic.Subject{Outcome: "success"}}, byCode: map[string][]diagnostic.Item{
		diagnostic.CodeJobOutcome: {{ID: "ev:job:outcome"}},
	}}
	state := stateCandidate(view)
	if state.finding.Code != "core.no_target_failure" || len(state.finding.SupportingEvidence) != 1 {
		t.Fatalf("stateCandidate(success) = %#v", state)
	}

	invalidHistory := evidenceView{byCode: map[string][]diagnostic.Item{
		diagnostic.CodeSimilarFailure: {{ID: "invalid", Value: json.RawMessage(`{}`)}},
	}}
	if _, ok := sameFingerprintHistoryCandidate(invalidHistory); ok {
		t.Fatal("sameFingerprintHistoryCandidate(invalid) returned a candidate")
	}

	repeated := evidenceView{byCode: map[string][]diagnostic.Item{
		diagnostic.CodeFailureClass: {
			{ID: "ev:job:class", Value: json.RawMessage(`{"class":"ignored"}`)},
			{ID: "ev:run:1:class", Value: json.RawMessage(`{"class":"same"}`)},
			{ID: "ev:run:2:class", Value: json.RawMessage(`{"class":"same"}`)},
		},
	}}
	if got := repeatedFailureEvidence(repeated); got != "ev:run:2:class" {
		t.Fatalf("repeatedFailureEvidence() = %q", got)
	}
	secondary := secondaryCandidates(repeated)
	if len(secondary) != 1 || secondary[0].finding.Code != "secondary.repeated_failure" {
		t.Fatalf("secondaryCandidates(repeated) = %#v", secondary)
	}

	left := observedCandidate(20, "b", "state", diagnosis.SeverityWarning, "B", "B", nil)
	right := observedCandidate(10, "a", "state", diagnosis.SeverityWarning, "A", "A", nil)
	if compareCandidates(left, right) != -1 || compareCandidates(right, left) != 1 {
		t.Fatal("compareCandidates() priority ordering changed")
	}
	right.priority = left.priority
	if compareCandidates(left, right) <= 0 {
		t.Fatal("compareCandidates() code tie-break changed")
	}
	if got := uniqueCandidates([]candidate{left, left, right}); len(got) != 2 {
		t.Fatalf("uniqueCandidates() = %#v", got)
	}
	if firstItemID(nil, []diagnostic.Item{{ID: "second"}}) != "second" || firstItemID(nil) != "" {
		t.Fatal("firstItemID() fallback changed")
	}
	if len(optionalEvidence("")) != 0 || len(optionalEvidence("item")) != 1 {
		t.Fatal("optionalEvidence() classification changed")
	}
}

func TestAnalyzeSkipsMalformedFactsAndHonorsLateCancellation(t *testing.T) {
	t.Parallel()

	view := evidenceView{
		evidence: diagnostic.Evidence{Subject: diagnostic.Subject{Outcome: "failure"}},
		byCode: map[string][]diagnostic.Item{
			diagnostic.CodeFailureClass: {{ID: "ev:run:1:class", Value: json.RawMessage(`not-json`)}},
		},
		byEnrichmentCode: map[string][]diagnosis.EnrichmentItem{},
	}
	candidates, err := analyze(t.Context(), view)
	if err != nil || len(candidates) != 1 || candidates[0].finding.Code != "core.insufficient_structured_evidence" {
		t.Fatalf("analyze(malformed fact) = %#v, %v", candidates, err)
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := analyze(canceled, view); err == nil {
		t.Fatal("analyze(canceled) error = nil")
	}
}
