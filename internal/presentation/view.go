package presentation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

type reportView struct {
	report           diagnosis.Report
	evidence         diagnosis.FailureEvidence
	primary          diagnosis.Finding
	citations        map[string]diagnosis.Citation
	items            map[string]diagnostic.Item
	artifacts        map[string]diagnostic.Artifact
	enrichment       map[string]diagnosis.EnrichmentItem
	evidenceAliases  map[string]string
	evidenceOrder    []string
	findingAliases   map[string]string
	jobName          string
	targetCommand    *diagnostic.Command
	workingDirectory string
}

func newReportView(report diagnosis.Report, evidence diagnosis.FailureEvidence) (reportView, error) {
	if err := diagnosis.ValidateAgainstEvidence(report, evidence); err != nil {
		return reportView{}, fmt.Errorf("validate report and evidence: %w", err)
	}
	primary, ok := findPrimary(report)
	if !ok {
		return reportView{}, fmt.Errorf("primary finding %q is unavailable", report.PrimaryFindingID)
	}
	view := reportView{
		report: report, evidence: evidence, primary: primary,
		citations:       make(map[string]diagnosis.Citation, len(report.Citations)),
		items:           make(map[string]diagnostic.Item, len(evidence.Core.Items)),
		artifacts:       make(map[string]diagnostic.Artifact, len(evidence.Core.Artifacts)),
		enrichment:      make(map[string]diagnosis.EnrichmentItem, len(evidence.Enrichment)),
		evidenceAliases: make(map[string]string, len(report.Citations)),
		findingAliases:  make(map[string]string, len(report.Findings)),
	}
	for _, citation := range report.Citations {
		view.citations[citation.EvidenceID] = citation
	}
	for _, item := range evidence.Core.Items {
		view.items[item.ID] = item
	}
	for _, artifact := range evidence.Core.Artifacts {
		view.artifacts[artifact.ID] = artifact
	}
	for _, item := range evidence.Enrichment {
		view.enrichment[item.ID] = item
	}
	view.assignAliases()
	view.collectJobContext()

	return view, nil
}

func findPrimary(report diagnosis.Report) (diagnosis.Finding, bool) {
	for _, finding := range report.Findings {
		if finding.ID == report.PrimaryFindingID {
			return finding, true
		}
	}

	return diagnosis.Finding{}, false
}

func (view *reportView) assignAliases() {
	for index, finding := range view.report.Findings {
		view.findingAliases[finding.ID] = "F" + strconv.Itoa(index+1)
	}
	ordered := make([]string, 0, len(view.report.Citations))
	add := func(values []string) {
		for _, value := range values {
			if _, ok := view.citations[value]; ok && !slices.Contains(ordered, value) {
				ordered = append(ordered, value)
			}
		}
	}
	add(view.primary.SupportingEvidence)
	add(view.primary.ContradictingEvidence)
	add(view.report.Retry.SupportingEvidence)
	for _, finding := range view.report.Findings {
		add(finding.SupportingEvidence)
		add(finding.ContradictingEvidence)
	}
	for _, action := range view.report.Actions {
		add(action.SupportingEvidence)
	}
	for _, citation := range view.report.Citations {
		add([]string{citation.EvidenceID})
	}
	view.evidenceOrder = ordered
	for index, id := range ordered {
		view.evidenceAliases[id] = "E" + strconv.Itoa(index+1)
	}
}

func (view *reportView) collectJobContext() {
	for _, item := range view.evidence.Core.Items {
		switch item.Code {
		case diagnostic.CodeJobName:
			var name string
			if json.Unmarshal(item.Value, &name) == nil {
				view.jobName = name
			}
		case diagnostic.CodeTargetCommand:
			var command diagnostic.Command
			if json.Unmarshal(item.Value, &command) == nil && command.Validate() == nil {
				view.targetCommand = &command
			}
		case diagnostic.CodeTargetWorkingDirectory:
			var workingDirectory string
			if json.Unmarshal(item.Value, &workingDirectory) == nil {
				view.workingDirectory = workingDirectory
			}
		}
	}
}

