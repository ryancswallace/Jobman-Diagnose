# Render this tape from docs/screencaps in the Jobman-Diagnose repository.
#
# Prerequisites:
#   - build Jobman-Diagnose with `make build`
#   - set JOBMAN_DEMO_CORE to an absolute path to a Jobman executable
#   - set JOBMAN_DIAGNOSE_CONFIG to an absolute path to a trusted AI config
#   - select a default profile that permits log_content disclosure
#   - start and warm the configured model before recording
#
# Example:
#   JOBMAN_DEMO_CORE=/path/to/jobman/bin/jobman \
#     JOBMAN_DIAGNOSE_CONFIG=/path/to/diagnosis.yaml \
#     vhs tape/diagnose.tape

Output gif/diagnose.gif
Output webm/diagnose.webm
Output mp4/diagnose.mp4

Set Shell "zsh"
Set FontSize 18
Set Width 1200
Set Height 800
Set TypingSpeed 60ms
Set Padding 24

Hide
Type `: "${JOBMAN_DEMO_CORE:?set JOBMAN_DEMO_CORE to the Jobman executable}"`
Enter
Type `: "${JOBMAN_DIAGNOSE_CONFIG:?set JOBMAN_DIAGNOSE_CONFIG to the diagnosis config}"`
Enter
Type `test -x "$JOBMAN_DEMO_CORE"`
Enter
Type `test -x ../../bin/jobman-diagnose`
Enter
Type `DEMO_ROOT=$(mktemp -d)`
Enter
Type `mkdir -p "$DEMO_ROOT/work"`
Enter
Type `DIAGNOSE_BIN="$PWD/../../bin"`
Enter
Type `export PATH="$DIAGNOSE_BIN:$PATH"`
Enter
Type `jobman() { "$JOBMAN_DEMO_CORE" --state-dir "$DEMO_ROOT/state" "$@"; }`
Enter
Type `fresh_screen() { printf '\033[2J\033[3J\033[H'; }`
Enter
Type `cd "$DEMO_ROOT/work"`
Enter
Type `fresh_screen`
Enter
Wait

Show
Type `jobman run --name config-demo -- /bin/sh -c 'cat ./missing-demo.yml'`
Sleep 400ms
Enter
Wait
Sleep 2s

Type `jobman wait config-demo`
Sleep 400ms
Enter
Wait
Sleep 2s

Hide
Type `fresh_screen`
Enter
Wait
Show

Type `jobman diagnose config-demo`
Sleep 400ms
Enter
Wait
ScrollUp@5ms 200
Sleep 5s
ScrollDown@35ms 22
Sleep 5s
ScrollDown@35ms 22
Sleep 4s

Hide
Type `fresh_screen`
Enter
Wait
Show

Type `jobman diagnose --ai-logs config-demo`
Sleep 400ms
Enter
Wait
ScrollUp@5ms 200
Sleep 6s
ScrollDown@35ms 22
Sleep 5s
ScrollDown@35ms 22
Sleep 5s
ScrollDown@35ms 22
Sleep 5s
ScrollDown@35ms 22
Sleep 5s

Hide
Type `jobman clean --all --force`
Enter
Wait
Type `cd /`
Enter
Type `rm -rf "$DEMO_ROOT"`
Enter
Wait
