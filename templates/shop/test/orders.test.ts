import { test } from "node:test"
import assert from "node:assert/strict"
import { bankAccountOf, displayId, orderView, paymentMethodOf, providerOf, type OrderLike } from "../src/lib/orders"
import { money, templates } from "../src/lib/emails"

const order: OrderLike = {
  id: "order_1",
  display_id: 42,
  email: "kiri@example.nz",
  currency_code: "nzd",
  metadata: { po_number: "PO-7" },
  item_total: 45,
  shipping_total: 10,
  discount_total: 0,
  tax_total: { numeric: 7.17 },
  total: "55",
  items: [{ title: "Merino Beanie", variant_title: "Charcoal", quantity: 1, total: 45 }],
  shipping_methods: [{ name: "Standard delivery" }],
  shipping_address: { first_name: "Kiri", last_name: "Tane", address_1: "1 Queen Street", city: "Auckland", postal_code: "1010", country_code: "nz" },
  payment_collections: [{ payment_sessions: [{ provider_id: "pp_system_default" }] }],
}
const shop = { name: "Kiwi Goods", url: "https://kiwi.whisk.page" }
const bank = { accountName: "Kiwi Goods Ltd", accountNumber: "12-3456-7890123-00" }

test("payment methods by provider", () => {
  const cases: [string, string][] = [
    ["pp_system_default", "bank_transfer"], ["pp_stripe_stripe", "card"], ["pp_windcave_windcave", "card"], ["pp_paypal_paypal", "paypal"], ["pp_account_account", "account"], ["pp_other", "other"], ["", "other"],
  ]
  for (const [id, want] of cases) assert.equal(paymentMethodOf(id), want, id)
  assert.equal(providerOf({ ...order, payment_collections: [{ payment_sessions: [{ provider_id: "pp_a" }], payments: [{ provider_id: "pp_b" }] }] }), "pp_b")
  assert.equal(displayId(order as any), "#42")
  assert.equal(displayId({ id: "order_1" }), "order_1")
})

test("the bank account comes from the store's metadata, both parts or nothing", () => {
  assert.deepEqual(bankAccountOf({ bank_account_name: " Kiwi Goods Ltd ", bank_account_number: "12-3456-7890123-00" }), bank)
  assert.equal(bankAccountOf({ bank_account_name: "Kiwi Goods Ltd" }), null)
  assert.equal(bankAccountOf({ bank_account_name: "", bank_account_number: "1" }), null)
  assert.equal(bankAccountOf(null), null)
})

test("orderView shapes an order for its emails", () => {
  const v = orderView(order, shop, bank)
  assert.equal(v.displayId, "#42")
  assert.equal(v.tax, 7.17)
  assert.equal(v.total, 55)
  assert.equal(v.paymentMethod, "bank_transfer")
  assert.deepEqual(v.bankTransfer, { ...bank, reference: "42" })
  assert.equal(v.poNumber, "PO-7")
  assert.deepEqual(v.shippingAddress, { name: "Kiri Tane", company: null, lines: ["1 Queen Street", "Auckland 1010", "NZ"] })
  assert.equal(orderView(order, shop, null).bankTransfer, null)
  assert.equal(orderView({ ...order, payment_collections: [{ payments: [{ provider_id: "pp_stripe_stripe" }] }] }, shop, bank).bankTransfer, null)
})

test("the order email says how to pay by bank transfer, and escapes what customers typed", () => {
  const v = orderView({ ...order, items: [{ title: "<script>x</script>", quantity: 2, total: 45 }] }, shop, bank)
  const m = templates.orderPlaced(v)
  assert.equal(m.subject, "Order #42 confirmed")
  assert.match(m.text, /12-3456-7890123-00/)
  assert.match(m.text, /reference 42/)
  assert.match(m.html, /PO PO-7/)
  assert.doesNotMatch(m.html, /<script>/)
  assert.match(m.html, /&lt;script&gt;/)
})

test("every email renders a subject, html and text", () => {
  const data = {
    orderPlaced: orderView(order, shop, bank),
    orderShipped: { shop: "Kiwi Goods", shopUrl: shop.url, displayId: "#42", tracking: [{ number: "NZ123", url: "https://track/NZ123" }] },
    orderCanceled: { shop: "Kiwi Goods", shopUrl: shop.url, displayId: "#42" },
    refunded: { shop: "Kiwi Goods", shopUrl: shop.url, displayId: "#42", amount: 10, currency: "nzd" },
    signInCode: { shop: "Kiwi Goods", shopUrl: shop.url, code: "123456", minutes: 10 },
    passwordReset: { shop: "Kiwi Goods", shopUrl: shop.url, link: "https://kiwi.whisk.page/account/password?token=t", minutes: 15 },
    rendered: { subject: "An order waits for you", html: "<p>Kiwi Goods</p>", text: "Kiwi Goods" },
  }
  for (const [name, render] of Object.entries(templates)) {
    const m = (render as (d: unknown) => { subject: string; html: string; text: string })(data[name as keyof typeof data])
    assert.ok(m.subject && m.html.includes("Kiwi Goods") && m.text, name)
  }
  assert.match(templates.signInCode(data.signInCode).text, /123456/)
  assert.equal(money(55, "nzd"), "$55.00")
})

test("an order whose prices are before tax shows them so, with the tax above the total", () => {
  const before = {
    ...order, item_total: 1163.8, item_subtotal: 1058, shipping_total: 16.5, shipping_subtotal: 15, tax_total: 107.3, total: 1180.3,
    items: [{ title: "Breathalyser", quantity: 1, total: 1163.8, subtotal: 1058, is_tax_inclusive: false }],
  }
  const v = orderView(before, shop, bank)
  assert.deepEqual([v.lines[0].total, v.subtotal, v.shipping, v.tax, v.total, v.taxIncluded], [1058, 1058, 15, 107.3, 1180.3, false])
  const text = templates.orderPlaced(v).text
  assert.ok(text.indexOf("GST") < text.indexOf("Total"), text)
  assert.ok(!text.includes("Includes GST"), text)
  const included = orderView(order, shop, bank)
  assert.equal(included.subtotal, 45)
  assert.ok(templates.orderPlaced(included).text.includes("Includes GST"))
})
