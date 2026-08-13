package sourcecontext

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestCollectLimitedInfersSourceAndRuntimeLine(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "worker.py")
	lines := make([]string, 60)
	for index := range lines {
		lines[index] = fmt.Sprintf("value_%02d = %d", index+1, index+1)
	}
	writeSource(t, path, strings.Join(lines, "\n")+"\n")
	log := []byte("Traceback (most recent call last):\n  File \"worker.py\", line 45, in main\nValueError: invalid batch size\n")
	evidence := sourceEvidence(t, directory, diagnostic.Command{
		Executable: "python3", Arguments: []string{"worker.py"},
	}, log)

	contexts, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextLimited, MaximumBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 1 {
		t.Fatalf("contexts = %#v", contexts)
	}
	context := contexts[0]
	if context.Path != path || context.AnchorLine != 45 || context.AnchorReason != "runtime_log" ||
		context.StartLine != 25 || context.EndLine != 60 || context.Mode != diagnosis.SourceContextLimited {
		t.Fatalf("context selection = %#v", context)
	}
	if !strings.HasPrefix(string(context.Data), "value_25 = 25\n") ||
		!strings.HasSuffix(string(context.Data), "value_60 = 60\n") {
		t.Fatalf("selected data = %q", context.Data)
	}
	sealed, err := diagnosis.SealFailureEvidenceWithContext(evidence, nil, contexts)
	if err != nil {
		t.Fatal(err)
	}
	if err := diagnosis.VerifyFailureEvidence(sealed); err != nil {
		t.Fatal(err)
	}
}

func TestCollectLimitedUsesExplicitLineAndProfileBound(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "main.go")
	writeSource(t, path, strings.Repeat("package main // source line\n", 50))
	evidence := sourceEvidence(t, filepath.Dir(path), diagnostic.Command{
		Executable: "go", Arguments: []string{"run", "main.go"},
	}, nil)
	contexts, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextLimited, File: path, Line: 2, MaximumBytes: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	if contexts[0].StartLine != 1 || contexts[0].EndLine != 22 || contexts[0].AnchorReason != "explicit_line" {
		t.Fatalf("context = %#v", contexts[0])
	}
	if _, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextLimited, File: path, Line: 2, MaximumBytes: 10,
	}); err == nil || !strings.Contains(err.Error(), "profile allows 10") {
		t.Fatalf("Collect(undersized profile) error = %v", err)
	}
}

func TestCollectLimitedUsesConfiguredSymmetricRadius(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "worker.py")
	lines := make([]string, 30)
	for index := range lines {
		lines[index] = fmt.Sprintf("line_%02d", index+1)
	}
	writeSource(t, path, strings.Join(lines, "\n")+"\n")
	evidence := sourceEvidence(t, filepath.Dir(path), diagnostic.Command{
		Executable: "python3", Arguments: []string{path},
	}, nil)
	contexts, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextLimited, File: path, Line: 15,
		LinesBeforeAndAfter: 3, MaximumBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if contexts[0].StartLine != 12 || contexts[0].EndLine != 18 ||
		!strings.HasPrefix(string(contexts[0].Data), "line_12\n") ||
		!strings.HasSuffix(string(contexts[0].Data), "line_18\n") {
		t.Fatalf("configured context = %#v", contexts[0])
	}
}

func TestCollectFullRequiresExactBoundedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.js")
	content := "function main() {\n  throw new Error('boom');\n}\n"
	writeSource(t, path, content)
	evidence := sourceEvidence(t, filepath.Dir(path), diagnostic.Command{
		Executable: "node", Arguments: []string{path},
	}, nil)
	contexts, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextFull, MaximumBytes: uint64(len(content)),
	})
	if err != nil {
		t.Fatal(err)
	}
	context := contexts[0]
	if string(context.Data) != content || context.ByteStart != 0 || context.ByteEnd != uint64(len(content)) ||
		context.Digest != context.ContentDigest || context.AnchorLine != 0 || context.AnchorReason != "full_file" {
		t.Fatalf("full context = %#v", context)
	}
	if _, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextFull, Line: 1, MaximumBytes: uint64(len(content)),
	}); err == nil {
		t.Fatal("Collect(full with line) error = nil")
	}
}

