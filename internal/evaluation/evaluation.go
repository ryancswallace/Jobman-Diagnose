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
)

const (
	// Kind identifies an evaluation corpus manifest.
	Kind = "jobman.diagnosis_evaluation_corpus"
	// SchemaVersion is the newest corpus schema supported by this package.
	SchemaVersion = 1

	maximumCorpusBytes = 1024 * 1024
	maximumCases       = 512
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
}

// Result records one case without copying evidence or generated prose.
type Result struct {
	Name              string   `json:"name"`
	Passed            bool     `json:"passed"`
	PrimaryCode       string   `json:"primary_code,omitempty"`
	Retry             string   `json:"retry,omitempty"`
	ExistingPolicy    string   `json:"existing_policy,omitempty"`
	GeneratedCodes    []string `json:"generated_codes"`
	Violations        []string `json:"violations"`
	ReportFingerprint string   `json:"report_fingerprint,omitempty"`
}

// Metrics are intentionally separated so fluent model output cannot hide a
// citation, action, or retry-policy regression.
type Metrics struct {
	PrimaryCodePrecision   float64 `json:"primary_code_precision"`
	UnsupportedClaimRate   float64 `json:"unsupported_claim_rate"`
	CitationValidity       float64 `json:"citation_validity"`
	SafeActionRate         float64 `json:"safe_action_rate"`
	RetryAdviceAccuracy    float64 `json:"retry_advice_accuracy"`
	DeterministicStability float64 `json:"deterministic_stability"`
	ProviderFallbackRate   float64 `json:"provider_fallback_rate"`
}

// Summary is the versioned machine-readable evaluation result.
type Summary struct {
	Kind          string   `json:"kind"`
	SchemaVersion int      `json:"schema_version"`
	Mode          string   `json:"mode"`
	Cases         int      `json:"cases"`
	Passed        int      `json:"passed"`
	Metrics       Metrics  `json:"metrics"`
	Results       []Result `json:"results"`
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

// Run evaluates a diagnostician and an independent deterministic stability
// baseline. The target may be deterministic or explicitly model-augmented.
func Run(ctx context.Context, corpus Corpus, target diagnosis.Diagnostician, mode string) (Summary, error) {
	if ctx == nil || target == nil || strings.TrimSpace(mode) == "" {
		return Summary{}, errors.New("run evaluation: context, diagnostician, and mode are required")
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
		Kind: "jobman.diagnosis_evaluation_result", SchemaVersion: 1, Mode: mode,
		Cases: len(corpus.Cases), Results: make([]Result, 0, len(corpus.Cases)),
	}
	var primaryOK, citationsOK, retryOK, stable, fallback, generatedClaims, unsupportedClaims int
	var safeActions, actions int
	for _, test := range corpus.Cases {
		if err := ctx.Err(); err != nil {
			return Summary{}, fmt.Errorf("run evaluation: %w", err)
		}
		result, counters, err := runCase(ctx, corpus.root, test, target, baseline)
		if err != nil {
			result = Result{Name: test.Name, GeneratedCodes: []string{}, Violations: []string{err.Error()}}
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
		summary.Results = append(summary.Results, result)
	}
	summary.Metrics = Metrics{
		PrimaryCodePrecision:   ratio(primaryOK, summary.Cases),
		UnsupportedClaimRate:   zeroRatio(unsupportedClaims, generatedClaims),
		CitationValidity:       ratio(citationsOK, summary.Cases),
		SafeActionRate:         ratio(safeActions, actions),
		RetryAdviceAccuracy:    ratio(retryOK, summary.Cases),
		DeterministicStability: ratio(stable, summary.Cases),
		ProviderFallbackRate:   ratio(fallback, summary.Cases),
	}

	return summary, nil
}

type caseCounters struct {
	primaryOK, citationsOK, retryOK, stable, fallback        int
	actions, safeActions, generatedClaims, unsupportedClaims int
}

//nolint:cyclop,gocognit // A corpus case intentionally evaluates all independent quality and safety metrics together.
func runCase(
	ctx context.Context,
	root string,
	test Case,
	target diagnosis.Diagnostician,
	baseline diagnosis.Diagnostician,
) (Result, caseCounters, error) {
	result := Result{Name: test.Name, GeneratedCodes: []string{}, Violations: []string{}}
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
	evidence, err := enrichment.Collect(ctx, core)
	if err != nil {
		return result, counters, err
	}
	report, err := target.Diagnose(ctx, evidence)
	if err != nil {
		return result, counters, fmt.Errorf("diagnose: %w", err)
	}
	result.PrimaryCode = primaryCode(report)
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
	for _, finding := range report.Findings {
		findingCodes = append(findingCodes, finding.Code)
		if finding.Analyzer != "generator.proposal/1" {
			continue
		}
		counters.generatedClaims++
		result.GeneratedCodes = append(result.GeneratedCodes, finding.Code)
		if !slices.Contains(test.AllowedGeneratedCodes, finding.Code) {
			counters.unsupportedClaims++
			result.Violations = append(result.Violations, "generated diagnosis code is not labeled as supported for this case")
		}
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
	prior := ""
	for _, test := range corpus.Cases {
		path := filepath.Clean(filepath.Join(root, filepath.FromSlash(test.Evidence)))
		if test.Name <= prior || !validCode(test.Name) || filepath.IsAbs(test.Evidence) ||
			!strings.HasPrefix(path, rootWithSeparator) || len(test.AcceptedPrimaryCodes) == 0 ||
			test.MinimumConfidence < 0 || test.MaximumConfidence > 100 ||
			test.MinimumConfidence > test.MaximumConfidence || !validExpectedCodes(test) {
			return fmt.Errorf("validate evaluation corpus: invalid case %q", test.Name)
		}
		prior = test.Name
	}

	return nil
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
