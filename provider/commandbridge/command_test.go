package commandbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman-diagnose/provider"
)

func TestCommandBridgeGeneratesWithoutInheritingAmbientEnvironment(t *testing.T) {
	t.Setenv("UNRELATED_PROVIDER_SECRET", "must-not-reach-child")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	generator, err := New(Config{
		Executable: executable, Arguments: []string{"-test.run=^TestCommandBridgeHelper$"},
		Model: "test", Credential: []byte("explicit-credential"),
		MaximumInputBytes: 64 * 1024, MaximumOutputBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := bridgeRequest(t)
	response, err := generator.Generate(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := provider.DecodeProposal(bytes.NewReader(response.JSON), request)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.RequestID != request.RequestID || response.Provider != "command" {
		t.Fatalf("response/proposal = %#v / %#v", response, proposal)
	}
}

func TestCommandBridgeBoundsOutputAndHidesStderr(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"oversize", "exit"} {
		generator, newErr := New(Config{
			Executable: executable, Arguments: []string{"-test.run=^TestCommandBridgeHelper$"},
			Model: model, MaximumInputBytes: 64 * 1024, MaximumOutputBytes: 1024,
		})
		if newErr != nil {
			t.Fatal(newErr)
		}
		if _, generateErr := generator.Generate(t.Context(), bridgeRequest(t)); generateErr == nil ||
			strings.Contains(generateErr.Error(), "stderr-secret-canary") {
			t.Fatalf("Generate(%s) error = %v", model, generateErr)
		}
	}
}

func TestCommandBridgeValidatesConfigurationAndClonesSecrets(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{}); err == nil {
		t.Fatal("New(empty configuration) error = nil")
	}
	missing := filepath.Join(t.TempDir(), "missing-provider")
	if _, err := New(Config{
		Executable: missing, Model: "test", MaximumInputBytes: 1, MaximumOutputBytes: 1,
	}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("New(missing executable) error = %v", err)
	}
	nonExecutable := filepath.Join(t.TempDir(), "provider")
	if err := os.WriteFile(nonExecutable, []byte("provider"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertNonExecutableRejected(t, nonExecutable)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, newErr := New(Config{
		Executable: executable, Arguments: []string{strings.Repeat("x", 4097)}, Model: "test",
		MaximumInputBytes: 1, MaximumOutputBytes: 1,
	}); newErr == nil {
		t.Fatal("New(oversized argument) error = nil")
	}
	arguments := []string{"-test.run=^TestCommandBridgeHelper$"}
	credential := []byte("credential")
	generator, err := New(Config{
		Executable: executable, Arguments: arguments, Model: "model", Credential: credential,
		MaximumInputBytes: 4096, MaximumOutputBytes: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments[0] = "changed"
	credential[0] = 'X'
	if generator.config.Arguments[0] == "changed" || string(generator.config.Credential) != "credential" {
		t.Fatal("generator retained caller-owned configuration slices")
	}
	capabilities := generator.Capabilities()
	if generator.Name() != "command" || !capabilities.NativeJSONSchema ||
		capabilities.Locality != provider.LocalityLocal || capabilities.MaximumInputBytes != 4096 ||
		capabilities.MaximumOutputBytes != 2048 {
		t.Fatalf("generator description = %q / %#v", generator.Name(), capabilities)
	}
}

func assertNonExecutableRejected(t *testing.T, executable string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if _, err := New(Config{
		Executable: executable, Model: "test", MaximumInputBytes: 1, MaximumOutputBytes: 1,
	}); err == nil {
		t.Fatal("New(non-executable file) error = nil")
	}
}

func TestCommandBridgeClassifiesRequestBoundaryFailures(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	base, err := New(Config{
		Executable: executable, Arguments: []string{"-test.run=^TestCommandBridgeHelper$"},
		Model: "test", MaximumInputBytes: 64 * 1024, MaximumOutputBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, generateErr := base.Generate(nil, bridgeRequest(t)) //nolint:staticcheck // Explicit nil-context contract.
	assertFailureCode(t, generateErr, provider.FailureInvalidRequest)
	_, generateErr = base.Generate(t.Context(), provider.Request{})
	assertFailureCode(t, generateErr, provider.FailureInvalidRequest)

	inputLimited := *base
	inputLimited.config.MaximumInputBytes = 1
	_, generateErr = inputLimited.Generate(t.Context(), bridgeRequest(t))
	assertFailureCode(t, generateErr, provider.FailureInputOversized)

	missing := *base
	missing.config.Executable = filepath.Join(t.TempDir(), "removed-provider")
	_, generateErr = missing.Generate(t.Context(), bridgeRequest(t))
	assertFailureCode(t, generateErr, provider.FailureRequestFailed)

	empty := *base
	empty.config.Model = "empty"
	_, generateErr = empty.Generate(t.Context(), bridgeRequest(t))
	assertFailureCode(t, generateErr, provider.FailureOutputEmpty)

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, generateErr = base.Generate(canceled, bridgeRequest(t))
	assertFailureCode(t, generateErr, provider.FailureRequestCanceled)

	blocking := *base
	blocking.config.Model = "block"
	deadline, cancelDeadline := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancelDeadline()
	_, generateErr = blocking.Generate(deadline, bridgeRequest(t))
	assertFailureCode(t, generateErr, provider.FailureRequestTimeout)
}

func assertFailureCode(t *testing.T, err error, want provider.FailureCode) {
	t.Helper()
	code, _, ok := provider.Diagnostic(err)
	if !ok || code != want {
		t.Fatalf("failure = %q/%t (%v), want %q", code, ok, err, want)
	}
}

func TestCommandBridgeHelper(_ *testing.T) {
	if os.Getenv("JOBMAN_DIAGNOSE_PROVIDER_PROTOCOL") != "3" {
		return
	}
	if os.Getenv("UNRELATED_PROVIDER_SECRET") != "" {
		if _, err := os.Stderr.WriteString("ambient environment leaked"); err != nil {
			os.Exit(12)
		}
		os.Exit(9)
	}
	model := os.Getenv("JOBMAN_DIAGNOSE_PROVIDER_MODEL")
	switch model {
	case "empty":
		if err := os.Stdout.Close(); err != nil {
			os.Exit(15)
		}
		os.Exit(0)
	case "block":
		time.Sleep(time.Minute)
		return
	case "oversize":
		if _, err := os.Stdout.WriteString(strings.Repeat("x", 4096)); err != nil {
			os.Exit(13)
		}
		return
	case "exit":
		if _, err := os.Stderr.WriteString("stderr-secret-canary"); err != nil {
			os.Exit(14)
		}
		os.Exit(7)
	case "test":
		if os.Getenv("JOBMAN_DIAGNOSE_PROVIDER_CREDENTIAL") != "explicit-credential" {
			os.Exit(8)
		}
	}
	request, err := provider.DecodeRequest(os.Stdin, 64*1024)
	if err != nil {
		os.Exit(10)
	}
	proposal := provider.Proposal{
		Kind: provider.ProposalKind, SchemaVersion: provider.ProposalSchemaVersion, RequestID: request.RequestID,
		Hypotheses: []provider.Hypothesis{{
			Code: "generated.bridge_test", Category: "process", Summary: "Bridge hypothesis",
			RootCause:             "The projected worker setting is incompatible with the bridge runtime.",
			Explanation:           "Runtime validation rejects the setting before the worker starts.",
			SupportingEvidence:    []string{request.Manifest.ItemIDs[0]},
			ContradictingEvidence: []string{}, ContradictsFindings: []string{},
		}},
		RecommendedActions: []string{}, MissingEvidence: []provider.MissingEvidence{},
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		os.Exit(11)
	}
	os.Exit(0)
}

func bridgeRequest(t *testing.T) provider.Request {
	t.Helper()
	request, err := provider.SealRequest(provider.Request{
		AnalysisEvidenceID: "sha256:" + strings.Repeat("a", 64),
		Subject:            provider.Subject{Phase: "completed", Outcome: "failure", SelectedRuns: []uint64{1}},
		Projection: provider.Projection{Items: []provider.ProjectedItem{{
			ID: "ev:run:1:exit", Code: "jobman.run.exit.code", Value: json.RawMessage(`7`),
			Quality: "observed", Disclosure: "metadata",
		}}},
		Manifest: provider.ProjectionManifest{
			Classes: []string{"metadata"}, ItemIDs: []string{"ev:run:1:exit"}, ItemCount: 1,
		},
		Deterministic: []provider.DeterministicCandidate{{
			ID: "finding:001", Code: "core.nonzero_exit", Category: "process", Summary: "Nonzero exit",
			Explanation: "The exit was observed.", SupportingEvidence: []string{"ev:run:1:exit"},
			ContradictingEvidence: []string{},
		}},
		AllowedCategories:      []string{"process"},
		AllowedHypothesisCodes: []string{"generated.bridge_test"}, AllowedActions: []provider.AllowedAction{},
		Instructions: provider.RequiredInstructions(), MaximumOutputBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	return request
}
