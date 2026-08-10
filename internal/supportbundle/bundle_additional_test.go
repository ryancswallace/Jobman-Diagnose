package supportbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/engine"
	"github.com/ryancswallace/jobman-diagnose/internal/enrichment"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestBundleWritersRejectInvalidInputsAndWriteFailures(t *testing.T) {
	t.Parallel()

	if Encode(nil, Bundle{}) == nil || Encode(&bytes.Buffer{}, Bundle{}) == nil {
		t.Fatal("Encode() accepted an invalid destination or bundle")
	}
	invalid := Inventory{}
	if WriteInventory(nil, invalid) == nil || WriteInventory(&bytes.Buffer{}, invalid) == nil {
		t.Fatal("WriteInventory() accepted invalid input")
	}
	if EncodeInventory(nil, invalid) == nil || EncodeInventory(&bytes.Buffer{}, invalid) == nil {
		t.Fatal("EncodeInventory() accepted invalid input")
	}

	bundle := supportBundleFixture(t)
	if err := WriteInventory(failingBundleWriter{}, bundle.Inventory); err == nil {
		t.Fatal("WriteInventory(failing writer) error = nil")
	}
	if err := EncodeInventory(failingBundleWriter{}, bundle.Inventory); err == nil {
		t.Fatal("EncodeInventory(failing writer) error = nil")
	}
	if err := Encode(failingBundleWriter{}, bundle); err == nil {
		t.Fatal("Encode(failing writer) error = nil")
	}
}

func TestNewRequiresBuildVersion(t *testing.T) {
	t.Parallel()

	if _, err := New(bundleEvidence(t), bundleReport(t), Build{}); err == nil {
		t.Fatal("New(empty version) error = nil")
	}
}

func TestInventoryWriterReportsFailuresAtEverySection(t *testing.T) {
	t.Parallel()

	inventory := supportBundleFixture(t).Inventory
	for successfulWrites := 0; successfulWrites < len(inventory.Files)+3; successfulWrites++ {
		writer := &failAfterWrites{remaining: successfulWrites}
		if err := WriteInventory(writer, inventory); err == nil {
			t.Fatalf("WriteInventory(writer failing after %d writes) error = nil", successfulWrites)
		}
	}
}

func TestArchiveCleanupPreservesWriteAndCloseFailures(t *testing.T) {
	t.Parallel()

	cause := errors.New("write failed")
	compressed := gzip.NewWriter(failingBundleWriter{})
	tape := tar.NewWriter(compressed)
	if err := closeArchive(tape, compressed, cause); err == nil || !errors.Is(err, cause) {
		t.Fatalf("closeArchive(tar) error = %v", err)
	}
	compressed = gzip.NewWriter(failingBundleWriter{})
	if err := closeArchive(nil, compressed, cause); err == nil || !errors.Is(err, cause) {
		t.Fatalf("closeArchive(compressor) error = %v", err)
	}

	for limit := 0; limit < 256; limit += 31 {
		writer := &byteLimitWriter{remaining: limit}
		if err := Encode(writer, supportBundleFixture(t)); err == nil {
			t.Fatalf("Encode(writer limited to %d bytes) error = nil", limit)
		}
	}
}

func supportBundleFixture(t *testing.T) Bundle {
	t.Helper()
	bundle, err := New(bundleEvidence(t), bundleReport(t), Build{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func bundleEvidence(t *testing.T) (result diagnosis.FailureEvidence) {
	t.Helper()
	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err = enrichment.Collect(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func bundleReport(t *testing.T) diagnosis.Report {
	t.Helper()
	evidence := bundleEvidence(t)
	diagnostician, err := engine.New("test", func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostician.Diagnose(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

type failingBundleWriter struct{}

func (failingBundleWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type failAfterWrites struct{ remaining int }

func (writer *failAfterWrites) Write(data []byte) (int, error) {
	if writer.remaining == 0 {
		return 0, io.ErrClosedPipe
	}
	writer.remaining--
	return len(data), nil
}

type byteLimitWriter struct{ remaining int }

func (writer *byteLimitWriter) Write(data []byte) (int, error) {
	if len(data) > writer.remaining {
		written := writer.remaining
		writer.remaining = 0
		return written, io.ErrClosedPipe
	}
	writer.remaining -= len(data)
	return len(data), nil
}
