package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	amd64Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	arm64Digest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestRun(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	checksums := writeChecksums(t, root, amd64Digest, arm64Digest)
	output := filepath.Join(root, "Formula", "jobman-diagnose.rb")
	if err := run(checksums, output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	formula, err := os.ReadFile(output) // #nosec G304 -- The test controls the temporary path.
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`# typed: strict`,
		`# frozen_string_literal: true`,
		`class JobmanDiagnose < Formula`,
		`depends_on "jobman"`,
		`depends_on :macos`,
		"/releases/download/v1.2.3/jobman-diagnose_1.2.3_darwin_amd64.tar.gz",
		`sha256 "` + amd64Digest + `"`,
		"/releases/download/v1.2.3/jobman-diagnose_1.2.3_darwin_arm64.tar.gz",
		`sha256 "` + arm64Digest + `"`,
		`bin.install "jobman-diagnose"`,
		`assert_match "jobman-diagnose 1.2.3"`,
	} {
		if !strings.Contains(string(formula), want) {
			t.Errorf("formula does not contain %q", want)
		}
	}
	if strings.Contains(string(formula), `version "1.2.3"`) {
		t.Error("formula contains a redundant explicit version")
	}
}

func TestRunRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tests := []struct {
		name     string
		filename string
		contents string
	}{
		{name: "invalid name", filename: "checksums.txt"},
		{
			name:     "prerelease name",
			filename: "jobman-diagnose_1.2.3-rc.1_checksums.txt",
		},
		{
			name:     "missing architecture",
			filename: "jobman-diagnose_1.2.3_checksums.txt",
			contents: amd64Digest + "  jobman-diagnose_1.2.3_darwin_amd64.tar.gz\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checksums := filepath.Join(root, test.name, test.filename)
			if err := os.MkdirAll(filepath.Dir(checksums), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(checksums, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := run(checksums, filepath.Join(root, test.name, "jobman-diagnose.rb")); err == nil {
				t.Fatal("run() error = nil")
			}
		})
	}
}

func TestRunReportsFilesystemFailures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	valid := writeChecksums(t, root, amd64Digest, arm64Digest)
	if err := run("", filepath.Join(root, "jobman-diagnose.rb")); err == nil {
		t.Fatal("run(empty checksums) error = nil")
	}
	if err := run(filepath.Join(root, "missing.txt"), filepath.Join(root, "jobman-diagnose.rb")); err == nil {
		t.Fatal("run(missing checksums) error = nil")
	}

	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(valid, filepath.Join(blocked, "jobman-diagnose.rb")); err == nil {
		t.Fatal("run(blocked output parent) error = nil")
	}
}

func TestReadFormulaDataReportsScannerFailure(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "jobman-diagnose_1.2.3_checksums.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 128*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFormulaData(path); err == nil {
		t.Fatal("readFormulaData(oversized line) error = nil")
	}
}

func TestExecute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	checksums := writeChecksums(t, root, amd64Digest, arm64Digest)
	var stderr bytes.Buffer
	if status := execute([]string{
		"-checksums", checksums,
		"-output", filepath.Join(root, "jobman-diagnose.rb"),
	}, &stderr); status != 0 {
		t.Fatalf("execute(valid) status = %d, stderr = %q", status, stderr.String())
	}
	if status := execute([]string{"-unknown"}, &stderr); status != 1 {
		t.Errorf("execute(invalid flag) status = %d", status)
	}
	if status := execute([]string{"positional"}, &stderr); status != 1 {
		t.Errorf("execute(positional argument) status = %d", status)
	}
}

func TestWriteFormulaReportsFailures(t *testing.T) {
	t.Parallel()

	want := errors.New("failed")
	if err := writeFormula(&failingWriteCloser{writeErr: want}, formulaData{}); !errors.Is(err, want) {
		t.Errorf("writeFormula(write failure) error = %v", err)
	}
	if err := writeFormula(&failingWriteCloser{closeErr: want}, formulaData{}); !errors.Is(err, want) {
		t.Errorf("writeFormula(close failure) error = %v", err)
	}
}

func writeChecksums(t *testing.T, root, amd64, arm64 string) string {
	t.Helper()
	path := filepath.Join(root, "jobman-diagnose_1.2.3_checksums.txt")
	contents := amd64 + "  jobman-diagnose_1.2.3_darwin_amd64.tar.gz\n" +
		arm64 + "  jobman-diagnose_1.2.3_darwin_arm64.tar.gz\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type failingWriteCloser struct {
	writeErr error
	closeErr error
}

func (writer *failingWriteCloser) Write(data []byte) (int, error) {
	if writer.writeErr != nil {
		return 0, writer.writeErr
	}
	return len(data), nil
}

func (writer *failingWriteCloser) Close() error {
	return writer.closeErr
}
