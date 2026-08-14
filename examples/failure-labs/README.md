# Multi-ecosystem diagnosis failure lab

These deliberately failing programs exercise AI-assisted diagnosis with real
runtime, compiler, wrapper, and cause-chain output. The shell and Go cases use
tools already required by this repository. Node.js, C, JVM, and Rust cases run
when their corresponding local toolchains are available and are otherwise
reported as skipped.

This executable lab declares 11 programs and is intentionally smaller than the
72-case normalized evaluation corpus. The observed count can be lower when an
optional Node.js, C, JVM, or Rust toolchain is unavailable or its setup step is
skipped. Use `devel/evaluate` for the full corpus; it does not execute these
programs during the evaluation run.

Build the companion in this checkout, then run the lab:

```console
make build
./examples/failure-labs/run_with_jobman.sh
```

The runner prepends the checkout's `bin/` directory so Jobman's extension
dispatch selects the freshly built `jobman-diagnose`. Set `JOBMAN` or
`JOBMAN_DIAGNOSE` to override either executable. Every target failure is
expected; submission, setup, or diagnosis failures make the runner exit
nonzero.

Set `JOBMAN_AI_SOURCE=limited` or `JOBMAN_AI_SOURCE=full` to share the
corresponding source file for every lab case. The runner supplies an explicit
source path even for compiled JVM and Rust targets. The profile must allow
`source_content`; source snapshots are unredacted, point-in-time context.

The executable lab is for realistic exploratory testing. Stable, normalized
versions of these failures live in the generated evaluation corpus and run in
ordinary CI without requiring every language toolchain:

```console
make evaluate
go run ./devel/evaluate --tags language.node --summary
```

Use `--repeat` during live evaluation to measure consistency:

```console
go run ./devel/evaluate \
  --live \
  --diagnosis-config /absolute/path/diagnosis.yml \
  --profile local-vllm \
  --share metadata,log_content \
  --tags language.go,language.node \
  --repeat 3 \
  --allow-fallback \
  --output evaluation-multi-ecosystem.json \
  --capture-proposals proposals-multi-ecosystem.json
```

Raw proposal captures contain the disclosed synthetic content and full model
output. They are private local artifacts and must not be committed.
