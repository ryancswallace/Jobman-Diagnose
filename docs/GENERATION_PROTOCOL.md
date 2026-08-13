# Structured-generation protocol 4

The provider boundary consists of a schema-4 generation request and a schema-2
proposal. It is a proposal protocol, not a chat, agent, tool, or remote Jobman
API.

## Request

`jobman.diagnosis_generation_request` schema 4 contains:

- a SHA-256 `request_id` over normalized semantic content;
- the sealed companion analysis-evidence ID, which commits to core evidence,
  attributed enrichment, and any selected source snapshot, plus a minimal subject;
- the exact approved evidence projection and accounting manifest;
- deterministic candidates whose every citation is in that projection;
- controlled hypothesis codes and categories plus existing non-executing
  action catalog entries;
- fixed instructions that classify all projected content as untrusted data;
- a maximum response byte count; and
- a request-specific response JSON Schema derived from the sealed authority
  catalogs.

The response schema binds `request_id` to the exact request digest and narrows
hypothesis codes, categories, evidence references, deterministic finding
references, and action IDs to request-specific `const` or `enum` values. It
also requires at least one supporting evidence reference for every hypothesis
and constrains the action array to empty when the deterministic report offers
no actions. Grammar-constrained providers therefore cannot invent those
scalar authority values and leave the host to discover the mismatch later.

The response schema is a deterministic derived field. Request identity hashes
the normalized authoritative request fields without that derived copy, then
the host rebuilds the schema from those fields and the resulting request ID
and requires exact equality during verification. This avoids a
self-referential hash while preventing schema substitution or authority
expansion.

Projected metadata and explicitly approved command, path, and environment-name
context retain typed JSON values, quality, timestamps, disclosure classes, and
stable evidence IDs. Store-local failure fingerprints and routine
complete-at-exit resource usage observations are intentionally omitted from
the generation projection because neither can explain the target-specific
cause and both can distract smaller models. The deterministic engine still
retains and analyzes the complete evidence bundle. Metadata may include bounded
point-in-time filesystem and cgroup constraints; cumulative cgroup event
counters do not claim per-run attribution. Direct command values preserve executable and ordered
argument boundaries. Environment items contain names and roles only. A log
artifact is bounded post-redaction UTF-8 data with its
source digest, byte accounting, and truncation state. It is data, never part of
the system instruction string. When log content is approved, deterministic
traceback, panic, JVM, compiler, and causal-message enrichment may accompany it
with collector provenance, exact source byte ranges, and bounded diagnostic
lines deterministically selected from those already disclosed ranges.
Traceback selection retains the terminal line of every visible exception-group
branch and cause-chain member; causal-message selection exposes short exact
lines such as a DNS name, TLS verification error, endpoint, rejected value, or
database constraint. These lines remain untrusted data and do not expand
artifact disclosure. The manifest accounts for their IDs and encoded bytes
separately. Other `local_only` evidence is never present.

An explicitly approved `source_content` artifact contains either one exact
continuous line window or one exact complete UTF-8 source file. Its projection
records the absolute path, language and media type, selection mode, line and
byte bounds, whole-file and selected-content digests, capture time, and
`point_in_time` quality. The current source is unredacted and untrusted. It is
companion-collected context rather than core Jobman evidence and does not
attest to the bytes executed by the recorded run. Fixed instructions require
the model to cite both runtime evidence and source context when source text
materially supports a cause. The host excludes source artifacts from direct
cause authorization, so source alone cannot enable a generated hypothesis.

Request decoding is bounded and rejects unknown fields, duplicate keys,
trailing values, excessive nesting, unsorted or duplicate IDs, manifest
mismatches, invalid evidence references, response-schema substitution, and
digest mutation.

## Proposal

`jobman.diagnosis_proposal` contains the matching `request_id` and only:

- zero or one hypothesis selected from the request's controlled code taxonomy,
  with a controlled category, an issue-specific summary, a concrete
  `root_cause`, a concise causal-path explanation, projected supporting
  evidence IDs, and contradiction arrays constrained to empty;
- up to eight action IDs selected from the supplied catalog; and
- up to eight descriptions of missing evidence.

