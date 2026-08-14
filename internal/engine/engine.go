// Package engine implements the deterministic diagnosis pipeline.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/sourcecontext"
)

const deterministicAnalyzer = "builtin.rules/1"

// Engine applies controlled rules to a sealed evidence bundle.
type Engine struct {
	companionVersion string
	now              func() time.Time
}

// New returns a deterministic, network-free diagnostician.
func New(companionVersion string, now func() time.Time) (*Engine, error) {
	if strings.TrimSpace(companionVersion) == "" {
		return nil, errors.New("construct diagnosis engine: companion version must not be empty")
	}
	if now == nil {
		return nil, errors.New("construct diagnosis engine: clock must not be nil")
	}

	return &Engine{companionVersion: companionVersion, now: now}, nil
}

// Diagnose produces a sealed report whose claims cite the supplied evidence.
func (engine *Engine) Diagnose(ctx context.Context, evidence diagnosis.FailureEvidence) (diagnosis.Report, error) {
	if ctx == nil {
		return diagnosis.Report{}, errors.New("diagnose: nil context")
	}
	if err := diagnosis.VerifyFailureEvidence(evidence); err != nil {
		return diagnosis.Report{}, fmt.Errorf("diagnose: verify evidence: %w", err)
	}
	core := evidence.Core
	view := newEvidenceView(evidence)
	candidates, err := analyze(ctx, view)
	if err != nil {
		return diagnosis.Report{}, err
	}
	slices.SortStableFunc(candidates, compareCandidates)
	candidates = uniqueCandidates(candidates)
	if len(candidates) == 0 {
		return diagnosis.Report{}, errors.New("diagnose: deterministic analyzers returned no finding")
	}
	primary := candidates[0]
	findings := make([]diagnosis.Finding, 0, len(candidates))
	for index, candidate := range candidates {
		finding := candidate.finding
		finding.ID = fmt.Sprintf("finding:%03d:%s", index+1, strings.ReplaceAll(finding.Code, ".", "-"))
		findings = append(findings, finding)
	}
	actions := actionsFor(primary, view)
	retry := retryFor(primary, view)
	missing, warnings := limitations(view, primary)
	warnings = append(warnings, sourceContextWarnings(evidence)...)
	references := referencedEvidence(findings, actions, retry)
	citations, err := buildCitations(view, references)
	if err != nil {
		return diagnosis.Report{}, err
	}
	report := diagnosis.Report{
		GeneratedAt: engine.now().UTC(), CoreEvidenceID: core.EvidenceID,
		AnalysisEvidenceID: evidence.AnalysisEvidenceID,
		Versions: diagnosis.Versions{
			CompanionVersion: engine.companionVersion, EngineVersion: diagnosis.EngineVersion,
			JobmanVersion: core.Source.JobmanVersion, EvidenceSchemaVersion: core.SchemaVersion,
			ReportSchemaVersion: diagnosis.SchemaVersion,
		},
		Analyzers: analyzerDescriptors(evidence),
		Subject: diagnosis.Subject{
			JobID: core.Subject.JobID, JobRevision: core.Subject.JobRevision,
			SelectedRuns: slices.Clone(core.Subject.SelectedRuns), Phase: core.Subject.Phase,
			Outcome: core.Subject.Outcome,
		},
		Mode: diagnosis.ModeDeterministic, PrimaryFindingID: findings[0].ID,
		Findings: findings, Actions: actions, Retry: retry, Citations: citations,
		MissingEvidence: missing, Warnings: warnings,
		Disclosure:   diagnosis.DisclosureManifest{Locality: diagnosis.ProviderNotUsed},
		Fingerprints: diagnosis.Fingerprints{Core: coreFailureFingerprint(view)},
	}
	sealed, err := diagnosis.Seal(report)
	if err != nil {
		return diagnosis.Report{}, fmt.Errorf("diagnose: seal report: %w", err)
	}
	if err := diagnosis.ValidateAgainstEvidence(sealed, evidence); err != nil {
		return diagnosis.Report{}, fmt.Errorf("diagnose: validate report: %w", err)
	}

	return sealed, nil
}

