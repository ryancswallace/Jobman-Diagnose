package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckAcceptsResolvableLinksAndSkipsCodeFences(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "[docs](docs/guide.md)\n```md\n[ignored](missing.md)\n```\n")
	mustWrite(t, filepath.Join(root, "docs", "guide.md"), "[root](/README.md#top)\n[remote](https://example.com)\n")

	problems, err := check(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %v, want none", problems)
	}
}

func TestCheckReportsMissingRelativeLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "See [missing](docs/missing.md).\n")

	problems, err := check(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 1 {
		t.Fatalf("problems = %v, want one", problems)
	}
}

func TestDestinationsHandleTitlesAndReferenceLinks(t *testing.T) {
	t.Parallel()

	inline := destinations(`[one](docs/one.md "title")`)
	reference := destinations(`[two]: <docs/two.md>`)
	if len(inline) != 1 || inline[0] != "docs/one.md" {
		t.Fatalf("inline destinations = %v", inline)
	}
	if len(reference) != 1 || reference[0] != "docs/two.md" {
		t.Fatalf("reference destinations = %v", reference)
	}
}

func TestResolveDestinationUsesMarkdownRootSemantics(t *testing.T) {
	t.Parallel()

	root := filepath.Join("repository", "root")
	resolved, local := resolveDestination(root, filepath.Join(root, "docs"), "/README.md#top")
	if !local {
		t.Fatal("root-relative Markdown link was not classified as local")
	}
	if want := filepath.Join(root, "README.md"); resolved != want {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}
}

func TestExecuteReportsSuccessBrokenLinksAndUsage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "[valid](guide.md)\n")
	mustWrite(t, filepath.Join(root, "guide.md"), "guide\n")
	var stdout, stderr bytes.Buffer
	if status := execute([]string{"-root", root}, &stdout, &stderr); status != 0 ||
		!strings.Contains(stdout.String(), "links resolve") {
		t.Fatalf("execute(valid) = %d, stdout %q, stderr %q", status, stdout.String(), stderr.String())
	}

	mustWrite(t, filepath.Join(root, "README.md"), "[missing](missing.md)\n")
	stdout.Reset()
	stderr.Reset()
	if status := execute([]string{"-root", root}, &stdout, &stderr); status != 1 ||
		!strings.Contains(stderr.String(), "does not resolve") {
		t.Fatalf("execute(broken) = %d, stderr %q", status, stderr.String())
	}

	for _, arguments := range [][]string{{"-unknown"}, {"positional"}, {"-root", filepath.Join(root, "missing")}} {
		stderr.Reset()
		if status := execute(arguments, &stdout, &stderr); status != 2 {
			t.Errorf("execute(%q) status = %d", arguments, status)
		}
	}
}

func TestMarkdownDestinationClassification(t *testing.T) {
	t.Parallel()

	for _, name := range []string{".git", "bin", "dist", "site-build", "vendor"} {
		if !skippedDirectory(name) {
			t.Errorf("skippedDirectory(%q) = false", name)
		}
	}
	if skippedDirectory("docs") {
		t.Fatal("skippedDirectory(docs) = true")
	}
	for _, destination := range []string{"", "#fragment", "//example.com/path", "https://example.com", "%zz"} {
		if resolved, local := resolveDestination("root", "source", destination); local || resolved != "" {
			t.Errorf("resolveDestination(%q) = %q, %t", destination, resolved, local)
		}
	}
	if got := markdownDestination("<guide.md> title"); got != "guide.md" {
		t.Fatalf("markdownDestination(angle) = %q", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
