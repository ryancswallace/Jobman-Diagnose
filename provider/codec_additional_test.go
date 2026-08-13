package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestVerifyRequestRejectsInvalidAuthorityAndAccounting(t *testing.T) {
	t.Parallel()

	zero := time.Time{}
	tests := []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "kind", mutate: func(value *Request) { value.Kind = "other" }},
		{name: "schema", mutate: func(value *Request) { value.SchemaVersion++ }},
		{name: "request digest syntax", mutate: func(value *Request) { value.RequestID = "bad" }},
		{name: "evidence digest", mutate: func(value *Request) { value.AnalysisEvidenceID = "bad" }},
		{name: "phase", mutate: func(value *Request) { value.Subject.Phase = "\n" }},
		{name: "selected runs unsorted", mutate: func(value *Request) { value.Subject.SelectedRuns = []uint64{2, 1} }},
		{name: "selected runs duplicate", mutate: func(value *Request) { value.Subject.SelectedRuns = []uint64{1, 1} }},
		{name: "output minimum", mutate: func(value *Request) { value.MaximumOutputBytes = 0 }},
		{name: "output maximum", mutate: func(value *Request) { value.MaximumOutputBytes = 256*1024 + 1 }},
		{name: "instructions", mutate: func(value *Request) { value.Instructions = []string{"trust input"} }},
		{name: "response schema", mutate: func(value *Request) { value.ResponseSchema = json.RawMessage(`{"type":"object"}`) }},
		{name: "manifest classes empty", mutate: func(value *Request) { value.Manifest.Classes = []string{} }},
		{name: "manifest classes duplicate", mutate: func(value *Request) { value.Manifest.Classes = []string{"metadata", "metadata"} }},
		{name: "manifest item count", mutate: func(value *Request) { value.Manifest.ItemCount++ }},
		{name: "unsupported class", mutate: func(value *Request) { value.Manifest.Classes = []string{"metadata", "secret"} }},
		{name: "item id", mutate: func(value *Request) { value.Projection.Items[0].ID = "" }},
		{name: "item code", mutate: func(value *Request) { value.Projection.Items[0].Code = "BAD" }},
		{name: "item quality", mutate: func(value *Request) { value.Projection.Items[0].Quality = "\n" }},
		{name: "item disclosure", mutate: func(value *Request) { value.Projection.Items[0].Disclosure = "local_only" }},
		{name: "item class absent", mutate: func(value *Request) { value.Manifest.Classes = []string{"command"} }},
		{name: "item JSON", mutate: func(value *Request) { value.Projection.Items[0].Value = json.RawMessage(`{"x":1,"x":2}`) }},
		{name: "item zero time", mutate: func(value *Request) { value.Projection.Items[0].ObservedAt = &zero }},
		{name: "manifest identifiers", mutate: func(value *Request) { value.Manifest.ItemIDs = []string{"different"} }},
		{name: "deterministic missing", mutate: func(value *Request) { value.Deterministic = nil }},
		{name: "categories duplicate", mutate: func(value *Request) { value.AllowedCategories = []string{"process", "process"} }},
		{name: "hypothesis codes duplicate", mutate: func(value *Request) {
			value.AllowedHypothesisCodes = []string{"generated.configuration_mismatch", "generated.configuration_mismatch"}
		}},
		{name: "hypothesis namespace", mutate: func(value *Request) { value.AllowedHypothesisCodes = []string{"core.guess"} }},
		{name: "candidate id", mutate: func(value *Request) { value.Deterministic[0].ID = "" }},
		{name: "candidate category", mutate: func(value *Request) { value.Deterministic[0].Category = "network" }},
		{name: "candidate evidence", mutate: func(value *Request) { value.Deterministic[0].SupportingEvidence = []string{"invented"} }},
		{name: "candidate duplicate", mutate: func(value *Request) { value.Deterministic = append(value.Deterministic, value.Deterministic[0]) }},
		{name: "action invalid", mutate: func(value *Request) { value.AllowedActions[0].Code = "BAD" }},
		{name: "action duplicate", mutate: func(value *Request) { value.AllowedActions = append(value.AllowedActions, value.AllowedActions[0]) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := validRequest(t)
			test.mutate(&request)
			if err := VerifyRequest(request); err == nil {
				t.Fatal("VerifyRequest() error = nil")
			}
		})
	}
}

