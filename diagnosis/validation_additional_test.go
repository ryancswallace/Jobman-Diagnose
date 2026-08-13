package diagnosis

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/internal/testevidence"
)

func TestReportValidationRejectsInvalidVariants(t *testing.T) {
	t.Parallel()

	nonUTC := time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("offset", 3600))
	zero := time.Time{}
	tests := []struct {
		name   string
		mutate func(*Report)
	}{
		{name: "kind", mutate: func(value *Report) { value.Kind = "other" }},
		{name: "schema", mutate: func(value *Report) { value.SchemaVersion++ }},
		{name: "report ID", mutate: func(value *Report) { value.ReportID = "bad" }},
		{name: "core ID", mutate: func(value *Report) { value.CoreEvidenceID = "bad" }},
		{name: "analysis ID", mutate: func(value *Report) { value.AnalysisEvidenceID = "bad" }},
		{name: "generated zero", mutate: func(value *Report) { value.GeneratedAt = zero }},
		{name: "generated zone", mutate: func(value *Report) { value.GeneratedAt = nonUTC }},
		{name: "versions companion", mutate: func(value *Report) { value.Versions.CompanionVersion = "" }},
		{name: "versions evidence", mutate: func(value *Report) { value.Versions.EvidenceSchemaVersion = 0 }},
		{name: "versions report", mutate: func(value *Report) { value.Versions.ReportSchemaVersion++ }},
		{name: "versions protocol pair", mutate: func(value *Report) { value.Versions.GenerationRequestSchemaVersion = 1 }},
		{name: "subject job", mutate: func(value *Report) { value.Subject.JobID = "" }},
		{name: "subject revision", mutate: func(value *Report) { value.Subject.JobRevision = 0 }},
		{name: "subject phase", mutate: func(value *Report) { value.Subject.Phase = "" }},
		{name: "subject runs unsorted", mutate: func(value *Report) { value.Subject.SelectedRuns = []uint64{2, 1} }},
		{name: "subject runs duplicate", mutate: func(value *Report) { value.Subject.SelectedRuns = []uint64{1, 1} }},
		{name: "mode", mutate: func(value *Report) { value.Mode = "other" }},
		{name: "analyzers empty", mutate: func(value *Report) { value.Analyzers = nil }},
		{name: "analyzers duplicate", mutate: func(value *Report) { value.Analyzers = append(value.Analyzers, value.Analyzers[0]) }},
		{name: "report fingerprint", mutate: func(value *Report) { value.Fingerprints.Report = "bad" }},
		{name: "core fingerprint", mutate: func(value *Report) { value.Fingerprints.Core = "bad" }},
		{name: "findings empty", mutate: func(value *Report) { value.Findings = nil }},
		{name: "primary empty", mutate: func(value *Report) { value.PrimaryFindingID = "" }},
		{name: "finding id", mutate: func(value *Report) { value.Findings[0].ID = "" }},
		{name: "finding code", mutate: func(value *Report) { value.Findings[0].Code = "BAD" }},
		{name: "finding severity", mutate: func(value *Report) { value.Findings[0].Severity = "other" }},
		{name: "finding confidence", mutate: func(value *Report) { value.Findings[0].Confidence.Band = "low" }},
		{name: "finding duplicate", mutate: func(value *Report) { value.Findings = append(value.Findings, value.Findings[0]) }},
		{name: "primary unavailable", mutate: func(value *Report) { value.PrimaryFindingID = "finding:missing" }},
		{name: "finding self contradiction", mutate: func(value *Report) { value.Findings[0].ContradictingFindings = []string{value.Findings[0].ID} }},
		{name: "finding unavailable contradiction", mutate: func(value *Report) { value.Findings[0].ContradictingFindings = []string{"finding:missing"} }},
		{name: "action invalid", mutate: func(value *Report) { value.Actions = []Action{{ID: "", Code: "inspect", Kind: ActionInspect}} }},
		{name: "action unsafe", mutate: func(value *Report) {
			value.Actions = []Action{validAction(value)}
			value.Actions[0].SafeToAutomate = true
		}},
		{name: "action duplicate", mutate: func(value *Report) { action := validAction(value); value.Actions = []Action{action, action} }},
		{name: "retry verdict", mutate: func(value *Report) { value.Retry.Verdict = "other" }},
		{name: "retry confidence", mutate: func(value *Report) { value.Retry.Confidence.Score = -1 }},
		{name: "retry policy", mutate: func(value *Report) { value.Retry.ExistingPolicy = "other" }},
		{name: "retry reason", mutate: func(value *Report) { value.Retry.Reasons = []string{"BAD"} }},
		{name: "retry time zero", mutate: func(value *Report) { value.Retry.EarliestAt = &zero }},
		{name: "retry time zone", mutate: func(value *Report) { value.Retry.EarliestAt = &nonUTC }},
		{name: "citation kind", mutate: func(value *Report) { value.Citations[0].Kind = "other" }},
		{name: "citation enrichment range", mutate: func(value *Report) { value.Citations[0].Kind = "enrichment" }},
		{name: "citation item range", mutate: func(value *Report) { value.Citations[0].SourceEvidenceID = "artifact"; value.Citations[0].ByteEnd = 1 }},
		{name: "missing evidence", mutate: func(value *Report) {
			value.MissingEvidence = []MissingEvidence{{Code: "BAD", Description: "description"}}
		}},
		{name: "warning", mutate: func(value *Report) { value.Warnings = []Warning{{Code: "warning", Message: ""}} }},
		{name: "unused disclosure", mutate: func(value *Report) { value.Disclosure.Classes = []string{"metadata"} }},
		{name: "unused generator", mutate: func(value *Report) { value.Generators = []GeneratorDescriptor{{Provider: "test"}} }},
		{name: "mode disclosure mismatch", mutate: func(value *Report) { value.Mode = ModeMixed }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report, _ := validReportAndEvidence(t)
			test.mutate(&report)
			if err := Verify(report); err == nil {
				t.Fatal("Verify() error = nil")
			}
		})
	}
}

