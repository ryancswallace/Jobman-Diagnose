package presentation

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ryancswallace/jobman/diagnostic"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

func appendWrapped(output *strings.Builder, prefix, continuation, value string, width int) {
	paragraphs := strings.Split(value, "\n")
	for index, paragraph := range paragraphs {
		if index > 0 && strings.TrimSpace(paragraph) == "" {
			output.WriteByte('\n')
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			output.WriteString(prefix)
			output.WriteByte('\n')
			continue
		}
		line := prefix
		for _, word := range words {
			separator := ""
			if line != prefix && line != continuation {
				separator = " "
			}
			if visibleWidth(line)+visibleWidth(separator)+visibleWidth(word) > width && line != prefix {
				output.WriteString(strings.TrimRight(line, " "))
				output.WriteByte('\n')
				line = continuation + word
				continue
			}
			line += separator + word
		}
		output.WriteString(strings.TrimRight(line, " "))
		output.WriteByte('\n')
		prefix = continuation
	}
}

func visibleWidth(value string) int { return utf8.RuneCountInString(value) }

func titleWords(value string) string {
	words := strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(value))
	if len(words) == 0 {
		return "Unknown"
	}
	for index := range words {
		if index == 0 {
			runes := []rune(strings.ToLower(words[index]))
			runes[0] = unicode.ToUpper(runes[0])
			words[index] = string(runes)
		} else {
			words[index] = strings.ToLower(words[index])
		}
	}

	return strings.Join(words, " ")
}

func formatState(phase, outcome string) string {
	state := map[string]string{
		"success": "Succeeded", "failure": "Failed", "timed_out": "Timed out",
		"cancelled": "Cancelled", "start_failed": "Failed to start", "lost": "Lost",
	}[outcome]
	if state == "" {
		state = titleWords(phase)
	}
	phaseText := map[string]string{
		"completed": "job completed", "active": "job active", "queued": "job queued",
		"waiting": "job waiting", "paused": "job paused",
	}[phase]
	if outcome != "" && phaseText != "" {
		return state + " (" + phaseText + ")"
	}

	return state
}

func retryVerdictText(verdict diagnosis.RetryVerdict) string {
	switch verdict {
	case diagnosis.RetryNow:
		return "Retry now"
	case diagnosis.RetryAfterDelay:
		return "Retry after the current delay"
	case diagnosis.RetryAfterChange:
		return "Retry after changing the command, environment, resources, or policy"
	case diagnosis.RetryNo:
		return "Do not retry"
	case diagnosis.RetryNotApplicable:
		return "Not applicable"
	case diagnosis.RetryUnknown:
		return "Unable to determine whether a retry makes sense"
	default:
		return titleWords(string(verdict))
	}
}

func existingPolicyText(policy diagnosis.ExistingPolicy) string {
	switch policy {
	case diagnosis.PolicyScheduled:
		return "Jobman has another run scheduled"
	case diagnosis.PolicyBackoff:
		return "An automatic retry is waiting for its backoff delay"
	case diagnosis.PolicyWaitingPrerequisite:
		return "The job is waiting for a prerequisite"
	case diagnosis.PolicyExhausted:
		return "Automatic retries are exhausted"
	case diagnosis.PolicyNonretryable:
		return "The current policy will not retry this failure"
	case diagnosis.PolicyNone:
		return "No automatic retry is scheduled"
	case diagnosis.PolicyUnknown:
		return "The automatic retry policy could not be determined"
	default:
		return titleWords(string(policy))
	}
}

func retryReasonText(reason string) string {
	values := map[string]string{
		"primary_diagnosis_policy":    "The primary diagnosis determines this recommendation",
		"existing_backoff":            "An existing retry is already in backoff",
		"waiting_prerequisite":        "The job is waiting for a prerequisite",
		"retry_budget_exhausted":      "The configured retry budget is exhausted",
		"outcome_nonretryable":        "The current policy classifies the outcome as nonretryable",
		"no_existing_retry":           "No automatic retry is scheduled",
		"change_required":             "A change is required before retrying",
		"delay_recommended":           "A delay is recommended before retrying",
		"immediate_retry_reasonable":  "An immediate retry may reasonably produce a different result",
		"retry_not_useful":            "Another target run would not address this condition",
		"retry_not_applicable":        "Retry depends on renewed user intent",
		"insufficient_retry_evidence": "There is not enough evidence to recommend a retry",
	}
	if value, ok := values[reason]; ok {
		return value
	}

	return titleWords(reason)
}

func analysisModeText(mode diagnosis.AnalysisMode) string {
	switch mode {
	case diagnosis.ModeDeterministic:
		return "Deterministic rules"
	case diagnosis.ModeGenerated:
		return "Validated AI hypotheses"
	case diagnosis.ModeMixed:
		return "Deterministic rules plus validated AI hypotheses"
	default:
		return titleWords(string(mode))
	}
}

