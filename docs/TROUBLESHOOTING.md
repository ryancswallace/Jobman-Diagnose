# Troubleshooting

Start with a deterministic diagnosis. It exercises Jobman evidence collection,
validation, and rendering without involving a model or network service:

```console
jobman diagnose JOB
```

If that fails, verify that both executables are on `PATH`, run the companion
directly with `--jobman /absolute/path/to/jobman`, and compare the installed
versions with the [compatibility matrix](COMPATIBILITY.md).

## Validate AI configuration before invoking a model

```console
jobman diagnose config paths
jobman diagnose config validate
jobman diagnose config show
jobman diagnose profiles
```

These commands do not resolve credentials or call a provider. They show the
selected file, effective nonsecret settings, and available profiles. An
explicit `--diagnosis-config` or `JOBMAN_DIAGNOSE_CONFIG` takes precedence over
the per-user default.

For a provider call that may take time, use durable progress messages:

```console
jobman diagnose --ai --progress plain JOB
```

Interactive human output uses a delayed spinner automatically. JSON and
redirected output remain free of progress text so they are safe to parse.

## A health endpoint passes but generation fails

A models or health endpoint proves only that the server is reachable. The chat
request can still fail because of an unsupported JSON Schema feature, model
name mismatch, context limit, timeout, malformed structured output, or a
proposal that violates Jobman Diagnose's semantic invariants.

Run with `--require-model` to make optional augmentation failure fatal and read
the stable provider failure classification. Check the provider's server log for
the corresponding request, then confirm the configured endpoint, model,
timeout, input/output limits, and structured-output support. Provider response
bodies are intentionally not copied into CLI errors because they can contain
untrusted or sensitive content.

Without `--require-model`, the complete deterministic report is returned with
a warning. This is the expected safe fallback, not a silent partial diagnosis.

## The model lacks useful context

AI activation shares bounded metadata, direct command arguments, paths,
environment variable names, effective execution policy, and—when supported by
core—point-in-time state-filesystem and cgroup constraints when the selected
profile permits those classes. It does not share environment values, host
paths, hostnames, process lists, or system logs. Jobman v1.4.0 is retried
without the newer `--system` flag; upgrade core to collect that additional
context.

Log bytes require explicit intent because they can contain secrets that no
automatic redactor recognizes:

```console
jobman diagnose --ai-logs --log-bytes 64KiB JOB
```

Inspect the report's disclosure section to see exactly which evidence classes,
items, and bytes were projected. Review the [security model](SECURITY_MODEL.md)
before sending evidence outside your security boundary.

## Collect safe support information

Preview a support bundle before writing it:

```console
jobman diagnose --support-bundle diagnosis-support.tar.gz --bundle-dry-run JOB
```

Then create it only if the inventory is appropriate. Review `evidence.json`
inside the archive before sharing it. Never attach credentials, raw private
logs, provider tokens, local configuration containing secret references, or a
Jobman state database to a public issue. See [support bundles](SUPPORT_BUNDLES.md)
and the [support policy](../SUPPORT.md).
