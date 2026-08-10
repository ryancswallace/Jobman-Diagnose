#!/usr/bin/env python3
"""Fail after a child process reports a specific application error."""

import subprocess
import sys


child_program = """
import sys
print('migration rejected: database schema is version 8, expected version 11', file=sys.stderr)
print('hint: apply migrations 009 through 011 before starting the worker', file=sys.stderr)
raise SystemExit(17)
"""

print("checking database migration compatibility", file=sys.stderr)
subprocess.run([sys.executable, "-c", child_program], check=True)

