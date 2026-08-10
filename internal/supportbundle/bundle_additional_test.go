package supportbundle

import (
	"bytes"
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