func sourceContextWarnings(evidence diagnosis.FailureEvidence) []diagnosis.Warning {
	for _, source := range evidence.SourceContext {
		assessment := sourcecontext.Assess(evidence.Core.Artifacts, source)
		if assessment.Status == sourcecontext.AssessmentMismatch {
			return []diagnosis.Warning{{
				Code: "source_context_mismatch", Message: sourcecontext.MismatchMessage(assessment),
			}}
		}
	}

	return []diagnosis.Warning{}
}

func analyzerDescriptors(evidence diagnosis.FailureEvidence) []diagnosis.AnalyzerDescriptor {
	result := []diagnosis.AnalyzerDescriptor{{Name: "builtin.rules", Version: "1"}}
	seen := map[diagnosis.AnalyzerDescriptor]struct{}{result[0]: {}}
	for _, item := range evidence.Enrichment {
		if _, ok := seen[item.Collector]; ok {
			continue
		}
		seen[item.Collector] = struct{}{}
		result = append(result, item.Collector)
	}
	slices.SortFunc(result, func(left, right diagnosis.AnalyzerDescriptor) int {
		if left.Name != right.Name {
			return strings.Compare(left.Name, right.Name)
		}

		return strings.Compare(left.Version, right.Version)
	})

	return result
}

type evidenceView struct {
	evidence         diagnostic.Evidence
	failure          diagnosis.FailureEvidence
	itemsByID        map[string]diagnostic.Item
	byCode           map[string][]diagnostic.Item
	artifacts        map[string]diagnostic.Artifact
	omissions        map[string]diagnostic.Omission
	enrichment       map[string]diagnosis.EnrichmentItem
	byEnrichmentCode map[string][]diagnosis.EnrichmentItem
}

func newEvidenceView(evidence diagnosis.FailureEvidence) evidenceView {
	core := evidence.Core
	view := evidenceView{
		evidence: core, failure: evidence, itemsByID: make(map[string]diagnostic.Item, len(core.Items)),
		byCode: make(map[string][]diagnostic.Item), artifacts: make(map[string]diagnostic.Artifact, len(core.Artifacts)),
		omissions:        make(map[string]diagnostic.Omission, len(core.Omissions)),
		enrichment:       make(map[string]diagnosis.EnrichmentItem, len(evidence.Enrichment)),
		byEnrichmentCode: make(map[string][]diagnosis.EnrichmentItem),
	}
	for _, item := range core.Items {
		view.itemsByID[item.ID] = item
		view.byCode[item.Code] = append(view.byCode[item.Code], item)
	}
	for _, artifact := range core.Artifacts {
		view.artifacts[artifact.ID] = artifact
	}
	for _, omission := range core.Omissions {
		view.omissions[omission.Code] = omission
	}
	for _, item := range evidence.Enrichment {
		view.enrichment[item.ID] = item
		view.byEnrichmentCode[item.Code] = append(view.byEnrichmentCode[item.Code], item)
	}

	return view
}

func (view evidenceView) primaryItems(code string) []diagnostic.Item {
	items := view.byCode[code]
	if len(view.evidence.Subject.SelectedRuns) == 0 {
		return slices.Clone(items)
	}
	selected := view.evidence.Subject.SelectedRuns[len(view.evidence.Subject.SelectedRuns)-1]
	prefix := fmt.Sprintf("ev:run:%020d:", selected)
	result := make([]diagnostic.Item, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.ID, prefix) || strings.HasPrefix(item.ID, "ev:job:") {
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return slices.Clone(items)
	}

	return result
}

type candidate struct {
	priority int
	finding  diagnosis.Finding
}

