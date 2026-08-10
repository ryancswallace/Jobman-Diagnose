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
		{name: "unknown field", encoded: []byte(`{"kind":"` + Kind + `","schema_version":1,"cases":[],"extra":true}`)},
		{name: "trailing value", encoded: []byte(`{"kind":"` + Kind + `","schema_version":1,"cases":[]} {}`)},
		{name: "invalid header", encoded: []byte(`{"kind":"wrong","schema_version":1,"cases":[]}`)},
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
		{name: "missing retry", mutate: func(corpus *Corpus) { corpus.Cases[0].ExpectedRetry = "" }},
		{name: "missing policy", mutate: func(corpus *Corpus) { corpus.Cases[0].ExpectedExistingPolicy = "" }},
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
		cloned.Cases[index].AcceptedPrimaryCodes = slices.Clone(source.Cases[index].AcceptedPrimaryCodes)
		cloned.Cases[index].AllowedGeneratedCodes = slices.Clone(source.Cases[index].AllowedGeneratedCodes)
		cloned.Cases[index].RequiredFindingCodes = slices.Clone(source.Cases[index].RequiredFindingCodes)
		cloned.Cases[index].ForbiddenFindingCodes = slices.Clone(source.Cases[index].ForbiddenFindingCodes)
		cloned.Cases[index].RequiredActionCodes = slices.Clone(source.Cases[index].RequiredActionCodes)
		cloned.Cases[index].ForbiddenActionCodes = slices.Clone(source.Cases[index].ForbiddenActionCodes)
	}

	return cloned
}
