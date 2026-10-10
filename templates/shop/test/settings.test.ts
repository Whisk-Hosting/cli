import { test } from "node:test"
import assert from "node:assert/strict"
import { browserKeysFrom, databaseFrom, paymentProvidersFrom, providerIds, secretsFrom, tradeOrdering } from "../src/lib/settings"

test("databaseFrom hands the driver what libpq means by sslmode", () => {
  const cases: [string, string, unknown][] = [
    ["postgres://u:p@db:5432/app?sslmode=require", "postgres://u:p@db:5432/app", { rejectUnauthorized: false }],
    ["postgres://u:p@db:5432/app?sslmode=verify-full", "postgres://u:p@db:5432/app", { rejectUnauthorized: true }],
    ["postgres://u:p@db:5432/app?sslmode=disable", "postgres://u:p@db:5432/app", false],
    ["postgres://u:p@db:5432/app", "postgres://u:p@db:5432/app", false],
    ["postgres://u:p@db/app?sslmode=prefer&application_name=x", "postgres://u:p@db/app?application_name=x", { rejectUnauthorized: false }],
  ]
  for (const [raw, url, ssl] of cases) {
    const db = databaseFrom(raw)
    assert.equal(db.url, url, raw)
    assert.deepEqual((db.driverOptions as any).connection.ssl, ssl, raw)
  }
})

test("secretsFrom derives three different secrets from the app's key", () => {
  const key = Buffer.alloc(32, 7).toString("base64")
  const s = secretsFrom({ WHISK_DELIVERY_KEY: key })
  assert.equal(new Set([s.jwt, s.cookie, s.codes]).size, 3)
  assert.ok(![s.jwt, s.cookie, s.codes].some((x) => x.includes(key)))
  assert.deepEqual(secretsFrom({ WHISK_DELIVERY_KEY: key }), s, "the same key gives the same secrets")
  assert.notEqual(secretsFrom({ WHISK_DELIVERY_KEY: Buffer.alloc(32, 8).toString("base64") }).jwt, s.jwt)
})

test("secretsFrom stops a server without the key, and lets a build, the migrate step or a laptop through", () => {
  assert.equal(secretsFrom({ SHOP_STEP: "migrate" }).jwt, "development-only")
  assert.throws(() => secretsFrom({ SHOP_STEP: "start" }), /WHISK_DELIVERY_KEY/)
  assert.throws(() => secretsFrom({}), /WHISK_DELIVERY_KEY/)
  assert.throws(() => secretsFrom({ WHISK_DELIVERY_KEY: "c2hvcnQ=" }), /WHISK_DELIVERY_KEY/)
  assert.equal(secretsFrom({}, true).jwt, "development-only")
  assert.equal(secretsFrom({ WHISK_DEV: "1" }).cookie, "development-only")
  assert.equal(secretsFrom({ NODE_ENV: "test" }).codes, "development-only")
})

test("paymentProvidersFrom turns on each provider whose keys are set", () => {
  const cases: [Record<string, string>, string[]][] = [
    [{}, []],
    [{ STRIPE_API_KEY: "sk" }, ["stripe"]],
    [{ WINDCAVE_USERNAME: "u" }, []],
    [{ WINDCAVE_USERNAME: "u", WINDCAVE_API_KEY: "k" }, ["windcave"]],
    [{ PAYPAL_CLIENT_ID: "id" }, []],
    [{ STRIPE_API_KEY: "sk", WINDCAVE_USERNAME: "u", WINDCAVE_API_KEY: "k", PAYPAL_CLIENT_ID: "i", PAYPAL_CLIENT_SECRET: "s" }, ["stripe", "windcave", "paypal"]],
  ]
  for (const [env, ids] of cases) assert.deepEqual(paymentProvidersFrom(env).map((p) => p.id), ids, JSON.stringify(env))
})

test("payment providers capture at once unless told to wait, and are live only when told", () => {
  const all = { STRIPE_API_KEY: "sk", WINDCAVE_USERNAME: "u", WINDCAVE_API_KEY: "k", PAYPAL_CLIENT_ID: "i", PAYPAL_CLIENT_SECRET: "s" }
  assert.ok(paymentProvidersFrom(all).every((p) => p.options.capture === true))
  const manual = paymentProvidersFrom({ ...all, STRIPE_CAPTURE: "manual", WINDCAVE_CAPTURE: "manual", PAYPAL_CAPTURE: "manual" })
  assert.ok(manual.every((p) => p.options.capture === false))
  const opts = (env: Record<string, string>, id: string) => paymentProvidersFrom({ ...all, ...env }).find((p) => p.id === id)!.options
  assert.equal(opts({}, "windcave").environment, "test")
  assert.equal(opts({ WINDCAVE_ENVIRONMENT: "live" }, "windcave").environment, "live")
  assert.equal(opts({}, "paypal").environment, "sandbox")
  assert.equal(opts({ PAYPAL_ENVIRONMENT: "live" }, "paypal").environment, "live")
})

test("providerIds names bank transfer first, then each provider as Medusa registers it", () => {
  assert.deepEqual(providerIds([]), ["pp_system_default"])
  assert.deepEqual(
    providerIds(paymentProvidersFrom({ STRIPE_API_KEY: "sk", PAYPAL_CLIENT_ID: "i", PAYPAL_CLIENT_SECRET: "s" })),
    ["pp_system_default", "pp_stripe_stripe", "pp_paypal_paypal"],
  )
  assert.deepEqual(providerIds(paymentProvidersFrom({}, true)), ["pp_system_default", "pp_account_whisk"])
})

test("trade ordering is on when the build installed it", () => {
  assert.equal(tradeOrdering(() => "/app/node_modules/@whisk/shop-b2b/package.json"), true)
  assert.equal(tradeOrdering(() => {
    throw new Error("Cannot find module")
  }), false)
  assert.equal(tradeOrdering(), false)
})

test("browserKeysFrom hands the pages only the public keys", () => {
  assert.deepEqual(browserKeysFrom({}), { stripe: null, paypal: null })
  assert.deepEqual(browserKeysFrom({ STRIPE_PUBLISHABLE_KEY: "pk", PAYPAL_CLIENT_ID: "id", STRIPE_API_KEY: "sk" }), { stripe: "pk", paypal: "id" })
})
