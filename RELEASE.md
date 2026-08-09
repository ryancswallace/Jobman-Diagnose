# Release checklist

The project is not yet released. Before the first tag:

1. require the first tagged Jobman module containing evidence schema 1 and
   remove the local `replace` directive;
2. replace copied fixture origin `unreleased` with that Jobman tag and verify
   every SHA-256;
3. run formatting, lint, race tests, vulnerability analysis, and native and
   cross-platform builds;
4. verify a direct and `jobman diagnose` assembled invocation against the
   minimum and newest supported Jobman versions;
5. run `make release-check release-build` and build reproducible CGO-free
   archives for the supported operating systems and architectures using the
   checked-in GoReleaser configuration;
6. push a `vX.Y.Z` tag only after the preceding gates pass; the Release
   workflow revalidates the source, builds a draft with GoReleaser, emits
   archive SBOMs and checksums, signs the checksum manifest keylessly with
   Cosign, and records GitHub build attestations;
7. verify a clean installation needs no config, credentials, Python, network,
   or provider runtime; and
8. update `CHANGELOG.md`, `docs/COMPATIBILITY.md`, and `SECURITY.md` before
   publishing;
9. run recorded-response evaluation and adapter conformance for every shipped
   provider, then perform opt-in live evaluation against at least one supported
   hosted strict-output service and one supported local runtime; and
10. verify secret-canary, prompt-injection, refusal, timeout, malformed-output,
    redirect, locality, and deterministic-fallback cases in the release build.

The release workflow deliberately fails while `go.mod` contains the sibling
Jobman `replace` directive. After the first compatible Jobman tag exists,
replace the placeholder requirement with that tag, remove the directive, copy
and identify released compatibility fixtures, and rerun `make check` before
tagging. Releases are drafts so a maintainer can verify archives, SBOMs, the
checksum Sigstore bundle, and attestations before publication.

Run the checked-in corpus for every candidate:

```console
make evaluate
```

Then follow [`docs/EVALUATION.md`](docs/EVALUATION.md) for one explicitly
configured hosted provider and one self-hosted provider. Retain the resulting
JSON, exact model/runtime identifiers, and profile limits with candidate
evidence.

Generated analysis remains explicit opt-in. Do not describe an adapter as
release-supported until its structured-output behavior, locality boundary,
failure behavior, and disclosure manifest have passed the provider security
and compatibility gate. A clean deterministic installation remains the minimum
supported configuration.
