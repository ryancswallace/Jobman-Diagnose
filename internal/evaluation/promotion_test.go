package evaluation

import (
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

func containsPromotionViolation(violations []string, substring string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, substring) {
			return true
		}
	}

	return false
}
