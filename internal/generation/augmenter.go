package generation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

const generatedAnalyzer = "generator.proposal/1"

// Generator is the provider capability required by generated diagnosis.
type Generator interface {
	provider.StructuredGenerator
	provider.Describer
}

// Augmenter wraps a deterministic diagnostician with one explicitly selected
// structured generator. The deterministic report remains authoritative.
type Augmenter struct {
	base        diagnosis.Diagnostician
	generator   Generator
	profileName string
	profile     config.Profile
	approved    []string
	required    bool
}

// NewAugmenter validates generator capabilities without making a request.
func NewAugmenter(
	base diagnosis.Diagnostician,
	generator Generator,
	profileName string,
	profile config.Profile,
	approved []string,
	required bool,
) (*Augmenter, error) {
	if base == nil || generator == nil || strings.TrimSpace(profileName) == "" {
		return nil, errors.New("construct generated diagnosis: base, generator, and profile are required")
	}
	capabilities := generator.Capabilities()
	if !capabilities.NativeJSONSchema || capabilities.MaximumInputBytes < profile.MaximumInputBytes ||
		capabilities.MaximumOutputBytes < profile.MaximumOutputBytes || capabilities.Locality != profile.Locality {
		return nil, errors.New("construct generated diagnosis: generator capabilities do not satisfy the profile")
	}

	return &Augmenter{
		base: base, generator: generator, profileName: profileName, profile: profile,
		approved: slices.Clone(approved), required: required,
	}, nil
}

// Diagnose produces the deterministic report first, then optionally appends
// only validated, uncalibrated generated proposals.
func (augmenter *Augmenter) Diagnose(ctx context.Context, evidence diagnosis.FailureEvidence) (diagnosis.Report, error) {
	if ctx == nil {
		return diagnosis.Report{}, errors.New("diagnose with generator: nil context")
	}
	report, err := augmenter.base.Diagnose(ctx, evidence)
	if err != nil {
		return diagnosis.Report{}, err
	}
	approved, err := augmenter.profile.ApprovedClasses(augmenter.approved)
	if err != nil {
		return diagnosis.Report{}, err
	}
	prepared, err := Prepare(evidence, report, augmenter.profileName, augmenter.profile, approved)
	if err != nil {
		return diagnosis.Report{}, err
	}
	requestContext, cancel := context.WithTimeout(ctx, augmenter.profile.TimeoutDuration())
	defer cancel()
	response, generateErr := augmenter.generator.Generate(requestContext, prepared.Request)
	if generateErr != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return diagnosis.Report{}, contextErr
		}

		return augmenter.handleFailure(report, evidence, prepared, "generator_failed", generatorFailureDetail(generateErr))
	}
	if response.RequestID != prepared.Request.RequestID || response.Provider != augmenter.generator.Name() ||
		response.Model != augmenter.profile.Model {
		return augmenter.handleFailure(
			report, evidence, prepared, "generator_provenance_invalid",
			"response_provenance_mismatch: provider response provenance did not match the selected request and profile",
		)
	}
	proposal, err := provider.DecodeProposal(bytes.NewReader(response.JSON), prepared.Request)
	if err != nil {
		return augmenter.handleFailure(
			report, evidence, prepared, "generator_proposal_invalid",
			"proposal_validation_failed: model output did not satisfy Jobman's bounded proposal rules",
		)
	}
	if len(proposal.Hypotheses) == 0 && len(proposal.RecommendedActions) == 0 && len(proposal.MissingEvidence) == 0 {
		return sealFallback(
			report, evidence, prepared, "generator_abstained",
			"model returned no hypotheses, recommended actions, or missing-evidence items",
		)
	}

	return reconcile(report, evidence, prepared, proposal)
}

func (augmenter *Augmenter) handleFailure(
	report diagnosis.Report,
	evidence diagnosis.FailureEvidence,
	prepared Prepared,
	code string,
	detail string,
) (diagnosis.Report, error) {
	if augmenter.required {
		if detail != "" {
			return diagnosis.Report{}, fmt.Errorf("generated diagnosis required: %s: %s", code, detail)
		}

		return diagnosis.Report{}, fmt.Errorf("generated diagnosis required: %s", code)
	}

	return sealFallback(report, evidence, prepared, code, detail)
}

