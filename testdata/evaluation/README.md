# Diagnosis evaluation corpus

`manifest.json` is the versioned, synthetic, nonsecret regression corpus used
by `make evaluate`. Corpus schema 4 records deterministic diagnosis, action,
retry, and confidence expectations plus semantic generated-output behavior.
Generated causes may be required, allowed, or required to abstain. Required
facts use wording alternatives; required relations retain both sides of a
causal link; forbidden claims catch plausible hallucinations; citation ceilings
reward evidence economy.

The 60 cases combine immutable, checksummed Jobman evidence under
`testdata/jobman-v1/` with generated evidence under `evidence/`. They cover the
Python failure lab, shell, Go, Node.js, C/native, JVM, and Rust diagnostics;
tracebacks, exception chains and groups, panics, compilers, linkers, HTTP,
network, database, filesystem, dependency, resource, timeout, and signal
failures; and ambiguous, truncated, and prompt-injection controls.

Corpus schema 4 can associate a case with a checked-in repository-relative
source file. The 28 current `context.source` cases map to the Python and
multi-ecosystem failure labs. A live evaluation whose selected profile enables
`source_context` attaches those snapshots while continuing to execute all 60
cases; evidence-only cases receive no fabricated source.

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

The development evaluator atomically replaces existing `--output` and
`--capture-proposals` files with private permissions, allowing stable filenames
across repeated experiments. This replacement behavior is limited to evaluation
artifacts; production evidence, reports, and support bundles remain
no-overwrite.
