# Diagnosis evaluation

<!-- cspell:ignore quantizations -->

The checked-in schema-4 corpus under `testdata/evaluation/` measures diagnostic
quality independently from exact prose. Its 60 nonsecret cases
combine immutable Jobman compatibility evidence with reproducibly generated
Python, shell, Go, Node.js, C/native, JVM, and Rust failures. The corpus also
contains ambiguous, truncated, and prompt-injection controls where the correct
model behavior is to abstain.

Every case defines deterministic finding, action, retry, and confidence
expectations. Generated behavior uses one of three dispositions:

- `required`: the disclosed evidence supports one concrete generated cause;
- `allowed`: a supported cause is useful but an abstention is acceptable; or
- `must_abstain`: the evidence does not justify a concrete generated cause.

Required-cause cases describe incident facts as sets of accepted wording
alternatives, causal relations whose cause and effect must both survive, claims
that must not appear, and a citation ceiling. For example, a network case can
require both `connection refused` and port `5432`, plus a relation from that
refusal to the failed database connection. This accepts natural wording while
rejecting fluent restatements such as “invalid input caused a nonzero exit.”

## Deterministic evaluation

Run the entire corpus without a provider, network, or diagnosis configuration:

```console
make evaluate
```

Regenerate the stable synthetic evidence and its manifest after intentionally
changing the generator:

```console
make gen-evaluation-fixtures
make evaluation-fixtures-check
```

The check regenerates both into a temporary directory and compares them byte
for byte. Never add developer logs, credentials, or production job data.

## Focused and repeated runs

Cases carry orthogonal `language.*`, `failure.*`, `format.*`, `lifecycle.*`,
`source.*`, `context.source`, and control tags. `context.source` identifies a
case with a checked-in source mapping. A case matches when it has any requested tag;
exact case names and tags are intersected when both filters are supplied.

```console
go run ./devel/evaluate --tags language.python --summary
go run ./devel/evaluate --cases tls_certificate,dns_failure --summary
go run ./devel/evaluate --tags language.go,language.node --repeat 3 --summary
```

`--repeat` accepts 1–20 and runs each selected case sequentially. Sequential
execution avoids turning provider capacity or scheduling into an accidental
test variable. The result records a one-based iteration for every execution.

## Live model evaluation

Live evaluation is manual and explicit because it may disclose the approved
fixture projection and consume provider capacity:

```console
go run ./devel/evaluate \
  --live \
  --diagnosis-config /absolute/path/diagnosis.yml \
  --profile local-vllm \
  --share metadata,log_content \
  --repeat 3 \
  --output evaluation-local-vllm.json
```

Live mode requires an explicit configuration path and requires the provider by
default. Use `--allow-fallback` only when intentionally measuring fallback
behavior. Approve `log_content` only for sanitized corpus evidence that carries
the configured-value redaction capability. Older immutable fixtures that lack
that capability are automatically evaluated with metadata only.

When the selected profile has `source_context.mode: limited` or `full`, live
evaluation automatically approves `source_content` and applies that mode and
the configured symmetric radius to every `context.source` case. The current
corpus runs all 60 cases and attaches checked-in source to 28 of them; the
remaining evidence-only cases still run without source. No additional
`--share` class or source flag is required. Each result records
`source_context_used`, and the summary reports the number of source-enabled
executions. Source mappings are repository-relative, strictly bounded, and
validated by the corpus generator.

To inspect rejected as well as accepted raw model output, capture proposals to
a separate private file:

```console
go run ./devel/evaluate \
  --live \
  --diagnosis-config /absolute/path/diagnosis.yml \
  --profile local-vllm \
  --share metadata,log_content \
  --allow-fallback \
  --output evaluation-local-vllm.json \
  --capture-proposals proposals-local-vllm.json
```

Proposal-capture schema 3 labels every record with its case and iteration,
binds it to the companion analysis-evidence ID,
provider result, parsed proposal when available, and the host acceptance or
rejection reason. The file can contain complete model output and disclosed
fixture content. Evaluation results and proposal captures are atomically
written with user-only permissions and replace an existing file at their
explicit paths, so repeated runs can use stable filenames. They must not be
committed.

## Metrics and promotion criteria

Evaluation-result schema 4 records both core and companion analysis-evidence
IDs and keeps correctness, safety, usefulness, and
stability separate:

- deterministic precision, citation validity, safe actions, retry accuracy,
  fingerprint stability, and provider fallback;
- proposal acceptance and taxonomy accuracy;
- useful-diagnosis rate for `required` cases;
- expected-entity preservation and causal-relation completeness;
- abstention accuracy for `must_abstain` cases;
- forbidden-claim rate and citation economy; and
- generated consistency across repeated executions; and
- the number of executions that received checked-in source context.

Every conditional metric includes its denominator. A metric with no applicable
cases is reported as `n/a` in the compact summary rather than as a misleading
zero. Generated consistency compares the accepted/abstained disposition and
generated code against the first iteration; it does not demand identical
wording.

Case-level semantic expectations require details that distinguish or make the
diagnosis actionable, not every incidental log operand. They accept bounded
wording alternatives for equivalent causal statements and do not require
remediation commands, generic lifecycle narration, or downstream output names
when the concrete root cause is already specific. Host grounding checks apply
the same principle to diagnostic identifiers, so formatting-only variants such
as `TimeoutError` and `timeout error` retain the same supported signal.

A model/runtime should not be promoted because its prose sounds better or
because its schema acceptance rate is high. It should preserve deterministic
findings and retry policy, meet the case-level semantic expectations, cite
economically, abstain on controls, and remain stable under repetition. Compare
candidate models and quantizations against the same corpus commit and record
the model identifier, runtime version, profile limits, and evaluation JSON with
release-candidate evidence.

Strict JSON Schema establishes shape and bounded authority. The host's
evidence-grounding checks and this corpus establish diagnostic usefulness.
