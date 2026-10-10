import { test } from "node:test"
import assert from "node:assert/strict"
import { amountOf, baseUrl, orderIdOfEvent, orderRequest, sessionIdOf, statusOf, value, type Order } from "../src/lib/paypal"

test("addresses and amounts", () => {
  assert.equal(baseUrl("live"), "https://api-m.paypal.com")
  assert.equal(baseUrl("sandbox"), "https://api-m.sandbox.paypal.com")
  assert.equal(value(55, "nzd"), "55.00")
})

test("an order carries the Medusa payment session's id", () => {
  const r = orderRequest({ amount: 55, currency: "nzd", paymentSessionId: "payses_1", capture: true })
  assert.equal(r.intent, "CAPTURE")
  assert.deepEqual(r.purchase_units[0].amount, { currency_code: "NZD", value: "55.00" })
  const o: Order = { id: "o", status: "CREATED", purchase_units: [{ custom_id: "payses_1", amount: { currency_code: "NZD", value: "55.00" } }] }
  assert.equal(sessionIdOf(o), "payses_1")
  assert.equal(amountOf(o), 55)
  assert.equal(orderRequest({ amount: 1, currency: "nzd", paymentSessionId: "p", capture: false }).intent, "AUTHORIZE")
})

test("statusOf reads an order", () => {
  const with_ = (status: string, payments?: object): Order => ({ id: "o", status, purchase_units: [{ payments }] as any })
  const cases: [Order, string][] = [
    [with_("CREATED"), "requires_more"],
    [with_("PAYER_ACTION_REQUIRED"), "requires_more"],
    [with_("APPROVED"), "pending"],
    [with_("COMPLETED", { captures: [{ id: "c", status: "COMPLETED" }] }), "captured"],
    [with_("COMPLETED", { captures: [{ id: "c", status: "PENDING" }] }), "pending"],
    [with_("COMPLETED", { authorizations: [{ id: "a", status: "CREATED" }] }), "authorized"],
    [with_("COMPLETED", { captures: [{ id: "c", status: "DECLINED" }] }), "error"],
    [with_("VOIDED"), "canceled"],
  ]
  for (const [o, want] of cases) assert.equal(statusOf(o), want, JSON.stringify(o))
})

test("a webhook event names its order", () => {
  assert.equal(orderIdOfEvent({ resource_type: "checkout-order", resource: { id: "o1" } }), "o1")
  assert.equal(orderIdOfEvent({ resource_type: "capture", resource: { supplementary_data: { related_ids: { order_id: "o2" } } } }), "o2")
  assert.equal(orderIdOfEvent({ resource_type: "capture", resource: {} }), null)
  assert.equal(orderIdOfEvent({}), null)
})
