package provider

import (
	"bytes"
	"encoding/json"
	"slices"
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
			Summary:            "The worker configuration uses an unsupported deployment region",
			RootCause:          "The selected region is not enabled for the worker deployment.",
			Explanation:        "Worker initialization rejects the unsupported region before processing begins.",
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
		"unknown retry field": `{"kind":"jobman.diagnosis_proposal","schema_version":2,"request_id":"` + request.RequestID + `","hypotheses":[],"recommended_action_ids":[],"missing_evidence":[],"retry":"now"}`,
		"invented citation":   `{"kind":"jobman.diagnosis_proposal","schema_version":2,"request_id":"` + request.RequestID + `","hypotheses":[{"code":"generated.guess","category":"process","summary":"Specific guess","root_cause":"A guessed condition exists.","explanation":"That condition prevents startup.","supporting_evidence":["invented"],"contradicting_evidence":[],"contradicts_findings":[]}],"recommended_action_ids":[],"missing_evidence":[]}`,
		"duplicate key":       `{"kind":"jobman.diagnosis_proposal","kind":"jobman.diagnosis_proposal","schema_version":2,"request_id":"` + request.RequestID + `","hypotheses":[],"recommended_action_ids":[],"missing_evidence":[]}`,
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

//nolint:cyclop,gocognit // One protocol test intentionally checks every specialized schema authority and bound.
func TestSealedRequestSpecializesProposalSchemaAuthority(t *testing.T) {
	t.Parallel()

	request := validRequest(t)
	if RequestSchemaVersion != 2 {
		t.Fatalf("request schema constant = %d", RequestSchemaVersion)
	}
	if request.SchemaVersion != RequestSchemaVersion {
		t.Fatalf("request schema version = %d", request.SchemaVersion)
	}
	var schema map[string]any
	if err := json.Unmarshal(request.ResponseSchema, &schema); err != nil {
		t.Fatal(err)
	}
	properties := mustSchemaObject(t, schema, "properties")
	requestID := mustSchemaObject(t, properties, "request_id")
	if requestID["const"] != request.RequestID || requestID["pattern"] != nil {
		t.Fatalf("request ID schema = %#v", requestID)
	}
	hypothesis := mustSchemaObject(t, properties, "hypotheses", "items", "properties")
	if ProposalSchemaVersion != 2 || mustSchemaObject(t, properties, "schema_version")["const"] != float64(2) {
		t.Fatalf("proposal schema version = %d / %#v", ProposalSchemaVersion, properties["schema_version"])
	}
	if got := schemaEnum(t, mustSchemaObject(t, hypothesis, "code")); !slices.Equal(got, request.AllowedHypothesisCodes) {
		t.Fatalf("hypothesis code enum = %v", got)
	}
	if got := schemaEnum(t, mustSchemaObject(t, hypothesis, "category")); !slices.Equal(got, request.AllowedCategories) {
		t.Fatalf("category enum = %v", got)
	}
	for _, name := range []string{"supporting_evidence", "contradicting_evidence"} {
		field := mustSchemaObject(t, hypothesis, name)
		if field["description"] == "" {
			t.Fatalf("%s description is empty", name)
		}
		if got := schemaEnum(t, mustSchemaObject(t, field, "items")); !slices.Equal(got, request.Manifest.ItemIDs) {
			t.Fatalf("%s enum = %v", name, got)
		}
	}
	if minimum := mustSchemaObject(t, hypothesis, "supporting_evidence")["minItems"]; minimum != float64(1) {
		t.Fatalf("supporting evidence minItems = %#v", minimum)
	}
	if got := schemaEnum(t, mustSchemaObject(t, hypothesis, "contradicts_findings", "items")); !slices.Equal(got, []string{"finding:001"}) {
		t.Fatalf("finding enum = %v", got)
	}
	if got := schemaEnum(t, mustSchemaObject(t, properties, "recommended_action_ids", "items")); !slices.Equal(got, []string{"action:001"}) {
		t.Fatalf("action enum = %v", got)
	}
	if strings.Contains(string(request.ResponseSchema), `"uniqueItems"`) {
		t.Fatal("request schema uses uniqueItems, which is unsupported by the required xgrammar backend")
	}
	for _, name := range []string{"summary", "root_cause", "explanation"} {
		if mustSchemaObject(t, hypothesis, name)["description"] == "" {
			t.Fatalf("%s specificity guidance is empty", name)
		}
	}
	if mustSchemaObject(t, hypothesis, "summary")["maxLength"] != float64(maximumSummaryText) ||
		mustSchemaObject(t, hypothesis, "root_cause")["maxLength"] != float64(maximumCauseText) ||
		mustSchemaObject(t, hypothesis, "explanation")["maxLength"] != float64(maximumCauseText) ||
		mustSchemaObject(t, hypothesis, "supporting_evidence")["maxItems"] != float64(maximumReferences) {
		t.Fatal("proposal schema text or citation limits diverged from host validation")
	}
}

func TestEncodedRequestKeepsTrustedInstructionsAfterUntrustedArtifacts(t *testing.T) {
	t.Parallel()

	base := validRequest(t)
	base.RequestID = ""
	base.ResponseSchema = nil
	base.Projection.Artifacts = []ProjectedArtifact{{
		ID: "artifact:stderr", Role: "target_stderr", Run: 1, Stream: "stderr",
		Content: "untrusted-artifact-marker", Encoding: "utf-8-lossy",
		Digest: "sha256:" + strings.Repeat("c", 64), SelectedBytes: 25, ContentBytes: 25,
		Disclosure: "log_content",
	}}
	base.Manifest.Classes = []string{"log_content", "metadata"}
	base.Manifest.ArtifactIDs = []string{"artifact:stderr"}
	base.Manifest.ArtifactCount = 1
	base.Manifest.ArtifactBytes = 25
	request, err := SealRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	schemaIndex := strings.Index(value, `"response_schema"`)
	artifactIndex := strings.Index(value, "untrusted-artifact-marker")
	instructionIndex := strings.LastIndex(value, "Do not propose commands")
	if schemaIndex < 0 || artifactIndex <= schemaIndex || instructionIndex <= artifactIndex {
		t.Fatalf("request attention order is schema=%d artifact=%d instructions=%d", schemaIndex, artifactIndex, instructionIndex)
	}
}

func TestRequiredInstructionsDescribeRelationalRules(t *testing.T) {
	t.Parallel()

	instructions := strings.Join(RequiredInstructions(), "\n")
	for _, expected := range []string{
		"never cite the same evidence as both supporting and contradicting",
		"application_configuration means a rejected",
		"unknown_target_error is a last resort",
		"smallest directly relevant evidence set",
		"All three text fields must add distinct information",
		"Never describe a traceback, sanitized byte range",
		"deterministic candidates as confirmed framing",
	} {
		if !strings.Contains(instructions, expected) {
			t.Fatalf("request instructions omit %q", expected)
		}
	}
}

func TestSealedRequestConstrainsEmptyActionCatalogAndDerivedSchema(t *testing.T) {
	t.Parallel()

	base := validRequest(t)
	base.RequestID = ""
	base.ResponseSchema = nil
	base.AllowedActions = nil
	first, err := SealRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := base
	secondInput.ResponseSchema = nil
	second, err := SealRequest(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestID != second.RequestID || !bytes.Equal(first.ResponseSchema, second.ResponseSchema) {
		t.Fatal("derived request identity or response schema is nondeterministic")
	}
	var schema map[string]any
	if decodeErr := json.Unmarshal(first.ResponseSchema, &schema); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	actions := mustSchemaObject(t, mustSchemaObject(t, schema, "properties"), "recommended_action_ids")
	if actions["maxItems"] != float64(0) {
		t.Fatalf("empty action maxItems = %#v", actions["maxItems"])
	}

	tampered := first
	tampered.ResponseSchema = ProposalJSONSchema()
	if verifyErr := VerifyRequest(tampered); verifyErr == nil || !strings.Contains(verifyErr.Error(), "request-specific") {
		t.Fatalf("VerifyRequest(tampered schema) error = %v", verifyErr)
	}
	tampered = first
	tampered.AllowedCategories = append(tampered.AllowedCategories, "state")
	slices.Sort(tampered.AllowedCategories)
	tampered.ResponseSchema, err = proposalJSONSchemaForRequest(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequest(tampered); err == nil || !strings.Contains(err.Error(), "semantic content") {
		t.Fatalf("VerifyRequest(tampered authority and schema) error = %v", err)
	}
	tampered = first
	tampered.ResponseSchema = ProposalJSONSchema()
	if _, err := SealRequest(tampered); err == nil || !strings.Contains(err.Error(), "must not be supplied") {
		t.Fatalf("SealRequest(prebuilt schema) error = %v", err)
	}
}

func TestSealedRequestSchemaIncludesArtifactAuthority(t *testing.T) {
	t.Parallel()

	base := validRequest(t)
	base.RequestID = ""
	base.ResponseSchema = nil
	base.Projection.Artifacts = []ProjectedArtifact{{
		ID: "artifact:stderr", Role: "target_stderr", Run: 1, Stream: "stderr", Content: "x",
		Encoding: "utf-8-lossy", Digest: "sha256:" + strings.Repeat("c", 64),
		SelectedBytes: 1, ContentBytes: 1, Disclosure: "log_content",
	}}
	base.Manifest.Classes = []string{"log_content", "metadata"}
	base.Manifest.ArtifactIDs = []string{"artifact:stderr"}
	base.Manifest.ArtifactCount = 1
	base.Manifest.ArtifactBytes = 1
	request, err := SealRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(request.ResponseSchema, &schema); err != nil {
		t.Fatal(err)
	}
	hypothesis := mustSchemaObject(t, mustSchemaObject(t, schema, "properties"), "hypotheses", "items", "properties")
	want := []string{"artifact:stderr", "ev:run:1:exit"}
	if got := schemaEnum(t, mustSchemaObject(t, hypothesis, "supporting_evidence", "items")); !slices.Equal(got, want) {
		t.Fatalf("artifact evidence enum = %v, want %v", got, want)
	}
}

func mustSchemaObject(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	current := root
	for _, name := range path {
		next, ok := current[name].(map[string]any)
		if !ok {
			t.Fatalf("schema path %q = %#v", name, current[name])
		}
		current = next
	}

	return current
}

func schemaEnum(t *testing.T, value map[string]any) []string {
	t.Helper()
	encoded, ok := value["enum"].([]any)
	if !ok {
		t.Fatalf("schema enum = %#v", value["enum"])
	}
	result := make([]string, len(encoded))
	for index, item := range encoded {
		current, ok := item.(string)
		if !ok {
			t.Fatalf("schema enum item = %#v", item)
		}
		result[index] = current
	}

	return result
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
		Instructions: RequiredInstructions(), MaximumOutputBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	return request
}
