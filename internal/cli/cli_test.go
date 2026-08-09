package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	diagnosisconfig "github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/coreclient"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/supportbundle"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestRunDiagnosesOfflineEvidenceAsJSON(t *testing.T) {
	evidence, path := writeEvidenceFixture(t)
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"--from-evidence", path, "--json"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v, stderr = %s", err, stderr.String())
	}
	report, err := diagnosis.Decode(&stdout, diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if err := diagnosis.ValidateAgainstEvidence(report, collectAnalysisEvidence(t, evidence)); err != nil {
		t.Fatal(err)
	}
	if report.Findings[0].Code != "core.executable_not_found" {
		t.Fatalf("primary finding = %#v", report.Findings[0])
	}
}

func TestRunWritesPrivateExportsWithoutOverwrite(t *testing.T) {
	evidence, path := writeEvidenceFixture(t)
	directory := t.TempDir()
	exported := filepath.Join(directory, "evidence.json")
	reportPath := filepath.Join(directory, "report.json")
	var stdout, stderr bytes.Buffer
	arguments := []string{
		"--from-evidence", path, "--export-evidence", exported,
		"--output", reportPath, "--json",
	}
	if err := Run(arguments, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when --output is used", stdout.String())
	}
	for _, file := range []string{exported, reportPath} {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %o", file, info.Mode().Perm())
		}
	}
	// #nosec G304 -- exported is a test-owned path under t.TempDir.
	exportedFile, err := os.Open(exported)
	if err != nil {
		t.Fatal(err)
	}
	decoded, decodeErr := coreclient.DecodeEvidence(exportedFile)
	closeErr := exportedFile.Close()
	if err := errors.Join(decodeErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if decoded.EvidenceID != evidence.EvidenceID {
		t.Fatalf("exported evidence ID = %q", decoded.EvidenceID)
	}
	if err := Run(arguments, bytes.NewReader(nil), &stdout, &stderr); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second Run() error = %v, want os.ErrExist", err)
	}
}

func TestRunSupportBundleDryRunAndPrivateCreation(t *testing.T) {
	_, evidencePath := writeEvidenceFixture(t)
	bundlePath := filepath.Join(t.TempDir(), "diagnosis-support.tar.gz")
	arguments := []string{
		"--from-evidence", evidencePath, "--support-bundle", bundlePath,
	}
	var stdout bytes.Buffer
	if err := Run(
		append(append([]string{}, arguments...), "--bundle-dry-run"),
		bytes.NewReader(nil), &stdout, &bytes.Buffer{},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bundlePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run archive stat error = %v, want not exist", err)
	}
	if !strings.Contains(stdout.String(), "no archive created") || !strings.Contains(stdout.String(), "evidence.json") {
		t.Fatalf("dry-run output = %q", stdout.String())
	}
	stdout.Reset()
	if err := Run(
		append(append([]string{}, arguments...), "--bundle-dry-run", "--json"),
		bytes.NewReader(nil), &stdout, &bytes.Buffer{},
	); err != nil {
		t.Fatal(err)
	}
	var inventory supportbundle.Inventory
	if err := json.Unmarshal(stdout.Bytes(), &inventory); err != nil || inventory.Kind != supportbundle.Kind {
		t.Fatalf("JSON dry-run inventory/error = %#v / %v", inventory, err)
	}

	stdout.Reset()
	if err := Run(arguments, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Size() == 0 {
		t.Fatalf("support bundle mode/size = %o/%d", info.Mode().Perm(), info.Size())
	}
	if err := Run(arguments, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second bundle creation error = %v, want os.ErrExist", err)
	}
}

func TestRunHumanOutputAndUsage(t *testing.T) {
	_, path := writeEvidenceFixture(t)
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"--from-evidence", path}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Diagnosis\n") || !strings.Contains(stdout.String(), "Retry\n") ||
		!strings.Contains(stdout.String(), "Recommended next steps\n") {
		t.Fatalf("human report = %q", stdout.String())
	}
	if err := Run(nil, bytes.NewReader(nil), &stdout, &stderr); !errors.Is(err, errUsage) || ExitCode(err) != 2 {
		t.Fatalf("Run(no args) error/code = %v/%d", err, ExitCode(err))
	}
}

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run([]string{"--version"}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "evidence schema 1") || !strings.Contains(stdout.String(), "report schema 1") ||
		!strings.Contains(stdout.String(), "configuration schema 2") {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunHelpIsSuccessful(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	if err := Run([]string{"--help"}, bytes.NewReader(nil), &bytes.Buffer{}, &stderr); err != nil {
		t.Fatalf("Run(--help) error = %v", err)
	}
	if !strings.Contains(stderr.String(), "usage: jobman-diagnose") {
		t.Fatalf("Run(--help) output = %q", stderr.String())
	}
}

func TestRunUsesDefaultGeneratedProfileWithImpliedMetadata(t *testing.T) {
	evidence, evidencePath := writeEvidenceFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(incoming.Body).Decode(&payload); err != nil || len(payload.Messages) != 2 {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		request, err := provider.DecodeRequest(strings.NewReader(payload.Messages[1].Content), 512*1024)
		if err != nil {
			http.Error(writer, "bad protocol", http.StatusBadRequest)
			return
		}
		proposalJSON, marshalErr := json.Marshal(provider.Proposal{
			Kind: provider.ProposalKind, SchemaVersion: 1, RequestID: request.RequestID,
			Hypotheses: []provider.Hypothesis{{
				Code: "generated.application_configuration", Category: "process", Summary: "Generated CLI alternative",
				Explanation:           "This uncalibrated alternative cites a projected fact.",
				SupportingEvidence:    []string{request.Manifest.ItemIDs[0]},
				ContradictingEvidence: []string{}, ContradictsFindings: []string{},
			}},
			RecommendedActions: []string{}, MissingEvidence: []provider.MissingEvidence{},
		})
		if marshalErr != nil {
			http.Error(writer, "encode proposal", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(writer).Encode(map[string]any{
			"id": "cli-provider-request", "model": "test-model",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": string(proposalJSON)},
			}},
		}); encodeErr != nil {
			t.Errorf("encode fake response: %v", encodeErr)
		}
	}))
	defer server.Close()
	configuration := writeDiagnosisConfig(t, server.URL+"/v1/chat/completions")
	t.Setenv(diagnosisconfig.EnvironmentPath, configuration)
	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"--from-evidence", evidencePath, "--json", "--ai",
	}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() error = %v, stderr = %s", err, stderr.String())
	}
	report, err := diagnosis.Decode(&stdout, diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := diagnosis.ValidateAgainstEvidence(report, collectAnalysisEvidence(t, evidence)); err != nil {
		t.Fatal(err)
	}
	if report.Mode != diagnosis.ModeMixed || !report.Disclosure.ProviderInvoked ||
		!report.Disclosure.GeneratedContentUsed {
		t.Fatalf("generated report = %#v", report)
	}
}

