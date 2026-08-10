# Configuration

Deterministic diagnosis is the default and reads no diagnosis configuration or
provider credential. `--deterministic` makes that choice explicit. Even when
`--diagnosis-config` names a malformed file, deterministic mode does not open
it.

Generated augmentation is activated by `--ai`, `-a`, `--ai-logs`, or
`--profile NAME`. `--ai` uses the configured default profile and approves the
bounded `metadata`, `command`, `path`, and `environment_name` classes for that
invocation. Command evidence preserves executable and ordered argument
boundaries; environment evidence contains names and roles but never values or
secret-reference identifiers. For live collection, AI also requests bounded
point-in-time state-filesystem and Linux cgroup-v2 context in the `metadata`
class. AI never activates merely because a
configuration or credential exists.

## AI progress output

`--progress` accepts `auto`, `plain`, or `off` and defaults to `auto`. Progress
is emitted only when an AI profile is selected and always goes to standard
error, leaving report output on standard output unchanged.

- `auto` starts a spinner after 300 ms only when standard error is an
  interactive terminal and the report is human-readable. It shows evidence
  collection, bounded evidence preparation, provider waiting, response
  validation, and optional deterministic fallback. The provider-waiting phase
  includes elapsed time, the profile/model, configured timeout, and a `Ctrl-C`
  reminder.
- `plain` writes one non-animated line for each distinct phase. It is explicit,
  so it also works with `--json` or redirected output for CI troubleshooting.
- `off` always suppresses progress.

Automatic JSON and non-interactive runs remain silent. Profile and model labels
are bounded and stripped of terminal control characters; progress never shows
an endpoint, credential, evidence value, prompt, response, or log content.

## Configuration paths and precedence

AI mode resolves its configuration in this order:

1. `--diagnosis-config PATH`;
2. `JOBMAN_DIAGNOSE_CONFIG`, which must be a clean absolute path; and
3. `diagnosis.yml` in the platform per-user Jobman configuration directory.

The ordinary defaults are `~/Library/Application Support/jobman/diagnosis.yml`
on macOS, `${XDG_CONFIG_HOME:-$HOME/.config}/jobman/diagnosis.yml` on Linux,
and the corresponding user configuration directory on Windows. There is no
system or project-file discovery. Inspect the concrete paths with
`jobman diagnose config paths`.

## `diagnosis.yml` schema 2

The decoder is bounded, accepts exactly one YAML document, rejects duplicate
keys, aliases, anchors, merge keys, unknown fields, and unsupported versions,
and validates every profile before resolving a credential. The file must be a
regular file rather than a symlink and, on Unix-like systems, must not be group
or world writable.

```yaml
schema_version: 2

defaults:
  profile: hosted

profiles:
  hosted:
    provider: openai_compatible
    locality: remote
    endpoint: https://api.example.com/v1/chat/completions
    model: structured-output-model
    require_json_schema: true
    timeout: 30s
    maximum_input_bytes: 262144
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
      command:
        maximum_items: 16
        maximum_bytes: 131072
      path:
        maximum_items: 256
        maximum_bytes: 131072
      environment_name:
        maximum_items: 256
        maximum_bytes: 131072
    credential:
      environment: PROVIDER_API_KEY
```

Profile names contain only lowercase letters, digits, `.`, `_`, and `-`.
`defaults.profile` is required and must name one defined profile.
`require_json_schema` must be true. Deadlines range from `100ms` through `5m`;
input limits range from 4 KiB through 2 MiB; output limits range from 1 KiB
through 256 KiB.

`disclosure.metadata` is mandatory and requires `maximum_items` and
`maximum_bytes`. Optional `command`, `path`, and `environment_name` classes
bound typed execution-context items with `maximum_items` and `maximum_bytes`;
optional `log_content` requires
`maximum_artifacts` and `maximum_bytes`:

```yaml
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
      command:
        maximum_items: 16
        maximum_bytes: 131072
      path:
        maximum_items: 256
        maximum_bytes: 131072
      environment_name:
        maximum_items: 256
        maximum_bytes: 131072
      log_content:
        maximum_artifacts: 2
        maximum_bytes: 65536
```

These are hard ceilings, not truncation requests. If the approved evidence
exceeds a profile limit, the provider is not invoked and the command reports a
policy error. Reduce core collection (`--run`, `--log-bytes`, or omit
`--all-runs`) or deliberately revise the profile.

The AI activation flag and profile are intersected. A profile allowance alone
never sends a class. `--ai` and `--profile` approve metadata plus bounded
command, path, environment-name, and system context; selecting a non-default profile is
therefore concise:

```console
jobman-diagnose --from-evidence evidence.json \
  --profile hosted
```

`--share` approves additional classes and accepts repeated or comma-separated
values. Schema 2 supports `metadata`, `command`, `path`, `environment_name`, and
`log_content`. For a live job,
`--share log_content` automatically changes the default log collection mode to
`tail`; an explicit `--logs metadata` or `--logs none` conflicts. `--ai-logs`
combines AI activation, execution-context approval, tail collection, and
log-content approval. `local_only` and `sensitive` classes are never eligible.
In particular, failure fingerprints and same-fingerprint
history are excluded even for local models.

When an approved log contains a recognized traceback, panic stack, JVM chain,
or compiler diagnostic, the request also includes the companion collector's
code and exact source byte range. This attributed enrichment adds no bytes from
outside the already approved artifact and is accounted separately in the
disclosure manifest.

Use `--log-bytes` to reduce or enlarge the bounded tail. Log content is never a
persistent configuration default; each invocation must use `--ai-logs` or
`--share log_content`.

## Inspection commands

