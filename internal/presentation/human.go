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

const (
	defaultFindingLimit     = 2
	defaultActionLimit      = 3
	defaultEvidenceLimit    = 4
	uncalibratedWarningCode = "generated_content_uncalibrated"
)

// HumanOptions controls presentation-only detail without changing the sealed
// report or its canonical JSON representation.
type HumanOptions struct {
	Details bool
	Color   bool
}

// Human writes a readable, evidence-aware report for an interactive user.
// Canonical identifiers and schemas remain available through JSON output; the
// human view uses report-local aliases and never renders raw artifact bytes.
func Human(destination io.Writer, report diagnosis.Report, evidence diagnosis.FailureEvidence) error {
	return HumanWithOptions(destination, report, evidence, HumanOptions{})
}

// HumanWithOptions writes an answer-first human report. Details retains the
// complete evidence inventory and machine provenance for interactive audits.
func HumanWithOptions(
	destination io.Writer,
	report diagnosis.Report,
	evidence diagnosis.FailureEvidence,
	options HumanOptions,
) error {
	if destination == nil {
		return fmt.Errorf("write diagnosis: nil destination")
	}
	view, err := newReportView(report, evidence)
	if err != nil {
		return fmt.Errorf("write diagnosis: %w", err)
	}
	renderer := humanRenderer{
		view: view, width: humanOutputWidth, details: options.Details,
		style: newHumanStyle(options.Color),
	}
	renderer.render()
	if _, err := io.WriteString(destination, renderer.output.String()); err != nil {
		return fmt.Errorf("write diagnosis: %w", err)
	}

	return nil
}

type humanRenderer struct {
	view    reportView
	width   int
	details bool
	style   humanStyle
	output  strings.Builder
}

func (renderer *humanRenderer) render() {
	renderer.renderDiagnosis()
	renderer.renderActions()
	renderer.renderRetry()
	renderer.renderJob()
	if renderer.details {
		renderer.renderEvidence()
	} else {
		renderer.renderEvidenceHighlights()
	}
	renderer.renderMissingEvidence()
	renderer.renderCaveats()
	renderer.renderAIDisclosure()
	if renderer.details {
		renderer.renderTechnicalDetails()
	}
}

func (renderer *humanRenderer) renderDiagnosis() {
	primary := renderer.view.primary
	renderer.section("Diagnosis")
	generated := renderer.generatedFindings()
	limit := len(generated)
	if !renderer.details && limit > defaultFindingLimit {
		limit = defaultFindingLimit
	}
	for index, finding := range generated[:limit] {
		label := "AI-assisted likely cause"
		if index != 0 {
			label = "AI-assisted alternative"
		}
		renderer.renderFinding(finding, label)
	}
	if remaining := len(generated) - limit; remaining > 0 {
		renderer.subparagraph(fmt.Sprintf("%d additional AI hypotheses are available with --details.", remaining))
	}

	label := "Confirmed by Jobman"
	if len(generated) == 0 {
		label = "Primary finding"
	}
	renderer.renderFinding(primary, label)

	additional := renderer.additionalDeterministicFindings()
	limit = len(additional)
	if !renderer.details && limit > defaultFindingLimit {
		limit = defaultFindingLimit
	}
	for _, finding := range additional[:limit] {
		renderer.renderFinding(finding, "Additional observation")
	}
	if remaining := len(additional) - limit; remaining > 0 {
		renderer.subparagraph(fmt.Sprintf("%d additional observations are available with --details.", remaining))
	}
}

