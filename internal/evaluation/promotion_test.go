package evaluation

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckedInPromotionPolicyAcceptsRepresentativePerfectRun(t *testing.T) {
	t.Parallel()

	policy, err := LoadPromotionPolicy(filepath.Join("..", "..", "testdata", "evaluation", "promotion-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if policy.MinimumSourceContextCases != 87 || policy.RequiredTagCases["context.source"] != 29 {
		t.Fatalf("source-context release thresholds = %d / %d", policy.MinimumSourceContextCases, policy.RequiredTagCases["context.source"])
	}
	corpus, err := Load(filepath.Join("..", "..", "testdata", "evaluation", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	results := make([]Result, 0, len(corpus.Cases))
	for _, test := range corpus.Cases {
		results = append(results, Result{Name: test.Name, Tags: test.Tags})
	}
	summary := Summary{
		Mode: "live:release", UniqueCases: len(corpus.Cases), Repeats: 3,
		Cases: len(corpus.Cases) * 3, Passed: len(corpus.Cases) * 3, Results: results,
		Metrics: Metrics{
			PrimaryCodePrecision: 1, CitationValidity: 1, SafeActionRate: 1, RetryAdviceAccuracy: 1,
			DeterministicStability: 1, GeneratedSpecificity: 1, ProposalAcceptance: 1,
			ProviderInvokedCases: len(corpus.Cases) * 3, UsefulDiagnosisRate: 1, RequiredCauseCases: 165,
			TaxonomyAccuracy: 1, TaxonomyCases: 165, EntityPreservation: 1, ExpectedEntities: 360,
			CausalCompleteness: 1, ExpectedRelations: 21, AbstentionAccuracy: 1, AbstentionCases: 51,
			CitationEconomy: 1, CitationEconomyCases: 165,
			GeneratedConsistency: 1, ConsistencyComparisons: len(corpus.Cases) * 2,
			SourceContextCases: 87,
		},
	}
	assessment := AssessPromotion(policy, summary)
	if !assessment.Passed || len(assessment.Violations) != 0 {
		t.Fatalf("assessment = %#v", assessment)
	}

	summary.Repeats = 1
	summary.Metrics.ProviderFallbackRate = 0.01
	assessment = AssessPromotion(policy, summary)
	if assessment.Passed || !containsPromotionViolation(assessment.Violations, "repeats 1") ||
		!containsPromotionViolation(assessment.Violations, "provider_fallback_rate") {
		t.Fatalf("failing assessment = %#v", assessment)
	}
}

func TestLoadPromotionPolicyRejectsUnknownAndInvalidFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	unknown := filepath.Join(root, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"kind":"jobman.diagnosis_evaluation_promotion_policy","schema_version":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPromotionPolicy(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadPromotionPolicy(unknown) error = %v", err)
	}

	minimum := 0.9
	policy := PromotionPolicy{
		Kind: PromotionPolicyKind, SchemaVersion: PromotionPolicySchemaVersion,
		MinimumUniqueCases: 1, MinimumRepeats: 3, MinimumProviderInvokedCases: 3,
		MinimumConsistencyChecks: 2, RequiredTagCases: map[string]int{"language.go": 1},
		Metrics: map[string]MetricThreshold{"unknown": {Minimum: &minimum}},
	}
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "unknown metric") {
		t.Fatalf("Validate(unknown metric) error = %v", err)
	}
}

func TestLoadPromotionPolicyRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := LoadPromotionPolicy(filepath.Join(root, "missing.json")); err == nil || !strings.Contains(err.Error(), "load evaluation promotion policy") {
		t.Fatalf("LoadPromotionPolicy(missing) error = %v", err)
	}

	oversized := filepath.Join(root, "oversized.json")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte{' '}, maximumPromotionPolicyBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPromotionPolicy(oversized); err == nil || !strings.Contains(err.Error(), "exceeds its byte limit") {
		t.Fatalf("LoadPromotionPolicy(oversized) error = %v", err)
	}

	trailing := filepath.Join(root, "trailing.json")
	if err := os.WriteFile(trailing, []byte(`{} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPromotionPolicy(trailing); err == nil || !strings.Contains(err.Error(), "trailing JSON value") {
		t.Fatalf("LoadPromotionPolicy(trailing) error = %v", err)
	}

	invalid := filepath.Join(root, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPromotionPolicy(invalid); err == nil || !strings.Contains(err.Error(), "unsupported kind or schema version") {
		t.Fatalf("LoadPromotionPolicy(invalid) error = %v", err)
	}
}

func TestPromotionPolicyValidateRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*PromotionPolicy)
		wantError string
	}{
		{
			name: "schema",
			mutate: func(policy *PromotionPolicy) {
				policy.SchemaVersion++
			},
			wantError: "unsupported kind or schema version",
		},
		{
			name: "workload",
			mutate: func(policy *PromotionPolicy) {
				policy.MinimumRepeats = 1
			},
			wantError: "invalid minimum workload",
		},
		{
			name: "empty tags",
			mutate: func(policy *PromotionPolicy) {
				policy.RequiredTagCases = nil
			},
			wantError: "must not be empty",
		},
		{
			name: "invalid tag",
			mutate: func(policy *PromotionPolicy) {
				policy.RequiredTagCases = map[string]int{"Invalid Tag": 1}
			},
			wantError: "invalid required tag threshold",
		},
		{
			name: "missing metric bound",
			mutate: func(policy *PromotionPolicy) {
				policy.Metrics = map[string]MetricThreshold{"citation_validity": {}}
			},
			wantError: "has no bound",
		},
		{
			name: "negative denominator",
			mutate: func(policy *PromotionPolicy) {
				minimum := 0.9
				policy.Metrics = map[string]MetricThreshold{
					"citation_validity": {Minimum: &minimum, MinimumDenominator: -1},
				}
			},
			wantError: "invalid minimum denominator",
		},
		{
			name: "non-finite bound",
			mutate: func(policy *PromotionPolicy) {
				minimum := math.NaN()
				policy.Metrics = map[string]MetricThreshold{"citation_validity": {Minimum: &minimum}}
			},
			wantError: "bound outside [0,1]",
		},
		{
			name: "inverted bounds",
			mutate: func(policy *PromotionPolicy) {
				minimum, maximum := 0.9, 0.8
				policy.Metrics = map[string]MetricThreshold{
					"citation_validity": {Minimum: &minimum, Maximum: &maximum},
				}
			},
			wantError: "inverted bounds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			policy := validPromotionPolicy()
			test.mutate(&policy)
			if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestAssessPromotionReportsEveryViolationClass(t *testing.T) {
	t.Parallel()

	policy := validPromotionPolicy()
	minimum, maximum := 0.9, 0.0
	policy.Metrics = map[string]MetricThreshold{
		"primary_code_precision": {Minimum: &minimum},
		"provider_fallback_rate": {Maximum: &maximum},
		"useful_diagnosis_rate": {
			Minimum:            &minimum,
			MinimumDenominator: 3,
		},
	}
	summary := Summary{
		Mode: "deterministic", UniqueCases: 1, Repeats: 2, Cases: 2,
		Results: []Result{{Name: "only-case", Tags: []string{"context.source"}}},
		Metrics: Metrics{
			ProviderInvokedCases: 1, ConsistencyComparisons: 1, SourceContextCases: 1,
			PrimaryCodePrecision: 0.8, ProviderFallbackRate: 0.5,
			UsefulDiagnosisRate: 0.8, RequiredCauseCases: 1,
		},
	}
	assessment := AssessPromotion(policy, summary)
	if assessment.Passed {
		t.Fatalf("AssessPromotion() passed with violations: %#v", assessment)
	}
	for _, want := range []string{
		"requires a live evaluation",
		"unique cases 1 is below 2",
		"repeats 2 is below 3",
		"provider-invoked cases 1 is below 2",
		"consistency comparisons 1 is below 2",
		"source-context cases 1 is below 2",
		"tag context.source has 1 unique cases, below 2",
		"metric primary_code_precision 0.800000 is below 0.900000",
		"metric provider_fallback_rate 0.500000 exceeds 0.000000",
		"metric useful_diagnosis_rate denominator 1 is below 3",
	} {
		if !containsPromotionViolation(assessment.Violations, want) {
			t.Errorf("AssessPromotion() violations %q do not contain %q", assessment.Violations, want)
		}
	}

	invalid := validPromotionPolicy()
	invalid.Kind = "invalid"
	assessment = AssessPromotion(invalid, Summary{})
	if assessment.Passed || !containsPromotionViolation(assessment.Violations, "invalid policy") {
		t.Fatalf("AssessPromotion(invalid) = %#v", assessment)
	}
}

func validPromotionPolicy() PromotionPolicy {
	minimum := 0.9

	return PromotionPolicy{
		Kind: PromotionPolicyKind, SchemaVersion: PromotionPolicySchemaVersion,
		MinimumUniqueCases: 2, MinimumRepeats: 3, MinimumProviderInvokedCases: 2,
		MinimumConsistencyChecks: 2, MinimumSourceContextCases: 2,
		RequiredTagCases: map[string]int{"context.source": 2},
		Metrics: map[string]MetricThreshold{
			"citation_validity": {Minimum: &minimum},
		},
	}
}

func containsPromotionViolation(violations []string, substring string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, substring) {
			return true
		}
	}

	return false
}
