# Maintenance and support policy

Jobman Diagnose v0.1.0 is the initial pre-v1 release. The newest published
pre-v1 minor line is the normal maintenance target; support for unreleased
`main` snapshots remains best effort. Patch releases may contain compatible correctness,
security, documentation, and provider-conformance fixes. Breaking schema or CLI
changes require a new major version and migration guidance.

Issues and pull requests are handled on a best-effort basis. Ordinary bug
reports should include:

- Jobman Diagnose and Jobman versions or commits;
- operating system and architecture;
- invocation with credentials and private paths removed;
- profile provider, locality, model identifier, and limits without credentials;
- expected and actual behavior; and
- safe error classifications or a minimized, redacted evidence fixture.

Do not post private logs, credentials, complete support bundles, provider
response bodies, or sensitive command arguments. Security reports use the
private process in [SECURITY.md](SECURITY.md).

Deterministic diagnosis is the minimum supported configuration. Provider
adapters are supported only when their structured-output, locality, failure,
and disclosure behavior has passed the release evaluation documented in
[docs/EVALUATION.md](docs/EVALUATION.md).
