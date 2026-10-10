// Emailed sign-in codes, as pure decisions. A code is six digits, works for ten minutes, allows
// five wrong tries, and only in the browser that asked for it (its nonce cookie). Codes are kept
// hashed; the store (src/lib/kv.ts) holds what these functions return.
import { createHmac, randomInt, randomBytes, timingSafeEqual } from "node:crypto"

export const CODE_MINUTES = 10
export const MAX_TRIES = 5
// At most this many codes to one address in ten minutes, and from one network address in an
// hour, so the shop cannot be used to flood someone's inbox.
export const PER_EMAIL = 3
export const PER_IP = 20

export type Stored = { hash: string; nonce: string; expires: number; tries: number }

export const normalEmail = (email: string) => email.trim().toLowerCase()

// A plausible address: something@something.tld, no spaces, at most 254 characters.
export const validEmail = (email: string) => email.length <= 254 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)

export const newCode = () => String(randomInt(0, 1_000_000)).padStart(6, "0")
export const newNonce = () => randomBytes(18).toString("base64url")

const hashOf = (key: string, email: string, code: string) => createHmac("sha256", key).update(`${normalEmail(email)}\n${code}`).digest("hex")

export const issue = (key: string, email: string, code: string, nonce: string, now: number): Stored => ({
  hash: hashOf(key, email, code),
  nonce: createHmac("sha256", key).update(nonce).digest("hex"),
  expires: now + CODE_MINUTES * 60_000,
  tries: 0,
})

const same = (a: string, b: string) => {
  const x = Buffer.from(a)
  const y = Buffer.from(b)
  return x.length === y.length && timingSafeEqual(x, y)
}

export type Verdict = { ok: true } | { ok: false; reason: "none" | "expired" | "tries" | "browser" | "wrong"; stored?: Stored }

// check decides a try. A wrong code answers the stored record with one more try counted, for
// the caller to save; every other refusal leaves nothing to save but a deletion.
export const check = (key: string, stored: Stored | null, email: string, code: string, nonce: string, now: number): Verdict => {
  if (!stored) return { ok: false, reason: "none" }
  if (stored.expires <= now) return { ok: false, reason: "expired" }
  if (stored.tries >= MAX_TRIES) return { ok: false, reason: "tries" }
  if (!same(stored.nonce, createHmac("sha256", key).update(nonce).digest("hex"))) return { ok: false, reason: "browser" }
  if (!/^\d{6}$/.test(code) || !same(stored.hash, hashOf(key, email, code))) return { ok: false, reason: "wrong", stored: { ...stored, tries: stored.tries + 1 } }
  return { ok: true }
}

export const mayRequest = (sentToEmail: number, sentFromIp: number) => sentToEmail < PER_EMAIL && sentFromIp < PER_IP
