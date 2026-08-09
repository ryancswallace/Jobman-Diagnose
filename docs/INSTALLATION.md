# Installation

Jobman Diagnose v0.1.0 is the initial pre-v1 release. Install Jobman v1.4.0 or
newer first, then install the companion on the same `PATH`.

## Release archive

The v0.1.0 release provides CGO-free archives for Linux, macOS, and Windows on
amd64 and arm64, plus Linux and Windows 386:

| Platform | Archive |
| --- | --- |
| Linux amd64 | `jobman-diagnose_0.1.0_linux_amd64.tar.gz` |
| Linux arm64 | `jobman-diagnose_0.1.0_linux_arm64.tar.gz` |
| Linux 386 | `jobman-diagnose_0.1.0_linux_386.tar.gz` |
| macOS Intel | `jobman-diagnose_0.1.0_darwin_amd64.tar.gz` |
| macOS Apple silicon | `jobman-diagnose_0.1.0_darwin_arm64.tar.gz` |
| Windows amd64 | `jobman-diagnose_0.1.0_windows_amd64.zip` |
| Windows arm64 | `jobman-diagnose_0.1.0_windows_arm64.zip` |
| Windows 386 | `jobman-diagnose_0.1.0_windows_386.zip` |

Download the archive, checksum manifest, and Sigstore bundle from the [v0.1.0
release]. This example selects the Apple-silicon archive; substitute the asset
name from the table for another platform:

```console
gh release download v0.1.0 \
  --repo ryancswallace/Jobman-Diagnose \
  --pattern 'jobman-diagnose_0.1.0_darwin_arm64.tar.gz' \
  --pattern 'jobman-diagnose_0.1.0_checksums.txt' \
  --pattern 'jobman-diagnose_0.1.0_checksums.txt.sigstore.json'
```

Verify the keyless signature on the checksum manifest, then the selected
archive's checksum and GitHub build attestation:

```console
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

Use `sha256sum -c -` instead of `shasum -a 256 -c -` where the GNU checksum
tool is installed. On Windows, `gh attestation verify` provides the artifact digest and
provenance check; compare that digest with `Get-FileHash -Algorithm SHA256` and
the checksum manifest. The repository identity in the Cosign command is
case-sensitive.

Extract and install the executable:

```console
tar -xzf jobman-diagnose_0.1.0_darwin_arm64.tar.gz
install -m 0755 jobman-diagnose "$HOME/.local/bin/jobman-diagnose"
jobman-diagnose --version
```

Windows users can expand the `.zip` archive and copy `jobman-diagnose.exe` to a
directory on `PATH`.

[v0.1.0 release]: https://github.com/ryancswallace/Jobman-Diagnose/releases/tag/v0.1.0

## Build from source

The Go dependency resolves from the tagged Jobman module and does not require a
sibling checkout:

```console
git clone https://github.com/ryancswallace/jobman-diagnose.git
cd jobman-diagnose
git checkout v0.1.0
make setup
make check
install -m 0755 bin/jobman-diagnose "$HOME/.local/bin/jobman-diagnose"
```

The exact Go version is recorded in `go.version`. `make setup` rejects a
different active toolchain and installs pinned development tools into `bin/`.
The [devcontainer](../.devcontainer/README.md) provides the same toolchain in a
reproducible Linux environment.

Put `jobman-diagnose` beside `jobman` on `PATH`. Jobman discovers the external
command and makes `jobman diagnose JOB` equivalent to direct companion
invocation.

Each release also includes per-archive SBOMs. Checksums and attestations bind
the archive bytes to the tagged source; they do not make an untrusted build or
model runtime safe.

Deterministic diagnosis needs no configuration, network, credentials, Python,
or model runtime. AI augmentation is optional and configured separately; see
[configuration](CONFIGURATION.md).
