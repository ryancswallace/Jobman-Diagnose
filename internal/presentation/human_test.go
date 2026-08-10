package presentation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestHumanRendersReadableEvidenceAwareReport(t *testing.T) {
	t.Parallel()

	report, evidence := presentationFixture(t, nil)
	var output bytes.Buffer
	if err := Human(&output, report, evidence); err != nil {
		t.Fatalf("Human() error = %v", err)
	}
	rendered := output.String()
	for _, wanted := range []string{
		"Diagnosis\n", "[F1] The command was explicitly configured to report failure",
		"Confidence: Very high (100/100)", "Job\n", "Name: presentation-test",
		"ID: " + testevidence.JobID, "Run: 1", `Command: /usr/bin/false --mode "batch size"`,
		"State: Failed (job completed)", "Retry\n", "current policy will not retry this failure",
		"Evidence\n", "[E1] Observed fact", "Run 1 exit code: 2",
		"Peak resident memory: 64 MiB", "Recommended next steps\n",
		"Technical details\n", "Report ID: " + report.ReportID,
		"Core evidence ID: " + report.CoreEvidenceID, "Evidence aliases: Report-local display labels",
	} {
		if !strings.Contains(rendered, wanted) {
			t.Fatalf("Human() output missing %q:\n%s", wanted, rendered)
		}
	}
	for _, unwanted := range []string{
		"ev:run:00000000000000000001", "analysis:000001", "DIRECT ARGUMENTS", "NEXT ACTIONS",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("Human() output contains %q:\n%s", unwanted, rendered)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if len([]rune(line)) > humanOutputWidth && !strings.Contains(line, "sha256:") {
			t.Fatalf("Human() line exceeds %d columns: %q", humanOutputWidth, line)
		}
	}
}

func TestHumanRendersEnrichmentWithoutRawArtifactContent(t *testing.T) {
	t.Parallel()

	report, evidence := presentationFixture(t, []byte(
		"Traceback (most recent call last):\n  <untrusted instructions>\nValueError: bad\n",
	))
	var output bytes.Buffer
	if err := Human(&output, report, evidence); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, wanted := range []string{
		"Other findings", "Deterministic finding: A Python exception traceback is present",
		"Python traceback detected in run 1 stderr", "Exact derivation", "Caveats",
		"Target log artifacts are untrusted data",
	} {
		if !strings.Contains(rendered, wanted) {
			t.Fatalf("Human() output missing %q:\n%s", wanted, rendered)
		}
	}
	if strings.Contains(rendered, "<untrusted instructions>") || strings.Contains(rendered, "ValueError: bad") {
		t.Fatalf("Human() copied raw artifact content:\n%s", rendered)
	}
}

func TestHumanRejectsNilDestinationAndMismatchedEvidence(t *testing.T) {
	t.Parallel()

	report, evidence := presentationFixture(t, nil)
	if err := Human(nil, report, evidence); err == nil {
		t.Fatal("Human(nil) error = nil")
	}
	other, err := testevidence.Failed("timeout", nil)
	if err != nil {
		t.Fatal(err)
	}
	otherEvidence, err := enrichment.Collect(t.Context(), other)
	if err != nil {
		t.Fatal(err)
	}
	if err := Human(&bytes.Buffer{}, report, otherEvidence); err == nil ||
		!strings.Contains(err.Error(), "report subject does not match core evidence") {
		t.Fatalf("Human(mismatched evidence) error = %v", err)
	}
}

