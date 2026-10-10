// Short-lived state (sign-in codes, rate-limit counters) in the app's own Valkey. Nothing here
// must survive a restart: a lost code is asked for again. Without WHISK_KV_URL (a laptop with no
// cache) it is kept in memory.
import Redis from "ioredis"

type Store = {
  get(key: string): Promise<string | null>
  set(key: string, value: string, ttlSeconds: number): Promise<void>
  del(key: string): Promise<void>
  incr(key: string, ttlSeconds: number): Promise<number>
}

const memory = (): Store => {
  const m = new Map<string, { v: string; until: number }>()
  const live = (k: string) => {
    const e = m.get(k)
    if (e && e.until <= Date.now()) m.delete(k)
    return m.get(k)
  }
  return {
    get: async (k) => live(k)?.v ?? null,
    set: async (k, v, ttl) => void m.set(k, { v, until: Date.now() + ttl * 1000 }),
    del: async (k) => void m.delete(k),
    incr: async (k, ttl) => {
      const e = live(k)
      const n = Number(e?.v ?? 0) + 1
      m.set(k, { v: String(n), until: e?.until ?? Date.now() + ttl * 1000 })
      return n
    },
  }
}

const valkey = (url: string): Store => {
  const r = new Redis(url, { maxRetriesPerRequest: 2, connectTimeout: 5_000, commandTimeout: 3_000, lazyConnect: false })
  return {
    get: (k) => r.get(k),
    set: async (k, v, ttl) => void (await r.set(k, v, "EX", ttl)),
    del: async (k) => void (await r.del(k)),
    incr: async (k, ttl) => {
      const [[, n]] = (await r.multi().incr(k).expire(k, ttl, "NX").exec()) as [[unknown, number]]
      return n
    },
  }
}

let store: Store | undefined
export const kv = (): Store => (store ??= process.env.WHISK_KV_URL ? valkey(process.env.WHISK_KV_URL) : memory())

export const getJson = async <T>(key: string): Promise<T | null> => {
  const v = await kv().get(key)
  return v ? (JSON.parse(v) as T) : null
}
export const setJson = (key: string, value: unknown, ttlSeconds: number) => kv().set(key, JSON.stringify(value), ttlSeconds)
