package evaluation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
)

const (
	// PromotionPolicyKind identifies a checked-in release evaluation policy.
	PromotionPolicyKind = "jobman.diagnosis_evaluation_promotion_policy"
	// PromotionPolicySchemaVersion is the newest promotion policy schema.
	PromotionPolicySchemaVersion = 1
	// PromotionAssessmentKind identifies a policy result embedded in an evaluation report.
	PromotionAssessmentKind = "jobman.diagnosis_evaluation_promotion_assessment"
	// PromotionAssessmentSchemaVersion is the newest promotion assessment schema.
	PromotionAssessmentSchemaVersion = 1

	maximumPromotionPolicyBytes = 256 * 1024
)

// MetricThreshold defines one inclusive release threshold. A nil bound is not
// enforced. MinimumDenominator prevents a perfect ratio over too little data
// from satisfying a release policy.
type MetricThreshold struct {
	Minimum            *float64 `json:"minimum,omitempty"`
	Maximum            *float64 `json:"maximum,omitempty"`
	MinimumDenominator int      `json:"minimum_denominator,omitempty"`
}

// PromotionPolicy defines the minimum representative workload and quality
// ratios required before an evaluated model/runtime may guide a release.
type PromotionPolicy struct {
	Kind                        string                     `json:"kind"`
	SchemaVersion               int                        `json:"schema_version"`
	MinimumUniqueCases          int                        `json:"minimum_unique_cases"`
	MinimumRepeats              int                        `json:"minimum_repeats"`
	MinimumProviderInvokedCases int                        `json:"minimum_provider_invoked_cases"`
	MinimumConsistencyChecks    int                        `json:"minimum_consistency_comparisons"`
	MinimumSourceContextCases   int                        `json:"minimum_source_context_cases"`
	RequiredTagCases            map[string]int             `json:"required_tag_cases"`
	Metrics                     map[string]MetricThreshold `json:"metrics"`
}

// PromotionAssessment records whether one evaluation result satisfies its
// checked-in release policy without copying the entire policy into the report.
type PromotionAssessment struct {
	Kind                string   `json:"kind"`
	SchemaVersion       int      `json:"schema_version"`
	PolicyKind          string   `json:"policy_kind"`
	PolicySchemaVersion int      `json:"policy_schema_version"`
	Passed              bool     `json:"passed"`
	Violations          []string `json:"violations"`
}

type metricObservation struct {
	value       float64
	denominator int
}