func analyze(ctx context.Context, view evidenceView) ([]candidate, error) {
	var candidates []candidate
	if intentional, ok := intentionalFalseCandidate(view); ok {
		candidates = append(candidates, intentional)
	}
	for _, item := range view.primaryItems(diagnostic.CodeFailureClass) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("diagnose: %w", err)
		}
		var value struct {
			Class string `json:"class"`
			Scope string `json:"scope"`
		}
		if err := json.Unmarshal(item.Value, &value); err != nil || value.Class == "" {
			continue
		}
		candidates = append(candidates, coreFailureCandidate(value.Class, item.ID, relatedEvidence(view, item.ID)...))
	}
	candidates = append(candidates, artifactCandidates(ctx, view)...)
	candidates = append(candidates, enrichmentCandidates(view)...)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("diagnose: %w", err)
	}
	candidates = append(candidates, secondaryCandidates(view)...)
	if len(candidates) == 0 {
		candidates = append(candidates, stateCandidate(view))
	}

	return candidates, nil
}

func intentionalFalseCandidate(view evidenceView) (candidate, bool) {
	commandID := ""
	executable := ""
	for _, item := range view.byCode[diagnostic.CodeTargetCommand] {
		var value diagnostic.Command
		if json.Unmarshal(item.Value, &value) != nil ||
			(value.Executable != "/usr/bin/false" && value.Executable != "/bin/false") {
			continue
		}
		commandID = item.ID
		executable = value.Executable
		break
	}
	if commandID == "" {
		return candidate{}, false
	}
	classID := ""
	for _, item := range view.primaryItems(diagnostic.CodeFailureClass) {
		var value struct {
			Class string `json:"class"`
		}
		if json.Unmarshal(item.Value, &value) == nil && value.Class == "nonzero_exit" {
			classID = item.ID
			break
		}
	}
	exitID := ""
	for _, item := range view.primaryItems(diagnostic.CodeRunExitCode) {
		var exitCode int
		if json.Unmarshal(item.Value, &exitCode) == nil && exitCode != 0 {
			exitID = item.ID
			break
		}
	}
	if classID == "" || exitID == "" {
		return candidate{}, false
	}

	return exactCandidate(
		100,
		"core.intentional_false",
		"application",
		diagnosis.SeverityInfo,
		"The command was explicitly configured to report failure",
		fmt.Sprintf("The selected target %s is the standard false utility, whose defined behavior is to exit unsuccessfully. Jobman therefore recorded the requested result.", executable),
		[]string{commandID, classID, exitID},
	), true
}

func coreFailureCandidate(class, classEvidence string, related ...string) candidate {
	support := append([]string{classEvidence}, related...)
	switch class {
	case "executable_not_found":
		return exactCandidate(100, "core.executable_not_found", "launch", diagnosis.SeverityError,
			"The target executable was not found",
			"Jobman directly failed executable resolution before the target process started.", support)
	case "working_directory_missing":
		return exactCandidate(99, "core.working_directory_missing", "launch", diagnosis.SeverityError,
			"The working directory was unavailable",
			"Jobman directly observed that the configured working directory could not be opened.", support)
	case "permission_denied":
		return exactCandidate(98, "core.permission_denied", "launch", diagnosis.SeverityError,
			"Permission was denied while starting the target",
			"Jobman received a permission error during a direct target-start operation.", support)
	case "job_timeout", "run_timeout", "timeout":
		return exactCandidate(96, "core.timeout", "policy", diagnosis.SeverityError,
			"A Jobman timeout ended the run",
			"The durable lifecycle state confirms that a configured timeout boundary was reached.", support)
	case "user_cancellation":
		return exactCandidate(95, "core.user_cancellation", "lifecycle", diagnosis.SeverityInfo,
			"The job was cancelled by an explicit request",
			"The durable cancellation intent, rather than a target fault, ended this execution.", support)
	case "ownership_lost", "supervisor_claim_expired":
		return exactCandidate(94, "core.supervisor_ownership_lost", "ownership", diagnosis.SeverityError,
			"Jobman lost or failed to establish supervisor ownership",
			"The durable supervisor lifecycle could not prove continued ownership of the managed process tree.", support)
	case "signal_termination":
		return exactCandidate(90, "core.signal_termination", "process", diagnosis.SeverityError,
			"The target terminated because of an observed signal",
			"Jobman recorded an operating-system signal as the factual termination mechanism.", support)
	case "nonzero_exit":
		return observedCandidate(82, "core.nonzero_exit", "process", diagnosis.SeverityError,
			"The target exited with a nonzero status",
			"The exit status confirms target failure, but does not by itself identify the target's root cause.", support)
	case "target_start_failed", "submission_failed":
		return observedCandidate(80, "core.start_failed", "launch", diagnosis.SeverityError,
			"The target did not start",
			"Jobman recorded a start failure, but the available class does not establish a more specific cause.", support)
	case "wait_evaluation_error":
		return observedCandidate(78, "core.wait_evaluation_error", "prerequisite", diagnosis.SeverityError,
			"A prerequisite evaluation failed",
			"Jobman could not complete a configured wait-condition evaluation.", support)
	case "log_recording_degraded":
		return observedCandidate(55, "core.log_recording_degraded", "logging", diagnosis.SeverityWarning,
			"Jobman log recording was degraded",
			"The target result and Jobman's ability to record output are separate; the recording path reported degradation.", support)
	default:
		return observedCandidate(65, "core.target_failure", "process", diagnosis.SeverityError,
			"The target failed",
			"Jobman recorded a target failure, but the available structured facts do not establish a narrower cause.", support)
	}
}

