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
4. Confirm the `main` environment contains `HOMEBREW_TAP_TOKEN` and
   `CLOUDSMITH_API_KEY`, and that the reusable `v*.*.*` environment tag policy
   is present. See the one-time distribution setup below.

## One-time distribution setup

### Homebrew tap

Create `HOMEBREW_TAP_TOKEN` as a fine-grained personal access token whose
resource owner is `ryancswallace`. Grant it access only to
`ryancswallace/homebrew-tap`, with **Contents: read and write** and **Pull
requests: read and write**. Store it as an environment secret named
`HOMEBREW_TAP_TOKEN` in this repository's protected `main` environment.

The publishing workflow checks the token's repository access before cloning
the tap, pushes an automation branch, opens a pull request, and requests
auto-merge. Protected tap `main` requires strict online audits and installation
tests for both formulas on Intel and Apple Silicon. Rotate
the secret before the token expires and immediately after suspected exposure.
Never put the token in repository, organization, or local configuration files.

### Cloudsmith

Keep `jobman/stable` public and classified as an open-source repository. Create
or reuse a narrowly scoped Cloudsmith service account/API key that can read and
upload packages only in `jobman/stable`; it does not need workspace-management
permissions. Store the key as an environment secret named
`CLOUDSMITH_API_KEY` in this repository's protected `main` environment.

The workflow grants no GitHub OIDC permission because the free Cloudsmith plan
does not support that authentication path. The pinned Cloudsmith action receives
the API key, verifies authentication, and installs pinned CLI version 1.21.0.
The publication helper never prints the key.

Both distribution jobs run only for a public, non-prerelease `vMAJOR.MINOR.PATCH`
release, use the protected environment, and can also be dispatched manually
from `main` to repair an interrupted publication.

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

The `Jobman compatibility` workflow verifies direct and `jobman diagnose`
assembled invocation against the minimum supported Jobman release and current
Jobman `main`. Before a release, confirm both lanes pass, then follow
[`docs/EVALUATION.md`](docs/EVALUATION.md) for recorded-response evaluation and
opt-in live evaluation against at least one supported hosted strict-output
service and one supported local runtime. Retain exact model/runtime identifiers,
profile limits, and resulting JSON with the candidate evidence.

Exercise secret-canary, prompt-injection, refusal, timeout, malformed-output,
redirect, locality, and deterministic-fallback cases in the release build. A
clean deterministic installation must need no configuration, credentials,
Python, network, or provider runtime.

## Publish

Create an annotated `vX.Y.Z` tag only after the candidate gates pass, sign it
when a configured signing identity is available, and push it to GitHub. GitHub
rules prevent release-tag updates or deletion; artifact identity is established
independently by keyless workflow signatures and attestations. The v0.1.0 tag
predates this policy and is an unsigned annotated tag. The protected Release workflow:

- rejects non-semantic tags, tags outside `main`, local Jobman replacements,
  and the unreleased `v0.0.0` placeholder;
- repeats source, security, documentation, evaluation, and release checks;
- creates CGO-free Linux, macOS, and Windows archives as a draft release;
- creates `.deb`, `.rpm`, and `.apk` packages for Linux 386, amd64, and arm64;
- emits SHA-256 checksums and per-archive and per-package SPDX SBOMs;
- signs the checksum manifest keylessly with Cosign; and
- records GitHub build attestations for every archive, native package, and
  SBOM in the checksum manifest.

Inspect the draft before publishing. Verify archive contents, an installation
on each operating-system family, checksums, the Sigstore bundle, SBOMs,
attestations, generated release notes, and the `jobman-diagnose --version`
output.
Publish only after those artifacts agree with the tag and commit.

Publishing a stable GitHub release triggers two independent distribution
workflows. The Homebrew workflow regenerates `Formula/jobman-diagnose.rb` from
the public checksum manifest and proposes it through a required-check pull
request in `ryancswallace/homebrew-tap`.
The Cloudsmith workflow downloads all nine native packages, verifies the
release's keyless checksum signature, checks every package checksum and GitHub
attestation, and uploads them to `jobman/stable`.

Cloudsmith can re-sign RPM bytes. Each upload therefore carries an immutable
`source-sha256-DIGEST` tag containing the original GitHub release digest. A
repair run accepts an existing filename only when that source-digest tag
matches, and refuses ambiguous or conflicting records.

If either distribution job fails, fix only its credential or hosted
configuration and dispatch **Publish Homebrew formula** or **Publish Linux
packages** from `main` with the existing stable tag. These workflows consume
the already-public release and never rebuild or replace its artifacts.

The v0.1.0 GitHub release predates native packages and remains archive-only.
Version v0.2.0 is the first release with native packages and automated
Cloudsmith publication. Never rebuild native packages under the v0.1.0 tag.

Generated analysis remains explicit opt-in. Do not describe an adapter as
release-supported until its structured-output behavior, locality boundary,
failure behavior, and disclosure manifest pass the provider security and
compatibility gate.

## Verify published artifacts

Download the archive or native package, checksum manifest, and Sigstore bundle
from the same release. For v0.2.0 on Apple silicon, for example:

```console
gh release download v0.2.0 \
  --repo ryancswallace/Jobman-Diagnose \
  --pattern 'jobman-diagnose_0.1.0_darwin_arm64.tar.gz' \
  --pattern 'jobman-diagnose_0.1.0_checksums.txt' \
  --pattern 'jobman-diagnose_0.1.0_checksums.txt.sigstore.json'
cosign verify-blob \
  --bundle jobman-diagnose_0.1.0_checksums.txt.sigstore.json \
  --certificate-identity \
    'https://github.com/ryancswallace/Jobman-Diagnose/.github/workflows/release.yml@refs/tags/v0.2.0' \
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
links and badges. Beginning with v0.2.0, also confirm the Homebrew formula
commit, install through Homebrew on macOS, install at least one Cloudsmith
package through each relevant repository format, and verify that all nine
Cloudsmith records carry their expected source-digest tag. Then record any
release-specific caveat in the changelog.
Never rebuild or replace artifacts under an existing tag; publish a new patch
version instead.
