package main

import (
	"os"
	"path/filepath"
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

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
