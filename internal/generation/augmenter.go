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

const generatedAnalyzer = "generator.proposal/2"

type attemptFailure struct {
	code   string
	detail string
}

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
	progress    ProgressObserver
}

// NewAugmenter validates generator capabilities without making a request.
func NewAugmenter(
	base diagnosis.Diagnostician,
	generator Generator,
	profileName string,
	profile config.Profile,
	approved []string,
	required bool,
	progress ProgressObserver,
) (*Augmenter, error) {
	if base == nil || generator == nil || strings.TrimSpace(profileName) == "" {
		return nil, errors.New("construct generated diagnosis: base, generator, and profile are required")
	}
	if err := ValidateGenerator(profile, generator); err != nil {
		return nil, fmt.Errorf("construct generated diagnosis: %w", err)
	}

	return &Augmenter{
		base: base, generator: generator, profileName: profileName, profile: profile,
		approved: slices.Clone(approved), required: required, progress: progress,
	}, nil
}

// ValidateGenerator verifies the adapter semantics required by a selected
// profile without contacting its provider.
func ValidateGenerator(profile config.Profile, generator Generator) error {
	if generator == nil {
		return errors.New("generator is required")
	}
	capabilities := generator.Capabilities()
	if !capabilities.NativeJSONSchema || capabilities.MaximumInputBytes < profile.MaximumInputBytes ||
		capabilities.MaximumOutputBytes < profile.MaximumOutputBytes || capabilities.Locality != profile.Locality {
		return errors.New("generator capabilities do not satisfy the profile")
	}

	return nil
}

// Diagnose produces the deterministic report first, then optionally appends
// only validated, uncalibrated generated proposals. Profiles may permit one
// bounded review call for complex or multi-branch incidents; both calls share
// the profile timeout and identical evidence authority.
//
//nolint:cyclop // The provider boundary keeps each provenance, validation, abstention, and fallback decision explicit.
func (augmenter *Augmenter) Diagnose(ctx context.Context, evidence diagnosis.FailureEvidence) (diagnosis.Report, error) {
	if ctx == nil {
		return diagnosis.Report{}, errors.New("diagnose with generator: nil context")
	}
	report, err := augmenter.base.Diagnose(ctx, evidence)
	if err != nil {
		return diagnosis.Report{}, err
	}
	augmenter.notify(ProgressPreparing)
	approved, err := augmenter.profile.ApprovedClasses(augmenter.approved)
	if err != nil {
		return diagnosis.Report{}, err
	}
	prepared, err := Prepare(evidence, report, augmenter.profileName, augmenter.profile, approved)
	if err != nil {
		return diagnosis.Report{}, err
	}
	generationContext, cancel := context.WithTimeout(ctx, augmenter.profile.TimeoutDuration())
	defer cancel()
	proposal, failure := augmenter.requestProposal(generationContext, prepared.Request, ProgressWaiting)
	if contextErr := ctx.Err(); contextErr != nil {
		return diagnosis.Report{}, contextErr
	}
	if failure != nil {
		return augmenter.handleFailure(report, evidence, prepared, failure.code, failure.detail)
	}
	if !proposalHasContent(proposal) {
		augmenter.notify(ProgressFallback)
		return sealFallback(
			report, evidence, prepared, "generator_abstained",
			"model returned no hypotheses, recommended actions, or missing-evidence items",
		)
	}
	warnings := make([]diagnosis.Warning, 0, 1)
	if augmenter.profile.AnalysisPassLimit() > 1 && shouldReview(prepared.Request, proposal) {
		reviewPrepared, reviewErr := PrepareReview(prepared, proposal, augmenter.profile.MaximumInputBytes)
		if reviewErr != nil {
			warnings = append(warnings, reviewWarning(
				"generator_review_skipped",
				"The optional second synthesis pass did not fit the configured request limit; the validated first-pass assessment was retained.",
			))
		} else {
			reviewed, reviewFailure := augmenter.requestProposal(
				generationContext, reviewPrepared.Request, ProgressReviewing,
			)
			if contextErr := ctx.Err(); contextErr != nil {
				return diagnosis.Report{}, contextErr
			}
			// The review request has now been attempted. Record its digest and
			// combined byte count conservatively even when the first proposal is
			// retained because review transport, validation, or quality failed.
			prepared = reviewPrepared
			switch {
			case reviewFailure != nil:
				warnings = append(warnings, reviewWarning(
					"generator_review_failed",
					"The optional second synthesis pass failed validation or transport; the validated first-pass assessment was retained.",
				))
			case !proposalHasContent(reviewed):
				warnings = append(warnings, reviewWarning(
					"generator_review_abstained",
					"The optional second synthesis pass returned no assessment; the validated first-pass assessment was retained.",
				))
			default:
				if reviewLosesGroundedCausalDetail(reviewed, proposal, reviewPrepared.Request.Projection) {
					warnings = append(warnings, reviewWarning(
						"generator_review_regressed",
						"The optional second synthesis pass lost grounded causal detail; the more specific validated first-pass assessment was retained.",
					))
				} else {
					proposal = reviewed
				}
			}
		}
	}

	return reconcile(report, evidence, prepared, proposal, warnings...)
}

