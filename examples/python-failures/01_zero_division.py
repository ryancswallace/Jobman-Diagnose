#!/usr/bin/env python3
"""Fail with a simple arithmetic exception through multiple stack frames."""

import sys


def average_unit_cost(total_cost: float, units: int) -> float:
    return total_cost / units


def summarize_batch(batch: dict[str, object]) -> str:
    average = average_unit_cost(float(batch["total_cost"]), int(batch["units"]))
    return f"batch {batch['id']} average cost: {average:.2f}"


batch = {"id": "nightly-042", "total_cost": 125.50, "units": 0}
print(f"processing batch {batch['id']} with {batch['units']} units", file=sys.stderr)
print(summarize_batch(batch))
