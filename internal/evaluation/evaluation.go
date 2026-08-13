// Package evaluation runs checked-in, nonsecret diagnosis cases and reports
// safety and correctness metrics. It is development tooling, not a runtime
// input to the production engine.
package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/sourcecontext"
)

const (
	// Kind identifies an evaluation corpus manifest.
	Kind = "jobman.diagnosis_evaluation_corpus"
	// SchemaVersion is the newest corpus schema supported by this package.
	SchemaVersion = 4

	maximumCorpusBytes = 1024 * 1024
	maximumCases       = 512
)

// GeneratedDisposition states whether accepted model output must contain a
// cause, may contain one, or must explicitly abstain.
type GeneratedDisposition string

const (
	// GeneratedCauseRequired means disclosed evidence is sufficient for one
	// concrete generated root cause.
	GeneratedCauseRequired GeneratedDisposition = "required"
	// GeneratedCauseAllowed means a cause is useful when supported but is not
	// required for the case to pass.
	GeneratedCauseAllowed GeneratedDisposition = "allowed"
	// GeneratedMustAbstain means the disclosed evidence intentionally does not
	// support a concrete generated root cause.
	GeneratedMustAbstain GeneratedDisposition = "must_abstain"
)

// Corpus is a checked-in collection of expected diagnosis behavior.
type Corpus struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	Cases         []Case `json:"cases"`
	root          string
}

// Case describes one evidence input and its controlled expectations.
type Case struct {
	Name                   string                   `json:"name"`
	Evidence               string                   `json:"evidence"`
	Source                 string                   `json:"source,omitempty"`
	Tags                   []string                 `json:"tags"`
	AcceptedPrimaryCodes   []string                 `json:"accepted_primary_codes"`
	AllowedGeneratedCodes  []string                 `json:"allowed_generated_codes"`
	RequiredFindingCodes   []string                 `json:"required_finding_codes"`
	ForbiddenFindingCodes  []string                 `json:"forbidden_finding_codes"`
	RequiredActionCodes    []string                 `json:"required_action_codes"`
	ForbiddenActionCodes   []string                 `json:"forbidden_action_codes"`
	ExpectedRetry          diagnosis.RetryVerdict   `json:"expected_retry"`
	ExpectedExistingPolicy diagnosis.ExistingPolicy `json:"expected_existing_policy"`
	MinimumConfidence      int                      `json:"minimum_confidence"`
	MaximumConfidence      int                      `json:"maximum_confidence"`
	GeneratedExpectation   GeneratedExpectation     `json:"generated_expectation"`
}

// GeneratedExpectation describes semantic properties of accepted generated
// prose without prescribing one exact wording.
type GeneratedExpectation struct {
	Disposition       GeneratedDisposition `json:"disposition"`
	RequiredFacts     []ExpectedConcept    `json:"required_facts,omitempty"`
	RequiredRelations []ExpectedRelation   `json:"required_relations,omitempty"`
	ForbiddenClaims   []ExpectedConcept    `json:"forbidden_claims,omitempty"`
	MaximumCitations  int                  `json:"maximum_citations,omitempty"`
}

// ExpectedConcept is one incident fact with accepted wording alternatives.
type ExpectedConcept struct {
	Name         string   `json:"name"`
	Alternatives []string `json:"alternatives"`
}

// ExpectedRelation requires both sides of one causal path to remain visible.
type ExpectedRelation struct {
	Name    string   `json:"name"`
	Causes  []string `json:"causes"`
	Effects []string `json:"effects"`
}

// Result records one case without copying evidence or generated prose.
type Result struct {
	Name               string   `json:"name"`
	Iteration          int      `json:"iteration"`
	Tags               []string `json:"tags"`
	Passed             bool     `json:"passed"`
	PrimaryCode        string   `json:"primary_code,omitempty"`
	Retry              string   `json:"retry,omitempty"`
	ExistingPolicy     string   `json:"existing_policy,omitempty"`
	GeneratedCodes     []string `json:"generated_codes"`
	EvidenceID         string   `json:"evidence_id,omitempty"`
	AnalysisEvidenceID string   `json:"analysis_evidence_id,omitempty"`
	SourceContextUsed  bool     `json:"source_context_used"`
	ProviderInvoked    bool     `json:"provider_invoked"`
	ProposalAccepted   bool     `json:"proposal_accepted"`
	Violations         []string `json:"violations"`
	ReportFingerprint  string   `json:"report_fingerprint,omitempty"`
}

