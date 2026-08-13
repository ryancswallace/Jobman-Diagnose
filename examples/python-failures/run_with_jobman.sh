#!/bin/sh

# Submit each deliberately failing Python fixture, wait for its terminal
# outcome, and render an AI diagnosis with its bounded target-log tail.

set -u

script_directory=$(CDPATH='' cd -P "$(dirname "$0")" && pwd) || exit 1
repository_directory=$(CDPATH='' cd -P "$script_directory/../.." && pwd) || exit 1
jobman_command=${JOBMAN:-jobman}
diagnose_command=${JOBMAN_DIAGNOSE:-"$repository_directory/bin/jobman-diagnose"}
python_command=${PYTHON:-python3}
source_mode=${JOBMAN_AI_SOURCE:-}

usage() {
    cat <<'EOF'
usage: run_with_jobman.sh [SCRIPT ...]

Run every numbered Python failure fixture through Jobman, then display
`jobman diagnose --ai-logs` for the newly submitted job. Optional SCRIPT
arguments are base filenames such as 03_invalid_json.py.

Environment:
  JOBMAN           Jobman executable path or command name (default: jobman)
  JOBMAN_DIAGNOSE  Companion executable path or command name
                   (default: this checkout's bin/jobman-diagnose)
  PYTHON           Python 3.11+ executable path or command name (default: python3)
  JOBMAN_AI_SOURCE Optional source disclosure mode: limited or full

Run `make build` from the repository root before using the default companion.
EOF
}

case ${1-} in
    -h | --help)
        usage
        exit 0
        ;;
esac

case $source_mode in
    '' | limited | full) ;;
    *)
        printf 'error: JOBMAN_AI_SOURCE must be limited, full, or empty\n' >&2
        exit 2
        ;;
esac

if ! command -v "$jobman_command" >/dev/null 2>&1; then
    printf 'error: Jobman executable not found: %s\n' "$jobman_command" >&2
    exit 1
fi
jobman_path=$(command -v "$jobman_command")
if ! diagnose_path=$(command -v "$diagnose_command"); then
    printf 'error: Jobman Diagnose executable not found: %s\n' "$diagnose_command" >&2
    if [ -z "${JOBMAN_DIAGNOSE+x}" ]; then
        printf 'hint: run make build from %s\n' "$repository_directory" >&2
    fi
    exit 1
fi
diagnose_directory=$(CDPATH='' cd -P "$(dirname "$diagnose_path")" && pwd) || exit 1
diagnose_path="$diagnose_directory/$(basename "$diagnose_path")"
if [ "$(basename "$diagnose_path")" != "jobman-diagnose" ]; then
    printf 'error: JOBMAN_DIAGNOSE must resolve to an executable named jobman-diagnose: %s\n' \
        "$diagnose_path" >&2
    exit 1
fi
# Jobman discovers extension commands by their jobman-<name> executable name.
# Put the explicitly selected build ahead of any installed companion.
PATH="$diagnose_directory${PATH:+:$PATH}"
export PATH
if ! command -v "$python_command" >/dev/null 2>&1; then
    printf 'error: Python executable not found: %s\n' "$python_command" >&2
    exit 1
fi
if ! "$python_command" -c 'import sys; raise SystemExit(sys.version_info < (3, 11))'; then
    printf 'error: Python 3.11 or newer is required\n' >&2
    exit 1
fi
if ! "$jobman_path" diagnose --help >/dev/null 2>&1; then
    printf 'error: Jobman could not dispatch the selected companion: %s\n' "$diagnose_path" >&2
    exit 1
fi

printf 'Jobman:          %s\n' "$jobman_path"
printf 'Jobman Diagnose: %s\n' "$diagnose_path"

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/jobman-python-failures.XXXXXX") || {
    printf 'error: could not create a temporary directory\n' >&2
    exit 1
}
run_stdout="$temporary_directory/run.stdout"
run_stderr="$temporary_directory/run.stderr"

cleanup() {
    rm -f "$run_stdout" "$run_stderr"
    rmdir "$temporary_directory" 2>/dev/null || true
}
trap cleanup 0
trap 'exit 130' HUP INT TERM

if [ "$#" -eq 0 ]; then
    set -- "$script_directory"/[0-9][0-9]_*.py
else
    for argument do
        case $argument in
            */*)
                printf 'error: SCRIPT must be a basename, not a path: %s\n' "$argument" >&2
                exit 2
                ;;
        esac
        if [ ! -f "$script_directory/$argument" ]; then
            printf 'error: failure fixture not found: %s\n' "$argument" >&2
            exit 2
        fi
    done
fi

processed=0
issues=0

for fixture_argument do
    case $fixture_argument in
        /*) script_path=$fixture_argument ;;
        *) script_path="$script_directory/$fixture_argument" ;;
    esac
    fixture=$(basename "$script_path")
    stem=${fixture%.py}
    job_suffix=$(printf '%s' "$stem" | tr '_' '-')
    job_name="py-$job_suffix-$$"

    : >"$run_stdout"
    : >"$run_stderr"
    if [ "$fixture" = "15_hangs_until_timeout.py" ]; then
        "$jobman_path" run \
            --name "$job_name" \
            --run-timeout 2s \
            --stop-grace 1s \
            --wait \
            -- "$python_command" "$script_path" \
            >"$run_stdout" 2>"$run_stderr"
        run_status=$?
    else
        "$jobman_path" run \
            --name "$job_name" \
            --wait \
            -- "$python_command" "$script_path" \
            >"$run_stdout" 2>"$run_stderr"
        run_status=$?
    fi

    job_id=$(sed -n '1p' "$run_stdout")
    if ! printf '%s\n' "$job_id" | grep -Eq \
        '^[[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}$'; then
        printf '\n%s\n' '=============================================================================='
        printf 'Fixture: %s\n' "$fixture"
        printf 'Submission failed before Jobman returned a canonical job ID.\n' >&2
        sed -n '1,20p' "$run_stderr" >&2
        issues=$((issues + 1))
        continue
    fi

    processed=$((processed + 1))
    printf '\n%s\n' '=============================================================================='
    printf 'Fixture: %s\n' "$fixture"
    printf 'Job:     %s\n' "$job_name"
    printf 'ID:      %s\n' "$job_id"
    if [ "$run_status" -eq 0 ]; then
        printf 'Outcome: unexpected success (this fixture should fail)\n'
        issues=$((issues + 1))
    else
        printf 'Outcome: expected failure (jobman run status %s)\n' "$run_status"
    fi
    if [ -n "$source_mode" ]; then
        printf '\nDiagnosis: jobman diagnose --ai-logs --ai-source %s --source-file %s %s\n\n' \
            "$source_mode" "$script_path" "$job_id"
        "$jobman_path" diagnose --ai-logs --ai-source "$source_mode" \
            --source-file "$script_path" "$job_id"
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
done

printf '\n%s\n' '=============================================================================='
printf 'Completed %s fixture diagnoses with %s runner issue(s).\n' "$processed" "$issues"

if [ "$issues" -ne 0 ]; then
    exit 1
fi
