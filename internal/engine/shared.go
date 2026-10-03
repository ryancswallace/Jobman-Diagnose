package engine

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

func applySharedReport(report *diagnosis.Report, provenance *diagnostic.SharedProvenance) {
	shared := *provenance
	shared.Runs = slices.Clone(provenance.Runs)
	shared.Logs = slices.Clone(provenance.Logs)
	report.Shared = &shared
	report.SchemaVersion = diagnosis.SharedSchemaVersion
	report.Versions.ReportSchemaVersion = diagnosis.SharedSchemaVersion
	report.Versions.EngineVersion = diagnosis.SharedEngineVersion
	report.Analyzers = append(report.Analyzers, diagnosis.AnalyzerDescriptor{Name: "builtin.shared", Version: "1"})
	for index := range report.Actions {
		// A local CLI selector would have no Control/namespace authority. Shared
		// recommendations are text only, even when analogous local advice has
		// an allowlisted read-only argv.
		report.Actions[index].Execution = diagnosis.ActionExecutionNone
		report.Actions[index].Arguments = []string{}
		report.Actions[index].SafeToAutomate = false
		if strings.Contains(report.Actions[index].Description, "local log command") {
			report.Actions[index].Description = "Inspect the full authorized logs for this job in its recorded Control deployment and namespace. Keep log content within the approved private environment."
		}
	}
	report.Retry.ExistingPolicy = diagnosis.PolicyUnknown
	if strings.HasPrefix(primaryReportCode(*report), "shared.") {
		report.Retry.Verdict = diagnosis.RetryUnknown
		report.Retry.Rationale = "Inspect the cited Control state before deciding whether to submit new work; this report does not establish a safe retry policy."
	}
	if primaryReportCode(*report) == "shared.cancelled" {
		report.Retry.Verdict = diagnosis.RetryNotApplicable
		report.Retry.Rationale = "Control records cancellation. Any new submission requires renewed user intent."
	}
}

func primaryReportCode(report diagnosis.Report) string {
	for _, finding := range report.Findings {
		if finding.ID == report.PrimaryFindingID {
			return finding.Code
		}
	}
	return ""
}

func (view evidenceView) sharedPrimaryItems(items []diagnostic.Item) []diagnostic.Item {
	runs := view.evidence.Shared.Runs
	if len(runs) == 0 {
		return slices.Clone(items)
	}
	selected := runs[len(runs)-1].ID
	result := make([]diagnostic.Item, 0, len(items))
	for _, item := range items {
		if item.Source.EntityID == selected || item.Source.EntityID == view.evidence.Subject.JobID {
			result = append(result, item)
		}
	}
	// Unlike local fixture fallback, never borrow another selected run's
	// observation when the primary run lacks that fact.
	return result
}

func (view evidenceView) primaryArtifact(id string) bool {
	if view.evidence.Shared == nil {
		return true
	}
	runs := view.evidence.Shared.Runs
	artifact, exists := view.artifacts[id]
	return exists && len(runs) != 0 && artifact.Run == runs[len(runs)-1].Number
}

func sharedCandidates(view evidenceView) []candidate {
	result := make([]candidate, 0)
	for _, item := range view.primaryItems(diagnostic.CodeRunExitCode) {
		var code int64
		if json.Unmarshal(item.Value, &code) == nil && code != 0 {
			result = append(result, coreFailureCandidate("nonzero_exit", item.ID))
		}
	}
	for _, item := range view.primaryItems(diagnostic.CodeRunExitSignal) {
		var signal string
		if json.Unmarshal(item.Value, &signal) == nil && signal != "" {
			result = append(result, coreFailureCandidate("signal_termination", item.ID))
		}
	}
	outcomes := view.byCode[diagnostic.CodeJobOutcome]
	if len(view.evidence.Shared.Runs) != 0 {
		outcomes = view.primaryItems(diagnostic.CodeRunOutcome)
	}
	for _, item := range outcomes {
		if finding, ok := sharedOutcomeCandidate(item); ok {
			result = append(result, finding)
		}
	}
	result = append(result, sharedStateCandidates(view)...)
	for index := range result {
		if strings.HasPrefix(result[index].finding.Code, "shared.") {
			result[index].finding.Analyzer = "builtin.shared/1"
			result[index].finding.Confidence.Basis = "The cited Control observation establishes the recorded state; it does not establish an unobserved process result or application cause."
		}
	}
	return result
}

