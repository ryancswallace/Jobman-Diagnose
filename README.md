<!-- markdownlint-disable MD041 -->
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

Jobman-Diagnose helps you understand why a [Jobman] job failed. It points to
the relevant evidence, suggests what to try next, and tells you whether a retry
is likely to help.

It works locally with no setup beyond installation. You do not need an AI
model, network access, credentials, or Python. If you choose to connect an AI
provider, Jobman-Diagnose can add suggestions while keeping its built-in
findings and retry advice in charge.

<!-- Terminal demo slot. Generate the recording from
docs/screencaps/tape/diagnose.tape, then replace this comment with:
<p align="center">
  <img src="docs/screencaps/gif/diagnose.gif"
       alt="A Jobman-Diagnose terminal demo"
       width="900">
</p>
-->

> [!TIP]
> Start with `jobman diagnose JOB`. No AI setup is required.

## Quick start

Install Jobman v1.4.0 or newer and Jobman-Diagnose, then run:

```console
jobman diagnose JOB
```

Replace `JOB` with a Jobman job ID or name. The report shows what happened,
the evidence behind that conclusion, suggested next steps, and retry advice.
If you get stuck, see the [troubleshooting guide].

## Features

| Capability | Jobman-Diagnose provides... |
| --- | --- |
| Clear explanations | Likely causes, next steps, and retry recommendations |
| Evidence you can check | Each factual finding points back to Jobman's diagnostic data |
| Useful local defaults | Repeatable analysis without a network connection or AI model |
| Optional AI suggestions | Extra suggestions from a configured local or hosted provider |
| Control over shared data | Logs and source code are shared only when you explicitly allow them |
| Reports you can save | JSON output, offline diagnosis, and support bundles |
| Read-only operation | Advice only: it never changes, retries, signals, or repairs a job |

> [!NOTE]
> Jobman-Diagnose is currently pre-v1. Check the [compatibility guide] when
> choosing Jobman and Jobman-Diagnose versions. AI suggestions are advisory;
> the built-in findings, actions, and retry advice remain authoritative.

## Command overview

| Task | Command |
| --- | --- |
| Diagnose locally | `jobman diagnose JOB` |
| Add AI suggestions | `jobman diagnose --ai JOB` |
| Let AI use relevant log excerpts | `jobman diagnose --ai-logs JOB` |
| Show more report detail | `jobman diagnose --details JOB` |
| Produce JSON | `jobman diagnose --json JOB` |
| Save a support bundle | `jobman diagnose --support-bundle FILE JOB` |

Install both binaries on `PATH` to use the `jobman diagnose` form. You can also
run the companion directly:

```console
jobman-diagnose --jobman /absolute/path/to/jobman JOB
```

For configuration inspection, offline diagnosis, evidence export, source
sharing, and other advanced commands, see the [configuration guide] and the
CLI help. Programs that consume JSON output should follow the [report schema].

## Installation

Install Jobman v1.4.0 or newer first, then choose an option for your system:

| Environment | Recommended installation |
| --- | --- |
| macOS | `brew install ryancswallace/tap/jobman-diagnose` |
| Debian or Ubuntu | Configure [Cloudsmith], then `sudo apt install jobman jobman-diagnose` |
| Fedora, RHEL, Rocky, AlmaLinux, Amazon Linux | Configure [Cloudsmith], then `sudo dnf install jobman jobman-diagnose` |
| Alpine Linux | Configure [Cloudsmith], then `sudo apk add jobman jobman-diagnose` |
| Other Linux or Windows | Install a verified archive from the [latest release] |

The [installation guide] has step-by-step package setup, archive names,
upgrade instructions, and release verification.

## Optional AI suggestions

The default local diagnosis is usually the best place to start. To add AI
suggestions, configure a provider profile and run:

```console
jobman diagnose --ai JOB
jobman diagnose --ai-logs JOB
jobman diagnose config paths
jobman diagnose config validate
jobman diagnose doctor
```

Use `--profile NAME` to select a different profile. `doctor` checks that the
selected provider and model can return the format Jobman-Diagnose expects.

AI use is always explicit. Logs are shared only with `--ai-logs`, and source
code requires a separate source-sharing option. Source code is not redacted,
so review it before sharing it with a provider. If the provider fails or its
suggestions do not pass validation, Jobman-Diagnose still returns its local
report.

The [configuration guide] explains profiles, supported providers, log and
source sharing, limits, and the `doctor` command. Read the [security model] for
the exact privacy and validation rules, or the [generation protocol] if you are
building a provider integration.

## Offline use and support

You can export a job's diagnostic data and inspect it later or on another
machine:

```console
jobman-diagnose --export-evidence evidence.json JOB
jobman-diagnose --from-evidence evidence.json
jobman-diagnose --from-evidence evidence.json --json --output report.json
```

You can export diagnostic evidence on one machine and inspect it on another.
You can also create a support bundle when a maintainer needs more context.
These files can contain sensitive job details, so review them before sharing.
See [support bundles] for the bundle contents and safety guidance, and the
[security model] for file-handling details.

## Documentation

| Topic | Resource |
| --- | --- |
| Installation and upgrades | [Installation guide][installation guide] |
| AI profiles and providers | [Configuration guide][configuration guide] |
| Common failures | [Troubleshooting guide] |
| Privacy and trust boundaries | [Security model][security model] |
| How JSON reports are structured | [Report schema][report schema] |
| How AI requests and responses work | [Generation protocol][generation protocol] |
| Creating and safely sharing support bundles | [Support bundles] |
| Jobman version support | [Compatibility guide][compatibility guide] |
| How the parts of Jobman-Diagnose fit together | [Architecture] |
| Testing diagnosis quality and AI models | [Evaluation guide] |
| Examples of failures in several languages | [Failure labs] |
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
release targets.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution requirements.

[architecture]: docs/ARCHITECTURE.md
[cloudsmith]: https://cloudsmith.io/~jobman/repos/stable/
[compatibility guide]: docs/COMPATIBILITY.md
[configuration guide]: docs/CONFIGURATION.md
[evaluation guide]: docs/EVALUATION.md
[failure labs]: examples/failure-labs/README.md
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
