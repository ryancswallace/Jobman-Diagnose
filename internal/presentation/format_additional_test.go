package presentation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

//nolint:cyclop,gocognit // The catalog table intentionally verifies every controlled display value together.
func TestFormattingCatalogsRemainReadable(t *testing.T) {
	t.Parallel()

	if titleWords("") != "Unknown" || titleWords("RETRY_AFTER-change") != "Retry after change" {
		t.Fatal("titleWords() did not normalize controlled identifiers")
	}
	for _, value := range []struct{ phase, outcome string }{
		{"completed", "success"},
		{"completed", "failure"},
		{"completed", "timed_out"},
		{"completed", "cancelled"},
		{"completed", "start_failed"},
		{"completed", "lost"},
		{"queued", ""},
		{"custom_phase", ""},
	} {
		if formatState(value.phase, value.outcome) == "" {
			t.Fatalf("formatState(%q, %q) was empty", value.phase, value.outcome)
		}
	}
	for _, value := range []diagnosis.RetryVerdict{
		diagnosis.RetryNow, diagnosis.RetryAfterDelay, diagnosis.RetryAfterChange,
		diagnosis.RetryNo, diagnosis.RetryNotApplicable, diagnosis.RetryUnknown, "future_verdict",
	} {
		if retryVerdictText(value) == "" {
			t.Fatalf("retryVerdictText(%q) was empty", value)
		}
	}
	for _, value := range []diagnosis.ExistingPolicy{
		diagnosis.PolicyScheduled, diagnosis.PolicyBackoff, diagnosis.PolicyWaitingPrerequisite,
		diagnosis.PolicyExhausted, diagnosis.PolicyNonretryable, diagnosis.PolicyNone,
		diagnosis.PolicyUnknown, "future_policy",
	} {
		if existingPolicyText(value) == "" {
			t.Fatalf("existingPolicyText(%q) was empty", value)
		}
	}
	for _, value := range []string{
		"primary_diagnosis_policy", "existing_backoff", "waiting_prerequisite", "retry_budget_exhausted",
		"outcome_nonretryable", "no_existing_retry", "change_required", "delay_recommended",
		"immediate_retry_reasonable", "retry_not_useful", "retry_not_applicable",
		"insufficient_retry_evidence", "future_reason",
	} {
		if retryReasonText(value) == "" {
			t.Fatalf("retryReasonText(%q) was empty", value)
		}
	}
	for _, value := range []diagnosis.AnalysisMode{
		diagnosis.ModeDeterministic, diagnosis.ModeGenerated, diagnosis.ModeMixed, "future_mode",
	} {
		if analysisModeText(value) == "" {
			t.Fatalf("analysisModeText(%q) was empty", value)
		}
	}
	for _, value := range []diagnosis.ActionKind{
		diagnosis.ActionInspect, diagnosis.ActionChange, diagnosis.ActionWait, diagnosis.ActionRetry, "future_action",
	} {
		if actionKindText(value) == "" {
			t.Fatalf("actionKindText(%q) was empty", value)
		}
	}
	if findingSource("generator.proposal/1") != "AI hypothesis" || findingSource("engine/1") != "Deterministic finding" {
		t.Fatal("findingSource() classification changed")
	}
}

func TestFormattingHandlesBoundaryValues(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	appendWrapped(&output, "- ", "  ", "one two three\n\nfour", 8)
	if got := output.String(); !strings.Contains(got, "- one\n  two") || !strings.Contains(got, "\n\n") {
		t.Fatalf("appendWrapped() = %q", got)
	}
	for _, value := range []time.Duration{
		-time.Nanosecond, 500 * time.Microsecond, 1500 * time.Millisecond,
		90 * time.Second, 90 * time.Minute,
	} {
		if friendlyDuration(value) == "" {
			t.Fatalf("friendlyDuration(%s) was empty", value)
		}
	}
	for _, value := range []uint64{0, 1023, 1024, 1024 * 1024, 1024 * 1024 * 1024, 1 << 50} {
		if formatBytes(value) == "" {
			t.Fatalf("formatBytes(%d) was empty", value)
		}
	}
	for _, argument := range []string{"simple", "", "two words", "line\nbreak"} {
		if formatArgument(argument) == "" && argument != "" {
			t.Fatalf("formatArgument(%q) was empty", argument)
		}
	}
	if got := formatCommand(diagnostic.Command{Executable: "/bin/echo", Arguments: []string{"two words"}}); got != `/bin/echo "two words"` {
		t.Fatalf("formatCommand() = %q", got)
	}

	report := diagnosis.Report{Disclosure: diagnosis.DisclosureManifest{
		Provider: "adapter", Model: "model", Locality: "local", Profile: "default",
	}}
	if got := formatProvider(report); got != "adapter/model (local; profile default)" {
		t.Fatalf("formatProvider() = %q", got)
	}
	report.Disclosure.Locality, report.Disclosure.Profile = "", ""
	if got := formatProvider(report); got != "adapter/model" {
		t.Fatalf("formatProvider(minimal) = %q", got)
	}
	versions := diagnosis.Versions{
		JobmanVersion: "1", CompanionVersion: "2", EngineVersion: "3",
		EvidenceSchemaVersion: 1, ReportSchemaVersion: 1,
		GenerationRequestSchemaVersion: 1, ProposalSchemaVersion: 1,
	}
	if got := formatVersions(versions); !strings.Contains(got, "generation request schema 1") || !strings.Contains(got, "proposal schema 1") {
		t.Fatalf("formatVersions() = %q", got)
	}
}