// Metrics are intentionally separated so fluent model output cannot hide a
// citation, action, or retry-policy regression.
type Metrics struct {
	PrimaryCodePrecision      float64 `json:"primary_code_precision"`
	UnsupportedClaimRate      float64 `json:"unsupported_claim_rate"`
	CitationValidity          float64 `json:"citation_validity"`
	SafeActionRate            float64 `json:"safe_action_rate"`
	RetryAdviceAccuracy       float64 `json:"retry_advice_accuracy"`
	DeterministicStability    float64 `json:"deterministic_stability"`
	ProviderFallbackRate      float64 `json:"provider_fallback_rate"`
	GeneratedSpecificity      float64 `json:"generated_specificity"`
	GeneratedSpecificityCases int     `json:"generated_specificity_cases"`
	ProposalAcceptance        float64 `json:"proposal_acceptance"`
	ProviderInvokedCases      int     `json:"provider_invoked_cases"`
	UsefulDiagnosisRate       float64 `json:"useful_diagnosis_rate"`
	RequiredCauseCases        int     `json:"required_cause_cases"`
	TaxonomyAccuracy          float64 `json:"taxonomy_accuracy"`
	TaxonomyCases             int     `json:"taxonomy_cases"`
	EntityPreservation        float64 `json:"entity_preservation"`
	ExpectedEntities          int     `json:"expected_entities"`
	CausalCompleteness        float64 `json:"causal_completeness"`
	ExpectedRelations         int     `json:"expected_relations"`
	AbstentionAccuracy        float64 `json:"abstention_accuracy"`
	AbstentionCases           int     `json:"abstention_cases"`
	ForbiddenClaimRate        float64 `json:"forbidden_claim_rate"`
	CitationEconomy           float64 `json:"citation_economy"`
	CitationEconomyCases      int     `json:"citation_economy_cases"`
	GeneratedConsistency      float64 `json:"generated_consistency"`
	ConsistencyComparisons    int     `json:"consistency_comparisons"`
	SourceContextCases        int     `json:"source_context_cases"`
}

// Summary is the versioned machine-readable evaluation result.
type Summary struct {
	Kind          string   `json:"kind"`
	SchemaVersion int      `json:"schema_version"`
	Mode          string   `json:"mode"`
	UniqueCases   int      `json:"unique_cases"`
	Repeats       int      `json:"repeats"`
	Cases         int      `json:"cases"`
	Passed        int      `json:"passed"`
	Metrics       Metrics  `json:"metrics"`
	Results       []Result `json:"results"`
}

// RunOptions controls bounded repeated evaluation.
type RunOptions struct {
	Repeats       int
	SourceContext *SourceContextOptions
}

// SourceContextOptions enables source snapshots for corpus cases with a
// checked-in source mapping.
type SourceContextOptions struct {
	Mode                diagnosis.SourceContextMode
	LinesBeforeAndAfter uint64
	MaximumBytes        uint64
}

