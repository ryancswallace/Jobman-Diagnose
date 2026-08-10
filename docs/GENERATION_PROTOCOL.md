# Structured-generation protocol 2

The provider boundary consists of a schema-2 generation request and a schema-1
proposal. It is a proposal protocol, not a chat, agent, tool, or remote Jobman
API.

## Request

`jobman.diagnosis_generation_request` schema 2 contains:

- a SHA-256 `request_id` over normalized semantic content;
- the sealed core evidence ID and a minimal subject;
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
stable evidence IDs. Metadata may include bounded point-in-time filesystem and
cgroup constraints; cumulative cgroup event counters do not claim per-run
attribution. Direct command values preserve executable and ordered
argument boundaries. Environment items contain names and roles only. A log
artifact is bounded post-redaction UTF-8 data with its
source digest, byte accounting, and truncation state. It is data, never part of
the system instruction string. When log content is approved, deterministic
traceback/compiler enrichment may accompany it with collector provenance and
exact source byte ranges. The manifest accounts for those IDs and encoded
bytes separately. Other `local_only` evidence is never present.

Request decoding is bounded and rejects unknown fields, duplicate keys,
trailing values, excessive nesting, unsorted or duplicate IDs, manifest
mismatches, invalid evidence references, response-schema substitution, and
digest mutation.

## Proposal

`jobman.diagnosis_proposal` contains the matching `request_id` and only:

- up to eight hypotheses selected from the request's controlled code taxonomy,
  with controlled categories, concise summary and explanation, projected
  supporting/contradicting evidence IDs, and optional
  deterministic finding IDs they contradict;
- up to eight action IDs selected from the supplied catalog; and
- up to eight descriptions of missing evidence.

The schema deliberately has no field for confidence, retry advice, commands,
arguments, tools, URLs, environment, paths, lifecycle facts, or mutations.
Host validation rejects invented references and controlled values even after a
backend reports successful schema enforcement. It also retains relational
checks that portable grammar backends cannot express, including duplicate
hypothesis codes, duplicate catalog references, and overlap between supporting
and contradicting evidence. Fixed request instructions and schema field
descriptions state those relational rules explicitly so smaller local models
can satisfy them even though the grammar cannot encode cross-field equality.
An empty proposal is an abstention.

The fixed instructions define the intended meaning of each generated code,
reserve `generated.unknown_target_error` for cases where no specific supplied
code is supported, ask for distinct cause and reasoning text, prohibit
verbatim artifact quotation, and request the smallest directly relevant
evidence set. These are content-quality constraints in addition to the
request-specific structural authority enforced by JSON Schema and the host.

## Reconciliation

The deterministic report exists before the provider call. Valid hypotheses are
appended at fixed uncalibrated confidence 40 and can explicitly contradict,
but cannot replace, deterministic findings. Action IDs may reorder only the
existing deterministic action list. For the first recognized generated cause
code, reconciliation may prepend fixed, non-executing guidance written by
Jobman; the model supplies neither its prose nor an execution vector. Unknown
target errors receive no specific guidance. Missing-evidence descriptions are
advisory. Retry advice and the primary finding remain deterministic.

Provider or proposal failure produces a sealed deterministic fallback with an
exact attempted-disclosure manifest unless the user selected
`--require-model`. Parent cancellation is always returned directly.

## Command bridge mapping

An explicitly configured absolute executable receives one request JSON value
on standard input and writes one raw proposal JSON value on standard output.
There is no shell and no inherited ambient environment. The child receives:

```text
JOBMAN_DIAGNOSE_PROVIDER_PROTOCOL=2
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
