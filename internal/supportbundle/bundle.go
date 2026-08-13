// Package supportbundle creates deterministic, private diagnosis archives.
// It only packages already collected and validated evidence; it never reads a
// target file, Jobman database, provider credential, or process environment.
package supportbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

const (
	// Kind identifies the support-bundle manifest.
	Kind = "jobman.diagnosis_support_bundle"
	// SchemaVersion is the newest bundle schema emitted by this package.
	SchemaVersion = 2

	archiveRoot          = "jobman-diagnosis-support/"
	maximumPayloadBytes  = 8 * 1024 * 1024
	maximumArchiveFiles  = 16
	manifestPath         = "manifest.json"
	humanInventoryPath   = "INVENTORY.txt"
	deterministicUnixSec = 0
)

// Build describes the companion executable that created a bundle.
type Build struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

// File describes one archive member without exposing its content.
type File struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Disclosure  string `json:"disclosure"`
	Bytes       uint64 `json:"bytes"`
	SHA256      string `json:"sha256"`
}

// Inventory is both the dry-run result and the archive manifest. The manifest
// deliberately does not hash itself; ManifestPath makes that exception
// explicit while every other member is content-addressed.
type Inventory struct {
	Kind               string   `json:"kind"`
	SchemaVersion      int      `json:"schema_version"`
	ManifestPath       string   `json:"manifest_path"`
	CoreEvidenceID     string   `json:"core_evidence_id"`
	AnalysisEvidenceID string   `json:"analysis_evidence_id"`
	ReportID           string   `json:"report_id"`
	CreatedAt          string   `json:"created_at"`
	Files              []File   `json:"files"`
	TotalPayloadBytes  uint64   `json:"total_payload_bytes"`
	Warnings           []string `json:"warnings"`
}

// Bundle is an immutable in-memory archive plan. Use Encode to write it.
type Bundle struct {
	Inventory Inventory
	entries   []entry
}

type entry struct {
	path        string
	description string
	disclosure  string
	data        []byte
}

type analysisContextDocument struct {
	Kind               string                     `json:"kind"`
	SchemaVersion      int                        `json:"schema_version"`
	CoreEvidenceID     string                     `json:"core_evidence_id"`
	AnalysisEvidenceID string                     `json:"analysis_evidence_id"`
	Enrichment         []diagnosis.EnrichmentItem `json:"enrichment"`
	SourceContext      []diagnosis.SourceContext  `json:"source_context"`
}

type capabilityDocument struct {
	Kind                 string                `json:"kind"`
	SchemaVersion        int                   `json:"schema_version"`
	JobmanVersion        string                `json:"jobman_version"`
	EvidenceCapabilities []string              `json:"evidence_capabilities"`
	Omissions            []diagnostic.Omission `json:"omissions"`
}

type buildDocument struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"companion_version"`
	Commit        string `json:"commit,omitempty"`
	BuildDate     string `json:"build_date,omitempty"`
	GoVersion     string `json:"go_version"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
}

// New validates all inputs and constructs a deterministic support archive.
// The archive contains no data other than the supplied sealed values and
// derived version/capability metadata.
func New(evidence diagnosis.FailureEvidence, report diagnosis.Report, build Build) (Bundle, error) {
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err != nil {
		return Bundle{}, fmt.Errorf("build support bundle: %w", err)
	}
	if strings.TrimSpace(build.Version) == "" {
		return Bundle{}, errors.New("build support bundle: companion version is required")
	}

	entries, err := payloadEntries(evidence, report, build)
	if err != nil {
		return Bundle{}, err
	}
	paths := make([]string, 0, len(entries)+2)
	paths = append(paths, humanInventoryPath, manifestPath)
	for _, value := range entries {
		paths = append(paths, value.path)
	}
	slices.Sort(paths)
	humanInventory := []byte(renderInventory(paths))
	entries = append(entries, entry{
		path: humanInventoryPath, description: "Human-readable inventory and sharing cautions.",
		disclosure: "metadata", data: humanInventory,
	})
	slices.SortFunc(entries, func(left, right entry) int { return strings.Compare(left.path, right.path) })

	inventory := Inventory{
		Kind: Kind, SchemaVersion: SchemaVersion, ManifestPath: manifestPath,
		CoreEvidenceID: evidence.Core.EvidenceID, AnalysisEvidenceID: evidence.AnalysisEvidenceID,
		ReportID: report.ReportID, CreatedAt: report.GeneratedAt.UTC().Format(time.RFC3339Nano),
		Files: make([]File, 0, len(entries)), Warnings: []string{
			"Review evidence.json and analysis-context.json before sharing; explicitly collected commands, paths, log tails, or source text may contain sensitive data.",
			"Provider credentials, environment values, Jobman database files, and the fingerprint key are never collected by this bundle writer.",
		},
	}
	for _, value := range entries {
		digest := sha256.Sum256(value.data)
		inventory.Files = append(inventory.Files, File{
			Path: value.path, Description: value.description, Disclosure: value.disclosure,
			Bytes: uint64(len(value.data)), SHA256: hex.EncodeToString(digest[:]),
		})
		inventory.TotalPayloadBytes += uint64(len(value.data))
	}
	manifest, err := encodeJSON(inventory)
	if err != nil {
		return Bundle{}, fmt.Errorf("build support bundle manifest: %w", err)
	}
	entries = append(entries, entry{
		path: manifestPath, description: "Versioned bundle manifest and payload digests.",
		disclosure: "metadata", data: manifest,
	})
	slices.SortFunc(entries, func(left, right entry) int { return strings.Compare(left.path, right.path) })
	if len(entries) > maximumArchiveFiles || inventory.TotalPayloadBytes+uint64(len(manifest)) > maximumPayloadBytes {
		return Bundle{}, errors.New("build support bundle: archive exceeds its file or byte limit")
	}

	return Bundle{Inventory: inventory, entries: entries}, nil
}

