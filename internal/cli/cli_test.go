package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	diagnosisconfig "github.com/ryancswallace/jobman-diagnose/internal/config"
	"github.com/ryancswallace/jobman-diagnose/internal/coreclient"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/sourcecontext"
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
		if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
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
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 || info.Size() == 0 {
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
		!strings.Contains(stdout.String(), "Recommended next steps\n") ||
		strings.Contains(stdout.String(), "Technical details\n") {
		t.Fatalf("human report = %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"--from-evidence", path, "--details"}, bytes.NewReader(nil), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Technical details\n") || !strings.Contains(stdout.String(), "Evidence\n") {
		t.Fatalf("detailed human report = %q", stdout.String())
	}
	if err := Run(nil, bytes.NewReader(nil), &stdout, &stderr); !errors.Is(err, errUsage) || ExitCode(err) != 2 {
		t.Fatalf("Run(no args) error/code = %v/%d", err, ExitCode(err))
	}
}

func TestRunColorStylesOnlyHumanOutput(t *testing.T) {
	_, path := writeEvidenceFixture(t)
	var stdout, stderr bytes.Buffer
	if err := Run(
		[]string{"--from-evidence", path, "--color", "always"},
		bytes.NewReader(nil), &stdout, &stderr,
	); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "\x1b[1mDiagnosis\x1b[0m") {
		t.Fatalf("forced-color human report = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run(
		[]string{"--from-evidence", path, "--json", "--color", "always"},
		bytes.NewReader(nil), &stdout, &stderr,
	); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "\x1b[") || !json.Valid(stdout.Bytes()) {
		t.Fatalf("forced color contaminated JSON = %q", stdout.String())
	}
}

