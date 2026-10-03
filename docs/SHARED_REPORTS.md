# Embed deterministic shared-job reports

Use `github.com/ryancswallace/jobman-diagnose/deterministic` to interpret sealed
Jobman evidence in an application. The engine accepts both the existing
schema-1 local evidence and schema-2 Control evidence. It does not discover a
server, authenticate, read files, execute a Jobman CLI, or call a model.

```go
engine, err := deterministic.New(companionVersion, time.Now)
if err != nil {
    return err
}
analysisEvidence, err := deterministic.Prepare(ctx, coreEvidence)
if err != nil {
    return err
}
report, err := engine.Diagnose(ctx, analysisEvidence)
if err != nil {
    return err
}
if err := diagnosis.ValidateAgainstEvidence(report, analysisEvidence); err != nil {
    return err
}
```

`New` returns `diagnosis.Diagnostician`. `Prepare` derives bounded, attributed
structures from already-selected sanitized core artifacts and seals them in
`diagnosis.FailureEvidence`. It does not acquire additional bytes or source
files. Applications that want no enrichment can call
`diagnosis.CoreFailureEvidence` instead.

Store the exact verified core evidence, analysis wrapper and report together.
An enrichment citation uses byte offsets inside its sanitized sealed artifact,
not the original filesystem stream or the current log. Resolve citations from
that exact immutable wrapper after rechecking current source/namespace access.
Never replace old evidence with a fresh capture that happens to have matching
job names or byte offsets. Verify report and wrapper on retrieval.

Reload a stored analysis wrapper with `diagnosis.DecodeFailureEvidence(reader,
diagnosis.DecodeLimits{})`. It rejects duplicate fields, trailing documents,
unknown fields, and invalid wrapper/core seals before returning evidence. Input
is bounded to 4 MiB and depth 32; callers can require tighter bounds. Use
`diagnosis.Decode` for the paired report, then `ValidateAgainstEvidence` to
verify the exact pair. Decoder errors contain no source text or item IDs.

Shared reports use schema 2. Their `shared` field is copied from verified core
evidence and participates in report identity, including deployment and Control
instance, namespace, actual run UUID/number and execution, manifests and
disclosure profile. `ValidateAgainstEvidence` compares it exactly. Schema-1
report decoding and local engine behavior remain supported. For shared reports,
core/artifact capture time, derived enrichment capture time and report generation
time are excluded from semantic IDs. Enrichment `observed_at` retains the
artifact capture time in the stored wrapper; it does not describe when a runtime
event occurred. Actual source observation times in core facts remain part of
semantic identity. Schema-1 local analysis keeps its existing digest behavior.

The shared engine uses actual selected run identity to find exit observations.
It does not parse a run number from an opaque citation ID or fall back to a
different run's exit status. Missing historical observations remain missing.
It distinguishes cancellation outcomes from cancellation intent, lost execution
from proven process termination, scheduler state from Control phase, dependency
decisions from locally inferred readiness, and stale observations from failure.
Exit 137 alone is not an out-of-memory diagnosis. Unknown outcome/state tokens
remain available in evidence and do not crash the analyzer.

Shared recommendations have `execution: none`, empty argument vectors and
`safe_to_automate: false`. They cannot act as unqualified local job commands or
execute changes. Shared retry policy remains unknown without authoritative
policy evidence. Confidence scores express analyzer strength and basis, not a
calibrated probability of successful retry. Findings copied from log patterns
retain their observed/heuristic basis; source text is not executed or treated
as instructions.

The embedding host owns source authentication, current authorization, request
deduplication, cancellation and queue limits. Bound report generation to the
application's deadline and verify the 2 MiB/depth-32 report decoding limit before
publishing. Keep raw sealed objects out of notification payloads and public
artifacts. The standard library entry point performs no provider activation;
opening a stored report never implicitly invokes AI.

Release builds must resolve published, compatible module tags and pass checks
without a local workspace override. The original `testdata/jobman-v1` fixtures
remain the minimum supported local baseline; shared fixture origin and digest
are recorded separately in `testdata/jobman-v2`.
