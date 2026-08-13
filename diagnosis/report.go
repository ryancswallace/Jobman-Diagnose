// Package diagnosis defines the stable, provider-independent diagnosis report
// contract emitted by jobman-diagnose.
package diagnosis

import (
	"context"
	"time"
)

const (
	// Kind identifies a diagnosis report document.
	Kind = "jobman.diagnosis_report"
	// SchemaVersion is the newest report schema understood by this package.
	SchemaVersion = 1
	// EngineVersion identifies the initial deterministic engine semantics.
	EngineVersion = "1.2.0"
)

// Diagnostician turns sealed core evidence plus attributed companion context
// into a cited report.
type Diagnostician interface {
	Diagnose(context.Context, FailureEvidence) (Report, error)
}

// Report is one immutable diagnosis of a specific sealed evidence bundle.
type Report struct {
	Kind               string                `json:"kind"`
	SchemaVersion      int                   `json:"schema_version"`
	ReportID           string                `json:"report_id"`
	GeneratedAt        time.Time             `json:"generated_at"`
	CoreEvidenceID     string                `json:"core_evidence_id"`
	AnalysisEvidenceID string                `json:"analysis_evidence_id"`
	Versions           Versions              `json:"versions"`
	Analyzers          []AnalyzerDescriptor  `json:"analyzers"`
	Generators         []GeneratorDescriptor `json:"generators"`
	Subject            Subject               `json:"subject"`
	Mode               AnalysisMode          `json:"mode"`
	PrimaryFindingID   string                `json:"primary_finding_id"`
	Findings           []Finding             `json:"findings"`
	Actions            []Action              `json:"actions"`
	Retry              RetryAdvice           `json:"retry"`
	Citations          []Citation            `json:"citations"`
	MissingEvidence    []MissingEvidence     `json:"missing_evidence"`
	Warnings           []Warning             `json:"warnings"`
	Disclosure         DisclosureManifest    `json:"disclosure"`
	Fingerprints       Fingerprints          `json:"fingerprints"`
}

// GeneratorDescriptor identifies the optional structured generator selected
// for an attempted augmentation. It records configuration identity, not a
// claim that generated content was accepted.
type GeneratorDescriptor struct {
	Provider string           `json:"provider"`
	Model    string           `json:"model"`
	Profile  string           `json:"profile"`
	Locality ProviderLocality `json:"locality"`
}

// Versions records independently versioned components that affected a report.
type Versions struct {
	CompanionVersion               string `json:"companion_version"`
	EngineVersion                  string `json:"engine_version"`
	JobmanVersion                  string `json:"jobman_version"`
	EvidenceSchemaVersion          int    `json:"evidence_schema_version"`
	ReportSchemaVersion            int    `json:"report_schema_version"`
	GenerationRequestSchemaVersion int    `json:"generation_request_schema_version"`
	ProposalSchemaVersion          int    `json:"proposal_schema_version"`
}

// Subject identifies the diagnosed Jobman state without copying job secrets.
type Subject struct {
	JobID        string   `json:"job_id"`
	JobRevision  uint64   `json:"job_revision"`
	SelectedRuns []uint64 `json:"selected_runs"`
	Phase        string   `json:"phase"`
	Outcome      string   `json:"outcome,omitempty"`
}

// AnalysisMode identifies which analyzer classes contributed.
type AnalysisMode string

// Supported analysis modes.
const (
	ModeDeterministic AnalysisMode = "deterministic"
	ModeGenerated     AnalysisMode = "generated"
	ModeMixed         AnalysisMode = "mixed"
)

// Finding is one ranked, controlled diagnosis or secondary observation.
type Finding struct {
	ID                    string     `json:"id"`
	Code                  string     `json:"code"`
	Category              string     `json:"category"`
	Severity              Severity   `json:"severity"`
	Summary               string     `json:"summary"`
	Explanation           string     `json:"explanation"`
	Confidence            Confidence `json:"confidence"`
	SupportingEvidence    []string   `json:"supporting_evidence"`
	ContradictingEvidence []string   `json:"contradicting_evidence"`
	ContradictingFindings []string   `json:"contradicting_findings"`
	Analyzer              string     `json:"analyzer"`
}

// Severity describes user impact, not certainty.
type Severity string

// Supported finding severities.
const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Confidence is an explicitly calibrated score and readable band.
type Confidence struct {
	Score int    `json:"score"`
	Band  string `json:"band"`
	Basis string `json:"basis"`
}

