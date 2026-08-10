#!/usr/bin/env python3
"""Fail while parsing a malformed embedded deployment document."""

import json
import sys


document = """{
  "region": "moon-1",
  "retries": 3,
  "features": ["billing", "search",],
  "enabled": true
}"""

print("loading deployment configuration from generated JSON", file=sys.stderr)
configuration = json.loads(document)
print(configuration)

