// Package generation builds explicit disclosure projections and reconciles
// untrusted generated proposals with deterministic diagnosis reports.
package generation

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

var allowedCategories = []string{
	"application", "history", "launch", "lifecycle", "logging", "notification", "ownership",
	"network", "policy", "prerequisite", "process", "resource", "state", "storage",
}

var allowedHypothesisCodes = []string{
	"generated.access_denied",
	"generated.application_configuration",
	"generated.application_defect",
	"generated.application_input",
	"generated.data_validation",
	"generated.dependency_missing",
	"generated.dependency_unavailable",
	"generated.environment_mismatch",
	"generated.external_service_failure",
	"generated.resource_pressure",
	"generated.transient_infrastructure",
	"generated.unknown_target_error",
}

// Prepared contains a sealed provider request and the exact disclosure facts
// needed for the final report.
type Prepared struct {
	Request      provider.Request
	ProfileName  string
	Provider     string
	Model        string
	Locality     provider.Locality
	RequestBytes uint64
}

// Prepare verifies the deterministic report, projects the approved classes,
// and seals a generation request. It performs no provider operation.
func Prepare(
	evidence diagnosis.FailureEvidence,
	report diagnosis.Report,
	profileName string,
	profile config.Profile,
	approvedClasses []string,
) (Prepared, error) {
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err != nil {
		return Prepared{}, fmt.Errorf("prepare generated diagnosis: %w", err)
	}
	if !slices.Contains(approvedClasses, string(diagnostic.DisclosureMetadata)) {
		return Prepared{}, errors.New("prepare generated diagnosis: metadata disclosure approval is required")
	}
	projection, manifest, err := projectEvidence(evidence, profile, approvedClasses)
	if err != nil {
		return Prepared{}, err
	}
	deterministic := projectDeterministic(report, manifest)
	if len(deterministic) == 0 {
		return Prepared{}, errors.New("prepare generated diagnosis: no deterministic candidate is fully supported by the projection")
	}
	actions := make([]provider.AllowedAction, 0, len(report.Actions))
	for _, action := range report.Actions {
		actions = append(actions, provider.AllowedAction{
			ID: action.ID, Code: action.Code, Summary: action.Summary, Description: action.Description,
		})
	}
	request, err := provider.SealRequest(provider.Request{
		AnalysisEvidenceID: evidence.AnalysisEvidenceID,
		Subject: provider.Subject{
			Phase: evidence.Core.Subject.Phase, Outcome: evidence.Core.Subject.Outcome,
			SelectedRuns: slices.Clone(evidence.Core.Subject.SelectedRuns),
		},
		Projection: projection, Manifest: manifest, Deterministic: deterministic,
		AllowedCategories:      slices.Clone(allowedCategories),
		AllowedHypothesisCodes: relevantHypothesisCodes(projection), AllowedActions: actions,
		Instructions: provider.RequiredInstructions(), MaximumOutputBytes: profile.MaximumOutputBytes,
	})
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare generated diagnosis: seal request: %w", err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare generated diagnosis: encode request: %w", err)
	}
	if len(encoded) > profile.MaximumInputBytes {
		return Prepared{}, fmt.Errorf(
			"prepare generated diagnosis: request requires %d bytes but profile allows %d",
			len(encoded), profile.MaximumInputBytes,
		)
	}

	return Prepared{
		Request: request, ProfileName: profileName, Provider: profile.Provider, Model: profile.Model,
		Locality: profile.Locality, RequestBytes: uint64(len(encoded)),
	}, nil
}

func relevantHypothesisCodes(projection provider.Projection) []string {
	result := make([]string, 0, len(allowedHypothesisCodes))
	for _, code := range allowedHypothesisCodes {
		if provider.RequiresDirectCauseSignal(code) && !provider.DirectCauseSignalSupported(code, projection) {
			continue
		}
		result = append(result, code)
	}
	if len(result) == 0 {
		// Request/schema validation requires a nonempty authority enum even when
		// the specialized hypothesis array is constrained to empty.
		return []string{"generated.unknown_target_error"}
	}

	return result
}

