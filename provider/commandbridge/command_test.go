package commandbridge

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

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

func TestCommandBridgeHelper(_ *testing.T) {
	if os.Getenv("JOBMAN_DIAGNOSE_PROVIDER_PROTOCOL") != "1" {
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
		Kind: provider.ProposalKind, SchemaVersion: 1, RequestID: request.RequestID,
		Hypotheses: []provider.Hypothesis{{
			Code: "generated.bridge_test", Category: "process", Summary: "Bridge hypothesis",
			Explanation:           "The bridge returned a cited test proposal.",
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
		EvidenceID: "sha256:" + strings.Repeat("a", 64),
		Subject:    provider.Subject{Phase: "completed", Outcome: "failure", SelectedRuns: []uint64{1}},
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
		ResponseSchema: provider.ProposalJSONSchema(),
	})
	if err != nil {
		t.Fatal(err)
	}

	return request
}
