package diagnosis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"
)

const (
	defaultMaximumReportBytes = 2 * 1024 * 1024
	defaultMaximumReportDepth = 32
	maximumTextBytes          = 16 * 1024
)

// DecodeLimits bounds untrusted report input. Zero values select safe defaults.
type DecodeLimits struct {
	MaxBytes int64
	MaxDepth int
}

// NewConfidence validates a score and supplies its controlled band.
func NewConfidence(score int, basis string) (Confidence, error) {
	if score < 0 || score > 100 || strings.TrimSpace(basis) == "" || len(basis) > maximumTextBytes {
		return Confidence{}, errors.New("construct confidence: invalid score or basis")
	}

	return Confidence{Score: score, Band: confidenceBand(score), Basis: basis}, nil
}

// Seal normalizes, validates, and hashes a diagnosis report.
func Seal(report Report) (Report, error) {
	report.Kind = Kind
	report.SchemaVersion = SchemaVersion
	report.ReportID = ""
	report = normalize(report)
	fingerprint, err := diagnosisFingerprint(report)
	if err != nil {
		return Report{}, err
	}
	report.Fingerprints.Report = fingerprint
	report.ReportID = "sha256:" + strings.Repeat("0", sha256.Size*2)
	if validationErr := validate(report, true); validationErr != nil {
		return Report{}, validationErr
	}
	digest, err := semanticDigest(report)
	if err != nil {
		return Report{}, err
	}
	report.ReportID = digest
	if err := Validate(report); err != nil {
		return Report{}, err
	}

	return report, nil
}

// Validate checks the structural and semantic invariants of a sealed report.
func Validate(report Report) error { return validate(report, false) }

// Verify checks a sealed report and its semantic digest.
func Verify(report Report) error {
	if err := Validate(report); err != nil {
		return err
	}
	want, err := semanticDigest(report)
	if err != nil {
		return err
	}
	if report.ReportID != want {
		return errors.New("verify diagnosis: report ID does not match semantic content")
	}

	return nil
}

// ValidateAgainstEvidence ensures every citation refers to the exact sealed
// core evidence bundle used by the report.
func ValidateAgainstEvidence(report Report, evidence FailureEvidence) error {
	if err := Verify(report); err != nil {
		return err
	}
	if err := VerifyFailureEvidence(evidence); err != nil {
		return fmt.Errorf("validate diagnosis evidence: %w", err)
	}
	if err := validateEvidenceProvenance(report, evidence); err != nil {
		return err
	}
	if err := validateActionSubjects(report.Actions, evidence.Core.Subject); err != nil {
		return err
	}

	return validateEvidenceReferences(report, evidence)
}

func validateEvidenceProvenance(report Report, evidence FailureEvidence) error {
	core := evidence.Core
	if report.CoreEvidenceID != core.EvidenceID || report.AnalysisEvidenceID != evidence.AnalysisEvidenceID ||
		report.Subject.JobID != core.Subject.JobID || report.Subject.JobRevision != core.Subject.JobRevision ||
		report.Subject.Phase != core.Subject.Phase || report.Subject.Outcome != core.Subject.Outcome ||
		!slices.Equal(report.Subject.SelectedRuns, core.Subject.SelectedRuns) {
		return errors.New("validate diagnosis evidence: report subject does not match core evidence")
	}
	if report.Versions.JobmanVersion != core.Source.JobmanVersion ||
		report.Versions.EvidenceSchemaVersion != core.SchemaVersion {
		return errors.New("validate diagnosis evidence: report provenance does not match core evidence")
	}
	if report.Fingerprints.Core != "" && !failureEvidenceHasCoreFingerprint(evidence, report.Fingerprints.Core) {
		return errors.New("validate diagnosis evidence: core fingerprint is unavailable")
	}

	return nil
}

func validateActionSubjects(actions []Action, subject diagnostic.Subject) error {
	for _, action := range actions {
		if action.Execution != ActionExecutionReadOnly {
			continue
		}
		if len(action.Arguments) == 0 || action.Arguments[len(action.Arguments)-1] != subject.JobID {
			return fmt.Errorf("validate diagnosis evidence: action %q targets another job", action.ID)
		}
		if action.Arguments[1] != "logs" {
			continue
		}
		run, err := strconv.ParseUint(strings.TrimPrefix(action.Arguments[2], "--run="), 10, 64)
		if err != nil || run == 0 ||
			(len(subject.SelectedRuns) != 0 && !slices.Contains(subject.SelectedRuns, run)) {
			return fmt.Errorf("validate diagnosis evidence: action %q targets an unavailable run", action.ID)
		}
	}

	return nil
}