// Disclosure builds a report manifest after a provider invocation was
// attempted. The request projection is conservatively treated as disclosed.
func (prepared Prepared) Disclosure(generatedContentUsed bool) diagnosis.DisclosureManifest {
	locality := diagnosis.ProviderRemote
	if prepared.Locality == provider.LocalityLocal {
		locality = diagnosis.ProviderLocal
	}
	manifest := prepared.Request.Manifest

	return diagnosis.DisclosureManifest{
		ProviderInvoked: true, GeneratedContentUsed: generatedContentUsed, Locality: locality,
		Profile: prepared.ProfileName, Provider: prepared.Provider, Model: prepared.Model,
		RequestID: prepared.Request.RequestID, Classes: slices.Clone(manifest.Classes),
		ItemIDs: slices.Clone(manifest.ItemIDs), ArtifactIDs: slices.Clone(manifest.ArtifactIDs),
		EnrichmentIDs: slices.Clone(manifest.EnrichmentIDs),
		ItemCount:     manifest.ItemCount, ArtifactCount: manifest.ArtifactCount,
		EnrichmentCount: manifest.EnrichmentCount,
		ArtifactBytes:   manifest.ArtifactBytes, EnrichmentBytes: manifest.EnrichmentBytes,
		RequestBytes:         prepared.RequestBytes,
		RedactionNoticeCount: manifest.RedactionNoticeCount,
	}
}

