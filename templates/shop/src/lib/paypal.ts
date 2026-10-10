// PayPal's Orders API, as pure functions: the requests the shop makes and how it reads the
// answers. The provider in src/modules/paypal sends them. Reference:
// https://developer.paypal.com/docs/api/orders/v2/

export type PaypalEnv = "sandbox" | "live"

export const baseUrl = (env: PaypalEnv) => (env === "live" ? "https://api-m.paypal.com" : "https://api-m.sandbox.paypal.com")

const DECIMALS: Record<string, number> = { jpy: 0, huf: 0, twd: 0 }
export const value = (amount: number, currency: string) => amount.toFixed(DECIMALS[currency.toLowerCase()] ?? 2)

export type OrderRequest = {
  intent: "CAPTURE" | "AUTHORIZE"
  purchase_units: { reference_id: string; custom_id: string; amount: { currency_code: string; value: string } }[]
}

// orderRequest is one PayPal order for one Medusa payment session, whose id rides in
// custom_id so a notification can find it.
export const orderRequest = (input: { amount: number; currency: string; paymentSessionId: string; capture: boolean }): OrderRequest => ({
  intent: input.capture ? "CAPTURE" : "AUTHORIZE",
  purchase_units: [
    {
      reference_id: "default",
      custom_id: input.paymentSessionId,
      amount: { currency_code: input.currency.toUpperCase(), value: value(input.amount, input.currency) },
    },
  ],
})

export type Money = { currency_code: string; value: string }
export type Capture = { id: string; status: string; amount?: Money }
export type Authorization = { id: string; status: string; amount?: Money }
export type Order = {
  id: string
  status: string
  intent?: string
  purchase_units?: {
    custom_id?: string
    amount?: Money
    payments?: { captures?: Capture[]; authorizations?: Authorization[] }
  }[]
}

export type Status = "authorized" | "captured" | "pending" | "requires_more" | "error" | "canceled"

const unit = (o: Order) => o.purchase_units?.[0]

export const capturesOf = (o: Order) => unit(o)?.payments?.captures ?? []
export const authorizationsOf = (o: Order) => unit(o)?.payments?.authorizations ?? []
export const sessionIdOf = (o: Order) => unit(o)?.custom_id ?? null
export const amountOf = (o: Order) => Number(unit(o)?.amount?.value ?? 0)

// statusOf reads an order. APPROVED means the buyer has said yes and the shop has not yet
// captured or authorised it, which authorizePayment does next.
export const statusOf = (o: Order): Status => {
  if (capturesOf(o).some((c) => c.status === "COMPLETED")) return "captured"
  if (capturesOf(o).some((c) => c.status === "PENDING")) return "pending"
  if (authorizationsOf(o).some((a) => a.status === "CREATED" || a.status === "CAPTURED")) return "authorized"
  switch (o.status) {
    case "APPROVED":
      return "pending"
    case "CREATED":
    case "SAVED":
    case "PAYER_ACTION_REQUIRED":
      return "requires_more"
    case "VOIDED":
      return "canceled"
    case "COMPLETED":
      return "error"
    default:
      return "pending"
  }
}

// The order a webhook event is about: the order itself, or the order a capture or
// authorisation belongs to.
export const orderIdOfEvent = (event: { resource_type?: string; resource?: Record<string, any> }): string | null => {
  const r = event.resource ?? {}
  if (event.resource_type === "checkout-order") return typeof r.id === "string" ? r.id : null
  const related = r.supplementary_data?.related_ids?.order_id
  return typeof related === "string" ? related : null
}