// Load strictly decodes and validates a corpus. Evidence paths may reference
// sibling directories under the same testdata root but cannot escape it.
func Load(path string) (Corpus, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Corpus{}, fmt.Errorf("load evaluation corpus: resolve path: %w", err)
	}
	// #nosec G304 -- path is an explicit development-tool input and remains bounded.
	encoded, err := os.ReadFile(absolute)
	if err != nil {
		return Corpus{}, fmt.Errorf("load evaluation corpus: %w", err)
	}
	if len(encoded) > maximumCorpusBytes {
		return Corpus{}, errors.New("load evaluation corpus: manifest exceeds its byte limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var corpus Corpus
	if err := decoder.Decode(&corpus); err != nil {
		return Corpus{}, fmt.Errorf("load evaluation corpus: decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Corpus{}, errors.New("load evaluation corpus: trailing JSON value")
	}
	corpus.root = filepath.Dir(filepath.Dir(absolute))
	if err := validateCorpus(corpus, filepath.Dir(absolute)); err != nil {
		return Corpus{}, err
	}

	return corpus, nil
}

// Select returns the intersection of exact case names and tags. A case matches
// the tag filter when it contains at least one requested tag.
//
//nolint:gocognit // Selection validates both user filters and the resulting corpus contract together.
func Select(corpus Corpus, names, tags []string) (Corpus, error) {
	if err := validateCorpus(corpus, filepath.Join(corpus.root, "evaluation")); err != nil {
		return Corpus{}, err
	}
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if !validCode(name) {
			return Corpus{}, fmt.Errorf("select evaluation corpus: invalid case name %q", name)
		}
		nameSet[name] = struct{}{}
	}
	tagSet := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !validCode(tag) {
			return Corpus{}, fmt.Errorf("select evaluation corpus: invalid tag %q", tag)
		}
		tagSet[tag] = struct{}{}
	}
	selected := Corpus{Kind: corpus.Kind, SchemaVersion: corpus.SchemaVersion, root: corpus.root}
	for _, test := range corpus.Cases {
		if len(nameSet) != 0 {
			if _, ok := nameSet[test.Name]; !ok {
				continue
			}
		}
		if len(tagSet) != 0 && !containsTag(test.Tags, tagSet) {
			continue
		}
		selected.Cases = append(selected.Cases, test)
	}
	if len(nameSet) != 0 {
		for name := range nameSet {
			if !slices.ContainsFunc(selected.Cases, func(test Case) bool { return test.Name == name }) {
				return Corpus{}, fmt.Errorf("select evaluation corpus: case %q was not found or did not match the tag filter", name)
			}
		}
	}
	if len(selected.Cases) == 0 {
		return Corpus{}, errors.New("select evaluation corpus: no cases matched the requested filters")
	}

	return selected, nil
}

func containsTag(actual []string, requested map[string]struct{}) bool {
	for _, tag := range actual {
		if _, ok := requested[tag]; ok {
			return true
		}
	}

	return false
}

// Run evaluates a diagnostician and an independent deterministic stability
// baseline. The target may be deterministic or explicitly model-augmented.
func Run(ctx context.Context, corpus Corpus, target diagnosis.Diagnostician, mode string) (Summary, error) {
	return RunWithOptions(ctx, corpus, target, mode, RunOptions{Repeats: 1})
}