func generatorFailureDetail(err error) string {
	if code, message, ok := provider.Diagnostic(err); ok {
		return fmt.Sprintf("%s: %s", code, message)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "request_timeout: the provider request exceeded the configured timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "request_canceled: the provider request was canceled"
	}

	return "provider_failure_unspecified: the provider failed without a safe diagnostic classification"
}

func sealFallback(
	report diagnosis.Report,
	evidence diagnosis.FailureEvidence,
	prepared Prepared,
	code string,
	detail string,
) (diagnosis.Report, error) {
	report.Versions.GenerationRequestSchemaVersion = provider.RequestSchemaVersion
	report.Versions.ProposalSchemaVersion = provider.ProposalSchemaVersion
	report.Disclosure = prepared.Disclosure(false)
	report.Generators = []diagnosis.GeneratorDescriptor{augmenterDescriptor(prepared)}
	message := "The optional structured generator did not contribute; this report contains the complete deterministic result."
	if detail != "" {
		message += " Provider diagnostic: " + detail + "."
	}
	report.Warnings = append(report.Warnings, diagnosis.Warning{
		Code:    code,
		Message: message,
	})
	sealed, err := diagnosis.Seal(report)
	if err != nil {
		return diagnosis.Report{}, fmt.Errorf("seal deterministic fallback: %w", err)
	}
	if err := diagnosis.ValidateAgainstEvidence(sealed, evidence); err != nil {
		return diagnosis.Report{}, fmt.Errorf("validate deterministic fallback: %w", err)
	}

	return sealed, nil
}

func reconcile(
	report diagnosis.Report,
	evidence diagnosis.FailureEvidence,
	prepared Prepared,
	proposal provider.Proposal,
) (diagnosis.Report, error) {
	report.Versions.GenerationRequestSchemaVersion = provider.RequestSchemaVersion
	report.Versions.ProposalSchemaVersion = provider.ProposalSchemaVersion
	confidence, err := diagnosis.NewConfidence(
		40,
		"This is an uncalibrated generated hypothesis ranked below Jobman's observed and exact deterministic findings.",
	)
	if err != nil {
		return diagnosis.Report{}, err
	}
	for _, hypothesis := range proposal.Hypotheses {
		if duplicatesDeterministicFinding(hypothesis, report.Findings) {
			continue
		}
		identifier := fmt.Sprintf(
			"finding:%03d:%s", len(report.Findings)+1, strings.ReplaceAll(hypothesis.Code, ".", "-"),
		)
		report.Findings = append(report.Findings, diagnosis.Finding{
			ID: identifier, Code: hypothesis.Code, Category: hypothesis.Category, Severity: diagnosis.SeverityWarning,
			Summary: hypothesis.Summary, Explanation: hypothesis.Explanation, Confidence: confidence,
			SupportingEvidence:    slices.Clone(hypothesis.SupportingEvidence),
			ContradictingEvidence: slices.Clone(hypothesis.ContradictingEvidence),
			ContradictingFindings: slices.Clone(hypothesis.ContradictsFindings), Analyzer: generatedAnalyzer,
		})
	}
	report.Actions = reorderActions(report.Actions, proposal.RecommendedActions)
	for _, missing := range proposal.MissingEvidence {
		report.MissingEvidence = append(report.MissingEvidence, diagnosis.MissingEvidence{
			Code: missing.Code, Description: missing.Description,
		})
	}
	report.Warnings = append(report.Warnings, diagnosis.Warning{
		Code:    "generated_content_uncalibrated",
		Message: "Generated hypotheses are advisory and uncalibrated; deterministic facts, actions, and retry policy remain authoritative.",
	})
	report.Citations = appendGeneratedCitations(report.Citations, evidence, proposal)
	report.Mode = diagnosis.ModeMixed
	report.Disclosure = prepared.Disclosure(true)
	report.Generators = []diagnosis.GeneratorDescriptor{augmenterDescriptor(prepared)}
	sealed, err := diagnosis.Seal(report)
	if err != nil {
		return diagnosis.Report{}, fmt.Errorf("reconcile generated diagnosis: seal: %w", err)
	}
	if err := diagnosis.ValidateAgainstEvidence(sealed, evidence); err != nil {
		return diagnosis.Report{}, fmt.Errorf("reconcile generated diagnosis: validate: %w", err)
	}

	return sealed, nil
}