// Encode writes a deterministic gzip-compressed tar archive. Archive members
// use private modes, stable ordering, and fixed ownership and timestamps.
func Encode(destination io.Writer, bundle Bundle) error {
	if destination == nil {
		return errors.New("encode support bundle: destination is nil")
	}
	if len(bundle.entries) == 0 || bundle.Inventory.Kind != Kind || bundle.Inventory.SchemaVersion != SchemaVersion {
		return errors.New("encode support bundle: invalid bundle")
	}

	compressed, err := gzip.NewWriterLevel(destination, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("encode support bundle: create compressor: %w", err)
	}
	compressed.Name = ""
	compressed.Comment = ""
	compressed.ModTime = time.Unix(deterministicUnixSec, 0).UTC()
	compressed.OS = 255
	tape := tar.NewWriter(compressed)
	for _, value := range bundle.entries {
		header := &tar.Header{
			Name: archiveRoot + value.path, Mode: 0o600, Size: int64(len(value.data)),
			ModTime: time.Unix(deterministicUnixSec, 0).UTC(), Format: tar.FormatPAX,
		}
		if err := tape.WriteHeader(header); err != nil {
			return closeArchive(tape, compressed, fmt.Errorf("encode support bundle header: %w", err))
		}
		if _, err := tape.Write(value.data); err != nil {
			return closeArchive(tape, compressed, fmt.Errorf("encode support bundle payload: %w", err))
		}
	}
	if err := tape.Close(); err != nil {
		return closeArchive(nil, compressed, fmt.Errorf("encode support bundle tar: %w", err))
	}
	if err := compressed.Close(); err != nil {
		return fmt.Errorf("encode support bundle compression: %w", err)
	}

	return nil
}

// WriteInventory writes the dry-run inventory in a stable human-readable form.
func WriteInventory(destination io.Writer, inventory Inventory) error {
	if destination == nil {
		return errors.New("write support bundle inventory: destination is nil")
	}
	if inventory.Kind != Kind || inventory.SchemaVersion != SchemaVersion {
		return errors.New("write support bundle inventory: invalid inventory")
	}
	if _, err := fmt.Fprintf(
		destination,
		"Support bundle dry run (no archive created)\nReport: %s\nCore evidence: %s\nAnalysis evidence: %s\n\nFILES\n",
		inventory.ReportID, inventory.CoreEvidenceID, inventory.AnalysisEvidenceID,
	); err != nil {
		return fmt.Errorf("write support bundle inventory: %w", err)
	}
	if _, err := fmt.Fprintf(destination, "- %s — Versioned bundle manifest.\n", manifestPath); err != nil {
		return fmt.Errorf("write support bundle inventory: %w", err)
	}
	for _, value := range inventory.Files {
		if _, err := fmt.Fprintf(
			destination, "- %s (%d bytes, %s) — %s\n", value.Path, value.Bytes, value.Disclosure, value.Description,
		); err != nil {
			return fmt.Errorf("write support bundle inventory: %w", err)
		}
	}
	if _, err := fmt.Fprintln(destination, "\nCAUTIONS"); err != nil {
		return fmt.Errorf("write support bundle inventory: %w", err)
	}
	for _, warning := range inventory.Warnings {
		if _, err := fmt.Fprintf(destination, "- %s\n", warning); err != nil {
			return fmt.Errorf("write support bundle inventory: %w", err)
		}
	}

	return nil
}

