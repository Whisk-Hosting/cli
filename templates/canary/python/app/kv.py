"""The cache (CONTRACT.md section 8).

WHISK_KV_URL is an ordinary Redis URL to a Valkey this app has to itself, so keys need no
prefix and nothing else can read them. It empties when the app sleeps: counters, short caches
and locks, never anything you need back.
"""

import os

import redis.asyncio as redis


async def diag_kv(key: str, value: str | None = None, ttl: int | None = None) -> dict:
    """Write a value, read it back, count a visit, and report how many keys the instance
    holds, which is only ever this app's."""
    url = os.environ.get("WHISK_KV_URL")
    if not url:
        return {"configured": False}
    client = redis.from_url(url, decode_responses=True, socket_timeout=5)
    try:
        out: dict = {"configured": True, "key": key}
        if value:
            if ttl and ttl > 0:
                await client.set(key, value, ex=ttl)
                out["ttl_seconds"] = ttl
            else:
                await client.set(key, value)
        out["value"] = await client.get(key)
        out["visits"] = await client.incr("diag:visits")
        out["keys"] = await client.dbsize()
        return out
    except Exception as err:  # a cache is never worth failing a request for
        return {"configured": True, "error": str(err)}
    finally:
        await client.aclose()