func TestRunDeterministicIgnoresMalformedModelConfiguration(t *testing.T) {
	_, evidencePath := writeEvidenceFixture(t)
	t.Setenv(diagnosisconfig.EnvironmentPath, "relative/path/that/must/not/be/resolved.yml")
	malformed := filepath.Join(t.TempDir(), "diagnosis.yml")
	if err := os.WriteFile(malformed, []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run([]string{
		"--from-evidence", evidencePath, "--json", "--deterministic", "--diagnosis-config", malformed,
	}, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	report, err := diagnosis.Decode(&stdout, diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != diagnosis.ModeDeterministic || report.Disclosure.ProviderInvoked {
		t.Fatalf("deterministic report = %#v", report)
	}
}

func TestRunGeneratedFailureFallsBackUnlessRequired(t *testing.T) {
	_, evidencePath := writeEvidenceFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		if _, writeErr := writer.Write([]byte(`{"error":"provider-secret-canary"}`)); writeErr != nil {
			t.Errorf("write fake error response: %v", writeErr)
		}
	}))
	defer server.Close()
	configuration := writeDiagnosisConfig(t, server.URL+"/v1/chat/completions")
	baseArguments := []string{
		"--from-evidence", evidencePath, "--json", "--ai", "--diagnosis-config", configuration,
	}
	var stdout bytes.Buffer
	if err := Run(baseArguments, bytes.NewReader(nil), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	report, err := diagnosis.Decode(&stdout, diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != diagnosis.ModeDeterministic || !report.Disclosure.ProviderInvoked ||
		report.Disclosure.GeneratedContentUsed {
		t.Fatalf("fallback report = %#v", report)
	}
	if len(report.Warnings) == 0 || report.Warnings[len(report.Warnings)-1].Code != "generator_failed" ||
		!strings.Contains(report.Warnings[len(report.Warnings)-1].Message, "http_status") ||
		strings.Contains(report.Warnings[len(report.Warnings)-1].Message, "provider-secret-canary") {
		t.Fatalf("fallback warnings = %#v", report.Warnings)
	}
	requiredArguments := append(append([]string{}, baseArguments...), "--require-model")
	if err := Run(requiredArguments, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); err == nil ||
		strings.Contains(err.Error(), "provider-secret-canary") || !strings.Contains(err.Error(), "http_status") {
		t.Fatalf("required Run() error = %v", err)
	}
}

func TestRunRejectsIncompleteAIUsage(t *testing.T) {
	_, evidencePath := writeEvidenceFixture(t)
	tests := [][]string{
		{"--from-evidence", evidencePath, "--require-model"},
		{"--from-evidence", evidencePath, "--share", "metadata"},
		{"--from-evidence", evidencePath, "--deterministic", "--ai"},
		{"--from-evidence", evidencePath, "--bundle-dry-run"},
		{"--from-evidence", evidencePath, "--support-bundle", "same", "--output", "same"},
	}
	for _, arguments := range tests {
		if err := Run(arguments, bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{}); !errors.Is(err, errUsage) {
			t.Fatalf("Run(%v) error = %v, want usage", arguments, err)
		}
	}
}

func TestParseAIShortcutsAndLogSharing(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"--ai", "demo"},
		{"--ai-logs", "demo"},
		{"--ai", "--share", "log_content", "demo"},
		{"-a", "--share", "log_content", "demo"},
	} {
		parsed, err := parse(arguments, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("parse(%v): %v", arguments, err)
		}
		if !hasDefaultAIContext(parsed) {
			t.Fatalf("parse(%v) = %#v", arguments, parsed)
		}
		if slices.Contains(arguments, "--ai-logs") || slices.Contains(arguments, "log_content") {
			if parsed.request.Logs != diagnostic.LogsTail || !slices.Contains(parsed.share, "log_content") {
				t.Fatalf("parse(%v) log sharing = %#v", arguments, parsed)
			}
		}
	}
	if _, err := parse([]string{"--ai-logs", "--logs", "none", "demo"}, &bytes.Buffer{}); !errors.Is(err, errUsage) {
		t.Fatalf("conflicting log options error = %v", err)
	}
}

