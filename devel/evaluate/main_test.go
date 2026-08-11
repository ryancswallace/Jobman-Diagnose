package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseEvaluationOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
		wantError string
	}{
		{name: "defaults"},
		{name: "explicit deterministic", arguments: []string{"--corpus", "corpus.json", "--summary"}},
		{name: "flags only", arguments: []string{"corpus.json"}, wantError: "flags only"},
		{name: "nonempty corpus", arguments: []string{"--corpus", " "}, wantError: "requires a corpus path"},
		{name: "live config", arguments: []string{"--live"}, wantError: "--diagnosis-config"},
		{name: "live-only config", arguments: []string{"--diagnosis-config", "config.yml"}, wantError: "require --live"},
		{name: "live-only profile", arguments: []string{"--profile", "local"}, wantError: "require --live"},
		{name: "live-only fallback", arguments: []string{"--allow-fallback"}, wantError: "require --live"},
		{name: "live-only disclosure", arguments: []string{"--share", "metadata,command"}, wantError: "require --live"},
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
				if parsed.corpus == "" {
					t.Fatal("parse() returned an empty corpus")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("parse() error = %v, want substring %q", err, test.wantError)
			}
		})
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

//nolint:gocognit // The cases exercise every output and filesystem boundary of one CLI operation.
func TestRunDeterministicEvaluationOutputs(t *testing.T) {
	t.Parallel()

	corpus := filepath.Join("..", "..", "testdata", "evaluation", "manifest.json")
	t.Run("summary", func(t *testing.T) {
		t.Parallel()
		var stdout bytes.Buffer
		if err := run([]string{"--corpus", corpus, "--summary"}, &stdout, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), "evaluation: 19/19 cases passed") ||
			!strings.Contains(stdout.String(), "specificity n/a") {
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
			Mode   string `json:"mode"`
			Cases  int    `json:"cases"`
			Passed int    `json:"passed"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Mode != "deterministic" || result.Cases != 19 || result.Passed != 19 {
			t.Fatalf("result = %#v", result)
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
		if err := run([]string{"--corpus", corpus, "--output", output}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatal("run() overwrote an existing report")
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

type errorWriter struct{}

func (*errorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
