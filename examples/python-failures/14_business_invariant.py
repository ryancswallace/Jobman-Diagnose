#!/usr/bin/env python3
"""Fail because validly parsed data violates a business invariant."""

import sys


order = {"id": "ORD-2048", "sku": "WIDGET-BLUE", "quantity": 12}
inventory = {"WIDGET-BLUE": 4}
available = inventory[order["sku"]]

print(
    f"reserving {order['quantity']} units of {order['sku']} for {order['id']}",
    file=sys.stderr,
)
assert available >= order["quantity"], (
    f"inventory invariant violated for {order['id']}: requested "
    f"{order['quantity']} units but only {available} are available"
)