// RunWithOptions evaluates a corpus one or more times. Repetition remains
// sequential so a local model endpoint receives bounded, auditable load.
//
//nolint:cyclop,gocognit // The loop deliberately accumulates independent correctness and quality denominators.
func RunWithOptions(
	ctx context.Context,
	corpus Corpus,
	target diagnosis.Diagnostician,
	mode string,
	options RunOptions,
) (Summary, error) {
	if ctx == nil || target == nil || strings.TrimSpace(mode) == "" {
		return Summary{}, errors.New("run evaluation: context, diagnostician, and mode are required")
	}
	if options.Repeats < 1 || options.Repeats > 20 {
		return Summary{}, errors.New("run evaluation: repeat count must be between 1 and 20")
	}
	if options.SourceContext != nil && options.SourceContext.Mode != diagnosis.SourceContextLimited &&
		options.SourceContext.Mode != diagnosis.SourceContextFull {
		return Summary{}, errors.New("run evaluation: source context mode must be limited or full")
	}
	if err := validateCorpus(corpus, filepath.Join(corpus.root, "evaluation")); err != nil {
		return Summary{}, err
	}
	baseline, err := engine.New("evaluation", func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		Kind: "jobman.diagnosis_evaluation_result", SchemaVersion: 4, Mode: mode,
		UniqueCases: len(corpus.Cases), Repeats: options.Repeats,
		Cases:   len(corpus.Cases) * options.Repeats,
		Results: make([]Result, 0, len(corpus.Cases)*options.Repeats),
	}
	var primaryOK, citationsOK, retryOK, stable, fallback, generatedClaims, unsupportedClaims int
	var safeActions, actions int
	var sourceContextCases int
	var totals generatedCounters
	var consistencyMatches, consistencyComparisons int
	for _, test := range corpus.Cases {
		firstSignature := ""
		for iteration := 1; iteration <= options.Repeats; iteration++ {
			if err := ctx.Err(); err != nil {
				return Summary{}, fmt.Errorf("run evaluation: %w", err)
			}
			result, counters, err := runCase(ctx, corpus.root, test, target, baseline, options.SourceContext)
			result.Iteration = iteration
			if err != nil {
				result.Name = test.Name
				result.Iteration = iteration
				result.Tags = slices.Clone(test.Tags)
				if result.GeneratedCodes == nil {
					result.GeneratedCodes = []string{}
				}
				result.Violations = append(result.Violations, err.Error())
			}
			if result.Passed {
				summary.Passed++
			}
			primaryOK += counters.primaryOK
			citationsOK += counters.citationsOK
			retryOK += counters.retryOK
			stable += counters.stable
			fallback += counters.fallback
			actions += counters.actions
			safeActions += counters.safeActions
			generatedClaims += counters.generatedClaims
			unsupportedClaims += counters.unsupportedClaims
			if result.SourceContextUsed {
				sourceContextCases++
			}
			totals.add(counters.generated)
			if result.ProviderInvoked {
				signature := generatedSignature(result)
				if iteration == 1 {
					firstSignature = signature
				} else {
					consistencyComparisons++
					if signature == firstSignature {
						consistencyMatches++
					}
				}
			}
			summary.Results = append(summary.Results, result)
		}
	}
	summary.Metrics = Metrics{
		PrimaryCodePrecision:      ratio(primaryOK, summary.Cases),
		UnsupportedClaimRate:      zeroRatio(unsupportedClaims, generatedClaims),
		CitationValidity:          ratio(citationsOK, summary.Cases),
		SafeActionRate:            ratio(safeActions, actions),
		RetryAdviceAccuracy:       ratio(retryOK, summary.Cases),
		DeterministicStability:    ratio(stable, summary.Cases),
		ProviderFallbackRate:      ratio(fallback, summary.Cases),
		GeneratedSpecificity:      ratio(totals.usefulDiagnoses, totals.requiredCauseCases),
		GeneratedSpecificityCases: totals.requiredCauseCases,
		ProposalAcceptance:        ratio(totals.proposalsAccepted, totals.providerInvoked),
		ProviderInvokedCases:      totals.providerInvoked,
		UsefulDiagnosisRate:       ratio(totals.usefulDiagnoses, totals.requiredCauseCases),
		RequiredCauseCases:        totals.requiredCauseCases,
		TaxonomyAccuracy:          ratio(totals.taxonomyCorrect, totals.taxonomyCases),
		TaxonomyCases:             totals.taxonomyCases,
		EntityPreservation:        ratio(totals.entitiesMatched, totals.entitiesExpected),
		ExpectedEntities:          totals.entitiesExpected,
		CausalCompleteness:        ratio(totals.relationsMatched, totals.relationsExpected),
		ExpectedRelations:         totals.relationsExpected,
		AbstentionAccuracy:        ratio(totals.abstentionsCorrect, totals.abstentionCases),
		AbstentionCases:           totals.abstentionCases,
		ForbiddenClaimRate:        zeroRatio(totals.forbiddenClaims, totals.forbiddenChecks),
		CitationEconomy:           ratio(totals.economicalCitations, totals.citationEconomyCases),
		CitationEconomyCases:      totals.citationEconomyCases,
		GeneratedConsistency:      ratio(consistencyMatches, consistencyComparisons),
		ConsistencyComparisons:    consistencyComparisons,
		SourceContextCases:        sourceContextCases,
	}

	return summary, nil
}

type caseCounters struct {
	primaryOK, citationsOK, retryOK, stable, fallback        int
	actions, safeActions, generatedClaims, unsupportedClaims int
	generated                                                generatedCounters
}

type generatedCounters struct {
	providerInvoked, proposalsAccepted        int
	requiredCauseCases, usefulDiagnoses       int
	taxonomyCases, taxonomyCorrect            int
	entitiesExpected, entitiesMatched         int
	relationsExpected, relationsMatched       int
	abstentionCases, abstentionsCorrect       int
	forbiddenChecks, forbiddenClaims          int
	citationEconomyCases, economicalCitations int
}

