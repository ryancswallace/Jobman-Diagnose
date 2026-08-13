#!/bin/sh

# Deliberately invoke a helper that should not exist in the lab environment.
printf '%s\n' 'rendering summary.pdf through report-converter' >&2
printf '%s\n' 'quarterly report data' | report-converter --format pdf
