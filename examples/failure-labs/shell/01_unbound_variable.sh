#!/bin/sh

# Deliberately require an unset deployment value.
set -u

printf '%s\n' 'validating deployment environment' >&2
printf 'selected region: %s\n' "$APP_REGION"