//nolint:cyclop,gocognit // This security boundary cross-checks every report reference and disclosed ID against one source.
func validateEvidenceReferences(report Report, evidence FailureEvidence) error {
	core := evidence.Core
	available := make(map[string]string, len(core.Items)+len(core.Artifacts)+len(evidence.Enrichment)+len(evidence.SourceContext))
	for _, item := range core.Items {
		available[item.ID] = item.Code
	}
	for _, artifact := range core.Artifacts {
		available[artifact.ID] = artifact.Role
	}
	enrichment := make(map[string]EnrichmentItem, len(evidence.Enrichment))
	for _, item := range evidence.Enrichment {
		available[item.ID] = item.Code
		enrichment[item.ID] = item
	}
	sourceContext := make(map[string]SourceContext, len(evidence.SourceContext))
	for _, source := range evidence.SourceContext {
		available[source.ID] = source.Role
		sourceContext[source.ID] = source
	}
	cited := make(map[string]struct{}, len(report.Citations))
	for _, citation := range report.Citations {
		code, ok := available[citation.EvidenceID]
		if !ok {
			return fmt.Errorf("validate diagnosis evidence: citation %q is unavailable", citation.EvidenceID)
		}
		if citation.Code != code {
			return fmt.Errorf("validate diagnosis evidence: citation %q has the wrong code", citation.EvidenceID)
		}
		if item, ok := enrichment[citation.EvidenceID]; ok {
			if citation.Kind != "enrichment" || citation.SourceEvidenceID != item.SourceArtifactID ||
				citation.ByteStart != item.ByteStart || citation.ByteEnd != item.ByteEnd {
				return fmt.Errorf("validate diagnosis evidence: enrichment citation %q has invalid provenance", citation.EvidenceID)
			}
		} else if _, ok := sourceContext[citation.EvidenceID]; ok {
			if citation.Kind != "artifact" || citation.SourceEvidenceID != "" ||
				citation.ByteStart != 0 || citation.ByteEnd != 0 {
				return fmt.Errorf("validate diagnosis evidence: source citation %q has invalid provenance", citation.EvidenceID)
			}
		} else if citation.SourceEvidenceID != "" || citation.ByteStart != 0 || citation.ByteEnd != 0 {
			return fmt.Errorf("validate diagnosis evidence: core citation %q has enrichment provenance", citation.EvidenceID)
		}
		cited[citation.EvidenceID] = struct{}{}
	}
	for _, reference := range reportEvidenceReferences(report) {
		if _, ok := available[reference]; !ok {
			return fmt.Errorf("validate diagnosis evidence: reference %q is unavailable", reference)
		}
		if _, ok := cited[reference]; !ok {
			return fmt.Errorf("validate diagnosis evidence: reference %q lacks a citation summary", reference)
		}
	}
	coreItems := make(map[string]struct{}, len(core.Items))
	for _, item := range core.Items {
		coreItems[item.ID] = struct{}{}
	}
	availableArtifacts := make(map[string]struct{}, len(core.Artifacts)+len(evidence.SourceContext))
	for _, artifact := range core.Artifacts {
		availableArtifacts[artifact.ID] = struct{}{}
	}
	for _, source := range evidence.SourceContext {
		availableArtifacts[source.ID] = struct{}{}
	}
	for _, id := range report.Disclosure.ItemIDs {
		if _, ok := coreItems[id]; !ok {
			return fmt.Errorf("validate diagnosis evidence: disclosed item %q is unavailable", id)
		}
	}
	for _, id := range report.Disclosure.ArtifactIDs {
		if _, ok := availableArtifacts[id]; !ok {
			return fmt.Errorf("validate diagnosis evidence: disclosed artifact %q is unavailable", id)
		}
	}
	for _, id := range report.Disclosure.EnrichmentIDs {
		item, ok := enrichment[id]
		if !ok || !slices.Contains(report.Disclosure.ArtifactIDs, item.SourceArtifactID) {
			return fmt.Errorf("validate diagnosis evidence: disclosed enrichment %q is unavailable", id)
		}
	}

	return nil
}

