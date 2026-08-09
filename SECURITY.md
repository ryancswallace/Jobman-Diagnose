# Security policy

## Supported versions

Jobman Diagnose has not published its first release. Security fixes currently
land on `main`; source builds from arbitrary historical commits are not
supported. After the first release, this table will identify the maintained
release lines explicitly.

| Version | Supported |
| --- | --- |
| `main` before v1 | Yes |

## Reporting a vulnerability

Please report suspected vulnerabilities privately through a
[GitHub security advisory]. Do not include credentials, raw private logs, or
other unrelated personal data in a report.

Include the affected version or commit, platform, impact, and the smallest safe
reproduction you can provide. You should receive an acknowledgement within
seven days. The maintainer will coordinate validation, a fix, disclosure, and
credit with you; response and release timing depends on severity and
complexity.

Do not open a public issue until a coordinated disclosure is complete. If a
report needs sensitive evidence, first describe what you have so a safe
transfer method can be agreed upon.

## Security model

`jobman-diagnose` treats Jobman logs and generated model output as untrusted
data. Installing the executable still grants it the authority of the invoking
user, so releases should be verified before installation. No evidence is sent
to a network service in deterministic mode.

Generated augmentation is disabled unless the user activates AI mode or names
a profile. AI mode selects the configured default profile and approves bounded
metadata plus typed command, path, environment-name, and policy context when
the profile permits those classes. Log content remains a separate per-command
opt-in. The available adapters use bounded schema-constrained requests;
generated output remains an untrusted proposal and cannot control facts, retry
policy, commands, or job state. Credentials are references to environment
values or private files and are not representable as literals in configuration.

Evidence and reports are bounded and digest-verified; explicit file exports
use private permissions and never overwrite an existing destination. Target
logs may nevertheless contain unknown secrets, so review bundles before
collecting or sharing log content. Local runtimes and command bridges execute
with the invoking user's authority.

Support bundles are private deterministic archives of already selected
evidence and report metadata. Preview the inventory and review `evidence.json`
before sharing; command arguments, paths, or log tails may be present when the
diagnosis explicitly collected them.

The detailed threat boundaries and generated-analysis controls are documented
in [`docs/SECURITY_MODEL.md`](docs/SECURITY_MODEL.md).

[GitHub security advisory]: https://github.com/ryancswallace/jobman-diagnose/security/advisories/new