func (view reportView) evidenceAlias(id string) string {
	if value, ok := view.evidenceAliases[id]; ok {
		return value
	}

	return id
}

func (view reportView) findingAlias(id string) string {
	if value, ok := view.findingAliases[id]; ok {
		return value
	}

	return id
}

func (view reportView) referenceList(ids []string) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, "["+view.evidenceAlias(id)+"]")
	}

	return strings.Join(values, ", ")
}

func (view reportView) findingReferenceList(ids []string) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, "["+view.findingAlias(id)+"]")
	}

	return strings.Join(values, ", ")
}

func (view reportView) evidenceDetail(id string) string {
	citation := view.citations[id]
	if item, ok := view.items[id]; ok {
		return qualityText(item.Quality) + " — " + view.itemDetail(citation, item)
	}
	if item, ok := view.enrichment[id]; ok {
		return qualityText(item.Quality) + " — " + view.enrichmentDetail(item)
	}
	if artifact, ok := view.artifacts[id]; ok {
		return qualityText(artifact.Quality) + " — " + artifactDetail(artifact)
	}

	return strings.TrimSuffix(citation.Summary, ".")
}

//nolint:cyclop // Evidence codes are deliberately rendered through an explicit, safe display allowlist.
func (view reportView) itemDetail(citation diagnosis.Citation, item diagnostic.Item) string {
	subject := evidenceSubject(item.ID)
	switch item.Code {
	case diagnostic.CodeJobName:
		return labeledString("Job name", item.Value)
	case diagnostic.CodeJobPhase:
		return labeledString("Job phase", item.Value)
	case diagnostic.CodeJobOutcome:
		return labeledString("Job outcome", item.Value)
	case diagnostic.CodeRunPhase:
		return labeledString(subject+" phase", item.Value)
	case diagnostic.CodeRunOutcome:
		return labeledString(subject+" outcome", item.Value)
	case diagnostic.CodeRunExitCode:
		return labeledScalar(subject+" exit code", item.Value)
	case diagnostic.CodeRunExitSignal:
		return labeledString(subject+" exit signal", item.Value)
	case diagnostic.CodeRunExitPlatformReason:
		return labeledString(subject+" platform exit reason", item.Value)
	case diagnostic.CodeRunStopReason:
		return labeledString(subject+" stop reason", item.Value)
	case diagnostic.CodeRunTimeoutScope:
		return labeledString(subject+" timeout scope", item.Value)
	case diagnostic.CodeRunDuration:
		return labeledDuration(subject+" duration", item.Value)
	case diagnostic.CodeFailureClass:
		return failureClassDetail(subject, item.Value)
	case diagnostic.CodeResourceObservation:
		var observation diagnostic.ResourceObservation
		if decode(item.Value, &observation) == nil {
			return subject + " resource — " + formatResource(observation)
		}
	case diagnostic.CodeTargetCommand, diagnostic.CodeWaitCommand, diagnostic.CodeNotifierCommand:
		var command diagnostic.Command
		if decode(item.Value, &command) == nil {
			return commandLabel(item.Code) + ": " + formatCommand(command)
		}
	case diagnostic.CodeTargetWorkingDirectory, diagnostic.CodeTargetStdinPath,
		diagnostic.CodeWaitPath, diagnostic.CodeNotifierWorkingDirectory,
		diagnostic.CodeRunResolvedExecutable:
		return labeledString(pathLabel(item.Code, subject), item.Value)
	case diagnostic.CodeTargetEnvironmentNames, diagnostic.CodeWaitEnvironmentNames,
		diagnostic.CodeNotifierEnvironmentNames:
		var names diagnostic.EnvironmentNames
		if decode(item.Value, &names) == nil {
			return environmentDetail(environmentLabel(item.Code), names)
		}
	case diagnostic.CodeSimilarFailure:
		var failure diagnostic.SimilarFailure
		if decode(item.Value, &failure) == nil {
			return similarFailureDetail(failure)
		}
	case diagnostic.CodeLogStdoutBytes:
		return labeledBytes(subject+" stdout log size", item.Value)
	case diagnostic.CodeLogStderrBytes:
		return labeledBytes(subject+" stderr log size", item.Value)
	case diagnostic.CodeRuntimeRunCount:
		return labeledScalar("Recorded run count", item.Value)
	case diagnostic.CodeRuntimeSuccessCount:
		return labeledScalar("Recorded success count", item.Value)
	case diagnostic.CodeRuntimeFailureCount:
		return labeledScalar("Recorded failure count", item.Value)
	case diagnostic.CodeRuntimeNextRunAt:
		return labeledString("Next automatic run", item.Value)
	case diagnostic.CodeRuntimeWaitingReason:
		return labeledEnum("Waiting reason", item.Value)
	case diagnostic.CodeExecutionPolicy:
		return "Job execution and retry policy captured"
	}
	if label, value, ok := safeScalarDetail(citation, item.Value); ok {
		return label + ": " + value
	}

	return strings.TrimSuffix(citation.Summary, ".")
}

