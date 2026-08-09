# Release process

Jobman Diagnose has not published its first release. Releases are deliberately
maintainer-triggered semantic-version tags rather than automatic releases from
every merge. This leaves an explicit compatibility and live-provider
evaluation gate while the evidence and generation contracts mature.

## First-release prerequisites

Jobman v1.4.0 is the immutable core compatibility baseline: it publishes
diagnostic evidence schema 1, the module requires its tag directly, copied
fixtures record that origin and exact hashes, and continuous integration builds
without a sibling checkout.

Before creating the first companion tag:

1. Confirm `CHANGELOG.md`, `docs/COMPATIBILITY.md`, `SECURITY.md`, and
   `docs/INSTALLATION.md` describe the final supported versions and
   verification commands.
2. Confirm the GitHub Settings app has applied `.github/settings.yml`, private
   vulnerability reporting and secret scanning are enabled where available,
   and the `main` release environment requires maintainer approval.
3. Complete the candidate and live-provider validation below and retain its
   release evidence.

## Candidate validation

Run the complete local gate from a clean checkout using the exact Go toolchain:

```console
make setup
make check
make snapshot
```

The gate verifies module integrity, formatting, lint, Actions syntax, reachable
vulnerabilities, race-enabled coverage, the deterministic evaluation corpus,
documentation links, every supported architecture, GoReleaser configuration,
and release builds.

Also verify direct and `jobman diagnose` assembled invocation against the
minimum and newest supported Jobman versions. Follow
[`docs/EVALUATION.md`](docs/EVALUATION.md) for recorded-response evaluation and
opt-in live evaluation against at least one supported hosted strict-output
service and one supported local runtime. Retain exact model/runtime identifiers,
profile limits, and resulting JSON with the candidate evidence.

Exercise secret-canary, prompt-injection, refusal, timeout, malformed-output,
redirect, locality, and deterministic-fallback cases in the release build. A
clean deterministic installation must need no configuration, credentials,
Python, network, or provider runtime.

## Publish

Create a signed `vX.Y.Z` tag only after the candidate gates pass and push it to
GitHub. The protected Release workflow:

- rejects non-semantic tags, tags outside `main`, local Jobman replacements,
  and the unreleased `v0.0.0` placeholder;
- repeats source, security, documentation, evaluation, and release checks;
- creates CGO-free Linux, macOS, and Windows archives as a draft release;
- emits SHA-256 checksums and per-archive SBOMs;
- signs the checksum manifest keylessly with Cosign; and
- records GitHub build attestations for the checksum manifest.

Inspect the draft before publishing. Verify archive contents, an installation
on each operating-system family, checksums, the Sigstore bundle, SBOMs,
attestations, generated release notes, and the `jobman-diagnose version` output.
Publish only after those artifacts agree with the tag and commit.

Generated analysis remains explicit opt-in. Do not describe an adapter as
release-supported until its structured-output behavior, locality boundary,
failure behavior, and disclosure manifest pass the provider security and
compatibility gate.

## Post-release

Install the published archive in a clean environment and repeat one
deterministic and one configured provider smoke test. Confirm documentation
links and badges, then record any release-specific caveat in the changelog.
Never rebuild or replace artifacts under an existing tag; publish a new patch
version instead.