func exactCandidate(priority int, code, category string, severity diagnosis.Severity, summary, explanation string, support []string) candidate {
	//nolint:errcheck // Scores and bases are controlled constants; Seal validates them again.
	confidence, _ := diagnosis.NewConfidence(min(priority, 100), "Jobman directly established the cited condition.")

	return candidate{priority: priority, finding: diagnosis.Finding{
		Code: code, Category: category, Severity: severity, Summary: summary, Explanation: explanation,
		Confidence: confidence, SupportingEvidence: support, ContradictingEvidence: []string{}, Analyzer: deterministicAnalyzer,
	}}
}

func observedCandidate(priority int, code, category string, severity diagnosis.Severity, summary, explanation string, support []string) candidate {
	score := min(priority, 85)
	//nolint:errcheck // Scores and bases are controlled constants; Seal validates them again.
	confidence, _ := diagnosis.NewConfidence(score, "The cited observations establish the failure mechanism but may not establish its root cause.")

	return candidate{priority: priority, finding: diagnosis.Finding{
		Code: code, Category: category, Severity: severity, Summary: summary, Explanation: explanation,
		Confidence: confidence, SupportingEvidence: support, ContradictingEvidence: []string{}, Analyzer: deterministicAnalyzer,
	}}
}

func artifactCandidates(ctx context.Context, view evidenceView) []candidate {
	result := make([]candidate, 0)
	for _, artifact := range view.evidence.Artifacts {
		if ctx.Err() != nil {
			return result
		}
		lower := bytes.ToLower(artifact.Data)
		switch {
		case len(view.byEnrichmentCode["enrichment.traceback.python"]) == 0 &&
			bytes.Contains(lower, []byte("traceback (most recent call last)")):
			result = append(result, heuristicCandidate(75, "target.python_exception", "application",
				"A Python exception traceback is present",
				"The bounded target log contains a Python traceback signature. The exception type and application cause were not promoted to trusted metadata.", artifact.ID))
		case bytes.Contains(lower, []byte("no space left on device")):
			result = append(result, heuristicCandidate(72, "target.storage_exhausted_message", "resource",
				"The target reported exhausted storage",
				"The bounded target log contains a conventional storage-exhaustion message; Jobman did not independently confirm filesystem capacity.", artifact.ID))
		case bytes.Contains(lower, []byte("command not found")):
			result = append(result, heuristicCandidate(68, "target.shell_command_not_found", "application",
				"A shell reported a missing nested command",
				"The target log contains a shell command-not-found signature. This concerns a command inside the target, not Jobman's direct executable lookup.", artifact.ID))
		case bytes.Contains(lower, []byte("permission denied")):
			result = append(result, heuristicCandidate(62, "target.permission_message", "application",
				"The target reported a permission problem",
				"The bounded target log contains a permission-denied signature, but the affected target operation is not structured evidence.", artifact.ID))
		case bytes.Contains(lower, []byte("connection refused")):
			result = append(result, heuristicCandidate(58, "target.connection_refused_message", "network",
				"The target reported a refused connection",
				"The bounded target log contains a connection-refused signature; the destination and network layer were not independently verified.", artifact.ID))
		}
	}

	return result
}

