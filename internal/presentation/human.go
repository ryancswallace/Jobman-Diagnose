// Package presentation renders diagnosis reports without raw artifact content.
package presentation

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

const humanOutputWidth = 100

// Human writes a readable, evidence-aware report for an interactive user.
// Canonical identifiers and schemas remain available through JSON output; the
// human view uses report-local aliases and never renders raw artifact bytes.
func Human(destination io.Writer, report diagnosis.Report, evidence diagnosis.FailureEvidence) error {
	if destination == nil {
		return fmt.Errorf("write diagnosis: nil destination")
	}
	view, err := newReportView(report, evidence)
	if err != nil {
		return fmt.Errorf("write diagnosis: %w", err)
	}
	renderer := humanRenderer{view: view, width: humanOutputWidth}
	renderer.render()
	if _, err := io.WriteString(destination, renderer.output.String()); err != nil {
		return fmt.Errorf("write diagnosis: %w", err)
	}

	return nil
}

type humanRenderer struct {
	view   reportView
	width  int
	output strings.Builder
}

func (renderer *humanRenderer) render() {
	renderer.renderDiagnosis()
	renderer.renderJob()
	renderer.renderRetry()
	renderer.renderEvidence()
	renderer.renderOtherFindings()
	renderer.renderActions()
	renderer.renderMissingEvidence()
	renderer.renderCaveats()
	renderer.renderTechnicalDetails()
}

func (renderer *humanRenderer) renderDiagnosis() {
	primary := renderer.view.primary
	renderer.section("Diagnosis")
	renderer.paragraph("  ["+renderer.view.findingAlias(primary.ID)+"] ", "       ", primary.Summary)
	renderer.field("Confidence", formatConfidence(primary.Confidence, primary.Analyzer))
	renderer.field("Why", primary.Explanation)
	if primary.Confidence.Basis != "" && primary.Confidence.Basis != primary.Explanation {
		renderer.field("Confidence basis", primary.Confidence.Basis)
	}
	if references := renderer.view.referenceList(primary.SupportingEvidence); references != "" {
		renderer.field("Evidence", references)
	}
	if references := renderer.view.referenceList(primary.ContradictingEvidence); references != "" {
		renderer.field("Contradicting evidence", references)
	}
}

func (renderer *humanRenderer) renderJob() {
	renderer.section("Job")
	if renderer.view.jobName != "" {
		renderer.field("Name", renderer.view.jobName)
	}
	renderer.field("ID", renderer.view.report.Subject.JobID)
	if runs := formatRuns(renderer.view.report.Subject.SelectedRuns); runs != "" {
		renderer.field(runLabel(len(renderer.view.report.Subject.SelectedRuns)), runs)
	}
	if renderer.view.targetCommand != nil {
		renderer.field("Command", formatCommand(*renderer.view.targetCommand))
	}
	if renderer.view.workingDirectory != "" {
		renderer.field("Working directory", formatArgument(renderer.view.workingDirectory))
	}
	renderer.field("State", formatState(renderer.view.report.Subject.Phase, renderer.view.report.Subject.Outcome))
}

func (renderer *humanRenderer) renderRetry() {
	retry := renderer.view.report.Retry
	renderer.section("Retry")
	renderer.field("Recommendation", retryVerdictText(retry.Verdict))
	renderer.field("Automatic policy", existingPolicyText(retry.ExistingPolicy))
	renderer.field("Reason", retry.Rationale)
	if retry.Confidence.Basis != "" {
		renderer.field("Confidence", formatConfidence(retry.Confidence, ""))
		renderer.field("Confidence basis", retry.Confidence.Basis)
	}
	if retry.EarliestAt != nil {
		renderer.field("Eligible", formatRelativeTime(*retry.EarliestAt, renderer.view.report.GeneratedAt))
	}
	for _, reason := range retry.Reasons {
		renderer.bullet("Reason", retryReasonText(reason))
	}
	if references := renderer.view.referenceList(retry.SupportingEvidence); references != "" {
		renderer.field("Evidence", references)
	}
}

func (renderer *humanRenderer) renderEvidence() {
	if len(renderer.view.evidenceOrder) == 0 {
		return
	}
	renderer.section("Evidence")
	for _, id := range renderer.view.evidenceOrder {
		alias := renderer.view.evidenceAlias(id)
		detail := renderer.view.evidenceDetail(id)
		renderer.paragraph("  ["+alias+"] ", "       ", detail)
	}
}

func (renderer *humanRenderer) renderOtherFindings() {
	if len(renderer.view.report.Findings) < 2 {
		return
	}
	renderer.section("Other findings")
	for _, finding := range renderer.view.report.Findings {
		if finding.ID == renderer.view.primary.ID {
			continue
		}
		prefix := "  [" + renderer.view.findingAlias(finding.ID) + "] " + findingSource(finding.Analyzer) + ": "
		renderer.paragraph(prefix, strings.Repeat(" ", visibleWidth(prefix)), finding.Summary)
		renderer.subfield("Confidence", formatConfidence(finding.Confidence, finding.Analyzer))
		renderer.subfield("Why", finding.Explanation)
		if finding.Confidence.Basis != "" && finding.Confidence.Basis != finding.Explanation {
			renderer.subfield("Confidence basis", finding.Confidence.Basis)
		}
		if references := renderer.view.referenceList(finding.SupportingEvidence); references != "" {
			renderer.subfield("Evidence", references)
		}
		if references := renderer.view.referenceList(finding.ContradictingEvidence); references != "" {
			renderer.subfield("Contradicting evidence", references)
		}
		if findings := renderer.view.findingReferenceList(finding.ContradictingFindings); findings != "" {
			renderer.subfield("Conflicts with", findings)
		}
	}
}

