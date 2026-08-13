# Python diagnosis failure lab

These deliberately failing, standard-library-only programs exercise both
Jobman's deterministic diagnosis and AI-assisted interpretation of bounded
target logs. They are fixtures, not examples of production error handling.

Use Python 3.11 or newer. Run commands from this directory so the command and
working-directory evidence are easy to compare:

```console
cd examples/python-failures
jobman run --name py-zero-division --wait -- python3 "$PWD/01_zero_division.py"
jobman diagnose py-zero-division
jobman diagnose --ai-logs py-zero-division
jobman diagnose --ai-logs --details py-zero-division
```

`--ai` shares bounded execution context but not log text. Use `--ai-logs` for
these fixtures when you want the model to see the sanitized stderr tail that
contains the target-specific failure. Generated conclusions remain advisory.

## Run the complete lab

The included POSIX shell runner submits each fixture, waits for its expected
failure, extracts the newly created canonical job ID, and displays
`jobman diagnose --ai-logs` before continuing to the next case. It prepends the
checkout's `bin/` directory while it runs, so Jobman's extension dispatch uses
the companion produced by `make build` instead of an installed release:

```console
make -C ../.. build
./run_with_jobman.sh
```

Run only selected cases by passing their base filenames:

```console
./run_with_jobman.sh 03_invalid_json.py 12_async_exception_group.py
```

Set `JOBMAN`, `JOBMAN_DIAGNOSE`, or `PYTHON` to select alternate executable
paths. `JOBMAN_DIAGNOSE` must resolve to an executable named
`jobman-diagnose`, matching Jobman's extension protocol. The runner prints both
selected binary paths before submitting jobs. It gives the hanging case a
two-second Jobman run timeout and treats target failures as expected;
configuration, submission, or diagnosis failures make the runner exit nonzero
after it has attempted the remaining cases.

Set `JOBMAN_AI_SOURCE=limited` to add a line-centered snapshot of each Python
fixture to its AI request, or `JOBMAN_AI_SOURCE=full` to add the complete
fixture. The runner supplies `--source-file` explicitly. The selected profile
must allow `source_content`; source text is unredacted and point-in-time.

## Cases

| Script | Failure | Diagnostic challenge |
| --- | --- | --- |
| `01_zero_division.py` | `ZeroDivisionError` through several frames | Simple traceback and immediate cause |
| `02_missing_environment.py` | Required environment variable absent | Configuration versus application failure |
| `03_invalid_json.py` | `JSONDecodeError` with line and column | Malformed configuration syntax |
| `04_configuration_schema.py` | Several related validation errors | Summarizing multiple root causes without losing detail |
| `05_missing_dependency.py` | `ModuleNotFoundError` | Packaging or deployment dependency mismatch |
| `06_missing_file.py` | `FileNotFoundError` for an application input | Path, working-directory, or deployment issue |
| `07_permission_denied.py` | Stable `PermissionError` | Identity, mount, or file-mode diagnosis |
| `08_connection_refused.py` | Loopback `ConnectionRefusedError` | Service unavailable versus bad endpoint |
| `09_chained_timeout.py` | Domain error chained from `TimeoutError` | Separating the high-level failure from its cause |
| `10_missing_executable.py` | Subprocess executable absent | PATH or incomplete installation |
| `11_child_process_exit.py` | Child emits an error and exits 17 | Finding the child failure behind `CalledProcessError` |
| `12_async_exception_group.py` | Two concurrent task failures | Reading an `ExceptionGroup` traceback |
| `13_unicode_decode.py` | Invalid UTF-8 input | Encoding mismatch and bad-record location |
| `14_business_invariant.py` | Assertion with domain context | Data problem rather than infrastructure failure |
| `15_hangs_until_timeout.py` | Target remains alive | Jobman run-timeout classification; logs show the stuck phase |
| `16_signal_termination.py` | Target terminates itself with `SIGTERM` | Signal classification rather than an exception |
| `17_syntax_error.py` | Parser rejects the program | Source/deployment defect before application startup |
| `18_pipeline_cause_chain.py` | Record transform error chained from decimal parsing | Layered ETL-style root-cause analysis |

Most cases run normally:

```console
jobman run --name py-invalid-json --wait -- python3 "$PWD/03_invalid_json.py"
jobman diagnose --ai-logs py-invalid-json
```

The hanging case must have a Jobman timeout. The script has a ten-second safety
stop, but the intended failure is Jobman terminating it first:

```console
jobman run --name py-hang --run-timeout 2s --stop-grace 1s --wait -- \
  python3 "$PWD/15_hangs_until_timeout.py"
jobman diagnose --ai-logs py-hang
```

The signal case needs no special option:

```console
jobman run --name py-signal --wait -- python3 "$PWD/16_signal_termination.py"
jobman diagnose --ai-logs py-signal
```

For useful comparisons, diagnose each completed job three ways:

```console
jobman diagnose JOB
jobman diagnose --ai JOB
jobman diagnose --ai-logs JOB
jobman diagnose --ai-logs --ai-source limited JOB
```

The first shows deterministic evidence only. The second tests inference from
metadata, command, path, environment-name, and system context. The third adds
the bounded redacted log tail. The fourth adds current source around the
traceback location and should help distinguish the implementation path from
the runtime symptom.