func (augmenter *Augmenter) requestProposal(
	ctx context.Context,
	request provider.Request,
	waiting ProgressEvent,
) (provider.Proposal, *attemptFailure) {
	augmenter.notify(waiting)
	response, err := augmenter.generator.Generate(ctx, request)
	if err != nil {
		return provider.Proposal{}, &attemptFailure{code: "generator_failed", detail: generatorFailureDetail(err)}
	}
	augmenter.notify(ProgressValidating)
	if response.RequestID != request.RequestID || response.Provider != augmenter.generator.Name() ||
		response.Model != augmenter.profile.Model {
		return provider.Proposal{}, &attemptFailure{
			code:   "generator_provenance_invalid",
			detail: "response_provenance_mismatch: provider response provenance did not match the selected request and profile",
		}
	}
	proposal, err := provider.DecodeProposal(bytes.NewReader(response.JSON), request)
	if err == nil {
		return proposal, nil
	}
	detail := "proposal_validation_failed: model output did not satisfy Jobman's bounded proposal rules"
	if errors.Is(err, provider.ErrProposalNotSpecific) {
		detail = "proposal_not_specific: model output did not provide a distinct, incident-specific causal assessment"
	} else if errors.Is(err, provider.ErrProposalUnsupported) {
		detail = "proposal_evidence_unsupported: model output asserted a cause not supported by its cited evidence"
	}

	return provider.Proposal{}, &attemptFailure{code: "generator_proposal_invalid", detail: detail}
}

func proposalHasContent(proposal provider.Proposal) bool {
	return len(proposal.Hypotheses) != 0 || len(proposal.RecommendedActions) != 0 || len(proposal.MissingEvidence) != 0
}

func shouldReview(request provider.Request, proposal provider.Proposal) bool {
	if request.Stage != provider.AnalysisInitial || len(proposal.Hypotheses) == 0 {
		return false
	}
	if len(proposal.Hypotheses) > 1 {
		return true
	}
	logArtifacts, sourceArtifacts := 0, 0
	for _, artifact := range request.Projection.Artifacts {
		switch artifact.Disclosure {
		case string(diagnostic.DisclosureLogContent):
			logArtifacts++
		case string(diagnosis.DisclosureSourceContent):
			sourceArtifacts++
		}
	}
	diagnosticLines := 0
	for _, focus := range request.DiagnosticFocus {
		diagnosticLines += len(focus.Lines)
	}

	return diagnosticLines > 1 || len(request.Projection.Enrichment) > 1 ||
		logArtifacts > 1 || logArtifacts != 0 && sourceArtifacts != 0
}

func reviewWarning(code, message string) diagnosis.Warning {
	return diagnosis.Warning{Code: code, Message: message}
}