func presentationFixture(t *testing.T, stderr []byte) (diagnosis.Report, diagnosis.FailureEvidence) {
	t.Helper()
	evidence, err := testevidence.Failed("nonzero_exit", stderr)
	if err != nil {
		t.Fatal(err)
	}
	values := []diagnostic.Item{
		fixtureItem(t, "ev:job:name", diagnostic.CodeJobName, "presentation-test", diagnostic.DisclosureMetadata),
		fixtureItem(t, "ev:job:target:command", diagnostic.CodeTargetCommand, diagnostic.Command{
			Executable: "/usr/bin/false", Arguments: []string{"--mode", "batch size"},
		}, diagnostic.DisclosureCommand),
		fixtureItem(t, "ev:job:target:working_directory", diagnostic.CodeTargetWorkingDirectory,
			"/tmp/presentation test", diagnostic.DisclosurePath),
		fixtureItem(t, "ev:run:00000000000000000001:resource:peak_resident_memory",
			diagnostic.CodeResourceObservation, diagnostic.ResourceObservation{
				Metric: diagnostic.ResourcePeakRSS, Value: 64 * 1024 * 1024, Unit: diagnostic.ResourceUnitBytes,
				Scope: diagnostic.ResourceScopeProcess, Source: diagnostic.ResourceSourceWaitRusage,
				Completeness: diagnostic.ResourceCompleteAtExit,
			}, diagnostic.DisclosureMetadata),
		fixtureItem(t, "ev:job:policy", diagnostic.CodeExecutionPolicy, diagnostic.ExecutionPolicy{
			StdinPolicy: "null", StopGracePeriod: "0s", ForceAfterGrace: true,
			RunTimeout: "0s", JobTimeout: "0s",
			Completion: diagnostic.CompletionPolicy{
				MaximumRuns:   diagnostic.CountLimit{Unlimited: true},
				SuccessTarget: diagnostic.CountLimit{Unlimited: true},
				FailureLimit:  diagnostic.CountLimit{Unlimited: true},
			},
			FailureDelay: diagnostic.DelayPolicy{Base: "0s", Backoff: "constant", Jitter: "0s"},
			SuccessDelay: diagnostic.DelayPolicy{Base: "0s", Backoff: "constant", Jitter: "0s"},
			WaitMode:     "all", ConcurrencySlots: 1, LogCapture: "both", LogRetention: "unlimited",
		}, diagnostic.DisclosureMetadata),
		fixtureItem(t, "ev:runtime:run_count", diagnostic.CodeRuntimeRunCount, uint64(1), diagnostic.DisclosureMetadata),
		fixtureItem(t, "ev:runtime:failure_count", diagnostic.CodeRuntimeFailureCount, uint64(1), diagnostic.DisclosureMetadata),
	}
	evidence.Items = append(evidence.Items, values...)
	evidence, err = diagnostic.Seal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	failureEvidence, err := enrichment.Collect(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	diagnostician, err := engine.New("presentation-test", func() time.Time {
		return time.Date(2026, 8, 8, 13, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), failureEvidence)
	if err != nil {
		t.Fatal(err)
	}

	return report, failureEvidence
}

func fixtureItem(
	t *testing.T,
	id string,
	code string,
	value any,
	disclosure diagnostic.DisclosureClass,
) diagnostic.Item {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return diagnostic.Item{
		ID: id, Code: code, Value: encoded,
		Source:  diagnostic.ItemSource{Kind: "job_snapshot", EntityID: testevidence.JobID, Revision: 8},
		Quality: diagnostic.QualityObserved, Disclosure: disclosure,
	}
}

func TestFormatSystemContextDistinguishesCumulativeCgroupEvents(t *testing.T) {
	t.Parallel()

	memory := uint64(1024 * 1024 * 1024)
	pids := uint64(12)
	oom := uint64(3)
	kills := uint64(1)
	formatted := formatSystemContext(systemContextView{
		Filesystem: &filesystemCapacityView{AvailableBytes: 20 * 1024 * 1024 * 1024, TotalBytes: 100 * 1024 * 1024 * 1024},
		LinuxCgroup: &linuxCgroupView{
			MemoryCurrentBytes: &memory, MemoryMaximum: &systemLimitView{Value: 4 * 1024 * 1024 * 1024},
			PIDsCurrent: &pids, PIDsMaximum: &systemLimitView{Unlimited: true},
			CumulativeOOM: &oom, CumulativeOOMKills: &kills,
		},
		ContainerHint: "docker",
	})
	for _, expected := range []string{
		"20 GiB available of 100 GiB", "cgroup memory 1 GiB / 4 GiB", "cgroup PIDs 12 / unlimited",
		"cumulative cgroup OOM events 3 (kills 1)", "container hint docker",
	} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf("formatSystemContext() = %q, want %q", formatted, expected)
		}
	}
}