func TestColorEnabledRespectsModeTerminalAndEnvironment(t *testing.T) {
	t.Parallel()

	destination := &bytes.Buffer{}
	environment := runtimeEnvironment{
		interactive: func(io.Writer) bool { return true },
		lookupEnv:   func(string) (string, bool) { return "", false },
	}
	if !colorEnabled(colorAuto, destination, environment) || !colorEnabled(colorAlways, destination, runtimeEnvironment{}) {
		t.Fatal("interactive auto or explicit always color was disabled")
	}
	if colorEnabled(colorNever, destination, environment) || colorEnabled("invalid", destination, environment) {
		t.Fatal("never or invalid color mode was enabled")
	}

	for _, test := range []struct {
		name   string
		lookup func(string) (string, bool)
	}{
		{name: "NO_COLOR", lookup: func(name string) (string, bool) {
			return map[string]string{"NO_COLOR": "1"}[name], name == "NO_COLOR"
		}},
		{name: "dumb terminal", lookup: func(name string) (string, bool) {
			return map[string]string{"TERM": " DUMB "}[name], name == "TERM"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			blocked := environment
			blocked.lookupEnv = test.lookup
			if colorEnabled(colorAuto, destination, blocked) {
				t.Fatal("automatic color ignored environment opt-out")
			}
		})
	}

	noninteractive := environment
	noninteractive.interactive = func(io.Writer) bool { return false }
	if colorEnabled(colorAuto, destination, noninteractive) {
		t.Fatal("automatic color was enabled for redirected output")
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

func TestRunProviderDoctorExercisesConfiguredModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		decodeErr := json.NewDecoder(request.Body).Decode(&payload)
		closeErr := request.Body.Close()
		if decodeErr != nil || closeErr != nil || len(payload.Messages) < 2 {
			http.Error(response, "invalid request", http.StatusBadRequest)
			return
		}
		var generationRequest provider.Request
		if err := json.Unmarshal([]byte(payload.Messages[len(payload.Messages)-1].Content), &generationRequest); err != nil {
			http.Error(response, "invalid generation request", http.StatusBadRequest)
			return
		}
		proposal, err := json.Marshal(provider.Proposal{
			Kind: provider.ProposalKind, SchemaVersion: provider.ProposalSchemaVersion,
			RequestID: generationRequest.RequestID,
			Hypotheses: []provider.Hypothesis{{
				Code: "generated.dependency_unavailable", Category: "network",
				Summary:            "Inventory synchronization was refused by 127.0.0.1:65535",
				RootCause:          "The synchronize inventory connection to 127.0.0.1:65535 failed with connection refused.",
				Explanation:        "The connection refused signal prevents the inventory synchronization request from completing.",
				SupportingEvidence: []string{"doctor:artifact:stderr"}, ContradictingEvidence: []string{},
				ContradictsFindings: []string{},
			}},
			RecommendedActions: []string{}, MissingEvidence: []provider.MissingEvidence{},
		})
		if err != nil {
			http.Error(response, "encode failure", http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(response).Encode(map[string]any{
			"id": "doctor-test",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"content": string(proposal), "refusal": ""},
			}},
			"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 40},
		}); encodeErr != nil {
			t.Errorf("encode doctor response: %v", encodeErr)
		}
	}))
	defer server.Close()

	configuration := writeDiagnosisConfig(t, server.URL+"/v1/chat/completions")
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"doctor", "--diagnosis-config", configuration, "--json"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("doctor error = %v, stderr = %s", err, stderr.String())
	}
	var report struct {
		Kind  string `json:"kind"`
		Ready bool   `json:"ready"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil ||
		report.Kind != "jobman.diagnosis_provider_doctor" || !report.Ready {
		t.Fatalf("doctor report/error = %s / %v", stdout.String(), err)
	}
}

func TestRunProviderDoctorReturnsFailedReportWithoutResponseContent(t *testing.T) {
	secret := "secret-looking provider response"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, secret, http.StatusNotFound)
	}))
	defer server.Close()
	configuration := writeDiagnosisConfig(t, server.URL+"/v1/chat/completions")
	var stdout bytes.Buffer
	err := Run([]string{"doctor", "--diagnosis-config", configuration, "--json"}, nil, &stdout, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "failed readiness check") ||
		strings.Contains(err.Error(), secret) || strings.Contains(stdout.String(), secret) ||
		!strings.Contains(stdout.String(), `"ready": false`) || !strings.Contains(stdout.String(), "http_status") {
		t.Fatalf("doctor failure/report = %v / %s", err, stdout.String())
	}
}

func TestRunProviderDoctorReportsCredentialSetupFailure(t *testing.T) {
	t.Setenv("JOBMAN_DIAGNOSE_DOCTOR_TEST_CREDENTIAL", "")
	configuration := writeDiagnosisConfig(t, "http://127.0.0.1:1/v1/chat/completions")
	contents, err := os.ReadFile(configuration) // #nosec G304 -- test-owned path under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	contents = bytes.Replace(contents, []byte("    model: test-model\n"), []byte(
		"    model: test-model\n    credential:\n      environment: JOBMAN_DIAGNOSE_DOCTOR_TEST_CREDENTIAL\n",
	), 1)
	if writeErr := os.WriteFile(configuration, contents, 0o600); writeErr != nil { // #nosec G703 -- test-owned path under t.TempDir.
		t.Fatal(writeErr)
	}
	var stdout bytes.Buffer
	err = Run([]string{"doctor", "--diagnosis-config", configuration, "--json"}, nil, &stdout, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "failed readiness check") ||
		!strings.Contains(stdout.String(), `"ready": false`) ||
		!strings.Contains(stdout.String(), `"code": "credential_adapter"`) ||
		strings.Contains(stdout.String(), "JOBMAN_DIAGNOSE_DOCTOR_TEST_CREDENTIAL") {
		t.Fatalf("doctor setup failure/report = %v / %s", err, stdout.String())
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

func TestRunUsesDefaultGeneratedProfileWithImpliedMetadataAndAbstains(t *testing.T) {
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
			Kind: provider.ProposalKind, SchemaVersion: provider.ProposalSchemaVersion, RequestID: request.RequestID,
			Hypotheses:         []provider.Hypothesis{},
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
	if report.Mode != diagnosis.ModeDeterministic || !report.Disclosure.ProviderInvoked ||
		report.Disclosure.GeneratedContentUsed || len(report.Warnings) == 0 ||
		report.Warnings[len(report.Warnings)-1].Code != "generator_abstained" {
		t.Fatalf("generated report = %#v", report)
	}
	if stderr.Len() != 0 {
		t.Fatalf("automatic progress with JSON output = %q, want empty", stderr.String())
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
		"--progress", "plain",
	}
	var stdout, progress bytes.Buffer
	if err := Run(baseArguments, bytes.NewReader(nil), &stdout, &progress); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, progress.String(), []string{
		"collecting Jobman evidence", "preparing bounded, redacted evidence",
		"waiting for profile test", "using the deterministic fallback",
	})
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

func TestRunCollectsExplicitSourceContextForAIRequest(t *testing.T) {
	_, evidencePath := writeEvidenceFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	configuration := writeDiagnosisConfigWithSource(t, server.URL+"/v1/chat/completions")
	sourcePath := filepath.Join(t.TempDir(), "worker.py")
	if err := os.WriteFile(sourcePath, []byte("raise RuntimeError('source context test')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := Run([]string{
		"--from-evidence", evidencePath, "--json", "--ai-source", "limited",
		"--source-file", sourcePath, "--source-line", "1", "--diagnosis-config", configuration,
	}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnosis.Decode(&stdout, diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(report.Disclosure.Classes, string(diagnosis.DisclosureSourceContent)) ||
		!slices.Contains(report.Disclosure.ArtifactIDs, "context:source:001") ||
		!slices.ContainsFunc(report.Warnings, func(warning diagnosis.Warning) bool {
			return warning.Code == "source_context_point_in_time"
		}) {
		t.Fatalf("source disclosure report = %#v", report)
	}
}

func TestRunUsesProfileSourceContextWithoutCLIEnablement(t *testing.T) {
	_, evidencePath := writeEvidenceFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	configuration := writeDiagnosisConfigWithSourcePolicy(t, server.URL+"/v1/chat/completions", "limited", 3)
	sourcePath := filepath.Join(t.TempDir(), "worker.py")
	if err := os.WriteFile(sourcePath, []byte("one = 1\ntwo = 2\nraise RuntimeError('boom')\nfour = 4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := Run([]string{
		"--from-evidence", evidencePath, "--json", "--ai", "--source-file", sourcePath,
		"--source-line", "3", "--diagnosis-config", configuration,
	}, bytes.NewReader(nil), &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnosis.Decode(&stdout, diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(report.Disclosure.Classes, string(diagnosis.DisclosureSourceContent)) ||
		!slices.Contains(report.Disclosure.ArtifactIDs, "context:source:001") {
		t.Fatalf("profile source disclosure report = %#v", report)
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

func TestParseSourceContextOptIn(t *testing.T) {
	t.Parallel()

	limited, err := parse([]string{
		"--ai-source", "limited", "--source-file", "/srv/app.py", "--source-line", "42", "demo",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if limited.aiSource != "limited" || limited.sourceFile != "/srv/app.py" || limited.sourceLine != 42 ||
		!limited.aiEnabled() || !slices.Contains(limited.share, string(diagnosis.DisclosureSourceContent)) ||
		limited.request.Logs != diagnostic.LogsMetadata {
		t.Fatalf("limited source options = %#v", limited)
	}
	full, err := parse([]string{"--ai-source", "FULL", "demo"}, &bytes.Buffer{})
	if err != nil || full.aiSource != "full" {
		t.Fatalf("full source options = %#v, %v", full, err)
	}
	none, err := parse([]string{"--ai-source", "NONE", "demo"}, &bytes.Buffer{})
	if err != nil || none.aiSource != "none" || slices.Contains(none.share, string(diagnosis.DisclosureSourceContent)) {
		t.Fatalf("disabled source options = %#v, %v", none, err)
	}
	for _, arguments := range [][]string{
		{"--source-file", "app.py", "demo"},
		{"--source-line", "2", "demo"},
		{"--ai-source", "full", "--source-line", "2", "demo"},
		{"--ai-source", "summary", "demo"},
	} {
		if _, err := parse(arguments, &bytes.Buffer{}); !errors.Is(err, errUsage) {
			t.Fatalf("parse(%v) error = %v, want usage", arguments, err)
		}
	}
}

func TestSelectGeneratorResolvesProfileSourcePolicyAndCLIOverrides(t *testing.T) {
	t.Parallel()

	configuration := writeDiagnosisConfigWithSourcePolicy(
		t, "http://127.0.0.1:8000/v1/chat/completions", "limited", 7,
	)
	tests := []struct {
		name       string
		arguments  []string
		wantMode   diagnosis.SourceContextMode
		wantRadius uint64
		wantSource bool
	}{
		{name: "profile default", arguments: []string{"--ai", "demo"}, wantMode: diagnosis.SourceContextLimited, wantRadius: 7, wantSource: true},
		{name: "command line none", arguments: []string{"--ai", "--ai-source", "none", "demo"}},
		{name: "command line full", arguments: []string{"--ai", "--ai-source", "full", "demo"}, wantMode: diagnosis.SourceContextFull, wantSource: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := append([]string{}, test.arguments...)
			arguments = append(arguments[:len(arguments)-1], "--diagnosis-config", configuration, "demo")
			parsed, err := parse(arguments, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			selection, err := selectGenerator(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if selection.sourceMode != test.wantMode || selection.sourceRadius != test.wantRadius ||
				slices.Contains(selection.approved, string(diagnosis.DisclosureSourceContent)) != test.wantSource {
				t.Fatalf("selection = %#v", selection)
			}
		})
	}
}

func TestSelectGeneratorResolvesFullAndDisabledProfileSourcePolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policyMode string
		arguments  []string
		wantMode   diagnosis.SourceContextMode
		wantRadius uint64
	}{
		{name: "full profile", policyMode: "full", arguments: []string{"--ai", "demo"}, wantMode: diagnosis.SourceContextFull},
		{name: "disabled profile", policyMode: "none", arguments: []string{"--ai", "demo"}},
		{
			name: "limited override uses standard radius", policyMode: "full",
			arguments: []string{"--ai", "--ai-source", "limited", "demo"},
			wantMode:  diagnosis.SourceContextLimited, wantRadius: sourcecontext.DefaultLinesBeforeAndAfter,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := writeDiagnosisConfigWithSourcePolicy(
				t, "http://127.0.0.1:8000/v1/chat/completions", test.policyMode, 0,
			)
			arguments := append([]string{}, test.arguments...)
			arguments = append(arguments[:len(arguments)-1], "--diagnosis-config", configuration, "demo")
			parsed, err := parse(arguments, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			selection, err := selectGenerator(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if selection.sourceMode != test.wantMode || selection.sourceRadius != test.wantRadius {
				t.Fatalf("selection = %#v", selection)
			}
		})
	}
}

func TestSelectGeneratorRequiresSourceDisclosureInProfile(t *testing.T) {
	t.Parallel()

	configuration := writeDiagnosisConfig(t, "http://127.0.0.1:8000/v1/chat/completions")
	parsed, err := parse([]string{
		"--ai-source", "limited", "--diagnosis-config", configuration, "demo",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	_, selectionErr := selectGenerator(parsed)
	if selectionErr == nil || !strings.Contains(selectionErr.Error(), "does not allow source_content") {
		t.Fatalf("selectGenerator() error = %v", selectionErr)
	}
	parsed, err = parse([]string{
		"--ai", "--share", "source_content", "--diagnosis-config", configuration, "demo",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	_, selectionErr = selectGenerator(parsed)
	if !errors.Is(selectionErr, errUsage) ||
		!strings.Contains(selectionErr.Error(), "requires profile source_context") {
		t.Fatalf("selectGenerator(source share without mode) error = %v", selectionErr)
	}
}

func TestParseProgressModes(t *testing.T) {
	t.Parallel()

	parsed, err := parse([]string{"demo"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.progress != progressAuto {
		t.Fatalf("default progress = %q, want %q", parsed.progress, progressAuto)
	}
	for _, mode := range []progressMode{progressAuto, progressPlain, progressOff} {
		parsed, err = parse([]string{"--progress", string(mode), "demo"}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("parse progress %q: %v", mode, err)
		}
		if parsed.progress != mode {
			t.Fatalf("parsed progress = %q, want %q", parsed.progress, mode)
		}
	}
	if _, err = parse([]string{"--progress", "animated", "demo"}, &bytes.Buffer{}); !errors.Is(err, errUsage) {
		t.Fatalf("invalid progress error = %v, want usage", err)
	}
}

func hasDefaultAIContext(parsed options) bool {
	return parsed.aiEnabled() && parsed.request.IncludeCommand && parsed.request.IncludePaths &&
		parsed.request.IncludeEnvironmentNames && parsed.includeSystem && slices.Contains(parsed.share, "metadata") &&
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
		{arguments: []string{"profiles"}, contains: []string{"* test", "provider=openai_compatible", "source=none", "disclosure=metadata"}},
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
	sourceConfiguration := writeDiagnosisConfigWithSourcePolicy(
		t, "http://127.0.0.1:8000/v1/chat/completions", "limited", 7,
	)
	var profiles bytes.Buffer
	if err := Run(
		[]string{"profiles", "--diagnosis-config", sourceConfiguration},
		bytes.NewReader(nil), &profiles, &bytes.Buffer{},
	); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(profiles.String(), "source=limited:7-lines-each-side") {
		t.Fatalf("profiles output = %q", profiles.String())
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
	return writeDiagnosisConfigDocument(t, endpoint, "")
}

func writeDiagnosisConfigWithSource(t *testing.T, endpoint string) string {
	t.Helper()
	return writeDiagnosisConfigDocument(t, endpoint, `      source_content:
        maximum_artifacts: 1
        maximum_bytes: 65536
`)
}

func writeDiagnosisConfigWithSourcePolicy(t *testing.T, endpoint, mode string, lines uint64) string {
	t.Helper()
	policy := fmt.Sprintf(`      source_content:
        maximum_artifacts: 1
        maximum_bytes: 65536
    source_context:
      mode: %s
`, mode)
	if mode == diagnosisconfig.SourceContextModeLimited {
		policy += fmt.Sprintf("      lines_before_and_after: %d\n", lines)
	}

	return writeDiagnosisConfigDocument(t, endpoint, policy)
}

func writeDiagnosisConfigDocument(t *testing.T, endpoint, extraDisclosure string) string {
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
%s`, endpoint, extraDisclosure)
	path := filepath.Join(t.TempDir(), "diagnosis.yml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func assertContainsAll(t *testing.T, actual string, expected []string) {
	t.Helper()

	for _, value := range expected {
		if !strings.Contains(actual, value) {
			t.Fatalf("output = %q, want %q", actual, value)
		}
	}
}
