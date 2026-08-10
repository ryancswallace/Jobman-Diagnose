#!/usr/bin/env python3
"""Terminate the current process with SIGTERM after emitting context."""

# cspell:ignore getpid

import os
import signal
import sys


print("worker detected an unrecoverable coordinator state", file=sys.stderr, flush=True)
print("worker is terminating itself with SIGTERM", file=sys.stderr, flush=True)
os.kill(os.getpid(), signal.SIGTERM)
