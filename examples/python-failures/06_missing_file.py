#!/usr/bin/env python3
"""Fail because a required application input was not deployed."""

import sys
from pathlib import Path


input_path = Path(__file__).with_name("inputs") / "customer-segments.csv"
print(f"reading customer segments from {input_path}", file=sys.stderr)
rows = input_path.read_text(encoding="utf-8").splitlines()
print(f"loaded {len(rows)} customer segments")

