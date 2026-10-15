import { Redis } from "ioredis";

// The cache (CONTRACT.md section 8). WHISK_KV_URL is an ordinary Redis URL to a Valkey this app
// has to itself, so keys need no prefix and nothing else can read them. It empties when the app
// sleeps: counters, short caches and locks, never anything you need back.

export type KVResult = {
  configured: boolean;
  key?: string;
  value?: string | null;
  ttl_seconds?: number;
  visits?: number;
  keys?: number;
  error?: string;
};

// diagKV is the diagnostic the platform's canary drives: write a value, read it back, count a
// visit, and report how many keys the instance holds, which is only ever this app's.
export const diagKV = async (key: string, value?: string, ttl?: number): Promise<KVResult> => {
  const url = process.env.WHISK_KV_URL;
  if (!url) return { configured: false };
  const redis = new Redis(url, { maxRetriesPerRequest: 2, connectTimeout: 5000 });
  try {
    const out: KVResult = { configured: true, key };
    if (value !== undefined && value !== "") {
      if (ttl && ttl > 0) {
        await redis.set(key, value, "EX", ttl);
        out.ttl_seconds = ttl;
      } else {
        await redis.set(key, value);
      }
    }
    out.value = await redis.get(key);
    out.visits = await redis.incr("diag:visits");
    out.keys = await redis.dbsize();
    return out;
  } catch (err) {
    return { configured: true, error: err instanceof Error ? err.message : String(err) };
  } finally {
    redis.disconnect();
  }
};
