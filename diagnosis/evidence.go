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
)

const (
	// FailureEvidenceKind identifies the companion-owned wrapper around sealed
	// core evidence and deterministic, attributed enrichment.
	FailureEvidenceKind = "jobman.failure_evidence"
	// FailureEvidenceSchemaVersion is the newest wrapper schema supported by
	// this package.
	FailureEvidenceSchemaVersion = 1

	maximumEnrichmentItems = 128
	maximumCollectorText   = 256
	maximumEnrichmentBytes = 512 * 1024
)

// FailureEvidence retains immutable Jobman evidence and separately attributed
// companion enrichment. AnalysisEvidenceID commits to the core evidence ID and
// all enrichment semantics, but not to a collection wall clock.
type FailureEvidence struct {
	Kind               string              `json:"kind"`
	SchemaVersion      int                 `json:"schema_version"`
	AnalysisEvidenceID string              `json:"analysis_evidence_id"`
	Core               diagnostic.Evidence `json:"core"`
	Enrichment         []EnrichmentItem    `json:"enrichment"`
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

// CoreFailureEvidence constructs a verified wrapper without companion
// enrichment. Embedders can use it when they deliberately want core-only
// analysis.
func CoreFailureEvidence(core diagnostic.Evidence) (FailureEvidence, error) {
	return SealFailureEvidence(core, nil)
}

// SealFailureEvidence verifies core evidence, normalizes enrichment, and
// computes the companion analysis-evidence identity.
func SealFailureEvidence(core diagnostic.Evidence, enrichment []EnrichmentItem) (FailureEvidence, error) {
	if err := diagnostic.Verify(core); err != nil {
		return FailureEvidence{}, fmt.Errorf("seal failure evidence: verify core evidence: %w", err)
	}
	value := FailureEvidence{
		Kind: FailureEvidenceKind, SchemaVersion: FailureEvidenceSchemaVersion,
		AnalysisEvidenceID: "sha256:" + strings.Repeat("0", sha256.Size*2),
		Core:               core, Enrichment: slices.Clone(enrichment),
	}
	slices.SortFunc(value.Enrichment, func(left, right EnrichmentItem) int {
		return strings.Compare(left.ID, right.ID)
	})
	if value.Enrichment == nil {
		value.Enrichment = []EnrichmentItem{}
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

//nolint:cyclop // Wrapper validation cross-checks identity, source ranges, collector provenance, and bounds.
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

	return nil
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
	}{
		Kind: value.Kind, SchemaVersion: value.SchemaVersion,
		CoreEvidenceID: value.Core.EvidenceID, Enrichment: value.Enrichment,
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("hash failure evidence: %w", err)
	}
	digest := sha256.Sum256(encoded)

	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