func (renderer *humanRenderer) renderActions() {
	if len(renderer.view.report.Actions) == 0 {
		return
	}
	renderer.section("Recommended next steps")
	for index, action := range renderer.view.report.Actions {
		prefix := "  " + strconv.Itoa(index+1) + ". "
		renderer.paragraph(prefix, strings.Repeat(" ", visibleWidth(prefix)), action.Summary)
		renderer.subparagraph(action.Description)
		renderer.subfield("Type", actionKindText(action.Kind))
		if references := renderer.view.referenceList(action.SupportingEvidence); references != "" {
			renderer.subfield("Evidence", references)
		}
		if action.Execution == diagnosis.ActionExecutionReadOnly {
			renderer.subfield("Suggested command", formatArguments(action.Arguments))
			renderer.subfield("Execution", "Read-only suggestion; Jobman Diagnose will not run it")
		}
		if action.RequiresConfirmation {
			renderer.subfield("Confirmation", "Required before taking this action")
		}
	}
}

func (renderer *humanRenderer) renderMissingEvidence() {
	if len(renderer.view.report.MissingEvidence) == 0 {
		return
	}
	renderer.section("Missing context")
	for _, value := range renderer.view.report.MissingEvidence {
		renderer.bullet("", value.Description)
	}
}

func (renderer *humanRenderer) renderCaveats() {
	if len(renderer.view.report.Warnings) == 0 {
		return
	}
	renderer.section("Caveats")
	for _, value := range renderer.view.report.Warnings {
		renderer.bullet("", value.Message)
	}
}

func (renderer *humanRenderer) renderTechnicalDetails() {
	report := renderer.view.report
	renderer.section("Technical details")
	renderer.field("Analysis", analysisModeText(report.Mode))
	renderer.field("Primary finding", renderer.view.primary.Code+" via "+renderer.view.primary.Analyzer)
	if report.Disclosure.ProviderInvoked {
		status := "Deterministic fallback; no generated content was accepted"
		if report.Disclosure.GeneratedContentUsed {
			status = "Validated generated hypotheses included"
		}
		renderer.field("AI augmentation", status)
		renderer.field("Provider", formatProvider(report))
		if len(report.Disclosure.Classes) != 0 {
			renderer.field("Shared classes", strings.Join(report.Disclosure.Classes, ", "))
		}
		renderer.field("Shared evidence", fmt.Sprintf(
			"%d facts, %d artifacts, %d enrichments; %s artifact content",
			report.Disclosure.ItemCount, report.Disclosure.ArtifactCount,
			report.Disclosure.EnrichmentCount, formatBytes(report.Disclosure.ArtifactBytes),
		))
	}
	renderer.field("Report ID", report.ReportID)
	renderer.field("Core evidence ID", report.CoreEvidenceID)
	renderer.field("Analysis evidence ID", report.AnalysisEvidenceID)
	renderer.field("Versions", formatVersions(report.Versions))
	renderer.field("Evidence aliases", "Report-local display labels; use --json for canonical evidence IDs")
	for _, finding := range report.Findings {
		renderer.bullet(renderer.view.findingAlias(finding.ID), finding.Code+" via "+finding.Analyzer)
	}
}

func (renderer *humanRenderer) section(title string) {
	if renderer.output.Len() != 0 {
		renderer.output.WriteByte('\n')
	}
	renderer.output.WriteString(title)
	renderer.output.WriteString("\n\n")
}

func (renderer *humanRenderer) paragraph(prefix, continuation, value string) {
	appendWrapped(&renderer.output, prefix, continuation, value, renderer.width)
}

func (renderer *humanRenderer) field(label, value string) {
	if value == "" {
		return
	}
	prefix := "  " + label + ": "
	renderer.paragraph(prefix, strings.Repeat(" ", visibleWidth(prefix)), value)
}

func (renderer *humanRenderer) subfield(label, value string) {
	if value == "" {
		return
	}
	prefix := "     " + label + ": "
	renderer.paragraph(prefix, strings.Repeat(" ", visibleWidth(prefix)), value)
}

func (renderer *humanRenderer) subparagraph(value string) {
	renderer.paragraph("     ", "     ", value)
}

func (renderer *humanRenderer) bullet(label, value string) {
	prefix := "  - "
	if label != "" {
		prefix += label + ": "
	}
	renderer.paragraph(prefix, "    ", value)
}

func formatConfidence(confidence diagnosis.Confidence, analyzer string) string {
	value := titleWords(confidence.Band) + " (" + strconv.Itoa(confidence.Score) + "/100)"
	if strings.HasPrefix(analyzer, "generator.") {
		value += "; AI estimate, not calibrated"
	}

	return value
}

func formatRuns(runs []uint64) string {
	values := make([]string, len(runs))
	for index, run := range runs {
		values[index] = strconv.FormatUint(run, 10)
	}

	return strings.Join(values, ", ")
}

func runLabel(count int) string {
	if count == 1 {
		return "Run"
	}

	return "Runs"
}

func formatRelativeTime(value, now time.Time) string {
	exact := value.Format(time.RFC3339)
	if now.IsZero() || !value.After(now) {
		return exact
	}

	return "in " + friendlyDuration(value.Sub(now)) + " (" + exact + ")"
}