// Action is a non-executing remediation or inspection recommendation.
type Action struct {
	ID                   string          `json:"id"`
	Code                 string          `json:"code"`
	Kind                 ActionKind      `json:"kind"`
	Summary              string          `json:"summary"`
	Description          string          `json:"description"`
	SupportingEvidence   []string        `json:"supporting_evidence"`
	Execution            ActionExecution `json:"execution"`
	Arguments            []string        `json:"arguments"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	SafeToAutomate       bool            `json:"safe_to_automate"`
}

// ActionExecution identifies whether an action carries a bounded direct
// argument vector. The first release permits only no execution or allowlisted
// read-only Jobman inspection vectors.
type ActionExecution string

// Supported action execution classes.
const (
	ActionExecutionNone     ActionExecution = "none"
	ActionExecutionReadOnly ActionExecution = "read_only_argv"
)

// ActionKind groups recommendations by the authority they would require.
type ActionKind string

// Supported action kinds.
const (
	ActionInspect ActionKind = "inspect"
	ActionChange  ActionKind = "change"
	ActionWait    ActionKind = "wait"
	ActionRetry   ActionKind = "retry"
)

// RetryAdvice describes whether an unchanged immediate retry is sensible. It
// does not authorize Jobman to create a run.
type RetryAdvice struct {
	Verdict            RetryVerdict   `json:"verdict"`
	ExistingPolicy     ExistingPolicy `json:"existing_policy"`
	Confidence         Confidence     `json:"confidence"`
	Rationale          string         `json:"rationale"`
	Reasons            []string       `json:"reasons"`
	SupportingEvidence []string       `json:"supporting_evidence"`
	EarliestAt         *time.Time     `json:"earliest_at,omitempty"`
}

// ExistingPolicy summarizes what Jobman's immutable policy and current state
// say will happen without a new submission.
type ExistingPolicy string

// Supported existing-policy states.
const (
	PolicyScheduled           ExistingPolicy = "scheduled"
	PolicyBackoff             ExistingPolicy = "backoff"
	PolicyWaitingPrerequisite ExistingPolicy = "waiting_prerequisite"
	PolicyExhausted           ExistingPolicy = "exhausted"
	PolicyNonretryable        ExistingPolicy = "nonretryable"
	PolicyNone                ExistingPolicy = "none"
	PolicyUnknown             ExistingPolicy = "unknown"
)

// RetryVerdict is a controlled retry recommendation.
type RetryVerdict string

// Supported retry verdicts.
const (
	RetryNow           RetryVerdict = "now"
	RetryAfterDelay    RetryVerdict = "after_delay"
	RetryAfterChange   RetryVerdict = "after_change"
	RetryNo            RetryVerdict = "no"
	RetryNotApplicable RetryVerdict = "not_applicable"
	RetryUnknown       RetryVerdict = "unknown"
)

// Citation is a compact join table for evidence referenced by findings or
// actions. It never copies artifact content into the report.
type Citation struct {
	EvidenceID       string `json:"evidence_id"`
	Code             string `json:"code"`
	Summary          string `json:"summary"`
	Kind             string `json:"kind"`
	SourceEvidenceID string `json:"source_evidence_id,omitempty"`
	ByteStart        uint64 `json:"byte_start,omitempty"`
	ByteEnd          uint64 `json:"byte_end,omitempty"`
}

// MissingEvidence records a fact that would materially improve confidence.
type MissingEvidence struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Warning records a report limitation or security-relevant caveat.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DisclosureManifest records the exact evidence projection made available to
// an optional structured generator. An attempted invocation is conservative:
// the input may have reached the provider even when no generated content was
// accepted into the report.
type DisclosureManifest struct {
	ProviderInvoked      bool             `json:"provider_invoked"`
	GeneratedContentUsed bool             `json:"generated_content_used"`
	Locality             ProviderLocality `json:"locality"`
	Profile              string           `json:"profile,omitempty"`
	Provider             string           `json:"provider,omitempty"`
	Model                string           `json:"model,omitempty"`
	RequestID            string           `json:"request_id,omitempty"`
	Classes              []string         `json:"classes"`
	ItemIDs              []string         `json:"item_ids"`
	ArtifactIDs          []string         `json:"artifact_ids"`
	EnrichmentIDs        []string         `json:"enrichment_ids"`
	ItemCount            uint64           `json:"item_count"`
	ArtifactCount        uint64           `json:"artifact_count"`
	EnrichmentCount      uint64           `json:"enrichment_count"`
	ArtifactBytes        uint64           `json:"artifact_bytes"`
	EnrichmentBytes      uint64           `json:"enrichment_bytes"`
	RequestBytes         uint64           `json:"request_bytes"`
	RedactionNoticeCount uint64           `json:"redaction_notice_count"`
}

// Fingerprints contains grouping identities that deliberately exclude model
// prose. Core is an optional opaque store-local factual fingerprint; Report is
// the companion's stable diagnosis-grouping fingerprint.
type Fingerprints struct {
	Core   string `json:"core,omitempty"`
	Report string `json:"report"`
}

// ProviderLocality states whether a generator stays on the invoking host or
// may disclose its projection over a network.
type ProviderLocality string

// Supported generator localities.
const (
	ProviderNotUsed ProviderLocality = "not_used"
	ProviderLocal   ProviderLocality = "local"
	ProviderRemote  ProviderLocality = "remote"
)