func sharedOutcomeCandidate(item diagnostic.Item) (candidate, bool) {
	var outcome string
	if json.Unmarshal(item.Value, &outcome) != nil {
		return candidate{}, false
	}
	support := []string{item.ID}
	switch outcome {
	case "timed_out":
		return exactCandidate(96, "shared.timed_out", "policy", diagnosis.SeverityError,
			"Control records a timeout outcome",
			"The source records the terminal timeout outcome. This alone does not identify which execution or scheduler timeout boundary was reached.", support), true
	case "cancelled":
		return exactCandidate(95, "shared.cancelled", "lifecycle", diagnosis.SeverityInfo,
			"Control records a cancellation outcome",
			"The terminal outcome records cancellation. Cancellation intent and the observed terminal outcome are separate facts; the initiating actor is not inferred.", support), true
	case "lost":
		return exactCandidate(94, "shared.lost", "ownership", diagnosis.SeverityError,
			"Control cannot establish the execution's final result",
			"The cited outcome is lost. That does not prove the process stopped or establish an application exit result; reconcile execution ownership before submitting duplicate work.", support), true
	case "aborted":
		return observedCandidate(68, "shared.aborted", "lifecycle", diagnosis.SeverityWarning,
			"Control records an aborted outcome",
			"The source records an aborted outcome. Dependency disposition and lifecycle events may distinguish a skipped or blocked node from an execution that began.", support), true
	case "failure":
		return observedCandidate(65, "shared.failure", "process", diagnosis.SeverityError,
			"Control records a failure outcome",
			"The cited terminal outcome is failure. It does not establish a specific application cause or the result of a different historical run.", support), true
	default:
		return candidate{}, false
	}
}

func sharedFallbackCandidate(view evidenceView) candidate {
	outcomes := view.byCode[diagnostic.CodeJobOutcome]
	if len(view.evidence.Shared.Runs) != 0 {
		outcomes = view.primaryItems(diagnostic.CodeRunOutcome)
	}
	for _, item := range outcomes {
		var outcome string
		if json.Unmarshal(item.Value, &outcome) == nil && outcome == "success" {
			return exactCandidate(98, "core.no_target_failure", "state", diagnosis.SeverityInfo,
				"Control records success for the selected state",
				"The cited outcome records success. This does not establish the result of another run or exclude independent logging issues.", []string{item.ID})
		}
	}
	result := observedCandidate(35, "core.insufficient_structured_evidence", "state", diagnosis.SeverityWarning,
		"The selected snapshot does not establish a diagnosis",
		"A current job outcome is not substituted for missing historical run observations. Obtain the selected run's lifecycle facts or an explicitly requested bounded log tail.",
		optionalEvidence(firstItemID(view.byCode[diagnostic.CodeJobPhase])))
	result.finding.Confidence.Basis = "Available source observations do not establish the selected run's result or a failure mechanism."
	return result
}

func sharedStateCandidates(view evidenceView) []candidate {
	result := make([]candidate, 0)
	for _, item := range view.byCode[diagnostic.CodeSharedObservationConfidence] {
		var confidence string
		if json.Unmarshal(item.Value, &confidence) == nil && (confidence == "stale" || confidence == "uncertain") {
			result = append(result, observedCandidate(50, "shared.observation_uncertain", "state", diagnosis.SeverityWarning,
				"Execution observations are stale or uncertain",
				"Control's observation confidence limits what is known about current execution. A successful API fetch does not make an old execution observation current, and staleness is not failure.", []string{item.ID}))
		}
	}
	for _, item := range view.byCode[diagnostic.CodeSharedDependencyObservation] {
		var dependency diagnostic.SharedDependencyObservation
		if json.Unmarshal(item.Value, &dependency) == nil && !dependency.Satisfied {
			result = append(result, observedCandidate(70, "shared.dependency_unsatisfied", "prerequisite", diagnosis.SeverityWarning,
				"A source-reported dependency is unsatisfied",
				"Control reports an unmet prerequisite for this job. Inspect the cited predecessor outcome, predicate and disposition; this analyzer does not recalculate graph readiness.", []string{item.ID}))
		}
	}
	for _, item := range view.primaryItems(diagnostic.CodeSharedSchedulerObservation) {
		var scheduler diagnostic.SharedSchedulerObservation
		if json.Unmarshal(item.Value, &scheduler) == nil && (scheduler.State == "PENDING" || scheduler.State == "pending") {
			result = append(result, observedCandidate(45, "shared.scheduler_pending", "state", diagnosis.SeverityInfo,
				"The scheduler observation records pending work",
				"The cited scheduler observation records a pending state and its reason code at a specific time. It is independent of Control's job phase and current API availability.", []string{item.ID}))
		}
	}
	return result
}
