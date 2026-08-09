# Compatibility

Status: unreleased development matrix

Compatibility is governed by explicit schemas and capabilities, not matching
semantic versions.

| Surface | Supported now |
| --- | --- |
| Jobman diagnostic evidence | Schema 1 |
| Jobman CLI envelope | Schema 1 with `data.evidence` |
| Jobman extension environment | Protocol 1 |
| Diagnosis report | Schema 1 |
| Deterministic engine | Version 1.2.0 |
| Diagnosis configuration | YAML schema 2 |
| Generation request | `jobman.diagnosis_generation_request` schema 1 |
| Generated proposal | `jobman.diagnosis_proposal` schema 1 |
| Support bundle | `jobman.diagnosis_support_bundle` schema 1 |
| Evaluation corpus/result | Schema 1 |
| Provider adapters | Command protocol 1, strict OpenAI-compatible Chat Completions, local Ollama `/api/chat` |

The companion rejects a newer required evidence schema with an actionable
error. It ignores unknown additive evidence item codes, but validates all known
envelope invariants, semantic identity, sizes, citations, and provenance.
Within report schema 1, existing fields and controlled values do not change
meaning or type. Breaking semantics require a new schema version.

Jobman store schema 8 is an implementation detail of core, not a companion
compatibility surface. It adds resource observations and store-local failure
fingerprints as additive evidence-schema-1 item codes. Bundles from schema-7
stores remain valid: they carry explicit resource or similarity omissions, and
the deterministic engine lowers its claims accordingly. Fingerprint history is
never assumed complete when core reports that older failures were not indexed.

Provider compatibility is narrower than accepting arbitrary JSON. Every
selected generator must claim native JSON Schema enforcement and satisfy the
profile's byte limits and locality before invocation. The host then validates
the response again against the exact schema-1 request. There is no fallback to
JSON mode, a different endpoint, a different provider, or a remote service.
OpenAI-compatible servers must implement the configured Chat Completions
strict `json_schema` request and response shape. Ollama profiles must implement
local `/api/chat` structured output. Command bridges must implement the raw
schema-1 stdin/stdout protocol documented in
[`GENERATION_PROTOCOL.md`](GENERATION_PROTOCOL.md).

Copied core fixtures under `testdata/jobman-v1/` record their Jobman origin,
evidence IDs, and exact SHA-256 values. Compatibility tests never download
fixtures. The corpus includes pre-fingerprint omissions, typed resource facts,
local-only fingerprints, exact same-fingerprint history, maximum-budget data,
and secret canaries. When Jobman publishes the first evidence release, the
manifest's temporary `unreleased` origin will be replaced by that tag and
retained as the oldest schema-1 fixture set.

The development `go.mod` uses a sibling checkout because the public evidence
package has not yet been tagged. This replacement is not a release contract and
will be removed before publishing the first companion module version.