func (renderer *humanRenderer) renderFinding(finding diagnosis.Finding, label string) {
	alias := renderer.style.muted("[" + renderer.view.findingAlias(finding.ID) + "]")
	styledLabel := renderer.style.confirmed(label)
	if isGeneratedFinding(finding) {
		styledLabel = renderer.style.ai(label)
	}
	renderer.paragraph("  • "+styledLabel+" "+alias, "    ", "")
	renderer.paragraph("    ", "    ", finding.Summary)
	confidenceLabel := "Confidence"
	confidenceValue := formatConfidence(finding.Confidence, finding.Analyzer)
	if isGeneratedFinding(finding) {
		confidenceLabel = "Status"
		confidenceValue = renderer.style.warning(confidenceValue)
	}
	renderer.findingDetail(confidenceLabel, confidenceValue)
	rootCause, failurePath, structuredCause := generatedCauseDetails(finding)
	if structuredCause {
		renderer.findingDetail("Root cause", rootCause)
		renderer.findingDetail("Failure path", failurePath)
	} else if !equivalentText(finding.Explanation, finding.Summary) {
		renderer.findingDetail("Why", finding.Explanation)
	}
	if renderer.details && finding.Confidence.Basis != "" &&
		!equivalentText(finding.Confidence.Basis, finding.Explanation) {
		renderer.findingDetail("Confidence basis", finding.Confidence.Basis)
	}
	if references := renderer.referenceList(finding.SupportingEvidence); references != "" {
		renderer.findingDetail("Evidence", references)
	}
	if references := renderer.referenceList(finding.ContradictingEvidence); references != "" {
		renderer.findingDetail("Contradicting evidence", references)
	}
	if findings := renderer.view.findingReferenceList(finding.ContradictingFindings); findings != "" {
		renderer.findingDetail("Conflicts with", findings)
	}
}

func (renderer *humanRenderer) renderJob() {
	renderer.section("Job")
	headline := make([]string, 0, 3)
	if renderer.view.jobName != "" {
		headline = append(headline, renderer.view.jobName)
	} else {
		headline = append(headline, "Selected job")
	}
	if runs := formatRuns(renderer.view.report.Subject.SelectedRuns); runs != "" {
		headline = append(headline, runLabel(len(renderer.view.report.Subject.SelectedRuns))+" "+runs)
	}
	state := formatState(renderer.view.report.Subject.Phase, renderer.view.report.Subject.Outcome)
	if renderer.view.report.Subject.Outcome == "failure" || renderer.view.report.Subject.Outcome == "start_failed" ||
		renderer.view.report.Subject.Outcome == "timed_out" || renderer.view.report.Subject.Outcome == "lost" {
		state = renderer.style.failure(state)
	}
	headline = append(headline, state)
	renderer.paragraph("  • ", "    ", strings.Join(headline, " · "))
	renderer.subfield("ID", renderer.view.report.Subject.JobID)
	if renderer.view.targetCommand != nil {
		renderer.subfieldStyled("Command", renderer.style.command(formatCommand(*renderer.view.targetCommand)))
	}
	if renderer.view.workingDirectory != "" {
		renderer.subfield("Working directory", formatArgument(renderer.view.workingDirectory))
	}
}

func (renderer *humanRenderer) renderRetry() {
	retry := renderer.view.report.Retry
	renderer.section("Retry")
	renderer.statement("Recommendation", renderer.style.warning(retryVerdictText(retry.Verdict)))
	renderer.statement("Automatic retries", existingPolicyText(retry.ExistingPolicy))
	renderer.statement("Why", retry.Rationale)
	if renderer.details && retry.Confidence.Basis != "" {
		renderer.statement("Confidence", formatConfidence(retry.Confidence, ""))
		renderer.statement("Confidence basis", retry.Confidence.Basis)
	}
	if retry.EarliestAt != nil {
		renderer.statement("Eligible", formatRelativeTime(*retry.EarliestAt, renderer.view.report.GeneratedAt))
	}
	if renderer.details {
		for _, reason := range retry.Reasons {
			renderer.nestedBullet("Reason", retryReasonText(reason))
		}
		if references := renderer.view.referenceList(retry.SupportingEvidence); references != "" {
			renderer.statement("Evidence", references)
		}
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
		renderer.paragraph("  • "+renderer.style.muted("["+alias+"]")+" ", "    ", detail)
	}
}

