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
	"policy", "prerequisite", "process", "resource", "state", "storage",
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
		EvidenceID: evidence.Core.EvidenceID,
		Subject: provider.Subject{
			Phase: evidence.Core.Subject.Phase, Outcome: evidence.Core.Subject.Outcome,
			SelectedRuns: slices.Clone(evidence.Core.Subject.SelectedRuns),
		},
		Projection: projection, Manifest: manifest, Deterministic: deterministic,
		AllowedCategories:      slices.Clone(allowedCategories),
		AllowedHypothesisCodes: slices.Clone(allowedHypothesisCodes), AllowedActions: actions,
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
			if item.Disclosure != class {
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
			manifest.ArtifactBytes = contentBytes
			for _, item := range failureEvidence.Enrichment {
				if !slices.Contains(manifest.ArtifactIDs, item.SourceArtifactID) {
					continue
				}
				projected := provider.ProjectedEnrichment{
					ID: item.ID, Code: item.Code, Format: item.Format,
					SourceArtifactID: item.SourceArtifactID, ByteStart: item.ByteStart, ByteEnd: item.ByteEnd,
					Collector: item.Collector.Name, CollectorVersion: item.Collector.Version,
					Quality: string(item.Quality), Disclosure: string(diagnostic.DisclosureLogContent),
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

func projectDeterministic(report diagnosis.Report, manifest provider.ProjectionManifest) []provider.DeterministicCandidate {
	available := append(slices.Clone(manifest.ItemIDs), manifest.ArtifactIDs...)
	available = append(available, manifest.EnrichmentIDs...)
	result := make([]provider.DeterministicCandidate, 0, len(report.Findings))
	for _, finding := range report.Findings {
		references := append(slices.Clone(finding.SupportingEvidence), finding.ContradictingEvidence...)
		if !allAvailable(references, available) {
			continue
		}
		result = append(result, provider.DeterministicCandidate{
			ID: finding.ID, Code: finding.Code, Category: finding.Category, Summary: finding.Summary,
			Explanation: finding.Explanation, SupportingEvidence: slices.Clone(finding.SupportingEvidence),
			ContradictingEvidence: slices.Clone(finding.ContradictingEvidence),
		})
	}

	return result
}

func allAvailable(references, available []string) bool {
	for _, reference := range references {
		if !slices.Contains(available, reference) {
			return false
		}
	}

	return true
}

func encodedSize(value any) (uint64, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("prepare generated diagnosis: encode projected value: %w", err)
	}

	return uint64(len(encoded)), nil
}
