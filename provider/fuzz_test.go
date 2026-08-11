package provider

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func FuzzDecodeProposal(f *testing.F) {
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
		AllowedHypothesisCodes: []string{"generated.unknown_target_error"},
		AllowedActions:         []AllowedAction{}, Instructions: RequiredInstructions(),
		MaximumOutputBytes: 16 * 1024,
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(`{"kind":"jobman.diagnosis_proposal","schema_version":2}`))
	f.Add([]byte(`{"kind":"jobman.diagnosis_proposal","kind":"duplicate"}`))
	f.Fuzz(func(_ *testing.T, encoded []byte) {
		if _, err := DecodeProposal(bytes.NewReader(encoded), request); err != nil {
			return
		}
	})
}