func (renderer *humanRenderer) renderEvidenceHighlights() {
	identifiers := renderer.view.evidenceHighlights(defaultEvidenceLimit)
	if len(identifiers) == 0 {
		return
	}
	renderer.section("Evidence highlights")
	for _, id := range identifiers {
		alias := renderer.view.evidenceAlias(id)
		detail := renderer.view.evidenceDetail(id)
		renderer.paragraph("  • "+renderer.style.muted("["+alias+"]")+" ", "    ", detail)
	}
	if remaining := len(renderer.view.evidenceOrder) - len(identifiers); remaining > 0 {
		renderer.paragraph("  • ", "    ", fmt.Sprintf("%d additional facts are available with --details.", remaining))
	}
}

func (renderer *humanRenderer) renderActions() {
	if len(renderer.view.report.Actions) == 0 {
		return
	}
	renderer.section("Recommended next steps")
	limit := len(renderer.view.report.Actions)
	if !renderer.details && limit > defaultActionLimit {
		limit = defaultActionLimit
	}
	for index, action := range renderer.view.report.Actions[:limit] {
		prefix := "  " + strconv.Itoa(index+1) + ". "
		renderer.paragraph(prefix, "     ", renderer.style.action(action.Summary))
		renderer.actionParagraph(action.Description)
		if renderer.details {
			renderer.actionDetail("Type", actionKindText(action.Kind))
			if references := renderer.view.referenceList(action.SupportingEvidence); references != "" {
				renderer.actionDetail("Evidence", references)
			}
		}
		if action.Execution == diagnosis.ActionExecutionReadOnly {
			renderer.actionDetailStyled("Suggested command", renderer.style.command(formatArguments(action.Arguments)))
			renderer.actionDetail("Execution", "Read-only suggestion; not run automatically")
		}
		if action.RequiresConfirmation {
			renderer.actionDetail("Confirmation", "Required; Jobman Diagnose will not make this change")
		}
	}
	if remaining := len(renderer.view.report.Actions) - limit; remaining > 0 {
		renderer.paragraph("  • ", "    ", fmt.Sprintf("%d additional recommendations are available with --details.", remaining))
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
	warnings := make([]diagnosis.Warning, 0, len(renderer.view.report.Warnings))
	for _, warning := range renderer.view.report.Warnings {
		if warning.Code != uncalibratedWarningCode {
			warnings = append(warnings, warning)
		}
	}
	if len(warnings) == 0 {
		return
	}
	renderer.section("Caveats")
	for _, value := range warnings {
		renderer.bullet("", value.Message)
	}
}

func (renderer *humanRenderer) renderAIDisclosure() {
	report := renderer.view.report
	if !report.Disclosure.ProviderInvoked {
		return
	}
	renderer.section("AI disclosure")
	if report.Disclosure.GeneratedContentUsed {
		renderer.paragraph(
			"  • ", "    ",
			"Validated generated hypotheses from "+report.Disclosure.Model+" contributed to this diagnosis.",
		)
		renderer.paragraph(
			"  • ", "    ",
			"Generated conclusions are advisory; Jobman's observed facts and retry policy remain authoritative.",
		)
		return
	}
	renderer.paragraph("  • ", "    ", "No generated hypothesis was accepted; this is the complete deterministic diagnosis.")
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

func (renderer *humanRenderer) generatedFindings() []diagnosis.Finding {
	result := make([]diagnosis.Finding, 0)
	for _, finding := range renderer.view.report.Findings {
		if isGeneratedFinding(finding) {
			result = append(result, finding)
		}
	}

	return result
}

func (renderer *humanRenderer) additionalDeterministicFindings() []diagnosis.Finding {
	result := make([]diagnosis.Finding, 0)
	for _, finding := range renderer.view.report.Findings {
		if finding.ID != renderer.view.primary.ID && !isGeneratedFinding(finding) {
			result = append(result, finding)
		}
	}

	return result
}

func (renderer *humanRenderer) referenceList(ids []string) string {
	ids = renderer.view.orderedEvidenceReferences(ids)
	if renderer.details || len(ids) <= defaultEvidenceLimit {
		return renderer.view.referenceList(ids)
	}
	visible := renderer.view.referenceList(ids[:defaultEvidenceLimit])

	return visible + fmt.Sprintf(" (+%d more; --details)", len(ids)-defaultEvidenceLimit)
}

func isGeneratedFinding(finding diagnosis.Finding) bool {
	return strings.HasPrefix(finding.Analyzer, "generator.")
}

func generatedCauseDetails(finding diagnosis.Finding) (string, string, bool) {
	const (
		rootPrefix = "Root cause: "
		pathMarker = " Failure path: "
	)
	if !isGeneratedFinding(finding) || !strings.HasPrefix(finding.Explanation, rootPrefix) {
		return "", "", false
	}
	remaining := strings.TrimPrefix(finding.Explanation, rootPrefix)
	marker := strings.Index(remaining, pathMarker)
	if marker < 1 {
		return "", "", false
	}
	rootCause := strings.TrimSpace(remaining[:marker])
	failurePath := strings.TrimSpace(remaining[marker+len(pathMarker):])
	if rootCause == "" || failurePath == "" {
		return "", "", false
	}

	return rootCause, failurePath, true
}

func equivalentText(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.Join(strings.Fields(value), " "))
		return strings.Trim(value, " .,:;!?-")
	}

	return normalize(left) == normalize(right)
}

