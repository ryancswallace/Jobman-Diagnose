# AGENTS.md

This file defines repository-wide guidance for coding agents working on
Jobman Diagnose. More specific `AGENTS.md` files override it within their
directories.

## Project boundaries

Jobman Diagnose is an optional Go companion to Jobman. Core Jobman owns durable
execution, evidence collection, redaction, and the versioned diagnostic
evidence contract. This repository owns deterministic interpretation,
provider-neutral generation requests, untrusted proposal validation, report
presentation, and support bundles.

Keep Jobman lightweight and model-agnostic. Do not add model SDKs, prompts, or
provider credentials to core. Keep this companion read-only: it may suggest
actions, but it must not mutate, retry, signal, or repair jobs.

The development module uses a sibling Jobman checkout until evidence schema 1
has a tagged release. Preserve that coordinated layout, update all pinned core
workflow revisions together, and push the compatible core revision before a
companion commit that selects it.

## Start safely

1. Read relevant code, tests, documentation, and configuration before editing.
2. Check `git status` and preserve unrelated or developer-local changes.
3. Never read or commit ignored diagnosis configuration, credentials, logs, or
   evidence from a developer's machine.
4. Prefer the smallest coherent change and do not edit generated output
   directly.
5. Do not commit, push, publish, tag, or change hosted settings unless the user
   explicitly requests it.

## Implementation rules

- Use the exact Go patch in `go.version`; `go.mod` records only the compatible
  language major/minor version.
- Run `make format`; do not substitute another formatter.
- Keep interfaces narrow and close to their consumers. Providers implement one
  bounded structured-generation operation rather than a general chat API.
- Pass `context.Context` through external calls and subprocesses. Bound bytes,
  time, goroutines, child processes, and shutdown paths.
- Treat core evidence, logs, configuration labels, and every provider response
  as untrusted. Never concatenate evidence into system instructions.
- Validate generated JSON against the exact schema and semantic invariants,
  including request identity, controlled taxonomy, citations, contradictions,
  and allowlisted action identifiers.
- Preserve deterministic findings, actions, retry advice, and fallback behavior
  when optional generation fails.
- Keep stable JSON separate from human output. Progress belongs on stderr and
  must remain silent automatically for JSON and non-interactive output.
- Errors must be lowercase, wrapped with `%w` when useful, and must not expose
  credentials, response bodies, command stderr, evidence values, or generated
  content.
- Use private, atomic, no-overwrite writes for evidence, reports, support
  bundles, and other potentially sensitive artifacts.

## Tests and documentation

Keep unit tests beside implementations and make them deterministic under
parallel execution, shuffling, and the race detector. Use local loopback test
servers only; never call a live model or the public network from unit tests.
Provider changes need protocol, failure, size, locality, and secret-canary
coverage. Public schemas require compatibility fixtures and additive/breaking
versioning analysis.

Update CLI help, `README.md`, `docs/`, and `CHANGELOG.md` for user-visible
changes. Update `docs/SECURITY_MODEL.md` whenever trust or disclosure boundaries
change. Do not claim an adapter is release-supported until recorded and live
evaluation gates pass.

Useful checks are:

```sh
make setup
make quick-check
make docs
make check
```

Before handoff, run `make check` when the environment permits. Report the exact
command and unverified scope when a gate cannot run.