func (totals *generatedCounters) add(current generatedCounters) {
	totals.providerInvoked += current.providerInvoked
	totals.proposalsAccepted += current.proposalsAccepted
	totals.requiredCauseCases += current.requiredCauseCases
	totals.usefulDiagnoses += current.usefulDiagnoses
	totals.taxonomyCases += current.taxonomyCases
	totals.taxonomyCorrect += current.taxonomyCorrect
	totals.entitiesExpected += current.entitiesExpected
	totals.entitiesMatched += current.entitiesMatched
	totals.relationsExpected += current.relationsExpected
	totals.relationsMatched += current.relationsMatched
	totals.abstentionCases += current.abstentionCases
	totals.abstentionsCorrect += current.abstentionsCorrect
	totals.forbiddenChecks += current.forbiddenChecks
	totals.forbiddenClaims += current.forbiddenClaims
	totals.citationEconomyCases += current.citationEconomyCases
	totals.economicalCitations += current.economicalCitations
}

//nolint:cyclop,gocognit // A corpus case intentionally evaluates all independent quality and safety metrics together.
func runCase(
	ctx context.Context,
	root string,
	test Case,
	target diagnosis.Diagnostician,
	baseline diagnosis.Diagnostician,
	sourceOptions *SourceContextOptions,
) (Result, caseCounters, error) {
	result := Result{
		Name: test.Name, Tags: slices.Clone(test.Tags),
		GeneratedCodes: []string{}, Violations: []string{},
	}
	var counters caseCounters
	path := filepath.Join(root, filepath.FromSlash(test.Evidence))
	// #nosec G304 -- validated corpus paths cannot escape the checked-in testdata root.
	encoded, err := os.ReadFile(path)
	if err != nil {
		return result, counters, fmt.Errorf("read evidence: %w", err)
	}
	core, err := diagnostic.Decode(bytes.NewReader(encoded), diagnostic.DecodeLimits{})
	if err != nil {
		return result, counters, fmt.Errorf("decode evidence: %w", err)
	}
	result.EvidenceID = core.EvidenceID
	evidence, err := enrichment.Collect(ctx, core)
	if err != nil {
		return result, counters, err
	}
	if sourceOptions != nil && test.Source != "" {
		sourcePath := filepath.Join(filepath.Dir(root), filepath.FromSlash(test.Source))
		source, sourceErr := sourcecontext.Collect(ctx, core, sourcecontext.Options{
			Mode: sourceOptions.Mode, File: sourcePath,
			LinesBeforeAndAfter: sourceOptions.LinesBeforeAndAfter,
			MaximumBytes:        sourceOptions.MaximumBytes,
		})
		if sourceErr != nil {
			return result, counters, fmt.Errorf("collect source context: %w", sourceErr)
		}
		evidence, err = diagnosis.SealFailureEvidenceWithContext(core, evidence.Enrichment, source)
		if err != nil {
			return result, counters, fmt.Errorf("attach source context: %w", err)
		}
		result.SourceContextUsed = true
	}
	result.AnalysisEvidenceID = evidence.AnalysisEvidenceID
	report, err := target.Diagnose(ctx, evidence)
	if err != nil {
		return result, counters, fmt.Errorf("diagnose: %w", err)
	}
	result.PrimaryCode = primaryCode(report)
	result.EvidenceID = report.CoreEvidenceID
	result.AnalysisEvidenceID = report.AnalysisEvidenceID
	result.ProviderInvoked = report.Disclosure.ProviderInvoked
	result.ProposalAccepted = report.Disclosure.GeneratedContentUsed || hasWarningCode(report, "generator_abstained")
	result.Retry = string(report.Retry.Verdict)
	result.ExistingPolicy = string(report.Retry.ExistingPolicy)
	result.ReportFingerprint = report.Fingerprints.Report
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err != nil {
		result.Violations = append(result.Violations, "report citations or provenance are invalid")
	} else {
		counters.citationsOK = 1
	}
	if slices.Contains(test.AcceptedPrimaryCodes, result.PrimaryCode) {
		counters.primaryOK = 1
	} else {
		result.Violations = append(result.Violations, "primary diagnosis code is outside the accepted set")
	}
	if report.Retry.Verdict == test.ExpectedRetry && report.Retry.ExistingPolicy == test.ExpectedExistingPolicy {
		counters.retryOK = 1
	} else {
		result.Violations = append(result.Violations, "retry verdict or existing policy differs from the expected state")
	}
	primary := findPrimary(report)
	if primary.Confidence.Score < test.MinimumConfidence || primary.Confidence.Score > test.MaximumConfidence {
		result.Violations = append(result.Violations, "primary confidence is outside the accepted bound")
	}
	findingCodes := make([]string, 0, len(report.Findings))
	generatedText := strings.Builder{}
	generatedFindings := make([]diagnosis.Finding, 0, 1)
	for _, finding := range report.Findings {
		findingCodes = append(findingCodes, finding.Code)
		if finding.Analyzer != "generator.proposal/1" {
			continue
		}
		counters.generatedClaims++
		generatedFindings = append(generatedFindings, finding)
		generatedText.WriteString(finding.Summary)
		generatedText.WriteByte(' ')
		generatedText.WriteString(finding.Explanation)
		generatedText.WriteByte(' ')
		result.GeneratedCodes = append(result.GeneratedCodes, finding.Code)
		if !slices.Contains(test.AllowedGeneratedCodes, finding.Code) {
			counters.unsupportedClaims++
			result.Violations = append(result.Violations, "generated diagnosis code is not labeled as supported for this case")
		}
	}
	if report.Disclosure.ProviderInvoked {
		counters.generated = evaluateGenerated(
			&result, test, generatedFindings, generatedText.String(), result.ProposalAccepted,
		)
	}
	checkCodes(&result, findingCodes, test.RequiredFindingCodes, test.ForbiddenFindingCodes, "finding")
	actionCodes := make([]string, 0, len(report.Actions))
	for _, action := range report.Actions {
		counters.actions++
		actionCodes = append(actionCodes, action.Code)
		if !action.SafeToAutomate && (action.Execution == diagnosis.ActionExecutionNone ||
			action.Execution == diagnosis.ActionExecutionReadOnly) {
			counters.safeActions++
		} else {
			result.Violations = append(result.Violations, "action escaped the read-only catalog")
		}
	}
	checkCodes(&result, actionCodes, test.RequiredActionCodes, test.ForbiddenActionCodes, "action")
	for _, warning := range report.Warnings {
		if warning.Code == "generator_failed" || warning.Code == "generator_proposal_invalid" ||
			warning.Code == "generator_provenance_invalid" {
			counters.fallback = 1
		}
	}
	first, firstErr := baseline.Diagnose(ctx, evidence)
	second, secondErr := baseline.Diagnose(ctx, evidence)
	if firstErr == nil && secondErr == nil && first.Fingerprints.Report == second.Fingerprints.Report {
		counters.stable = 1
	} else {
		result.Violations = append(result.Violations, "deterministic report fingerprint is unstable")
	}
	result.Passed = len(result.Violations) == 0

	return result, counters, nil
}

