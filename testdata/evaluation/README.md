# Diagnosis evaluation corpus

`manifest.json` is the versioned, synthetic, nonsecret regression corpus used
by `make evaluate`. It references copied and checksummed Jobman evidence under
`testdata/jobman-v1/` and records expected diagnosis, action, retry, confidence,
and generated-claim behavior.

The deterministic corpus is part of ordinary tests. Live model evaluation is
explicit and separate because it can disclose the selected fixture projection,
consume provider capacity, and vary with a model/runtime release. Add or revise
a case whenever a taxonomy, rule, action, retry policy, prompt, or evidence
projection changes. Do not add developer logs, credentials, or production job
data.
