# Compatibility

Status: Jobman Diagnose v0.4.0 compatibility matrix

Compatibility is governed by explicit schemas and capabilities, not matching
semantic versions.

| Surface | Supported now |
| --- | --- |
| Jobman Diagnose release | v0.4.0 |
| Jobman release | v1.4.0 or newer with evidence schema 1 |
| Jobman diagnostic evidence | Schema 1 |
| Jobman CLI envelope | Schema 1 with `data.evidence` |
| Jobman extension environment | Protocol 1 |
| Diagnosis report | Schema 1 |
| Deterministic engine | Version 1.2.0 |
| Diagnosis configuration | YAML schema 2 |
| Generation request | `jobman.diagnosis_generation_request` schema 4 |
| Generated proposal | `jobman.diagnosis_proposal` schema 2; report decoding retains recorded schema 1 provenance |
| Support bundle | `jobman.diagnosis_support_bundle` schema 2 |
| Evaluation corpus/result | Corpus schema 3; result schema 3 |
| Provider adapters | Command protocol 2, strict OpenAI-compatible Chat Completions, local Ollama `/api/chat` |

The companion rejects a newer required evidence schema with an actionable
error. It ignores unknown additive evidence item codes, but validates all known
envelope invariants, semantic identity, sizes, citations, and provenance.
Within report schema 1, existing fields and controlled values do not change
meaning or type. Breaking semantics require a new schema version.

AI-mode live collection requests Jobman's additive `--system` context. When the
minimum supported Jobman v1.4.0 reports that exact flag as unknown, the client
performs one safe read-only retry without it. Other collection failures are not
retried or hidden. Newer bundles may contain the additive
`jobman.system.context` metadata item; v0.1.0-era decoders preserve and project
unknown schema-1 items normally.

Jobman store schema 8 is an implementation detail of core, not a companion
compatibility surface. It adds resource observations and store-local failure
fingerprints as additive evidence-schema-1 item codes. Bundles from schema-7
stores remain valid: they carry explicit resource or similarity omissions, and
the deterministic engine lowers its claims accordingly. Fingerprint history is
never assumed complete when core reports that older failures were not indexed.

Provider compatibility is narrower than accepting arbitrary JSON. Every
selected generator must claim native JSON Schema enforcement and satisfy the
profile's byte limits and locality before invocation. The host then validates
the response again against the exact schema-4 request. The response schema
binds the exact request ID and request-specific code, category, citation,
finding, and action catalogs before generation; relational validation remains
authoritative after decoding. There is no fallback to JSON mode, a different
endpoint, a different provider, or a remote service.
OpenAI-compatible servers must implement the configured Chat Completions
strict `json_schema` request and response shape. Ollama profiles must implement
local `/api/chat` structured output. Command bridges must implement the raw
generation-request-schema-4/proposal-schema-2 stdin/stdout protocol documented
in [`GENERATION_PROTOCOL.md`](GENERATION_PROTOCOL.md).

Copied core fixtures under `testdata/jobman-v1/` record their Jobman v1.4.0
origin, evidence IDs, and exact SHA-256 values. Compatibility tests never
download fixtures. The corpus includes pre-fingerprint omissions, typed
resource facts, local-only fingerprints, exact same-fingerprint history,
maximum-budget data, and secret canaries. The v1.4.0 fixture set is retained as
the oldest schema-1 baseline even after newer Jobman releases add compatible
evidence items.

The development and release module both resolve a tagged Jobman dependency.
Continuous integration uses the module graph directly, which verifies that a
clean checkout can build without an unpublished sibling repository.
