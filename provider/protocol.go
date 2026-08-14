package provider

import (
	"encoding/json"
	"time"
)

const (
	// RequestKind identifies the structured-generation request protocol.
	RequestKind = "jobman.diagnosis_generation_request"
	// RequestSchemaVersion is the newest request schema understood here.
	RequestSchemaVersion = 5
	// ProposalKind identifies generated proposal documents.
	ProposalKind = "jobman.diagnosis_proposal"
	// ProposalSchemaVersion is the newest proposal schema understood here.
	ProposalSchemaVersion = 2
)

// Subject is the minimum operational context disclosed to a generator.
type Subject struct {
	Phase        string   `json:"phase"`
	Outcome      string   `json:"outcome,omitempty"`
	SelectedRuns []uint64 `json:"selected_runs"`
}

// Projection contains only evidence classes approved by both profile and CLI.
type Projection struct {
	Items            []ProjectedItem       `json:"items"`
	Enrichment       []ProjectedEnrichment `json:"enrichment"`
	RedactionNotices []ProjectedRedaction  `json:"redaction_notices"`
	// Artifacts intentionally encode last. The trusted instructions follow the
	// projection in Request, keeping bounded target output close to the task
	// while still ending the user message with host-authored guidance.
	Artifacts []ProjectedArtifact `json:"artifacts"`
}

// ProjectedItem is one typed core value with transport-irrelevant source data removed.
type ProjectedItem struct {
	ID         string          `json:"id"`
	Code       string          `json:"code"`
	Value      json.RawMessage `json:"value"`
	ObservedAt *time.Time      `json:"observed_at,omitempty"`
	Quality    string          `json:"quality"`
	Disclosure string          `json:"disclosure"`
}

// ProjectedArtifact is either a bounded, sanitized causal log context or an
// explicitly approved point-in-time source snapshot represented as UTF-8.
// Log selections retain source and selected-content provenance. All artifact
// content is untrusted data and never becomes instructions.
type ProjectedArtifact struct {
	ID            string     `json:"id"`
	Role          string     `json:"role"`
	Run           uint64     `json:"run"`
	Stream        string     `json:"stream,omitempty"`
	Path          string     `json:"path,omitempty"`
	Language      string     `json:"language,omitempty"`
	MediaType     string     `json:"media_type,omitempty"`
	Selection     string     `json:"selection,omitempty"`
	AnchorLine    uint64     `json:"anchor_line,omitempty"`
	AnchorReason  string     `json:"anchor_reason,omitempty"`
	StartLine     uint64     `json:"start_line,omitempty"`
	EndLine       uint64     `json:"end_line,omitempty"`
	TotalLines    uint64     `json:"total_lines,omitempty"`
	ByteStart     uint64     `json:"byte_start,omitempty"`
	ByteEnd       uint64     `json:"byte_end,omitempty"`
	FileBytes     uint64     `json:"file_bytes,omitempty"`
	Content       string     `json:"content"`
	Encoding      string     `json:"encoding"`
	Digest        string     `json:"digest"`
	ContentDigest string     `json:"content_digest,omitempty"`
	CapturedAt    *time.Time `json:"captured_at,omitempty"`
	Quality       string     `json:"quality,omitempty"`
	Truncated     bool       `json:"truncated"`
	SelectedBytes uint64     `json:"selected_bytes"`
	ContentBytes  uint64     `json:"content_bytes"`
	Disclosure    string     `json:"disclosure"`
}

// ProjectedEnrichment exposes deterministic structure derived from an
// explicitly disclosed artifact. It never adds content or expands the source
// byte range approved by the caller.
type ProjectedEnrichment struct {
	ID               string   `json:"id"`
	Code             string   `json:"code"`
	Format           string   `json:"format"`
	SourceArtifactID string   `json:"source_artifact_id"`
	ByteStart        uint64   `json:"byte_start"`
	ByteEnd          uint64   `json:"byte_end"`
	Collector        string   `json:"collector"`
	CollectorVersion string   `json:"collector_version"`
	Quality          string   `json:"quality"`
	Disclosure       string   `json:"disclosure"`
	DiagnosticLines  []string `json:"diagnostic_lines"`
}

// ProjectedRedaction states which projected identifiers were changed by core
// sanitization without exposing a pattern or original value.
type ProjectedRedaction struct {
	Code    string   `json:"code"`
	Affects []string `json:"affects"`
	Count   uint64   `json:"count"`
}

// ProjectionManifest accounts for the exact evidence identifiers and bytes
// selected before provider-specific encoding.
type ProjectionManifest struct {
	Classes              []string `json:"classes"`
	ItemIDs              []string `json:"item_ids"`
	ArtifactIDs          []string `json:"artifact_ids"`
	EnrichmentIDs        []string `json:"enrichment_ids"`
	ItemCount            uint64   `json:"item_count"`
	ArtifactCount        uint64   `json:"artifact_count"`
	EnrichmentCount      uint64   `json:"enrichment_count"`
	ArtifactBytes        uint64   `json:"artifact_bytes"`
	EnrichmentBytes      uint64   `json:"enrichment_bytes"`
	RedactionNoticeCount uint64   `json:"redaction_notice_count"`
}

// DeterministicCandidate is a controlled local finding supplied as context.
type DeterministicCandidate struct {
	ID                    string   `json:"id"`
	Code                  string   `json:"code"`
	Category              string   `json:"category"`
	Summary               string   `json:"summary"`
	Explanation           string   `json:"explanation"`
	SupportingEvidence    []string `json:"supporting_evidence"`
	ContradictingEvidence []string `json:"contradicting_evidence"`
}

// AllowedAction is a non-executing catalog entry. A proposal can reference its
// ID but cannot create an argument vector or new operation.
type AllowedAction struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

// Proposal contains untrusted hypotheses and references. It intentionally has
// no retry verdict, lifecycle facts, tools, commands, URLs, or arguments.
type Proposal struct {
	Kind               string            `json:"kind"`
	SchemaVersion      int               `json:"schema_version"`
	RequestID          string            `json:"request_id"`
	Hypotheses         []Hypothesis      `json:"hypotheses"`
	RecommendedActions []string          `json:"recommended_action_ids"`
	MissingEvidence    []MissingEvidence `json:"missing_evidence"`
}

// Hypothesis is one uncalibrated generated alternative.
type Hypothesis struct {
	Code                  string   `json:"code"`
	Category              string   `json:"category"`
	Summary               string   `json:"summary"`
	RootCause             string   `json:"root_cause"`
	Explanation           string   `json:"explanation"`
	SupportingEvidence    []string `json:"supporting_evidence"`
	ContradictingEvidence []string `json:"contradicting_evidence"`
	ContradictsFindings   []string `json:"contradicts_findings"`
}

// MissingEvidence describes additional factual input that could distinguish
// hypotheses. It remains advisory and cannot request a tool invocation.
type MissingEvidence struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}
