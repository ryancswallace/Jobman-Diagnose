#!/usr/bin/env python3
"""Fail with several related configuration validation problems."""

import sys


def validate(configuration: dict[str, object]) -> None:
    errors: list[str] = []
    if configuration.get("region") not in {"us-east-1", "us-west-2"}:
        errors.append("region must be one of us-east-1 or us-west-2")
    if not isinstance(configuration.get("retries"), int):
        errors.append("retries must be an integer, not a string")
    timeout = configuration.get("request_timeout_seconds")
    if not isinstance(timeout, (int, float)) or timeout <= 0:
        errors.append("request_timeout_seconds must be greater than zero")
    database = configuration.get("database")
    if not isinstance(database, dict) or not database.get("dsn"):
        errors.append("database.dsn is required")
    if errors:
        rendered = "\n  - ".join(errors)
        raise ValueError(f"deployment configuration is invalid:\n  - {rendered}")


candidate = {
    "region": "moon-1",
    "retries": "three",
    "request_timeout_seconds": -5,
    "database": {"pool_size": 8},
}
print("validating production deployment configuration", file=sys.stderr)
validate(candidate)