func TestDisclosureAndGeneratorValidation(t *testing.T) {
	t.Parallel()

	digest := "sha256:" + strings.Repeat("a", 64)
	valid := DisclosureManifest{
		ProviderInvoked: true, GeneratedContentUsed: true, Locality: ProviderRemote,
		Profile: "profile", Provider: "provider", Model: "model", RequestID: digest,
		Classes: []string{"metadata"}, ItemIDs: []string{"ev:item"}, ItemCount: 1, RequestBytes: 100,
	}
	if err := validateDisclosure(valid); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*DisclosureManifest){
		func(value *DisclosureManifest) { value.Locality = "other" },
		func(value *DisclosureManifest) { value.Profile = "BAD" },
		func(value *DisclosureManifest) { value.RequestID = "bad" },
		func(value *DisclosureManifest) { value.Classes = nil },
		func(value *DisclosureManifest) { value.RequestBytes = 0 },
		func(value *DisclosureManifest) { value.ItemCount = 2 },
		func(value *DisclosureManifest) { value.ItemIDs = nil; value.ItemCount = 0 },
	}
	for index, mutate := range mutations {
		value := valid
		mutate(&value)
		if err := validateDisclosure(value); err == nil {
			t.Fatalf("disclosure variant %d error = nil", index)
		}
	}
	descriptor := GeneratorDescriptor{Provider: "provider", Model: "model", Profile: "profile", Locality: ProviderRemote}
	if err := validateGenerators([]GeneratorDescriptor{descriptor}, valid); err != nil {
		t.Fatal(err)
	}
	if validateGenerators(nil, valid) == nil {
		t.Fatal("validateGenerators(missing) error = nil")
	}
	descriptor.Model = "other"
	if validateGenerators([]GeneratorDescriptor{descriptor}, valid) == nil {
		t.Fatal("validateGenerators(mismatch) error = nil")
	}
}

