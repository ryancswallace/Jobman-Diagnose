# Installation

Jobman Diagnose has not published its first release. Until then, build it from
a coordinated pair of sibling checkouts so the development-only Jobman module
replacement resolves:

```console
git clone https://github.com/ryancswallace/jobman.git
git clone https://github.com/ryancswallace/jobman-diagnose.git
cd jobman-diagnose
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

## Release artifacts

Tagged releases will provide CGO-free archives for Linux, macOS, and Windows on
amd64 and arm64, plus Linux and Windows 386. Each release is designed to include
a SHA-256 manifest, a keyless Sigstore bundle for that manifest, per-archive
SBOMs, and GitHub build attestations.

Before using a future release, compare the archive digest with the checksum
manifest and verify the repository identity recorded by the Sigstore bundle or
GitHub attestation. Exact commands will be added here after the first release
workflow has produced and verified its final artifact names.

Deterministic diagnosis needs no configuration, network, credentials, Python,
or model runtime. AI augmentation is optional and configured separately; see
[configuration](CONFIGURATION.md).
