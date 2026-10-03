package deterministic_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/deterministic"
	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

func sharedCore(t *testing.T) diagnostic.Evidence {
	t.Helper()
	encoded, err := os.ReadFile("../testdata/jobman-v2/shared-control-failure-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	core, err := diagnostic.Decode(bytes.NewReader(encoded), diagnostic.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func analyze(t *testing.T, core diagnostic.Evidence) (diagnosis.Report, diagnosis.FailureEvidence) {
	t.Helper()
	engine, err := deterministic.New("test", func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := deterministic.Prepare(t.Context(), core)
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Diagnose(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err != nil {
		t.Fatal(err)
	}
	return report, evidence
}

func seal(t *testing.T, core diagnostic.Evidence) diagnostic.Evidence {
	t.Helper()
	sealed, err := diagnostic.Seal(core)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func fact(t *testing.T, code string, value any, entity string) diagnostic.Item {
	t.Helper()
	encoded, err := diagnostic.JSONValue(value)
	if err != nil {
		t.Fatal(err)
	}
	return diagnostic.Item{
		ID: "fact:" + code, Code: code, Value: encoded,
		Source:  diagnostic.ItemSource{Kind: "control_snapshot", EntityID: entity},
		Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureMetadata,
	}
}

func primary(report diagnosis.Report) diagnosis.Finding {
	for _, finding := range report.Findings {
		if finding.ID == report.PrimaryFindingID {
			return finding
		}
	}
	return diagnosis.Finding{}
}

func TestPublicSharedDiagnosisBindsAuthorityAndUsesNoExecutableActions(t *testing.T) {
	core := sharedCore(t)
	report, evidence := analyze(t, core)
	if report.SchemaVersion != diagnosis.SharedSchemaVersion || report.Versions.EngineVersion != diagnosis.SharedEngineVersion ||
		report.Shared.Source != core.Shared.Source || report.Shared.Runs[0] != core.Shared.Runs[0] ||
		primary(report).Code != "core.nonzero_exit" || report.Disclosure.ProviderInvoked {
		t.Fatalf("unexpected shared report: %#v", report)
	}
	for _, action := range report.Actions {
		if action.Execution != diagnosis.ActionExecutionNone || len(action.Arguments) != 0 {
			t.Fatal("shared report has unqualified local argv")
		}
	}
	roundTrip(t, report, evidence)
	var err error
	oldID := report.ReportID
	report.Shared.Source.NamespaceID = "01990000-0000-7000-8000-000000000099"
	if verifyErr := diagnosis.Verify(report); verifyErr == nil {
		t.Fatal("accepted tampered source provenance")
	}
	report, err = diagnosis.Seal(report)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReportID == oldID {
		t.Fatal("report identity ignores shared authority")
	}
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err == nil {
		t.Fatal("accepted correctly resealed report for another namespace")
	}
	if core.Shared.Source.NamespaceID != evidence.Core.Shared.Source.NamespaceID {
		t.Fatal("report mutation altered core evidence")
	}
}

func TestSharedRulesDistinguishOutcomesAndUncertainty(t *testing.T) {
	for name, test := range map[string]struct {
		code  string
		value any
		want  string
	}{
		"timeout":        {diagnostic.CodeRunOutcome, "timed_out", "shared.timed_out"},
		"cancellation":   {diagnostic.CodeRunOutcome, "cancelled", "shared.cancelled"},
		"lost":           {diagnostic.CodeRunOutcome, "lost", "shared.lost"},
		"aborted":        {diagnostic.CodeRunOutcome, "aborted", "shared.aborted"},
		"failure":        {diagnostic.CodeRunOutcome, "failure", "shared.failure"},
		"success":        {diagnostic.CodeRunOutcome, "success", "core.no_target_failure"},
		"future outcome": {diagnostic.CodeRunOutcome, "future_outcome", "core.insufficient_structured_evidence"},
		"exit 137":       {diagnostic.CodeRunExitCode, 137, "core.nonzero_exit"},
		"signal":         {diagnostic.CodeRunExitSignal, "SIGTERM", "core.signal_termination"},
	} {
		t.Run(name, func(t *testing.T) {
			core := sharedCore(t)
			core.Items = []diagnostic.Item{fact(t, test.code, test.value, core.Shared.Runs[0].ID)}
			report, _ := analyze(t, seal(t, core))
			if got := primary(report).Code; got != test.want {
				t.Fatalf("finding=%q, want %q", got, test.want)
			}
			if report.Retry.ExistingPolicy != diagnosis.PolicyUnknown {
				t.Fatal("invented shared retry policy")
			}
		})
	}
	core := sharedCore(t)
	core.Items = []diagnostic.Item{fact(t, diagnostic.CodeSharedObservationConfidence, "stale", core.Subject.JobID)}
	report, _ := analyze(t, seal(t, core))
	if primary(report).Code != "shared.observation_uncertain" {
		t.Fatal("stale observation became a job failure")
	}
}

func TestSharedSchedulerAndDependencyCitationsPreserveSourceDecisions(t *testing.T) {
	core := sharedCore(t)
	core.Items = []diagnostic.Item{
		fact(t, diagnostic.CodeSharedDependencyObservation, diagnostic.SharedDependencyObservation{
			JobID: "01990000-0000-7000-8000-000000000099", Predicate: "success", ObservedOutcome: "failure", Disposition: "blocked",
		}, core.Subject.JobID),
		fact(t, diagnostic.CodeSharedSchedulerObservation, diagnostic.SharedSchedulerObservation{
			State: "PENDING", Reason: "DependencyNeverSatisfied", ObservedAt: core.CapturedAt,
		}, core.Shared.Runs[0].ID),
	}
	report, evidence := analyze(t, seal(t, core))
	if primary(report).Code != "shared.dependency_unsatisfied" || len(report.Citations) != 2 {
		t.Fatalf("findings: %#v", report.Findings)
	}
	report.Citations[0].EvidenceID = "fact:foreign"
	report, err := diagnosis.Seal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err == nil {
		t.Fatal("accepted foreign citation")
	}
}

func TestSharedLatestRunNeverBorrowsAnotherRunsExit(t *testing.T) {
	core := sharedCore(t)
	core.Shared.Runs = append(core.Shared.Runs, diagnostic.SharedRun{
		ID: "01990000-0000-7000-8000-000000000099", Number: 19,
	})
	core.Subject.SelectedRuns = append(core.Subject.SelectedRuns, 19)
	core.Subject.Outcome = "success"
	core.Items = []diagnostic.Item{
		fact(t, diagnostic.CodeJobOutcome, "success", core.Subject.JobID),
		fact(t, diagnostic.CodeRunExitCode, 137, core.Shared.Runs[0].ID),
	}
	report, _ := analyze(t, seal(t, core))
	if primary(report).Code != "core.insufficient_structured_evidence" {
		t.Fatal("borrowed prior run failure or current job success for unobserved selected run")
	}
}

func TestSharedLogEnrichmentStaysBoundToExactSealedBytes(t *testing.T) {
	core := sharedCore(t)
	core.Shared.Profile = diagnostic.SharedProfileIncludeLogTail
	data := []byte("permission denied\n")
	core.Artifacts = []diagnostic.Artifact{{
		ID: core.Shared.Logs[0].ID, Role: diagnostic.ArtifactRoleLogTail, Run: 3, Stream: "stderr",
		MediaType: "application/octet-stream", Data: data, OriginalBytes: uint64(len(data)),
		ByteEnd: uint64(len(data)), CapturedAt: core.CapturedAt, Quality: diagnostic.QualityConfirmed, Disclosure: diagnostic.DisclosureLogContent,
	}}
	core.Shared.Logs[0].Bytes = uint64(len(data))
	core.Consistency.Artifacts = diagnostic.ArtifactsStable
	report, evidence := analyze(t, seal(t, core))
	if len(evidence.Enrichment) == 0 || !slices.ContainsFunc(report.Citations, func(c diagnosis.Citation) bool { return c.Kind == "enrichment" }) {
		t.Fatal("bounded log observation has no exact enrichment citation")
	}
	evidence.Core.Artifacts[0].Data[0] = 'X'
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err == nil {
		t.Fatal("accepted substituted log citation bytes")
	}
}

func TestSharedLatestRunNeverBorrowsHistoricalLogs(t *testing.T) {
	core := sharedCore(t)
	core.Shared.Profile = diagnostic.SharedProfileIncludeLogTail
	data := []byte("permission denied\n")
	core.Artifacts = []diagnostic.Artifact{{
		ID: core.Shared.Logs[0].ID, Role: diagnostic.ArtifactRoleLogTail, Run: 3, Stream: "stderr",
		MediaType: "application/octet-stream", Data: data, OriginalBytes: uint64(len(data)),
		ByteEnd: uint64(len(data)), CapturedAt: core.CapturedAt, Quality: diagnostic.QualityConfirmed, Disclosure: diagnostic.DisclosureLogContent,
	}}
	core.Shared.Logs[0].Bytes = uint64(len(data))
	core.Consistency.Artifacts = diagnostic.ArtifactsStable
	core.Shared.Runs = append(core.Shared.Runs, diagnostic.SharedRun{
		ID: "01990000-0000-7000-8000-000000000099", Number: 19,
	})
	core.Subject.SelectedRuns = append(core.Subject.SelectedRuns, 19)
	core.Items = []diagnostic.Item{fact(t, diagnostic.CodeLogRecordingHealth, "degraded", core.Shared.Runs[0].ID)}
	core = seal(t, core)
	report, evidence := analyze(t, core)
	if len(evidence.Enrichment) == 0 || primary(report).Code != "core.insufficient_structured_evidence" {
		t.Fatal("historical enrichment or recording health became the selected run's diagnosis")
	}
	wrapped, err := diagnosis.CoreFailureEvidence(core)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := deterministic.New("test", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	report, err = engine.Diagnose(t.Context(), wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if primary(report).Code != "core.insufficient_structured_evidence" {
		t.Fatal("historical artifact signature became the selected run's diagnosis")
	}
}

func TestPublicEngineRejectsBadConstructionAndCanceledWork(t *testing.T) {
	if _, err := deterministic.New("", time.Now); err == nil {
		t.Fatal("accepted missing version")
	}
	if _, err := deterministic.New("test", nil); err == nil {
		t.Fatal("accepted missing clock")
	}
	engine, err := deterministic.New("test", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	core := sharedCore(t)
	evidence, err := diagnostic.JSONValue(core)
	if err != nil {
		t.Fatal(err)
	}
	var copied diagnostic.Evidence
	if decodeErr := json.Unmarshal(evidence, &copied); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	wrapped, err := deterministic.Prepare(t.Context(), copied)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := engine.Diagnose(ctx, wrapped); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	wrapped.Core.Subject.JobRevision++
	if _, err := engine.Diagnose(t.Context(), wrapped); err == nil {
		t.Fatal("accepted unsealed core mutation")
	}
}

func roundTrip(t *testing.T, report diagnosis.Report, evidence diagnosis.FailureEvidence) {
	t.Helper()
	var encoded bytes.Buffer
	if err := diagnosis.Encode(&encoded, report); err != nil {
		t.Fatal(err)
	}
	decoded, err := diagnosis.Decode(&encoded, diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if validationErr := diagnosis.ValidateAgainstEvidence(decoded, evidence); validationErr != nil {
		t.Fatal(validationErr)
	}
}

func TestSharedFixtureHasRecordedImmutableOrigin(t *testing.T) {
	encoded, err := os.ReadFile("../testdata/jobman-v2/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceCommit   string `json:"source_commit"`
		EvidenceSchema int    `json:"evidence_schema"`
		Fixtures       []struct {
			File       string `json:"file"`
			EvidenceID string `json:"evidence_id"`
			SHA256     string `json:"sha256"`
		} `json:"fixtures"`
	}
	if decodeErr := json.Unmarshal(encoded, &manifest); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if len(manifest.SourceCommit) != 40 || manifest.EvidenceSchema != 2 || len(manifest.Fixtures) != 1 {
		t.Fatal("fixture origin incomplete")
	}
	fixture := manifest.Fixtures[0]
	if fixture.File != "shared-control-failure-v2.json" {
		t.Fatal("unexpected fixture identity")
	}
	data, err := os.ReadFile("../testdata/jobman-v2/" + fixture.File)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != fixture.SHA256 || sharedCore(t).EvidenceID != fixture.EvidenceID {
		t.Fatal("published fixture bytes or semantic ID changed")
	}
}

func TestReportRejectsCitationTypeAndSourceCollectionCollisions(t *testing.T) {
	core := sharedCore(t)
	report, evidence := analyze(t, core)
	report.Citations[0].Kind = "artifact"
	changed, err := diagnosis.Seal(report)
	if err != nil {
		t.Fatal(err)
	}
	if validationErr := diagnosis.ValidateAgainstEvidence(changed, evidence); validationErr == nil {
		t.Fatal("accepted a fact relabeled as an artifact")
	}
	evidence.Enrichment = []diagnosis.EnrichmentItem{{ID: core.Items[0].ID}}
	if _, sealErr := diagnosis.SealFailureEvidence(core, evidence.Enrichment); sealErr == nil {
		t.Fatal("accepted core/enrichment citation ID collision")
	}
}

func TestSharedReportRejectsInvalidProvenanceAndExecutableAdvice(t *testing.T) {
	base, _ := analyze(t, sharedCore(t))
	for name, mutate := range map[string]func(*diagnosis.Report){
		"missing shared":   func(r *diagnosis.Report) { r.Shared = nil },
		"local schema":     func(r *diagnosis.Report) { r.SchemaVersion = 1; r.Versions.ReportSchemaVersion = 1 },
		"schema mismatch":  func(r *diagnosis.Report) { r.Versions.ReportSchemaVersion = 1 },
		"wrong authority":  func(r *diagnosis.Report) { r.Shared.Source.ControlInstanceID = "wrong" },
		"wrong run":        func(r *diagnosis.Report) { r.Shared.Runs[0].Number++ },
		"wrong execution":  func(r *diagnosis.Report) { r.Shared.Logs[0].ExecutionID = "01990000-0000-7000-8000-000000000099" },
		"automated advice": func(r *diagnosis.Report) { r.Actions[0].SafeToAutomate = true },
		"local argv": func(r *diagnosis.Report) {
			r.Actions[0].Execution = diagnosis.ActionExecutionReadOnly
			r.Actions[0].Arguments = []string{"jobman", "show", "job", r.Subject.JobID}
		},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			var changed diagnosis.Report
			if decodeErr := json.Unmarshal(encoded, &changed); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			mutate(&changed)
			if _, sealErr := diagnosis.Seal(changed); sealErr == nil {
				t.Fatal("accepted invalid shared report")
			}
		})
	}
}
