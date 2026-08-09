// Package testevidence builds deterministic contract fixtures for tests.
package testevidence

import (
	"time"

	"github.com/ryancswallace/jobman/diagnostic"
)

// JobID is the stable synthetic identifier shared by companion tests.
const JobID = "01980f4c-7b2a-7a6f-8c10-0123456789ab"

// Failed returns one sealed failed-run evidence bundle.
func Failed(class string, stderr []byte) (diagnostic.Evidence, error) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	phase, err := diagnostic.JSONValue("completed")
	if err != nil {
		return diagnostic.Evidence{}, err
	}
	outcome, err := diagnostic.JSONValue("failure")
	if err != nil {
		return diagnostic.Evidence{}, err
	}
	failure, err := diagnostic.JSONValue(map[string]string{"class": class, "scope": "run"})
	if err != nil {
		return diagnostic.Evidence{}, err
	}
	exit, err := diagnostic.JSONValue(2)
	if err != nil {
		return diagnostic.Evidence{}, err
	}
	source := diagnostic.ItemSource{Kind: "run_snapshot", EntityID: "01980f4c-7b2a-7a6f-8c10-1123456789ab", Revision: 3}
	evidence := diagnostic.Evidence{
		CapturedAt: now,
		Source: diagnostic.Source{
			JobmanVersion: "1.4.0", CollectorVersion: diagnostic.CollectorVersion,
			StoreSchemaVersion: 7, Platform: "linux",
			Capabilities: []string{"diagnostic_records_v1", "log_tail"},
		},
		Subject: diagnostic.Subject{
			JobID: JobID, JobRevision: 8, SelectedRuns: []uint64{1},
			Phase: "completed", Outcome: "failure",
		},
		Consistency: diagnostic.Consistency{
			Metadata: diagnostic.MetadataTransactionalSnapshot, Artifacts: diagnostic.ArtifactsNotCollected,
		},
		Items: []diagnostic.Item{
			{
				ID: "ev:job:outcome", Code: diagnostic.CodeJobOutcome, Value: outcome,
				Source:  diagnostic.ItemSource{Kind: "job_snapshot", EntityID: JobID, Revision: 8},
				Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureMetadata,
			},
			{
				ID: "ev:job:phase", Code: diagnostic.CodeJobPhase, Value: phase,
				Source:  diagnostic.ItemSource{Kind: "job_snapshot", EntityID: JobID, Revision: 8},
				Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureMetadata,
			},
			{
				ID: "ev:run:00000000000000000001:exit:code", Code: diagnostic.CodeRunExitCode, Value: exit,
				Source: source, Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureMetadata,
			},
			{
				ID: "ev:run:00000000000000000001:failure_class", Code: diagnostic.CodeFailureClass, Value: failure,
				Source: source, Quality: diagnostic.QualityConfirmed, Disclosure: diagnostic.DisclosureMetadata,
			},
			{
				ID: "ev:run:00000000000000000001:outcome", Code: diagnostic.CodeRunOutcome, Value: outcome,
				Source: source, Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureMetadata,
			},
		},
		Artifacts: []diagnostic.Artifact{},
		Omissions: []diagnostic.Omission{{
			Code: diagnostic.OmissionResourceUnsupported, Affects: []string{"resource_observations"},
		}},
		RedactionNotices: []diagnostic.RedactionNotice{},
	}
	if stderr != nil {
		evidence.Consistency.Artifacts = diagnostic.ArtifactsStable
		evidence.Artifacts = []diagnostic.Artifact{{
			ID: "artifact:run:00000000000000000001:stderr", Role: diagnostic.ArtifactRoleLogTail,
			Run: 1, Stream: "stderr", MediaType: "application/octet-stream", Data: append([]byte(nil), stderr...),
			OriginalBytes: uint64(len(stderr)), ByteEnd: uint64(len(stderr)), CapturedAt: now,
			Quality: diagnostic.QualityObserved, Disclosure: diagnostic.DisclosureLogContent,
		}}
	}

	return diagnostic.Seal(evidence)
}