func enrichmentCandidates(view evidenceView) []candidate {
	result := make([]candidate, 0, len(view.enrichment))
	latestDiagnostic := make(map[string]uint64)
	for _, item := range view.failure.Enrichment {
		if item.Code == enrichment.CodeCausalMessage && item.ByteStart >= latestDiagnostic[item.SourceArtifactID] {
			latestDiagnostic[item.SourceArtifactID] = item.ByteStart
		}
	}
	for _, item := range view.failure.Enrichment {
		switch item.Code {
		case enrichment.CodePythonTraceback:
			result = append(result, heuristicCandidate(76, "target.python_exception", "application",
				"A Python exception traceback is present",
				"The bounded target log contains a structurally delimited Python traceback. Its exact sanitized byte range is attributed as companion enrichment.", item.ID))
		case enrichment.CodeGoPanic:
			result = append(result, heuristicCandidate(75, "target.go_panic", "application",
				"A Go panic stack is present",
				"The bounded target log contains a structurally delimited Go panic and stack range.", item.ID))
		case enrichment.CodeJVMException:
			result = append(result, heuristicCandidate(73, "target.jvm_exception", "application",
				"A JVM exception chain is present",
				"The bounded target log contains a structurally delimited JVM exception or caused-by chain.", item.ID))
		case enrichment.CodeCompilerDiagnostic:
			result = append(result, heuristicCandidate(70, "target.compiler_error", "application",
				"A compiler error diagnostic is present",
				"The bounded target log contains a conventional file/line/column compiler error record.", item.ID))
		case enrichment.CodeCausalMessage:
			format := diagnosticFormat(item, view)
			rule, ok := targetDiagnosticRules[format]
			if !ok {
				continue
			}
			priority := 84
			if latestDiagnostic[item.SourceArtifactID] == item.ByteStart {
				priority = 86
			}
			result = append(result, heuristicCandidate(
				priority, rule.code, rule.category, rule.summary, rule.explanation, item.ID,
			))
		}
	}

	return result
}

type targetDiagnosticRule struct {
	code        string
	category    string
	summary     string
	explanation string
}

