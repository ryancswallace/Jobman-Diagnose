#!/usr/bin/env python3
"""This file is intentionally invalid so Python reports a SyntaxError."""


def calculate_total(items)
    return sum(item["price"] for item in items)


print(calculate_total([{"price": 10}]))

