#!/usr/bin/env python3
"""Fail with two concurrent exceptions in an asyncio TaskGroup."""

import asyncio
import sys


async def fail_after_release(
    release: asyncio.Event,
    error: Exception,
) -> None:
    await release.wait()
    raise error


async def reconcile_accounts() -> None:
    release = asyncio.Event()
    async with asyncio.TaskGroup() as group:
        group.create_task(
            fail_after_release(release, LookupError("customer C-1042 was not found"))
        )
        group.create_task(
            fail_after_release(
                release,
                ValueError("invoice INV-778 has a negative settlement amount"),
            )
        )
        await asyncio.sleep(0)
        release.set()


print("reconciling customer and invoice records concurrently", file=sys.stderr)
asyncio.run(reconcile_accounts())
