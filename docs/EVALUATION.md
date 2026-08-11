# Diagnosis evaluation

The checked-in schema-2 corpus under `testdata/evaluation/` measures diagnosis
quality independently from wording. Every case names accepted and forbidden
findings/actions, retry state, confidence bounds, and allowed generated codes.
Inputs are synthetic and nonsecret. They combine immutable checksummed Jobman
compatibility fixtures with reproducibly generated multi-ecosystem target-log
cases and adversarial noise.

Log-backed cases may additionally require a generated cause and define groups
of incident concepts. A result must contain at least one alternative from each
group, such as both `connection refused` and port `5432`. This accepts natural
wording variation while rejecting a fluent answer that says only that a
traceback, invalid input, or nonzero exit occurred.

Run deterministic evaluation with no network or configuration:

```console
make evaluate
```

The JSON result reports each violation plus these separate metrics:

- primary-code precision;
- unsupported generated-claim rate;
- citation/provenance validity;
- safe-action rate;
- retry-advice accuracy;
- deterministic fingerprint stability; and
- provider-fallback rate; and
- generated specificity for provider-invoked cases with required cause
  concepts.

The JSON result includes `generated_specificity_cases` as that metric's
denominator. The compact deterministic summary prints specificity as `n/a`
because it deliberately invokes no provider.

Live evaluation is manual and explicit because it may disclose the approved
fixture projection and consume provider capacity:

```console
go run ./devel/evaluate \
  --live \
  --diagnosis-config /absolute/path/diagnosis.yml \
  --profile local-vllm \
  --share metadata,log_content \
  --output evaluation-local-vllm.json
```

Live mode requires an explicit configuration path and requires the provider by
default. Use `--allow-fallback` only when measuring fallback behavior. Add
`log_content` to `--share` only for corpus evidence that already contains a
sanitized log and the required value-redaction capability. No developer or
production logs belong in the checked-in corpus.

A model/runtime release should not be promoted because its prose sounds better.
It must preserve valid citations and safe actions, avoid unsupported codes,
meet the generated-specificity expectations, and leave deterministic primary
findings and retry policy unchanged. Strict JSON Schema proves shape and
bounded authority, not diagnostic quality; compare candidate models and
quantized variants with the same corpus. Record the model identifier, runtime
version, profile limits, corpus commit, and evaluation JSON with
release-candidate evidence.
