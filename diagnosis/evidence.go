package diagnosis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/internal/portablepath"
)

const (
	// FailureEvidenceKind identifies the companion-owned wrapper around sealed
	// core evidence and deterministic, attributed enrichment.
	FailureEvidenceKind = "jobman.failure_evidence"
	// FailureEvidenceSchemaVersion is the newest wrapper schema supported by
	// this package.
	FailureEvidenceSchemaVersion = 2

	// DisclosureSourceContent identifies source text collected by the companion
	// only after a separate command-line opt in. It is deliberately not a core
	// Jobman disclosure class because Jobman neither reads nor attests to the
	// current contents of a target source file.
	DisclosureSourceContent diagnostic.DisclosureClass = "source_content"

	maximumEnrichmentItems = 128
	maximumCollectorText   = 256
	maximumEnrichmentBytes = 512 * 1024
	maximumSourceContexts  = 8
	maximumSourceBytes     = 1024 * 1024
)

// FailureEvidence retains immutable Jobman evidence, separately attributed
// companion enrichment, and explicitly selected point-in-time source context.
// AnalysisEvidenceID commits to every field in that companion-owned context.
type FailureEvidence struct {
	Kind               string              `json:"kind"`
	SchemaVersion      int                 `json:"schema_version"`
	AnalysisEvidenceID string              `json:"analysis_evidence_id"`
	Core               diagnostic.Evidence `json:"core"`
	Enrichment         []EnrichmentItem    `json:"enrichment"`
	SourceContext      []SourceContext     `json:"source_context"`
}

// AnalyzerDescriptor identifies one deterministic collector or analyzer.
type AnalyzerDescriptor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// EnrichmentItem is one bounded deterministic observation derived from an
// already selected core artifact. ByteStart and ByteEnd are offsets into the
// sanitized artifact Data value, so every derived observation is auditable
// without reading another file or expanding the core collection budget.
type EnrichmentItem struct {
	ID               string                     `json:"id"`
	Code             string                     `json:"code"`
	Format           string                     `json:"format"`
	SourceArtifactID string                     `json:"source_artifact_id"`
	ByteStart        uint64                     `json:"byte_start"`
	ByteEnd          uint64                     `json:"byte_end"`
	ObservedAt       time.Time                  `json:"observed_at"`
	Collector        AnalyzerDescriptor         `json:"collector"`
	Quality          diagnostic.Quality         `json:"quality"`
	Disclosure       diagnostic.DisclosureClass `json:"disclosure"`
}

// SourceContextMode controls how much of one explicitly approved source file
// is retained in analysis evidence.
type SourceContextMode string

// Supported source-context selection modes.
const (
	SourceContextLimited SourceContextMode = "limited"
	SourceContextFull    SourceContextMode = "full"
)

// SourceContext is a companion-collected, point-in-time snapshot of current
// source text. It is supplemental diagnostic context, not proof of the bytes
// executed by the recorded Jobman run. ByteStart and ByteEnd address the
// complete current file; StartLine and EndLine describe the selected text.
type SourceContext struct {
	ID            string                     `json:"id"`
	Role          string                     `json:"role"`
	Path          string                     `json:"path"`
	Language      string                     `json:"language"`
	MediaType     string                     `json:"media_type"`
	Mode          SourceContextMode          `json:"mode"`
	AnchorLine    uint64                     `json:"anchor_line,omitempty"`
	AnchorReason  string                     `json:"anchor_reason"`
	StartLine     uint64                     `json:"start_line"`
	EndLine       uint64                     `json:"end_line"`
	TotalLines    uint64                     `json:"total_lines"`
	ByteStart     uint64                     `json:"byte_start"`
	ByteEnd       uint64                     `json:"byte_end"`
	FileBytes     uint64                     `json:"file_bytes"`
	ContentBytes  uint64                     `json:"content_bytes"`
	Data          []byte                     `json:"data"`
	Digest        string                     `json:"digest"`
	ContentDigest string                     `json:"content_digest"`
	CapturedAt    time.Time                  `json:"captured_at"`
	Collector     AnalyzerDescriptor         `json:"collector"`
	Quality       diagnostic.Quality         `json:"quality"`
	Disclosure    diagnostic.DisclosureClass `json:"disclosure"`
}

// CoreFailureEvidence constructs a verified wrapper without companion
// enrichment. Embedders can use it when they deliberately want core-only
// analysis.
func CoreFailureEvidence(core diagnostic.Evidence) (FailureEvidence, error) {
	return SealFailureEvidence(core, nil)
}

// SealFailureEvidence verifies core evidence, normalizes enrichment, and
// computes the companion analysis-evidence identity.
func SealFailureEvidence(core diagnostic.Evidence, enrichment []EnrichmentItem) (FailureEvidence, error) {
	return SealFailureEvidenceWithContext(core, enrichment, nil)
}

