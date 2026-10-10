import { test } from "node:test"
import assert from "node:assert/strict"
import { amountString, authHeader, baseUrl, completeRequest, hostedPageUrl, refundRequest, sessionRequest, statusOf, voidRequest, type Transaction } from "../src/lib/windcave"

test("addresses and amounts", () => {
  assert.equal(baseUrl("live"), "https://sec.windcave.com/api/v1")
  assert.equal(baseUrl("test"), "https://uat.windcave.com/api/v1")
  assert.equal(authHeader("user", "key"), `Basic ${Buffer.from("user:key").toString("base64")}`)
  assert.equal(amountString(55, "nzd"), "55.00")
  assert.equal(amountString(12.5, "NZD"), "12.50")
})

test("a session pays the Medusa payment session and comes back to the shop", () => {
  const r = sessionRequest({ amount: 55, currency: "nzd", paymentSessionId: "payses_1", publicUrl: "https://kiwi.whisk.page/", capture: true })
  assert.equal(r.type, "purchase")
  assert.equal(r.currency, "NZD")
  assert.equal(r.amount, "55.00")
  assert.equal(r.merchantReference, "payses_1")
  assert.equal(r.callbackUrls.approved, "https://kiwi.whisk.page/checkout/return/windcave?result=approved")
  assert.equal(r.notificationUrl, "https://kiwi.whisk.page/hooks/windcave")
  assert.equal(sessionRequest({ amount: 1, currency: "nzd", paymentSessionId: "p", publicUrl: "https://x", capture: false }).type, "auth")
})

test("statusOf reads a session", () => {
  const tx = (t: Partial<Transaction>): Transaction => ({ id: "t", authorised: false, type: "purchase", amount: "55.00", currency: "NZD", ...t })
  const cases: [Parameters<typeof statusOf>[0], string][] = [
    [{ id: "s" }, "pending"],
    [{ id: "s", transactions: [tx({ authorised: true })] }, "captured"],
    [{ id: "s", transactions: [tx({ authorised: true, type: "auth" })] }, "authorized"],
    [{ id: "s", transactions: [tx({})] }, "error"],
    [{ id: "s", transactions: [tx({}), tx({ authorised: true, id: "t2" })] }, "captured"],
    [{ id: "s", state: "cancelled" }, "canceled"],
    [{ id: "s", state: "expired" }, "canceled"],
  ]
  for (const [s, want] of cases) assert.equal(statusOf(s).status, want, JSON.stringify(s))
})

test("hosted page, complete, refund and void", () => {
  assert.equal(hostedPageUrl({ id: "s", links: [{ rel: "self", href: "a" }, { rel: "hpp", href: "https://pay" }] }), "https://pay")
  assert.equal(hostedPageUrl({ id: "s" }), null)
  const t: Transaction = { id: "t1", authorised: true, type: "auth", amount: "55.00", currency: "NZD" }
  assert.deepEqual(completeRequest(t), { type: "complete", amount: "55.00", currency: "NZD", transactionId: "t1" })
  assert.deepEqual(refundRequest(t, 10), { type: "refund", amount: "10.00", currency: "NZD", transactionId: "t1" })
  assert.deepEqual(voidRequest(t), { type: "void", transactionId: "t1" })
})
