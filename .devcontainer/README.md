# Devcontainer

The devcontainer provides the exact Go toolchain, pinned Go editor and
debugging tools, a pinned GitHub CLI, and the repository tools installed by
`make setup`. It supports both amd64 and arm64 hosts.

Clone Jobman and Jobman Diagnose as sibling directories before opening this
repository in the container. The configuration mounts only the sibling
`jobman` checkout at `/workspaces/jobman`, where the development-only module
replacement can resolve it. Container creation fails clearly if that sibling
directory does not exist; it never clones or changes core implicitly.

Treat the container as a trusted development environment. Keep credentials in
the editor's secret facility, Codespaces secrets, or an ignored
`.devcontainer/.env.local`; never add them to the shared configuration.
