# Deterministic analyzers

The built-in engine evaluates trusted structured core classifications and
exact companion-derived log ranges separately.
Those rules identify direct executable, working-directory, permission,
timeout, cancellation, ownership, signal, start, wait, log, and nonzero-exit
mechanisms. Direct core observations receive a stronger confidence basis than
target-output signatures. Exact launch, lifecycle, and policy facts continue to
outrank target messages; a recognized causal target message can outrank the
otherwise generic `core.nonzero_exit` mechanism.

When explicitly collected command evidence identifies `/usr/bin/false` or
`/bin/false` and the selected run has a nonzero exit, the engine reports that
the standard false utility intentionally returns failure. This exact rule
outranks the generic nonzero-exit mechanism, advises replacing the placeholder
command, and states that an unchanged retry will fail again.

Bounded log artifacts are untrusted data. The collector first attributes exact
byte ranges for Python, Go, and JVM traces, compiler diagnostics, and a bounded
catalog of conventional causal messages. Causal ranges are classified into
controlled target-reported diagnoses covering:

- missing commands, modules, artifacts, libraries, files, configuration, and
  migrations;
- permission, authentication, read-only filesystem, storage, file-descriptor,
  listener, and linker failures;
- refused connections, deadlines, DNS, TLS, unavailable services, and rate
  limits; and
- parse/validation errors, database deadlocks, and uniqueness violations.

It never treats those bytes as instructions, does not copy them into findings,
and does not promote a target message to a confirmed platform fact. For
example, a storage-exhaustion message remains distinct from a measured
filesystem-capacity observation.

All causal-message findings state that the target reported the condition and
that Jobman did not independently inspect the relevant host, dependency,
database, input, or policy. Their actions are bounded, local, read-only Jobman
inspection commands. No process is stopped, package installed, credential
printed, migration applied, data changed, or run created automatically.

When one artifact contains multiple causal messages, the final recognized
diagnostic range is primary and earlier recognized ranges remain separately
cited findings. This handles logs where a recoverable warning precedes the
terminal cause without erasing the warning. Ambiguous messages such as
`Killed`, exit status 137 without a recorded reason, or `CrashLoopBackOff`
without terminated-container logs do not authorize an OOM or application-cause
diagnosis.

Secondary rules identify degraded Jobman log recording, failed notification
delivery, repeated structured failure classes, and exact same-fingerprint
history. Exact history is stronger than matching a broad failure class: every
matching summary cites a core-supplied `local_only` item derived from the same
keyed factual fingerprint. Resource observations are also cited as related
evidence, but the engine does not infer OOM, CPU exhaustion, or a root cause
from process-scoped CPU time or peak resident memory alone. A successful target
can therefore have a notification finding without being mislabeled as a target
failure.

Retry advice follows a separate controlled policy. Persistent configuration,
dependency, access, validation, storage, TLS, linker, and uniqueness conditions
produce `after_change`. Target-reported refusal, deadline, deadlock, DNS,
rate-limit, and service-unavailable messages produce conservative
`after_delay`: they may be transient, but the evidence does not prove they have
cleared. Explicit cancellation and success produce `not_applicable`; ownership
loss may permit `now` with a caveat; and an observed signal without cause
produces `unknown`. Advice never creates a run and must cite its factual basis.
For an otherwise persistent condition, multiple exact historical failures
raise confidence in `after_change`. If an exact match was later followed by a
successful run of the same job, advice becomes conservative `after_delay`:
that is evidence of past transience, not proof that the condition has cleared
now. Partial or truncated history is surfaced as a warning.