func TestReportAcceptsSupportedGenerationProtocolVersions(t *testing.T) {
	t.Parallel()

	for _, versions := range []struct{ request, proposal int }{{1, 1}, {2, 1}, {2, 2}, {3, 2}} {
		report, _ := validReportAndEvidence(t)
		report.Versions.GenerationRequestSchemaVersion = versions.request
		report.Versions.ProposalSchemaVersion = versions.proposal
		report.Disclosure = DisclosureManifest{
			ProviderInvoked: true, Locality: ProviderLocal,
			Profile: "profile", Provider: "provider", Model: "model",
			RequestID: "sha256:" + strings.Repeat("a", 64), Classes: []string{"metadata"},
			ItemIDs: []string{"ev:item"}, ItemCount: 1, RequestBytes: 100,
		}
		report.Generators = []GeneratorDescriptor{{
			Provider: "provider", Model: "model", Profile: "profile", Locality: ProviderLocal,
		}}
		if _, err := Seal(report); err != nil {
			t.Fatalf("Seal(generation/proposal schema %d/%d): %v", versions.request, versions.proposal, err)
		}
	}
	report, _ := validReportAndEvidence(t)
	report.Versions.GenerationRequestSchemaVersion = 1
	report.Versions.ProposalSchemaVersion = 2
	if _, err := Seal(report); err == nil {
		t.Fatal("Seal(generation/proposal schema 1/2) error = nil")
	}
}

func TestReportCodecAndJSONBoundaries(t *testing.T) {
	t.Parallel()

	report, _ := validReportAndEvidence(t)
	if _, err := NewConfidence(-1, "basis"); err == nil {
		t.Fatal("NewConfidence(-1) error = nil")
	}
	if _, err := NewConfidence(101, "basis"); err == nil {
		t.Fatal("NewConfidence(101) error = nil")
	}
	if _, err := NewConfidence(50, " "); err == nil {
		t.Fatal("NewConfidence(blank basis) error = nil")
	}
	if err := Encode(nil, report); err == nil {
		t.Fatal("Encode(nil) error = nil")
	}
	if err := Encode(failingWriter{}, report); err == nil {
		t.Fatal("Encode(failing writer) error = nil")
	}
	if _, err := Decode(nil, DecodeLimits{}); err == nil {
		t.Fatal("Decode(nil) error = nil")
	}
	if _, err := Decode(strings.NewReader("{}"), DecodeLimits{MaxBytes: -1}); err == nil {
		t.Fatal("Decode(invalid limit) error = nil")
	}
	for _, encoded := range []string{
		`1`, `[]`, `{} {}`, `{"x":1,"x":2}`, strings.Repeat(`{"x":`, defaultMaximumReportDepth+2) + `0` + strings.Repeat(`}`, defaultMaximumReportDepth+2),
	} {
		if _, err := Decode(strings.NewReader(encoded), DecodeLimits{}); err == nil {
			t.Fatalf("Decode(%q) error = nil", encoded)
		}
	}
}

