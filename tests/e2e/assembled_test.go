package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

const (
	jobmanBinaryEnvironment   = "JOBMAN_E2E_BINARY"
	diagnoseBinaryEnvironment = "JOBMAN_DIAGNOSE_E2E_BINARY"
)

func TestAssembledCoreAndCompanionCompatibility(t *testing.T) {
	jobman := requiredBinary(t, jobmanBinaryEnvironment)
	diagnose := requiredBinary(t, diagnoseBinaryEnvironment)
	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0o700); err != nil { // #nosec G302 -- private directories require owner execute permission.
		t.Fatalf("secure state directory: %v", err)
	}
	pathEnvironment := filepath.Dir(diagnose) + string(os.PathListSeparator) + os.Getenv("PATH")
	environment := append(os.Environ(), "PATH="+pathEnvironment)

	jobID := strings.TrimSpace(runSuccess(t, environment, jobman,
		"--state-dir", stateDir, "run", "--name", "assembled-failure", "--", "/usr/bin/false"))
	if jobID == "" {
		t.Fatal("jobman run returned an empty job ID")
	}
	runStatus(t, environment, 1, jobman, "--state-dir", stateDir, "wait", jobID)

	evidencePath := filepath.Join(stateDir, "exported-evidence.json")
	direct := decodeReport(t, runSuccess(t, environment, diagnose,
		"--jobman", jobman,
		"--state-dir", stateDir,
		"--export-evidence", evidencePath,
		"--json",
		"assembled-failure",
	))
	extension := decodeReport(t, runSuccess(t, environment, jobman,
		"--state-dir", stateDir,
		"diagnose",
		"--json",
		"assembled-failure",
	))
	imported := decodeReport(t, runSuccess(t, environment, diagnose,
		"--from-evidence", evidencePath,
		"--json",
	))

	for name, report := range map[string]diagnosis.Report{
		"direct": direct, "extension": extension, "imported": imported,
	} {
		t.Run(name, func(t *testing.T) {
			if report.Subject.JobID != jobID || report.Subject.Outcome != "failure" {
				t.Fatalf("subject = %#v, want job %q with failure outcome", report.Subject, jobID)
			}
			primary := primaryFinding(t, report)
			if primary.Code != "core.nonzero_exit" {
				t.Fatalf("primary finding = %#v, want nonzero-exit diagnosis", primary)
			}
			if report.Retry.Verdict != diagnosis.RetryAfterChange {
				t.Fatalf("retry verdict = %q, want %q", report.Retry.Verdict, diagnosis.RetryAfterChange)
			}
			if report.Versions.JobmanVersion == "" || report.Versions.EvidenceSchemaVersion != 1 {
				t.Fatalf("core versions = %#v, want a Jobman version and evidence schema 1", report.Versions)
			}
		})
	}

	assertEquivalent(t, direct, extension)
	assertEquivalent(t, direct, imported)
	if info, err := os.Stat(evidencePath); err != nil {
		t.Fatalf("inspect exported evidence: %v", err)
	} else if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("exported evidence permissions = %04o, want no group or other access", info.Mode().Perm())
	}
}

func requiredBinary(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not set; assembled compatibility tests require prebuilt binaries", name)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		t.Fatalf("resolve %s: %v", name, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		t.Fatalf("inspect %s: %v", name, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("%s does not identify an executable regular file", name)
	}
	return absolute
}

func runSuccess(t *testing.T, environment []string, executable string, arguments ...string) string {
	t.Helper()
	stdout, stderr, err := run(t, environment, executable, arguments...)
	if err != nil {
		t.Fatalf("%s %s: %v\nstderr: %s", executable, strings.Join(arguments, " "), err, stderr)
	}
	return stdout
}

func runStatus(t *testing.T, environment []string, want int, executable string, arguments ...string) {
	t.Helper()
	_, stderr, err := run(t, environment, executable, arguments...)
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != want {
		t.Fatalf("%s %s status error = %v, want %d\nstderr: %s",
			executable, strings.Join(arguments, " "), err, want, stderr)
	}
}

func run(t *testing.T, environment []string, executable string, arguments ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...) // #nosec G204,G702 -- test inputs select prebuilt binaries and fixed arguments.
	command.Env = environment
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s timed out: %v", executable, ctx.Err())
	}
	return stdout.String(), stderr.String(), err
}

func decodeReport(t *testing.T, encoded string) diagnosis.Report {
	t.Helper()
	report, err := diagnosis.Decode(strings.NewReader(encoded), diagnosis.DecodeLimits{})
	if err != nil {
		t.Fatalf("decode assembled diagnosis report: %v\noutput: %s", err, encoded)
	}
	return report
}

func primaryFinding(t *testing.T, report diagnosis.Report) diagnosis.Finding {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.ID == report.PrimaryFindingID {
			return finding
		}
	}
	t.Fatalf("primary finding %q is absent", report.PrimaryFindingID)
	return diagnosis.Finding{}
}

func assertEquivalent(t *testing.T, left, right diagnosis.Report) {
	t.Helper()
	if left.CoreEvidenceID != right.CoreEvidenceID || left.AnalysisEvidenceID != right.AnalysisEvidenceID ||
		!reflect.DeepEqual(left.Subject, right.Subject) || left.PrimaryFindingID != right.PrimaryFindingID ||
		!reflect.DeepEqual(left.Findings, right.Findings) || !reflect.DeepEqual(left.Actions, right.Actions) ||
		!reflect.DeepEqual(left.Retry, right.Retry) || !reflect.DeepEqual(left.Citations, right.Citations) {
		t.Fatalf("assembled diagnosis semantics differ:\nleft:  %#v\nright: %#v", left, right)
	}
}
