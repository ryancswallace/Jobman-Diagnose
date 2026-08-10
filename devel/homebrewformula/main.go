// Command homebrewformula generates the Jobman Diagnose Homebrew formula from
// a release checksum manifest.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

var (
	versionRE = regexp.MustCompile(`^jobman-diagnose_(\d+\.\d+\.\d+)_checksums\.txt$`)
	digestRE  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type formulaData struct {
	Version      string
	AMD64Archive string
	AMD64Digest  string
	ARM64Archive string
	ARM64Digest  string
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stderr))
}

func execute(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("homebrewformula", flag.ContinueOnError)
	flags.SetOutput(stderr)
	checksums := flags.String("checksums", "", "release checksum manifest")
	output := flags.String("output", "", "formula output path")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() != 0 {
		if _, err := fmt.Fprintln(stderr, "generate Homebrew formula: positional arguments are not accepted"); err != nil {
			return 1
		}
		return 1
	}
	if err := run(*checksums, *output); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "generate Homebrew formula: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}

func run(checksums, output string) error {
	if checksums == "" || output == "" {
		return errors.New("-checksums and -output are required")
	}

	data, err := readFormulaData(checksums)
	if err != nil {
		return err
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(output), 0o750); mkdirErr != nil {
		return fmt.Errorf("create output directory: %w", mkdirErr)
	}

	file, err := os.Create(output) // #nosec G304 -- The caller intentionally selects the generated output path.
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	return writeFormula(file, data)
}

func writeFormula(file io.WriteCloser, data formulaData) error {
	if err := formulaTemplate.Execute(file, data); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("render formula: %w", errors.Join(err, closeErr))
		}
		return fmt.Errorf("render formula: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	return nil
}

func readFormulaData(path string) (formulaData, error) {
	match := versionRE.FindStringSubmatch(filepath.Base(path))
	if match == nil {
		return formulaData{}, fmt.Errorf("invalid checksum manifest name %q", filepath.Base(path))
	}

	data := formulaData{
		Version:      match[1],
		AMD64Archive: "jobman-diagnose_" + match[1] + "_darwin_amd64.tar.gz",
		ARM64Archive: "jobman-diagnose_" + match[1] + "_darwin_arm64.tar.gz",
	}
	file, err := os.Open(path) // #nosec G304 -- The caller intentionally selects the checksum manifest.
	if err != nil {
		return formulaData{}, fmt.Errorf("open checksums: %w", err)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || !digestRE.MatchString(fields[0]) {
			continue
		}
		switch fields[1] {
		case data.AMD64Archive:
			data.AMD64Digest = fields[0]
		case data.ARM64Archive:
			data.ARM64Digest = fields[0]
		}
	}
	if err := scanner.Err(); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return formulaData{}, fmt.Errorf("read checksums: %w", errors.Join(err, closeErr))
		}
		return formulaData{}, fmt.Errorf("read checksums: %w", err)
	}
	if err := file.Close(); err != nil {
		return formulaData{}, fmt.Errorf("close checksums: %w", err)
	}
	if data.AMD64Digest == "" || data.ARM64Digest == "" {
		return formulaData{}, errors.New("checksum manifest is missing a macOS release archive")
	}
	return data, nil
}

var formulaTemplate = template.Must(template.New("formula").Parse(`# typed: strict
# frozen_string_literal: true

# Installs the Jobman Diagnose companion from its verified release archive.
class JobmanDiagnose < Formula
  desc "Deterministic and AI-assisted diagnostics for Jobman failures"
  homepage "https://github.com/ryancswallace/Jobman-Diagnose"
  license "MIT"

  depends_on "jobman"
  depends_on :macos

  on_macos do
    on_intel do
      url "https://github.com/ryancswallace/Jobman-Diagnose/releases/download/v{{ .Version }}/{{ .AMD64Archive }}"
      sha256 "{{ .AMD64Digest }}"
    end
    on_arm do
      url "https://github.com/ryancswallace/Jobman-Diagnose/releases/download/v{{ .Version }}/{{ .ARM64Archive }}"
      sha256 "{{ .ARM64Digest }}"
    end
  end

  def install
    bin.install "jobman-diagnose"
    doc.install "README.md", "CHANGELOG.md", "SECURITY.md", "SUPPORT.md"
    (doc/"guides").install Dir["docs/*.md"]
  end

  test do
    assert_match "jobman-diagnose {{ .Version }}", shell_output("#{bin}/jobman-diagnose --version")
  end
end
`))
