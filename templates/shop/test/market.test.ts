import { test } from "node:test"
import assert from "node:assert/strict"
import { marketFrom, taxLabel } from "../src/lib/market"

test("marketFrom", () => {
  const cases: [Record<string, string | undefined>, Partial<ReturnType<typeof marketFrom>>][] = [
    [{}, { country: "nz", currency: "nzd", taxRate: 15, taxInclusive: true, locale: "en-NZ" }],
    [{ SHOP_COUNTRY: "AU" }, { country: "au", currency: "aud", taxRate: 10, taxInclusive: true, timeZone: "Australia/Sydney" }],
    [{ SHOP_COUNTRY: "au", SHOP_PRICES_INCLUDE_TAX: "false", SHOP_TIME_ZONE: "Australia/Perth" }, { taxInclusive: false, timeZone: "Australia/Perth" }],
    [{ SHOP_PRICES_INCLUDE_TAX: "maybe" }, { taxInclusive: true }],
  ]
  for (const [env, want] of cases) {
    const got = marketFrom(env)
    for (const [k, v] of Object.entries(want)) assert.equal((got as any)[k], v, `${JSON.stringify(env)} ${k}`)
  }
  assert.throws(() => marketFrom({ SHOP_COUNTRY: "us" }), /not one the shop sells in/)
})

test("taxLabel", () => {
  assert.equal(taxLabel({ taxName: "GST", taxInclusive: true }), "Includes GST")
  assert.equal(taxLabel({ taxName: "GST", taxInclusive: false }), "GST")
})
