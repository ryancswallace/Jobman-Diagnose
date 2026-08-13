// Command evaluate runs Jobman Diagnose's checked-in quality corpus. Live
// provider use requires the explicit --live flag and an explicit config path.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/buildinfo"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/evaluation"
	"github.com/ryancswallace/jobman-diagnose/internal/generation"
	"github.com/ryancswallace/jobman-diagnose/internal/securefile"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

type options struct {
	corpus           string
	cases            string
	tags             string
	repeat           int
	live             bool
	diagnosisConfig  string
	profile          string
	share            string
	allowFallback    bool
	output           string
	captureProposals string
	summary          bool
}

type proposalCaptureDocument struct {
	Kind          string            `json:"kind"`
	SchemaVersion int               `json:"schema_version"`
	Records       []proposalCapture `json:"records"`
}

type proposalCapture struct {
	CaseName           string          `json:"case_name,omitempty"`
	Iteration          int             `json:"iteration,omitempty"`
	RequestID          string          `json:"request_id"`
	AnalysisEvidenceID string          `json:"analysis_evidence_id"`
	Provider           string          `json:"provider,omitempty"`
	Model              string          `json:"model,omitempty"`
	ProviderRequestID  string          `json:"provider_request_id,omitempty"`
	InputUnits         uint64          `json:"input_units,omitempty"`
	OutputUnits        uint64          `json:"output_units,omitempty"`
	FailureCode        string          `json:"failure_code,omitempty"`
	ProposalAccepted   bool            `json:"proposal_accepted"`
	ValidationCode     string          `json:"validation_code,omitempty"`
	Proposal           json.RawMessage `json:"proposal,omitempty"`
}

type captureGenerator struct {
	generation.Generator
	mu      sync.Mutex
	records []proposalCapture
}

type capabilityRoutedDiagnostician struct {
	metadata diagnosis.Diagnostician
	logs     diagnosis.Diagnostician
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}

