package diagnosis

import "errors"

// A citation is an ID-only join, so every collection must share one namespace.
func validateEvidenceCitationIDs(evidence FailureEvidence) error {
	seen := make(map[string]bool)
	claim := func(id string) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	for _, item := range evidence.Core.Items {
		if !claim(item.ID) {
			return errors.New("validate failure evidence: ambiguous core citation identity")
		}
	}
	for _, artifact := range evidence.Core.Artifacts {
		if !claim(artifact.ID) {
			return errors.New("validate failure evidence: ambiguous artifact citation identity")
		}
	}
	for _, item := range evidence.Enrichment {
		if !claim(item.ID) {
			return errors.New("validate failure evidence: ambiguous enrichment citation identity")
		}
	}
	for _, source := range evidence.SourceContext {
		if !claim(source.ID) {
			return errors.New("validate failure evidence: ambiguous source citation identity")
		}
	}
	return nil
}
