# Support bundles

A support bundle is a deterministic, private `.tar.gz` archive for sharing one
diagnosis with an operator or maintainer. It packages only data already selected
for that diagnosis; bundle creation performs no additional Jobman query, file
read, provider call, or system-event collection.

Preview every member without creating a file:

```console
jobman diagnose --support-bundle diagnosis-support.tar.gz --bundle-dry-run JOB
```

Then create it explicitly:

```console
jobman diagnose --support-bundle diagnosis-support.tar.gz JOB
```

The path is created with private permissions, published atomically, and never
overwritten. The normal diagnosis is still written to standard output. JSON
mode remains a clean report stream; the archive path is not injected into JSON.

Schema-1 archives contain one fixed root directory and these members:

| File | Contents |
| --- | --- |
| `INVENTORY.txt` | Human-readable member list and sharing caution |
| `manifest.json` | Bundle IDs, member descriptions, disclosure labels, sizes, and SHA-256 values |
| `evidence.json` | Exact sealed, sanitized core evidence selected for diagnosis |
| `enrichment.json` | Companion enrichment IDs, collectors, and exact source ranges |
| `diagnosis.json` | Validated diagnosis report |
| `disclosure.json` | Exact optional-provider disclosure manifest |
| `capabilities.json` | Jobman capability and omission facts |
| `build.json` | Companion version/commit/date, Go version, OS, and architecture |

Entries have stable ordering, timestamps, ownership, and private modes. Given
the same sealed evidence, report, and build metadata, archive bytes are
reproducible. The manifest hashes every member other than itself.

The writer never includes provider credentials, environment values, a Jobman
database, raw state files, or the store fingerprint key. The selected evidence
may include commands, arguments, paths, or bounded logs when the invocation
explicitly collected them. Review `evidence.json` before sending an archive
outside your security boundary.
