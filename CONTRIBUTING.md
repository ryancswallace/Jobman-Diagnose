# Contributing

Thanks for helping improve Jobman Diagnose. Bug fixes, tests, provider
conformance work, deterministic analyzers, documentation, and evaluation cases
are welcome. Discuss large schema, trust-boundary, or provider-architecture
changes in an issue before investing in an implementation; focused fixes can go
directly to a pull request.

By participating, you agree to follow the [code of conduct](CODE_OF_CONDUCT.md).
Report vulnerabilities through [SECURITY.md](SECURITY.md), not a public issue.

## Development setup

Jobman Diagnose uses the tagged Jobman module recorded in `go.mod`; a sibling
Jobman checkout is not required:

```console
git clone https://github.com/ryancswallace/jobman-diagnose.git
cd jobman-diagnose
make setup
make quick-check
```

Use the exact Go patch recorded in `go.version`. The devcontainer is the
supported reproducible environment; a local toolchain is equally welcome when
it uses the same version. Pinned repository tools are installed under `bin/`.

## Verification

Run focused tests while iterating, then the complete gate before submission:

```console
make format
make check
```

Useful narrower targets include:

- `make test` for race-enabled unit and compatibility tests;
- `make coverage-check` for the aggregate coverage floor;
- `make evaluate` for the deterministic diagnosis corpus;
- `make fuzz FUZZ_PACKAGE=... FUZZ_TARGET=...` for one bounded decoder fuzz
  target;
- `make docs` for repository documentation and relative links;
- `make workflow-check` for GitHub Actions;
- `make release-build` for every declared release target; and
- `make snapshot` for complete local archives, checksums, and SBOMs without
  publishing or signing.

Live provider evaluation is opt-in and must use an explicit local
configuration. Never put credentials, private evidence, or model output into a
fixture. Follow [the evaluation guide](docs/EVALUATION.md) and retain exact
runtime/model identifiers for release evidence.

The standard root-level evidence, report, support-bundle, and diagnosis-config
filenames are ignored by Git and Docker. That is a guardrail, not a substitute
for reviewing `git status` before every commit.

## Compatibility and security

Core evidence, report, configuration, generation request, proposal, and support
bundle schemas are independent public contracts. Prefer additive changes within
a schema version; breaking semantics require a new version and fixtures for the
oldest supported input.

All evidence and provider output is untrusted. Generated content may add only
validated, uncalibrated hypotheses and reorder existing action identifiers. It
must not change deterministic facts, retry policy, commands, or job state.
Provider errors must remain bounded and secret-safe.

When the minimum compatible Jobman release changes, update `go.mod`, the copied
fixture origin, compatibility documentation, and changelog together. Never use
a branch, pseudo-version, or local replacement as a published dependency.

## Commits and pull requests

Use [Conventional Commits](https://www.conventionalcommits.org/) with prefixes
such as `fix:`, `feat:`, `docs:`, `test:`, `ci:`, or `chore:`. Keep pull
requests focused. Explain the problem, chosen approach, compatibility/security
impact, and verification performed. Update user documentation and
`CHANGELOG.md` for notable behavior.

Contributions are accepted under the [MIT License](LICENSE).