//nolint:cyclop,gocognit // Semantic facts, relations, abstention, taxonomy, and citation economy remain independently auditable.
func evaluateGenerated(
	result *Result,
	test Case,
	findings []diagnosis.Finding,
	text string,
	proposalAccepted bool,
) generatedCounters {
	quality := generatedCounters{providerInvoked: 1}
	if proposalAccepted {
		quality.proposalsAccepted = 1
	}
	expectation := test.GeneratedExpectation
	hasCause := len(findings) != 0
	taxonomyCorrect := hasCause
	for _, finding := range findings {
		if !slices.Contains(test.AllowedGeneratedCodes, finding.Code) {
			taxonomyCorrect = false
		}
	}
	if expectation.Disposition == GeneratedCauseRequired || hasCause {
		quality.taxonomyCases = 1
		if taxonomyCorrect {
			quality.taxonomyCorrect = 1
		}
	}

	shouldScoreContent := expectation.Disposition == GeneratedCauseRequired || hasCause
	contentOK := true
	if shouldScoreContent {
		for _, fact := range expectation.RequiredFacts {
			quality.entitiesExpected++
			if matchesAlternatives(text, fact.Alternatives) {
				quality.entitiesMatched++
			} else {
				contentOK = false
				result.Violations = append(result.Violations, "generated diagnosis omitted required fact: "+fact.Name)
			}
		}
		for _, relation := range expectation.RequiredRelations {
			quality.relationsExpected++
			if matchesAlternatives(text, relation.Causes) && matchesAlternatives(text, relation.Effects) {
				quality.relationsMatched++
			} else {
				contentOK = false
				result.Violations = append(result.Violations, "generated diagnosis omitted causal relationship: "+relation.Name)
			}
		}
		for _, forbidden := range expectation.ForbiddenClaims {
			quality.forbiddenChecks++
			if matchesAlternatives(text, forbidden.Alternatives) {
				quality.forbiddenClaims++
				contentOK = false
				result.Violations = append(result.Violations, "generated diagnosis included forbidden claim: "+forbidden.Name)
			}
		}
		if hasCause && expectation.MaximumCitations > 0 {
			quality.citationEconomyCases++
			economical := true
			for _, finding := range findings {
				if len(finding.SupportingEvidence) > expectation.MaximumCitations {
					economical = false
					break
				}
			}
			if economical {
				quality.economicalCitations = 1
			} else {
				contentOK = false
				result.Violations = append(result.Violations, "generated diagnosis cited more evidence than the case permits")
			}
		}
	}

	switch expectation.Disposition {
	case GeneratedCauseRequired:
		quality.requiredCauseCases = 1
		if !proposalAccepted {
			contentOK = false
			result.Violations = append(result.Violations, "the provider did not return an accepted proposal")
		}
		if !hasCause {
			contentOK = false
			result.Violations = append(result.Violations, "a specific generated cause is required for this case")
		}
		if contentOK && taxonomyCorrect {
			quality.usefulDiagnoses = 1
		}
	case GeneratedMustAbstain:
		quality.abstentionCases = 1
		if proposalAccepted && !hasCause {
			quality.abstentionsCorrect = 1
		} else {
			result.Violations = append(result.Violations, "the model must return an accepted abstention for this case")
		}
	case GeneratedCauseAllowed:
		// A supported generated cause was scored above; an abstention is also a
		// valid outcome and intentionally adds no required-cause denominator.
	}

	return quality
}