The following commands do not resolve credentials or contact a provider:

```console
jobman diagnose config paths
jobman diagnose config validate [PATH]
jobman diagnose config show [PATH]
jobman diagnose profiles [--diagnosis-config PATH]
```

`config show` emits validated JSON. `profiles` marks the default profile with
`*` and reports provider, locality, model, and allowed disclosure classes.

## Credentials by reference

A profile may reference exactly one environment value:

```yaml
credential:
  environment: PROVIDER_API_KEY
```

or one clean absolute private-file path:

```yaml
credential:
  file: /absolute/private/path/provider-token
```

Credential files must be regular and no larger than 64 KiB. On Unix-like
systems they must grant no group or other permission bits. A final newline is
removed. Literal token, key, header, or password fields are unknown and
therefore rejected. URLs with user information, query strings, or fragments
are also rejected. Provider response bodies and command stderr are never copied
into errors because they may echo a credential or evidence.

## Provider profiles

### OpenAI-compatible HTTP

This adapter sends one non-streaming Chat Completions request with
`response_format.type: json_schema`, `strict: true`, and the embedded proposal
schema. It accepts an explicitly configured hosted endpoint or a loopback
self-hosted compatible endpoint.

Remote profiles require HTTPS and a non-loopback host. Local profiles may use
HTTP or HTTPS but the host must be `localhost` or a loopback IP and every DNS
answer is rechecked at dial time:

```yaml
profiles:
  local-compatible:
    provider: openai_compatible
    locality: local
    endpoint: http://127.0.0.1:8000/v1/chat/completions
    model: local-structured-model
    require_json_schema: true
    timeout: 45s
    maximum_input_bytes: 524288
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
      command:
        maximum_items: 16
        maximum_bytes: 131072
      path:
        maximum_items: 256
        maximum_bytes: 131072
      environment_name:
        maximum_items: 256
        maximum_bytes: 131072
```

The adapter follows the current [OpenAI Structured Outputs contract]. A model
selected for this generic endpoint must actually support strict structured
output; there is no fallback to JSON mode.

### Ollama

Ollama uses its native local endpoint. The exact path must be `/api/chat` and
locality must be `local`:

```yaml
profiles:
  local-ollama:
    provider: ollama
    locality: local
    endpoint: http://127.0.0.1:11434/api/chat
    model: YOUR_LOCAL_MODEL
    require_json_schema: true
    timeout: 60s
    maximum_input_bytes: 524288
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
      command:
        maximum_items: 16
        maximum_bytes: 131072
      path:
        maximum_items: 256
        maximum_bytes: 131072
      environment_name:
        maximum_items: 256
        maximum_bytes: 131072
```

The response schema is placed in Ollama's `format` field, streaming and
thinking output are disabled, and temperature is zero, matching Ollama's
[structured-output guidance]. Ollama Cloud is not selected because its current
documentation does not support structured outputs there.

### Absolute command bridge

The command provider launches one clean absolute regular executable directly,
with fixed arguments and no shell:

```yaml
profiles:
  local-bridge:
    provider: command
    locality: local
    command:
      executable: /absolute/path/to/my-generator
      arguments: [--mode, jobman]
    model: bridge-model-name
    require_json_schema: true
    timeout: 30s
    maximum_input_bytes: 262144
    maximum_output_bytes: 32768
    disclosure:
      metadata:
        maximum_items: 256
        maximum_bytes: 131072
      command:
        maximum_items: 16
        maximum_bytes: 131072
      path:
        maximum_items: 256
        maximum_bytes: 131072
      environment_name:
        maximum_items: 256
        maximum_bytes: 131072
```

The child receives one `jobman.diagnosis_generation_request` schema-1 JSON
value on standard input and must write one raw
`jobman.diagnosis_proposal` schema-1 JSON value to standard output. Its
environment is minimal and does not inherit ambient variables. The bridge sets
`JOBMAN_DIAGNOSE_PROVIDER_PROTOCOL=1`, `JOBMAN_DIAGNOSE_PROVIDER_MODEL`, and
`JOBMAN_DIAGNOSE_REQUEST_ID`; an explicitly referenced credential is available
only as `JOBMAN_DIAGNOSE_PROVIDER_CREDENTIAL`. Standard output, standard error,
wall time, and the Unix process group are bounded.

## Failure and precedence

Configuration and disclosure-policy errors fail before invocation. Once an
approved request is attempted, transport failure, refusal, malformed output,
invented citations, or semantic violations produce a sealed deterministic
fallback report with an exact disclosure manifest and a controlled warning.
`--require-model` turns those provider-stage failures into a safe nonzero
operation error; it never relaxes validation. Provider failures include a
stable, nonsecret reason such as `request_timeout`, `http_status`,
`response_truncated`, `incomplete_response`, `invalid_response`, or
`structured_content_oversized`. The diagnostic never includes provider error
bodies, model output, evidence values, credentials, or an untrusted low-level
error string.

The profile `timeout` bounds the complete provider operation, including the
wait for a non-streaming HTTP provider to finish inference and return response
headers. Choose a value that covers cold model startup and worst-case
structured generation; the supported range is 100ms through 5m.

Core invocation context remains separate. When launched through Jobman
extension protocol 1, `JOBMAN_EXECUTABLE`, `JOBMAN_STATE_DIR`, and optional
`JOBMAN_CONFIG` identify core acquisition. Users should not set those reserved
variables manually; direct invocation should use `--jobman`, `--state-dir`,
and `--config`.

[OpenAI Structured Outputs contract]: https://developers.openai.com/api/docs/guides/structured-outputs
[structured-output guidance]: https://docs.ollama.com/capabilities/structured-outputs
