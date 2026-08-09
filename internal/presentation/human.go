// Package presentation renders diagnosis reports without raw artifact content.
package presentation

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ryancswallace/jobman-diagnose/diagnosis"
)

// Human writes a concise, safe report for an interactive user.
//
//nolint:cyclop // Human output presents every validated report section in one stable order.
func Human(destination io.Writer, report diagnosis.Report) error {
	if destination == nil {
		return fmt.Errorf("write diagnosis: nil destination")
	}
	primary, ok := primaryFinding(report)
	if !ok {
		return fmt.Errorf("write diagnosis: primary finding %q is unavailable", report.PrimaryFindingID)
	}
	writer := tabwriter.NewWriter(destination, 0, 4, 2, ' ', 0)
	fields := [][2]string{
		{"Diagnosis", primary.Summary},
		{"Confidence", fmt.Sprintf("%d/100 (%s)", primary.Confidence.Score, primary.Confidence.Band)},
		{"Why", primary.Explanation},
		{"Retry", strings.ReplaceAll(string(report.Retry.Verdict), "_", " ")},
		{"Current policy", strings.ReplaceAll(string(report.Retry.ExistingPolicy), "_", " ")},
		{"Retry rationale", report.Retry.Rationale},
		{"Job", report.Subject.JobID},
		{"State", strings.TrimSpace(report.Subject.Phase + " " + report.Subject.Outcome)},
		{"Report ID", report.ReportID},
	}
	if report.Disclosure.ProviderInvoked {
		status := "deterministic fallback"
		if report.Disclosure.GeneratedContentUsed {
			status = "validated generated proposal content included"
		}
		fields = append(fields,
			[2]string{"Model augmentation", status},
			[2]string{"Provider", fmt.Sprintf(
				"%s/%s (%s, profile %s)", report.Disclosure.Provider, report.Disclosure.Model,
				report.Disclosure.Locality, report.Disclosure.Profile,
			)},
		)
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(writer, "%s:\t%s\n", field[0], field[1]); err != nil {
			return fmt.Errorf("write diagnosis summary: %w", err)
		}
	}
	if report.Retry.EarliestAt != nil {
		if _, err := fmt.Fprintf(writer, "Retry eligible at:\t%s\n", report.Retry.EarliestAt.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("write diagnosis retry time: %w", err)
		}
	}
	if len(report.Retry.Reasons) != 0 {
		if _, err := fmt.Fprintf(writer, "Retry reasons:\t%s\n", strings.Join(report.Retry.Reasons, ", ")); err != nil {
			return fmt.Errorf("write diagnosis retry reasons: %w", err)
		}
	}
	if err := writeEvidence(writer, report, primary); err != nil {
		return err
	}
	if err := writeAdditionalFindings(writer, report, primary); err != nil {
		return err
	}
	if err := writeActions(writer, report.Actions); err != nil {
		return err
	}
	if err := writeLimitations(writer, report.MissingEvidence, report.Warnings); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush diagnosis: %w", err)
	}

	return nil
}

func writeAdditionalFindings(writer io.Writer, report diagnosis.Report, primary diagnosis.Finding) error {
	if len(report.Findings) < 2 {
		return nil
	}
	if _, err := fmt.Fprintln(writer, "\nADDITIONAL FINDINGS"); err != nil {
		return fmt.Errorf("write additional findings heading: %w", err)
	}
	citations := citationIndex(report.Citations)
	for _, finding := range report.Findings {
		if finding.ID == primary.ID {
			continue
		}
		if _, err := fmt.Fprintf(
			writer, "-\t%s (%d/100, %s) — %s\n",
			finding.Summary, finding.Confidence.Score, finding.Confidence.Band, finding.Explanation,
		); err != nil {
			return fmt.Errorf("write additional finding: %w", err)
		}
		for _, evidenceID := range finding.SupportingEvidence {
			citation := citations[evidenceID]
			if _, err := fmt.Fprintf(writer, " \tEvidence: %s [%s]\n", citation.Summary, evidenceID); err != nil {
				return fmt.Errorf("write additional finding evidence: %w", err)
			}
		}
		for _, findingID := range finding.ContradictingFindings {
			if _, err := fmt.Fprintf(writer, " \tContradicts deterministic finding: %s\n", findingID); err != nil {
				return fmt.Errorf("write contradictory finding: %w", err)
			}
		}
	}

	return nil
}

