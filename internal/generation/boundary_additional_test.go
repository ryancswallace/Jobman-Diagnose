package generation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
	"github.com/ryancswallace/jobman-diagnose/provider"
)

type failingDiagnostician struct{ err error }

func (diagnostician failingDiagnostician) Diagnose(
	context.Context,
	diagnosis.FailureEvidence,
) (diagnosis.Report, error) {
	return diagnosis.Report{}, diagnostician.err
}

func TestNewAugmenterValidatesDependenciesAndCapabilities(t *testing.T) {
	t.Parallel()

	profile := testProfile(t, false)
	validGenerator := &fakeGenerator{profile: profile, generate: func(provider.Request) (provider.Response, error) {
		return provider.Response{}, nil
	}}
	for _, test := range []struct {
		name      string
		base      diagnosis.Diagnostician
		generator Generator
		profile   string
	}{
		{name: "nil base", generator: validGenerator, profile: "test"},
		{name: "nil generator", base: deterministic(t), profile: "test"},
		{name: "blank profile", base: deterministic(t), generator: validGenerator, profile: "  "},
	} {
		if _, err := NewAugmenter(test.base, test.generator, test.profile, profile, nil, false, nil); err == nil {
			t.Fatalf("NewAugmenter(%s) error = nil", test.name)
		}
	}
	undersizedProfile := profile
	undersizedProfile.MaximumInputBytes--
	undersized := &fakeGenerator{profile: undersizedProfile, generate: validGenerator.generate}
	if _, err := NewAugmenter(deterministic(t), undersized, "test", profile, nil, false, nil); err == nil {
		t.Fatal("NewAugmenter(undersized generator) error = nil")
	}
}