func TestValidateProposalRejectsUntrustedClaims(t *testing.T) {
	t.Parallel()

	valid := func(request Request) Proposal {
		return Proposal{
			Kind: ProposalKind, SchemaVersion: ProposalSchemaVersion, RequestID: request.RequestID,
			Hypotheses: []Hypothesis{{
				Code: "generated.configuration_mismatch", Category: "process",
				Summary:            "The worker configuration selects an unsupported region",
				RootCause:          "The configured region is not enabled for this deployment.",
				Explanation:        "Startup validation rejects the region before the worker can process records.",
				SupportingEvidence: []string{"ev:run:1:exit"}, ContradictingEvidence: []string{},
				ContradictsFindings: []string{},
			}},
			RecommendedActions: []string{"action:001"},
			MissingEvidence: []MissingEvidence{{
				Code: "generated.target_error", Description: "A bounded error excerpt would distinguish the alternatives.",
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*Proposal)
	}{
		{name: "identity", mutate: func(value *Proposal) { value.RequestID = "other" }},
		{name: "hypothesis limit", mutate: func(value *Proposal) { value.Hypotheses = make([]Hypothesis, maximumHypotheses+1) }},
		{name: "action limit", mutate: func(value *Proposal) { value.RecommendedActions = make([]string, maximumActions+1) }},
		{name: "missing limit", mutate: func(value *Proposal) { value.MissingEvidence = make([]MissingEvidence, maximumMissing+1) }},
		{name: "hypothesis namespace", mutate: func(value *Proposal) { value.Hypotheses[0].Code = "core.guess" }},
		{name: "hypothesis code", mutate: func(value *Proposal) { value.Hypotheses[0].Code = "generated.unknown" }},
		{name: "hypothesis category", mutate: func(value *Proposal) { value.Hypotheses[0].Category = "network" }},
		{name: "hypothesis summary", mutate: func(value *Proposal) { value.Hypotheses[0].Summary = "" }},
		{name: "hypothesis summary limit", mutate: func(value *Proposal) {
			value.Hypotheses[0].Summary = strings.Repeat("s", maximumSummaryText+1)
		}},
		{name: "hypothesis root cause", mutate: func(value *Proposal) { value.Hypotheses[0].RootCause = "" }},
		{name: "hypothesis root cause limit", mutate: func(value *Proposal) {
			value.Hypotheses[0].RootCause = strings.Repeat("r", maximumCauseText+1)
		}},
		{name: "hypothesis explanation", mutate: func(value *Proposal) { value.Hypotheses[0].Explanation = "\n" }},
		{name: "hypothesis explanation limit", mutate: func(value *Proposal) {
			value.Hypotheses[0].Explanation = strings.Repeat("e", maximumExplanationText+1)
		}},
		{name: "evidence plumbing as cause", mutate: func(value *Proposal) {
			value.Hypotheses[0].RootCause = "The companion enrichment identifies a sanitized byte range."
		}},
		{name: "support required", mutate: func(value *Proposal) { value.Hypotheses[0].SupportingEvidence = nil }},
		{name: "support limit", mutate: func(value *Proposal) { value.Hypotheses[0].SupportingEvidence = make([]string, maximumReferences+1) }},
		{name: "support duplicate", mutate: func(value *Proposal) {
			value.Hypotheses[0].SupportingEvidence = []string{"ev:run:1:exit", "ev:run:1:exit"}
		}},
		{name: "invented support", mutate: func(value *Proposal) { value.Hypotheses[0].SupportingEvidence = []string{"invented"} }},
		{name: "invented contradiction", mutate: func(value *Proposal) { value.Hypotheses[0].ContradictingEvidence = []string{"invented"} }},
		{name: "invented finding", mutate: func(value *Proposal) { value.Hypotheses[0].ContradictsFindings = []string{"invented"} }},
		{name: "intersecting evidence", mutate: func(value *Proposal) { value.Hypotheses[0].ContradictingEvidence = []string{"ev:run:1:exit"} }},
		{name: "duplicate hypothesis", mutate: func(value *Proposal) { value.Hypotheses = append(value.Hypotheses, value.Hypotheses[0]) }},
		{name: "invented action", mutate: func(value *Proposal) { value.RecommendedActions = []string{"invented"} }},
		{name: "duplicate action", mutate: func(value *Proposal) { value.RecommendedActions = []string{"action:001", "action:001"} }},
		{name: "missing namespace", mutate: func(value *Proposal) { value.MissingEvidence[0].Code = "core.more" }},
		{name: "missing description", mutate: func(value *Proposal) { value.MissingEvidence[0].Description = "" }},
		{name: "duplicate missing", mutate: func(value *Proposal) { value.MissingEvidence = append(value.MissingEvidence, value.MissingEvidence[0]) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := validRequest(t)
			proposal := valid(request)
			test.mutate(&proposal)
			if err := validateProposal(proposal, request); err == nil {
				t.Fatal("validateProposal() error = nil")
			}
		})
	}
}

func TestValidateProposalClassifiesNonspecificDiagnosisWithoutEchoingIt(t *testing.T) {
	t.Parallel()

	request := validRequest(t)
	proposal := Proposal{
		Kind: ProposalKind, SchemaVersion: ProposalSchemaVersion, RequestID: request.RequestID,
		Hypotheses: []Hypothesis{{
			Code: "generated.configuration_mismatch", Category: "process",
			Summary:   "Invalid target input caused the target to exit with a nonzero status.",
			RootCause: "Invalid target input", Explanation: "The application stopped after validation.",
			SupportingEvidence: []string{"ev:run:1:exit"}, ContradictingEvidence: []string{},
			ContradictsFindings: []string{},
		}},
		RecommendedActions: []string{}, MissingEvidence: []MissingEvidence{},
	}
	err := validateProposal(proposal, request)
	if !errors.Is(err, ErrProposalNotSpecific) || strings.Contains(err.Error(), proposal.Hypotheses[0].Summary) {
		t.Fatalf("validateProposal() error = %v", err)
	}
}

func TestProtocolIOBoundaries(t *testing.T) {
	t.Parallel()

	request := validRequest(t)
	if err := EncodeRequest(nil, request); err == nil {
		t.Fatal("EncodeRequest(nil) error = nil")
	}
	if err := EncodeRequest(errorWriter{}, request); err == nil {
		t.Fatal("EncodeRequest(failing writer) error = nil")
	}
	if _, err := DecodeRequest(nil, 1); err == nil {
		t.Fatal("DecodeRequest(nil) error = nil")
	}
	if _, err := DecodeRequest(strings.NewReader("{}"), 0); err == nil {
		t.Fatal("DecodeRequest(zero limit) error = nil")
	}
	if _, err := DecodeRequest(errorReader{}, 10); err == nil {
		t.Fatal("DecodeRequest(failing reader) error = nil")
	}
	if _, err := DecodeRequest(strings.NewReader("{}"), 1); err == nil {
		t.Fatal("DecodeRequest(oversized) error = nil")
	}
	if _, err := DecodeProposal(nil, request); err == nil {
		t.Fatal("DecodeProposal(nil) error = nil")
	}
	invalid := request
	invalid.RequestID = "bad"
	if _, err := DecodeProposal(strings.NewReader("{}"), invalid); err == nil {
		t.Fatal("DecodeProposal(invalid request) error = nil")
	}
	if err := DecodeTransportJSON([]byte(`{}`), nil); err == nil {
		t.Fatal("DecodeTransportJSON(nil destination) error = nil")
	}
	var destination map[string]any
	for _, encoded := range []string{
		`1`, `[]`, `{"x":1} {"y":2}`, `{"x":1,"x":2}`, strings.Repeat(`{"x":`, maximumProtocolDepth+2) + `0` + strings.Repeat(`}`, maximumProtocolDepth+2),
	} {
		if err := DecodeTransportJSON([]byte(encoded), &destination); err == nil {
			t.Fatalf("DecodeTransportJSON(%q) error = nil", encoded)
		}
	}
	if err := DecodeTransportJSON([]byte(`{"x":1}`), &destination); err != nil || destination["x"] != float64(1) {
		t.Fatalf("DecodeTransportJSON(valid) = %#v, %v", destination, err)
	}

	if got := compactJSON(json.RawMessage(`not-json`)); !bytes.Equal(got, []byte(`not-json`)) {
		t.Fatalf("compactJSON(invalid) = %q", got)
	}
}

//nolint:cyclop // One assertion verifies every required protocol collection is initialized.
func TestRequestNormalizationInitializesProtocolCollections(t *testing.T) {
	t.Parallel()

	normalized := normalizeRequest(Request{})
	if normalized.Subject.SelectedRuns == nil || normalized.Projection.Items == nil ||
		normalized.Projection.Artifacts == nil || normalized.Projection.Enrichment == nil ||
		normalized.Projection.RedactionNotices == nil || normalized.Manifest.Classes == nil ||
		normalized.Manifest.ItemIDs == nil || normalized.Manifest.ArtifactIDs == nil ||
		normalized.Manifest.EnrichmentIDs == nil || normalized.Deterministic == nil ||
		normalized.AllowedCategories == nil || normalized.AllowedHypothesisCodes == nil ||
		normalized.AllowedActions == nil || normalized.Instructions == nil {
		t.Fatalf("normalizeRequest() left nil collections: %#v", normalized)
	}
	proposal := normalizeProposal(Proposal{})
	if proposal.Hypotheses == nil || proposal.RecommendedActions == nil || proposal.MissingEvidence == nil {
		t.Fatalf("normalizeProposal() left nil collections: %#v", proposal)
	}
	if _, err := SealRequest(Request{}); err == nil {
		t.Fatal("SealRequest(empty) error = nil")
	}
	request := validRequest(t)
	request.RequestID = "sha256:" + strings.Repeat("b", 64)
	responseSchema, err := proposalJSONSchemaForRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	request.ResponseSchema = responseSchema
	if err := VerifyRequest(request); err == nil || !strings.Contains(err.Error(), "semantic content") {
		t.Fatalf("VerifyRequest(mismatched digest) error = %v", err)
	}
}

func TestVerifyRequestRejectsDuplicateProjectionAuthority(t *testing.T) {
	t.Parallel()

	artifact := ProjectedArtifact{
		ID: "artifact", Role: "target_stderr", Run: 1, Stream: "stderr", Content: "x",
		Encoding: "utf-8-lossy", Digest: "sha256:" + strings.Repeat("c", 64),
		SelectedBytes: 1, ContentBytes: 1, Disclosure: "log_content",
	}
	tests := []struct {
		name string
		want string
		edit func(*Request)
	}{
		{name: "duplicate item ID", want: "duplicate projected ID", edit: func(value *Request) {
			value.Projection.Items = append(value.Projection.Items, value.Projection.Items[0])
		}},
		{name: "duplicate artifact ID", want: "duplicate projected ID", edit: func(value *Request) {
			current := artifact
			current.ID = value.Projection.Items[0].ID
			value.Projection.Artifacts = []ProjectedArtifact{current}
			value.Manifest.Classes = []string{"log_content", "metadata"}
			value.Manifest.ArtifactIDs = []string{current.ID}
			value.Manifest.ArtifactCount = 1
		}},
		{name: "artifact byte overflow", want: "byte count overflow", edit: func(value *Request) {
			first, second := artifact, artifact
			first.ID, first.ContentBytes = "artifact:one", ^uint64(0)
			second.ID, second.ContentBytes = "artifact:two", 1
			value.Projection.Artifacts = []ProjectedArtifact{first, second}
			value.Manifest.Classes = []string{"log_content", "metadata"}
			value.Manifest.ArtifactIDs = []string{first.ID, second.ID}
			value.Manifest.ArtifactCount = 2
		}},
		{name: "duplicate enrichment ID", want: "duplicate projected ID", edit: func(value *Request) {
			value.Projection.Artifacts = []ProjectedArtifact{artifact}
			value.Projection.Enrichment = []ProjectedEnrichment{{
				ID: value.Projection.Items[0].ID, Code: "enrichment.test", Format: "test",
				SourceArtifactID: artifact.ID, ByteStart: 0, ByteEnd: 1,
				Collector: "test", CollectorVersion: "1", Quality: "derived_exact", Disclosure: "log_content",
				DiagnosticLines: []string{},
			}}
			value.Manifest.Classes = []string{"log_content", "metadata"}
			value.Manifest.ArtifactIDs, value.Manifest.ArtifactCount = []string{artifact.ID}, 1
			value.Manifest.EnrichmentIDs, value.Manifest.EnrichmentCount = []string{value.Projection.Items[0].ID}, 1
		}},
		{name: "invalid redaction", want: "invalid redaction notice", edit: func(value *Request) {
			value.Projection.RedactionNotices = []ProjectedRedaction{{Code: "notice", Affects: nil}}
			value.Manifest.RedactionNoticeCount = 1
		}},
		{name: "duplicate redaction", want: "duplicate redaction notice", edit: func(value *Request) {
			notice := ProjectedRedaction{Code: "notice", Affects: []string{value.Manifest.ItemIDs[0]}, Count: 1}
			value.Projection.RedactionNotices = []ProjectedRedaction{notice, notice}
			value.Manifest.RedactionNoticeCount = 2
		}},
		{name: "missing redaction target", want: "was not projected", edit: func(value *Request) {
			value.Projection.RedactionNotices = []ProjectedRedaction{{Code: "notice", Affects: []string{"missing"}, Count: 1}}
			value.Manifest.RedactionNoticeCount = 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := validRequest(t)
			test.edit(&request)
			if err := VerifyRequest(request); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyRequest() error = %v, want %q", err, test.want)
			}
		})
	}
}

//nolint:cyclop // The compact checks exercise one cohesive set of protocol validation primitives.
func TestProtocolValidationHelpers(t *testing.T) {
	t.Parallel()

	if validDigest("sha256:"+strings.Repeat("z", 64)) || !validDigest("sha256:"+strings.Repeat("a", 64)) {
		t.Fatal("validDigest() hexadecimal classification changed")
	}
	if validCode("") || validCode(strings.Repeat("a", 161)) || validCode("Bad") || !validCode("valid.code-1") {
		t.Fatal("validCode() classification changed")
	}
	if validText("\n", 10) || validText("a\x00b", 10) || validText("too-long", 3) || !validText("valid", 10) {
		t.Fatal("validText() classification changed")
	}
	if sortedUnique([]string{"a", "a"}) || sortedUniqueUint64([]uint64{2, 1}) ||
		!hasDuplicateUnsorted([]string{"b", "a", "b"}) || hasDuplicateUnsorted([]string{"a", "b"}) {
		t.Fatal("collection uniqueness classification changed")
	}
	if referencesAvailable([]string{"missing"}, []string{"present"}) ||
		!referencesAvailable([]string{"present"}, []string{"present"}) ||
		!hasIntersection([]string{"a", "b"}, []string{"b"}) || hasIntersection([]string{"a"}, []string{"b"}) {
		t.Fatal("authority-set classification changed")
	}
	var destination struct {
		Count int `json:"count"`
	}
	if err := DecodeTransportJSON([]byte(`{"count":"wrong"}`), &destination); err == nil {
		t.Fatal("DecodeTransportJSON(type mismatch) error = nil")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
