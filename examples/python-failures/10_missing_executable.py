#!/usr/bin/env python3
"""Fail because a required subprocess executable is not installed."""

import subprocess
import sys


command = ["warehouse-migrate", "--database", "analytics", "--check"]
print(f"launching migration helper: {command[0]}", file=sys.stderr)
subprocess.run(command, check=True)
