#!/bin/sh

# Submit each deliberately failing Python fixture, wait for its terminal
# outcome, and render an AI diagnosis with its bounded target-log tail.

set -u

script_directory=$(CDPATH= cd -P "$(dirname "$0")" && pwd) || exit 1
jobman_command=${JOBMAN:-jobman}
python_command=${PYTHON:-python3}

usage() {
    cat <<'EOF'
usage: run_with_jobman.sh [SCRIPT ...]

Run every numbered Python failure fixture through Jobman, then display
`jobman diagnose --ai-logs` for the newly submitted job. Optional SCRIPT
arguments are base filenames such as 03_invalid_json.py.

Environment:
  JOBMAN  Jobman executable path or command name (default: jobman)
  PYTHON  Python 3.11+ executable path or command name (default: python3)
EOF
}

case ${1-} in
    -h | --help)
        usage
        exit 0
        ;;
esac

if ! command -v "$jobman_command" >/dev/null 2>&1; then
    printf 'error: Jobman executable not found: %s\n' "$jobman_command" >&2
    exit 1
fi
if ! command -v "$python_command" >/dev/null 2>&1; then
    printf 'error: Python executable not found: %s\n' "$python_command" >&2
    exit 1
fi
if ! "$python_command" -c 'import sys; raise SystemExit(sys.version_info < (3, 11))'; then
    printf 'error: Python 3.11 or newer is required\n' >&2
    exit 1
fi
if ! "$jobman_command" diagnose --help >/dev/null 2>&1; then
    printf 'error: jobman diagnose is not installed or discoverable\n' >&2
    exit 1
fi

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
        "$jobman_command" run \
            --name "$job_name" \
            --run-timeout 2s \
            --stop-grace 1s \
            --wait \
            -- "$python_command" "$script_path" \
            >"$run_stdout" 2>"$run_stderr"
        run_status=$?
    else
        "$jobman_command" run \
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
    printf '\nDiagnosis: jobman diagnose --ai-logs %s\n\n' "$job_id"

    if ! "$jobman_command" diagnose --ai-logs "$job_id"; then
        printf 'error: diagnosis failed for %s\n' "$job_id" >&2
        issues=$((issues + 1))
    fi
done

printf '\n%s\n' '=============================================================================='
printf 'Completed %s fixture diagnoses with %s runner issue(s).\n' "$processed" "$issues"

if [ "$issues" -ne 0 ]; then
    exit 1
fi