func groundedCausalDetailScore(proposal provider.Proposal, projection provider.Projection) int {
	evidence := make(map[string]struct{})
	for _, token := range causalDetailTokens(projectedCausalText(projection)) {
		evidence[token] = struct{}{}
	}
	seen := make(map[string]struct{})
	score := 0
	for _, hypothesis := range proposal.Hypotheses {
		causal := strings.Join([]string{hypothesis.Summary, hypothesis.RootCause, hypothesis.Explanation}, " ")
		for _, token := range causalDetailTokens(causal) {
			if _, duplicate := seen[token]; duplicate {
				continue
			}
			if _, grounded := evidence[token]; !grounded {
				continue
			}
			seen[token] = struct{}{}
			score++
			if strings.IndexFunc(token, func(value rune) bool { return value >= '0' && value <= '9' }) >= 0 ||
				len(token) >= 10 {
				score++
			}
		}
	}

	return score
}

func reviewLosesGroundedCausalDetail(reviewed, initial provider.Proposal, projection provider.Projection) bool {
	initialScore := groundedCausalDetailScore(initial, projection)
	reviewedScore := groundedCausalDetailScore(reviewed, projection)

	return initialScore > 0 && reviewedScore*5 < initialScore*4
}

func projectedCausalText(projection provider.Projection) string {
	var value strings.Builder
	for _, enrichment := range projection.Enrichment {
		for _, line := range enrichment.DiagnosticLines {
			value.WriteString(line)
			value.WriteByte(' ')
		}
	}
	for _, artifact := range projection.Artifacts {
		if artifact.Disclosure == "log_content" || artifact.Disclosure == "source_content" {
			value.WriteString(artifact.Content)
			value.WriteByte(' ')
		}
	}

	return value.String()
}

func causalDetailTokens(value string) []string {
	ignored := map[string]struct{}{
		"application": {}, "caused": {}, "condition": {}, "configuration": {}, "data": {}, "error": {},
		"failed": {}, "failure": {}, "invalid": {}, "nonzero": {}, "process": {}, "status": {},
		"target": {}, "validation": {}, "with": {}, "without": {},
	}
	fields := strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return character < 'a' || character > 'z' && (character < '0' || character > '9')
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, skip := ignored[field]; !skip {
			result = append(result, field)
		}
	}

	return result
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
	augmenter.notify(ProgressFallback)

	return sealFallback(report, evidence, prepared, code, detail)
}

