# jobman-diagnose

[![Test](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/test.yml/badge.svg)](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/test.yml)
[![CodeQL](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/codeql.yml/badge.svg)](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/ryancswallace/jobman-diagnose/badge)](https://securityscorecards.dev/viewer/?uri=github.com/ryancswallace/jobman-diagnose)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

`jobman-diagnose` is the optional diagnostic companion for
[Jobman](https://github.com/ryancswallace/jobman). It turns bounded, versioned
core evidence into a cited diagnosis report with confidence, limitations,
recommended next actions, and explicit retry advice.

Status: deterministic diagnosis, generated augmentation, and initial provider
adapters are implemented on `main`; not yet released. Live provider evaluation
and release packaging remain gates before a first tag.

[Documentation](docs/README.md) · [Installation](docs/INSTALLATION.md) ·
[Contributing](CONTRIBUTING.md) · [Support](SUPPORT.md) ·
[Security](SECURITY.md)

The default mode is local, deterministic, read-only, network-free, and does not
require configuration, credentials, Python, or a model runtime. Generated
augmentation is explicit opt-in through `--ai`, `-a`, `--ai-logs`, or
`--profile`. AI mode reads a strict per-user `diagnosis.yml`, uses its default
profile, and explicitly requests and approves bounded metadata plus a typed
execution context: direct command argument vectors, paths, environment variable
names (never values), and effective execution policy. Log content remains a
separate opt-in. AI can append validated, uncalibrated hypotheses but cannot
replace Jobman facts, choose retry policy, create an executable action, or
mutate a job.

## What it reports

Each report contains:

- a controlled primary diagnosis and any secondary observations;
- a 0–100 confidence score, readable band, and basis statement;
- exact Jobman evidence IDs supporting or contradicting each finding, with
  compact report-local aliases in human output;
- prose actions and allowlisted read-only Jobman argument vectors (displayed,
  never executed);
- `now`, `after_delay`, `after_change`, `no`, `not_applicable`, or `unknown`
  retry advice;
- missing evidence and consistency or security warnings;
- core/analysis/report IDs, grouping fingerprints, analyzer/generator
  provenance, and independently versioned components; and
- an exact disclosure manifest showing whether a provider was invoked, its
  locality, request digest, projected IDs/classes/bytes, and whether generated
  content was accepted.

Confidence measures the strength of a controlled rule and its cited evidence.
It is not presented as a probability. A report is advisory: the companion
cannot signal, rerun, mutate, or repair a Jobman job.

## Build during coordinated development

The module currently uses a local `replace` for the adjacent unreleased Jobman
evidence package. Clone both repositories as siblings, then build and test:

```console
git clone https://github.com/ryancswallace/jobman.git
git clone https://github.com/ryancswallace/jobman-diagnose.git
cd jobman-diagnose
make check
```

Until that package has a release tag, coordinated GitHub Actions workflows pin
their Jobman checkout to the exact compatible core revision. When the evidence
contract changes, first push the corresponding Jobman commit, then update the
pin in `test.yml`, `codeql.yml`, `fuzz.yml`, and the compatibility test before
pushing the companion. This order ensures every companion workflow can resolve
the sibling module reproducibly.

Before the first companion release, the local replacement will be removed and
the module will require the first tagged Jobman version containing diagnostic
evidence schema 1.

## Use it

Put both `jobman` and `jobman-diagnose` on `PATH`. Jobman's extension protocol
then provides the natural form:

```console
jobman diagnose JOB
jobman diagnose --ai JOB
jobman diagnose --ai-logs JOB
jobman diagnose --logs tail --log-bytes 64KiB JOB
jobman diagnose --json JOB
```

Direct invocation works too:

```console
jobman-diagnose --jobman /absolute/path/to/jobman JOB
```

By default the companion asks core for log metadata but no log bytes. A bounded
local-only tail requires `--logs tail`; `--ai-logs` collects and shares the
bounded redacted tail in one step. The current core bundle ceiling is 1 MiB. Select a
specific run with `--run N`, or compare bounded run history with `--all-runs`.
Use `--similar N` to request up to N other failures with the same exact,
store-local factual fingerprint. Current runs also include typed process CPU
observations and, where the operating system supplies it, peak resident memory.
Older runs created before Jobman store schema 8 are not backfilled, and the
report calls out that partial history explicitly.

Evidence can be reviewed, transported, and diagnosed offline:

```console
jobman-diagnose --export-evidence evidence.json JOB
jobman-diagnose --from-evidence evidence.json
jobman-diagnose --from-evidence evidence.json --json --output report.json
```

Explicit export and output paths are created with private permissions,
published atomically, and never overwritten. `--from-evidence -` reads a raw
evidence value or Jobman's JSON envelope from standard input.

Create a reproducible private support archive, or inspect its complete
inventory without writing it:

```console
jobman diagnose --support-bundle diagnosis-support.tar.gz --bundle-dry-run JOB
jobman diagnose --support-bundle diagnosis-support.tar.gz JOB
```

The archive contains selected sealed evidence, attributed enrichment, report,
disclosure, capabilities, and build metadata—never credentials, environment
values, database files, or the fingerprint key. See [support
bundles](docs/SUPPORT_BUNDLES.md).

## Optional generated augmentation

Generated analysis uses the configured default profile and approves bounded
metadata plus the typed execution context with one explicit flag:

```console
jobman diagnose --ai JOB
jobman diagnose -a JOB
```

Use `--profile hosted` to select a non-default profile. The configuration path
defaults to the per-user Jobman configuration directory; an explicit
`--diagnosis-config PATH` or `JOBMAN_DIAGNOSE_CONFIG` overrides it. Inspect the
effective setup with:

```console
jobman diagnose config paths
jobman diagnose config validate
jobman diagnose config show
jobman diagnose profiles
```

Interactive human-output runs show a delayed AI progress indicator on standard
error, including the current phase, elapsed phase time, selected profile/model,
provider timeout while waiting, and a cancellation reminder. Fast calls finish
before the 300 ms delay without flicker. Automatic progress is silent for JSON
and redirected output. Use `--progress plain` for durable milestone lines or
`--progress off` to suppress progress explicitly:

```console
jobman diagnose --ai --progress plain JOB
jobman diagnose --ai --progress off JOB
```

The initial adapters are an absolute local command bridge, an exact
OpenAI-compatible Chat Completions endpoint, and local Ollama `/api/chat`.
Endpoints are never discovered or changed, HTTP proxies are not used,
redirects are not followed, remote profiles require HTTPS, and local profiles
must resolve only to loopback addresses. Literal credentials are rejected;
profiles can reference an environment value or a private regular file.

`metadata`, `command`, `path`, and `environment_name` are implied by AI
activation. The `command` class preserves direct executable and ordered
argument boundaries for the target, wait probes, and command notifiers. The
path class includes working directories, configured file paths, and per-run
resolved executables. Environment evidence contains names and roles only—never
literal values or secret-reference identifiers. Each selected profile must
allow a class before it is projected. Share log
content with the single-purpose shortcut:

```console
jobman diagnose --ai-logs --log-bytes 64KiB JOB
```

Equivalently, `--ai --share log_content` automatically collects a live log
tail. `log_content` still requires all of:

- a profile that allows bounded `log_content`;
- per-invocation `--ai-logs` or `--share log_content` intent; and
- Jobman evidence capability `configured_value_redaction_v1`, proving a
  value-aware redaction pattern was active before evidence was sealed.

Store-local fingerprints and similar-job summaries are never projected to a
generator. With explicit log approval, exact-range traceback/compiler
enrichment is projected only alongside its sanitized source artifact.
Generated hypotheses must select from the request's controlled taxonomy.
Optional provider or proposal failure returns the complete
deterministic report with a warning. `--require-model` instead returns a safe
operation error. See [configuration](docs/CONFIGURATION.md) and the
[generation protocol](docs/GENERATION_PROTOCOL.md).

## Example human output

```text
Diagnosis

  [F1] The target exited with a nonzero status
  Confidence: High (82/100)
  Why: The exit status confirms target failure, but does not by itself identify the target's
       root cause.
  Evidence: [E1], [E2], [E3]

Job

  Name: test
  ID: 019fe6ab-0e59-72bf-bdca-40f7ac7c5faa
  Run: 1
  Command: /usr/bin/example --mode batch
  State: Failed (job completed)

Retry

  Recommendation: Retry after changing the command, environment, resources, or policy
  Automatic policy: The current policy will not retry this failure

Evidence

  [E1] Observed fact — Job outcome: failure
  [E2] Observed fact — Run 1 exit code: 2
  [E3] Confirmed fact — Run 1 failure class: nonzero exit

Recommended next steps

  1. Inspect the cited evidence and bounded target logs
     Identify the target-specific error before changing configuration or creating another run.
     Suggested command: jobman show evidence --logs=metadata test
```

Human wording is for people. Automation should consume the sealed
`jobman.diagnosis_report` schema 1 JSON value documented in
[`docs/REPORT_SCHEMA.md`](docs/REPORT_SCHEMA.md). Human evidence aliases such
as `[E2]` and finding aliases such as `[F1]` are local to one rendering; JSON
retains every canonical ID and digest unchanged.

## Trust and privacy boundary

Core Jobman decides what factual evidence exists and sanitizes it before
sealing. The companion verifies the evidence digest and limits before analysis,
treats target output as untrusted data rather than instructions, never echoes
raw artifact bytes in the human report, and validates every citation against
the exact evidence bundle.

Failure fingerprints are opaque keyed values scoped to one Jobman state store.
They and cross-job matching summaries are marked `local_only`; the private key,
raw command, arguments, paths, environment, and log content are not part of the
similarity evidence.

Deterministic mode performs no network operation. Target logs may still contain
unknown secrets even after configured redaction, so review an exported bundle
before sharing it. A local model is still native code with the invoking user's
authority; a remote profile discloses its exact projection to another system.
Installing an extension grants it the invoking user's authority; verify
releases and control `PATH` accordingly.

See the [security model](docs/SECURITY_MODEL.md), [compatibility
matrix](docs/COMPATIBILITY.md), and [architecture](docs/ARCHITECTURE.md) for the
full boundary.

## Project layout

```text
cmd/jobman-diagnose/  executable boundary
diagnosis/            stable report contract and validation
provider/             narrow optional structured-generation seam
provider/commandbridge bounded local command protocol
provider/openaicompat exact strict-schema compatible HTTP transport
provider/ollama/      local Ollama structured-output transport
internal/config/      strict profiles and credential references
internal/generation/  disclosure projection, fallback, and reconciliation
internal/coreclient/  bounded direct Jobman process client
internal/engine/      deterministic analyzers, ranking, actions, retry policy
internal/enrichment/  bounded exact-range structure from selected artifacts
internal/evaluation/  checked-in diagnosis quality corpus and metrics
internal/presentation human rendering without raw artifacts
internal/securefile/  private no-overwrite exports
internal/supportbundle deterministic private support archives
devel/evaluate/       deterministic and opt-in live evaluation runner
testdata/              copied core compatibility fixtures
```

## Development

```console
make setup
make format
make lint
make test
make evaluate
make build
make check
```

The production dependency graph has no provider SDK. Its only non-Jobman
runtime dependency is the strict YAML decoder used for explicit profiles.
Tests use copied fixtures, local fake servers, and helper processes; they never
require a live provider, credentials, or a developer home directory.
The complete gate also checks module integrity, reachable vulnerabilities,
workflow syntax, every supported cross-build target, and the GoReleaser build
matrix.

Live provider evaluation is an explicit release-candidate activity; see the
[evaluation guide](docs/EVALUATION.md). Decoder fuzz targets and scheduled
CodeQL/fuzz workflows supplement the normal race-enabled test gate.
