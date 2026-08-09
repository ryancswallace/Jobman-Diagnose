# Architecture

`jobman-diagnose` is an independently released consumer of Jobman's factual
evidence boundary. It does not read Jobman's SQLite database or log directory
directly and does not import any Jobman `internal` package.

```text
jobman store and log files
        |
        | transactional metadata + bounded sanitized artifacts
        v
jobman show evidence JOB --json
        |
        | jobman.diagnostic_evidence schema 1
        v
core client -> verification -> exact-range enrichment -> deterministic engine
                                                        -> sealed local report
                                                   |
                                  explicit --ai/--profile activation
                                                   |
                                                   v
                           disclosure projection -> sealed request schema 1
                                                   |
                                                   v
                             one StructuredGenerator call (optional)
                                                   |
                                                   v
                        strict proposal schema 1 -> deterministic reconciliation
                                                   |
                                                   v
                                      diagnosis report schema 1
```

## Ownership

Jobman owns execution truth, exact low-level classification, evidence
selection, value-aware redaction, artifact bounds, store-local fingerprints,
and the evidence schema. The companion owns deterministic interpretation,
confidence, citations, actions, retry advice, report schema, disclosure policy,
provider configuration, and generated hypotheses. Core never reads a diagnosis
report, and a report never changes lifecycle state.

Live acquisition invokes a validated absolute Jobman executable directly,
without a shell. When launched as `jobman diagnose`, extension protocol 1
provides that executable, the canonical state directory, and any explicitly
selected core configuration. The child sets `JOBMAN_NO_EXTENSIONS=1` on its
core call to prevent recursion. Standard output and error are separately
bounded.

## Deterministic pipeline

1. Decode and verify one bounded evidence value.
2. Derive bounded tracebacks, panic stacks, JVM chains, and compiler
   diagnostics from already selected artifacts, retaining the core evidence
   unchanged and citing exact sanitized byte ranges.
3. Index factual items, artifacts, and attributed enrichment by stable ID.
4. Apply controlled core-failure rules before inspecting untrusted log bytes.
5. Add conservative target-output signatures, typed resource context, and
   exact store-local failure-history observations.
6. Rank and deduplicate findings deterministically.
7. Resolve prose or allowlisted read-only argument-vector actions and retry
   advice independently from the current Jobman policy state.
8. Build citations only for evidence IDs present in the source bundle.
9. Seal the report and validate every provenance value and reference against
   the original evidence.
10. For human output, build a non-persistent presentation view from that
    validated report and evidence, assign compact local aliases, format safe
    typed values, and never render raw artifact bytes.

Report identity excludes only generation wall time. A fixed evidence bundle,
engine version, companion version, and accepted generated proposal therefore
produce the same semantic `report_id`.

The companion never queries Jobman's store directly. Core performs indexed
fingerprint matching inside one state store and emits bounded, privacy-safe
summaries. This preserves core's ownership of transactionality, secret key
material, and exact-match semantics while keeping interpretation in the
companion.

## Generated augmentation

Generated analysis wraps, rather than replaces, the deterministic engine:

1. A strict schema-2 configuration names a default profile; every profile
   names exactly one adapter, endpoint or absolute
   command, locality, model, limits, credential reference, and disclosure
   allowance.
2. `--ai`, `-a`, or `--profile` explicitly activates generation and requests
   and approves metadata plus bounded command argument vectors, filesystem
   context, and environment variable names/roles. `--ai-logs` or
   `--share log_content` additionally requests
   and approves a bounded live tail. The CLI approvals and profile disclosure
   allowlist are intersected; `local_only` evidence is never projected on its
   own. Exact-range structure derived from a disclosed log is projected only
   with that source artifact and accounted separately. Log
   content also requires Jobman's `configured_value_redaction_v1` capability.
3. The projection is rejected, not silently truncated, if a class or total
   request exceeds its configured ceiling. Deterministic findings supported by
   excluded evidence are omitted from the request as well.
4. The companion seals a `jobman.diagnosis_generation_request` value containing
   typed data, an exact manifest, controlled hypothesis codes, candidates and
   actions, fixed
   untrusted-data instructions, and the response JSON Schema.
5. Exactly one configured generator is called. There is no discovery, proxy,
   redirect, provider fallback, tool call, or multi-agent loop.
6. The returned `jobman.diagnosis_proposal` is decoded again in Go and checked
   for size, structure, controlled values, valid citations, valid finding
   contradictions, and allowlisted action IDs.
7. Accepted hypotheses are appended below deterministic findings at a fixed
   uncalibrated confidence. Generated output cannot alter factual findings,
   create actions, or control retry advice.

Once a request is attempted, the final disclosure manifest conservatively
records the projection as disclosed even if transport or proposal validation
fails. Optional provider-stage failure reseals the complete deterministic
report with a controlled warning. `--require-model` instead returns an error.

The generation orchestrator also emits typed preparation, provider-waiting,
response-validation, and fallback lifecycle events. These events carry no
evidence or generated content. The CLI is the only renderer: it combines them
with evidence-collection state for delayed interactive animation or explicit
plain milestones on standard error. Engines and provider adapters never write
progress directly.

## Extension seams

The public `diagnosis.Diagnostician` interface accepts `FailureEvidence`
(verified core evidence plus separately attributed enrichment) and returns a
concrete report. `internal/generation.Augmenter` implements that
same domain interface around another diagnostician.

The public `provider.StructuredGenerator` interface is intentionally narrower
than a chat API: one bounded request asks for one JSON value constrained by a
supplied schema, and one bounded response carries nonsecret transport
provenance. Provider packages own transport mapping and authentication, while
projection and reconciliation remain provider-independent.

The initial adapters are:

- an absolute local command bridge with a minimal environment and no shell;
- an exact OpenAI-compatible Chat Completions endpoint using strict structured
  output; and
- a loopback-only Ollama `/api/chat` endpoint using its native schema format.

The production dependency graph contains no model SDK. A new adapter can be
added without changing core Jobman, the evidence contract, or the diagnosis
domain interface, but it must satisfy the same capability and conformance
tests.