var targetDiagnosticRules = map[string]targetDiagnosticRule{
	"address_in_use": {
		code: "target.address_in_use_message", category: "network",
		summary:     "The target reported that a listen address was already in use",
		explanation: "An exact bounded target-log range contains an address-in-use signature. Jobman did not independently inspect host listeners.",
	},
	"authentication_denied": {
		code: "target.authentication_denied_message", category: "access",
		summary:     "The target reported that a remote request was unauthorized",
		explanation: "An exact bounded target-log range contains an unauthorized-response signature. Jobman did not independently validate credentials or remote policy.",
	},
	"configuration_missing": {
		code: "target.configuration_missing_message", category: "configuration",
		summary:     "The target reported a missing required configuration value",
		explanation: "An exact bounded target-log range contains a missing-configuration signature. Jobman did not independently inspect the target's configuration source.",
	},
	"connection_refused": {
		code: "target.connection_refused_message", category: "network",
		summary:     "The target reported a refused connection",
		explanation: "An exact bounded target-log range contains a connection-refused signature. Jobman did not independently verify the destination or network layer.",
	},
	"data_validation": {
		code: "target.data_validation_message", category: "data",
		summary:     "The target reported that input data was rejected",
		explanation: "An exact bounded target-log range contains a parse or validation signature. Jobman did not independently validate the input or application rule.",
	},
	"database_deadlock": {
		code: "target.database_deadlock_message", category: "database",
		summary:     "The target reported a database deadlock",
		explanation: "An exact bounded target-log range contains a database-deadlock signature. Jobman did not independently inspect database locks or transaction state.",
	},
	"database_unique_violation": {
		code: "target.database_unique_violation_message", category: "data",
		summary:     "The target reported a database uniqueness violation",
		explanation: "An exact bounded target-log range contains a duplicate-key signature. Jobman did not independently inspect the database record or constraint.",
	},
	"deadline_exceeded": {
		code: "target.deadline_exceeded_message", category: "network",
		summary:     "The target reported that an operation exceeded its deadline",
		explanation: "An exact bounded target-log range contains a deadline or timeout signature. Jobman did not independently determine which dependency or workload caused the delay.",
	},
	"dependency_missing": {
		code: "target.dependency_missing_message", category: "dependency",
		summary:     "The target reported a missing runtime or build dependency",
		explanation: "An exact bounded target-log range contains a missing module, artifact, or shared-library signature. Jobman did not independently inspect installed dependencies.",
	},
	"dns_resolution_failed": {
		code: "target.dns_resolution_message", category: "network",
		summary:     "The target reported a DNS resolution failure",
		explanation: "An exact bounded target-log range contains a name-resolution signature. Jobman did not independently query DNS or verify the reported host.",
	},
	"file_descriptor_exhausted": {
		code: "target.file_descriptor_exhausted_message", category: "resource",
		summary:     "The target reported exhausted file descriptors",
		explanation: "An exact bounded target-log range contains a file-descriptor-exhaustion signature. Jobman did not independently inspect process or host limits.",
	},
	"linker_undefined_reference": {
		code: "target.linker_error_message", category: "dependency",
		summary:     "The target's linker reported an undefined reference",
		explanation: "An exact bounded target-log range contains an undefined-reference signature. Jobman did not independently inspect object files or linker inputs.",
	},
	"migration_rejected": {
		code: "target.migration_rejected_message", category: "configuration",
		summary:     "The target reported that its database migration state was rejected",
		explanation: "An exact bounded target-log range contains a migration-rejection signature. Jobman did not independently inspect the database schema version.",
	},
	"migration_required": {
		code: "target.migration_required_message", category: "configuration",
		summary:     "The target reported that database migrations are required",
		explanation: "An exact bounded target-log range contains a migration-required signature. Jobman did not independently inspect or modify the database schema.",
	},
	"missing_file": {
		code: "target.missing_file_message", category: "dependency",
		summary:     "The target reported a missing file or executable",
		explanation: "An exact bounded target-log range contains a file-not-found signature. Jobman did not independently resolve the referenced target path.",
	},
	"nested_command_missing": {
		code: "target.shell_command_not_found", category: "dependency",
		summary:     "A shell reported a missing nested command",
		explanation: "An exact bounded target-log range contains a shell command-not-found signature. Jobman did not independently resolve this command inside the target.",
	},
	"permission_denied": {
		code: "target.permission_message", category: "access",
		summary:     "The target reported a permission denial",
		explanation: "An exact bounded target-log range contains a permission-denied signature. Jobman did not independently inspect the affected target operation or access policy.",
	},
	"rate_limited": {
		code: "target.rate_limited_message", category: "network",
		summary:     "The target reported that a remote service rate-limited a request",
		explanation: "An exact bounded target-log range contains a too-many-requests signature. Jobman did not independently verify the remote service or retry window.",
	},
	"read_only_filesystem": {
		code: "target.read_only_filesystem_message", category: "access",
		summary:     "The target reported a read-only filesystem",
		explanation: "An exact bounded target-log range contains a read-only-filesystem signature. Jobman did not independently inspect the mount or storage policy.",
	},
	"service_unavailable": {
		code: "target.service_unavailable_message", category: "network",
		summary:     "The target reported that a remote service was unavailable",
		explanation: "An exact bounded target-log range contains a service-unavailable signature. Jobman did not independently verify the service or its health.",
	},
	"storage_exhausted": {
		code: "target.storage_exhausted_message", category: "resource",
		summary:     "The target reported exhausted storage",
		explanation: "An exact bounded target-log range contains a storage-exhaustion signature. Jobman did not independently confirm filesystem capacity.",
	},
	"tls_verification_failed": {
		code: "target.tls_verification_message", category: "network",
		summary:     "The target reported a TLS certificate verification failure",
		explanation: "An exact bounded target-log range contains a certificate-verification signature. Jobman did not independently inspect the certificate chain or trust store.",
	},
}