func execute(arguments []string, stdout, stderr io.Writer) int {
	if err := run(arguments, stdout, stderr); err != nil {
		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}

//nolint:cyclop,gocognit // The development command keeps mode selection, evaluation, output, and gate status in one auditable flow.
func run(arguments []string, stdout, stderr io.Writer) error {
	parsed, err := parse(arguments, stderr)
	if err != nil {
		return err
	}
	corpus, err := evaluation.Load(parsed.corpus)
	if err != nil {
		return err
	}
	caseNames, err := parseSelection(parsed.cases)
	if err != nil {
		return fmt.Errorf("parse case filter: %w", err)
	}
	tags, err := parseSelection(parsed.tags)
	if err != nil {
		return fmt.Errorf("parse tag filter: %w", err)
	}
	corpus, err = evaluation.Select(corpus, caseNames, tags)
	if err != nil {
		return err
	}
	clock := func() time.Time { return time.Now().UTC() }
	deterministic, err := engine.New(buildinfo.Version, clock)
	if err != nil {
		return err
	}
	var diagnostician diagnosis.Diagnostician = deterministic
	var capturedProposals *captureGenerator
	mode := "deterministic"
	runOptions := evaluation.RunOptions{Repeats: parsed.repeat}
	if parsed.live {
		configuration, loadErr := config.LoadFile(parsed.diagnosisConfig)
		if loadErr != nil {
			return fmt.Errorf("load live evaluation configuration: %w", loadErr)
		}
		profileName, profile, selectErr := configuration.SelectProfile(parsed.profile)
		if selectErr != nil {
			return selectErr
		}
		generator, generatorErr := generation.NewGenerator(profile)
		if generatorErr != nil {
			return generatorErr
		}
		if parsed.captureProposals != "" {
			capturedProposals = &captureGenerator{Generator: generator, records: []proposalCapture{}}
			generator = capturedProposals
		}
		approved, approvalErr := parseShare(parsed.share)
		if approvalErr != nil {
			return approvalErr
		}
		sourceOptions, sourceErr := profileSourceOptions(profile)
		if sourceErr != nil {
			return sourceErr
		}
		if sourceOptions != nil {
			approved = append(approved, string(diagnosis.DisclosureSourceContent))
			runOptions.SourceContext = sourceOptions
		}
		augmenter, augmenterErr := newLiveDiagnostician(
			deterministic, generator, profileName, profile, approved, !parsed.allowFallback,
		)
		if augmenterErr != nil {
			return augmenterErr
		}
		diagnostician = augmenter
		mode = "live:" + profileName
		if sourceOptions != nil {
			mode += ":source=" + string(sourceOptions.Mode)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	summary, evaluationErr := evaluation.RunWithOptions(
		ctx, corpus, diagnostician, mode, runOptions,
	)
	if parsed.captureProposals != "" {
		if capturedProposals == nil {
			return errors.New("write proposal capture: live capture generator is unavailable")
		}
		capturedProposals.annotate(summary.Results)
		if err := capturedProposals.write(parsed.captureProposals); err != nil {
			return fmt.Errorf("write proposal capture: %w", err)
		}
	}
	if evaluationErr != nil {
		return evaluationErr
	}
	write := func(destination io.Writer) error {
		encoder := json.NewEncoder(destination)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}
	if parsed.output != "" {
		if err := securefile.WriteAtomicReplace(parsed.output, write); err != nil {
			return fmt.Errorf("write evaluation report: %w", err)
		}
	} else if parsed.summary {
		specificity := metricText(summary.Metrics.GeneratedSpecificity, summary.Metrics.GeneratedSpecificityCases)
		abstention := metricText(summary.Metrics.AbstentionAccuracy, summary.Metrics.AbstentionCases)
		acceptance := metricText(summary.Metrics.ProposalAcceptance, summary.Metrics.ProviderInvokedCases)
		consistency := metricText(summary.Metrics.GeneratedConsistency, summary.Metrics.ConsistencyComparisons)
		if _, err := fmt.Fprintf(
			stdout, "evaluation: %d/%d executions passed (%d cases x%d); primary %.3f, citations %.3f, actions %.3f, retry %.3f, unsupported %.3f, useful %s, abstention %s, accepted %s, consistent %s, source %d\n",
			summary.Passed, summary.Cases, summary.UniqueCases, summary.Repeats,
			summary.Metrics.PrimaryCodePrecision,
			summary.Metrics.CitationValidity, summary.Metrics.SafeActionRate,
			summary.Metrics.RetryAdviceAccuracy, summary.Metrics.UnsupportedClaimRate,
			specificity, abstention, acceptance, consistency, summary.Metrics.SourceContextCases,
		); err != nil {
			return fmt.Errorf("write evaluation summary: %w", err)
		}
	} else if err := write(stdout); err != nil {
		return fmt.Errorf("write evaluation report: %w", err)
	}
	if summary.Passed != summary.Cases {
		return fmt.Errorf("evaluation failed: %d of %d executions passed", summary.Passed, summary.Cases)
	}

	return nil
}

func profileSourceOptions(profile config.Profile) (*evaluation.SourceContextOptions, error) {
	if profile.SourceContext == nil || profile.SourceContext.Mode == config.SourceContextModeNone {
		return nil, nil
	}
	limits, allowed := profile.Disclosure[string(diagnosis.DisclosureSourceContent)]
	if !allowed {
		return nil, errors.New("live evaluation source context requires bounded source_content disclosure")
	}

	return &evaluation.SourceContextOptions{
		Mode:                diagnosis.SourceContextMode(profile.SourceContext.Mode),
		LinesBeforeAndAfter: profile.SourceContext.LinesBeforeAndAfter,
		MaximumBytes:        limits.MaximumBytes,
	}, nil
}

func metricText(value float64, cases int) string {
	if cases == 0 {
		return "n/a"
	}

	return fmt.Sprintf("%.3f", value)
}

func parseSelection(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("selection contains an empty value")
		}
		if !slices.Contains(result, part) {
			result = append(result, part)
		}
	}

	return result, nil
}

//nolint:cyclop // Development-only flag validation keeps the live disclosure boundary visible in one place.
func parse(arguments []string, stderr io.Writer) (options, error) {
	parsed := options{corpus: "testdata/evaluation/manifest.json", repeat: 1, share: "metadata"}
	flags := flag.NewFlagSet("jobman-diagnose-evaluate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&parsed.corpus, "corpus", parsed.corpus, "path to the versioned evaluation corpus")
	flags.StringVar(&parsed.cases, "cases", "", "comma-separated exact case names")
	flags.StringVar(&parsed.tags, "tags", "", "comma-separated tags; a case may match any tag")
	flags.IntVar(&parsed.repeat, "repeat", parsed.repeat, "run each selected case 1-20 times")
	flags.BoolVar(&parsed.live, "live", false, "invoke an explicitly configured provider")
	flags.StringVar(&parsed.diagnosisConfig, "diagnosis-config", "", "explicit diagnosis.yml path for live evaluation")
	flags.StringVar(&parsed.profile, "profile", "", "named live profile (the configured default when omitted)")
	flags.StringVar(&parsed.share, "share", parsed.share, "comma-separated approved disclosure classes")
	flags.BoolVar(&parsed.allowFallback, "allow-fallback", false, "allow provider failures to use deterministic fallback")
	flags.StringVar(&parsed.output, "output", "", "atomically write or replace the private evaluation result")
	flags.StringVar(
		&parsed.captureProposals, "capture-proposals", "",
		"atomically write or replace raw live model proposals (synthetic corpora only)",
	)
	flags.BoolVar(&parsed.summary, "summary", false, "print one concise metric line instead of result JSON")
	if err := flags.Parse(arguments); err != nil {
		return parsed, err
	}
	if flags.NArg() != 0 || strings.TrimSpace(parsed.corpus) == "" {
		return parsed, errors.New("evaluation accepts flags only and requires a corpus path")
	}
	if parsed.repeat < 1 || parsed.repeat > 20 {
		return parsed, errors.New("--repeat must be between 1 and 20")
	}
	if parsed.live && strings.TrimSpace(parsed.diagnosisConfig) == "" {
		return parsed, errors.New("--live requires an explicit --diagnosis-config path")
	}
	if !parsed.live && (parsed.diagnosisConfig != "" || parsed.profile != "" || parsed.allowFallback ||
		parsed.share != "metadata" || parsed.captureProposals != "") {
		return parsed, errors.New("provider options require --live")
	}
	if parsed.captureProposals != "" && parsed.captureProposals == parsed.output {
		return parsed, errors.New("--capture-proposals and --output must name different files")
	}

	return parsed, nil
}

func newLiveDiagnostician(
	base diagnosis.Diagnostician,
	generator generation.Generator,
	profileName string,
	profile config.Profile,
	approved []string,
	required bool,
) (diagnosis.Diagnostician, error) {
	metadataClasses := slices.Clone(approved)
	metadataClasses = slices.DeleteFunc(metadataClasses, func(value string) bool { return value == "log_content" })
	metadata, err := generation.NewAugmenter(
		base, generator, profileName, profile, metadataClasses, required, nil,
	)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(approved, "log_content") {
		return metadata, nil
	}
	logs, err := generation.NewAugmenter(base, generator, profileName, profile, approved, required, nil)
	if err != nil {
		return nil, err
	}

	return capabilityRoutedDiagnostician{metadata: metadata, logs: logs}, nil
}

func (routed capabilityRoutedDiagnostician) Diagnose(
	ctx context.Context,
	evidence diagnosis.FailureEvidence,
) (diagnosis.Report, error) {
	if routed.logs != nil && slices.Contains(evidence.Core.Source.Capabilities, "configured_value_redaction_v1") {
		return routed.logs.Diagnose(ctx, evidence)
	}

	return routed.metadata.Diagnose(ctx, evidence)
}

func (generator *captureGenerator) Generate(
	ctx context.Context,
	request provider.Request,
) (provider.Response, error) {
	response, err := generator.Generator.Generate(ctx, request)
	record := proposalCapture{RequestID: request.RequestID, AnalysisEvidenceID: request.AnalysisEvidenceID}
	if err != nil {
		code, _, ok := provider.Diagnostic(err)
		if ok {
			record.FailureCode = string(code)
		} else {
			record.FailureCode = "unclassified"
		}
	} else {
		record.Provider = response.Provider
		record.Model = response.Model
		record.ProviderRequestID = response.ProviderRequestID
		record.InputUnits = response.InputUnits
		record.OutputUnits = response.OutputUnits
		record.Proposal = slices.Clone(response.JSON)
		_, validationErr := provider.DecodeProposal(bytes.NewReader(response.JSON), request)
		switch {
		case validationErr == nil:
			record.ProposalAccepted = true
		case errors.Is(validationErr, provider.ErrProposalNotSpecific):
			record.ValidationCode = "proposal_not_specific"
		case errors.Is(validationErr, provider.ErrProposalUnsupported):
			record.ValidationCode = "proposal_evidence_unsupported"
		default:
			record.ValidationCode = "proposal_validation_failed"
		}
	}
	generator.mu.Lock()
	generator.records = append(generator.records, record)
	generator.mu.Unlock()

	return response, err
}

func (generator *captureGenerator) write(path string) error {
	generator.mu.Lock()
	document := proposalCaptureDocument{
		Kind: "jobman.diagnosis_evaluation_proposal_capture", SchemaVersion: 3,
		Records: slices.Clone(generator.records),
	}
	generator.mu.Unlock()
	return securefile.WriteAtomicReplace(path, func(destination io.Writer) error {
		encoder := json.NewEncoder(destination)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	})
}

func (generator *captureGenerator) annotate(results []evaluation.Result) {
	labels := make(map[string][]evaluation.Result)
	for _, result := range results {
		if result.AnalysisEvidenceID != "" {
			labels[result.AnalysisEvidenceID] = append(labels[result.AnalysisEvidenceID], result)
		}
	}
	positions := make(map[string]int, len(labels))
	generator.mu.Lock()
	defer generator.mu.Unlock()
	for index := range generator.records {
		evidenceID := generator.records[index].AnalysisEvidenceID
		position := positions[evidenceID]
		if position >= len(labels[evidenceID]) {
			continue
		}
		label := labels[evidenceID][position]
		generator.records[index].CaseName = label.Name
		generator.records[index].Iteration = label.Iteration
		positions[evidenceID]++
	}
}

func parseShare(value string) ([]string, error) {
	values := strings.Split(value, ",")
	result := make([]string, 0, len(values))
	allowed := map[string]struct{}{
		"metadata": {}, "command": {}, "path": {}, "environment_name": {}, "log_content": {},
	}
	for _, current := range values {
		current = strings.TrimSpace(current)
		if _, ok := allowed[current]; !ok {
			return nil, fmt.Errorf("unsupported disclosure class %q", current)
		}
		if !slices.Contains(result, current) {
			result = append(result, current)
		}
	}
	if !slices.Contains(result, "metadata") {
		return nil, errors.New("live evaluation requires metadata disclosure")
	}

	return result, nil
}
