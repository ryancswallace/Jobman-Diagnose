package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	diagnosisconfig "github.com/ryancswallace/jobman-diagnose/internal/config"
)

type failingWriter struct{ err error }

func (writer failingWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestExitCodeClassifiesPublicFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "success", want: 0},
		{name: "usage", err: usageError(errors.New("bad option")), want: 2},
		{name: "canceled", err: fmtWrap(context.Canceled), want: 130},
		{name: "failure", err: errors.New("broken"), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ExitCode(test.err); got != test.want {
				t.Fatalf("ExitCode(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}

func fmtWrap(err error) error { return errors.Join(errors.New("outer"), err) }

//nolint:cyclop,gocognit // The test keeps the small CLI flag.Value contracts together.
func TestFlagValueParsers(t *testing.T) {
	t.Parallel()

	var mode logModeValue
	for _, encoded := range []string{"metadata", " TAIL ", "none"} {
		if err := mode.Set(encoded); err != nil {
			t.Fatalf("log mode Set(%q): %v", encoded, err)
		}
	}
	if mode.String() != "none" || (*logModeValue)(nil).String() != "" {
		t.Fatalf("log mode strings = %q/%q", mode.String(), (*logModeValue)(nil).String())
	}
	if err := mode.Set("complete"); err == nil {
		t.Fatal("invalid log mode error = nil")
	}

	var color colorModeValue
	for _, encoded := range []string{"auto", " ALWAYS ", "never"} {
		if err := color.Set(encoded); err != nil {
			t.Fatalf("color mode Set(%q): %v", encoded, err)
		}
	}
	if color.String() != "never" || (*colorModeValue)(nil).String() != "" {
		t.Fatalf("color mode strings = %q/%q", color.String(), (*colorModeValue)(nil).String())
	}
	if err := color.Set("sometimes"); err == nil {
		t.Fatal("invalid color mode error = nil")
	}

	byteSizes := []struct {
		encoded string
		want    uint64
	}{
		{encoded: "1", want: 1},
		{encoded: "2B", want: 2},
		{encoded: " 3 KiB ", want: 3 << 10},
		{encoded: "4MiB", want: 4 << 20},
	}
	for _, test := range byteSizes {
		encoded, want := test.encoded, test.want
		var value byteSizeValue
		if err := value.Set(encoded); err != nil {
			t.Fatalf("byte size Set(%q): %v", encoded, err)
		}
		wantValue := byteSizeValue(want)
		if uint64(value) != want || value.String() != wantValue.String() {
			t.Fatalf("byte size %q = %d/%q, want %d", encoded, value, value.String(), want)
		}
	}
	if (*byteSizeValue)(nil).String() != "0B" {
		t.Fatalf("nil byte size = %q", (*byteSizeValue)(nil).String())
	}
	for _, encoded := range []string{"1GB", "-1", "18446744073709551615MiB"} {
		if _, err := parseByteSize(encoded); err == nil {
			t.Fatalf("parseByteSize(%q) error = nil", encoded)
		}
	}

	var classes stringListValue
	if err := classes.Set(" metadata, command,path,environment_name,log_content "); err != nil {
		t.Fatal(err)
	}
	if classes.String() != "metadata,command,path,environment_name,log_content" ||
		(*stringListValue)(nil).String() != "" {
		t.Fatalf("disclosure classes = %q", classes.String())
	}
	for _, encoded := range []string{"", "secret"} {
		if err := classes.Set(encoded); err == nil {
			t.Fatalf("stringListValue.Set(%q) error = nil", encoded)
		}
	}
}

func TestValidateOptionsRejectsConflictingContracts(t *testing.T) {
	t.Parallel()

	valid := options{selector: "job", request: diagnostic.EvidenceRequest{Logs: diagnostic.LogsMetadata}}
	tests := []struct {
		name   string
		mutate func(*options)
	}{
		{name: "no input", mutate: func(value *options) { value.selector = "" }},
		{name: "two inputs", mutate: func(value *options) { value.fromEvidence = "saved.json" }},
		{name: "run history", mutate: func(value *options) { value.request.Run, value.request.AllRuns = 1, true }},
		{name: "log bytes without tail", mutate: func(value *options) { value.request.LogBytes = 1 }},
		{name: "deterministic AI", mutate: func(value *options) { value.deterministic, value.ai = true, true }},
		{name: "model not enabled", mutate: func(value *options) { value.requireModel = true }},
		{name: "sharing not enabled", mutate: func(value *options) { value.share = []string{"metadata"} }},
		{name: "details with JSON", mutate: func(value *options) { value.details, value.jsonOutput = true, true }},
		{name: "dry run without bundle", mutate: func(value *options) { value.bundleDryRun = true }},
		{name: "dry run creates output", mutate: func(value *options) {
			value.bundleDryRun, value.supportBundle, value.output = true, "bundle", "report"
		}},
		{name: "dry run creates evidence", mutate: func(value *options) {
			value.bundleDryRun, value.supportBundle, value.exportEvidence = true, "bundle", "evidence"
		}},
		{name: "bundle overwrites report", mutate: func(value *options) {
			value.supportBundle, value.output = "same", "same"
		}},
		{name: "bundle overwrites evidence", mutate: func(value *options) {
			value.supportBundle, value.exportEvidence = "same", "same"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed := valid
			test.mutate(&parsed)
			if err := validateOptions(parsed); !errors.Is(err, errUsage) {
				t.Fatalf("validateOptions() error = %v, want usage", err)
			}
		})
	}
	if err := validateOptions(valid); err != nil {
		t.Fatalf("validateOptions(valid): %v", err)
	}
}

func TestFromEvidenceRejectsEveryLiveCollectionOption(t *testing.T) {
	t.Parallel()

	liveOptions := []func(*options){
		func(value *options) { value.jobman = "/bin/jobman" },
		func(value *options) { value.stateDir = "/state" },
		func(value *options) { value.configPath = "/config" },
		func(value *options) { value.request.Run = 1 },
		func(value *options) { value.request.AllRuns = true },
		func(value *options) { value.request.Similar = 1 },
		func(value *options) { value.request.IncludeCommand = true },
		func(value *options) { value.request.IncludePaths = true },
		func(value *options) { value.request.IncludeEnvironmentNames = true },
		func(value *options) { value.includeSystem = true },
		func(value *options) { value.request.Logs = diagnostic.LogsNone },
		func(value *options) { value.request.Logs, value.request.LogBytes = diagnostic.LogsTail, 1 },
	}
	for index, mutate := range liveOptions {
		parsed := options{fromEvidence: "saved.json", request: diagnostic.EvidenceRequest{Logs: diagnostic.LogsMetadata}}
		mutate(&parsed)
		if !hasLiveCollectionOptions(parsed) {
			t.Fatalf("live option %d was not detected", index)
		}
		if err := validateOptions(parsed); !errors.Is(err, errUsage) {
			t.Fatalf("live option %d error = %v, want usage", index, err)
		}
	}
}

func TestNormalizeAIOptionsRespectsEvidenceBoundary(t *testing.T) {
	t.Parallel()

	if err := normalizeAIOptions(nil); err != nil {
		t.Fatal(err)
	}
	disabled := options{}
	if err := normalizeAIOptions(&disabled); err != nil || len(disabled.share) != 0 {
		t.Fatalf("disabled normalization = %#v, %v", disabled, err)
	}
	live := options{ai: true}
	if err := normalizeAIOptions(&live); err != nil || !hasDefaultAIContext(live) {
		t.Fatalf("live normalization = %#v, %v", live, err)
	}
	saved := options{aiLogs: true, fromEvidence: "-", request: diagnostic.EvidenceRequest{Logs: diagnostic.LogsMetadata}}
	if err := normalizeAIOptions(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.includeSystem || saved.request.IncludeCommand || saved.request.Logs != diagnostic.LogsMetadata ||
		!slices.Contains(saved.share, "log_content") {
		t.Fatalf("saved evidence normalization crossed collection boundary: %#v", saved)
	}
}

func TestParseAndInspectionUsageErrors(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{{"--version", "job"}, {"one", "two"}, {"--unknown"}} {
		if _, err := parse(arguments, io.Discard); !errors.Is(err, errUsage) {
			t.Fatalf("parse(%v) error = %v, want usage", arguments, err)
		}
	}
	writeErr := errors.New("write failed")
	if _, err := parse([]string{"--unknown"}, failingWriter{err: writeErr}); !errors.Is(err, errUsage) || !errors.Is(err, writeErr) {
		t.Fatalf("parse with failed usage writer = %v", err)
	}
	if commandLineError(errors.New("failure")) == nil || commandLineError(errors.New("failure")).Error() != "failure" {
		t.Fatal("commandLineError discarded ordinary failure")
	}
}

func TestObtainEvidenceReadsStdinAndFiles(t *testing.T) {
	t.Parallel()

	evidence, path := writeEvidenceFixture(t)
	// #nosec G304 -- path is a test-owned fixture under t.TempDir.
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fromStdin, err := obtainEvidence(t.Context(), options{fromEvidence: "-"}, bytes.NewReader(encoded))
	if err != nil || fromStdin.EvidenceID != evidence.EvidenceID {
		t.Fatalf("stdin evidence = %q, %v", fromStdin.EvidenceID, err)
	}
	fromFile, err := obtainEvidence(t.Context(), options{fromEvidence: path}, strings.NewReader("ignored"))
	if err != nil || fromFile.EvidenceID != evidence.EvidenceID {
		t.Fatalf("file evidence = %q, %v", fromFile.EvidenceID, err)
	}
	if _, err := obtainEvidence(t.Context(), options{fromEvidence: path + ".missing"}, strings.NewReader("")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing evidence error = %v", err)
	}
}

func TestConfigAndProfilesCommandUsage(t *testing.T) {
	t.Parallel()

	usageCases := [][]string{
		nil, {"unknown"}, {"paths", "extra"}, {"validate", "one", "two"}, {"show", "one", "two"},
	}
	for _, arguments := range usageCases {
		if err := runConfigCommand(arguments, io.Discard, io.Discard); !errors.Is(err, errUsage) {
			t.Fatalf("runConfigCommand(%v) error = %v, want usage", arguments, err)
		}
	}
	if err := runConfigCommand([]string{"--help"}, io.Discard, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("config help error = %v", err)
	}
	writeErr := errors.New("write failed")
	if err := runConfigCommand([]string{"-h"}, io.Discard, failingWriter{err: writeErr}); !errors.Is(err, writeErr) {
		t.Fatalf("config help write error = %v", err)
	}
	if err := runProfilesCommand([]string{"profile"}, io.Discard, io.Discard); !errors.Is(err, errUsage) {
		t.Fatalf("profiles positional error = %v", err)
	}
	if err := runProfilesCommand([]string{"--unknown"}, io.Discard, io.Discard); err == nil {
		t.Fatal("profiles unknown flag error = nil")
	}
}

func TestGeneratorSelectionReportsConfigurationFailures(t *testing.T) {
	if _, err := selectGenerator(options{ai: true, diagnosisConfig: "relative/path.yml"}); err == nil {
		t.Fatal("selectGenerator(relative configuration) error = nil")
	}
	malformed := filepath.Join(t.TempDir(), "malformed.yml")
	if err := os.WriteFile(malformed, []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := selectGenerator(options{ai: true, diagnosisConfig: malformed}); err == nil {
		t.Fatal("selectGenerator(malformed configuration) error = nil")
	}
	configuration := writeDiagnosisConfig(t, "http://127.0.0.1:8000/v1/chat/completions")
	if _, err := selectGenerator(options{
		ai: true, profile: "missing", diagnosisConfig: configuration,
	}); err == nil {
		t.Fatal("selectGenerator(missing profile) error = nil")
	}
}

func TestCommandsPropagateOutputFailures(t *testing.T) {
	writeErr := errors.New("write failed")
	failed := failingWriter{err: writeErr}
	configuration := writeDiagnosisConfig(t, "http://127.0.0.1:8000/v1/chat/completions")
	t.Setenv(diagnosisconfig.EnvironmentPath, configuration)
	_, evidencePath := writeEvidenceFixture(t)

	commands := []struct {
		name string
		run  func() error
	}{
		{name: "version", run: func() error {
			return Run([]string{"--version"}, strings.NewReader(""), failed, io.Discard)
		}},
		{name: "JSON report", run: func() error {
			return Run([]string{"--from-evidence", evidencePath, "--json"}, strings.NewReader(""), failed, io.Discard)
		}},
		{name: "human report", run: func() error {
			return Run([]string{"--from-evidence", evidencePath}, strings.NewReader(""), failed, io.Discard)
		}},
		{name: "bundle JSON inventory", run: func() error {
			return Run([]string{
				"--from-evidence", evidencePath, "--support-bundle", filepath.Join(t.TempDir(), "bundle.tar.gz"),
				"--bundle-dry-run", "--json",
			}, strings.NewReader(""), failed, io.Discard)
		}},
		{name: "config paths", run: func() error { return writeConfigPaths(failed) }},
		{name: "config validate", run: func() error {
			return validateConfigCommand([]string{configuration}, failed)
		}},
		{name: "config show", run: func() error { return showConfigCommand([]string{configuration}, failed) }},
		{name: "profiles", run: func() error {
			return runProfilesCommand([]string{"--diagnosis-config", configuration}, failed, io.Discard)
		}},
	}
	for _, test := range commands {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, writeErr) {
				t.Fatalf("output error = %v, want %v", err, writeErr)
			}
		})
	}
	if err := Run([]string{"config", "--help"}, strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatalf("Run(config --help) error = %v", err)
	}
}