func diagnosticFormat(item diagnosis.EnrichmentItem, view evidenceView) string {
	if item.Format != "causal_message" {
		return item.Format
	}
	artifact, ok := view.artifacts[item.SourceArtifactID]
	if !ok || item.ByteEnd > uint64(len(artifact.Data)) || item.ByteStart >= item.ByteEnd {
		return ""
	}

	return enrichment.ClassifyDiagnostic(artifact.Data[item.ByteStart:item.ByteEnd])
}

func heuristicCandidate(priority int, code, category, summary, explanation, artifactID string) candidate {
	//nolint:errcheck // Scores and bases are controlled constants; Seal validates them again.
	confidence, _ := diagnosis.NewConfidence(priority, "A deterministic signature matched untrusted, bounded target output.")

	return candidate{priority: priority, finding: diagnosis.Finding{
		Code: code, Category: category, Severity: diagnosis.SeverityError,
		Summary: summary, Explanation: explanation, Confidence: confidence,
		SupportingEvidence: []string{artifactID}, ContradictingEvidence: []string{}, Analyzer: deterministicAnalyzer,
	}}
}

func secondaryCandidates(view evidenceView) []candidate {
	result := make([]candidate, 0, 4)
	for _, item := range view.byCode[diagnostic.CodeLogRecordingHealth] {
		var health string
		if json.Unmarshal(item.Value, &health) == nil && health == "degraded" {
			result = append(result, observedCandidate(52, "secondary.log_recording_degraded", "logging",
				diagnosis.SeverityWarning, "Captured logs may be incomplete",
				"Jobman's recording health is degraded, so absence of a message in the captured output is not strong contrary evidence.", []string{item.ID}))
			break
		}
	}
	for _, item := range view.byCode[diagnostic.CodeNotificationStatus] {
		var value struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(item.Value, &value) == nil && value.Status == "failed" {
			result = append(result, observedCandidate(50, "secondary.notification_failed", "notification",
				diagnosis.SeverityWarning, "A notification delivery failed",
				"The notification failure is separate from the managed target's outcome and may need independent remediation.", []string{item.ID}))
			break
		}
	}
	if history, found := sameFingerprintHistoryCandidate(view); found {
		result = append(result, history)
	} else if repeatedID := repeatedFailureEvidence(view); repeatedID != "" {
		result = append(result, observedCandidate(48, "secondary.repeated_failure", "history",
			diagnosis.SeverityWarning, "The same structured failure class occurred in multiple runs",
			"Repeated attempts with the same factual failure class reduce the value of an unchanged immediate retry.", []string{repeatedID}))
	}

	return result
}

func sameFingerprintHistoryCandidate(view evidenceView) (candidate, bool) {
	items := view.byCode[diagnostic.CodeSimilarFailure]
	if len(items) == 0 {
		return candidate{}, false
	}
	support := make([]string, 0, len(items))
	laterSucceeded := 0
	for _, item := range items {
		var failure diagnostic.SimilarFailure
		if err := json.Unmarshal(item.Value, &failure); err != nil || failure.Validate() != nil {
			continue
		}
		support = append(support, item.ID)
		if failure.LaterSucceeded {
			laterSucceeded++
		}
	}
	if len(support) == 0 {
		return candidate{}, false
	}
	explanation := fmt.Sprintf(
		"Jobman's store-local fingerprint matched %d other completed failure(s). An unchanged retry is less useful when the same factual pattern keeps recurring.",
		len(support),
	)
	if laterSucceeded != 0 {
		explanation = fmt.Sprintf(
			"Jobman's store-local fingerprint matched %d other completed failure(s); %d were followed by a successful run of the same job. This supports a transient-history signal, not a proven root cause.",
			len(support),
			laterSucceeded,
		)
	}
	//nolint:errcheck // Score and basis are controlled constants; Seal validates them again.
	confidence, _ := diagnosis.NewConfidence(90, "The cited matches came from Jobman's exact, keyed factual fingerprint index.")

	return candidate{priority: 60, finding: diagnosis.Finding{
		Code: "secondary.same_fingerprint_history", Category: "history", Severity: diagnosis.SeverityWarning,
		Summary: "The same store-local failure fingerprint occurred before", Explanation: explanation,
		Confidence: confidence, SupportingEvidence: support, ContradictingEvidence: []string{}, Analyzer: deterministicAnalyzer,
	}}, true
}