// LoadPromotionPolicy strictly decodes and validates an explicit policy file.
func LoadPromotionPolicy(path string) (PromotionPolicy, error) {
	encoded, err := os.ReadFile(path) // #nosec G304 -- path is an explicit development-tool input.
	if err != nil {
		return PromotionPolicy{}, fmt.Errorf("load evaluation promotion policy: %w", err)
	}
	if len(encoded) > maximumPromotionPolicyBytes {
		return PromotionPolicy{}, errors.New("load evaluation promotion policy: policy exceeds its byte limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var policy PromotionPolicy
	if err := decoder.Decode(&policy); err != nil {
		return PromotionPolicy{}, fmt.Errorf("load evaluation promotion policy: decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return PromotionPolicy{}, errors.New("load evaluation promotion policy: trailing JSON value")
	}
	if err := policy.Validate(); err != nil {
		return PromotionPolicy{}, fmt.Errorf("load evaluation promotion policy: %w", err)
	}

	return policy, nil
}

// Validate checks the bounded promotion-policy contract.
//
//nolint:cyclop,gocognit // Policy validation keeps every bounded field and metric rule visible in one contract.
func (policy PromotionPolicy) Validate() error {
	if policy.Kind != PromotionPolicyKind || policy.SchemaVersion != PromotionPolicySchemaVersion {
		return errors.New("unsupported kind or schema version")
	}
	if policy.MinimumUniqueCases < 1 || policy.MinimumUniqueCases > maximumCases ||
		policy.MinimumRepeats < 2 || policy.MinimumRepeats > 20 ||
		policy.MinimumProviderInvokedCases < 1 || policy.MinimumConsistencyChecks < 1 ||
		policy.MinimumSourceContextCases < 0 {
		return errors.New("invalid minimum workload")
	}
	if len(policy.RequiredTagCases) == 0 || len(policy.Metrics) == 0 {
		return errors.New("required tag cases and metric thresholds must not be empty")
	}
	for tag, minimum := range policy.RequiredTagCases {
		if !validCode(tag) || minimum < 1 || minimum > maximumCases {
			return fmt.Errorf("invalid required tag threshold %q", tag)
		}
	}
	for name, threshold := range policy.Metrics {
		if _, ok := metricObservationFor(name, Summary{}); !ok {
			return fmt.Errorf("unknown metric %q", name)
		}
		if threshold.Minimum == nil && threshold.Maximum == nil {
			return fmt.Errorf("metric %q has no bound", name)
		}
		if threshold.MinimumDenominator < 0 {
			return fmt.Errorf("metric %q has an invalid minimum denominator", name)
		}
		for _, bound := range []*float64{threshold.Minimum, threshold.Maximum} {
			if bound != nil && (math.IsNaN(*bound) || math.IsInf(*bound, 0) || *bound < 0 || *bound > 1) {
				return fmt.Errorf("metric %q has a bound outside [0,1]", name)
			}
		}
		if threshold.Minimum != nil && threshold.Maximum != nil && *threshold.Minimum > *threshold.Maximum {
			return fmt.Errorf("metric %q has inverted bounds", name)
		}
	}

	return nil
}

// AssessPromotion applies a validated release policy to a completed result.
//
//nolint:cyclop // Independent release-gate violations are accumulated rather than failing opaquely at the first one.
func AssessPromotion(policy PromotionPolicy, summary Summary) PromotionAssessment {
	assessment := PromotionAssessment{
		Kind: PromotionAssessmentKind, SchemaVersion: PromotionAssessmentSchemaVersion,
		PolicyKind: policy.Kind, PolicySchemaVersion: policy.SchemaVersion, Violations: []string{},
	}
	if err := policy.Validate(); err != nil {
		assessment.Violations = append(assessment.Violations, "invalid policy: "+err.Error())
		return assessment
	}
	if !strings.HasPrefix(summary.Mode, "live:") {
		assessment.Violations = append(assessment.Violations, "promotion requires a live evaluation")
	}
	if summary.UniqueCases < policy.MinimumUniqueCases {
		assessment.Violations = append(assessment.Violations, fmt.Sprintf(
			"unique cases %d is below %d", summary.UniqueCases, policy.MinimumUniqueCases,
		))
	}
	if summary.Repeats < policy.MinimumRepeats {
		assessment.Violations = append(assessment.Violations, fmt.Sprintf(
			"repeats %d is below %d", summary.Repeats, policy.MinimumRepeats,
		))
	}
	if summary.Metrics.ProviderInvokedCases < policy.MinimumProviderInvokedCases {
		assessment.Violations = append(assessment.Violations, fmt.Sprintf(
			"provider-invoked cases %d is below %d",
			summary.Metrics.ProviderInvokedCases, policy.MinimumProviderInvokedCases,
		))
	}
	if summary.Metrics.ConsistencyComparisons < policy.MinimumConsistencyChecks {
		assessment.Violations = append(assessment.Violations, fmt.Sprintf(
			"consistency comparisons %d is below %d",
			summary.Metrics.ConsistencyComparisons, policy.MinimumConsistencyChecks,
		))
	}
	if summary.Metrics.SourceContextCases < policy.MinimumSourceContextCases {
		assessment.Violations = append(assessment.Violations, fmt.Sprintf(
			"source-context cases %d is below %d",
			summary.Metrics.SourceContextCases, policy.MinimumSourceContextCases,
		))
	}
	for _, tag := range sortedKeys(policy.RequiredTagCases) {
		actual := uniqueTagCases(summary.Results, tag)
		if actual < policy.RequiredTagCases[tag] {
			assessment.Violations = append(assessment.Violations, fmt.Sprintf(
				"tag %s has %d unique cases, below %d", tag, actual, policy.RequiredTagCases[tag],
			))
		}
	}
	for _, name := range sortedKeys(policy.Metrics) {
		threshold := policy.Metrics[name]
		observation, _ := metricObservationFor(name, summary)
		if observation.denominator < threshold.MinimumDenominator {
			assessment.Violations = append(assessment.Violations, fmt.Sprintf(
				"metric %s denominator %d is below %d", name, observation.denominator, threshold.MinimumDenominator,
			))
			continue
		}
		if threshold.Minimum != nil && observation.value < *threshold.Minimum {
			assessment.Violations = append(assessment.Violations, fmt.Sprintf(
				"metric %s %.6f is below %.6f", name, observation.value, *threshold.Minimum,
			))
		}
		if threshold.Maximum != nil && observation.value > *threshold.Maximum {
			assessment.Violations = append(assessment.Violations, fmt.Sprintf(
				"metric %s %.6f exceeds %.6f", name, observation.value, *threshold.Maximum,
			))
		}
	}
	assessment.Passed = len(assessment.Violations) == 0

	return assessment
}

func metricObservationFor(name string, summary Summary) (metricObservation, bool) {
	metrics := summary.Metrics
	values := map[string]metricObservation{
		"abstention_accuracy":     {metrics.AbstentionAccuracy, metrics.AbstentionCases},
		"causal_completeness":     {metrics.CausalCompleteness, metrics.ExpectedRelations},
		"citation_economy":        {metrics.CitationEconomy, metrics.CitationEconomyCases},
		"citation_validity":       {metrics.CitationValidity, summary.Cases},
		"deterministic_stability": {metrics.DeterministicStability, summary.Cases},
		"entity_preservation":     {metrics.EntityPreservation, metrics.ExpectedEntities},
		"forbidden_claim_rate":    {metrics.ForbiddenClaimRate, metrics.AbstentionCases},
		"generated_consistency":   {metrics.GeneratedConsistency, metrics.ConsistencyComparisons},
		"primary_code_precision":  {metrics.PrimaryCodePrecision, summary.Cases},
		"proposal_acceptance":     {metrics.ProposalAcceptance, metrics.ProviderInvokedCases},
		"provider_fallback_rate":  {metrics.ProviderFallbackRate, summary.Cases},
		"retry_advice_accuracy":   {metrics.RetryAdviceAccuracy, summary.Cases},
		"safe_action_rate":        {metrics.SafeActionRate, summary.Cases},
		"taxonomy_accuracy":       {metrics.TaxonomyAccuracy, metrics.TaxonomyCases},
		"unsupported_claim_rate":  {metrics.UnsupportedClaimRate, 0},
		"useful_diagnosis_rate":   {metrics.UsefulDiagnosisRate, metrics.RequiredCauseCases},
	}
	value, ok := values[name]

	return value, ok
}

func uniqueTagCases(results []Result, tag string) int {
	names := make(map[string]struct{})
	for _, result := range results {
		if slices.Contains(result.Tags, tag) {
			names[result.Name] = struct{}{}
		}
	}

	return len(names)
}

func sortedKeys[Value any](values map[string]Value) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	slices.Sort(result)

	return result
}