func TestFailureEvidenceValidationAndEncoding(t *testing.T) {
	t.Parallel()

	core, err := testevidence.Failed("nonzero_exit", []byte("panic: test\n"))
	if err != nil {
		t.Fatal(err)
	}
	artifact := core.Artifacts[0]
	item := EnrichmentItem{
		ID: "analysis:000001", Code: "enrichment.go_panic", Format: "go_panic",
		SourceArtifactID: artifact.ID, ByteStart: 0, ByteEnd: uint64(len(artifact.Data)),
		ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Collector:  AnalyzerDescriptor{Name: "collector", Version: "1"},
		Quality:    diagnostic.QualityDerivedExact, Disclosure: diagnostic.DisclosureLocalOnly,
	}
	sourceData := []byte("package main\n")
	source := SourceContext{
		ID: "context:source:001", Role: "source.context", Path: "/srv/main.go",
		Language: "go", MediaType: "text/x-go", Mode: SourceContextFull,
		AnchorReason: "full_file", StartLine: 1, EndLine: 1, TotalLines: 1,
		ByteEnd: uint64(len(sourceData)), FileBytes: uint64(len(sourceData)),
		ContentBytes: uint64(len(sourceData)), Data: sourceData,
		Digest: contentDigest(sourceData), ContentDigest: contentDigest(sourceData),
		CapturedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Collector:  AnalyzerDescriptor{Name: "source", Version: "1"},
		Quality:    diagnostic.QualityPointInTime, Disclosure: DisclosureSourceContent,
	}
	value, err := SealFailureEvidenceWithContext(core, []EnrichmentItem{item}, []SourceContext{source})
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := EncodeFailureEvidence(&encoded, value); err != nil || encoded.Len() == 0 {
		t.Fatalf("EncodeFailureEvidence() = %d bytes, %v", encoded.Len(), err)
	}
	if EncodeFailureEvidence(nil, value) == nil || EncodeFailureEvidence(failingWriter{}, value) == nil {
		t.Fatal("EncodeFailureEvidence() accepted an invalid destination")
	}
	mutations := []func(*FailureEvidence){
		func(current *FailureEvidence) { current.Kind = "other" },
		func(current *FailureEvidence) { current.AnalysisEvidenceID = "bad" },
		func(current *FailureEvidence) { current.Enrichment = nil },
		func(current *FailureEvidence) { current.Enrichment[0].ID = "bad" },
		func(current *FailureEvidence) { current.Enrichment[0].SourceArtifactID = "missing" },
		func(current *FailureEvidence) { current.Enrichment[0].ByteEnd++ },
		func(current *FailureEvidence) { current.Enrichment[0].ObservedAt = time.Time{} },
		func(current *FailureEvidence) { current.Enrichment[0].Collector.Name = "" },
		func(current *FailureEvidence) { current.Enrichment[0].Quality = diagnostic.QualityObserved },
		func(current *FailureEvidence) { current.SourceContext = nil },
		func(current *FailureEvidence) { current.SourceContext[0].Path = "relative.go" },
		func(current *FailureEvidence) { current.SourceContext[0].Data[0] = 'X' },
		func(current *FailureEvidence) { current.SourceContext[0].AnchorReason = "runtime_log" },
	}
	for index, mutate := range mutations {
		current := value
		current.Enrichment = append([]EnrichmentItem{}, value.Enrichment...)
		current.SourceContext = append([]SourceContext{}, value.SourceContext...)
		current.SourceContext[0].Data = append([]byte{}, value.SourceContext[0].Data...)
		mutate(&current)
		if err := VerifyFailureEvidence(current); err == nil {
			t.Fatalf("failure evidence variant %d error = nil", index)
		}
	}
	brokenCore := core
	brokenCore.EvidenceID = "bad"
	if _, err := CoreFailureEvidence(brokenCore); err == nil {
		t.Fatal("CoreFailureEvidence(invalid) error = nil")
	}
}

func TestReadOnlyActionArgumentAllowlist(t *testing.T) {
	t.Parallel()

	jobID := testevidence.JobID
	tests := []struct {
		arguments []string
		valid     bool
	}{
		{[]string{"jobman", "show", "job", jobID}, true},
		{[]string{"jobman", "show", "evidence", "--logs=metadata", jobID}, true},
		{[]string{"jobman", "logs", "--run=1", "--stream=stderr", jobID}, true},
		{[]string{"jobman", "logs", "--run=0", "--stream=stderr", jobID}, false},
		{[]string{"jobman", "logs", "--run=x", "--stream=stderr", jobID}, false},
		{[]string{"jobman", "mutate", "job", jobID}, false},
		{[]string{"jobman", "show"}, false},
		{[]string{"jobman", "show", "job", "bad\nargument"}, false},
	}
	for _, test := range tests {
		if got := validReadOnlyJobmanArguments(test.arguments); got != test.valid {
			t.Fatalf("validReadOnlyJobmanArguments(%q) = %t", test.arguments, got)
		}
	}
	if !validActionExecution(ActionExecutionNone, nil) || validActionExecution(ActionExecutionNone, []string{"x"}) ||
		validActionExecution("other", nil) {
		t.Fatal("validActionExecution() classification changed")
	}
}

