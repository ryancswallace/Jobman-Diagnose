#!/usr/bin/env python3
"""Fail in a record pipeline while preserving a low-level parsing cause."""

import sys
from decimal import Decimal, InvalidOperation


class RecordTransformError(RuntimeError):
    """A source record could not be converted into the canonical schema."""


def parse_amount(record: dict[str, str]) -> Decimal:
    try:
        return Decimal(record["amount"])
    except InvalidOperation as error:
        raise RecordTransformError(
            f"record {record['record_id']} has invalid decimal amount "
            f"{record['amount']!r} from source {record['source']}"
        ) from error


incoming = {
    "record_id": "partner-west:8841",
    "source": "quarterly-rebate.csv",
    "amount": "1,204.5O",
}
print(
    f"transforming record {incoming['record_id']} from {incoming['source']}",
    file=sys.stderr,
)
parse_amount(incoming)