//nolint:cyclop,gocognit // Projection jointly enforces class selection, redaction capability, and exact byte accounting.
func projectEvidence(
	failureEvidence diagnosis.FailureEvidence,
	profile config.Profile,
	approved []string,
) (provider.Projection, provider.ProjectionManifest, error) {
	evidence := failureEvidence.Core
	projection := provider.Projection{
		Items: []provider.ProjectedItem{}, Artifacts: []provider.ProjectedArtifact{},
		Enrichment:       []provider.ProjectedEnrichment{},
		RedactionNotices: []provider.ProjectedRedaction{},
	}
	manifest := provider.ProjectionManifest{
		Classes: []string{}, ItemIDs: []string{}, ArtifactIDs: []string{}, EnrichmentIDs: []string{},
	}
	for _, class := range []diagnostic.DisclosureClass{
		diagnostic.DisclosureMetadata, diagnostic.DisclosureCommand, diagnostic.DisclosurePath,
		diagnostic.DisclosureEnvironmentName,
	} {
		if !slices.Contains(approved, string(class)) {
			continue
		}
		limits, allowed := profile.Disclosure[string(class)]
		if !allowed {
			continue
		}
		var itemCount, classBytes uint64
		for _, item := range evidence.Items {
			if item.Disclosure != class || !usefulGenerationItem(item.Code) {
				continue
			}
			observedAt := item.ObservedAt
			if observedAt != nil {
				value := observedAt.UTC().Round(0)
				observedAt = &value
			}
			projected := provider.ProjectedItem{
				ID: item.ID, Code: item.Code, Value: slices.Clone(item.Value), ObservedAt: observedAt,
				Quality: string(item.Quality), Disclosure: string(item.Disclosure),
			}
			size, err := encodedSize(projected)
			if err != nil {
				return provider.Projection{}, provider.ProjectionManifest{}, err
			}
			if itemCount+1 > limits.MaximumItems || size > limits.MaximumBytes || classBytes > limits.MaximumBytes-size {
				return provider.Projection{}, provider.ProjectionManifest{}, fmt.Errorf(
					"prepare generated diagnosis: %s exceeds the selected profile disclosure limit", class,
				)
			}
			itemCount++
			classBytes += size
			projection.Items = append(projection.Items, projected)
			manifest.ItemIDs = append(manifest.ItemIDs, item.ID)
		}
		if itemCount != 0 {
			manifest.Classes = append(manifest.Classes, string(class))
		}
	}
	if !slices.Contains(manifest.Classes, string(diagnostic.DisclosureMetadata)) {
		return provider.Projection{}, provider.ProjectionManifest{}, errors.New(
			"prepare generated diagnosis: no metadata evidence is available for disclosure",
		)
	}
	if slices.Contains(approved, string(diagnostic.DisclosureLogContent)) && len(evidence.Artifacts) != 0 {
		if !slices.Contains(evidence.Source.Capabilities, "configured_value_redaction_v1") {
			return provider.Projection{}, provider.ProjectionManifest{}, errors.New(
				"prepare generated diagnosis: log_content requires core configured_value_redaction_v1",
			)
		}
		limits := profile.Disclosure[string(diagnostic.DisclosureLogContent)]
		var contentBytes uint64
		for _, artifact := range evidence.Artifacts {
			if artifact.Disclosure != diagnostic.DisclosureLogContent {
				continue
			}
			if uint64(len(projection.Artifacts)+1) > limits.MaximumArtifacts ||
				artifact.ContentBytes > limits.MaximumBytes || contentBytes > limits.MaximumBytes-artifact.ContentBytes {
				return provider.Projection{}, provider.ProjectionManifest{}, errors.New(
					"prepare generated diagnosis: log_content exceeds the selected profile disclosure limit",
				)
			}
			contentBytes += artifact.ContentBytes
			projection.Artifacts = append(projection.Artifacts, provider.ProjectedArtifact{
				ID: artifact.ID, Role: artifact.Role, Run: artifact.Run, Stream: artifact.Stream,
				Content: strings.ToValidUTF8(string(artifact.Data), "�"), Encoding: "utf-8-lossy",
				Digest: artifact.Digest, Truncated: artifact.Truncated, SelectedBytes: artifact.SelectedBytes,
				ContentBytes: artifact.ContentBytes, Disclosure: string(artifact.Disclosure),
			})
			manifest.ArtifactIDs = append(manifest.ArtifactIDs, artifact.ID)
		}
		if len(projection.Artifacts) != 0 {
			manifest.Classes = append(manifest.Classes, string(diagnostic.DisclosureLogContent))
			manifest.ArtifactBytes += contentBytes
			for _, item := range failureEvidence.Enrichment {
				if !slices.Contains(manifest.ArtifactIDs, item.SourceArtifactID) {
					continue
				}
				projected := provider.ProjectedEnrichment{
					ID: item.ID, Code: item.Code, Format: item.Format,
					SourceArtifactID: item.SourceArtifactID, ByteStart: item.ByteStart, ByteEnd: item.ByteEnd,
					Collector: item.Collector.Name, CollectorVersion: item.Collector.Version,
					Quality: string(item.Quality), Disclosure: string(diagnostic.DisclosureLogContent),
					DiagnosticLines: projectedDiagnosticLines(evidence.Artifacts, item),
				}
				size, err := encodedSize(projected)
				if err != nil {
					return provider.Projection{}, provider.ProjectionManifest{}, err
				}
				projection.Enrichment = append(projection.Enrichment, projected)
				manifest.EnrichmentIDs = append(manifest.EnrichmentIDs, item.ID)
				manifest.EnrichmentBytes += size
			}
		}
	}
	if slices.Contains(approved, string(diagnosis.DisclosureSourceContent)) &&
		len(failureEvidence.SourceContext) != 0 {
		limits, allowed := profile.Disclosure[string(diagnosis.DisclosureSourceContent)]
		if !allowed {
			return provider.Projection{}, provider.ProjectionManifest{}, errors.New(
				"prepare generated diagnosis: source_content is not allowed by the selected profile",
			)
		}
		var sourceCount, contentBytes uint64
		for _, source := range failureEvidence.SourceContext {
			if source.Disclosure != diagnosis.DisclosureSourceContent {
				continue
			}
			if sourceCount+1 > limits.MaximumArtifacts || source.ContentBytes > limits.MaximumBytes ||
				contentBytes > limits.MaximumBytes-source.ContentBytes {
				return provider.Projection{}, provider.ProjectionManifest{}, errors.New(
					"prepare generated diagnosis: source_content exceeds the selected profile disclosure limit",
				)
			}
			capturedAt := source.CapturedAt.UTC().Round(0)
			projection.Artifacts = append(projection.Artifacts, provider.ProjectedArtifact{
				ID: source.ID, Role: source.Role, Path: source.Path, Language: source.Language,
				MediaType: source.MediaType, Selection: string(source.Mode), AnchorLine: source.AnchorLine,
				AnchorReason: source.AnchorReason, StartLine: source.StartLine, EndLine: source.EndLine,
				TotalLines: source.TotalLines, ByteStart: source.ByteStart, ByteEnd: source.ByteEnd,
				FileBytes: source.FileBytes, Content: string(source.Data), Encoding: "utf-8",
				Digest: source.Digest, ContentDigest: source.ContentDigest, CapturedAt: &capturedAt,
				Quality: string(source.Quality), SelectedBytes: source.ContentBytes,
				ContentBytes: source.ContentBytes, Disclosure: string(source.Disclosure),
			})
			manifest.ArtifactIDs = append(manifest.ArtifactIDs, source.ID)
			sourceCount++
			contentBytes += source.ContentBytes
		}
		if sourceCount != 0 {
			manifest.Classes = append(manifest.Classes, string(diagnosis.DisclosureSourceContent))
			manifest.ArtifactBytes += contentBytes
		}
	}
	manifest.ItemCount = uint64(len(manifest.ItemIDs))
	manifest.ArtifactCount = uint64(len(manifest.ArtifactIDs))
	manifest.EnrichmentCount = uint64(len(manifest.EnrichmentIDs))
	projectedIDs := append(slices.Clone(manifest.ItemIDs), manifest.ArtifactIDs...)
	projectedIDs = append(projectedIDs, manifest.EnrichmentIDs...)
	for _, notice := range evidence.RedactionNotices {
		affects := make([]string, 0, len(notice.Affects))
		for _, id := range notice.Affects {
			if slices.Contains(projectedIDs, id) {
				affects = append(affects, id)
			}
		}
		if len(affects) != 0 {
			projection.RedactionNotices = append(projection.RedactionNotices, provider.ProjectedRedaction{
				Code: notice.Code, Affects: affects, Count: uint64(len(affects)),
			})
		}
	}
	manifest.RedactionNoticeCount = uint64(len(projection.RedactionNotices))

	return projection, manifest, nil
}

