package coreclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"
)

func TestClientFailureBoundaries(t *testing.T) {
	t.Parallel()

	client, err := New(Options{Executable: testExecutable(t), Environment: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Collect(nil, diagnostic.EvidenceRequest{}); err == nil { //nolint:staticcheck // Explicit nil-context contract.
		t.Fatal("Collect(nil) error = nil")
	}
	client.run = func(context.Context, string, []string, []string) ([]byte, []byte, error) {
		return nil, nil, errors.New("exit failure")
	}
	if _, err := client.Collect(t.Context(), diagnostic.EvidenceRequest{}); err == nil || !strings.Contains(err.Error(), "exit failure") {
		t.Fatalf("Collect(empty stderr) error = %v", err)
	}
	client.run = func(context.Context, string, []string, []string) ([]byte, []byte, error) {
		return []byte("not JSON"), nil, nil
	}
	if _, err := client.Collect(t.Context(), diagnostic.EvidenceRequest{}); err == nil || !strings.Contains(err.Error(), "decode output") {
		t.Fatalf("Collect(invalid output) error = %v", err)
	}
}

func TestDecodeEvidenceRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	if _, err := DecodeEvidence(nil); err == nil {
		t.Fatal("DecodeEvidence(nil) error = nil")
	}
	if _, err := DecodeEvidence(coreErrorReader{}); err == nil {
		t.Fatal("DecodeEvidence(failing reader) error = nil")
	}
	for _, encoded := range []string{
		`not-json`,
		`{"schema_version":1,"data":{"evidence":{}}} trailing`,
		`{"schema_version":2,"data":{"evidence":{}}}`,
		`{"schema_version":1,"data":{}} {}`,
	} {
		if _, err := DecodeEvidence(strings.NewReader(encoded)); err == nil {
			t.Fatalf("DecodeEvidence(%q) error = nil", encoded)
		}
	}
}

func TestResolveOptionsRejectsUnsafeExecutables(t *testing.T) {
	t.Parallel()

	if _, _, _, err := resolveOptions(Options{Executable: filepath.Join(t.TempDir(), "missing")}, nil); err == nil {
		t.Fatal("resolveOptions(missing) error = nil")
	}
	if _, _, _, err := resolveOptions(Options{Executable: t.TempDir()}, nil); err == nil {
		t.Fatal("resolveOptions(directory) error = nil")
	}
	path := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if _, _, _, err := resolveOptions(Options{Executable: path}, nil); err == nil {
			t.Fatal("resolveOptions(non-executable) error = nil")
		}
	}
}

func TestExecuteAndCappedBuffer(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		mode      string
		wantError string
	}{
		{mode: "success"},
		{mode: "exit", wantError: "exit status"},
		{mode: "stdout-overflow", wantError: "output exceeds"},
		{mode: "stderr-overflow", wantError: "error output was truncated"},
	}
	for _, test := range tests {
		stdout, stderr, err := execute(t.Context(), executable,
			[]string{"-test.run=^TestCoreClientExecuteHelper$"},
			[]string{"JOBMAN_DIAGNOSE_CORE_HELPER=" + test.mode},
		)
		if test.wantError == "" {
			if err != nil || !strings.HasPrefix(string(stdout), "ok") || len(stderr) != 0 {
				t.Fatalf("execute(%s) = %q / %q / %v", test.mode, stdout, stderr, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
			t.Fatalf("execute(%s) error = %v", test.mode, err)
		}
	}

	buffer := &cappedBuffer{maximum: 3}
	if count, err := buffer.Write([]byte("hello")); err != nil || count != 5 || string(buffer.Bytes()) != "hel" || !buffer.overflow {
		t.Fatalf("cappedBuffer = %q / %t / %d / %v", buffer.Bytes(), buffer.overflow, count, err)
	}
}

func TestCoreClientExecuteHelper(t *testing.T) {
	switch os.Getenv("JOBMAN_DIAGNOSE_CORE_HELPER") {
	case "":
		return
	case "success":
		if _, err := os.Stdout.WriteString("ok"); err != nil {
			os.Exit(20)
		}
	case "exit":
		if _, err := os.Stderr.WriteString("failed"); err != nil {
			os.Exit(21)
		}
		os.Exit(7)
	case "stdout-overflow":
		if _, err := io.CopyN(os.Stdout, bytes.NewReader(bytes.Repeat([]byte("x"), maximumCoreOutputBytes+1)), maximumCoreOutputBytes+1); err != nil {
			os.Exit(22)
		}
	case "stderr-overflow":
		if _, err := io.CopyN(os.Stderr, bytes.NewReader(bytes.Repeat([]byte("x"), maximumCoreErrorBytes+1)), maximumCoreErrorBytes+1); err != nil {
			os.Exit(23)
		}
	default:
		t.Fatalf("unknown helper mode")
	}
}

type coreErrorReader struct{}

func (coreErrorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