func TestResourceAndSystemContextFormatting(t *testing.T) {
	t.Parallel()

	observations := []diagnostic.ResourceObservation{
		{Metric: diagnostic.ResourceCPUUserTime, Value: uint64(time.Second), Unit: diagnostic.ResourceUnitNanoseconds, Scope: "process", Completeness: "complete"},
		{Metric: diagnostic.ResourceCPUSystemTime, Value: ^uint64(0), Unit: diagnostic.ResourceUnitNanoseconds, Scope: "process", Completeness: "partial"},
		{Metric: diagnostic.ResourcePeakRSS, Value: 4096, Unit: diagnostic.ResourceUnitBytes, Scope: "process", Completeness: "complete"},
		{Metric: "custom_metric", Value: 7, Unit: "count", Scope: "job", Completeness: "point_in_time"},
	}
	for _, observation := range observations {
		if got := formatResource(observation); got == "" || !strings.Contains(got, ":") {
			t.Fatalf("formatResource(%#v) = %q", observation, got)
		}
	}
	if got := formatSystemContext(systemContextView{}); got != "Point-in-time system context" {
		t.Fatalf("formatSystemContext(empty) = %q", got)
	}
	if got := formatSystemContext(systemContextView{Filesystem: &filesystemCapacityView{AvailableBytes: 2, TotalBytes: 1}}); got != "Point-in-time system context" {
		t.Fatalf("formatSystemContext(invalid filesystem) = %q", got)
	}
	current := uint64(1024)
	if formatCurrentLimit(nil, nil, false) != "unknown / unknown limit" ||
		formatCurrentLimit(&current, &systemLimitView{Value: 2048}, true) != "1 KiB / 2 KiB" ||
		formatCurrentLimit(&current, &systemLimitView{Unlimited: true}, false) != "1024 / unlimited" {
		t.Fatal("formatCurrentLimit() boundary formatting changed")
	}
	if formatOptionalUint(nil) != "unknown" || formatOptionalUint(&current) != "1024" {
		t.Fatal("formatOptionalUint() boundary formatting changed")
	}
}

func TestHumanFormattingUtilities(t *testing.T) {
	t.Parallel()

	if runLabel(1) != "Run" || runLabel(2) != "Runs" || formatRuns([]uint64{2, 4}) != "2, 4" {
		t.Fatal("run formatting changed")
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(90 * time.Second)
	if got := formatRelativeTime(future, now); !strings.HasPrefix(got, "in 1m30s") {
		t.Fatalf("formatRelativeTime(future) = %q", got)
	}
	if got := formatRelativeTime(now, now); got != now.Format(time.RFC3339) {
		t.Fatalf("formatRelativeTime(past) = %q", got)
	}
	if got := formatConfidence(diagnosis.Confidence{Score: 40, Band: "medium"}, "generator.proposal/1"); !strings.Contains(got, "not calibrated") {
		t.Fatalf("formatConfidence(generated) = %q", got)
	}
	generated := diagnosis.Finding{
		Analyzer:    "generator.proposal/1",
		Explanation: "Root cause: region moon-1 is disabled. Failure path: startup validation rejects it.",
	}
	rootCause, failurePath, ok := generatedCauseDetails(generated)
	if !ok || rootCause != "region moon-1 is disabled." || failurePath != "startup validation rejects it." {
		t.Fatalf("generatedCauseDetails() = %q, %q, %t", rootCause, failurePath, ok)
	}
	for _, malformed := range []diagnosis.Finding{
		{Analyzer: "builtin.rules/1", Explanation: generated.Explanation},
		{Analyzer: "generator.proposal/1", Explanation: "ordinary explanation"},
		{Analyzer: "generator.proposal/1", Explanation: "Root cause:  Failure path: stopped"},
	} {
		if _, _, parsed := generatedCauseDetails(malformed); parsed {
			t.Fatalf("generatedCauseDetails() parsed %#v", malformed)
		}
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
