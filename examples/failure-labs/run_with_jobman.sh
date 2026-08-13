#!/bin/sh

# cspell:ignore javac rustc

# Run realistic failures from several runtimes through Jobman and diagnose
# each completed job with the companion built in this checkout.

set -u

script_directory=$(CDPATH='' cd -P "$(dirname "$0")" && pwd) || exit 1
repository_directory=$(CDPATH='' cd -P "$script_directory/../.." && pwd) || exit 1
jobman_command=${JOBMAN:-jobman}
diagnose_command=${JOBMAN_DIAGNOSE:-"$repository_directory/bin/jobman-diagnose"}
source_mode=${JOBMAN_AI_SOURCE:-}

usage() {
    cat <<'EOF'
usage: run_with_jobman.sh

Run the shell and Go failure labs, plus Node.js, C, JVM, and Rust labs when
their toolchains are available. Every submitted target is expected to fail;
the runner displays `jobman diagnose --ai-logs` for each resulting job.

Environment:
  JOBMAN           Jobman executable path or command name (default: jobman)
  JOBMAN_DIAGNOSE  Companion executable path or command name
                   (default: this checkout's bin/jobman-diagnose)
  JOBMAN_AI_SOURCE Optional source disclosure mode: limited or full

Run `make build` from the repository root before using the default companion.
EOF
}

case ${1-} in
    -h | --help)
        usage
        exit 0
        ;;
    '') ;;
    *)
        usage >&2
        exit 2
        ;;
esac

case $source_mode in
    '' | limited | full) ;;
    *)
        printf 'error: JOBMAN_AI_SOURCE must be limited, full, or empty\n' >&2
        exit 2
        ;;
esac

if ! jobman_path=$(command -v "$jobman_command"); then
    printf 'error: Jobman executable not found: %s\n' "$jobman_command" >&2
    exit 1
fi
if ! diagnose_path=$(command -v "$diagnose_command"); then
    printf 'error: Jobman Diagnose executable not found: %s\n' "$diagnose_command" >&2
    printf 'hint: run make build from %s\n' "$repository_directory" >&2
    exit 1
fi
diagnose_directory=$(CDPATH='' cd -P "$(dirname "$diagnose_path")" && pwd) || exit 1
diagnose_path="$diagnose_directory/$(basename "$diagnose_path")"
if [ "$(basename "$diagnose_path")" != "jobman-diagnose" ]; then
    printf 'error: JOBMAN_DIAGNOSE must resolve to an executable named jobman-diagnose: %s\n' \
        "$diagnose_path" >&2
    exit 1
fi
PATH="$diagnose_directory${PATH:+:$PATH}"
export PATH
if ! "$jobman_path" diagnose --help >/dev/null 2>&1; then
    printf 'error: Jobman could not dispatch the selected companion: %s\n' "$diagnose_path" >&2
    exit 1
fi

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/jobman-failure-labs.XXXXXX") || exit 1
run_stdout="$temporary_directory/run.stdout"
run_stderr="$temporary_directory/run.stderr"

