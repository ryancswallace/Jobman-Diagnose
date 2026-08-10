package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"
)

func TestGenerateEvaluationFixtures(t *testing.T) {
	t.Parallel()

	output := filepath.Join(t.TempDir(), "nested", "evidence")
	if err := generate(output); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 9 {
		t.Fatalf("fixture count = %d", len(entries))
	}
	prior := ""
	for _, entry := range entries {
		if entry.Name() <= prior {
			t.Fatalf("fixtures not sorted: %q before %q", prior, entry.Name())
		}
		prior = entry.Name()
		path := filepath.Join(output, entry.Name())
		// #nosec G304 -- path is built from entries in the test-owned output directory.
		encoded, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, decodeErr := diagnostic.Decode(bytes.NewReader(encoded), diagnostic.DecodeLimits{}); decodeErr != nil {
			t.Fatalf("decode %s: %v", entry.Name(), decodeErr)
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", entry.Name(), info.Mode().Perm())
		}
	}

	// Regeneration is intentionally deterministic and replaces prior contents.
	if err := generate(output); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateEvaluationFixturesRejectsInvalidOutput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generate(path); err == nil {
		t.Fatal("generate(file) error = nil")
	}
}

func TestExecuteEvaluationFixtures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	var stderr bytes.Buffer
	if status := execute([]string{"-output", filepath.Join(root, "fixtures")}, &stderr); status != 0 {
		t.Fatalf("execute(valid) = %d, stderr %q", status, stderr.String())
	}
	for _, arguments := range [][]string{{"-unknown"}, {"positional"}} {
		stderr.Reset()
		if status := execute(arguments, &stderr); status != 2 {
			t.Errorf("execute(%q) = %d", arguments, status)
		}
	}
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if status := execute([]string{"-output", blocked}, &stderr); status != 1 ||
		!strings.Contains(stderr.String(), "generate evaluation fixtures") {
		t.Fatalf("execute(blocked) = %d, stderr %q", status, stderr.String())
	}
}
