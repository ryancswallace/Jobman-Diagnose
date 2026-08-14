# Diagnosis evaluation corpus

`manifest.json` is the versioned, synthetic, nonsecret regression corpus used
by `make evaluate`. Corpus schema 4 records deterministic diagnosis, action,
retry, and confidence expectations plus semantic generated-output behavior.
Generated causes may be required, allowed, or required to abstain. Required
facts use wording alternatives; required relations retain both sides of a
causal link; forbidden claims catch plausible hallucinations; citation ceilings
reward evidence economy.

The 72 cases combine immutable, checksummed Jobman evidence under
`testdata/jobman-v1/` with generated evidence under `evidence/`. They cover the
Python failure lab, shell, Go, Node.js, C/native, JVM, and Rust diagnostics;
tracebacks, exception chains and groups, panics, compilers, linkers, HTTP,
network, database, filesystem, dependency, resource, timeout, and signal
failures; and ambiguous, truncated, and prompt-injection controls.

Corpus schema 4 can associate a case with a checked-in repository-relative
source file. All 29 current source mappings are tagged `context.source` and
cover the Python and multi-ecosystem failure labs. A live evaluation whose
selected profile enables
`source_context` attaches those snapshots while continuing to execute all 72
cases; evidence-only cases receive no fabricated source.
The `context.stale_source` control must produce a source-context mismatch when
source collection is enabled, and evaluation-result schema 6 records both the
per-case detection and the aggregate mismatch count. The mismatched source must
not appear in provider disclosure.

Tags allow selection by language, failure class, diagnostic format, lifecycle,
source, or control role. Add or revise a case whenever a taxonomy, rule, action,
retry policy, prompt, validator, or evidence projection changes.

Synthetic fixtures and the manifest are generated together:

```console
make gen-evaluation-fixtures
make evaluation-fixtures-check
```

The deterministic corpus runs in ordinary CI. Live model evaluation is
explicit because it can disclose the selected fixture projection, consume
provider capacity, and vary with model/runtime releases. Raw proposal captures
are private diagnostic artifacts and must not be committed. Do not add
developer logs, credentials, or production job data. Add a sanitized
real-world case only when its provenance and redistribution are documented and
all identifying or secret material has been removed.

`promotion-policy.json` is the versioned release gate for the complete corpus.
It requires three live executions of every case without provider fallback,
minimum workload and tag coverage, and explicit quality thresholds. Use
`make evaluate-release` with `EVALUATION_DIAGNOSIS_CONFIG` and
`EVALUATION_PROFILE`; a filtered or deterministic run cannot satisfy the
policy.

The development evaluator atomically replaces existing `--output` and
`--capture-proposals` files with private permissions, allowing stable filenames
across repeated experiments. This replacement behavior is limited to evaluation
artifacts; production evidence, reports, and support bundles remain
no-overwrite.