func writeEvidence(writer io.Writer, report diagnosis.Report, primary diagnosis.Finding) error {
	if len(primary.SupportingEvidence) > 0 || len(primary.ContradictingEvidence) > 0 {
		if _, err := fmt.Fprintln(writer, "\nEVIDENCE"); err != nil {
			return fmt.Errorf("write evidence heading: %w", err)
		}
		citations := citationIndex(report.Citations)
		for _, evidenceID := range primary.SupportingEvidence {
			citation := citations[evidenceID]
			if _, err := fmt.Fprintf(writer, "-\t%s [%s]\n", citation.Summary, evidenceID); err != nil {
				return fmt.Errorf("write diagnosis evidence: %w", err)
			}
		}
		for _, evidenceID := range primary.ContradictingEvidence {
			citation := citations[evidenceID]
			if _, err := fmt.Fprintf(writer, "-	Contradicting: %s [%s]\n", citation.Summary, evidenceID); err != nil {
				return fmt.Errorf("write contradictory diagnosis evidence: %w", err)
			}
		}
	}

	return nil
}

func writeActions(writer io.Writer, actions []diagnosis.Action) error {
	if len(actions) > 0 {
		if _, err := fmt.Fprintln(writer, "\nNEXT ACTIONS"); err != nil {
			return fmt.Errorf("write action heading: %w", err)
		}
		for index, action := range actions {
			description := action.Description
			if action.RequiresConfirmation {
				description += " (Requires explicit confirmation.)"
			}
			if _, err := fmt.Fprintf(writer, "%s.\t%s — %s\n", strconv.Itoa(index+1), action.Summary, description); err != nil {
				return fmt.Errorf("write diagnosis action: %w", err)
			}
			if action.Execution == diagnosis.ActionExecutionReadOnly {
				if _, err := fmt.Fprintf(writer, " \tDirect arguments: %s\n", quoteArguments(action.Arguments)); err != nil {
					return fmt.Errorf("write diagnosis action arguments: %w", err)
				}
			}
		}
	}

	return nil
}

func quoteArguments(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = strconv.Quote(argument)
	}

	return "[" + strings.Join(quoted, ", ") + "]"
}

func writeLimitations(writer io.Writer, missing []diagnosis.MissingEvidence, warnings []diagnosis.Warning) error {
	if len(missing) > 0 || len(warnings) > 0 {
		if _, err := fmt.Fprintln(writer, "\nLIMITATIONS"); err != nil {
			return fmt.Errorf("write limitations heading: %w", err)
		}
		for _, value := range missing {
			if _, err := fmt.Fprintf(writer, "-\t%s\n", value.Description); err != nil {
				return fmt.Errorf("write missing evidence: %w", err)
			}
		}
		for _, value := range warnings {
			if _, err := fmt.Fprintf(writer, "-\t%s\n", value.Message); err != nil {
				return fmt.Errorf("write diagnosis warning: %w", err)
			}
		}
	}

	return nil
}

func primaryFinding(report diagnosis.Report) (diagnosis.Finding, bool) {
	for _, finding := range report.Findings {
		if finding.ID == report.PrimaryFindingID {
			return finding, true
		}
	}

	return diagnosis.Finding{}, false
}

func citationIndex(values []diagnosis.Citation) map[string]diagnosis.Citation {
	result := make(map[string]diagnosis.Citation, len(values))
	for _, value := range values {
		result[value.EvidenceID] = value
	}

	return result
}
