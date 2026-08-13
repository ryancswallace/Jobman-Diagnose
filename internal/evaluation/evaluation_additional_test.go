package evaluation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

type diagnosticianFunc func(context.Context, diagnosis.FailureEvidence) (diagnosis.Report, error)

func (function diagnosticianFunc) Diagnose(
	ctx context.Context,
	evidence diagnosis.FailureEvidence,
) (diagnosis.Report, error) {
	return function(ctx, evidence)
}

func TestLoadRejectsUnreadableOrMalformedCorpus(t *testing.T) {
	t.Parallel()

	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load(missing) error = %v", err)
	}
	tests := []struct {
		name    string
		encoded []byte
	}{
		{name: "malformed", encoded: []byte(`{"kind":`)},
		{name: "unknown field", encoded: []byte(`{"kind":"` + Kind + `","schema_version":4,"cases":[],"extra":true}`)},
		{name: "trailing value", encoded: []byte(`{"kind":"` + Kind + `","schema_version":4,"cases":[]} {}`)},
		{name: "invalid header", encoded: []byte(`{"kind":"wrong","schema_version":4,"cases":[]}`)},
		{name: "oversized", encoded: bytes.Repeat([]byte(" "), maximumCorpusBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "evaluation", "manifest.json")
			if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, test.encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestValidateCorpusRejectsInvalidExpectationContracts(t *testing.T) {
	t.Parallel()

	loaded := loadCorpus(t)
	loaded.Cases = loaded.Cases[:1]
	tests := []struct {
		name   string
		mutate func(*Corpus)
	}{
		{name: "kind", mutate: func(corpus *Corpus) { corpus.Kind = "wrong" }},
		{name: "version", mutate: func(corpus *Corpus) { corpus.SchemaVersion++ }},
		{name: "no cases", mutate: func(corpus *Corpus) { corpus.Cases = nil }},
		{name: "too many cases", mutate: func(corpus *Corpus) {
			corpus.Cases = make([]Case, maximumCases+1)
		}},
		{name: "name", mutate: func(corpus *Corpus) { corpus.Cases[0].Name = "Invalid Name" }},
		{name: "absolute evidence", mutate: func(corpus *Corpus) {
			root := filepath.VolumeName(corpus.root) + string(os.PathSeparator)
			corpus.Cases[0].Evidence = filepath.Join(root, "outside.json")
		}},
		{name: "escaping evidence", mutate: func(corpus *Corpus) { corpus.Cases[0].Evidence = "../outside.json" }},
		{name: "absolute source", mutate: func(corpus *Corpus) {
			root := filepath.VolumeName(corpus.root) + string(os.PathSeparator)
			corpus.Cases[0].Source = filepath.Join(root, "outside.py")
		}},
		{name: "escaping source", mutate: func(corpus *Corpus) { corpus.Cases[0].Source = "../outside.py" }},
		{name: "no primary codes", mutate: func(corpus *Corpus) { corpus.Cases[0].AcceptedPrimaryCodes = nil }},
		{name: "negative confidence", mutate: func(corpus *Corpus) { corpus.Cases[0].MinimumConfidence = -1 }},
		{name: "excess confidence", mutate: func(corpus *Corpus) { corpus.Cases[0].MaximumConfidence = 101 }},
		{name: "reversed confidence", mutate: func(corpus *Corpus) {
			corpus.Cases[0].MinimumConfidence, corpus.Cases[0].MaximumConfidence = 80, 20
		}},
		{name: "unsorted codes", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AcceptedPrimaryCodes = []string{"z.code", "a.code"}
		}},
		{name: "duplicate codes", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AcceptedPrimaryCodes = []string{"same", "same"}
		}},
		{name: "invalid code", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AcceptedPrimaryCodes = []string{"UPPER"}
		}},
		{name: "missing tags", mutate: func(corpus *Corpus) { corpus.Cases[0].Tags = nil }},
		{name: "unsorted tags", mutate: func(corpus *Corpus) {
			corpus.Cases[0].Tags = []string{"z.tag", "a.tag"}
		}},
		{name: "missing retry", mutate: func(corpus *Corpus) { corpus.Cases[0].ExpectedRetry = "" }},
		{name: "missing policy", mutate: func(corpus *Corpus) { corpus.Cases[0].ExpectedExistingPolicy = "" }},
		{name: "unknown generated disposition", mutate: func(corpus *Corpus) {
			corpus.Cases[0].GeneratedExpectation.Disposition = "unknown"
		}},
		{name: "required cause without allowed code", mutate: func(corpus *Corpus) {
			corpus.Cases[0].GeneratedExpectation = GeneratedExpectation{
				Disposition:   GeneratedCauseRequired,
				RequiredFacts: []ExpectedConcept{{Name: "cause", Alternatives: []string{"specific cause"}}},
			}
		}},
		{name: "required cause without facts", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AllowedGeneratedCodes = []string{"generated.application_defect"}
			corpus.Cases[0].GeneratedExpectation = GeneratedExpectation{Disposition: GeneratedCauseRequired}
		}},
		{name: "empty generated fact", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AllowedGeneratedCodes = []string{"generated.application_defect"}
			corpus.Cases[0].GeneratedExpectation = GeneratedExpectation{
				Disposition:   GeneratedCauseRequired,
				RequiredFacts: []ExpectedConcept{{Name: "cause", Alternatives: []string{" "}}},
			}
		}},
		{name: "multiline generated fact", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AllowedGeneratedCodes = []string{"generated.application_defect"}
			corpus.Cases[0].GeneratedExpectation = GeneratedExpectation{
				Disposition:   GeneratedCauseRequired,
				RequiredFacts: []ExpectedConcept{{Name: "cause", Alternatives: []string{"cause\nother"}}},
			}
		}},
		{name: "too many generated facts", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AllowedGeneratedCodes = []string{"generated.application_defect"}
			corpus.Cases[0].GeneratedExpectation = GeneratedExpectation{
				Disposition: GeneratedCauseRequired, RequiredFacts: make([]ExpectedConcept, 17),
			}
			for index := range corpus.Cases[0].GeneratedExpectation.RequiredFacts {
				corpus.Cases[0].GeneratedExpectation.RequiredFacts[index] = ExpectedConcept{
					Name: "cause", Alternatives: []string{"cause"},
				}
			}
		}},
		{name: "too many generated fact alternatives", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AllowedGeneratedCodes = []string{"generated.application_defect"}
			alternatives := make([]string, 17)
			for index := range alternatives {
				alternatives[index] = "cause"
			}
			corpus.Cases[0].GeneratedExpectation = GeneratedExpectation{
				Disposition:   GeneratedCauseRequired,
				RequiredFacts: []ExpectedConcept{{Name: "cause", Alternatives: alternatives}},
			}
		}},
		{name: "oversized generated fact", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AllowedGeneratedCodes = []string{"generated.application_defect"}
			corpus.Cases[0].GeneratedExpectation = GeneratedExpectation{
				Disposition:   GeneratedCauseRequired,
				RequiredFacts: []ExpectedConcept{{Name: "cause", Alternatives: []string{strings.Repeat("c", 257)}}},
			}
		}},
		{name: "abstention with allowed code", mutate: func(corpus *Corpus) {
			corpus.Cases[0].AllowedGeneratedCodes = []string{"generated.application_defect"}
		}},
		{name: "abstention with required relation", mutate: func(corpus *Corpus) {
			corpus.Cases[0].GeneratedExpectation.RequiredRelations = []ExpectedRelation{{
				Name: "cause", Causes: []string{"cause"}, Effects: []string{"effect"},
			}}
		}},
		{name: "excess citation limit", mutate: func(corpus *Corpus) {
			corpus.Cases[0].GeneratedExpectation.MaximumCitations = 9
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			corpus := cloneCorpus(loaded)
			test.mutate(&corpus)
			if err := validateCorpus(corpus, filepath.Join(corpus.root, "evaluation")); err == nil {
				t.Fatal("validateCorpus() error = nil")
			}
		})
	}
}

