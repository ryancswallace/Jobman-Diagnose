<!-- markdownlint-disable MD033 -->
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/logo-diagnose.svg">
  <img alt="Jobman" src="assets/logo-dark-diagnose.svg" width="420">
</picture>
<!-- markdownlint-enable MD033 -->

[![Test](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/test.yml/badge.svg)](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/test.yml)
[![Codecov](https://codecov.io/gh/ryancswallace/Jobman-Diagnose/branch/main/graph/badge.svg)](https://codecov.io/gh/ryancswallace/Jobman-Diagnose)
[![CodeQL](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/codeql.yml/badge.svg)](https://github.com/ryancswallace/jobman-diagnose/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ryancswallace/Jobman-Diagnose/badge)](https://scorecard.dev/viewer/?uri=github.com/ryancswallace/Jobman-Diagnose)
[![Latest release](https://img.shields.io/github/v/release/ryancswallace/jobman-diagnose?sort=semver)](https://github.com/ryancswallace/jobman-diagnose/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/ryancswallace/jobman-diagnose)](https://github.com/ryancswallace/jobman-diagnose/blob/main/go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/ryancswallace/jobman-diagnose/diagnosis.svg)](https://pkg.go.dev/github.com/ryancswallace/jobman-diagnose/diagnosis)
[![Documentation](https://img.shields.io/badge/docs-reference-blue)](docs/README.md)
[![OSS hosting by Cloudsmith](https://img.shields.io/badge/OSS%20hosting%20by-Cloudsmith-blue?logo=cloudsmith)](https://cloudsmith.com/)

Jobman-Diagnose explains why a [Jobman] job failed and what to do next. The
optional, read-only companion turns bounded Jobman evidence into a cited
diagnosis with confidence, limitations, recommended actions, and explicit
retry advice.

It works locally without configuration, credentials, Python, network access,
or a model. Optional AI augmentation adds schema-validated hypotheses through
pluggable local or hosted providers without overriding deterministic facts.

<!-- Terminal demo slot. Generate the recording from
docs/screencaps/tape/diagnose.tape, then replace this comment with:
<p align="center">
  <img src="docs/screencaps/gif/diagnose.gif"
       alt="A Jobman-Diagnose terminal demo"
       width="900">
</p>
-->

> [!TIP]
> Start with `jobman diagnose JOB`; no model or configuration is required. See
> the [documentation] for installation, AI setup, troubleshooting, and stable
> data contracts.

## Features

| Capability | Jobman-Diagnose provides... |
| --- | --- |
| Actionable diagnoses | Controlled findings, likely causes, next actions, and retry recommendations |
| Cited evidence | Every factual finding points to exact evidence supplied and sealed by Jobman |
| Deterministic defaults | Useful, reproducible, network-free analysis without a model |
| Guarded AI augmentation | Strict structured generation whose proposals are validated before inclusion |
| Controlled disclosure | Per-profile evidence allowlists, bounded projections, and an exact disclosure manifest |
| Portable reports | Stable JSON, private evidence export, offline diagnosis, and reproducible support bundles |
| Pluggable providers | OpenAI-compatible endpoints, local Ollama, and a bounded command bridge |
| Safe operation | Read-only advice that never signals, retries, mutates, or repairs a job |

> [!NOTE]
> Jobman-Diagnose is currently pre-v1. Check the [compatibility contract] before
> combining versions. Generated hypotheses are advisory and uncalibrated;
> deterministic facts, actions, and retry policy remain authoritative.

## Command overview

| Task | Command |
| --- | --- |
| Diagnose locally | `jobman diagnose JOB` |
| Add AI hypotheses | `jobman diagnose --ai JOB` |
| Share a bounded redacted log tail with AI | `jobman diagnose --ai-logs JOB` |
| Include local system constraints | `jobman diagnose --system JOB` |
| Produce stable machine output | `jobman diagnose --json JOB` |
| Expand the human audit or control color | `jobman diagnose --details JOB`, `jobman diagnose --color=never JOB` |
| Export or replay evidence | `jobman-diagnose --export-evidence FILE JOB`, `jobman-diagnose --from-evidence FILE` |
| Create a private support archive | `jobman diagnose --support-bundle FILE JOB` |
| Inspect AI configuration | `jobman diagnose config show`, `jobman diagnose profiles` |

Install both binaries on `PATH`; Jobman's external-command protocol provides
the natural `jobman diagnose` form. Direct invocation also works:

```console
jobman-diagnose --jobman /absolute/path/to/jobman JOB
```

Human output is designed for scanning; automation should consume the sealed
[`jobman.diagnosis_report` schema 1][report schema] JSON value. Human aliases
such as `[E2]` and `[F1]` are report-local, while JSON retains canonical IDs
and digests.

## Installation

Install Jobman v1.4.0 or newer first, then choose the package or archive for
your system:

| Environment | Recommended installation |
| --- | --- |
| macOS | `brew install ryancswallace/tap/jobman-diagnose` |
| Debian or Ubuntu | Configure [Cloudsmith], then `sudo apt install jobman jobman-diagnose` |
| Fedora, RHEL, Rocky, AlmaLinux, Amazon Linux | Configure [Cloudsmith], then `sudo dnf install jobman jobman-diagnose` |
| Alpine Linux | Configure [Cloudsmith], then `sudo apk add jobman jobman-diagnose` |
| Other Linux or Windows | Install a verified archive from the [latest release] |

Releases include signed APK, DEB, and RPM packages for Linux 386, amd64, and
arm64, plus portable CGO-free archives for Linux, macOS, and Windows. See the
[installation guide] for repository setup, exact asset names, upgrades, and
checksum, signature, and attestation verification.

## Optional AI augmentation

AI mode uses the default profile in the strict per-user `diagnosis.yml`:

```console
jobman diagnose --ai JOB
jobman diagnose --ai-logs JOB
jobman diagnose config paths
jobman diagnose config validate
```

Use `--profile NAME` to select another configured model. Supported provider
boundaries are:

| Provider | Intended use |
| --- | --- |
| OpenAI-compatible Chat Completions | Hosted APIs, vLLM, and other strict-schema compatible servers |
| Ollama `/api/chat` | Local structured generation |
| Absolute command bridge | A bounded local adapter for another runtime |

Profiles fix the endpoint, model, locality, timeout, credentials by reference,
and allowed evidence classes. AI activation shares bounded metadata, command
arguments, paths, environment variable names—never values—and typed execution
context when the profile permits them. Log bytes remain a separate opt-in via
`--ai-logs` or `--share log_content`.

Provider responses are untrusted proposals. Jobman-Diagnose validates their
schema, taxonomy, citations, contradictions, actions, and request identity;
optional provider failure still returns the deterministic report. See the
[configuration guide], [generation protocol], and [security model].

## Evidence, offline use, and support

Evidence can be reviewed, transported, and diagnosed without a live Jobman
state store:

```console
jobman-diagnose --export-evidence evidence.json JOB
jobman-diagnose --from-evidence evidence.json
jobman-diagnose --from-evidence evidence.json --json --output report.json
```

Explicit exports use private permissions, atomic publication, and no-overwrite
semantics. Support bundles contain selected sealed evidence, reports,
disclosure, capabilities, and build metadata—never credentials, environment
values, database files, or Jobman's fingerprint key. Review logs and exported
evidence before sharing them; configured redaction cannot recognize every
possible secret.

## Documentation

| Topic | Resource |
| --- | --- |
| Installation and upgrades | [Installation guide][installation guide] |
| AI profiles and providers | [Configuration guide][configuration guide] |
| Common failures | [Troubleshooting guide] |
| Privacy and trust boundaries | [Security model][security model] |
| Stable machine output | [Report schema][report schema] |
| Model request and response contract | [Generation protocol][generation protocol] |
| Private diagnostic archives | [Support bundles] |
| Jobman version support | [Compatibility contract][compatibility contract] |
| Component boundaries | [Architecture] |
| Quality corpus and model evaluation | [Evaluation guide] |
| Release artifacts and verification | [Release guide] |

Use the [issue tracker] for reproducible bugs and feature proposals. Report
suspected vulnerabilities privately according to the [security policy].

## Development

Use the included devcontainer or a local Go installation:

```console
make setup
make quick-check
make check
```

`make help` lists development, evaluation, documentation, packaging, and
release targets. Production code has no provider SDK dependency, and tests use
copied evidence fixtures, local fake servers, and helper processes rather than
live models or credentials.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution requirements.

[architecture]: docs/ARCHITECTURE.md
[cloudsmith]: https://cloudsmith.io/~jobman/repos/stable/
[compatibility contract]: docs/COMPATIBILITY.md
[configuration guide]: docs/CONFIGURATION.md
[documentation]: docs/README.md
[evaluation guide]: docs/EVALUATION.md
[generation protocol]: docs/GENERATION_PROTOCOL.md
[installation guide]: docs/INSTALLATION.md
[issue tracker]: https://github.com/ryancswallace/jobman-diagnose/issues
[jobman]: https://github.com/ryancswallace/jobman
[latest release]: https://github.com/ryancswallace/jobman-diagnose/releases/latest
[release guide]: RELEASE.md
[report schema]: docs/REPORT_SCHEMA.md
[security model]: docs/SECURITY_MODEL.md
[security policy]: SECURITY.md
[support bundles]: docs/SUPPORT_BUNDLES.md
[troubleshooting guide]: docs/TROUBLESHOOTING.md