func stateCandidate(view evidenceView) candidate {
	if view.evidence.Subject.Outcome == "success" {
		item := firstItemID(view.byCode[diagnostic.CodeJobOutcome], view.byCode[diagnostic.CodeJobPhase])
		//nolint:errcheck // Scores and bases are controlled constants; Seal validates them again.
		confidence, _ := diagnosis.NewConfidence(98, "Jobman's durable outcome directly records target success.")
		return candidate{priority: 40, finding: diagnosis.Finding{
			Code: "core.no_target_failure", Category: "state", Severity: diagnosis.SeverityInfo,
			Summary:     "Jobman recorded no target failure",
			Explanation: "The selected durable state is successful. Delays, retries, logging degradation, or notification failures may still appear as secondary findings.",
			Confidence:  confidence, SupportingEvidence: optionalEvidence(item), ContradictingEvidence: []string{},
			Analyzer: deterministicAnalyzer,
		}}
	}
	item := firstItemID(view.byCode[diagnostic.CodeJobPhase], view.byCode[diagnostic.CodeJobOutcome])
	return observedCandidate(35, "core.insufficient_structured_evidence", "state", diagnosis.SeverityWarning,
		"The available facts do not establish a diagnosis",
		"Collect a bounded log tail or additional platform facts before changing the command based on speculation.", optionalEvidence(item))
}

func relatedEvidence(view evidenceView, classID string) []string {
	prefix := classID[:strings.LastIndex(classID, ":")+1]
	preferred := []string{
		diagnostic.CodeRunDiagnostic, diagnostic.CodeRunOutcome, diagnostic.CodeRunExitSignal,
		diagnostic.CodeRunExitCode, diagnostic.CodeRunTimeoutScope, diagnostic.CodeResourceObservation,
		diagnostic.CodeJobDiagnostic,
		diagnostic.CodeJobOutcome,
	}
	result := make([]string, 0, len(preferred))
	for _, code := range preferred {
		for _, item := range view.byCode[code] {
			if strings.HasPrefix(item.ID, prefix) || strings.HasPrefix(item.ID, "ev:job:") {
				result = append(result, item.ID)
			}
		}
	}

	return result
}

func repeatedFailureEvidence(view evidenceView) string {
	counts := make(map[string]int)
	lastID := make(map[string]string)
	for _, item := range view.byCode[diagnostic.CodeFailureClass] {
		if !strings.HasPrefix(item.ID, "ev:run:") {
			continue
		}
		var value struct {
			Class string `json:"class"`
		}
		if json.Unmarshal(item.Value, &value) == nil && value.Class != "" {
			counts[value.Class]++
			lastID[value.Class] = item.ID
		}
	}
	for class, count := range counts {
		if count > 1 {
			return lastID[class]
		}
	}

	return ""
}

func compareCandidates(left, right candidate) int {
	if left.priority > right.priority {
		return -1
	}
	if left.priority < right.priority {
		return 1
	}

	return strings.Compare(left.finding.Code, right.finding.Code)
}

func uniqueCandidates(values []candidate) []candidate {
	seen := make(map[string]struct{}, len(values))
	result := make([]candidate, 0, len(values))
	for _, value := range values {
		if _, duplicate := seen[value.finding.Code]; duplicate {
			continue
		}
		seen[value.finding.Code] = struct{}{}
		result = append(result, value)
	}

	return result
}

func firstItemID(groups ...[]diagnostic.Item) string {
	for _, group := range groups {
		if len(group) > 0 {
			return group[0].ID
		}
	}

	return ""
}

func optionalEvidence(value string) []string {
	if value == "" {
		return []string{}
	}

	return []string{value}
}
