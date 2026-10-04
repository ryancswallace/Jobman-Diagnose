package diagnosis

import (
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	"github.com/ryancswallace/jobman/diagnostic"
)

func supportedReportSchema(version int) bool {
	return version == SchemaVersion || version == SharedSchemaVersion
}

func validateSharedReport(report Report) error {
	if report.Versions.ReportSchemaVersion != report.SchemaVersion {
		return errors.New("validate diagnosis: report schema provenance mismatch")
	}
	if report.SchemaVersion == SchemaVersion {
		if report.Shared != nil || report.Versions.EvidenceSchemaVersion != diagnostic.SchemaVersion {
			return errors.New("validate diagnosis: local report has incompatible source provenance")
		}
		return nil
	}
	if report.Shared == nil || report.Versions.EvidenceSchemaVersion != diagnostic.SharedSchemaVersion {
		return errors.New("validate diagnosis: shared report lacks shared evidence provenance")
	}
	if err := validateSharedReportIdentity(report.Shared, report.Subject); err != nil {
		return err
	}
	for _, action := range report.Actions {
		if action.Execution != ActionExecutionNone || len(action.Arguments) != 0 || action.SafeToAutomate {
			return errors.New("validate diagnosis: shared report actions must be advice without execution")
		}
	}
	return nil
}

func validateSharedReportIdentity(shared *diagnostic.SharedProvenance, subject Subject) error {
	if err := validateSharedReportSource(shared.Source, shared.Profile, subject.JobID); err != nil {
		return err
	}
	if shared.Runs == nil || shared.Logs == nil || len(shared.Runs) > diagnostic.SharedMaximumRuns ||
		len(shared.Logs) > 2*diagnostic.SharedMaximumRuns {
		return errors.New("validate diagnosis: invalid shared collection bounds")
	}
	if err := validateSharedReportRuns(shared.Runs, subject.SelectedRuns); err != nil {
		return err
	}
	return validateSharedReportLogs(shared)
}

func validateSharedReportSource(source diagnostic.SharedSource, profile, jobID string) error {
	if source.Kind != diagnostic.SharedSourceControl || !validSharedUUID(source.DeploymentID) ||
		!validSharedUUID(source.ControlInstanceID) || !validSharedUUID(source.NamespaceID) ||
		!validSharedUUID(jobID) || !validText(source.ControlVersion) || !validText(source.ContractVersion) ||
		(profile != diagnostic.SharedProfileMetadata && profile != diagnostic.SharedProfileIncludeLogTail) {
		return errors.New("validate diagnosis: invalid shared source identity or profile")
	}
	return nil
}

func validateSharedReportRuns(runs []diagnostic.SharedRun, selected []uint64) error {
	numbers := make([]uint64, len(runs))
	seen := make(map[string]bool, len(runs))
	for index, run := range runs {
		if !validSharedUUID(run.ID) || run.Number == 0 || seen[run.ID] ||
			(run.ExecutionID != "" && !validSharedUUID(run.ExecutionID)) {
			return errors.New("validate diagnosis: invalid shared run identity")
		}
		seen[run.ID] = true
		numbers[index] = run.Number
	}
	if !slices.Equal(numbers, selected) {
		return errors.New("validate diagnosis: shared run selection does not match subject")
	}
	return nil
}

func validateSharedReportLogs(shared *diagnostic.SharedProvenance) error {
	runs := make(map[string]diagnostic.SharedRun, len(shared.Runs))
	for _, run := range shared.Runs {
		runs[run.ID] = run
	}
	prior := ""
	streams := make(map[string]bool, len(shared.Logs))
	for _, ref := range shared.Logs {
		run, exists := runs[ref.RunID]
		key := ref.RunID + ":" + ref.Stream
		if !validID(ref.ID) || ref.ID <= prior || !exists || streams[key] ||
			!validSharedUUID(ref.ExecutionID) || run.ExecutionID != ref.ExecutionID ||
			(ref.Stream != "stdout" && ref.Stream != "stderr") || ref.ManifestRevision == 0 {
			return errors.New("validate diagnosis: invalid sealed manifest identity")
		}
		prior = ref.ID
		streams[key] = true
	}
	return nil
}

func validSharedUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16 && compact != strings.Repeat("0", 32) && value == strings.ToLower(value)
}