func projectedDiagnosticLines(
	artifacts []diagnostic.Artifact,
	item diagnosis.EnrichmentItem,
) []string {
	for _, artifact := range artifacts {
		if artifact.ID != item.SourceArtifactID || item.ByteStart >= item.ByteEnd || item.ByteEnd > uint64(len(artifact.Data)) {
			continue
		}
		selected := strings.ToValidUTF8(string(artifact.Data[item.ByteStart:item.ByteEnd]), "�")
		return selectDiagnosticLines(item.Format, strings.Split(selected, "\n"))
	}

	return []string{}
}

func selectDiagnosticLines(format string, lines []string) []string {
	switch format {
	case "jvm_exception":
		return jvmDiagnosticLines(lines)
	case "python_traceback":
		return pythonDiagnosticLines(lines)
	case "go_panic":
		return firstDiagnosticLine(lines, func(line string) bool { return strings.HasPrefix(line, "panic:") })
	case "compiler_diagnostic":
		return firstDiagnosticLine(lines, func(line string) bool { return strings.Contains(strings.ToLower(line), ": error:") })
	case "causal_message":
		return firstDiagnosticLine(lines, func(line string) bool { return strings.TrimSpace(line) != "" })
	default:
		return []string{}
	}
}

func pythonDiagnosticLines(lines []string) []string {
	result := make([]string, 0, 4)
	for _, line := range lines {
		candidate := strings.TrimSpace(line)
		candidate = strings.TrimLeft(candidate, "|+- ")
		separator := strings.IndexByte(candidate, ':')
		if separator <= 0 {
			continue
		}
		class := candidate[:separator]
		if strings.ContainsAny(class, " \t,\"") || strings.EqualFold(class, "traceback (most recent call last)") {
			continue
		}
		result = appendBoundedDiagnosticLine(result, candidate)
	}

	return result
}

