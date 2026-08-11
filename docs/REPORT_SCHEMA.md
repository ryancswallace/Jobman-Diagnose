# Diagnosis report schema 1

The machine document kind is `jobman.diagnosis_report`; `schema_version` is
`1`. `diagnosis.Encode`, `Decode`, `Seal`, `Verify`, and
`ValidateAgainstEvidence` implement the public Go contract.

## Required sections

| Field | Purpose |
| --- | --- |
| `report_id` | SHA-256 identity of normalized semantic report content |
| `generated_at` | UTC presentation timestamp, excluded from semantic identity |
| `core_evidence_id` | Exact sealed source evidence identity |
| `analysis_evidence_id` | Identity of core evidence plus attributed companion enrichment |
| `versions` | Companion, engine, Jobman, evidence, report, generation-request, and proposal versions |
| `analyzers` | Sorted name/version descriptors for rules and contributing collectors |
| `generators` | Selected provider/model/profile/locality when invocation was attempted |
| `subject` | Job ID, revision, selected runs, phase, and outcome copied from evidence |
| `mode` | `deterministic`, `generated`, or `mixed` analysis provenance |
| `primary_finding_id` | ID of the highest-ranked deterministic finding |
| `findings` | Controlled code, category, severity, explanation, confidence, analyzer, and supporting/contradicting references |
| `actions` | Recommendations with `none` or allowlisted `read_only_argv` execution class |
| `retry` | Verdict, current Jobman policy, reasons, confidence, evidence, and optional earliest time |
| `citations` | Core references or enrichment with source artifact and exact byte range |
| `missing_evidence` | Facts that could materially improve confidence |
| `warnings` | Consistency, trust, provider, or security limitations |
| `disclosure` | Exact optional-generator invocation and projection provenance |
| `fingerprints` | Optional core factual and stable companion diagnosis grouping fingerprints |

Every evidence reference must exist in the exact source bundle and have one
citation entry with the matching item code or artifact role. Report validation
also compares the subject, Jobman version, evidence schema, and item/artifact
counts with that source. Generated or human text cannot invent factual
evidence. `contradicting_findings` names existing deterministic finding IDs and
makes a generated alternative's disagreement explicit.

The current engine emits `deterministic` when no generated content is accepted
and `mixed` when validated generated hypotheses are appended. `generated` is a
reserved schema value for a future engine that still preserves deterministic
safety policy; the current implementation never emits it.

## Human presentation

The human renderer validates the report against the same sealed failure
evidence before writing output. It uses report-local `[F1]` and `[E1]` aliases
instead of exposing zero-padded canonical identifiers throughout the terminal.
Aliases are assigned deterministically and are unambiguous within one report;
they are not part of schema 1 and must not be persisted as evidence identity.

The default renderer is an answer-first summary. Validated generated findings
are labeled as AI-assisted, advisory likely causes and appear beside the
deterministic primary finding without replacing its canonical authority.
Each accepted generated finding has an issue-specific headline followed by
explicit `Root cause` and `Failure path` clauses, making the model's actual
diagnosis distinguishable from Jobman's confirmed exit mechanism.
Recommendations and retry advice precede job context and a source-aware set of
up to four evidence highlights. Repeated generated summary/explanation text is
shown once, and the internal fixed generated ranking score is not presented as
model confidence.

Peer conclusions, retry statements, job context, and evidence use bullets;
recommended actions remain numbered; and subordinate facts use a nested dash.
This hierarchy is present in plain output and does not depend on color. For an
interactive terminal, semantic styling distinguishes section headings,
AI-assisted and deterministic labels, advisory text, failed state, and
suggested commands. `--color=auto|always|never` controls styling; automatic
mode honors `NO_COLOR` and `TERM=dumb`, and JSON is always free of terminal
escape sequences.

`--details` adds the complete cited evidence inventory, confidence bases,
action classifications, retry reasons, report identities, component versions,
analyzers, and disclosure accounting. The renderer uses allowlisted typed
evidence for sanitized command arguments, outcomes, exit details, resource
units, policy state, and exact enrichment ranges. Raw artifact bytes are never
rendered. `--json` remains the source for canonical evidence/finding IDs and
all machine provenance.

## Controlled values

Finding severity is `info`, `warning`, `error`, or `critical`. Action kind is
`inspect`, `change`, `wait`, or `retry`. Retry verdict is `now`,
`after_delay`, `after_change`, `no`, `not_applicable`, or `unknown`.
Existing policy is `scheduled`, `backoff`, `waiting_prerequisite`, `exhausted`,
`nonretryable`, `none`, or `unknown`.

Confidence scores are integers from 0 through 100 with derived bands:

| Score | Band |
| ---: | --- |
| 90–100 | `very_high` |
| 70–89 | `high` |
| 40–69 | `medium` |
| 0–39 | `low` |

Every confidence value includes a nonempty basis. Scores express the strength
of the named analyzer and cited facts; schema 1 does not claim probability
calibration. Accepted generated hypotheses use score 40 with an explicit
uncalibrated basis and rank below observed or exact deterministic findings.

Actions are never marked safe to automate. `read_only_argv` contains only a
host-validated direct Jobman inspection vector; it is displayed but never run.
A true `requires_confirmation` indicates that the described change would need
user authority if a future workflow offered it; the report itself cannot
perform it. A generator can only reorder action IDs already present in the
deterministic report. Reconciliation may also map the first supported
generated hypothesis code to fixed, host-authored guidance. That guidance has
no execution vector, is never safe to automate, and cannot contain
model-authored commands or URLs.

## Disclosure manifest

When `provider_invoked` is false, locality is `not_used`, generator identity
fields are absent, request/proposal protocol versions are zero, and all
projection vectors and counts are empty. Deterministic mode therefore provides
a machine-verifiable statement that no generator was called.

When invocation is attempted, the manifest records locality (`local` or
`remote`), selected profile, adapter, model, semantic request digest, projected
classes, exact item, artifact, and enrichment IDs, counts, artifact and
enrichment bytes, encoded request
bytes, and projected redaction-notice count. The generation request protocol
version is 2 and the current proposal protocol version is 2. The decoder
retains schema-1 proposal provenance in historical reports. These values
describe the request that may have reached the provider even if the provider
failed or its proposal was rejected.
`generated_content_used` is true only when validated proposal content entered
the report.

## Decoder security

The default report input ceiling is 2 MiB with nesting depth 32. The decoder
rejects duplicate object keys, duplicate or unsorted controlled identifiers,
trailing JSON values, invalid text, incomplete provenance, mismatched
confidence bands, invalid contradiction references, and report ID mutation.