//nolint:cyclop // One assertion verifies every canonical collection initialized by normalization.
func TestNormalizeInitializesCanonicalCollections(t *testing.T) {
	t.Parallel()

	earliest := time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("offset", 3600))
	report := normalize(Report{
		GeneratedAt: earliest,
		Findings:    []Finding{{Analyzer: "collector", SupportingEvidence: nil, ContradictingEvidence: nil, ContradictingFindings: nil}},
		Actions:     []Action{{Execution: "", Arguments: nil}},
		Retry:       RetryAdvice{EarliestAt: &earliest},
	})
	if report.Findings[0].SupportingEvidence == nil || report.Findings[0].ContradictingEvidence == nil ||
		report.Findings[0].ContradictingFindings == nil || report.Actions[0].Execution != ActionExecutionNone ||
		report.Actions[0].Arguments == nil || report.Retry.Reasons == nil ||
		report.Retry.ExistingPolicy != PolicyUnknown || report.Generators == nil || report.Citations == nil ||
		report.MissingEvidence == nil || report.Warnings == nil || report.Disclosure.Classes == nil ||
		report.Disclosure.ItemIDs == nil || report.Disclosure.ArtifactIDs == nil ||
		report.Disclosure.EnrichmentIDs == nil || report.Retry.EarliestAt == nil ||
		report.Retry.EarliestAt.Location() != time.UTC {
		t.Fatalf("normalize() left values outside canonical form: %#v", report)
	}
	if len(report.Analyzers) != 1 || report.Analyzers[0] != (AnalyzerDescriptor{Name: "collector", Version: "unknown"}) {
		t.Fatalf("normalize() analyzers = %#v", report.Analyzers)
	}
}

func TestValidateAgainstEvidenceRejectsCrossBoundaryReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
		edit func(*Report)
	}{
		{name: "unavailable fingerprint", want: "core fingerprint is unavailable", edit: func(value *Report) {
			value.Fingerprints.Core = "hmac-sha256-v1:" + strings.Repeat("a", 64)
		}},
		{name: "action another job", want: "targets another job", edit: func(value *Report) {
			value.Actions = []Action{{
				ID: "action:other", Code: "inspect_job", Kind: ActionInspect, Summary: "Inspect job",
				Description: "Inspect a different job.", Execution: ActionExecutionReadOnly,
				Arguments: []string{"jobman", "show", "job", "00000000-0000-0000-0000-000000000001"},
			}}
		}},
		{name: "action unavailable run", want: "unavailable run", edit: func(value *Report) {
			value.Actions = []Action{{
				ID: "action:run", Code: "inspect_logs", Kind: ActionInspect, Summary: "Inspect logs",
				Description: "Inspect an unavailable run.", Execution: ActionExecutionReadOnly,
				Arguments: []string{"jobman", "logs", "--run=2", "--stream=stderr", value.Subject.JobID},
			}}
		}},
		{name: "unavailable citation", want: "citation \"missing\" is unavailable", edit: func(value *Report) {
			value.Findings[0].SupportingEvidence = []string{"missing"}
			value.Retry.SupportingEvidence = []string{"missing"}
			value.Citations = []Citation{{EvidenceID: "missing", Code: "missing.code", Summary: "Missing evidence.", Kind: "item"}}
		}},
		{name: "wrong citation code", want: "wrong code", edit: func(value *Report) {
			value.Citations[0].Code = "wrong.code"
		}},
		{name: "reference without citation", want: "lacks a citation", edit: func(value *Report) {
			value.Citations = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report, evidence := validReportAndEvidence(t)
			test.edit(&report)
			sealed, err := Seal(report)
			if err != nil {
				t.Fatalf("Seal(test report): %v", err)
			}
			if err := ValidateAgainstEvidence(sealed, evidence); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateAgainstEvidence() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReportCodecAdditionalReadAndIdentityBoundaries(t *testing.T) {
	t.Parallel()

	report, _ := validReportAndEvidence(t)
	report.ReportID = "sha256:" + strings.Repeat("a", 64)
	if err := Verify(report); err == nil || !strings.Contains(err.Error(), "semantic content") {
		t.Fatalf("Verify(mismatched digest) error = %v", err)
	}
	if _, err := Seal(Report{}); err == nil {
		t.Fatal("Seal(empty report) error = nil")
	}
	if _, err := Decode(errorReader{}, DecodeLimits{}); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("Decode(read error) = %v", err)
	}
	if _, err := Decode(strings.NewReader(`{"kind":"wrong","schema_version":1}`), DecodeLimits{}); err == nil ||
		!strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("Decode(unsupported header) = %v", err)
	}
	if _, err := Decode(strings.NewReader(strings.Repeat("x", 20)), DecodeLimits{MaxBytes: 10}); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Decode(oversized) = %v", err)
	}
	if _, err := Decode(strings.NewReader(`{}`), DecodeLimits{MaxDepth: -1}); err == nil {
		t.Fatal("Decode(invalid depth) error = nil")
	}
}

