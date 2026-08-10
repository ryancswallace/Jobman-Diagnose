#!/usr/bin/env python3
"""Fail with a deterministic connection-refused application error."""

# cspell:ignore ECONNREFUSED

import errno
import sys


endpoint = "127.0.0.1:4319"
print(f"connecting to local inventory service at {endpoint}", file=sys.stderr)
raise ConnectionRefusedError(
    errno.ECONNREFUSED,
    "inventory service refused the configured connection",
    endpoint,
)