func matchesAlternatives(text string, alternatives []string) bool {
	text = strings.ToLower(text)
	for _, value := range alternatives {
		if strings.Contains(text, strings.ToLower(value)) {
			return true
		}
	}

	return false
}

func generatedSignature(result Result) string {
	return fmt.Sprintf("%t:%s", result.ProposalAccepted, strings.Join(result.GeneratedCodes, ","))
}

func hasWarningCode(report diagnosis.Report, code string) bool {
	return slices.ContainsFunc(report.Warnings, func(warning diagnosis.Warning) bool { return warning.Code == code })
}

//nolint:cyclop // Corpus validation checks the complete checked-in expectation contract at one boundary.
func validateCorpus(corpus Corpus, manifestDirectory string) error {
	if corpus.Kind != Kind || corpus.SchemaVersion != SchemaVersion || len(corpus.Cases) == 0 ||
		len(corpus.Cases) > maximumCases {
		return errors.New("validate evaluation corpus: unsupported header or case count")
	}
	root := corpus.root
	if root == "" {
		root = filepath.Dir(manifestDirectory)
	}
	rootWithSeparator := filepath.Clean(root) + string(os.PathSeparator)
	sourceRoot := filepath.Dir(root)
	sourceRootWithSeparator := filepath.Clean(sourceRoot) + string(os.PathSeparator)
	prior := ""
	for _, test := range corpus.Cases {
		path := filepath.Clean(filepath.Join(root, filepath.FromSlash(test.Evidence)))
		sourceValid := true
		if test.Source != "" {
			sourcePath := filepath.Clean(filepath.Join(sourceRoot, filepath.FromSlash(test.Source)))
			sourceValid = !filepath.IsAbs(test.Source) && strings.HasPrefix(sourcePath, sourceRootWithSeparator)
		}
		if test.Name <= prior || !validCode(test.Name) || filepath.IsAbs(test.Evidence) ||
			!strings.HasPrefix(path, rootWithSeparator) || !sourceValid || len(test.AcceptedPrimaryCodes) == 0 ||
			!validLabels(test.Tags) ||
			test.MinimumConfidence < 0 || test.MaximumConfidence > 100 ||
			test.MinimumConfidence > test.MaximumConfidence || !validExpectedCodes(test) ||
			!validGeneratedExpectations(test) {
			return fmt.Errorf("validate evaluation corpus: invalid case %q", test.Name)
		}
		prior = test.Name
	}

	return nil
}

