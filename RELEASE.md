# Release process

Jobman Diagnose releases are deliberately maintainer-triggered semantic-version
tags rather than automatic releases from every merge. This preserves an
explicit compatibility and live-provider evaluation gate while the evidence
and generation contracts mature. The initial pre-v1 release is v0.1.0.

## Release prerequisites

Jobman v1.4.0 is the immutable core compatibility baseline: it publishes
diagnostic evidence schema 1, the module requires its tag directly, copied
fixtures record that origin and exact hashes, and continuous integration builds
without a sibling checkout.

Before creating a companion tag:

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
attestations, generated release notes, and the `jobman-diagnose --version`
output.
Publish only after those artifacts agree with the tag and commit.

Generated analysis remains explicit opt-in. Do not describe an adapter as
release-supported until its structured-output behavior, locality boundary,
failure behavior, and disclosure manifest pass the provider security and
compatibility gate.

## Verify published artifacts

Download the archive, checksum manifest, and Sigstore bundle from the same
release. For v0.1.0 on Apple silicon, for example:

```console
gh release download v0.1.0 \
  --repo ryancswallace/Jobman-Diagnose \
  --pattern 'jobman-diagnose_0.1.0_darwin_arm64.tar.gz' \
  --pattern 'jobman-diagnose_0.1.0_checksums.txt' \
  --pattern 'jobman-diagnose_0.1.0_checksums.txt.sigstore.json'
cosign verify-blob \
  --bundle jobman-diagnose_0.1.0_checksums.txt.sigstore.json \
  --certificate-identity \
    'https://github.com/ryancswallace/Jobman-Diagnose/.github/workflows/release.yml@refs/tags/v0.1.0' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  jobman-diagnose_0.1.0_checksums.txt
grep '  jobman-diagnose_0.1.0_darwin_arm64.tar.gz$' \
  jobman-diagnose_0.1.0_checksums.txt | shasum -a 256 -c -
gh attestation verify \
  jobman-diagnose_0.1.0_darwin_arm64.tar.gz \
  --repo ryancswallace/Jobman-Diagnose
```

Use `sha256sum -c -` instead of `shasum -a 256 -c -` on systems that provide
the GNU checksum tool. Verification must use the canonical, case-sensitive repository
identity shown above. Substitute another archive name from the installation
guide when testing a different platform.

## Post-release

Install the published archive in a clean environment and repeat one
deterministic and one configured provider smoke test. Confirm documentation
links and badges, then record any release-specific caveat in the changelog.
Never rebuild or replace artifacts under an existing tag; publish a new patch
version instead.
