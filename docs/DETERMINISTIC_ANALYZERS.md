# Deterministic analyzers

The built-in engine evaluates trusted structured core classifications first.
Those rules identify direct executable, working-directory, permission,
timeout, cancellation, ownership, signal, start, wait, log, and nonzero-exit
mechanisms. The direct core observation receives a higher priority and stronger
confidence basis than any target-output heuristic.

When explicitly collected command evidence identifies `/usr/bin/false` or
`/bin/false` and the selected run has a nonzero exit, the engine reports that
the standard false utility intentionally returns failure. This exact rule
outranks the generic nonzero-exit mechanism, advises replacing the placeholder
command, and states that an unchanged retry will fail again.

Bounded log artifacts are untrusted data. The engine currently recognizes only
conservative signatures for:

- a Python traceback;
- `no space left on device`;
- a shell's nested `command not found`;
- `permission denied`; and
- `connection refused`.

It never treats those bytes as instructions, does not copy them into findings,
and does not promote a target message to a confirmed platform fact. For
example, a storage-exhaustion message remains distinct from a measured
filesystem-capacity observation.

Secondary rules identify degraded Jobman log recording, failed notification
delivery, repeated structured failure classes, and exact same-fingerprint
history. Exact history is stronger than matching a broad failure class: every
matching summary cites a core-supplied `local_only` item derived from the same
keyed factual fingerprint. Resource observations are also cited as related
evidence, but the engine does not infer OOM, CPU exhaustion, or a root cause
from process-scoped CPU time or peak resident memory alone. A successful target
can therefore have a notification finding without being mislabeled as a target
failure.

Retry advice follows a separate controlled policy. Persistent configuration or
application conditions generally produce `after_change`; explicit cancellation
and success produce `not_applicable`; ownership loss may permit `now` with a
caveat; and an observed signal without cause produces `unknown`. Advice never
creates a run and must cite its factual basis.
For an otherwise persistent condition, multiple exact historical failures
raise confidence in `after_change`. If an exact match was later followed by a
successful run of the same job, advice becomes conservative `after_delay`:
that is evidence of past transience, not proof that the condition has cleared
now. Partial or truncated history is surfaced as a warning.