// SealFailureEvidenceWithContext verifies core evidence, normalizes companion
// enrichment and source snapshots, and computes their joint analysis identity.
func SealFailureEvidenceWithContext(
	core diagnostic.Evidence,
	enrichment []EnrichmentItem,
	sourceContext []SourceContext,
) (FailureEvidence, error) {
	if err := diagnostic.Verify(core); err != nil {
		return FailureEvidence{}, fmt.Errorf("seal failure evidence: verify core evidence: %w", err)
	}
	value := FailureEvidence{
		Kind: FailureEvidenceKind, SchemaVersion: FailureEvidenceSchemaVersion,
		AnalysisEvidenceID: "sha256:" + strings.Repeat("0", sha256.Size*2),
		Core:               core, Enrichment: slices.Clone(enrichment), SourceContext: slices.Clone(sourceContext),
	}
	slices.SortFunc(value.Enrichment, func(left, right EnrichmentItem) int {
		return strings.Compare(left.ID, right.ID)
	})
	if value.Enrichment == nil {
		value.Enrichment = []EnrichmentItem{}
	}
	slices.SortFunc(value.SourceContext, func(left, right SourceContext) int {
		return strings.Compare(left.ID, right.ID)
	})
	if value.SourceContext == nil {
		value.SourceContext = []SourceContext{}
	}
	if err := validateFailureEvidence(value, true); err != nil {
		return FailureEvidence{}, err
	}
	digest, err := failureEvidenceDigest(value)
	if err != nil {
		return FailureEvidence{}, err
	}
	value.AnalysisEvidenceID = digest
	if err := VerifyFailureEvidence(value); err != nil {
		return FailureEvidence{}, err
	}

	return value, nil
}

// VerifyFailureEvidence validates the wrapper, its source ranges, and its
// semantic identity.
func VerifyFailureEvidence(value FailureEvidence) error {
	if err := validateFailureEvidence(value, false); err != nil {
		return err
	}
	digest, err := failureEvidenceDigest(value)
	if err != nil {
		return err
	}
	if value.AnalysisEvidenceID != digest {
		return errors.New("verify failure evidence: analysis evidence ID does not match semantic content")
	}

	return nil
}

// EncodeFailureEvidence writes one verified wrapper followed by a newline.
func EncodeFailureEvidence(destination io.Writer, value FailureEvidence) error {
	if destination == nil {
		return errors.New("encode failure evidence: destination is nil")
	}
	if err := VerifyFailureEvidence(value); err != nil {
		return err
	}
	encoder := json.NewEncoder(destination)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode failure evidence: %w", err)
	}

	return nil
}

//nolint:cyclop,gocognit // Wrapper validation cross-checks identity, source ranges, collector provenance, and bounds.
func validateFailureEvidence(value FailureEvidence, placeholder bool) error {
	if value.Kind != FailureEvidenceKind || value.SchemaVersion != FailureEvidenceSchemaVersion {
		return errors.New("validate failure evidence: unsupported kind or schema version")
	}
	placeholderID := "sha256:" + strings.Repeat("0", sha256.Size*2)
	if placeholder && value.AnalysisEvidenceID != placeholderID ||
		!placeholder && !validDigest(value.AnalysisEvidenceID) {
		return errors.New("validate failure evidence: invalid analysis evidence ID")
	}
	if err := diagnostic.Verify(value.Core); err != nil {
		return fmt.Errorf("validate failure evidence: verify core evidence: %w", err)
	}
	if value.Enrichment == nil || len(value.Enrichment) > maximumEnrichmentItems {
		return errors.New("validate failure evidence: invalid enrichment collection")
	}
	artifacts := make(map[string]diagnostic.Artifact, len(value.Core.Artifacts))
	for _, artifact := range value.Core.Artifacts {
		artifacts[artifact.ID] = artifact
	}
	prior := ""
	for _, item := range value.Enrichment {
		if item.ID <= prior || !validEnrichmentItem(item, artifacts) {
			return fmt.Errorf("validate failure evidence: invalid enrichment item %q", item.ID)
		}
		prior = item.ID
	}
	encoded, err := json.Marshal(value.Enrichment)
	if err != nil || len(encoded) > maximumEnrichmentBytes {
		return errors.New("validate failure evidence: enrichment exceeds its encoded limit")
	}
	if value.SourceContext == nil || len(value.SourceContext) > maximumSourceContexts {
		return errors.New("validate failure evidence: invalid source context collection")
	}
	prior = ""
	var sourceBytes uint64
	for _, source := range value.SourceContext {
		if source.ID <= prior || !validSourceContext(source) {
			return fmt.Errorf("validate failure evidence: invalid source context %q", source.ID)
		}
		if sourceBytes > maximumSourceBytes-source.ContentBytes {
			return errors.New("validate failure evidence: source context exceeds its content limit")
		}
		sourceBytes += source.ContentBytes
		prior = source.ID
	}

	return nil
}