The schema deliberately has no authority-bearing field for confidence, retry
advice, commands, arguments, tools, URLs, environment, paths, lifecycle facts,
or mutations. Diagnostic prose may identify a short path or endpoint from the
approved evidence, but cannot turn it into an operation. Host validation
rejects invented references and controlled values even after a backend reports
successful schema enforcement. It also retains relational checks that portable
grammar backends cannot express, including duplicate hypothesis codes and
duplicate catalog references. Fixed request instructions and schema field
descriptions state those relational rules explicitly so smaller local models
can satisfy them even though the grammar cannot encode cross-field equality.
An empty proposal is an abstention.

The fixed instructions define the intended meaning of each generated code,
reserve `generated.unknown_target_error` for cases where no specific supplied
code is supported, and require the model to distinguish the concrete incident
from its generic exit mechanism. Short exception names, setting names, paths,
endpoints, and diagnostic values from an approved projection may be reproduced
when necessary for specificity; complete artifacts, secrets, and
instruction-like target text remain prohibited. The host rejects known generic
root-cause restatements and proposals that mistake
tracebacks, enrichment metadata, or byte ranges for a root cause. A concise
summary, root cause, and concise explanation may overlap when the exact error
is itself the useful diagnosis. Host validation additionally requires direct
cited signals for every generated cause class, including resource exhaustion,
external-service failure, access denial, missing or unavailable dependencies,
transient infrastructure, and application defects. Ordinary CPU or memory
consumption cannot therefore be promoted into a resource-exhaustion claim
merely because it was observed. When cited content exposes a concrete endpoint,
causal path, or deepest exception chain, the proposal must retain the
corresponding distinguishing operand or causal identifier; prompts still ask
for both the deepest exception and operation when both are available.
Before reconciliation, the host collapses an optional generated explanation to
the supported root cause when it merely copies schema guidance, narrates Jobman
lifecycle bookkeeping, or introduces a high-salience causal condition absent
from the cited artifact. This preserves a useful grounded diagnosis without
displaying an unsupported secondary failure path.
When the evidence cannot support a specific cause, the model must abstain and
name the missing evidence instead. These are content-quality constraints in
addition to the request-specific structural authority enforced by JSON Schema
and the host.

## Reconciliation

The deterministic report exists before the provider call. Valid hypotheses are
appended at fixed uncalibrated confidence 40 and cannot contradict or replace
deterministic findings. The report preserves the generated
summary and renders the other proposal fields as explicit `Root cause` and
`Failure path` clauses. Action IDs may reorder only the existing deterministic
action list. For the first recognized generated cause code, reconciliation may
prepend fixed, non-executing guidance written by Jobman; the model supplies
neither its prose nor an execution vector. Unknown target errors receive no
specific guidance. Missing-evidence descriptions are advisory. Retry advice
and the primary finding remain deterministic.

When no projected artifact contains a direct causal signal, or the artifact
explicitly says it was truncated before the terminal exception or causal line,
the request-specific schema constrains the hypothesis array to empty so a
grammar-bound model must abstain. Provider or proposal failure produces a
sealed deterministic fallback with an exact attempted-disclosure manifest
unless the user selected `--require-model`. Parent cancellation is always
returned directly.

## Command bridge mapping

An explicitly configured absolute executable receives one request JSON value
on standard input and writes one raw proposal JSON value on standard output.
There is no shell and no inherited ambient environment. The child receives:

```text
JOBMAN_DIAGNOSE_PROVIDER_PROTOCOL=3
JOBMAN_DIAGNOSE_PROVIDER_MODEL=PROFILE_MODEL
JOBMAN_DIAGNOSE_REQUEST_ID=sha256:...
```

If the profile has a credential reference, its resolved value is provided only
as `JOBMAN_DIAGNOSE_PROVIDER_CREDENTIAL`. The child must not write credentials
or evidence to standard error. Input, output, error, wall time, and the Unix
process group are bounded by the parent.

## HTTP mappings

The OpenAI-compatible adapter maps the proposal schema to Chat Completions
`response_format` with type `json_schema` and `strict: true`. The request uses
one fixed system message and one user/data message containing the versioned
request. Streaming is disabled. Refusals and incomplete finish reasons are
provider failures.

The Ollama adapter maps the same schema to `/api/chat` `format`, disables
streaming and thinking, and sets temperature to zero. It is loopback-only.

HTTP transport envelopes may add fields, but duplicate keys, excessive
nesting, trailing values, oversized bodies, redirects, locality violations,
and malformed required fields fail closed. The inner proposal always receives
strict schema and semantic validation.