func findingSource(analyzer string) string {
	if strings.HasPrefix(analyzer, "generator.") {
		return "AI hypothesis"
	}

	return "Deterministic finding"
}

func actionKindText(kind diagnosis.ActionKind) string {
	switch kind {
	case diagnosis.ActionInspect:
		return "Inspection"
	case diagnosis.ActionChange:
		return "Configuration or environment change"
	case diagnosis.ActionWait:
		return "Wait"
	case diagnosis.ActionRetry:
		return "Retry"
	default:
		return titleWords(string(kind))
	}
}

func formatProvider(report diagnosis.Report) string {
	value := report.Disclosure.Provider + "/" + report.Disclosure.Model
	details := make([]string, 0, 2)
	if report.Disclosure.Locality != "" {
		details = append(details, string(report.Disclosure.Locality))
	}
	if report.Disclosure.Profile != "" {
		details = append(details, "profile "+report.Disclosure.Profile)
	}
	if len(details) != 0 {
		value += " (" + strings.Join(details, "; ") + ")"
	}

	return value
}

func formatVersions(versions diagnosis.Versions) string {
	values := []string{
		"Jobman " + versions.JobmanVersion,
		"companion " + versions.CompanionVersion,
		"engine " + versions.EngineVersion,
		"evidence schema " + strconv.Itoa(versions.EvidenceSchemaVersion),
		"report schema " + strconv.Itoa(versions.ReportSchemaVersion),
	}
	if versions.GenerationRequestSchemaVersion != 0 {
		values = append(values, "generation request schema "+strconv.Itoa(versions.GenerationRequestSchemaVersion))
	}
	if versions.ProposalSchemaVersion != 0 {
		values = append(values, "proposal schema "+strconv.Itoa(versions.ProposalSchemaVersion))
	}

	return strings.Join(values, "; ")
}

func formatCommand(command diagnostic.Command) string {
	arguments := make([]string, 0, len(command.Arguments)+1)
	arguments = append(arguments, command.Executable)
	arguments = append(arguments, command.Arguments...)

	return formatArguments(arguments)
}

func formatArguments(arguments []string) string {
	formatted := make([]string, len(arguments))
	for index, argument := range arguments {
		formatted[index] = formatArgument(argument)
	}

	return strings.Join(formatted, " ")
}

func formatArgument(value string) string {
	if value != "" && strings.IndexFunc(value, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			!strings.ContainsRune("_./:=@+,%~-", character)
	}) == -1 {
		return value
	}

	return strconv.Quote(value)
}

func formatBytes(value uint64) string {
	const unit = uint64(1024)
	if value < unit {
		return strconv.FormatUint(value, 10) + " B"
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	scaled := float64(value)
	selected := "B"
	for _, candidate := range units {
		scaled /= float64(unit)
		selected = candidate
		if scaled < float64(unit) {
			break
		}
	}
	formatted := strconv.FormatFloat(scaled, 'f', 1, 64)
	formatted = strings.TrimSuffix(formatted, ".0")

	return formatted + " " + selected
}

func friendlyDuration(value time.Duration) string {
	if value < 0 {
		value = -value
	}
	switch {
	case value < time.Millisecond:
		return value.String()
	case value < time.Second:
		return value.Round(time.Millisecond).String()
	case value < time.Minute:
		return value.Round(100 * time.Millisecond).String()
	case value < time.Hour:
		return value.Round(time.Second).String()
	default:
		return value.Round(time.Minute).String()
	}
}

func formatResource(observation diagnostic.ResourceObservation) string {
	const maximumDurationNanoseconds = ^uint64(0) >> 1

	metric := map[string]string{
		diagnostic.ResourceCPUUserTime:   "CPU user time",
		diagnostic.ResourceCPUSystemTime: "CPU system time",
		diagnostic.ResourcePeakRSS:       "Peak resident memory",
	}[observation.Metric]
	if metric == "" {
		metric = titleWords(observation.Metric)
	}
	value := strconv.FormatUint(observation.Value, 10) + " " + observation.Unit
	switch observation.Unit {
	case diagnostic.ResourceUnitBytes:
		value = formatBytes(observation.Value)
	case diagnostic.ResourceUnitNanoseconds:
		if observation.Value <= maximumDurationNanoseconds {
			// #nosec G115 -- the explicit bound above guarantees an int64 duration.
			value = friendlyDuration(time.Duration(observation.Value))
		}
	}
	details := titleWords(observation.Scope) + ", " + titleWords(observation.Completeness)

	return metric + ": " + value + " (" + strings.ToLower(details) + ")"
}

func formatRange(start, end uint64) string {
	return fmt.Sprintf("bytes %d–%d", start, end)
}