func TestCollectRejectsAmbiguousUnsafeAndInvalidSources(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	first := filepath.Join(directory, "first.py")
	second := filepath.Join(directory, "second.py")
	writeSource(t, first, "print('first')\n")
	writeSource(t, second, "print('second')\n")
	ambiguous := sourceEvidence(t, directory, diagnostic.Command{
		Executable: "python3", Arguments: []string{"first.py", "second.py"},
	}, nil)
	if _, err := Collect(t.Context(), ambiguous, Options{
		Mode: diagnosis.SourceContextLimited, MaximumBytes: 4096,
	}); err == nil || !strings.Contains(err.Error(), "multiple source files") {
		t.Fatalf("Collect(ambiguous) error = %v", err)
	}

	link := filepath.Join(directory, "linked.py")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(t.Context(), ambiguous, Options{
		Mode: diagnosis.SourceContextFull, File: link, MaximumBytes: 4096,
	}); err == nil || !strings.Contains(err.Error(), "regular, non-symlink") {
		t.Fatalf("Collect(symlink) error = %v", err)
	}

	invalid := filepath.Join(directory, "invalid.py")
	if err := os.WriteFile(invalid, []byte{0xff, 0x00}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(t.Context(), ambiguous, Options{
		Mode: diagnosis.SourceContextFull, File: invalid, MaximumBytes: 4096,
	}); err == nil || !strings.Contains(err.Error(), "NUL-free UTF-8") {
		t.Fatalf("Collect(invalid text) error = %v", err)
	}
}

func TestCollectRejectsInvalidOptionsAndCancellation(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(nil, evidence, Options{}); err == nil { //nolint:staticcheck // Tests the nil-context contract.
		t.Fatal("Collect(nil) error = nil")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Collect(ctx, evidence, Options{}); err == nil {
		t.Fatal("Collect(canceled) error = nil")
	}
	if _, err := Collect(t.Context(), evidence, Options{Mode: "unknown", MaximumBytes: 1}); err == nil {
		t.Fatal("Collect(unknown mode) error = nil")
	}
	if _, err := Collect(t.Context(), evidence, Options{Mode: diagnosis.SourceContextLimited}); err == nil {
		t.Fatal("Collect(zero limit) error = nil")
	}
	if _, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextLimited, MaximumBytes: 1,
		LinesBeforeAndAfter: maximumLinesBeforeAndAfter + 1,
	}); err == nil {
		t.Fatal("Collect(excessive context radius) error = nil")
	}
	if _, err := Collect(t.Context(), evidence, Options{
		Mode: diagnosis.SourceContextFull, MaximumBytes: 1, LinesBeforeAndAfter: 1,
	}); err == nil {
		t.Fatal("Collect(full with context radius) error = nil")
	}
}

func sourceEvidence(
	t *testing.T,
	workingDirectory string,
	command diagnostic.Command,
	stderr []byte,
) diagnostic.Evidence {
	t.Helper()
	evidence, err := testevidence.Failed("nonzero_exit", stderr)
	if err != nil {
		t.Fatal(err)
	}
	commandValue, err := diagnostic.JSONValue(command)
	if err != nil {
		t.Fatal(err)
	}
	pathValue, err := diagnostic.JSONValue(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	source := diagnostic.ItemSource{Kind: "job_snapshot", EntityID: evidence.Subject.JobID, Revision: evidence.Subject.JobRevision}
	evidence.Items = append(evidence.Items,
		diagnostic.Item{
			ID: "ev:job:target:command", Code: diagnostic.CodeTargetCommand, Value: commandValue,
			Source: source, Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureCommand,
		},
		diagnostic.Item{
			ID: "ev:job:target:working_directory", Code: diagnostic.CodeTargetWorkingDirectory, Value: pathValue,
			Source: source, Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosurePath,
		},
	)
	evidence.EvidenceID = ""
	sealed, err := diagnostic.Seal(evidence)
	if err != nil {
		t.Fatal(err)
	}

	return sealed
}

func writeSource(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
