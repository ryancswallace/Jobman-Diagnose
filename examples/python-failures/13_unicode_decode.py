#!/usr/bin/env python3
"""Fail when a Latin-1 byte sequence is decoded as UTF-8."""

import sys


record = b"customer_id=42,comment=invalid-\xe9-byte,region=west"
print("decoding partner feed record 184 as UTF-8", file=sys.stderr)
decoded = record.decode("utf-8")
print(decoded)
