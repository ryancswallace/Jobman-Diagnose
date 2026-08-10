# Security and privacy model

The companion is a read-only native executable, not a sandbox. It receives the
invoking user's authority to execute core Jobman, read an explicitly named
evidence or configuration file, and, only when selected, invoke one configured
generator. It has no code path that mutates Jobman state, signals a target,
executes a suggested action, or starts a retry.

## Trust boundaries

- Jobman's sealed evidence is trusted only after structural, size, digest, and
  semantic verification.
- Target stdout and stderr are untrusted bytes. They may contain secrets,
  terminal control text, prompt injection, or misleading error messages.
- Every generator response is untrusted proposal data even when a backend
  enforces JSON Schema.
- Human report text is presentation, not an executable command vector.
- The selected Jobman executable, installed extension, local model runtime, and
  command bridge are native code trusted by the user through installation and
  configuration policy.

The live core client uses no shell, validates an absolute regular executable,
preserves argument boundaries, sets `JOBMAN_NO_EXTENSIONS=1`, bounds stdout to
3 MiB and stderr to 64 KiB, and propagates context cancellation.

## Data minimization and disclosure

Core schema 1 always excludes environment values, secret-reference identifiers
and values, input, notification destinations, credentials, and raw Go errors.
Plain deterministic collection omits commands, paths, and environment names.
Explicit AI activation requests bounded, field-redacted direct command
specifications (including ordered arguments), filesystem context, and
environment variable names/roles, plus allowlisted point-in-time capacity and
cgroup constraints from core. System context contains no hostname, mount or
cgroup path, process list, or system log. A profile must allow each class before
projection. Log content is separately opt-in and bounded.
The companion does not echo artifact bytes in the human report. Human output
may render allowlisted typed values from the already sealed evidence, including
sanitized command arguments, outcomes, exit details, resource observations,
and exact enrichment ranges. It uses report-local citation aliases; canonical
evidence IDs and controlled citation summaries remain unchanged in JSON.

Core failure fingerprints are HMAC values created with a private key held in
the Jobman state store. The key never enters evidence or this process. Command
arguments, environment values, log bytes, and generated text do not contribute
to a fingerprint. Fingerprints and bounded cross-job matching summaries are
classified `local_only` and are never projected to any generator, including a
local one.

Deterministic mode opens no diagnosis configuration, resolves no credential,
invokes no provider, and performs no network operation. Generated augmentation
requires all of the following:

- explicit per-invocation activation through `--ai`, `-a`, `--ai-logs`, or
  `--profile NAME`;
- a strict schema-2 configuration resolved from an explicit override or the
  platform per-user Jobman configuration directory;
- a strict profile allowance for each disclosure class; and
- matching per-invocation approval, with metadata, command, path, and
  environment-name context implied by explicit AI activation, bounded system
  context requested automatically for a live job, and log content requiring `--ai-logs` or
  `--share log_content`.

For live evidence, log-content approval automatically requests a bounded tail;
an explicit conflicting log mode is rejected. Log content additionally
requires the sealed core capability `configured_value_redaction_v1`.
That capability proves a value-aware configured redaction rule was active; it
does not prove that arbitrary output contains no secrets. Review evidence
before authorizing disclosure.

System context is collector-host, point-in-time metadata rather than a durable
per-run measurement. Linux cgroup event counters may include other processes
and prior events. They are advisory context for generated hypotheses and never
independently establish an out-of-memory diagnosis.

Projection uses exact item, artifact, and attributed-enrichment IDs and byte
ceilings. Enrichment can be projected only alongside its explicitly approved
source log and cannot cite outside that sanitized artifact. Projection fails
before invocation rather than silently truncating an over-limit request. The final
report records the attempted projection, profile, locality, model, request
digest, exact IDs, counts, bytes, and whether generated content was accepted.
An attempted request is treated as disclosed even when the adapter later
fails.

## Prompt injection and generated authority

Projected values remain typed data and are sent separately from fixed system
instructions. Every request says that projected values and artifacts are
untrusted data, forbids tools, commands, URLs, mutations, and retry verdicts,
and supplies a strict response schema.

Host validation remains authoritative after backend schema enforcement. It
rejects duplicate keys, trailing data, excessive nesting or size, unknown
fields, unsupported hypothesis codes or categories, invented citations, invalid contradictions,
and action IDs outside the deterministic catalog. A proposal can append an
uncalibrated hypothesis, reorder existing action IDs, or name missing evidence.
It cannot create a fact, action, command, URL, retry decision, or lifecycle
operation. Deterministic findings remain primary and deterministic retry advice
is never replaced.

Generation-request schema 2 derives a response schema from the sealed request.
It constrains the model to the exact request ID and request-specific hypothesis,
category, evidence, finding, and action catalogs before decoding. The host
reconstructs that derived schema during request verification and rejects any
substitution. Relational rules that grammar backends cannot portably enforce,
such as disjoint supporting and contradicting evidence or duplicate hypothesis
codes, are stated in fixed instructions and schema annotations, then still
checked after decoding.

## Configuration, credentials, and transports

Configuration is bounded and versioned. The YAML decoder rejects unknown or
duplicate fields, aliases, anchors, merge keys, multiple documents, unsupported
versions, ambiguous transports, and unsafe limits. AI mode may discover only
the platform per-user `jobman/diagnosis.yml`; there is no system, project,
working-directory, endpoint, provider, or credential-based activation.
`defaults.profile` selects only after an explicit AI activation flag.

Credentials are references to one environment variable or one private regular
file. Literal secrets are not representable in the schema. Errors omit HTTP
response bodies and command stderr because either may echo credentials or
evidence.

HTTP transports do not use ambient proxies or follow redirects. Remote
profiles require an exact HTTPS endpoint. Local profiles require a loopback
endpoint and recheck resolved addresses when dialing. The command bridge
requires a clean absolute regular executable, uses no shell, receives a minimal
environment, and bounds standard input, output, error, wall time, and the Unix
process group. Fixed command arguments are configuration and must not contain
credentials; use the credential reference instead.

Provider failure, refusal, malformed output, or semantic rejection becomes a
warning on a sealed deterministic report unless `--require-model` was set.
Cancellation from the caller always stops the operation rather than degrading
to a fallback report.

AI progress is rendered only by the CLI and always on standard error. Automatic
progress is disabled for JSON and non-interactive output. It contains a
controlled phase plus bounded, terminal-sanitized profile/model labels and the
configured timeout; it never contains endpoints, credentials, evidence,
prompts, provider responses, or log bytes. Generation and provider packages
emit typed lifecycle events but cannot write to process-global streams.

## Files

`--output`, `--export-evidence`, and `--support-bundle` require a path other
than `-`, create a
temporary private file in the destination directory, sync and close it, and
publish with a no-overwrite hard link. A destination race therefore fails
instead of replacing existing data. Evidence and reports can still contain
sensitive operational metadata and should be retained only as needed. A
support bundle contains only selected sealed evidence, attributed enrichment,
the report, disclosure, capability, and build metadata. It excludes provider
credentials, environment values, database files, and the fingerprint key;
`--bundle-dry-run` lists every member without creating the archive.

Report vulnerabilities privately using the process in the repository root
[`SECURITY.md`](../SECURITY.md).
