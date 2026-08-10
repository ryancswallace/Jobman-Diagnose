package presentation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

func TestEvidenceDisplayCatalog(t *testing.T) {
	t.Parallel()

	for _, quality := range []diagnostic.Quality{
		diagnostic.QualityObserved, diagnostic.QualityConfirmed, diagnostic.QualityDerivedExact,
		diagnostic.QualityPointInTime, diagnostic.QualityUnknown, "future_quality",
	} {
		if qualityText(quality) == "" {
			t.Fatalf("qualityText(%q) was empty", quality)
		}
	}
	for _, code := range []string{diagnostic.CodeTargetCommand, diagnostic.CodeWaitCommand, diagnostic.CodeNotifierCommand, "other"} {
		if commandLabel(code) == "" {
			t.Fatalf("commandLabel(%q) was empty", code)
		}
	}
	for _, code := range []string{
		diagnostic.CodeTargetWorkingDirectory, diagnostic.CodeTargetStdinPath, diagnostic.CodeWaitPath,
		diagnostic.CodeNotifierWorkingDirectory, diagnostic.CodeRunResolvedExecutable, "other",
	} {
		if pathLabel(code, "Run 7") == "" {
			t.Fatalf("pathLabel(%q) was empty", code)
		}
	}
	for _, code := range []string{
		diagnostic.CodeTargetEnvironmentNames, diagnostic.CodeWaitEnvironmentNames,
		diagnostic.CodeNotifierEnvironmentNames, "other",
	} {
		if environmentLabel(code) == "" {
			t.Fatalf("environmentLabel(%q) was empty", code)
		}
	}
	if got := environmentDetail("Environment", diagnostic.EnvironmentNames{}); !strings.Contains(got, "no explicit names") {
		t.Fatalf("environmentDetail(empty) = %q", got)
	}
	if got := environmentDetail("Environment", diagnostic.EnvironmentNames{
		Inheritance: "submission", Set: []string{"PATH"}, Unset: []string{"OLD"}, Secret: []string{"TOKEN"},
	}); !strings.Contains(got, "secret-backed: TOKEN") {
		t.Fatalf("environmentDetail(full) = %q", got)
	}
}

func TestEvidenceDetailHelpers(t *testing.T) {
	t.Parallel()

	artifact := diagnostic.Artifact{
		Role: "target_stderr", Stream: "stderr", Run: 2, ContentBytes: 10,
		OriginalBytes: 100, ByteStart: 4, ByteEnd: 14, Truncated: true,
	}
	if got := artifactDetail(artifact); !strings.Contains(got, "run 2 stderr") || !strings.Contains(got, "truncated") {
		t.Fatalf("artifactDetail() = %q", got)
	}
	artifact.Stream, artifact.Run, artifact.OriginalBytes, artifact.Truncated = "", 0, 0, false
	if got := artifactDetail(artifact); !strings.HasPrefix(got, "target_stderr excerpt") {
		t.Fatalf("artifactDetail(minimal) = %q", got)
	}
	if evidenceSubject("ev:run:00000000000000000012:exit") != "Run 12" || evidenceSubject("ev:job:name") != "Job" {
		t.Fatal("evidenceSubject() parsing changed")
	}
	for _, value := range []struct {
		id     string
		prefix string
		ok     bool
	}{
		{"ev:run:7:exit", "ev:run:", true}, {"other", "ev:run:", false}, {"ev:run:7", "ev:run:", false}, {"ev:run:x:exit", "ev:run:", false},
	} {
		_, ok := numericIDSegment(value.id, value.prefix)
		if ok != value.ok {
			t.Fatalf("numericIDSegment(%q) ok = %t", value.id, ok)
		}
	}
	failure := diagnostic.SimilarFailure{
		JobID: "job", RunNumber: 3, FailureClass: "nonzero_exit",
		CompletedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), LaterSucceeded: true,
	}
	if got := similarFailureDetail(failure); !strings.Contains(got, "later run succeeded") {
		t.Fatalf("similarFailureDetail() = %q", got)
	}
	if got := failureClassDetail("Run 1", json.RawMessage(`{"class":"timeout","scope":"job"}`)); got != "Job failure class: timeout" {
		t.Fatalf("failureClassDetail() = %q", got)
	}
	if got := failureClassDetail("Run 1", json.RawMessage(`bad`)); got != "Run 1 failure class" {
		t.Fatalf("failureClassDetail(invalid) = %q", got)
	}
}