func TestReportValidationHelpers(t *testing.T) {
	t.Parallel()

	if validCoreFingerprint("hmac-sha256-v1:"+strings.Repeat("z", 64)) ||
		!validCoreFingerprint("hmac-sha256-v1:"+strings.Repeat("a", 64)) {
		t.Fatal("validCoreFingerprint() hexadecimal classification changed")
	}
	if sortedUnique([]string{"a", "a"}) || !hasDuplicateStrings([]string{"a", "a"}) ||
		hasDuplicateStrings([]string{"a", "b"}) || !hasDuplicates([]uint64{1, 1}) || hasDuplicates([]uint64{1, 2}) {
		t.Fatal("duplicate classification changed")
	}
	if validActionArgument("") || validActionArgument("bad\nargument") || !validActionArgument("valid") {
		t.Fatal("validActionArgument() classification changed")
	}
	if validAnalyzers(nil) || validAnalyzers([]AnalyzerDescriptor{{Name: "z", Version: "1"}, {Name: "a", Version: "1"}}) ||
		!validAnalyzers([]AnalyzerDescriptor{{Name: "a", Version: "1"}}) {
		t.Fatal("validAnalyzers() classification changed")
	}
}

func validReportAndEvidence(t *testing.T) (Report, FailureEvidence) {
	t.Helper()
	core, err := testevidence.Failed("nonzero_exit", nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := CoreFailureEvidence(core)
	if err != nil {
		t.Fatal(err)
	}
	confidence, err := NewConfidence(80, "The observed exit status supports this conclusion.")
	if err != nil {
		t.Fatal(err)
	}
	reference := "ev:run:00000000000000000001:exit:code"
	report, err := Seal(Report{
		GeneratedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CoreEvidenceID: core.EvidenceID, AnalysisEvidenceID: evidence.AnalysisEvidenceID,
		Versions: Versions{CompanionVersion: "test", EngineVersion: EngineVersion, JobmanVersion: core.Source.JobmanVersion, EvidenceSchemaVersion: 1, ReportSchemaVersion: 1},
		Subject:  Subject{JobID: core.Subject.JobID, JobRevision: core.Subject.JobRevision, SelectedRuns: core.Subject.SelectedRuns, Phase: core.Subject.Phase, Outcome: core.Subject.Outcome},
		Mode:     ModeDeterministic, PrimaryFindingID: "finding:1",
		Findings: []Finding{{
			ID: "finding:1", Code: "core.nonzero_exit", Category: "process", Severity: SeverityError,
			Summary: "The target exited unsuccessfully", Explanation: "The exit status was nonzero.",
			Confidence: confidence, SupportingEvidence: []string{reference}, Analyzer: "test/1",
		}},
		Retry: RetryAdvice{
			Verdict: RetryAfterChange, ExistingPolicy: PolicyUnknown, Confidence: confidence,
			Rationale: "A change is required before retrying.", SupportingEvidence: []string{reference},
		},
		Citations:  []Citation{{EvidenceID: reference, Code: diagnostic.CodeRunExitCode, Summary: "Observed exit code.", Kind: "item"}},
		Disclosure: DisclosureManifest{Locality: ProviderNotUsed},
	})
	if err != nil {
		t.Fatal(err)
	}
	return report, evidence
}

func validAction(report *Report) Action {
	return Action{
		ID: "action:1", Code: "inspect_job", Kind: ActionInspect, Summary: "Inspect the job",
		Description: "Inspect the selected job.", Execution: ActionExecutionReadOnly,
		Arguments: []string{"jobman", "show", "job", report.Subject.JobID},
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }
