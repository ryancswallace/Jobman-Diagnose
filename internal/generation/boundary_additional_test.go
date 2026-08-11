package generation

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
	"github.com/ryancswallace/jobman-diagnose/internal/config"
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

func TestGeneratedGuidanceCatalogIsHostAuthoredAndNonExecuting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code         string
		wantCode     string
		wantKind     diagnosis.ActionKind
		confirmation bool
	}{
		{"generated.access_denied", "review_target_access", diagnosis.ActionChange, true},
		{"generated.application_configuration", "review_application_configuration", diagnosis.ActionChange, true},
		{"generated.application_defect", "review_application_defect", diagnosis.ActionChange, true},
		{"generated.application_input", "review_application_input", diagnosis.ActionChange, true},
		{"generated.data_validation", "review_invalid_data", diagnosis.ActionChange, true},
		{"generated.dependency_missing", "restore_missing_dependency", diagnosis.ActionChange, true},
		{"generated.dependency_unavailable", "restore_required_dependency", diagnosis.ActionChange, true},
		{"generated.environment_mismatch", "review_target_environment", diagnosis.ActionChange, true},
		{"generated.external_service_failure", "inspect_external_service", diagnosis.ActionInspect, false},
		{"generated.resource_pressure", "inspect_resource_constraints", diagnosis.ActionInspect, false},
		{"generated.transient_infrastructure", "confirm_infrastructure_recovery", diagnosis.ActionWait, false},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			t.Parallel()
			hypothesis := provider.Hypothesis{Code: test.code, SupportingEvidence: []string{"evidence"}}
			action, ok := generatedGuidanceAction(hypothesis)
			if !ok || action.Code != test.wantCode || action.Kind != test.wantKind ||
				action.RequiresConfirmation != test.confirmation || action.Execution != diagnosis.ActionExecutionNone ||
				action.SafeToAutomate || !slices.Equal(action.SupportingEvidence, hypothesis.SupportingEvidence) {
				t.Fatalf("generatedGuidanceAction(%q) = %#v, %t", test.code, action, ok)
			}
		})
	}
	if _, ok := generatedGuidanceAction(provider.Hypothesis{Code: "generated.unknown_target_error"}); ok {
		t.Fatal("unknown target error received specific host guidance")
	}
	existing := []diagnosis.Action{{Code: "review_application_configuration"}}
	if got := prependGeneratedGuidance(existing, []provider.Hypothesis{{
		Code: "generated.application_configuration",
	}}); len(got) != 1 || got[0].Code != existing[0].Code {
		t.Fatalf("duplicate host guidance = %#v", got)
	}
}