// Encode writes one verified report followed by a newline.
func Encode(destination io.Writer, report Report) error {
	if destination == nil {
		return errors.New("encode diagnosis: destination is nil")
	}
	if err := Verify(report); err != nil {
		return err
	}
	encoder := json.NewEncoder(destination)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode diagnosis: %w", err)
	}

	return nil
}

// Decode bounds, decodes, and verifies one untrusted report.
func Decode(source io.Reader, limits DecodeLimits) (Report, error) {
	if source == nil {
		return Report{}, errors.New("decode diagnosis: source is nil")
	}
	maximumBytes := limits.MaxBytes
	if maximumBytes == 0 {
		maximumBytes = defaultMaximumReportBytes
	}
	maximumDepth := limits.MaxDepth
	if maximumDepth == 0 {
		maximumDepth = defaultMaximumReportDepth
	}
	if maximumBytes < 1 || maximumDepth < 1 {
		return Report{}, errors.New("decode diagnosis: limits must be positive")
	}
	encoded, err := io.ReadAll(io.LimitReader(source, maximumBytes+1))
	if err != nil {
		return Report{}, fmt.Errorf("decode diagnosis: read: %w", err)
	}
	if int64(len(encoded)) > maximumBytes {
		return Report{}, fmt.Errorf("decode diagnosis: input exceeds %d bytes", maximumBytes)
	}
	if err := validateJSONObject(encoded, maximumDepth); err != nil {
		return Report{}, fmt.Errorf("decode diagnosis: %w", err)
	}
	var header struct {
		Kind          string `json:"kind"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.Unmarshal(encoded, &header); err != nil {
		return Report{}, fmt.Errorf("decode diagnosis header: %w", err)
	}
	if header.Kind != Kind || header.SchemaVersion != SchemaVersion {
		return Report{}, fmt.Errorf("decode diagnosis: unsupported kind or schema version %q/%d", header.Kind, header.SchemaVersion)
	}
	var report Report
	if err := json.Unmarshal(encoded, &report); err != nil {
		return Report{}, fmt.Errorf("decode diagnosis: %w", err)
	}
	if err := Verify(report); err != nil {
		return Report{}, err
	}

	return report, nil
}

func validate(report Report, placeholder bool) error {
	if err := validateReportHeader(report, placeholder); err != nil {
		return err
	}
	if err := validateReportContents(report); err != nil {
		return err
	}

	if err := validateDisclosure(report.Disclosure); err != nil {
		return err
	}
	if err := validateGenerators(report.Generators, report.Disclosure); err != nil {
		return err
	}
	generatedProtocol := report.Versions.GenerationRequestSchemaVersion != 0 && validGenerationProtocolVersions(
		report.Versions.GenerationRequestSchemaVersion,
		report.Versions.ProposalSchemaVersion,
	)
	if report.Disclosure.ProviderInvoked != generatedProtocol ||
		(report.Mode == ModeMixed || report.Mode == ModeGenerated) != report.Disclosure.GeneratedContentUsed ||
		report.Mode == ModeDeterministic && report.Disclosure.GeneratedContentUsed {
		return errors.New("validate diagnosis: analysis mode, protocol versions, and disclosure are inconsistent")
	}

	return nil
}

func validateReportHeader(report Report, placeholder bool) error {
	if err := validateReportIdentity(report, placeholder); err != nil {
		return err
	}
	if err := validateVersions(report.Versions); err != nil {
		return err
	}
	if report.Subject.JobID == "" || report.Subject.JobRevision == 0 || report.Subject.Phase == "" ||
		!slices.IsSorted(report.Subject.SelectedRuns) || hasDuplicates(report.Subject.SelectedRuns) {
		return errors.New("validate diagnosis: invalid subject")
	}
	if report.Mode != ModeDeterministic && report.Mode != ModeGenerated && report.Mode != ModeMixed {
		return errors.New("validate diagnosis: invalid analysis mode")
	}
	if !validAnalyzers(report.Analyzers) || !validDigest(report.Fingerprints.Report) ||
		(report.Fingerprints.Core != "" && !validCoreFingerprint(report.Fingerprints.Core)) {
		return errors.New("validate diagnosis: invalid analyzer or fingerprint provenance")
	}

	return nil
}

func validateReportIdentity(report Report, placeholder bool) error {
	if report.Kind != Kind || report.SchemaVersion != SchemaVersion {
		return errors.New("validate diagnosis: unsupported kind or schema version")
	}
	wantPlaceholder := "sha256:" + strings.Repeat("0", sha256.Size*2)
	if placeholder && report.ReportID != wantPlaceholder || !placeholder && !validDigest(report.ReportID) {
		return errors.New("validate diagnosis: invalid report ID")
	}
	if !validDigest(report.CoreEvidenceID) || !validDigest(report.AnalysisEvidenceID) ||
		report.GeneratedAt.IsZero() || report.GeneratedAt.Location() != time.UTC {
		return errors.New("validate diagnosis: invalid evidence ID or generation time")
	}

	return nil
}

func validateVersions(versions Versions) error {
	if versions.CompanionVersion == "" || versions.EngineVersion == "" ||
		versions.JobmanVersion == "" || versions.EvidenceSchemaVersion < 1 ||
		versions.ReportSchemaVersion != SchemaVersion || versions.GenerationRequestSchemaVersion < 0 ||
		versions.GenerationRequestSchemaVersion > 5 || versions.ProposalSchemaVersion < 0 ||
		versions.ProposalSchemaVersion > 2 || !validGenerationProtocolVersions(
		versions.GenerationRequestSchemaVersion,
		versions.ProposalSchemaVersion,
	) {
		return errors.New("validate diagnosis: incomplete versions")
	}

	return nil
}

func validGenerationProtocolVersions(requestVersion, proposalVersion int) bool {
	return requestVersion == 0 && proposalVersion == 0 ||
		requestVersion == 1 && proposalVersion == 1 ||
		requestVersion == 2 && (proposalVersion == 1 || proposalVersion == 2) ||
		(requestVersion == 3 || requestVersion == 4 || requestVersion == 5) && proposalVersion == 2
}

func validateReportContents(report Report) error {
	if len(report.Findings) == 0 || report.PrimaryFindingID == "" {
		return errors.New("validate diagnosis: a primary finding is required")
	}
	if err := validateFindings(report); err != nil {
		return err
	}
	if err := validateActions(report.Actions); err != nil {
		return err
	}
	if err := validateRetry(report.Retry); err != nil {
		return err
	}
	if err := validateCitations(report.Citations); err != nil {
		return err
	}
	if err := validateLimitations(report.MissingEvidence, report.Warnings); err != nil {
		return err
	}

	return nil
}

//nolint:cyclop // Disclosure invariants intentionally cross-check all authority and accounting fields together.
func validateDisclosure(disclosure DisclosureManifest) error {
	if !sortedUnique(disclosure.Classes) || !sortedUnique(disclosure.ItemIDs) ||
		!sortedUnique(disclosure.ArtifactIDs) || !sortedUnique(disclosure.EnrichmentIDs) ||
		disclosure.ItemCount != uint64(len(disclosure.ItemIDs)) ||
		disclosure.ArtifactCount != uint64(len(disclosure.ArtifactIDs)) ||
		disclosure.EnrichmentCount != uint64(len(disclosure.EnrichmentIDs)) {
		return errors.New("validate diagnosis: invalid disclosure manifest")
	}
	if !disclosure.ProviderInvoked {
		if disclosure.GeneratedContentUsed || disclosure.Locality != ProviderNotUsed || disclosure.Profile != "" ||
			disclosure.Provider != "" || disclosure.Model != "" || disclosure.RequestID != "" ||
			len(disclosure.Classes) != 0 || disclosure.ItemCount != 0 || disclosure.ArtifactCount != 0 ||
			disclosure.EnrichmentCount != 0 || disclosure.ArtifactBytes != 0 || disclosure.EnrichmentBytes != 0 ||
			disclosure.RequestBytes != 0 || disclosure.RedactionNoticeCount != 0 {
			return errors.New("validate diagnosis: unused provider has disclosure data")
		}

		return nil
	}
	if disclosure.Locality != ProviderLocal && disclosure.Locality != ProviderRemote ||
		!validCode(disclosure.Profile) || !validCode(disclosure.Provider) || !validText(disclosure.Model) ||
		!validDigest(disclosure.RequestID) || len(disclosure.Classes) == 0 || disclosure.RequestBytes == 0 {
		return errors.New("validate diagnosis: invoked provider has incomplete disclosure data")
	}
	if disclosure.GeneratedContentUsed && disclosure.ItemCount == 0 && disclosure.ArtifactCount == 0 &&
		disclosure.EnrichmentCount == 0 {
		return errors.New("validate diagnosis: generated content used without projected evidence")
	}

	return nil
}

func validateGenerators(values []GeneratorDescriptor, disclosure DisclosureManifest) error {
	if !disclosure.ProviderInvoked {
		if len(values) != 0 {
			return errors.New("validate diagnosis: unused provider has generator provenance")
		}

		return nil
	}
	if len(values) != 1 {
		return errors.New("validate diagnosis: invoked provider requires one generator descriptor")
	}
	value := values[0]
	if value.Provider != disclosure.Provider || value.Model != disclosure.Model || value.Profile != disclosure.Profile ||
		value.Locality != disclosure.Locality {
		return errors.New("validate diagnosis: generator descriptor does not match disclosure")
	}

	return nil
}

//nolint:cyclop // Finding validation jointly enforces identity, evidence sets, and contradiction references.
func validateFindings(report Report) error {
	seen := make(map[string]struct{}, len(report.Findings))
	primaryFound := false
	for _, finding := range report.Findings {
		if !validID(finding.ID) || !validCode(finding.Code) || !validCode(finding.Category) ||
			!validSeverity(finding.Severity) || !validText(finding.Summary) || !validText(finding.Explanation) ||
			!validConfidence(finding.Confidence) || finding.Analyzer == "" ||
			!sortedUnique(finding.SupportingEvidence) || !sortedUnique(finding.ContradictingEvidence) ||
			!sortedUnique(finding.ContradictingFindings) {
			return fmt.Errorf("validate diagnosis: invalid finding %q", finding.ID)
		}
		if _, duplicate := seen[finding.ID]; duplicate {
			return fmt.Errorf("validate diagnosis: duplicate finding %q", finding.ID)
		}
		seen[finding.ID] = struct{}{}
		primaryFound = primaryFound || finding.ID == report.PrimaryFindingID
	}
	if !primaryFound {
		return errors.New("validate diagnosis: primary finding is unavailable")
	}
	for _, finding := range report.Findings {
		for _, reference := range finding.ContradictingFindings {
			if reference == finding.ID {
				return fmt.Errorf("validate diagnosis: finding %q contradicts itself", finding.ID)
			}
			if _, ok := seen[reference]; !ok {
				return fmt.Errorf("validate diagnosis: contradictory finding %q is unavailable", reference)
			}
		}
	}

	return nil
}

func validateActions(actions []Action) error {
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if !validID(action.ID) || !validCode(action.Code) || !validActionKind(action.Kind) ||
			!validText(action.Summary) || !validText(action.Description) || !sortedUnique(action.SupportingEvidence) ||
			action.SafeToAutomate || !validActionExecution(action.Execution, action.Arguments) {
			return fmt.Errorf("validate diagnosis: invalid read-only action %q", action.ID)
		}
		if _, duplicate := seen[action.ID]; duplicate {
			return fmt.Errorf("validate diagnosis: duplicate action %q", action.ID)
		}
		seen[action.ID] = struct{}{}
	}

	return nil
}

//nolint:cyclop // Retry validation intentionally enumerates every controlled verdict, policy, reason, and time invariant.
func validateRetry(retry RetryAdvice) error {
	if retry.Verdict != RetryNow && retry.Verdict != RetryAfterDelay && retry.Verdict != RetryAfterChange &&
		retry.Verdict != RetryNo && retry.Verdict != RetryNotApplicable && retry.Verdict != RetryUnknown {
		return errors.New("validate diagnosis: invalid retry verdict")
	}
	if !validConfidence(retry.Confidence) || !validText(retry.Rationale) || !sortedUnique(retry.SupportingEvidence) {
		return errors.New("validate diagnosis: invalid retry advice")
	}
	if !validExistingPolicy(retry.ExistingPolicy) || !sortedUnique(retry.Reasons) {
		return errors.New("validate diagnosis: invalid existing retry policy")
	}
	for _, reason := range retry.Reasons {
		if !validCode(reason) {
			return errors.New("validate diagnosis: invalid retry reason")
		}
	}
	if retry.EarliestAt != nil && (retry.EarliestAt.IsZero() || retry.EarliestAt.Location() != time.UTC) {
		return errors.New("validate diagnosis: invalid retry time")
	}

	return nil
}

//nolint:cyclop // Citation validation jointly enforces core and exact-range enrichment provenance.
func validateCitations(citations []Citation) error {
	prior := ""
	for _, citation := range citations {
		if !validID(citation.EvidenceID) || citation.EvidenceID <= prior || citation.Code == "" ||
			!validText(citation.Summary) ||
			(citation.Kind != "item" && citation.Kind != "artifact" && citation.Kind != "enrichment") ||
			(citation.Kind == "enrichment" && (citation.SourceEvidenceID == "" || citation.ByteStart >= citation.ByteEnd)) ||
			(citation.Kind != "enrichment" && (citation.SourceEvidenceID != "" || citation.ByteStart != 0 || citation.ByteEnd != 0)) {
			return errors.New("validate diagnosis: citations must be valid, sorted, and unique")
		}
		prior = citation.EvidenceID
	}

	return nil
}

func validateLimitations(missing []MissingEvidence, warnings []Warning) error {
	prior := ""
	for _, value := range missing {
		if !validCode(value.Code) || value.Code <= prior || !validText(value.Description) {
			return errors.New("validate diagnosis: missing evidence must be valid, sorted, and unique")
		}
		prior = value.Code
	}
	prior = ""
	for _, value := range warnings {
		if !validCode(value.Code) || value.Code <= prior || !validText(value.Message) {
			return errors.New("validate diagnosis: warnings must be valid, sorted, and unique")
		}
		prior = value.Code
	}

	return nil
}

//nolint:cyclop,gocognit // Canonical report identity requires all slices and optional values to normalize in one pass.
func normalize(report Report) Report {
	report.GeneratedAt = report.GeneratedAt.UTC().Round(0)
	slices.Sort(report.Subject.SelectedRuns)
	report.Subject.SelectedRuns = slices.Compact(report.Subject.SelectedRuns)
	for index := range report.Findings {
		slices.Sort(report.Findings[index].SupportingEvidence)
		report.Findings[index].SupportingEvidence = slices.Compact(report.Findings[index].SupportingEvidence)
		slices.Sort(report.Findings[index].ContradictingEvidence)
		report.Findings[index].ContradictingEvidence = slices.Compact(report.Findings[index].ContradictingEvidence)
		slices.Sort(report.Findings[index].ContradictingFindings)
		report.Findings[index].ContradictingFindings = slices.Compact(report.Findings[index].ContradictingFindings)
		if report.Findings[index].SupportingEvidence == nil {
			report.Findings[index].SupportingEvidence = []string{}
		}
		if report.Findings[index].ContradictingEvidence == nil {
			report.Findings[index].ContradictingEvidence = []string{}
		}
		if report.Findings[index].ContradictingFindings == nil {
			report.Findings[index].ContradictingFindings = []string{}
		}
	}
	for index := range report.Actions {
		slices.Sort(report.Actions[index].SupportingEvidence)
		report.Actions[index].SupportingEvidence = slices.Compact(report.Actions[index].SupportingEvidence)
		if report.Actions[index].Execution == "" {
			report.Actions[index].Execution = ActionExecutionNone
		}
		if report.Actions[index].Arguments == nil {
			report.Actions[index].Arguments = []string{}
		}
	}
	slices.Sort(report.Retry.SupportingEvidence)
	report.Retry.SupportingEvidence = slices.Compact(report.Retry.SupportingEvidence)
	slices.Sort(report.Retry.Reasons)
	report.Retry.Reasons = slices.Compact(report.Retry.Reasons)
	if report.Retry.Reasons == nil {
		report.Retry.Reasons = []string{}
	}
	if report.Retry.ExistingPolicy == "" {
		report.Retry.ExistingPolicy = PolicyUnknown
	}
	report.Analyzers = normalizeAnalyzers(report.Analyzers, report.Findings)
	if report.Generators == nil {
		report.Generators = []GeneratorDescriptor{}
	}
	slices.SortFunc(report.Citations, func(left, right Citation) int { return strings.Compare(left.EvidenceID, right.EvidenceID) })
	slices.SortFunc(report.MissingEvidence, func(left, right MissingEvidence) int { return strings.Compare(left.Code, right.Code) })
	slices.SortFunc(report.Warnings, func(left, right Warning) int { return strings.Compare(left.Code, right.Code) })
	slices.Sort(report.Disclosure.Classes)
	report.Disclosure.Classes = slices.Compact(report.Disclosure.Classes)
	slices.Sort(report.Disclosure.ItemIDs)
	report.Disclosure.ItemIDs = slices.Compact(report.Disclosure.ItemIDs)
	slices.Sort(report.Disclosure.ArtifactIDs)
	report.Disclosure.ArtifactIDs = slices.Compact(report.Disclosure.ArtifactIDs)
	slices.Sort(report.Disclosure.EnrichmentIDs)
	report.Disclosure.EnrichmentIDs = slices.Compact(report.Disclosure.EnrichmentIDs)
	if report.Findings == nil {
		report.Findings = []Finding{}
	}
	if report.Actions == nil {
		report.Actions = []Action{}
	}
	if report.Citations == nil {
		report.Citations = []Citation{}
	}
	if report.MissingEvidence == nil {
		report.MissingEvidence = []MissingEvidence{}
	}
	if report.Warnings == nil {
		report.Warnings = []Warning{}
	}
	if report.Disclosure.Classes == nil {
		report.Disclosure.Classes = []string{}
	}
	if report.Disclosure.ItemIDs == nil {
		report.Disclosure.ItemIDs = []string{}
	}
	if report.Disclosure.ArtifactIDs == nil {
		report.Disclosure.ArtifactIDs = []string{}
	}
	if report.Disclosure.EnrichmentIDs == nil {
		report.Disclosure.EnrichmentIDs = []string{}
	}
	if report.Retry.EarliestAt != nil {
		value := report.Retry.EarliestAt.UTC().Round(0)
		report.Retry.EarliestAt = &value
	}

	return report
}

func normalizeAnalyzers(configured []AnalyzerDescriptor, findings []Finding) []AnalyzerDescriptor {
	result := slices.Clone(configured)
	for _, finding := range findings {
		name, version := finding.Analyzer, "unknown"
		if separator := strings.LastIndex(finding.Analyzer, "/"); separator > 0 && separator < len(finding.Analyzer)-1 {
			name, version = finding.Analyzer[:separator], finding.Analyzer[separator+1:]
		}
		result = append(result, AnalyzerDescriptor{Name: name, Version: version})
	}
	slices.SortFunc(result, func(left, right AnalyzerDescriptor) int {
		if compared := strings.Compare(left.Name, right.Name); compared != 0 {
			return compared
		}

		return strings.Compare(left.Version, right.Version)
	})
	return slices.CompactFunc(result, func(left, right AnalyzerDescriptor) bool {
		return left == right
	})
}

func semanticDigest(report Report) (string, error) {
	projection := report
	projection.ReportID = ""
	projection.GeneratedAt = time.Time{}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("hash diagnosis: %w", err)
	}
	digest := sha256.Sum256(encoded)

	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func diagnosisFingerprint(report Report) (string, error) {
	primary := Finding{}
	for _, finding := range report.Findings {
		if finding.ID == report.PrimaryFindingID {
			primary = finding
			break
		}
	}
	projection := struct {
		Core      string               `json:"core,omitempty"`
		Code      string               `json:"code"`
		Analyzer  string               `json:"analyzer"`
		Analyzers []AnalyzerDescriptor `json:"analyzers"`
	}{
		Core: report.Fingerprints.Core, Code: primary.Code, Analyzer: primary.Analyzer,
		Analyzers: report.Analyzers,
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("hash diagnosis fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)

	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func reportEvidenceReferences(report Report) []string {
	values := make([]string, 0)
	for _, finding := range report.Findings {
		values = append(values, finding.SupportingEvidence...)
		values = append(values, finding.ContradictingEvidence...)
	}
	for _, action := range report.Actions {
		values = append(values, action.SupportingEvidence...)
	}
	values = append(values, report.Retry.SupportingEvidence...)
	slices.Sort(values)

	return slices.Compact(values)
}

func confidenceBand(score int) string {
	switch {
	case score >= 90:
		return "very_high"
	case score >= 70:
		return "high"
	case score >= 40:
		return "medium"
	default:
		return "low"
	}
}

func validConfidence(value Confidence) bool {
	return value.Score >= 0 && value.Score <= 100 && value.Band == confidenceBand(value.Score) && validText(value.Basis)
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))

	return err == nil
}

func validID(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\r\n\x00")
}

func validCode(value string) bool {
	if value == "" || len(value) > 160 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '_' || character == '.' || character == '-' {
			continue
		}
		return false
	}

	return true
}

func validText(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maximumTextBytes && !strings.ContainsRune(value, '\x00')
}

func validSeverity(value Severity) bool {
	return value == SeverityInfo || value == SeverityWarning || value == SeverityError || value == SeverityCritical
}

func validActionKind(value ActionKind) bool {
	return value == ActionInspect || value == ActionChange || value == ActionWait || value == ActionRetry
}

func validActionExecution(execution ActionExecution, arguments []string) bool {
	switch execution {
	case ActionExecutionNone:
		return len(arguments) == 0
	case ActionExecutionReadOnly:
		return validReadOnlyJobmanArguments(arguments)
	default:
		return false
	}
}

func validReadOnlyJobmanArguments(arguments []string) bool {
	for _, argument := range arguments {
		if !validActionArgument(argument) {
			return false
		}
	}
	if len(arguments) < 4 || arguments[0] != "jobman" {
		return false
	}
	switch arguments[1] {
	case "show":
		return (len(arguments) == 4 && arguments[2] == "job") ||
			(len(arguments) == 5 && arguments[2] == "evidence" && arguments[3] == "--logs=metadata")
	case "logs":
		if len(arguments) != 5 || arguments[3] != "--stream=stderr" {
			return false
		}
		run, err := strconv.ParseUint(strings.TrimPrefix(arguments[2], "--run="), 10, 64)
		return err == nil && run != 0
	default:
		return false
	}
}

func validActionArgument(argument string) bool {
	return argument != "" && len(argument) <= maximumTextBytes && !strings.ContainsAny(argument, "\r\n\x00")
}

func validExistingPolicy(value ExistingPolicy) bool {
	return value == PolicyScheduled || value == PolicyBackoff || value == PolicyWaitingPrerequisite ||
		value == PolicyExhausted || value == PolicyNonretryable || value == PolicyNone || value == PolicyUnknown
}

func validAnalyzers(values []AnalyzerDescriptor) bool {
	if len(values) == 0 {
		return false
	}
	prior := AnalyzerDescriptor{}
	for index, value := range values {
		if !validDescriptor(value) || index != 0 &&
			(value.Name < prior.Name || value.Name == prior.Name && value.Version <= prior.Version) {
			return false
		}
		prior = value
	}

	return true
}

func validCoreFingerprint(value string) bool {
	const prefix = "hmac-sha256-v1:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))

	return err == nil
}

func failureEvidenceHasCoreFingerprint(evidence FailureEvidence, wanted string) bool {
	for _, item := range evidence.Core.Items {
		if item.Code != diagnostic.CodeFailureFingerprint {
			continue
		}
		var fingerprint diagnostic.FailureFingerprint
		if json.Unmarshal(item.Value, &fingerprint) == nil && fingerprint.Validate() == nil &&
			"hmac-sha256-v1:"+fingerprint.Value == wanted {
			return true
		}
	}

	return false
}

func sortedUnique(values []string) bool {
	return slices.IsSorted(values) && !hasDuplicateStrings(values)
}

func hasDuplicateStrings(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return true
		}
	}

	return false
}

func hasDuplicates(values []uint64) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return true
		}
	}

	return false
}

// validateJSONObject rejects duplicate keys, excessive nesting, trailing
// values, and non-object report roots without retaining decoded content.
func validateJSONObject(encoded []byte, maximumDepth int) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := scanJSONValue(decoder, 0, maximumDepth, true); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		return err
	}

	return nil
}

//nolint:cyclop,gocognit // Recursive JSON validation jointly tracks containers, depth, duplicates, and root shape.
func scanJSONValue(decoder *json.Decoder, depth, maximumDepth int, root bool) error {
	if depth > maximumDepth {
		return fmt.Errorf("JSON nesting exceeds %d", maximumDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		if root {
			return errors.New("report root must be an object")
		}
		return nil
	}
	if root && delimiter != '{' {
		return errors.New("report root must be an object")
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return keyErr
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, depth+1, maximumDepth, false); err != nil {
				return err
			}
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder, depth+1, maximumDepth, false); err != nil {
				return err
			}
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}

	return nil
}
