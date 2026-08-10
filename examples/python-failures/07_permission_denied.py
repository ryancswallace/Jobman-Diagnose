#!/usr/bin/env python3
"""Fail with a deterministic permission error on every local identity."""

import errno
import sys


protected_path = "/srv/payments/private-key.pem"
print(f"loading signing material from {protected_path}", file=sys.stderr)
raise PermissionError(
    errno.EACCES,
    "service identity cannot read the configured signing key",
    protected_path,
)

