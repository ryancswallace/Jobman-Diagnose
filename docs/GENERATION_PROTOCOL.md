# Structured-generation protocol 6

The provider boundary consists of a schema-6 generation request and a schema-3
proposal. It is a proposal protocol, not a chat, agent, tool, or remote Jobman
API.

## Request

`jobman.diagnosis_generation_request` schema 6 contains:

- a SHA-256 `request_id` over normalized semantic content;
- an `initial` or `review` stage; a review also contains the previously
  validated proposal as untrusted draft material;
- the sealed companion analysis-evidence ID, which commits to core evidence,
  attributed enrichment, and any selected source snapshot, plus a minimal subject;
- the exact approved evidence projection and accounting manifest;
- a derived `diagnostic_focus` copy of every host-selected enrichment
  diagnostic line, grouped by its existing evidence ID;
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
argument boundaries. Environment items contain names and roles only. When log
content is approved, the companion ranks exact causal and structured
diagnostic ranges across the selected runs and streams, then projects bounded
continuous context around the strongest ranges. Each causal context is bounded
by both bytes and a fixed number of surrounding lines, so repetitive short
messages cannot consume the model's attention merely because they fit the byte
ceiling. A structured range that fits the byte budget is not further trimmed
by the line bound. If no diagnostic range is
available, it uses bounded terminal output. A projected log context records its
selection and anchor reason, line and byte bounds within the sealed core
artifact, source and selected-content digests, capture quality, byte accounting,
and combined source/context truncation state. It is post-redaction UTF-8 data,
never part of the system instruction string. Deterministic
traceback, panic, JVM, compiler, and causal-message enrichment may accompany it
with collector provenance, byte ranges rebased into the projected context, and
bounded diagnostic lines deterministically selected from those already
disclosed ranges.
Traceback selection retains the terminal line of every visible exception-group
branch and every visible traceback in a cause chain. Diagnostic-line selection
also retains outer exceptions and operations, multi-field validation details,
Python syntax locations, and bounded pre-traceback remediation messages.
Causal-message selection exposes short excerpts such as a DNS name, TLS
verification error, endpoint, rejected value, or database constraint. Derived
lines omit ANSI styling and skip excerpts with other control characters. The
projected log and its attributed byte ranges retain the original log bytes
after the existing UTF-8 conversion. The lines remain untrusted data. The manifest
accounts for their IDs and encoded bytes separately. Other `local_only`
evidence is never present.

`diagnostic_focus` is a deterministic attention index over those selected
lines. It contains no text or evidence ID that is not already in the sealed
projection, is covered by the request digest, and is rebuilt and compared
exactly during verification. It makes the model account for each important
branch before compression without creating a second disclosure channel.

The trusted prompt and fixed request instructions use one evidence-first
sequence: inspect the bounded diagnostic lines, verify them against their
runtime artifact, preserve every material fact and cause/effect pair, then
classify the result. This keeps the attention contract compact while retaining
the same untrusted-data, authority, citation, abstention, and no-mutation
boundaries.

An explicitly approved `source_content` artifact contains either one exact
continuous line window or one exact complete UTF-8 source file. Limited
automatic collection may include the direct-command source and additional
runtime-referenced files beneath the recorded working directory, within the
profile's combined artifact and byte ceilings. Explicit-file and full-file
selection remain single-file operations. Each projection records the absolute
path, language and media type, selection mode, line and
byte bounds, whole-file and selected-content digests, capture time, and
`point_in_time` quality. The current source is unredacted and untrusted. It is
companion-collected context rather than core Jobman evidence and does not
attest to the bytes executed by the recorded run. Fixed instructions require
the model to cite both runtime evidence and source context when source text
materially supports a cause. The host excludes source artifacts from direct
cause authorization, so source alone cannot enable a generated hypothesis.
Before building the projection, the host compares source against locations in
the sealed target logs. Detectable file, line-range, or Python traceback-line
mismatches are not projected; the deterministic report carries a controlled
`source_context_mismatch` warning instead. A compatible location does not
upgrade the artifact beyond `point_in_time` quality.

Request decoding is bounded and rejects unknown fields, duplicate keys,
trailing values, excessive nesting, unsorted or duplicate IDs, manifest
mismatches, invalid evidence references, response-schema substitution, and
digest mutation.

