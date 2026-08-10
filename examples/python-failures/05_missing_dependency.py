#!/usr/bin/env python3
"""Fail because a private application dependency is not installed."""

import sys

print("loading the feature-flag adapter", file=sys.stderr)

import acme_internal_feature_flags  # type: ignore[import-not-found]  # noqa: F401, E402