func validSourceContext(source SourceContext) bool {
	return validSourceIdentity(source) && validSourceBounds(source) && validSourcePayload(source) &&
		validSourceSelection(source)
}

func validSourceIdentity(source SourceContext) bool {
	return strings.HasPrefix(source.ID, "context:source:") &&
		validIdentifierText(source.ID, maximumCollectorText) && source.Role == "source.context" &&
		portablepath.IsCleanAbsolute(source.Path) &&
		validIdentifierText(source.Language, maximumCollectorText) &&
		validIdentifierText(source.MediaType, maximumCollectorText) &&
		validIdentifierText(source.AnchorReason, maximumCollectorText) &&
		!source.CapturedAt.IsZero() && source.CapturedAt.Location() == time.UTC &&
		validDescriptor(source.Collector) && source.Quality == diagnostic.QualityPointInTime &&
		source.Disclosure == DisclosureSourceContent
}

func validSourceBounds(source SourceContext) bool {
	return source.StartLine != 0 && source.EndLine >= source.StartLine && source.TotalLines >= source.EndLine &&
		source.ByteStart < source.ByteEnd && source.ByteEnd <= source.FileBytes &&
		source.ContentBytes == uint64(len(source.Data)) && source.ContentBytes == source.ByteEnd-source.ByteStart &&
		source.FileBytes != 0 && source.FileBytes <= maximumSourceBytes && source.ContentBytes <= maximumSourceBytes
}

func validSourcePayload(source SourceContext) bool {
	return utf8.Valid(source.Data) && !strings.ContainsRune(string(source.Data), '\x00') &&
		validDigest(source.Digest) && validDigest(source.ContentDigest) &&
		contentDigest(source.Data) == source.ContentDigest
}

func validSourceSelection(source SourceContext) bool {
	switch source.Mode {
	case SourceContextLimited:
		return source.AnchorLine >= source.StartLine && source.AnchorLine <= source.EndLine &&
			(source.AnchorReason == "explicit_line" || source.AnchorReason == "runtime_log" ||
				source.AnchorReason == "file_start")
	case SourceContextFull:
		return source.AnchorLine == 0 && source.AnchorReason == "full_file" && source.StartLine == 1 &&
			source.EndLine == source.TotalLines && source.ByteStart == 0 && source.ByteEnd == source.FileBytes &&
			source.ContentBytes == source.FileBytes && source.ContentDigest == source.Digest
	default:
		return false
	}
}

func contentDigest(data []byte) string {
	digest := sha256.Sum256(data)

	return "sha256:" + hex.EncodeToString(digest[:])
}

func validEnrichmentItem(item EnrichmentItem, artifacts map[string]diagnostic.Artifact) bool {
	artifact, ok := artifacts[item.SourceArtifactID]
	return ok && strings.HasPrefix(item.ID, "analysis:") && validIdentifierText(item.ID, maximumCollectorText) &&
		strings.HasPrefix(item.Code, "enrichment.") && validIdentifierText(item.Code, maximumCollectorText) &&
		validIdentifierText(item.Format, maximumCollectorText) && item.ByteStart < item.ByteEnd &&
		item.ByteEnd <= uint64(len(artifact.Data)) && !item.ObservedAt.IsZero() &&
		item.ObservedAt.Location() == time.UTC && validDescriptor(item.Collector) &&
		item.Quality == diagnostic.QualityDerivedExact && item.Disclosure == diagnostic.DisclosureLocalOnly
}

func validDescriptor(descriptor AnalyzerDescriptor) bool {
	return validIdentifierText(descriptor.Name, maximumCollectorText) &&
		validIdentifierText(descriptor.Version, maximumCollectorText)
}

func validIdentifierText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) &&
		!strings.ContainsRune(value, '\x00')
}

func failureEvidenceDigest(value FailureEvidence) (string, error) {
	projection := struct {
		Kind           string           `json:"kind"`
		SchemaVersion  int              `json:"schema_version"`
		CoreEvidenceID string           `json:"core_evidence_id"`
		Enrichment     []EnrichmentItem `json:"enrichment"`
		SourceContext  []SourceContext  `json:"source_context"`
	}{
		Kind: value.Kind, SchemaVersion: value.SchemaVersion,
		CoreEvidenceID: value.Core.EvidenceID, Enrichment: value.Enrichment,
		SourceContext: value.SourceContext,
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("hash failure evidence: %w", err)
	}
	digest := sha256.Sum256(encoded)

	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
