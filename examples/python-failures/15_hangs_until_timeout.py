#!/usr/bin/env python3
"""Remain stuck long enough for Jobman's run timeout to terminate the target."""

import sys
import time


print("connected to queue; waiting for partition ownership", file=sys.stderr, flush=True)
print("no assignment received; consumer remains in rebalancing", file=sys.stderr, flush=True)

deadline = time.monotonic() + 10
while time.monotonic() < deadline:
    time.sleep(0.5)

raise TimeoutError("safety stop: partition assignment did not arrive within 10 seconds")