func augmenterDescriptor(prepared Prepared) diagnosis.GeneratorDescriptor {
	return diagnosis.GeneratorDescriptor{
		Provider: prepared.Provider, Model: prepared.Model,
		Profile: prepared.ProfileName, Locality: diagnosis.ProviderLocality(prepared.Locality),
	}
}

func duplicatesDeterministicFinding(hypothesis provider.Hypothesis, findings []diagnosis.Finding) bool {
	if len(hypothesis.ContradictsFindings) != 0 {
		return false
	}
	hypothesisCode := strings.TrimPrefix(hypothesis.Code, "generated.")
	support := slices.Clone(hypothesis.SupportingEvidence)
	slices.Sort(support)
	for _, finding := range findings {
		if finding.Analyzer == generatedAnalyzer {
			continue
		}
		findingSupport := slices.Clone(finding.SupportingEvidence)
		slices.Sort(findingSupport)
		sameText := hypothesis.Summary == finding.Summary && hypothesis.Explanation == finding.Explanation
		if (hypothesisCode == finding.Code || sameText) && slices.Equal(support, findingSupport) {
			return true
		}
	}

	return false
}

func reorderActions(actions []diagnosis.Action, preferred []string) []diagnosis.Action {
	if len(preferred) == 0 {
		return actions
	}
	byID := make(map[string]diagnosis.Action, len(actions))
	for _, action := range actions {
		byID[action.ID] = action
	}
	result := make([]diagnosis.Action, 0, len(actions))
	used := make(map[string]struct{}, len(preferred))
	for _, id := range preferred {
		result = append(result, byID[id])
		used[id] = struct{}{}
	}
	for _, action := range actions {
		if _, ok := used[action.ID]; !ok {
			result = append(result, action)
		}
	}

	return result
}

func appendGeneratedCitations(
	citations []diagnosis.Citation,
	evidence diagnosis.FailureEvidence,
	proposal provider.Proposal,
) []diagnosis.Citation {
	seen := make(map[string]struct{}, len(citations))
	for _, citation := range citations {
		seen[citation.EvidenceID] = struct{}{}
	}
	references := make([]string, 0)
	for _, hypothesis := range proposal.Hypotheses {
		references = append(references, hypothesis.SupportingEvidence...)
		references = append(references, hypothesis.ContradictingEvidence...)
	}
	slices.Sort(references)
	references = slices.Compact(references)
	items := make(map[string]diagnostic.Item, len(evidence.Core.Items))
	for _, item := range evidence.Core.Items {
		items[item.ID] = item
	}
	artifacts := make(map[string]diagnostic.Artifact, len(evidence.Core.Artifacts))
	for _, artifact := range evidence.Core.Artifacts {
		artifacts[artifact.ID] = artifact
	}
	enrichment := make(map[string]diagnosis.EnrichmentItem, len(evidence.Enrichment))
	for _, item := range evidence.Enrichment {
		enrichment[item.ID] = item
	}
	for _, reference := range references {
		if _, ok := seen[reference]; ok {
			continue
		}
		if item, ok := items[reference]; ok {
			citations = append(citations, diagnosis.Citation{
				EvidenceID: reference, Code: item.Code, Summary: "A projected structured Jobman evidence item.", Kind: "item",
			})
		} else if artifact, ok := artifacts[reference]; ok {
			citations = append(citations, diagnosis.Citation{
				EvidenceID: reference, Code: artifact.Role,
				Summary: "A bounded, sanitized, untrusted target log excerpt.", Kind: "artifact",
			})
		} else if item, ok := enrichment[reference]; ok {
			citations = append(citations, diagnosis.Citation{
				EvidenceID: item.ID, Code: item.Code,
				Summary: "A bounded deterministic structure derived from the selected sanitized artifact.",
				Kind:    "enrichment", SourceEvidenceID: item.SourceArtifactID,
				ByteStart: item.ByteStart, ByteEnd: item.ByteEnd,
			})
		}
	}

	return citations
}

// Compile-time assertion that the wrapper preserves the public domain seam.
var _ diagnosis.Diagnostician = (*Augmenter)(nil)
