package coreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestClientCollectPreservesArgumentsAndDisablesExtensions(t *testing.T) {
	t.Parallel()

	executable := testExecutable(t)
	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(struct {
		SchemaVersion int `json:"schema_version"`
		Data          struct {
			Evidence diagnostic.Evidence `json:"evidence"`
		} `json:"data"`
	}{SchemaVersion: 1, Data: struct {
		Evidence diagnostic.Evidence `json:"evidence"`
	}{Evidence: evidence}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(Options{
		Executable: executable, StateDir: "/state with space", ConfigPath: "/config file",
		Environment: []string{"EXAMPLE=value", "JOBMAN_NO_EXTENSIONS=0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotPath string
	var gotArguments, gotEnvironment []string
	client.run = func(_ context.Context, path string, arguments, environment []string) ([]byte, []byte, error) {
		gotPath = path
		gotArguments = append([]string(nil), arguments...)
		gotEnvironment = append([]string(nil), environment...)
		return envelope, nil, nil
	}
	decoded, err := client.Collect(t.Context(), diagnostic.EvidenceRequest{
		Selector: "job name", Run: -1, IncludeCommand: true, IncludePaths: true, IncludeEnvironmentNames: true,
		Logs: diagnostic.LogsTail, LogBytes: 8192, Similar: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.EvidenceID != evidence.EvidenceID || gotPath != executable {
		t.Fatalf("decoded/path = %q/%q", decoded.EvidenceID, gotPath)
	}
	wantArguments := []string{
		"--state-dir", "/state with space", "--config", "/config file", "show", "evidence",
		"--run=-1", "--command", "--paths", "--environment-names", "--logs", "tail",
		"--log-bytes", "8192B", "--similar", "2", "--json", "job name",
	}
	if !reflect.DeepEqual(gotArguments, wantArguments) {
		t.Fatalf("arguments = %#v, want %#v", gotArguments, wantArguments)
	}
	if environmentValue(gotEnvironment, "JOBMAN_NO_EXTENSIONS") != "1" {
		t.Fatalf("environment = %#v", gotEnvironment)
	}
}

func TestNewUsesValidatedExtensionProtocolContext(t *testing.T) {
	t.Parallel()

	executable := testExecutable(t)
	client, err := New(Options{Environment: []string{
		"JOBMAN_EXTENSION_PROTOCOL=1", "JOBMAN_EXECUTABLE=" + executable,
		"JOBMAN_STATE_DIR=/state", "JOBMAN_CONFIG=/config",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if client.executable != executable || client.stateDir != "/state" || client.configPath != "/config" {
		t.Fatalf("client = %#v", client)
	}
	if _, err := New(Options{Environment: []string{"JOBMAN_EXTENSION_PROTOCOL=2"}}); err == nil {
		t.Fatal("New(unsupported protocol) error = nil")
	}
}

func TestDecodeEvidenceAcceptsRawAndEnvelopeAndBoundsInput(t *testing.T) {
	t.Parallel()

	evidence, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	if encodeErr := diagnostic.Encode(&raw, evidence); encodeErr != nil {
		t.Fatal(encodeErr)
	}
	decoded, err := DecodeEvidence(bytes.NewReader(raw.Bytes()))
	if err != nil || decoded.EvidenceID != evidence.EvidenceID {
		t.Fatalf("DecodeEvidence(raw) = %q, %v", decoded.EvidenceID, err)
	}
	if _, err := DecodeEvidence(strings.NewReader(`{"schema_version":1,"data":{}}`)); err == nil {
		t.Fatal("DecodeEvidence(incomplete envelope) error = nil")
	}
	if _, err := DecodeEvidence(bytes.NewReader(bytes.Repeat([]byte("x"), maximumCoreOutputBytes+1))); err == nil {
		t.Fatal("DecodeEvidence(oversized) error = nil")
	}
}

func TestClientIncludesBoundedCoreError(t *testing.T) {
	t.Parallel()

	client, err := New(Options{Executable: testExecutable(t), Environment: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	client.run = func(context.Context, string, []string, []string) ([]byte, []byte, error) {
		return nil, []byte("job not found\n"), errors.New("exit status 3")
	}
	if _, err := client.Collect(t.Context(), diagnostic.EvidenceRequest{Selector: "missing"}); err == nil || !strings.Contains(err.Error(), "job not found") {
		t.Fatalf("Collect() error = %v", err)
	}
}

func testExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jobman")
	// #nosec G306 -- an executable test fixture intentionally needs execute permission.
	if err := os.WriteFile(path, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}

	return absolute
}