func TestRunValidatesInputsAndCancellation(t *testing.T) {
	t.Parallel()

	corpus := loadCorpus(t)
	corpus.Cases = corpus.Cases[:1]
	target := diagnosticianFunc(func(context.Context, diagnosis.FailureEvidence) (diagnosis.Report, error) {
		return diagnosis.Report{}, errors.New("target failed")
	})
	for _, test := range []struct {
		name   string
		ctx    context.Context
		target diagnosis.Diagnostician
		mode   string
	}{
		{name: "nil context", target: target, mode: "test"},
		{name: "nil target", ctx: t.Context(), mode: "test"},
		{name: "blank mode", ctx: t.Context(), target: target, mode: "  "},
	} {
		if _, err := Run(test.ctx, corpus, test.target, test.mode); err == nil {
			t.Fatalf("Run(%s) error = nil", test.name)
		}
	}
	for _, repeats := range []int{0, 21} {
		if _, err := RunWithOptions(t.Context(), corpus, target, "test", RunOptions{Repeats: repeats}); err == nil {
			t.Fatalf("RunWithOptions(repeats %d) error = nil", repeats)
		}
	}
	if _, err := RunWithOptions(t.Context(), corpus, target, "test", RunOptions{
		Repeats: 1, SourceContext: &SourceContextOptions{Mode: "unknown", MaximumBytes: 1},
	}); err == nil {
		t.Fatal("RunWithOptions(invalid source mode) error = nil")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Run(canceled, corpus, target, "test"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run(canceled) error = %v", err)
	}

	summary, err := Run(t.Context(), corpus, target, "failing-target")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 0 || len(summary.Results) != 1 ||
		!strings.Contains(strings.Join(summary.Results[0].Violations, " "), "diagnose: target failed") {
		t.Fatalf("failing target summary = %#v", summary)
	}
}

func TestRunAttachesCheckedInSourceContext(t *testing.T) {
	t.Parallel()

	corpus, err := Select(loadCorpus(t), []string{"python_zero_division"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := engine.New("source-context-test", func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := RunWithOptions(t.Context(), corpus, target, "source", RunOptions{
		Repeats: 1,
		SourceContext: &SourceContextOptions{
			Mode: diagnosis.SourceContextFull, MaximumBytes: 256 * 1024,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || summary.Metrics.SourceContextCases != 1 ||
		len(summary.Results) != 1 || !summary.Results[0].SourceContextUsed {
		t.Fatalf("source-enabled summary = %#v", summary)
	}
}

func TestSelectCorpusByNameAndTag(t *testing.T) {
	t.Parallel()

	corpus := loadCorpus(t)
	selected, err := Select(corpus, nil, []string{"language.node"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Cases) != 4 {
		t.Fatalf("node case count = %d", len(selected.Cases))
	}
	selected, err = Select(corpus, []string{"node_type_error"}, []string{"language.node"})
	if err != nil || len(selected.Cases) != 1 || selected.Cases[0].Name != "node_type_error" {
		t.Fatalf("selected corpus = %#v, %v", selected.Cases, err)
	}
	for _, test := range []struct {
		names []string
		tags  []string
	}{
		{names: []string{"missing_case"}},
		{names: []string{"node_type_error"}, tags: []string{"language.go"}},
		{tags: []string{"missing.tag"}},
		{tags: []string{"INVALID"}},
	} {
		if _, err := Select(corpus, test.names, test.tags); err == nil {
			t.Fatalf("Select(%#v, %#v) error = nil", test.names, test.tags)
		}
	}
}

func TestRunWithOptionsRepeatsCasesDeterministically(t *testing.T) {
	t.Parallel()

	corpus := loadCorpus(t)
	corpus.Cases = corpus.Cases[:1]
	target, err := engine.New("repeat-test", func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := RunWithOptions(t.Context(), corpus, target, "repeat", RunOptions{Repeats: 3})
	if err != nil {
		t.Fatal(err)
	}
	if summary.UniqueCases != 1 || summary.Repeats != 3 || summary.Cases != 3 ||
		summary.Passed != 3 || len(summary.Results) != 3 || summary.Results[2].Iteration != 3 {
		t.Fatalf("repeated summary = %#v", summary)
	}
}

//nolint:cyclop // One table verifies the independent semantic counter dimensions and violations.
func TestEvaluateGeneratedMeasuresSemanticQualityAndAbstention(t *testing.T) {
	t.Parallel()

	test := Case{
		AllowedGeneratedCodes: []string{"generated.dependency_unavailable"},
		GeneratedExpectation: GeneratedExpectation{
			Disposition: GeneratedCauseRequired,
			RequiredFacts: []ExpectedConcept{
				{Name: "endpoint", Alternatives: []string{"127.0.0.1:5432"}},
				{Name: "condition", Alternatives: []string{"connection refused"}},
			},
			RequiredRelations: []ExpectedRelation{{
				Name: "refusal prevented connection", Causes: []string{"connection refused"},
				Effects: []string{"could not connect"},
			}},
			ForbiddenClaims:  []ExpectedConcept{{Name: "missing module", Alternatives: []string{"module is missing"}}},
			MaximumCitations: 1,
		},
	}
	finding := diagnosis.Finding{
		Code: "generated.dependency_unavailable", SupportingEvidence: []string{"stderr"},
	}
	result := Result{Violations: []string{}}
	quality := evaluateGenerated(
		&result, test, []diagnosis.Finding{finding},
		"Connection refused at 127.0.0.1:5432, so the worker could not connect.", true,
	)
	if len(result.Violations) != 0 || quality.usefulDiagnoses != 1 || quality.taxonomyCorrect != 1 ||
		quality.entitiesMatched != 2 || quality.relationsMatched != 1 || quality.economicalCitations != 1 {
		t.Fatalf("valid generated quality = %#v, violations %#v", quality, result.Violations)
	}

	result = Result{Violations: []string{}}
	finding.Code = "generated.application_defect"
	finding.SupportingEvidence = []string{"stderr", "exit"}
	quality = evaluateGenerated(
		&result, test, []diagnosis.Finding{finding}, "A module is missing, so processing stopped.", true,
	)
	if len(result.Violations) < 5 || quality.usefulDiagnoses != 0 || quality.forbiddenClaims != 1 ||
		quality.economicalCitations != 0 {
		t.Fatalf("invalid generated quality = %#v, violations %#v", quality, result.Violations)
	}

	abstainCase := Case{GeneratedExpectation: GeneratedExpectation{Disposition: GeneratedMustAbstain}}
	result = Result{Violations: []string{}}
	quality = evaluateGenerated(&result, abstainCase, nil, "", true)
	if len(result.Violations) != 0 || quality.abstentionsCorrect != 1 || quality.proposalsAccepted != 1 {
		t.Fatalf("accepted abstention = %#v, violations %#v", quality, result.Violations)
	}
	result = Result{Violations: []string{}}
	quality = evaluateGenerated(&result, abstainCase, nil, "", false)
	if len(result.Violations) != 1 || quality.abstentionsCorrect != 0 {
		t.Fatalf("fallback abstention = %#v, violations %#v", quality, result.Violations)
	}
}

//nolint:gocognit // Both filesystem failure modes share the same summary-level contract.
func TestRunRecordsUnreadableAndInvalidEvidence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "evidence"), 0o700); err != nil {
		t.Fatal(err)
	}
	validCase := cloneCorpus(loadCorpus(t)).Cases[0]
	validCase.Name = "bad_evidence"
	validCase.Evidence = "evidence/bad.json"
	corpus := Corpus{Kind: Kind, SchemaVersion: SchemaVersion, Cases: []Case{validCase}, root: root}
	target := diagnosticianFunc(func(context.Context, diagnosis.FailureEvidence) (diagnosis.Report, error) {
		t.Fatal("diagnostician called for invalid evidence")
		return diagnosis.Report{}, nil
	})
	for _, test := range []struct {
		name    string
		encoded []byte
		write   bool
		want    string
	}{
		{name: "missing", want: "read evidence"},
		{name: "invalid", encoded: []byte(`{"not":"evidence"}`), write: true, want: "decode evidence"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, "evidence", "bad.json")
			if test.write {
				if err := os.WriteFile(path, test.encoded, 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
			}
			summary, err := Run(t.Context(), corpus, target, "test")
			if err != nil {
				t.Fatal(err)
			}
			if len(summary.Results) != 1 || len(summary.Results[0].Violations) != 1 ||
				!strings.Contains(summary.Results[0].Violations[0], test.want) {
				t.Fatalf("summary = %#v, want %q violation", summary, test.want)
			}
		})
	}
}

//nolint:cyclop // Compact helper assertions cover all zero-denominator and signature semantics.
func TestEvaluationHelpersPreserveMetricSemantics(t *testing.T) {
	t.Parallel()

	if ratio(1, 0) != 1 || ratio(1, 2) != 0.5 || zeroRatio(1, 0) != 0 || zeroRatio(1, 2) != 0.5 {
		t.Fatal("ratio helpers changed zero-denominator semantics")
	}
	for _, invalid := range []string{"", "UPPER", "contains space", strings.Repeat("a", 161)} {
		if validCode(invalid) {
			t.Fatalf("validCode(%q) = true", invalid)
		}
	}
	if !validCode("lower.case_1-ok") || hasDuplicates([]string{"a", "b"}) ||
		!hasDuplicates([]string{"a", "a"}) {
		t.Fatal("code or duplicate validation changed")
	}
	result := Result{Violations: []string{}}
	checkCodes(&result, []string{"present", "forbidden"}, []string{"present", "missing"}, []string{"forbidden"}, "finding")
	if len(result.Violations) != 2 {
		t.Fatalf("checkCodes violations = %#v", result.Violations)
	}
	report := diagnosis.Report{
		PrimaryFindingID: "primary",
		Findings:         []diagnosis.Finding{{ID: "other", Code: "other"}, {ID: "primary", Code: "selected"}},
	}
	if primaryCode(report) != "selected" || findPrimary(diagnosis.Report{}).Code != "" {
		t.Fatal("primary finding lookup changed")
	}
	if !matchesAlternatives("ValueError: required key region is absent", []string{"missing field", "required key"}) ||
		matchesAlternatives("ValueError: input failed", []string{"missing field", "required key"}) {
		t.Fatal("matchesAlternatives() changed concept matching semantics")
	}
	if generatedSignature(Result{ProposalAccepted: true, GeneratedCodes: []string{"generated.data_validation"}}) !=
		"true:generated.data_validation" {
		t.Fatal("generatedSignature() changed consistency identity")
	}
}

func TestRunReportsIndependentExpectationRegressions(t *testing.T) {
	t.Parallel()

	corpus := cloneCorpus(loadCorpus(t))
	corpus.Cases = corpus.Cases[:1]
	test := &corpus.Cases[0]
	actualPrimary := test.AcceptedPrimaryCodes[0]
	actualAction := test.RequiredActionCodes[0]
	test.AcceptedPrimaryCodes = []string{"different.primary"}
	test.RequiredFindingCodes = []string{"missing.finding"}
	test.ForbiddenFindingCodes = []string{actualPrimary}
	test.RequiredActionCodes = []string{"missing.action"}
	test.ForbiddenActionCodes = []string{actualAction}
	test.ExpectedRetry = "different"
	test.ExpectedExistingPolicy = "different"
	test.MinimumConfidence, test.MaximumConfidence = 100, 100
	target, err := engine.New("expectation-test", func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := Run(t.Context(), corpus, target, "regression")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 0 || len(summary.Results) != 1 || len(summary.Results[0].Violations) < 7 {
		t.Fatalf("regression summary = %#v", summary)
	}
	if summary.Metrics.PrimaryCodePrecision != 0 || summary.Metrics.RetryAdviceAccuracy != 0 ||
		summary.Metrics.CitationValidity != 1 || summary.Metrics.DeterministicStability != 1 {
		t.Fatalf("independent metrics = %#v", summary.Metrics)
	}
}

func TestRunMeasuresGeneratedCauseSpecificity(t *testing.T) {
	t.Parallel()

	corpus := cloneCorpus(loadCorpus(t))
	index := slices.IndexFunc(corpus.Cases, func(test Case) bool { return test.Name == "python_traceback" })
	if index < 0 {
		t.Fatal("python_traceback evaluation case is missing")
	}
	corpus.Cases = []Case{corpus.Cases[index]}
	base, err := engine.New("specificity-test", func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		summary     string
		explanation string
		wantPassed  bool
	}{
		{
			name:    "specific",
			summary: "ValueError identifies invalid input in the target request",
			explanation: "Root cause: the supplied input violates the application's value constraint. " +
				"Failure path: validation raises ValueError before processing begins.",
			wantPassed: true,
		},
		{
			name:        "generic",
			summary:     "The target encountered an application error",
			explanation: "Root cause: target data was rejected. Failure path: processing stopped.",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			target := generatedEvaluationTarget(t, base, test.summary, test.explanation)
			summary, runErr := Run(t.Context(), corpus, target, "specificity-test")
			if runErr != nil {
				t.Fatal(runErr)
			}
			passed := summary.Passed == 1 && summary.Metrics.GeneratedSpecificity == 1 &&
				summary.Metrics.GeneratedSpecificityCases == 1
			if passed != test.wantPassed {
				t.Fatalf("specificity summary = %#v, want passed %t", summary, test.wantPassed)
			}
			if !test.wantPassed && (len(summary.Results[0].Violations) == 0 ||
				!strings.Contains(strings.Join(summary.Results[0].Violations, " "), "omitted required fact")) {
				t.Fatalf("generic violations = %#v", summary.Results[0].Violations)
			}
		})
	}
}

func generatedEvaluationTarget(
	t *testing.T,
	base diagnosis.Diagnostician,
	summary string,
	explanation string,
) diagnosis.Diagnostician {
	t.Helper()
	confidence, err := diagnosis.NewConfidence(40, "Uncalibrated generated evaluation hypothesis.")
	if err != nil {
		t.Fatal(err)
	}

	return diagnosticianFunc(func(ctx context.Context, evidence diagnosis.FailureEvidence) (diagnosis.Report, error) {
		report, diagnoseErr := base.Diagnose(ctx, evidence)
		if diagnoseErr != nil {
			return diagnosis.Report{}, diagnoseErr
		}
		if len(report.Findings) == 0 || len(report.Findings[0].SupportingEvidence) == 0 ||
			len(evidence.Core.Items) == 0 {
			return diagnosis.Report{}, errors.New("specificity fixture lacks required evidence")
		}
		report.Findings = append(report.Findings, diagnosis.Finding{
			ID: "finding:999:generated-data-validation", Code: "generated.data_validation",
			Category: "application", Severity: diagnosis.SeverityWarning,
			Summary: summary, Explanation: explanation, Confidence: confidence,
			SupportingEvidence:    slices.Clone(report.Findings[0].SupportingEvidence),
			ContradictingEvidence: []string{}, ContradictingFindings: []string{},
			Analyzer: "generator.proposal/1",
		})
		report.Mode = diagnosis.ModeMixed
		report.Versions.GenerationRequestSchemaVersion = provider.RequestSchemaVersion
		report.Versions.ProposalSchemaVersion = provider.ProposalSchemaVersion
		report.Disclosure = diagnosis.DisclosureManifest{
			ProviderInvoked: true, GeneratedContentUsed: true, Locality: diagnosis.ProviderLocal,
			Profile: "evaluation", Provider: "evaluation", Model: "fixture",
			RequestID: "sha256:" + strings.Repeat("a", 64), Classes: []string{"metadata"},
			ItemIDs: []string{evidence.Core.Items[0].ID}, ArtifactIDs: []string{}, EnrichmentIDs: []string{},
			ItemCount: 1, RequestBytes: 1,
		}
		report.Generators = []diagnosis.GeneratorDescriptor{{
			Provider: "evaluation", Model: "fixture", Profile: "evaluation", Locality: diagnosis.ProviderLocal,
		}}

		return diagnosis.Seal(report)
	})
}

func loadCorpus(t *testing.T) Corpus {
	t.Helper()
	corpus, err := Load(filepath.Join("..", "..", "testdata", "evaluation", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	return corpus
}

func cloneCorpus(source Corpus) Corpus {
	cloned := source
	cloned.Cases = slices.Clone(source.Cases)
	for index := range cloned.Cases {
		cloned.Cases[index].Tags = slices.Clone(source.Cases[index].Tags)
		cloned.Cases[index].AcceptedPrimaryCodes = slices.Clone(source.Cases[index].AcceptedPrimaryCodes)
		cloned.Cases[index].AllowedGeneratedCodes = slices.Clone(source.Cases[index].AllowedGeneratedCodes)
		cloned.Cases[index].RequiredFindingCodes = slices.Clone(source.Cases[index].RequiredFindingCodes)
		cloned.Cases[index].ForbiddenFindingCodes = slices.Clone(source.Cases[index].ForbiddenFindingCodes)
		cloned.Cases[index].RequiredActionCodes = slices.Clone(source.Cases[index].RequiredActionCodes)
		cloned.Cases[index].ForbiddenActionCodes = slices.Clone(source.Cases[index].ForbiddenActionCodes)
		expectation := source.Cases[index].GeneratedExpectation
		expectation.RequiredFacts = slices.Clone(expectation.RequiredFacts)
		for conceptIndex := range expectation.RequiredFacts {
			expectation.RequiredFacts[conceptIndex].Alternatives = slices.Clone(
				expectation.RequiredFacts[conceptIndex].Alternatives,
			)
		}
		expectation.RequiredRelations = slices.Clone(expectation.RequiredRelations)
		for relationIndex := range expectation.RequiredRelations {
			expectation.RequiredRelations[relationIndex].Causes = slices.Clone(
				expectation.RequiredRelations[relationIndex].Causes,
			)
			expectation.RequiredRelations[relationIndex].Effects = slices.Clone(
				expectation.RequiredRelations[relationIndex].Effects,
			)
		}
		expectation.ForbiddenClaims = slices.Clone(expectation.ForbiddenClaims)
		for conceptIndex := range expectation.ForbiddenClaims {
			expectation.ForbiddenClaims[conceptIndex].Alternatives = slices.Clone(
				expectation.ForbiddenClaims[conceptIndex].Alternatives,
			)
		}
		cloned.Cases[index].GeneratedExpectation = expectation
	}

	return cloned
}
