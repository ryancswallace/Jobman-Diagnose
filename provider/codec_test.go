package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
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
			ContradictsFindings: []string{},
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
	if RequestSchemaVersion != 4 {
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
	if maximum := mustSchemaObject(t, properties, "hypotheses")["maxItems"]; maximum != float64(maximumHypotheses) {
		t.Fatalf("hypothesis maxItems = %#v, want %d", maximum, maximumHypotheses)
	}
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
	for _, name := range []string{"contradicting_evidence", "contradicts_findings"} {
		if maximum := mustSchemaObject(t, hypothesis, name)["maxItems"]; maximum != float64(maximumContradictions) {
			t.Fatalf("%s maxItems = %#v, want %d", name, maximum, maximumContradictions)
		}
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
		mustSchemaObject(t, hypothesis, "explanation")["maxLength"] != float64(maximumExplanationText) ||
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
		"application_configuration for rejected",
		"unknown_target_error is a last resort",
		"smallest directly relevant evidence set",
		"root_cause names the deepest supported cause",
		"Return at most one hypothesis",
		"complete_at_exit resource observation reports consumption",
		"Never describe a traceback, sanitized byte range",
		"deterministic candidates as confirmed framing",
		"source text alone cannot establish a failure cause",
		"every material exception branch, validation item, cause and effect",
	} {
		if !strings.Contains(instructions, expected) {
			t.Fatalf("request instructions omit %q", expected)
		}
	}
}

func TestSealedRequestAcceptsBoundedPointInTimeSourceContext(t *testing.T) {
	t.Parallel()

	base := validRequest(t)
	base.RequestID = ""
	base.ResponseSchema = nil
	content := "def divide(total, count):\n    return total / count\n"
	capturedAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	artifact := ProjectedArtifact{
		ID: "context:source:001", Role: "source.context", Path: "/srv/app/worker.py",
		Language: "python", MediaType: "text/x-python", Selection: "full",
		AnchorReason: "full_file", StartLine: 1, EndLine: 2, TotalLines: 2,
		ByteEnd: uint64(len(content)), FileBytes: uint64(len(content)), Content: content,
		Encoding: "utf-8", Digest: contentDigest([]byte(content)), ContentDigest: contentDigest([]byte(content)),
		CapturedAt: &capturedAt, Quality: "point_in_time", SelectedBytes: uint64(len(content)),
		ContentBytes: uint64(len(content)), Disclosure: "source_content",
	}
	base.Projection.Artifacts = []ProjectedArtifact{artifact}
	base.Manifest.Classes = []string{"metadata", "source_content"}
	base.Manifest.ArtifactIDs = []string{artifact.ID}
	base.Manifest.ArtifactCount = 1
	base.Manifest.ArtifactBytes = artifact.ContentBytes
	request, err := SealRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if request.Projection.Artifacts[0].Path != artifact.Path ||
		request.Projection.Artifacts[0].Selection != "full" {
		t.Fatalf("source projection = %#v", request.Projection.Artifacts)
	}
	if DirectCauseSignalSupported("generated.application_defect", request.Projection) {
		t.Fatal("source context alone authorized a generated failure cause")
	}

	invalid := request
	invalid.Projection.Artifacts[0].Path = "relative.py"
	if err := VerifyRequest(invalid); err == nil {
		t.Fatal("VerifyRequest(relative source path) error = nil")
	}
}