func (view reportView) enrichmentDetail(item diagnosis.EnrichmentItem) string {
	format := map[string]string{
		"python_traceback": "Python traceback detected",
		"go_panic":         "Go panic stack detected",
		"jvm_exception":    "JVM exception chain detected",
		"compiler_error":   "Compiler diagnostic detected",
	}[item.Format]
	if format == "" {
		format = titleWords(item.Format) + " detected"
	}
	source := item.SourceArtifactID
	if artifact, ok := view.artifacts[source]; ok {
		source = artifactSource(artifact)
	}

	return format + " in " + source + " (" + formatRange(item.ByteStart, item.ByteEnd) + ")"
}

func artifactDetail(artifact diagnostic.Artifact) string {
	detail := artifactSource(artifact) + " excerpt: " + formatBytes(artifact.ContentBytes)
	if artifact.OriginalBytes != 0 {
		detail += " selected from " + formatBytes(artifact.OriginalBytes)
	}
	detail += " (" + formatRange(artifact.ByteStart, artifact.ByteEnd) + ")"
	if artifact.Truncated {
		detail += "; truncated"
	}

	return detail
}

func artifactSource(artifact diagnostic.Artifact) string {
	stream := artifact.Stream
	if stream == "" {
		stream = artifact.Role
	}
	if artifact.Run != 0 {
		return "run " + strconv.FormatUint(artifact.Run, 10) + " " + stream
	}

	return stream
}

func evidenceSubject(id string) string {
	if run, ok := numericIDSegment(id, "ev:run:"); ok {
		return "Run " + strconv.FormatUint(run, 10)
	}

	return "Job"
}

func numericIDSegment(id, prefix string) (uint64, bool) {
	remainder, ok := strings.CutPrefix(id, prefix)
	if !ok {
		return 0, false
	}
	value, _, ok := strings.Cut(remainder, ":")
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)

	return parsed, err == nil
}

func qualityText(quality diagnostic.Quality) string {
	switch quality {
	case diagnostic.QualityObserved:
		return "Observed fact"
	case diagnostic.QualityConfirmed:
		return "Confirmed fact"
	case diagnostic.QualityDerivedExact:
		return "Exact derivation"
	case diagnostic.QualityPointInTime:
		return "Point-in-time observation"
	case diagnostic.QualityUnknown:
		return "Unknown-quality observation"
	default:
		return titleWords(string(quality))
	}
}

func commandLabel(code string) string {
	switch code {
	case diagnostic.CodeTargetCommand:
		return "Target command"
	case diagnostic.CodeWaitCommand:
		return "Wait command"
	case diagnostic.CodeNotifierCommand:
		return "Notifier command"
	default:
		return "Command"
	}
}