func (renderer *humanRenderer) section(title string) {
	if renderer.output.Len() != 0 {
		renderer.output.WriteByte('\n')
	}
	renderer.output.WriteString(renderer.style.section(title))
	renderer.output.WriteString("\n\n")
}

func (renderer *humanRenderer) paragraph(prefix, continuation, value string) {
	appendWrapped(&renderer.output, prefix, continuation, value, renderer.width)
}

func (renderer *humanRenderer) field(label, value string) {
	renderer.statement(label, value)
}

func (renderer *humanRenderer) statement(label, value string) {
	if value == "" {
		return
	}
	prefix := "  • " + renderer.style.label(label) + ": "
	renderer.paragraph(prefix, "    ", value)
}

func (renderer *humanRenderer) subfield(label, value string) {
	renderer.subfieldStyled(label, value)
}

func (renderer *humanRenderer) subfieldStyled(label, value string) {
	if value == "" {
		return
	}
	prefix := "    - " + renderer.style.label(label) + ": "
	renderer.paragraph(prefix, strings.Repeat(" ", visibleWidth(prefix)), value)
}

func (renderer *humanRenderer) subparagraph(value string) {
	renderer.paragraph("    ", "    ", value)
}

func (renderer *humanRenderer) bullet(label, value string) {
	prefix := "  • "
	if label != "" {
		prefix += renderer.style.label(label) + ": "
	}
	renderer.paragraph(prefix, "    ", value)
}

func (renderer *humanRenderer) nestedBullet(label, value string) {
	prefix := "    - "
	if label != "" {
		prefix += renderer.style.label(label) + ": "
	}
	renderer.paragraph(prefix, "      ", value)
}

func (renderer *humanRenderer) findingDetail(label, value string) {
	if value == "" {
		return
	}
	prefix := "      - " + renderer.style.label(label) + ": "
	renderer.paragraph(prefix, strings.Repeat(" ", visibleWidth(prefix)), value)
}

func (renderer *humanRenderer) actionParagraph(value string) {
	renderer.paragraph("     ", "     ", value)
}

func (renderer *humanRenderer) actionDetail(label, value string) {
	renderer.actionDetailStyled(label, value)
}

func (renderer *humanRenderer) actionDetailStyled(label, value string) {
	if value == "" {
		return
	}
	prefix := "     - " + renderer.style.label(label) + ": "
	renderer.paragraph(prefix, strings.Repeat(" ", visibleWidth(prefix)), value)
}

func formatConfidence(confidence diagnosis.Confidence, analyzer string) string {
	if strings.HasPrefix(analyzer, "generator.") {
		return "Advisory; confidence not calibrated"
	}
	value := titleWords(confidence.Band) + " (" + strconv.Itoa(confidence.Score) + "/100)"

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
