package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/evaluation"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestParseEvaluationOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
		wantError string
	}{
		{name: "defaults"},
		{name: "explicit deterministic", arguments: []string{
			"--corpus", "corpus.json", "--summary", "--cases", "one,two", "--tags", "language.go", "--repeat", "3",
		}},
		{name: "flags only", arguments: []string{"corpus.json"}, wantError: "flags only"},
		{name: "nonempty corpus", arguments: []string{"--corpus", " "}, wantError: "requires a corpus path"},
		{name: "zero repeats", arguments: []string{"--repeat", "0"}, wantError: "between 1 and 20"},
		{name: "excess repeats", arguments: []string{"--repeat", "21"}, wantError: "between 1 and 20"},
		{name: "live config", arguments: []string{"--live"}, wantError: "--diagnosis-config"},
		{name: "live-only config", arguments: []string{"--diagnosis-config", "config.yml"}, wantError: "require --live"},
		{name: "live-only profile", arguments: []string{"--profile", "local"}, wantError: "require --live"},
		{name: "live-only fallback", arguments: []string{"--allow-fallback"}, wantError: "require --live"},
		{name: "live-only disclosure", arguments: []string{"--share", "metadata,command"}, wantError: "require --live"},
		{name: "live-only capture", arguments: []string{"--capture-proposals", "capture.json"}, wantError: "require --live"},
		{name: "live-only promotion", arguments: []string{"--promotion-policy", "policy.json"}, wantError: "require --live"},
		{
			name: "promotion rejects filters",
			arguments: []string{
				"--live", "--diagnosis-config", "config.yml", "--promotion-policy", "policy.json", "--tags", "language.go",
			},
			wantError: "complete unfiltered corpus",
		},
		{
			name: "capture output collision",
			arguments: []string{
				"--live", "--diagnosis-config", "config.yml", "--output", "same.json", "--capture-proposals", "same.json",
			},
			wantError: "different files",
		},
		{
			name: "live capture",
			arguments: []string{
				"--live", "--diagnosis-config", "config.yml", "--capture-proposals", "capture.json",
			},
		},
		{name: "unknown flag", arguments: []string{"--unknown"}, wantError: "flag provided"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			parsed, err := parse(test.arguments, &stderr)
			if test.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				if parsed.corpus == "" || parsed.repeat < 1 {
					t.Fatal("parse() returned invalid deterministic options")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("parse() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

//nolint:cyclop // This filesystem contract verifies initial publication, privacy, decoding, and replacement together.
func TestProposalCaptureWritesPrivateReplaceableDocument(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "capture.json")
	generator := captureGenerator{records: []proposalCapture{{
		RequestID: "sha256:" + strings.Repeat("a", 64), AnalysisEvidenceID: "sha256:" + strings.Repeat("b", 64),
		Provider: "openai_compatible", Model: "test", ProposalAccepted: false,
		ValidationCode: "proposal_not_specific", Proposal: json.RawMessage(`{"hypotheses":[]}`),
	}}}
	if err := generator.write(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("capture mode = %o", info.Mode().Perm())
	}
	// #nosec G304 -- path is created beneath this test's private temporary directory.
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document proposalCaptureDocument
	if unmarshalErr := json.Unmarshal(encoded, &document); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if document.Kind != "jobman.diagnosis_evaluation_proposal_capture" || document.SchemaVersion != 3 ||
		len(document.Records) != 1 ||
		document.Records[0].ValidationCode != "proposal_not_specific" || document.Records[0].ProposalAccepted {
		t.Fatalf("capture document = %#v", document)
	}
	generator.records[0].ValidationCode = "replacement"
	if writeErr := generator.write(path); writeErr != nil {
		t.Fatal(writeErr)
	}
	encoded, err = os.ReadFile(path) // #nosec G304 -- path is test-owned.
	if err != nil {
		t.Fatal(err)
	}
	if unmarshalErr := json.Unmarshal(encoded, &document); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if document.Records[0].ValidationCode != "replacement" {
		t.Fatalf("replacement document = %#v", document)
	}
}

func TestProposalCaptureAnnotatesCasesAndIterations(t *testing.T) {
	t.Parallel()

	evidenceID := "sha256:" + strings.Repeat("b", 64)
	generator := captureGenerator{records: []proposalCapture{{AnalysisEvidenceID: evidenceID}, {AnalysisEvidenceID: evidenceID}}}
	generator.annotate([]evaluation.Result{
		{Name: "case_one", Iteration: 1, AnalysisEvidenceID: evidenceID},
		{Name: "case_one", Iteration: 2, AnalysisEvidenceID: evidenceID},
	})
	if generator.records[0].CaseName != "case_one" || generator.records[0].Iteration != 1 ||
		generator.records[1].CaseName != "case_one" || generator.records[1].Iteration != 2 {
		t.Fatalf("annotated records = %#v", generator.records)
	}
}

func TestCapabilityRoutedDiagnosticianUsesLogsOnlyWithCoreCapability(t *testing.T) {
	t.Parallel()

	metadataErr := errors.New("metadata route")
	logsErr := errors.New("logs route")
	routed := capabilityRoutedDiagnostician{
		metadata: errorDiagnostician{err: metadataErr}, logs: errorDiagnostician{err: logsErr},
	}
	if _, err := routed.Diagnose(t.Context(), diagnosis.FailureEvidence{}); !errors.Is(err, metadataErr) {
		t.Fatalf("Diagnose(without capability) error = %v", err)
	}
	evidence := diagnosis.FailureEvidence{}
	evidence.Core.Source.Capabilities = []string{"configured_value_redaction_v1"}
	if _, err := routed.Diagnose(t.Context(), evidence); !errors.Is(err, logsErr) {
		t.Fatalf("Diagnose(with capability) error = %v", err)
	}
}

func TestParseShare(t *testing.T) {
	t.Parallel()

	classes, err := parseShare(" metadata, command,metadata ")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(classes, ","); got != "metadata,command" {
		t.Fatalf("parseShare() = %q", got)
	}
	for _, value := range []string{"metadata,unknown", "command", ""} {
		if _, err := parseShare(value); err == nil {
			t.Fatalf("parseShare(%q) error = nil", value)
		}
	}
}

func TestProfileSourceOptions(t *testing.T) {
	t.Parallel()

	profile := config.Profile{Disclosure: map[string]config.ClassLimits{
		string(diagnosis.DisclosureSourceContent): {MaximumArtifacts: 1, MaximumBytes: 4096},
	}}
	if options, err := profileSourceOptions(profile); err != nil || options != nil {
		t.Fatalf("disabled source options = %#v, %v", options, err)
	}
	profile.SourceContext = &config.SourceContextPolicy{
		Mode: config.SourceContextModeLimited, LinesBeforeAndAfter: 7,
	}
	options, err := profileSourceOptions(profile)
	if err != nil || options == nil || options.Mode != diagnosis.SourceContextLimited ||
		options.LinesBeforeAndAfter != 7 || options.MaximumBytes != 4096 {
		t.Fatalf("limited source options = %#v, %v", options, err)
	}
	delete(profile.Disclosure, string(diagnosis.DisclosureSourceContent))
	if _, err := profileSourceOptions(profile); err == nil {
		t.Fatal("profileSourceOptions(missing disclosure) error = nil")
	}
}

func TestParseSelection(t *testing.T) {
	t.Parallel()

	values, err := parseSelection(" one, two,one ")
	if err != nil || !slices.Equal(values, []string{"one", "two"}) {
		t.Fatalf("parseSelection() = %#v, %v", values, err)
	}
	if values, err = parseSelection(" "); err != nil || values != nil {
		t.Fatalf("parseSelection(empty) = %#v, %v", values, err)
	}
	if _, err := parseSelection("one,,two"); err == nil {
		t.Fatal("parseSelection(empty member) error = nil")
	}
}

//nolint:cyclop,gocognit // The cases exercise every output and filesystem boundary of one CLI operation.
func TestRunDeterministicEvaluationOutputs(t *testing.T) {
	t.Parallel()

	corpus := filepath.Join("..", "..", "testdata", "evaluation", "manifest.json")
	t.Run("summary", func(t *testing.T) {
		t.Parallel()
		var stdout bytes.Buffer
		if err := run([]string{"--corpus", corpus, "--summary"}, &stdout, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), "evaluation: 72/72 executions passed (72 cases x1)") ||
			!strings.Contains(stdout.String(), "useful n/a") {
			t.Fatalf("summary = %q", stdout.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		t.Parallel()
		var stdout bytes.Buffer
		if err := run([]string{"--corpus", corpus}, &stdout, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		var result struct {
			SchemaVersion int    `json:"schema_version"`
			Mode          string `json:"mode"`
			Cases         int    `json:"cases"`
			Passed        int    `json:"passed"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.SchemaVersion != 6 || result.Mode != "deterministic" || result.Cases != 72 || result.Passed != 72 {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("filtered repeated", func(t *testing.T) {
		t.Parallel()
		var stdout bytes.Buffer
		if err := run([]string{
			"--corpus", corpus, "--tags", "language.node", "--repeat", "2",
		}, &stdout, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		var result struct {
			UniqueCases int `json:"unique_cases"`
			Repeats     int `json:"repeats"`
			Cases       int `json:"cases"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.UniqueCases != 5 || result.Repeats != 2 || result.Cases != 10 {
			t.Fatalf("filtered repeated result = %#v", result)
		}
	})
	t.Run("private file", func(t *testing.T) {
		t.Parallel()
		output := filepath.Join(t.TempDir(), "evaluation.json")
		if err := run([]string{"--corpus", corpus, "--output", output}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(output)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("output mode = %o", info.Mode().Perm())
		}
		if err := run([]string{"--corpus", corpus, "--output", output}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRunEvaluationErrors(t *testing.T) {
	t.Parallel()

	corpus := filepath.Join("..", "..", "testdata", "evaluation", "manifest.json")
	tests := []struct {
		name      string
		arguments []string
		stdout    *errorWriter
		want      string
	}{
		{name: "missing corpus", arguments: []string{"--corpus", filepath.Join(t.TempDir(), "missing.json")}, want: "load evaluation corpus"},
		{name: "missing live config", arguments: []string{"--corpus", corpus, "--live", "--diagnosis-config", filepath.Join(t.TempDir(), "missing.yml")}, want: "load live evaluation configuration"},
		{name: "summary write", arguments: []string{"--corpus", corpus, "--summary"}, stdout: &errorWriter{}, want: "write evaluation summary"},
		{name: "json write", arguments: []string{"--corpus", corpus}, stdout: &errorWriter{}, want: "write evaluation report"},
		{name: "unknown case", arguments: []string{"--corpus", corpus, "--cases", "missing_case"}, want: "was not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout interface{ Write([]byte) (int, error) } = &bytes.Buffer{}
			if test.stdout != nil {
				stdout = test.stdout
			}
			err := run(test.arguments, stdout, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestExecuteEvaluation(t *testing.T) {
	t.Parallel()

	corpus := filepath.Join("..", "..", "testdata", "evaluation", "manifest.json")
	var stdout, stderr bytes.Buffer
	if status := execute([]string{"--corpus", corpus, "--summary"}, &stdout, &stderr); status != 0 {
		t.Fatalf("execute(valid) = %d, stderr %q", status, stderr.String())
	}
	stderr.Reset()
	if status := execute([]string{"--corpus", "missing.json"}, &stdout, &stderr); status != 1 || stderr.Len() == 0 {
		t.Fatalf("execute(missing) = %d, stderr %q", status, stderr.String())
	}
}

func TestRunLiveEvaluationValidatesProfileAndDisclosureBeforeInvocation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configuration := filepath.Join(root, "diagnosis.yml")
	contents := `schema_version: 2
defaults:
  profile: test
profiles:
  test:
    provider: openai_compatible
    locality: remote
    endpoint: https://example.com/v1/chat/completions
    model: test-model
    require_json_schema: true
    timeout: 2s
    maximum_input_bytes: 262144
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
`
	if err := os.WriteFile(configuration, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	corpus := filepath.Join("..", "..", "testdata", "evaluation", "manifest.json")
	tests := []struct {
		name  string
		extra []string
		want  string
	}{
		{name: "unknown profile", extra: []string{"--profile", "missing"}, want: "not defined"},
		{name: "unknown disclosure", extra: []string{"--share", "metadata,unknown"}, want: "unsupported disclosure"},
		{name: "metadata required", extra: []string{"--share", "command"}, want: "requires metadata"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			arguments := []string{"--corpus", corpus, "--live", "--diagnosis-config", configuration}
			arguments = append(arguments, test.extra...)
			if err := run(arguments, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run(live boundary) error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunLiveEvaluationWithCommandBridgeCapture(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	configuration := filepath.Join(root, "diagnosis.yml")
	contents := fmt.Sprintf(`schema_version: 2
defaults:
  profile: test
profiles:
  test:
    provider: command
    locality: local
    command:
      executable: %q
      arguments: ["-test.run=^TestEvaluateCommandBridgeHelper$"]
    model: test-model
    require_json_schema: true
    timeout: 10s
    maximum_input_bytes: 262144
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
`, executable)
	if writeErr := os.WriteFile(configuration, []byte(contents), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	corpus := filepath.Join("..", "..", "testdata", "evaluation", "manifest.json")
	output := filepath.Join(root, "evaluation.json")
	capture := filepath.Join(root, "proposals.json")
	if runErr := run([]string{
		"--corpus", corpus, "--cases", "ambiguous_worker_stop", "--live",
		"--diagnosis-config", configuration, "--allow-fallback",
		"--output", output, "--capture-proposals", capture,
	}, &bytes.Buffer{}, &bytes.Buffer{}); runErr != nil {
		t.Fatal(runErr)
	}
	encoded, err := os.ReadFile(capture) // #nosec G304 -- capture is in the test's private temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	var document proposalCaptureDocument
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Records) != 1 || !document.Records[0].ProposalAccepted ||
		document.Records[0].CaseName != "ambiguous_worker_stop" || document.Records[0].Iteration != 1 {
		t.Fatalf("capture document = %#v", document)
	}
}

func TestEvaluateCommandBridgeHelper(_ *testing.T) {
	if os.Getenv("JOBMAN_DIAGNOSE_PROVIDER_PROTOCOL") != "3" {
		return
	}
	request, err := provider.DecodeRequest(os.Stdin, 262144)
	if err != nil {
		os.Exit(20)
	}
	proposal := provider.Proposal{
		Kind: provider.ProposalKind, SchemaVersion: provider.ProposalSchemaVersion,
		RequestID: request.RequestID, Hypotheses: []provider.Hypothesis{},
		RecommendedActions: []string{}, MissingEvidence: []provider.MissingEvidence{},
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		os.Exit(21)
	}
	os.Exit(0)
}

func TestNewLiveDiagnosticianRoutesApprovedLogs(t *testing.T) {
	t.Parallel()

	profile := liveTestProfile()
	generator := &stubGenerator{profile: profile}
	diagnostician, err := newLiveDiagnostician(
		errorDiagnostician{err: errors.New("base")}, generator, "test", profile,
		[]string{"metadata", "log_content"}, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := diagnostician.(capabilityRoutedDiagnostician); !ok {
		t.Fatalf("diagnostician type = %T", diagnostician)
	}
	if _, err := newLiveDiagnostician(nil, generator, "test", profile, []string{"metadata"}, false); err == nil {
		t.Fatal("newLiveDiagnostician(nil base) error = nil")
	}
}

func TestCaptureGeneratorRecordsProviderFailures(t *testing.T) {
	t.Parallel()

	request := provider.Request{RequestID: "request", AnalysisEvidenceID: "evidence"}
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "classified", err: provider.NewFailure(provider.FailureRequestTimeout, errors.New("timeout")), code: "request_timeout"},
		{name: "unclassified", err: errors.New("failure"), code: "unclassified"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			capture := &captureGenerator{Generator: &stubGenerator{err: test.err}}
			if _, err := capture.Generate(t.Context(), request); !errors.Is(err, test.err) {
				t.Fatalf("Generate() error = %v", err)
			}
			if len(capture.records) != 1 || capture.records[0].FailureCode != test.code ||
				capture.records[0].RequestID != request.RequestID ||
				capture.records[0].AnalysisEvidenceID != request.AnalysisEvidenceID {
				t.Fatalf("capture records = %#v", capture.records)
			}
		})
	}
}

func liveTestProfile() config.Profile {
	return config.Profile{
		Provider: "command", Locality: provider.LocalityLocal, Model: "test-model",
		Timeout: "2s", MaximumInputBytes: 262144, MaximumOutputBytes: 32768,
		Disclosure: map[string]config.ClassLimits{
			"metadata":    {MaximumItems: 256, MaximumBytes: 131072},
			"log_content": {MaximumArtifacts: 2, MaximumBytes: 65536},
		},
	}
}

type stubGenerator struct {
	profile  config.Profile
	response provider.Response
	err      error
}

func (*stubGenerator) Name() string { return "command" }

func (generator *stubGenerator) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		NativeJSONSchema: true, MaximumInputBytes: generator.profile.MaximumInputBytes,
		MaximumOutputBytes: generator.profile.MaximumOutputBytes, Locality: generator.profile.Locality,
	}
}

func (generator *stubGenerator) Generate(context.Context, provider.Request) (provider.Response, error) {
	return generator.response, generator.err
}

type errorWriter struct{}

func (*errorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type errorDiagnostician struct{ err error }

func (diagnostician errorDiagnostician) Diagnose(context.Context, diagnosis.FailureEvidence) (diagnosis.Report, error) {
	return diagnosis.Report{}, diagnostician.err
}