//nolint:cyclop // The manifest boundary validates every bounded semantic expectation in one audit point.
func validGeneratedExpectations(test Case) bool {
	expectation := test.GeneratedExpectation
	if expectation.Disposition != GeneratedCauseRequired &&
		expectation.Disposition != GeneratedCauseAllowed &&
		expectation.Disposition != GeneratedMustAbstain {
		return false
	}
	if expectation.MaximumCitations < 0 || expectation.MaximumCitations > 8 ||
		len(expectation.RequiredFacts) > 16 || len(expectation.RequiredRelations) > 8 ||
		len(expectation.ForbiddenClaims) > 16 {
		return false
	}
	if expectation.Disposition == GeneratedCauseRequired &&
		(len(test.AllowedGeneratedCodes) == 0 || len(expectation.RequiredFacts) == 0) {
		return false
	}
	if expectation.Disposition == GeneratedMustAbstain &&
		(len(test.AllowedGeneratedCodes) != 0 || len(expectation.RequiredFacts) != 0 ||
			len(expectation.RequiredRelations) != 0 || expectation.MaximumCitations != 0) {
		return false
	}
	for _, concept := range append(slices.Clone(expectation.RequiredFacts), expectation.ForbiddenClaims...) {
		if !validExpectedConcept(concept) {
			return false
		}
	}
	for _, relation := range expectation.RequiredRelations {
		if !validExpectationName(relation.Name) || !validAlternatives(relation.Causes) ||
			!validAlternatives(relation.Effects) {
			return false
		}
	}

	return true
}

func validExpectedConcept(concept ExpectedConcept) bool {
	return validExpectationName(concept.Name) && validAlternatives(concept.Alternatives)
}

func validExpectationName(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 160 && !strings.ContainsAny(value, "\r\n\x00")
}

func validAlternatives(values []string) bool {
	if len(values) == 0 || len(values) > 16 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
			return false
		}
	}

	return true
}

func validLabels(values []string) bool {
	if len(values) == 0 || !slices.IsSorted(values) || hasDuplicates(values) {
		return false
	}
	for _, value := range values {
		if !validCode(value) {
			return false
		}
	}

	return true
}

func validExpectedCodes(test Case) bool {
	sets := [][]string{
		test.AcceptedPrimaryCodes, test.AllowedGeneratedCodes, test.RequiredFindingCodes,
		test.ForbiddenFindingCodes, test.RequiredActionCodes, test.ForbiddenActionCodes,
	}
	for _, values := range sets {
		if !slices.IsSorted(values) || hasDuplicates(values) {
			return false
		}
		for _, value := range values {
			if !validCode(value) {
				return false
			}
		}
	}
	return test.ExpectedRetry != "" && test.ExpectedExistingPolicy != ""
}

func checkCodes(result *Result, actual, required, forbidden []string, kind string) {
	for _, code := range required {
		if !slices.Contains(actual, code) {
			result.Violations = append(result.Violations, "required "+kind+" code is missing: "+code)
		}
	}
	for _, code := range forbidden {
		if slices.Contains(actual, code) {
			result.Violations = append(result.Violations, "forbidden "+kind+" code is present: "+code)
		}
	}
}

func primaryCode(report diagnosis.Report) string { return findPrimary(report).Code }

func findPrimary(report diagnosis.Report) diagnosis.Finding {
	for _, finding := range report.Findings {
		if finding.ID == report.PrimaryFindingID {
			return finding
		}
	}

	return diagnosis.Finding{}
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}

	return float64(numerator) / float64(denominator)
}

func zeroRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}

	return float64(numerator) / float64(denominator)
}

func validCode(value string) bool {
	if value == "" || len(value) > 160 {
		return false
	}
	for _, character := range value {
		if character != '.' && character != '_' && character != '-' &&
			(character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}

	return true
}

func hasDuplicates(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return true
		}
	}

	return false
}