func TestAugmenterPropagatesBoundaryFailures(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := wrapEvidence(t, core)
	profile := testProfile(t, false)
	generator := &fakeGenerator{profile: profile, generate: func(provider.Request) (provider.Response, error) {
		t.Fatal("generator called after an earlier boundary failure")
		return provider.Response{}, nil
	}}
	augmenter, err := NewAugmenter(deterministic(t), generator, "test", profile, []string{"metadata"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := augmenter.Diagnose(nil, evidence); err == nil { //nolint:staticcheck // Explicit nil-context contract.
		t.Fatal("Diagnose(nil context) error = nil")
	}
	baseErr := errors.New("deterministic diagnosis failed")
	augmenter.base = failingDiagnostician{err: baseErr}
	if _, err := augmenter.Diagnose(t.Context(), evidence); !errors.Is(err, baseErr) {
		t.Fatalf("Diagnose(base failure) error = %v", err)
	}
	augmenter.base = deterministic(t)
	augmenter.approved = []string{"unknown"}
	if _, err := augmenter.Diagnose(t.Context(), evidence); err == nil {
		t.Fatal("Diagnose(unapproved disclosure) error = nil")
	}
}

func TestPrepareRejectsInvalidReportsApprovalsAndBounds(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := wrapEvidence(t, core)
	profile := testProfile(t, false)
	if _, prepareErr := Prepare(evidence, diagnosis.Report{}, "test", profile, []string{"metadata"}); prepareErr == nil {
		t.Fatal("Prepare(invalid report) error = nil")
	}
	report, err := deterministic(t).Diagnose(t.Context(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(evidence, report, "test", profile, []string{"command"}); err == nil {
		t.Fatal("Prepare(without metadata approval) error = nil")
	}
	profile.MaximumInputBytes = 1
	if _, err := Prepare(evidence, report, "test", profile, []string{"metadata"}); err == nil ||
		!strings.Contains(err.Error(), "profile allows 1") {
		t.Fatalf("Prepare(undersized request bound) error = %v", err)
	}
}

func TestGenerationFailureAndReconciliationHelpers(t *testing.T) {
	t.Parallel()

	if got := generatorFailureDetail(context.DeadlineExceeded); !strings.Contains(got, "request_timeout") {
		t.Fatalf("deadline detail = %q", got)
	}
	if got := generatorFailureDetail(context.Canceled); !strings.Contains(got, "request_canceled") {
		t.Fatalf("cancellation detail = %q", got)
	}
	if got := generatorFailureDetail(errors.New("opaque")); !strings.Contains(got, "unspecified") {
		t.Fatalf("generic detail = %q", got)
	}

	actions := []diagnosis.Action{{ID: "first"}, {ID: "second"}, {ID: "third"}}
	if got := reorderActions(actions, nil); len(got) != 3 || got[0].ID != "first" {
		t.Fatalf("original actions = %#v", got)
	}
	got := reorderActions(actions, []string{"third", "first"})
	if got[0].ID != "third" || got[1].ID != "first" || got[2].ID != "second" {
		t.Fatalf("ranked actions = %#v", got)
	}

	finding := diagnosis.Finding{
		Code: "same", Summary: "summary", Explanation: "explanation",
		SupportingEvidence: []string{"ev:2", "ev:1"},
	}
	matching := provider.Hypothesis{
		Code: "generated.same", Summary: "different", Explanation: "different",
		SupportingEvidence: []string{"ev:1", "ev:2"},
	}
	if !duplicatesDeterministicFinding(matching, []diagnosis.Finding{finding}) {
		t.Fatal("equivalent deterministic finding was not detected")
	}
	matching.ContradictsFindings = []string{"finding:001"}
	if duplicatesDeterministicFinding(matching, []diagnosis.Finding{finding}) {
		t.Fatal("contradicting hypothesis was treated as duplicate")
	}
	finding.Analyzer = generatedAnalyzer
	matching.ContradictsFindings = nil
	if duplicatesDeterministicFinding(matching, []diagnosis.Finding{finding}) {
		t.Fatal("generated finding was used for deterministic deduplication")
	}
}

func TestAppendGeneratedCitationsAttributesEveryEvidenceKind(t *testing.T) {
	t.Parallel()

	evidence := diagnosis.FailureEvidence{
		Core: diagnostic.Evidence{
			Items:     []diagnostic.Item{{ID: "item", Code: "item.code"}},
			Artifacts: []diagnostic.Artifact{{ID: "artifact", Role: "stderr"}},
		},
		Enrichment: []diagnosis.EnrichmentItem{{
			ID: "enrichment", Code: "enrichment.code", SourceArtifactID: "artifact", ByteStart: 1, ByteEnd: 2,
		}},
	}
	proposal := provider.Proposal{Hypotheses: []provider.Hypothesis{{
		SupportingEvidence:    []string{"item", "artifact", "enrichment", "unknown"},
		ContradictingEvidence: []string{"item"},
	}}}
	citations := appendGeneratedCitations(
		[]diagnosis.Citation{{EvidenceID: "already"}}, evidence, proposal,
	)
	if len(citations) != 4 {
		t.Fatalf("citations = %#v", citations)
	}
	kinds := map[string]string{}
	for _, citation := range citations {
		kinds[citation.EvidenceID] = citation.Kind
	}
	if kinds["item"] != "item" || kinds["artifact"] != "artifact" || kinds["enrichment"] != "enrichment" {
		t.Fatalf("citation attribution = %#v", kinds)
	}

	prepared := Prepared{Locality: provider.LocalityRemote, Request: provider.Request{Manifest: provider.ProjectionManifest{
		Classes: []string{"metadata"}, ItemIDs: []string{"item"}, ItemCount: 1,
	}}}
	disclosure := prepared.Disclosure(false)
	if disclosure.Locality != diagnosis.ProviderRemote || disclosure.GeneratedContentUsed ||
		disclosure.ItemCount != 1 || disclosure.ItemIDs[0] != "item" {
		t.Fatalf("remote disclosure = %#v", disclosure)
	}
}