func TestProposalMayCiteRuntimeLogAndSupplementalSource(t *testing.T) {
	t.Parallel()

	base := validRequest(t)
	base.RequestID = ""
	base.ResponseSchema = nil
	logContent := "ZeroDivisionError: float division by zero"
	logArtifact := ProjectedArtifact{
		ID: "artifact:stderr", Role: "target.log_tail", Run: 1, Stream: "stderr",
		Content: logContent, Encoding: "utf-8-lossy", Digest: "sha256:" + strings.Repeat("d", 64),
		SelectedBytes: uint64(len(logContent)), ContentBytes: uint64(len(logContent)), Disclosure: "log_content",
	}
	sourceArtifact := projectedSourceFixture()
	base.Projection.Artifacts = []ProjectedArtifact{logArtifact, sourceArtifact}
	base.Manifest.Classes = []string{"metadata", "log_content", "source_content"}
	base.Manifest.ArtifactIDs = []string{logArtifact.ID, sourceArtifact.ID}
	base.Manifest.ArtifactCount = 2
	base.Manifest.ArtifactBytes = logArtifact.ContentBytes + sourceArtifact.ContentBytes
	base.AllowedCategories = append(base.AllowedCategories, "application")
	base.AllowedHypothesisCodes = []string{"generated.application_defect"}
	request, err := SealRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	proposal := Proposal{
		Kind: ProposalKind, SchemaVersion: ProposalSchemaVersion, RequestID: request.RequestID,
		Hypotheses: []Hypothesis{{
			Code: "generated.application_defect", Category: "application",
			Summary: "ZeroDivisionError in ratio", RootCause: "ratio divides total by a zero count",
			Explanation:           "The runtime reports float division by zero and the current source maps ratio to total / count.",
			SupportingEvidence:    []string{logArtifact.ID, sourceArtifact.ID},
			ContradictingEvidence: []string{}, ContradictsFindings: []string{},
		}},
		RecommendedActions: []string{}, MissingEvidence: []MissingEvidence{},
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, decodeErr := DecodeProposal(bytes.NewReader(encoded), request); decodeErr != nil {
		t.Fatalf("DecodeProposal(log and source) error = %v", decodeErr)
	}
	proposal.Hypotheses[0].SupportingEvidence = []string{sourceArtifact.ID}
	encoded, err = json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, decodeErr := DecodeProposal(bytes.NewReader(encoded), request); !errors.Is(decodeErr, ErrProposalUnsupported) {
		t.Fatalf("DecodeProposal(source only) error = %v", decodeErr)
	}
}

func projectedSourceFixture() ProjectedArtifact {
	content := "def ratio(total, count):\n    return total / count\n"
	capturedAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	return ProjectedArtifact{
		ID: "context:source:001", Role: "source.context", Path: "/srv/app/worker.py",
		Language: "python", MediaType: "text/x-python", Selection: "full",
		AnchorReason: "full_file", StartLine: 1, EndLine: 2, TotalLines: 2,
		ByteEnd: uint64(len(content)), FileBytes: uint64(len(content)), Content: content,
		Encoding: "utf-8", Digest: contentDigest([]byte(content)), ContentDigest: contentDigest([]byte(content)),
		CapturedAt: &capturedAt, Quality: "point_in_time", SelectedBytes: uint64(len(content)),
		ContentBytes: uint64(len(content)), Disclosure: "source_content",
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

func TestSealedRequestConstrainsHypothesesWithoutDirectArtifactSignal(t *testing.T) {
	t.Parallel()

	base := validRequest(t)
	base.RequestID = ""
	base.ResponseSchema = nil
	base.AllowedHypothesisCodes = []string{"generated.unknown_target_error"}
	request, err := SealRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(request.ResponseSchema, &schema); err != nil {
		t.Fatal(err)
	}
	hypotheses := mustSchemaObject(t, mustSchemaObject(t, schema, "properties"), "hypotheses")
	if hypotheses["maxItems"] != float64(0) {
		t.Fatalf("hypotheses maxItems = %#v, want 0", hypotheses["maxItems"])
	}
}

func TestNormalizeProposalPromotesSpecificCauseOverGenericSummary(t *testing.T) {
	t.Parallel()

	proposal := normalizeProposal(Proposal{Hypotheses: []Hypothesis{{
		Summary:   "The target exited with a nonzero status due to an incompatible type",
		RootCause: "worker.c:42:17: error: incompatible type for argument 1",
	}}})
	if proposal.Hypotheses[0].Summary != proposal.Hypotheses[0].RootCause {
		t.Fatalf("normalized hypothesis = %#v", proposal.Hypotheses[0])
	}
}

func TestNormalizeProposalRemovesNonCausalLifecycleNarration(t *testing.T) {
	t.Parallel()

	for _, explanation := range []string{
		"The process started with this run ID failed and completed with an exit code of 1.",
		"The target's causal path from root cause through the affected target operation or component. For chained exceptions, connect the deepest supported cause.",
	} {
		proposal := normalizeProposal(Proposal{Hypotheses: []Hypothesis{{
			Summary:     "The acme_internal_feature_flags module is missing",
			RootCause:   "ModuleNotFoundError: No module named 'acme_internal_feature_flags'",
			Explanation: explanation,
		}}})
		if proposal.Hypotheses[0].Explanation != proposal.Hypotheses[0].RootCause {
			t.Fatalf("normalized hypothesis = %#v", proposal.Hypotheses[0])
		}
	}
}

func TestNormalizeProposalPromotesCauseOverTracebackFraming(t *testing.T) {
	t.Parallel()

	proposal := normalizeProposal(Proposal{Hypotheses: []Hypothesis{{
		Summary:   "Exception Group Traceback (most recent call last): ValueError: negative settlement",
		RootCause: "ValueError: invoice INV-778 has a negative settlement amount",
	}}})
	if proposal.Hypotheses[0].Summary != proposal.Hypotheses[0].RootCause {
		t.Fatalf("normalized hypothesis = %#v", proposal.Hypotheses[0])
	}
}

func TestNormalizeProposalAgainstRequestRemovesUnsupportedExplanationSignal(t *testing.T) {
	t.Parallel()

	proposal := Proposal{Hypotheses: []Hypothesis{{
		RootCause:             "ValueError: deployment configuration is invalid: database.dsn is required",
		Explanation:           "The deployment configuration is invalid due to a missing environment variable.",
		SupportingEvidence:    []string{"stderr"},
		ContradictingEvidence: []string{},
		ContradictsFindings:   []string{},
	}}}
	request := Request{Projection: Projection{Artifacts: []ProjectedArtifact{{
		ID: "stderr", Content: "ValueError: deployment configuration is invalid: database.dsn is required",
	}}}}
	proposal = normalizeProposalAgainstRequest(proposal, request)
	if proposal.Hypotheses[0].Explanation != proposal.Hypotheses[0].RootCause {
		t.Fatalf("normalized hypothesis = %#v", proposal.Hypotheses[0])
	}
}

func TestNormalizeProposalAgainstRequestRetainsSupportedExplanationSignal(t *testing.T) {
	t.Parallel()

	proposal := Proposal{Hypotheses: []Hypothesis{{
		RootCause:          "ConnectionRefusedError at 127.0.0.1:4319",
		Explanation:        "The inventory connection was refused before synchronization.",
		SupportingEvidence: []string{"stderr"},
	}}}
	request := Request{Projection: Projection{Artifacts: []ProjectedArtifact{{
		ID: "stderr", Content: "ConnectionRefusedError: connection refused at 127.0.0.1:4319",
	}}}}
	proposal = normalizeProposalAgainstRequest(proposal, request)
	if proposal.Hypotheses[0].Explanation == proposal.Hypotheses[0].RootCause {
		t.Fatalf("normalized hypothesis = %#v", proposal.Hypotheses[0])
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
	want := []string{"artifact:stderr"}
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
		AnalysisEvidenceID: "sha256:" + strings.Repeat("a", 64),
		Subject:            Subject{Phase: "completed", Outcome: "failure", SelectedRuns: []uint64{1}},
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
