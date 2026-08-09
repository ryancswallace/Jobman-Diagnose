package provider

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestAndProposalProtocolRoundTrip(t *testing.T) {
	t.Parallel()

	request := validRequest(t)
	var encoded bytes.Buffer
	if err := EncodeRequest(&encoded, request); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRequest(&encoded, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.RequestID != request.RequestID {
		t.Fatalf("request ID = %q, want %q", decoded.RequestID, request.RequestID)
	}
	proposal := Proposal{
		Kind: ProposalKind, SchemaVersion: ProposalSchemaVersion, RequestID: request.RequestID,
		Hypotheses: []Hypothesis{{
			Code: "generated.configuration_mismatch", Category: "process",
			Summary: "Configuration may differ", Explanation: "The cited exit fact is consistent with this alternative.",
			SupportingEvidence: []string{"ev:run:1:exit"}, ContradictingEvidence: []string{},
			ContradictsFindings: []string{"finding:001"},
		}},
		RecommendedActions: []string{"action:001"},
		MissingEvidence:    []MissingEvidence{{Code: "generated.target_error", Description: "A bounded target error excerpt."}},
	}
	proposalJSON, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := DecodeProposal(bytes.NewReader(proposalJSON), request)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Hypotheses[0].Code != proposal.Hypotheses[0].Code {
		t.Fatalf("proposal = %#v", validated)
	}
}

func TestProposalRejectsUnknownAuthorityAndInventedEvidence(t *testing.T) {
	t.Parallel()

	request := validRequest(t)
	tests := map[string]string{
		"unknown retry field": `{"kind":"jobman.diagnosis_proposal","schema_version":1,"request_id":"` + request.RequestID + `","hypotheses":[],"recommended_action_ids":[],"missing_evidence":[],"retry":"now"}`,
		"invented citation":   `{"kind":"jobman.diagnosis_proposal","schema_version":1,"request_id":"` + request.RequestID + `","hypotheses":[{"code":"generated.guess","category":"process","summary":"Guess","explanation":"Guess","supporting_evidence":["invented"],"contradicting_evidence":[],"contradicts_findings":[]}],"recommended_action_ids":[],"missing_evidence":[]}`,
		"duplicate key":       `{"kind":"jobman.diagnosis_proposal","kind":"jobman.diagnosis_proposal","schema_version":1,"request_id":"` + request.RequestID + `","hypotheses":[],"recommended_action_ids":[],"missing_evidence":[]}`,
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeProposal(strings.NewReader(encoded), request); err == nil {
				t.Fatal("DecodeProposal() error = nil")
			}
		})
	}
}

func TestProposalSchemaIsReviewedStrictObject(t *testing.T) {
	t.Parallel()

	var schema map[string]any
	if err := json.Unmarshal(ProposalJSONSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("schema root = %#v", schema)
	}
	encoded := string(ProposalJSONSchema())
	for _, forbidden := range []string{`"retry"`, `"command"`, `"tool"`, `"url"`} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("proposal schema contains forbidden authority %s", forbidden)
		}
	}
}

func validRequest(t *testing.T) Request {
	t.Helper()
	request, err := SealRequest(Request{
		EvidenceID: "sha256:" + strings.Repeat("a", 64),
		Subject:    Subject{Phase: "completed", Outcome: "failure", SelectedRuns: []uint64{1}},
		Projection: Projection{Items: []ProjectedItem{{
			ID: "ev:run:1:exit", Code: "jobman.run.exit.code", Value: json.RawMessage(`7`),
			Quality: "observed", Disclosure: "metadata",
		}}},
		Manifest: ProjectionManifest{
			Classes: []string{"metadata"}, ItemIDs: []string{"ev:run:1:exit"}, ItemCount: 1,
		},
		Deterministic: []DeterministicCandidate{{
			ID: "finding:001", Code: "core.nonzero_exit", Category: "process", Summary: "Nonzero exit",
			Explanation: "The exit was observed.", SupportingEvidence: []string{"ev:run:1:exit"},
			ContradictingEvidence: []string{},
		}},
		AllowedCategories:      []string{"process"},
		AllowedHypothesisCodes: []string{"generated.configuration_mismatch"},
		AllowedActions: []AllowedAction{{
			ID: "action:001", Code: "inspect_evidence", Summary: "Inspect evidence", Description: "Inspect it locally.",
		}},
		Instructions: RequiredInstructions(), MaximumOutputBytes: 16 * 1024, ResponseSchema: ProposalJSONSchema(),
	})
	if err != nil {
		t.Fatal(err)
	}

	return request
}