cleanup() {
    rm -f "$temporary_directory"/*
    rmdir "$temporary_directory" 2>/dev/null || true
}
trap cleanup 0
trap 'exit 130' HUP INT TERM

processed=0
skipped=0
issues=0

run_case() {
    case_label=$1
    required_tool=$2
    source_file=$3
    shift 3
    if ! command -v "$required_tool" >/dev/null 2>&1; then
        printf 'SKIP %-28s required tool is unavailable: %s\n' "$case_label" "$required_tool"
        skipped=$((skipped + 1))
        return
    fi

    job_name="lab-$case_label-$$"
    : >"$run_stdout"
    : >"$run_stderr"
    "$jobman_path" run --name "$job_name" --wait -- "$@" >"$run_stdout" 2>"$run_stderr"
    run_status=$?
    job_id=$(sed -n '1p' "$run_stdout")
    if ! printf '%s\n' "$job_id" | grep -Eq \
        '^[[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}$'; then
        printf 'error: %s did not return a canonical Jobman ID\n' "$case_label" >&2
        sed -n '1,20p' "$run_stderr" >&2
        issues=$((issues + 1))
        return
    fi

    processed=$((processed + 1))
    printf '\n%s\n' '=============================================================================='
    printf 'Case:    %s\n' "$case_label"
    printf 'Command: %s\n' "$*"
    printf 'ID:      %s\n' "$job_id"
    if [ "$run_status" -eq 0 ]; then
        printf 'Outcome: unexpected success\n'
        issues=$((issues + 1))
    else
        printf 'Outcome: expected failure (jobman run status %s)\n' "$run_status"
    fi
    if [ -n "$source_mode" ]; then
        printf '\nDiagnosis: jobman diagnose --ai-logs --ai-source %s --source-file %s %s\n\n' \
            "$source_mode" "$source_file" "$job_id"
        "$jobman_path" diagnose --ai-logs --ai-source "$source_mode" \
            --source-file "$source_file" "$job_id"
        diagnose_status=$?
    else
        printf '\nDiagnosis: jobman diagnose --ai-logs %s\n\n' "$job_id"
        "$jobman_path" diagnose --ai-logs "$job_id"
        diagnose_status=$?
    fi
    if [ "$diagnose_status" -ne 0 ]; then
        printf 'error: diagnosis failed for %s\n' "$job_id" >&2
        issues=$((issues + 1))
    fi
}

printf 'Jobman:          %s\n' "$jobman_path"
printf 'Jobman Diagnose: %s\n' "$diagnose_path"

run_case shell-unbound-variable sh "$script_directory/shell/01_unbound_variable.sh" sh "$script_directory/shell/01_unbound_variable.sh"
run_case shell-pipeline-command sh "$script_directory/shell/02_pipeline_command.sh" sh "$script_directory/shell/02_pipeline_command.sh"
run_case go-wrapped-configuration go "$script_directory/go/01_wrapped_configuration.go" go run "$script_directory/go/01_wrapped_configuration.go"
run_case go-context-deadline go "$script_directory/go/02_context_deadline.go" go run "$script_directory/go/02_context_deadline.go"
run_case go-parse-record go "$script_directory/go/03_parse_record.go" go run "$script_directory/go/03_parse_record.go"
run_case node-missing-module node "$script_directory/node/01_missing_module.js" node "$script_directory/node/01_missing_module.js"
run_case node-type-error node "$script_directory/node/02_type_error.js" node "$script_directory/node/02_type_error.js"
run_case node-address-in-use node "$script_directory/node/03_address_in_use.js" node "$script_directory/node/03_address_in_use.js"
run_case native-linker-error cc "$script_directory/native/01_linker_error.c" cc "$script_directory/native/01_linker_error.c" -o "$temporary_directory/native-linker-error"

if command -v javac >/dev/null 2>&1 && command -v java >/dev/null 2>&1 &&
    javac -version >/dev/null 2>&1 && java -version >/dev/null 2>&1; then
    if javac -d "$temporary_directory" "$script_directory/jvm/FailureLab.java"; then
        run_case jvm-nested-cause java "$script_directory/jvm/FailureLab.java" java -cp "$temporary_directory" FailureLab
    else
        printf 'error: Java failure-lab setup did not compile\n' >&2
        issues=$((issues + 1))
    fi
else
    printf 'SKIP %-28s required tools are unavailable: java and javac\n' jvm-nested-cause
    skipped=$((skipped + 1))
fi

if command -v rustc >/dev/null 2>&1; then
    if rustc "$script_directory/rust/01_index_panic.rs" -o "$temporary_directory/rust-index-panic"; then
        run_case rust-index-panic rustc "$script_directory/rust/01_index_panic.rs" "$temporary_directory/rust-index-panic"
    else
        printf 'error: Rust failure-lab setup did not compile\n' >&2
        issues=$((issues + 1))
    fi
else
    printf 'SKIP %-28s required tool is unavailable: rustc\n' rust-index-panic
    skipped=$((skipped + 1))
fi

printf '\n%s\n' '=============================================================================='
printf 'Completed %s diagnoses; skipped %s unavailable toolchains; found %s runner issue(s).\n' \
    "$processed" "$skipped" "$issues"

if [ "$issues" -ne 0 ]; then
    exit 1
fi