//nolint:cyclop // This exercises the complete controlled scalar-rendering boundary in one place.
func TestScalarEvidenceRendering(t *testing.T) {
	t.Parallel()

	values := []struct {
		raw  string
		want string
		ok   bool
	}{
		{`"value"`, "value", true},
		{`true`, "true", true},
		{`12.5`, "12.5", true},
		{`null`, "none", true},
		{`[]`, "", false},
		{`bad`, "", false},
	}
	for _, value := range values {
		got, ok := scalarText(json.RawMessage(value.raw))
		if got != value.want || ok != value.ok {
			t.Fatalf("scalarText(%s) = %q, %t", value.raw, got, ok)
		}
	}
	if labeledString("Label", nil) != "Label" || labeledString("Label", json.RawMessage(`"value"`)) != "Label: value" {
		t.Fatal("labeledString() formatting changed")
	}
	if labeledScalar("Label", json.RawMessage(`[]`)) != "Label" || labeledScalar("Label", json.RawMessage(`4`)) != "Label: 4" {
		t.Fatal("labeledScalar() formatting changed")
	}
	if labeledDuration("Duration", json.RawMessage(`"1500ms"`)) != "Duration: 1.5s" ||
		labeledDuration("Duration", json.RawMessage(`"not-a-duration"`)) != "Duration: not-a-duration" ||
		labeledDuration("Duration", nil) != "Duration" {
		t.Fatal("labeledDuration() formatting changed")
	}
	if labeledEnum("State", json.RawMessage(`"waiting_prerequisite"`)) != "State: waiting prerequisite" || labeledEnum("State", nil) != "State" {
		t.Fatal("labeledEnum() formatting changed")
	}
	if labeledBytes("Size", json.RawMessage(`2048`)) != "Size: 2 KiB" || labeledBytes("Size", json.RawMessage(`"bad"`)) != "Size" {
		t.Fatal("labeledBytes() formatting changed")
	}
	if _, _, ok := safeScalarDetail(diagnosis.Citation{}, json.RawMessage(`[]`)); ok {
		t.Fatal("safeScalarDetail(array) succeeded")
	}
	label, value, ok := safeScalarDetail(diagnosis.Citation{Code: "jobman.run.exit.code"}, json.RawMessage(`2`))
	if !ok || label != "Jobman.run.exit.code" || value != "2" {
		t.Fatalf("safeScalarDetail() = %q, %q, %t", label, value, ok)
	}
	if decode(nil, &struct{}{}) == nil || decode(json.RawMessage(`{}`), nil) == nil {
		t.Fatal("decode() accepted missing input")
	}
}

func TestItemDetailCoversSafeDisplayAllowlist(t *testing.T) {
	t.Parallel()

	view := reportView{}
	citation := diagnosis.Citation{Code: "fallback.code", Summary: "Fallback summary."}
	command := diagnostic.Command{Executable: "/bin/true", Arguments: []string{}}
	environment := diagnostic.EnvironmentNames{Inheritance: "submission", Set: []string{"PATH"}}
	similar := diagnostic.SimilarFailure{
		JobID: "job", RunNumber: 1, FailureClass: "timeout", CompletedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		code  string
		value json.RawMessage
	}{
		{diagnostic.CodeJobName, mustJSON(t, "name")},
		{diagnostic.CodeJobPhase, mustJSON(t, "completed")},
		{diagnostic.CodeJobOutcome, mustJSON(t, "failure")},
		{diagnostic.CodeRunPhase, mustJSON(t, "completed")},
		{diagnostic.CodeRunOutcome, mustJSON(t, "failure")},
		{diagnostic.CodeRunExitCode, mustJSON(t, 2)},
		{diagnostic.CodeRunExitSignal, mustJSON(t, "TERM")},
		{diagnostic.CodeRunExitPlatformReason, mustJSON(t, "signal")},
		{diagnostic.CodeRunStopReason, mustJSON(t, "timeout")},
		{diagnostic.CodeRunTimeoutScope, mustJSON(t, "run")},
		{diagnostic.CodeRunDuration, mustJSON(t, "1500ms")},
		{diagnostic.CodeFailureClass, json.RawMessage(`{"class":"timeout","scope":"run"}`)},
		{diagnostic.CodeResourceObservation, mustJSON(t, diagnostic.ResourceObservation{Metric: diagnostic.ResourcePeakRSS, Value: 1024, Unit: diagnostic.ResourceUnitBytes, Scope: "process", Completeness: "complete"})},
		{coreSystemContextCode, mustJSON(t, systemContextView{})},
		{diagnostic.CodeTargetCommand, mustJSON(t, command)},
		{diagnostic.CodeWaitCommand, mustJSON(t, command)},
		{diagnostic.CodeNotifierCommand, mustJSON(t, command)},
		{diagnostic.CodeTargetWorkingDirectory, mustJSON(t, "/tmp")},
		{diagnostic.CodeTargetStdinPath, mustJSON(t, "/tmp/in")},
		{diagnostic.CodeWaitPath, mustJSON(t, "/tmp/wait")},
		{diagnostic.CodeNotifierWorkingDirectory, mustJSON(t, "/tmp")},
		{diagnostic.CodeRunResolvedExecutable, mustJSON(t, "/bin/true")},
		{diagnostic.CodeTargetEnvironmentNames, mustJSON(t, environment)},
		{diagnostic.CodeWaitEnvironmentNames, mustJSON(t, environment)},
		{diagnostic.CodeNotifierEnvironmentNames, mustJSON(t, environment)},
		{diagnostic.CodeSimilarFailure, mustJSON(t, similar)},
		{diagnostic.CodeLogStdoutBytes, mustJSON(t, 10)},
		{diagnostic.CodeLogStderrBytes, mustJSON(t, 20)},
		{diagnostic.CodeRuntimeRunCount, mustJSON(t, 1)},
		{diagnostic.CodeRuntimeSuccessCount, mustJSON(t, 0)},
		{diagnostic.CodeRuntimeFailureCount, mustJSON(t, 1)},
		{diagnostic.CodeRuntimeNextRunAt, mustJSON(t, "2026-01-01T00:00:00Z")},
		{diagnostic.CodeRuntimeWaitingReason, mustJSON(t, "backoff_delay")},
		{diagnostic.CodeExecutionPolicy, mustJSON(t, map[string]any{})},
		{"future.scalar", mustJSON(t, true)},
		{"future.object", mustJSON(t, map[string]any{"secret": "not rendered"})},
	}
	for _, test := range tests {
		item := diagnostic.Item{ID: "ev:run:00000000000000000001:value", Code: test.code, Value: test.value}
		if got := view.itemDetail(citation, item); got == "" {
			t.Fatalf("itemDetail(%q) was empty", test.code)
		}
	}
}