A review request retains exactly the initial request's evidence projection,
taxonomy, deterministic candidates, and action catalog. Its prior proposal is
revalidated against that authority before sealing and contributes to the new
request digest, but never becomes evidence or expands authority. The encoded
request places the evidence projection and diagnostic focus before the prior
draft so evidence remains the primary context.

## Proposal

`jobman.diagnosis_proposal` contains the matching `request_id` and only:

- an `evidence_coverage` checklist that copies every `diagnostic_focus` line
  exactly under an allowed evidence ID before any causal synthesis. The host
  requires the complete global set of focus IDs and lines, rejects additions
  or omissions, and does not render this checklist directly;
- zero to four causal claims selected from the request's controlled code
  taxonomy: at most one `primary`, up to two independently supported
  `contributing` conditions, and up to two genuinely competing `alternative`
  explanations. Every claim has a controlled category, an issue-specific
  summary, a concrete `root_cause`, a concise causal-path explanation,
  projected supporting evidence IDs, and empty contradiction arrays. The
  initial-stage schema omits resolution fields so the first call can focus on
  causal analysis. A review-stage claim also has a non-executing resolution
  plan containing a verification observation, high-level remediation, and
  observable success condition;
- up to eight action IDs selected from the supplied catalog; and
- up to eight descriptions of missing evidence.

The schema deliberately has no authority-bearing field for confidence, retry
advice, commands, arguments, tools, URLs, environment, paths, lifecycle facts,
or mutations. Diagnostic prose may identify a short path or endpoint from the
approved evidence, but cannot turn it into an operation. Host validation
rejects invented references and controlled values even after a backend reports
successful schema enforcement. It also retains relational checks that portable
grammar backends cannot express, including invalid causal-role composition,
duplicate causal claims, and duplicate catalog references. Repeated copies of
the same otherwise valid evidence citation are sorted and compacted before
validation; they cannot add authority, and this avoids discarding an otherwise
grounded local-model diagnosis for harmless repetition. Fixed request instructions and schema field
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
An explicit remote HTTP 5xx response authorizes
`generated.external_service_failure`, while connection refusal, DNS, TLS,
reset, and deadline signals authorize `generated.dependency_unavailable`.
When a later log line explicitly marks one of those reachability failures as
`terminal=true`, it takes precedence over an earlier retry warning such as an
HTTP 503 response; the earlier warning cannot authorize a competing
external-service primary cause.
Receiving a 5xx response does not by itself prove a separately transient
infrastructure condition.
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

The deterministic report exists before any provider call. Valid hypotheses are
appended at bounded, role-sensitive, uncalibrated confidence and cannot
contradict or replace deterministic findings. The report preserves the generated
summary and renders the other proposal fields as explicit `Root cause` and
`Failure path` clauses. For a claim that cites an attributed enrichment, the
host also appends the exact bounded focus lines as a separately labeled
`Observed diagnostics` clause. This deterministic quotation preserves concrete
operands even when generated prose compresses them. Action IDs may reorder only
the existing deterministic action list. When review-stage resolution is
available, reconciliation appends each accepted claim's validated verification
and remediation as non-executing advisory actions; changes require confirmation,
alternative claims receive only a distinguishing action, and no action contains
an execution vector. Schema-description and deterministic-action boilerplate is
discarded instead of being shown as generated guidance. Missing-evidence
descriptions are advisory. Retry advice and the primary finding remain
deterministic.

Profiles default to one provider call. `maximum_analysis_passes: 2` permits one
review call when the request contains multiple log streams, multiple structured
diagnostics, both runtime and source context, or an initial multi-claim
assessment. The review uses the same configured provider and model, the same
evidence authority, and the same overall timeout. It independently checks the
first proposal for unsupported claims, missing branches, causal-role mistakes,
and incomplete resolution guidance, then returns a complete replacement. If
the optional review cannot fit, fails transport or validation, or returns an
empty proposal, the already validated first pass is retained with a controlled
warning. A valid review also replaces the first pass only when it retains at
least as much grounded causal detail; otherwise the first pass is retained with
a controlled regression warning. There is no open-ended loop or provider
fallback.

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
