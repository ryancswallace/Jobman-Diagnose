package presentation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
	"github.com/ryancswallace/jobman-diagnose/provider"
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
		"Diagnosis\n", "  • Primary finding [F1]\n    The command was explicitly configured to report failure",
		"      - Confidence: Very high (100/100)", "Job\n", "  • presentation-test · Run 1",
		"    - ID: " + testevidence.JobID, `    - Command: /usr/bin/false --mode "batch size"`,
		"Failed (job completed)", "Retry\n", "  • Automatic retries: The current policy will not retry this failure",
		"Evidence highlights\n", "  • [E1] Observed fact", "Run 1 exit code: 2",
		"additional facts are available with --details", "Recommended next steps\n",
	} {
		if !strings.Contains(rendered, wanted) {
			t.Fatalf("Human() output missing %q:\n%s", wanted, rendered)
		}
	}
	for _, unwanted := range []string{
		"ev:run:00000000000000000001", "analysis:000001", "DIRECT ARGUMENTS", "NEXT ACTIONS",
		"Technical details", "Report ID: " + report.ReportID, "Peak resident memory: 64 MiB", "\x1b[",
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

func TestHumanDetailsRetainCompleteEvidenceAndProvenance(t *testing.T) {
	t.Parallel()

	report, evidence := presentationFixture(t, nil)
	var output bytes.Buffer
	if err := HumanWithOptions(&output, report, evidence, HumanOptions{Details: true}); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, wanted := range []string{
		"Evidence\n", "Peak resident memory: 64 MiB", "Technical details\n",
		"Report ID: " + report.ReportID, "Core evidence ID: " + report.CoreEvidenceID,
		"Confidence basis:", "Type: Configuration or environment change",
	} {
		if !strings.Contains(rendered, wanted) {
			t.Fatalf("detailed human output missing %q:\n%s", wanted, rendered)
		}
	}
	if strings.Contains(rendered, "Evidence highlights") || strings.Contains(rendered, "available with --details") {
		t.Fatalf("detailed human output retained concise-view hints:\n%s", rendered)
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
		"Additional observation [F3]\n    A Python exception traceback is present",
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

func TestHumanMakesGeneratedDiagnosisProminent(t *testing.T) {
	t.Parallel()

	report, evidence := mixedPresentationFixture(t)
	var output bytes.Buffer
	if err := Human(&output, report, evidence); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, wanted := range []string{
		"  • AI-assisted likely cause [F3]\n    Deployment region moon-1 is not enabled for the target application",
		"      - Status: Advisory; confidence not calibrated",
		"      - Root cause: region moon-1 is not enabled for this deployment",
		"      - Failure path: startup validation rejects the region before work begins",
		"  • Confirmed by Jobman [F1]",
		"Correct the rejected application configuration",
		"AI disclosure\n", "Validated generated hypotheses from jobman-llama contributed",
	} {
		if !strings.Contains(rendered, wanted) {
			t.Fatalf("mixed human output missing %q:\n%s", wanted, rendered)
		}
	}
	if strings.Index(rendered, "AI-assisted likely cause") > strings.Index(rendered, "Confirmed by Jobman") {
		t.Fatalf("generated diagnosis is not answer-first:\n%s", rendered)
	}
	if strings.Contains(rendered, "Medium (40/100)") || strings.Contains(rendered, "generated_content_uncalibrated") {
		t.Fatalf("mixed human output exposes internal AI ranking details:\n%s", rendered)
	}
}

func TestHumanColorStylesSemanticsWithoutChangingWrapping(t *testing.T) {
	t.Parallel()

	report, evidence := mixedPresentationFixture(t)
	var output bytes.Buffer
	if err := HumanWithOptions(&output, report, evidence, HumanOptions{Color: true}); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, wanted := range []string{
		"\x1b[1mDiagnosis\x1b[0m",
		"\x1b[1m\x1b[35mAI-assisted likely cause\x1b[0m",
		"\x1b[1m\x1b[36mConfirmed by Jobman\x1b[0m",
		"\x1b[33mAdvisory; confidence not calibrated\x1b[0m",
		"\x1b[31mFailed (job completed)\x1b[0m",
		"\x1b[36m/usr/bin/false --mode \"batch size\"\x1b[0m",
	} {
		if !strings.Contains(rendered, wanted) {
			t.Fatalf("colored human output missing %q:\n%q", wanted, rendered)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if visibleWidth(line) > humanOutputWidth && !strings.Contains(line, "sha256:") {
			t.Fatalf("colored line exceeds %d visible columns: %q", humanOutputWidth, line)
		}
	}
	styled := "\x1b[1m\x1b[35m" + "colored" + "\x1b[0m"
	if visibleWidth(styled) != len("colored") {
		t.Fatal("visibleWidth counted ANSI styling bytes")
	}
}

//nolint:gocognit // The presentation matrix checks width, color, ordering, and wrapping together.
func TestHumanLayoutsRemainAnswerFirstAcrossTerminalWidths(t *testing.T) {
	t.Parallel()

	report, evidence := mixedPresentationFixture(t)
	for _, width := range []int{60, 80, 120} {
		for _, color := range []bool{false, true} {
			name := fmt.Sprintf("width_%d_color_%t", width, color)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				var output bytes.Buffer
				if err := HumanWithOptions(&output, report, evidence, HumanOptions{
					Color: color, Width: width,
				}); err != nil {
					t.Fatal(err)
				}
				rendered := output.String()
				if !strings.Contains(rendered, "AI-assisted likely cause") ||
					strings.Index(rendered, "AI-assisted likely cause") > strings.Index(rendered, "Confirmed by Jobman") {
					t.Fatalf("AI diagnosis is not answer-first at width %d:\n%s", width, rendered)
				}
				for _, line := range strings.Split(rendered, "\n") {
					if visibleWidth(line) > width && !strings.Contains(line, "sha256:") {
						t.Fatalf("line exceeds %d visible columns: %q", width, line)
					}
				}
				if !color && strings.Contains(rendered, "\x1b[") {
					t.Fatalf("plain layout contains ANSI styling: %q", rendered)
				}
			})
		}
	}
}

func TestHumanRejectsUnsafeLayoutWidths(t *testing.T) {
	t.Parallel()

	report, evidence := presentationFixture(t, nil)
	for _, width := range []int{minimumHumanWidth - 1, maximumHumanWidth + 1} {
		if err := HumanWithOptions(&bytes.Buffer{}, report, evidence, HumanOptions{Width: width}); err == nil ||
			!strings.Contains(err.Error(), "width must be between") {
			t.Fatalf("HumanWithOptions(width %d) error = %v", width, err)
		}
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

func mixedPresentationFixture(t *testing.T) (diagnosis.Report, diagnosis.FailureEvidence) {
	t.Helper()
	report, evidence := presentationFixture(t, []byte("configuration rejected by target\n"))
	if len(evidence.Core.Artifacts) == 0 {
		t.Fatal("mixed presentation fixture has no artifact")
	}
	artifact := evidence.Core.Artifacts[0]
	exitID := ""
	for _, item := range evidence.Core.Items {
		if item.Code == diagnostic.CodeRunExitCode {
			exitID = item.ID
			break
		}
	}
	if exitID == "" {
		t.Fatal("mixed presentation fixture has no exit code")
	}
	artifactCited := false
	for _, citation := range report.Citations {
		artifactCited = artifactCited || citation.EvidenceID == artifact.ID
	}
	if !artifactCited {
		report.Citations = append(report.Citations, diagnosis.Citation{
			EvidenceID: artifact.ID, Code: artifact.Role,
			Summary: "A bounded, sanitized, untrusted target log excerpt.", Kind: "artifact",
		})
	}
	confidence, err := diagnosis.NewConfidence(40, "Generated hypotheses are advisory and uncalibrated.")
	if err != nil {
		t.Fatal(err)
	}
	support := []string{artifact.ID, exitID}
	report.Findings = append(report.Findings, diagnosis.Finding{
		ID: "finding:999:generated-application-configuration", Code: "generated.application_configuration",
		Category: "application", Severity: diagnosis.SeverityWarning,
		Summary: "Deployment region moon-1 is not enabled for the target application",
		Explanation: "Root cause: region moon-1 is not enabled for this deployment. " +
			"Failure path: startup validation rejects the region before work begins.",
		Confidence: confidence, SupportingEvidence: support,
		ContradictingEvidence: []string{}, ContradictingFindings: []string{}, Analyzer: "generator.proposal/1",
	})
	report.Actions = append([]diagnosis.Action{{
		ID: "action:000:generated-guidance", Code: "review_application_configuration", Kind: diagnosis.ActionChange,
		Summary:            "Correct the rejected application configuration",
		Description:        "Compare the affected setting with values enabled for the target deployment before creating a new run.",
		SupportingEvidence: support, Execution: diagnosis.ActionExecutionNone, Arguments: []string{},
		RequiresConfirmation: true, SafeToAutomate: false,
	}}, report.Actions...)
	report.Mode = diagnosis.ModeMixed
	report.Versions.GenerationRequestSchemaVersion = provider.RequestSchemaVersion
	report.Versions.ProposalSchemaVersion = provider.ProposalSchemaVersion
	report.Disclosure = diagnosis.DisclosureManifest{
		ProviderInvoked: true, GeneratedContentUsed: true, Locality: diagnosis.ProviderLocal,
		Profile: "local-vllm", Provider: "openai_compatible", Model: "jobman-llama",
		RequestID: "sha256:" + strings.Repeat("a", 64), Classes: []string{"metadata", "log_content"},
		ItemIDs: []string{exitID}, ArtifactIDs: []string{artifact.ID}, EnrichmentIDs: []string{},
		ItemCount: 1, ArtifactCount: 1, ArtifactBytes: artifact.ContentBytes, RequestBytes: 1024,
	}
	report.Generators = []diagnosis.GeneratorDescriptor{{
		Provider: "openai_compatible", Model: "jobman-llama", Profile: "local-vllm", Locality: diagnosis.ProviderLocal,
	}}
	report.Warnings = append(report.Warnings, diagnosis.Warning{
		Code:    uncalibratedWarningCode,
		Message: "Generated hypotheses are advisory and uncalibrated; deterministic facts remain authoritative.",
	})
	sealed, err := diagnosis.Seal(report)
	if err != nil {
		t.Fatal(err)
	}

	return sealed, evidence
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

func TestTechnicalDetailsExplainGeneratedAndFallbackDisclosure(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		generated  bool
		wantStatus string
	}{
		{name: "generated", generated: true, wantStatus: "Validated generated hypotheses included"},
		{name: "fallback", wantStatus: "Deterministic fallback; no generated content was accepted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report := diagnosis.Report{
				Mode: diagnosis.ModeMixed, ReportID: "report", CoreEvidenceID: "core", AnalysisEvidenceID: "analysis",
				Findings: []diagnosis.Finding{{ID: "finding", Code: "core.failure", Analyzer: "builtin.rules/1"}},
				Disclosure: diagnosis.DisclosureManifest{
					ProviderInvoked: true, GeneratedContentUsed: test.generated, Locality: diagnosis.ProviderLocal,
					Profile: "local", Provider: "ollama", Model: "model", Classes: []string{"metadata", "log_content"},
					ItemCount: 2, ArtifactCount: 1, EnrichmentCount: 1, ArtifactBytes: 1024,
				},
			}
			renderer := humanRenderer{view: reportView{
				report: report, primary: report.Findings[0], findingAliases: map[string]string{"finding": "F1"},
			}, width: humanOutputWidth}
			renderer.renderTechnicalDetails()
			rendered := renderer.output.String()
			for _, wanted := range []string{
				test.wantStatus, "ollama/model (local; profile local)", "metadata, log_content",
				"2 facts, 1 artifacts, 1 enrichments; 1 KiB artifact content", "F1: core.failure via builtin.rules/1",
			} {
				if !strings.Contains(rendered, wanted) {
					t.Fatalf("technical details missing %q:\n%s", wanted, rendered)
				}
			}
		})
	}
}
