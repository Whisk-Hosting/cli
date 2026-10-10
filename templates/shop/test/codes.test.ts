import { test } from "node:test"
import assert from "node:assert/strict"
import { CODE_MINUTES, MAX_TRIES, PER_EMAIL, PER_IP, check, issue, mayRequest, newCode, newNonce, normalEmail, validEmail } from "../src/lib/codes"

const KEY = "codes-key"
const NOW = 1_800_000_000_000

test("a code is six digits and a nonce is long and random", () => {
  for (let i = 0; i < 200; i++) assert.match(newCode(), /^\d{6}$/)
  assert.notEqual(newNonce(), newNonce())
  assert.ok(newNonce().length >= 24)
})

test("emails are compared trimmed and in lower case", () => {
  assert.equal(normalEmail("  Kiri@Example.NZ "), "kiri@example.nz")
  const cases: [string, boolean][] = [["a@b.nz", true], ["a@b", false], ["a b@c.nz", false], ["", false], [`${"a".repeat(250)}@b.nz`, false]]
  for (const [e, ok] of cases) assert.equal(validEmail(e), ok, e)
})

test("check decides each try", () => {
  const stored = issue(KEY, "kiri@example.nz", "123456", "nonce-1", NOW)
  assert.notEqual(stored.hash, "123456")
  const cases: [string, Parameters<typeof check>, string][] = [
    ["right", [KEY, stored, "kiri@example.nz", "123456", "nonce-1", NOW + 1000], "ok"],
    ["right, other case", [KEY, stored, "KIRI@example.nz", "123456", "nonce-1", NOW], "ok"],
    ["no code", [KEY, null, "kiri@example.nz", "123456", "nonce-1", NOW], "none"],
    ["expired", [KEY, stored, "kiri@example.nz", "123456", "nonce-1", NOW + CODE_MINUTES * 60_000], "expired"],
    ["other browser", [KEY, stored, "kiri@example.nz", "123456", "nonce-2", NOW], "browser"],
    ["wrong", [KEY, stored, "kiri@example.nz", "654321", "nonce-1", NOW], "wrong"],
    ["not digits", [KEY, stored, "kiri@example.nz", "12345x", "nonce-1", NOW], "wrong"],
    ["other address", [KEY, stored, "aroha@example.nz", "123456", "nonce-1", NOW], "wrong"],
    ["other key", ["other", stored, "kiri@example.nz", "123456", "nonce-1", NOW], "browser"],
    ["too many tries", [KEY, { ...stored, tries: MAX_TRIES }, "kiri@example.nz", "123456", "nonce-1", NOW], "tries"],
  ]
  for (const [name, args, want] of cases) {
    const v = check(...args)
    assert.equal(v.ok ? "ok" : v.reason, want, name)
  }
})

test("a wrong code counts a try", () => {
  const stored = issue(KEY, "kiri@example.nz", "123456", "n", NOW)
  const v = check(KEY, stored, "kiri@example.nz", "000000", "n", NOW)
  assert.equal(v.ok, false)
  assert.equal(!v.ok && v.stored?.tries, 1)
})

test("codes are limited per address and per network address", () => {
  assert.equal(mayRequest(0, 0), true)
  assert.equal(mayRequest(PER_EMAIL - 1, 0), true)
  assert.equal(mayRequest(PER_EMAIL, 0), false)
  assert.equal(mayRequest(0, PER_IP), false)
})