func (augmenter *Augmenter) notify(event ProgressEvent) {
	if augmenter.progress != nil {
		augmenter.progress(event)
	}
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
	report = appendSourceContextWarning(report, evidence, prepared.Request.Manifest.ArtifactIDs)
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
	additionalWarnings ...diagnosis.Warning,
) (diagnosis.Report, error) {
	report.Versions.GenerationRequestSchemaVersion = provider.RequestSchemaVersion
	report.Versions.ProposalSchemaVersion = provider.ProposalSchemaVersion
	for _, hypothesis := range proposal.Hypotheses {
		if duplicatesDeterministicFinding(hypothesis, report.Findings) {
			continue
		}
		confidence, err := generatedConfidence(hypothesis)
		if err != nil {
			return diagnosis.Report{}, err
		}
		identifier := fmt.Sprintf(
			"finding:%03d:%s", len(report.Findings)+1, strings.ReplaceAll(hypothesis.Code, ".", "-"),
		)
		report.Findings = append(report.Findings, diagnosis.Finding{
			ID: identifier, Code: hypothesis.Code, Category: hypothesis.Category, Severity: diagnosis.SeverityWarning,
			Summary: hypothesis.Summary,
			Explanation: generatedExplanation(
				hypothesis, diagnosticDetailsForHypothesis(hypothesis, prepared.Request),
			),
			Confidence:            confidence,
			SupportingEvidence:    slices.Clone(hypothesis.SupportingEvidence),
			ContradictingEvidence: slices.Clone(hypothesis.ContradictingEvidence),
			ContradictingFindings: slices.Clone(hypothesis.ContradictsFindings),
			Analyzer:              generatedAnalyzer + "/" + string(hypothesis.Role),
		})
	}
	report.Actions = reorderActions(report.Actions, proposal.RecommendedActions)
	report.Actions = appendGeneratedPlans(report.Actions, proposal.Hypotheses)
	for _, missing := range proposal.MissingEvidence {
		report.MissingEvidence = append(report.MissingEvidence, diagnosis.MissingEvidence{
			Code: missing.Code, Description: missing.Description,
		})
	}
	report.Warnings = append(report.Warnings, diagnosis.Warning{
		Code:    "generated_content_uncalibrated",
		Message: "Generated hypotheses are advisory and uncalibrated; deterministic facts, actions, and retry policy remain authoritative.",
	})
	report.Warnings = append(report.Warnings, additionalWarnings...)
	report = appendSourceContextWarning(report, evidence, prepared.Request.Manifest.ArtifactIDs)
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

func generatedConfidence(hypothesis provider.Hypothesis) (diagnosis.Confidence, error) {
	score := 30
	role := "alternative explanation"
	switch hypothesis.Role {
	case provider.HypothesisPrimary:
		score = 55
		role = "primary causal assessment"
	case provider.HypothesisContributing:
		score = 45
		role = "contributing causal assessment"
	case provider.HypothesisAlternative:
		// The defaults describe an alternative explanation.
	}
	if len(hypothesis.SupportingEvidence) > 1 {
		score += 5
	}
	if len(hypothesis.ContradictingEvidence) != 0 {
		score -= 10
	}

	return diagnosis.NewConfidence(
		score,
		"This is an uncalibrated "+role+" ranked from its causal role and bounded supporting evidence; Jobman's deterministic observations remain authoritative.",
	)
}

func appendSourceContextWarning(
	report diagnosis.Report,
	evidence diagnosis.FailureEvidence,
	disclosedArtifactIDs []string,
) diagnosis.Report {
	for _, source := range evidence.SourceContext {
		if !slices.Contains(disclosedArtifactIDs, source.ID) {
			continue
		}
		report.Warnings = append(report.Warnings, diagnosis.Warning{
			Code:    "source_context_point_in_time",
			Message: "Shared source context is a point-in-time snapshot of the current file and may differ from the code executed by the recorded run.",
		})

		return report
	}

	return report
}

func generatedExplanation(hypothesis provider.Hypothesis, diagnosticDetails []string) string {
	result := "Root cause: " + sentence(hypothesis.RootCause) + " Failure path: " + sentence(hypothesis.Explanation)
	if len(diagnosticDetails) != 0 {
		result += " Observed diagnostics: " + strings.Join(diagnosticDetails, " | ")
	}

	return result
}

func diagnosticDetailsForHypothesis(hypothesis provider.Hypothesis, request provider.Request) []string {
	support := make(map[string]struct{}, len(hypothesis.SupportingEvidence))
	for _, identifier := range hypothesis.SupportingEvidence {
		support[identifier] = struct{}{}
	}
	enrichmentArtifacts := make(map[string]string, len(request.Projection.Enrichment))
	for _, enrichment := range request.Projection.Enrichment {
		enrichmentArtifacts[enrichment.ID] = enrichment.SourceArtifactID
	}
	result := make([]string, 0, 8)
	totalBytes := 0
	for _, focus := range request.DiagnosticFocus {
		_, direct := support[focus.EvidenceID]
		_, attributed := support[enrichmentArtifacts[focus.EvidenceID]]
		if !direct && !attributed {
			continue
		}
		for _, line := range focus.Lines {
			if slices.Contains(result, line) || len(result) >= 8 || totalBytes+len(line) > 2048 {
				continue
			}
			result = append(result, line)
			totalBytes += len(line)
		}
	}

	return result
}

func sentence(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, ".") || strings.HasSuffix(value, "!") || strings.HasSuffix(value, "?") {
		return value
	}

	return value + "."
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
		if strings.HasPrefix(finding.Analyzer, "generator.") {
			continue
		}
		findingSupport := slices.Clone(finding.SupportingEvidence)
		slices.Sort(findingSupport)
		sameText := hypothesis.Summary == finding.Summary &&
			(hypothesis.RootCause == finding.Explanation || generatedExplanation(hypothesis, nil) == finding.Explanation)
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

func appendGeneratedPlans(actions []diagnosis.Action, hypotheses []provider.Hypothesis) []diagnosis.Action {
	plans := make([]diagnosis.Action, 0, len(hypotheses)*2)
	for index, hypothesis := range hypotheses {
		if hypothesis.Resolution == (provider.ResolutionPlan{}) {
			continue
		}
		support := slices.Clone(hypothesis.SupportingEvidence)
		sequence := index + 1
		verifySummary, verifyCode, remediationSummary, remediationCode := generatedPlanLabels(hypothesis.Role)
		if usefulGeneratedPlanText(hypothesis.Resolution.Verification) {
			description := sentence(hypothesis.Resolution.Verification)
			if usefulGeneratedPlanText(hypothesis.Resolution.SuccessCondition) {
				description += " Expected recovery: " + sentence(hypothesis.Resolution.SuccessCondition)
			}
			plans = append(plans, diagnosis.Action{
				ID: fmt.Sprintf("action:000:generated-verify-%02d", sequence), Code: verifyCode,
				Kind: diagnosis.ActionInspect, Summary: verifySummary + ": " + hypothesis.Summary,
				Description: description, SupportingEvidence: support, Execution: diagnosis.ActionExecutionNone,
				Arguments: []string{}, RequiresConfirmation: false, SafeToAutomate: false,
			})
		}
		if hypothesis.Role == provider.HypothesisAlternative {
			continue
		}
		if usefulGeneratedPlanText(hypothesis.Resolution.Remediation) {
			plans = append(plans, diagnosis.Action{
				ID: fmt.Sprintf("action:000:generated-remediate-%02d", sequence), Code: remediationCode,
				Kind: diagnosis.ActionChange, Summary: remediationSummary + ": " + hypothesis.Summary,
				Description: sentence(hypothesis.Resolution.Remediation), SupportingEvidence: support,
				Execution: diagnosis.ActionExecutionNone, Arguments: []string{},
				RequiresConfirmation: true, SafeToAutomate: false,
			})
		}
	}
	if len(plans) == 0 {
		return actions
	}

	return append(actions, plans...)
}

func generatedPlanLabels(role provider.HypothesisRole) (string, string, string, string) {
	switch role {
	case provider.HypothesisPrimary:
		return "Verify the AI-assisted likely cause", "generated.verify_primary_cause",
			"Address the AI-assisted likely cause", "generated.address_primary_cause"
	case provider.HypothesisContributing:
		return "Verify the contributing condition", "generated.verify_contributing_condition",
			"Address the contributing condition", "generated.address_contributing_condition"
	case provider.HypothesisAlternative:
		return "Distinguish the alternative explanation", "generated.distinguish_alternative", "", ""
	default:
		return "Verify the AI-assisted likely cause", "generated.verify_primary_cause",
			"Address the AI-assisted likely cause", "generated.address_primary_cause"
	}
}

func usefulGeneratedPlanText(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, generic := range []string{
		"a concrete, non-executing observation or comparison that would confirm or refute this claim",
		"the observable condition that would demonstrate recovery after remediation",
		"verify the intended endpoint, dependency health, name resolution, trust policy, and retry guidance",
		"identify the target-specific error before changing configuration or creating another run",
		"inspect the target-specific error before changing configuration or creating another run",
		"inspect the rejected input and validation boundary",
		"target configuration or migration state",
	} {
		if strings.Contains(normalized, generic) {
			return false
		}
	}

	return normalized != ""
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
	sourceContext := make(map[string]diagnosis.SourceContext, len(evidence.SourceContext))
	for _, source := range evidence.SourceContext {
		sourceContext[source.ID] = source
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
		} else if source, ok := sourceContext[reference]; ok {
			citations = append(citations, diagnosis.Citation{
				EvidenceID: source.ID, Code: source.Role,
				Summary: "Explicitly selected point-in-time source context; it may differ from the code that ran.",
				Kind:    "artifact",
			})
		}
	}

	return citations
}

// Compile-time assertion that the wrapper preserves the public domain seam.
var _ diagnosis.Diagnostician = (*Augmenter)(nil)