func TestGeneratedCauseTaxonomyCoversSpecificFailureClasses(t *testing.T) {
	t.Parallel()

	for _, code := range []string{
		"generated.access_denied",
		"generated.application_configuration",
		"generated.application_defect",
		"generated.application_input",
		"generated.data_validation",
		"generated.dependency_missing",
		"generated.dependency_unavailable",
		"generated.environment_mismatch",
		"generated.external_service_failure",
		"generated.resource_pressure",
		"generated.transient_infrastructure",
		"generated.unknown_target_error",
	} {
		if !slices.Contains(allowedHypothesisCodes, code) {
			t.Fatalf("generated taxonomy omits %q", code)
		}
	}
	if !slices.Contains(allowedCategories, "resource") {
		t.Fatal("generated category taxonomy omits resource")
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

func TestProjectEvidenceEnforcesClassAndItemLimits(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := wrapEvidence(t, core)
	profile := testProfile(t, false)

	withoutMetadata := profile
	withoutMetadata.Disclosure = map[string]config.ClassLimits{}
	if _, _, projectErr := projectEvidence(evidence, withoutMetadata, []string{"metadata"}); projectErr == nil ||
		!strings.Contains(projectErr.Error(), "no metadata evidence") {
		t.Fatalf("projectEvidence(without metadata policy) error = %v", projectErr)
	}

	observed := time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("offset", 3600))
	for index := range evidence.Core.Items {
		if evidence.Core.Items[index].Disclosure == diagnostic.DisclosureMetadata {
			evidence.Core.Items[index].ObservedAt = &observed
			break
		}
	}
	projection, manifest, err := projectEvidence(evidence, profile, []string{"metadata"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ItemCount == 0 || len(projection.Items) == 0 || projection.Items[0].ObservedAt == nil ||
		projection.Items[0].ObservedAt.Location() != time.UTC {
		t.Fatalf("projected metadata = %#v / %#v", projection.Items, manifest)
	}

	limits := profile.Disclosure["metadata"]
	limits.MaximumItems = 0
	profile.Disclosure["metadata"] = limits
	if _, _, err := projectEvidence(evidence, profile, []string{"metadata"}); err == nil ||
		!strings.Contains(err.Error(), "disclosure limit") {
		t.Fatalf("projectEvidence(item limit) error = %v", err)
	}
}

func TestProjectEvidenceFiltersArtifactsEnrichmentAndRedactionNotices(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", []byte("invalid \xff output"))
	if err != nil {
		t.Fatal(err)
	}
	core.Source.Capabilities = append(core.Source.Capabilities, "configured_value_redaction_v1")
	localArtifact := core.Artifacts[0]
	localArtifact.ID = "artifact:local"
	localArtifact.Disclosure = diagnostic.DisclosureLocalOnly
	core.Artifacts = append([]diagnostic.Artifact{localArtifact}, core.Artifacts...)
	projectedItemID := core.Items[0].ID
	core.RedactionNotices = []diagnostic.RedactionNotice{
		{Code: "configured_value", Affects: []string{"not-projected", projectedItemID}},
		{Code: "irrelevant", Affects: []string{"not-projected"}},
	}
	evidence := diagnosis.FailureEvidence{Core: core, Enrichment: []diagnosis.EnrichmentItem{
		{ID: "analysis:skipped", SourceArtifactID: "artifact:local"},
		{
			ID: "analysis:included", Code: "enrichment.test", Format: "test",
			SourceArtifactID: core.Artifacts[1].ID, ByteStart: 0, ByteEnd: 1,
			Collector: diagnosis.AnalyzerDescriptor{Name: "test", Version: "1"},
			Quality:   diagnostic.QualityDerivedExact,
		},
	}}
	profile := testProfile(t, true)
	projection, manifest, err := projectEvidence(evidence, profile, []string{"metadata", "log_content"})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Artifacts) != 1 || !strings.Contains(projection.Artifacts[0].Content, "�") ||
		len(projection.Enrichment) != 1 || projection.Enrichment[0].ID != "analysis:included" ||
		len(projection.RedactionNotices) != 1 ||
		!slices.Equal(projection.RedactionNotices[0].Affects, []string{projectedItemID}) ||
		manifest.RedactionNoticeCount != 1 {
		t.Fatalf("projection = %#v, manifest = %#v", projection, manifest)
	}

	limits := profile.Disclosure["log_content"]
	limits.MaximumArtifacts = 0
	profile.Disclosure["log_content"] = limits
	if _, _, err := projectEvidence(evidence, profile, []string{"metadata", "log_content"}); err == nil ||
		!strings.Contains(err.Error(), "log_content exceeds") {
		t.Fatalf("projectEvidence(artifact limit) error = %v", err)
	}
}

func TestProjectionHelpersRejectUnsupportedValues(t *testing.T) {
	t.Parallel()

	findings := []diagnosis.Finding{
		{ID: "supported", SupportingEvidence: []string{"item"}},
		{ID: "unsupported", SupportingEvidence: []string{"missing"}},
	}
	projected := projectDeterministic(diagnosis.Report{Findings: findings}, provider.ProjectionManifest{ItemIDs: []string{"item"}})
	if len(projected) != 1 || projected[0].ID != "supported" {
		t.Fatalf("projectDeterministic() = %#v", projected)
	}
	if allAvailable([]string{"item", "missing"}, []string{"item"}) {
		t.Fatal("allAvailable() accepted a missing reference")
	}
	if _, err := encodedSize(make(chan int)); err == nil {
		t.Fatal("encodedSize(channel) error = nil")
	}
	if size, err := encodedSize(json.RawMessage(`{"ok":true}`)); err != nil || size == 0 {
		t.Fatalf("encodedSize(valid) = %d, %v", size, err)
	}
}
