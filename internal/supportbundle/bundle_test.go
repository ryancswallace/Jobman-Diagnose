package supportbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

//nolint:cyclop // One archive read verifies ordering, privacy, contents, determinism, and inventory consistency.
func TestBundleIsDeterministicPrivateAndComplete(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", []byte(
		"Traceback (most recent call last):\n  File \"worker.py\", line 3\nValueError: bad value\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	failureEvidence, err := enrichment.Collect(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	diagnostician, err := engine.New("test", func() time.Time {
		return time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), failureEvidence)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := New(failureEvidence, report, Build{Version: "test", Commit: strings.Repeat("a", 40)})
	if err != nil {
		t.Fatal(err)
	}
	var first, second bytes.Buffer
	if err := Encode(&first, bundle); err != nil {
		t.Fatal(err)
	}
	if err := Encode(&second, bundle); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("support bundle encoding is not deterministic")
	}

	files := readArchive(t, first.Bytes())
	want := []string{
		"jobman-diagnosis-support/INVENTORY.txt",
		"jobman-diagnosis-support/analysis-context.json",
		"jobman-diagnosis-support/build.json",
		"jobman-diagnosis-support/capabilities.json",
		"jobman-diagnosis-support/diagnosis.json",
		"jobman-diagnosis-support/disclosure.json",
		"jobman-diagnosis-support/evidence.json",
		"jobman-diagnosis-support/manifest.json",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("archive files = %#v, want %#v", files, want)
	}
	if len(bundle.Inventory.Files) != len(want)-1 || bundle.Inventory.TotalPayloadBytes == 0 {
		t.Fatalf("inventory = %#v", bundle.Inventory)
	}
	var inventory bytes.Buffer
	if err := WriteInventory(&inventory, bundle.Inventory); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inventory.String(), "no archive created") ||
		!strings.Contains(inventory.String(), "evidence.json") ||
		!strings.Contains(inventory.String(), "Provider credentials") {
		t.Fatalf("dry-run inventory = %q", inventory.String())
	}
}

func readArchive(t *testing.T, encoded []byte) []string {
	t.Helper()

	compressed, err := gzip.NewReader(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	tape := tar.NewReader(compressed)
	var files []string
	for {
		header, err := tape.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Mode != 0o600 || header.Uid != 0 || header.Gid != 0 ||
			!strings.HasPrefix(header.Name, "jobman-diagnosis-support/") || strings.Contains(header.Name, "..") {
			t.Fatalf("unsafe archive header = %#v", header)
		}
		files = append(files, header.Name)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}

	return files
}

func TestNewRejectsMismatchedReport(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	failureEvidence, err := diagnosis.CoreFailureEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(failureEvidence, diagnosis.Report{}, Build{Version: "test"}); err == nil {
		t.Fatal("New(mismatched report) error = nil")
	}
}
