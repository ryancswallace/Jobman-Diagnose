#!/usr/bin/env python3
"""Fail because deployment configuration was not supplied."""

import os
import sys


def required_environment(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        raise RuntimeError(
            f"required environment variable {name} is missing; "
            "configure the payments service endpoint"
        )
    return value


print("initializing payment reconciliation", file=sys.stderr)
endpoint = required_environment("JOBMAN_DEMO_PAYMENTS_API_URL")
print(f"configured endpoint has {len(endpoint)} characters")
