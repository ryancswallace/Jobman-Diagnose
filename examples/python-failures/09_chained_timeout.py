#!/usr/bin/env python3
"""Fail with a domain exception chained from a simulated upstream timeout."""

import sys


class UpstreamUnavailable(RuntimeError):
    """The application could not obtain a required upstream response."""


def fetch_inventory_snapshot() -> dict[str, int]:
    raise TimeoutError("inventory service did not respond within 750 ms")


def synchronize_inventory() -> None:
    try:
        fetch_inventory_snapshot()
    except TimeoutError as error:
        raise UpstreamUnavailable(
            "inventory synchronization failed after the final retry"
        ) from error


print("inventory synchronization attempt 3 of 3", file=sys.stderr)
synchronize_inventory()