func jvmDiagnosticLines(lines []string) []string {
	result := make([]string, 0, 4)
	for index, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "Caused by:") {
			continue
		}
		result = appendBoundedDiagnosticLine(result, line)
		frame := firstDiagnosticLine(lines[index+1:], func(candidate string) bool {
			return strings.HasPrefix(candidate, "at ")
		})
		if len(frame) != 0 {
			result = appendBoundedDiagnosticLine(result, frame[0])
		}
	}

	return result
}

func firstDiagnosticLine(lines []string, matches func(string) bool) []string {
	for _, line := range lines {
		if matches(strings.TrimSpace(line)) {
			return appendBoundedDiagnosticLine([]string{}, line)
		}
	}

	return []string{}
}

func appendBoundedDiagnosticLine(lines []string, value string) []string {
	value = strings.TrimSpace(value)
	totalBytes := len(value)
	for _, line := range lines {
		totalBytes += len(line)
	}
	if value == "" || len(value) > 512 || len(lines) >= 8 || totalBytes > 2048 {
		return lines
	}

	return append(lines, value)
}

func usefulGenerationItem(code string) bool {
	// A store-local fingerprint cannot explain a target failure, and an
	// ordinary complete-at-exit resource observation reports usage rather than
	// a configured limit. Keeping both out of the provider projection reduces
	// small-model anchoring on causally irrelevant metadata. The deterministic
	// engine still retains and analyzes the complete evidence bundle.
	switch code {
	case diagnostic.CodeSourceContext,
		diagnostic.CodeJobRevision,
		diagnostic.CodeJobSubmittedAt,
		diagnostic.CodeJobClaimedAt,
		diagnostic.CodeJobStartedAt,
		diagnostic.CodeJobCompletedAt,
		diagnostic.CodeRunRevision,
		diagnostic.CodeRunReservedAt,
		diagnostic.CodeRunStartedAt,
		diagnostic.CodeRunCompletedAt,
		diagnostic.CodeLogStdoutBytes,
		diagnostic.CodeLogStderrBytes,
		diagnostic.CodeLifecycleEvent,
		diagnostic.CodeFailureFingerprint,
		diagnostic.CodeResourceObservation:
		return false
	default:
		return true
	}
}

func projectDeterministic(report diagnosis.Report, manifest provider.ProjectionManifest) []provider.DeterministicCandidate {
	available := append(slices.Clone(manifest.ItemIDs), manifest.ArtifactIDs...)
	available = append(available, manifest.EnrichmentIDs...)
	result := make([]provider.DeterministicCandidate, 0, len(report.Findings))
	for _, finding := range report.Findings {
		// Target-log signatures confirm that a recognizable structure exists but
		// intentionally stop short of root-cause analysis. Sending their generic
		// prose to a small model encourages paraphrase instead of inspection of
		// the already projected artifact content and enrichment range.
		if strings.HasPrefix(finding.Code, "target.") {
			continue
		}
		supporting := availableReferences(finding.SupportingEvidence, available)
		if len(supporting) == 0 {
			continue
		}
		result = append(result, provider.DeterministicCandidate{
			ID: finding.ID, Code: finding.Code, Category: finding.Category, Summary: finding.Summary,
			Explanation: finding.Explanation, SupportingEvidence: supporting,
			ContradictingEvidence: availableReferences(finding.ContradictingEvidence, available),
		})
	}

	return result
}

func availableReferences(references, available []string) []string {
	result := make([]string, 0, len(references))
	for _, reference := range references {
		if slices.Contains(available, reference) {
			result = append(result, reference)
		}
	}

	return result
}

func encodedSize(value any) (uint64, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("prepare generated diagnosis: encode projected value: %w", err)
	}

	return uint64(len(encoded)), nil
}