func hasDefaultAIContext(parsed options) bool {
	return parsed.aiEnabled() && parsed.request.IncludeCommand && parsed.request.IncludePaths &&
		parsed.request.IncludeEnvironmentNames && slices.Contains(parsed.share, "metadata") &&
		slices.Contains(parsed.share, "command") && slices.Contains(parsed.share, "path") &&
		slices.Contains(parsed.share, "environment_name")
}

func TestRunConfigurationInspectionCommands(t *testing.T) {
	configuration := writeDiagnosisConfig(t, "http://127.0.0.1:8000/v1/chat/completions")
	t.Setenv(diagnosisconfig.EnvironmentPath, configuration)
	tests := []struct {
		arguments []string
		contains  []string
	}{
		{arguments: []string{"config", "paths"}, contains: []string{"effective:", configuration, "environment"}},
		{arguments: []string{"config", "validate"}, contains: []string{"configuration is valid", configuration}},
		{arguments: []string{"config", "show"}, contains: []string{`"schema_version": 2`, `"profile": "test"`}},
		{arguments: []string{"profiles"}, contains: []string{"* test", "provider=openai_compatible", "disclosure=metadata"}},
	}
	for _, test := range tests {
		var stdout, stderr bytes.Buffer
		if err := Run(test.arguments, bytes.NewReader(nil), &stdout, &stderr); err != nil {
			t.Fatalf("Run(%v) error = %v; stderr = %s", test.arguments, err, stderr.String())
		}
		for _, expected := range test.contains {
			if !strings.Contains(stdout.String(), expected) {
				t.Fatalf("Run(%v) output = %q, want %q", test.arguments, stdout.String(), expected)
			}
		}
	}
}

func TestRunAIReportsMissingDefaultConfiguration(t *testing.T) {
	_, evidencePath := writeEvidenceFixture(t)
	missing := filepath.Join(t.TempDir(), "missing-diagnosis.yml")
	t.Setenv(diagnosisconfig.EnvironmentPath, missing)
	err := Run(
		[]string{"--from-evidence", evidencePath, "--ai"},
		bytes.NewReader(nil), &bytes.Buffer{}, &bytes.Buffer{},
	)
	if err == nil || !strings.Contains(err.Error(), "no configuration was found") || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing configuration error = %v", err)
	}
}

func writeEvidenceFixture(t *testing.T) (diagnostic.Evidence, string) {
	t.Helper()
	evidence, err := testevidence.Failed("executable_not_found", nil)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := diagnostic.Encode(&encoded, evidence); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	return evidence, path
}

func collectAnalysisEvidence(t *testing.T, evidence diagnostic.Evidence) diagnosis.FailureEvidence {
	t.Helper()

	failureEvidence, err := enrichment.Collect(t.Context(), evidence)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	return failureEvidence
}

func writeDiagnosisConfig(t *testing.T, endpoint string) string {
	t.Helper()
	contents := fmt.Sprintf(`schema_version: 2
defaults:
  profile: test
profiles:
  test:
    provider: openai_compatible
    locality: local
    endpoint: %s
    model: test-model
    require_json_schema: true
    timeout: 2s
    maximum_input_bytes: 262144
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
`, endpoint)
	path := filepath.Join(t.TempDir(), "diagnosis.yml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}