func pathLabel(code, subject string) string {
	switch code {
	case diagnostic.CodeTargetWorkingDirectory:
		return "Target working directory"
	case diagnostic.CodeTargetStdinPath:
		return "Target standard-input path"
	case diagnostic.CodeWaitPath:
		return "Wait path"
	case diagnostic.CodeNotifierWorkingDirectory:
		return "Notifier working directory"
	case diagnostic.CodeRunResolvedExecutable:
		return subject + " resolved executable"
	default:
		return "Path"
	}
}

func environmentLabel(code string) string {
	switch code {
	case diagnostic.CodeTargetEnvironmentNames:
		return "Target environment names"
	case diagnostic.CodeWaitEnvironmentNames:
		return "Wait-command environment names"
	case diagnostic.CodeNotifierEnvironmentNames:
		return "Notifier environment names"
	default:
		return "Environment names"
	}
}

func environmentDetail(label string, names diagnostic.EnvironmentNames) string {
	parts := make([]string, 0, 4)
	if names.Inheritance != "" {
		parts = append(parts, "inheritance: "+names.Inheritance)
	}
	for _, value := range []struct {
		label string
		names []string
	}{{"set", names.Set}, {"unset", names.Unset}, {"secret-backed", names.Secret}} {
		if len(value.names) != 0 {
			parts = append(parts, value.label+": "+strings.Join(value.names, ", "))
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "no explicit names")
	}

	return label + " — " + strings.Join(parts, "; ")
}

func similarFailureDetail(failure diagnostic.SimilarFailure) string {
	detail := "Matching prior failure: job " + failure.JobID + ", run " +
		strconv.FormatUint(failure.RunNumber, 10) + ", " + failure.FailureClass + ", completed " +
		failure.CompletedAt.Format(time.RFC3339)
	if failure.LaterSucceeded {
		detail += "; a later run succeeded"
	}

	return detail
}

func failureClassDetail(subject string, raw json.RawMessage) string {
	var value struct {
		Class string `json:"class"`
		Scope string `json:"scope"`
	}
	if decode(raw, &value) != nil {
		return subject + " failure class"
	}
	if value.Scope == "job" {
		subject = "Job"
	}

	return subject + " failure class: " + strings.ReplaceAll(value.Class, "_", " ")
}

func labeledString(label string, raw json.RawMessage) string {
	var value string
	if decode(raw, &value) != nil {
		return label
	}

	return label + ": " + value
}

func labeledScalar(label string, raw json.RawMessage) string {
	value, ok := scalarText(raw)
	if !ok {
		return label
	}

	return label + ": " + value
}

func labeledDuration(label string, raw json.RawMessage) string {
	var value string
	if decode(raw, &value) != nil {
		return label
	}
	if duration, err := time.ParseDuration(value); err == nil {
		value = friendlyDuration(duration)
	}

	return label + ": " + value
}

func labeledEnum(label string, raw json.RawMessage) string {
	var value string
	if decode(raw, &value) != nil {
		return label
	}

	return label + ": " + strings.ReplaceAll(value, "_", " ")
}

func labeledBytes(label string, raw json.RawMessage) string {
	var value uint64
	if decode(raw, &value) != nil {
		return label
	}

	return label + ": " + formatBytes(value)
}

func safeScalarDetail(citation diagnosis.Citation, raw json.RawMessage) (string, string, bool) {
	value, ok := scalarText(raw)
	if !ok {
		return "", "", false
	}
	label := strings.TrimSuffix(citation.Summary, ".")
	if label == "" {
		label = titleWords(citation.Code)
	}

	return label, value, true
}

func scalarText(raw json.RawMessage) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case json.Number:
		return typed.String(), true
	case nil:
		return "none", true
	default:
		return "", false
	}
}

func decode(raw json.RawMessage, target any) error {
	if len(raw) == 0 || target == nil {
		return errors.New("decode evidence value: missing input")
	}

	return json.Unmarshal(raw, target)
}