// EncodeInventory writes the versioned dry-run inventory as JSON.
func EncodeInventory(destination io.Writer, inventory Inventory) error {
	if destination == nil {
		return errors.New("encode support bundle inventory: destination is nil")
	}
	if inventory.Kind != Kind || inventory.SchemaVersion != SchemaVersion {
		return errors.New("encode support bundle inventory: invalid inventory")
	}
	encoder := json.NewEncoder(destination)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(inventory); err != nil {
		return fmt.Errorf("encode support bundle inventory: %w", err)
	}

	return nil
}

func payloadEntries(evidence diagnosis.FailureEvidence, report diagnosis.Report, build Build) ([]entry, error) {
	var core bytes.Buffer
	if err := diagnostic.Encode(&core, evidence.Core); err != nil {
		return nil, fmt.Errorf("build support bundle evidence: %w", err)
	}
	var diagnosed bytes.Buffer
	if err := diagnosis.Encode(&diagnosed, report); err != nil {
		return nil, fmt.Errorf("build support bundle report: %w", err)
	}
	analysisContext, err := encodeJSON(analysisContextDocument{
		Kind: "jobman.diagnosis_analysis_context", SchemaVersion: diagnosis.FailureEvidenceSchemaVersion,
		CoreEvidenceID: evidence.Core.EvidenceID, AnalysisEvidenceID: evidence.AnalysisEvidenceID,
		Enrichment: evidence.Enrichment, SourceContext: evidence.SourceContext,
	})
	if err != nil {
		return nil, fmt.Errorf("build support bundle analysis context: %w", err)
	}
	disclosure, err := encodeJSON(report.Disclosure)
	if err != nil {
		return nil, fmt.Errorf("build support bundle disclosure: %w", err)
	}
	capabilities := slices.Clone(evidence.Core.Source.Capabilities)
	slices.Sort(capabilities)
	capabilityBytes, err := encodeJSON(capabilityDocument{
		Kind: "jobman.diagnosis_capabilities", SchemaVersion: 1,
		JobmanVersion: evidence.Core.Source.JobmanVersion, EvidenceCapabilities: capabilities,
		Omissions: evidence.Core.Omissions,
	})
	if err != nil {
		return nil, fmt.Errorf("build support bundle capabilities: %w", err)
	}
	buildBytes, err := encodeJSON(buildDocument{
		Kind: "jobman.diagnosis_build", SchemaVersion: 1, Version: build.Version,
		Commit: build.Commit, BuildDate: build.BuildDate, GoVersion: runtime.Version(),
		OS: runtime.GOOS, Architecture: runtime.GOARCH,
	})
	if err != nil {
		return nil, fmt.Errorf("build support bundle version: %w", err)
	}

	return []entry{
		{path: "analysis-context.json", description: "Attributed enrichment and explicitly selected point-in-time source context.", disclosure: "selected_analysis_context", data: analysisContext},
		{path: "build.json", description: "Companion build and current platform facts.", disclosure: "metadata", data: buildBytes},
		{path: "capabilities.json", description: "Core capability and omission facts.", disclosure: "metadata", data: capabilityBytes},
		{path: "diagnosis.json", description: "Validated diagnosis report.", disclosure: "report", data: diagnosed.Bytes()},
		{path: "disclosure.json", description: "Exact optional-provider disclosure manifest.", disclosure: "metadata", data: disclosure},
		{path: "evidence.json", description: "Sealed, sanitized core evidence selected for diagnosis.", disclosure: "selected_evidence", data: core.Bytes()},
	}, nil
}

func encodeJSON(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(encoded, '\n'), nil
}

func renderInventory(paths []string) string {
	var result strings.Builder
	result.WriteString("Jobman diagnosis support bundle\n\nIncluded files:\n")
	for _, path := range paths {
		fmt.Fprintf(&result, "- %s\n", path)
	}
	result.WriteString("\nReview evidence.json and analysis-context.json before sharing. Explicitly collected command, path, log, and source data may be sensitive.\n")

	return result.String()
}

func closeArchive(tape *tar.Writer, compressed *gzip.Writer, cause error) error {
	var tarErr error
	if tape != nil {
		tarErr = tape.Close()
	}
	return errors.Join(cause, tarErr, compressed.Close())
}
