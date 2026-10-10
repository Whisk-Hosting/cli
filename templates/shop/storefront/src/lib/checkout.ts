// Checkout against the store API: details, delivery, payment, then the order.
import type { AstroCookies } from "astro"
import { api, ApiError, query, type ShopContext } from "./medusa"
import { dropCart } from "./store"

type Api = ReturnType<typeof api>

export const shippingOptions = async (a: Api, cartId: string) =>
  (await a.get<{ shipping_options: any[] }>(`/store/shipping-options${query({ cart_id: cartId })}`)).shipping_options

export type Method = { id: string; label: string; kind: "bank_transfer" | "stripe" | "windcave" | "paypal" }

// The ways to pay this shop offers: those the region takes and whose keys are set, and bank
// transfer only once the business has given its account.
export const paymentMethods = (shop: ShopContext): Method[] => {
  const has = (id: string) => shop.providers.includes(id)
  const out: Method[] = []
  if (has("pp_stripe_stripe") && shop.stripeKey) out.push({ id: "pp_stripe_stripe", label: "Card, Apple Pay, Google Pay or Afterpay", kind: "stripe" })
  if (has("pp_windcave_windcave")) out.push({ id: "pp_windcave_windcave", label: "Credit or debit card", kind: "windcave" })
  if (has("pp_paypal_paypal") && shop.paypalClientId) out.push({ id: "pp_paypal_paypal", label: "PayPal", kind: "paypal" })
  if (has("pp_system_default") && shop.bank) out.push({ id: "pp_system_default", label: "Bank transfer", kind: "bank_transfer" })
  return out
}

// The payment session for a provider, made (or remade for a changed cart) on the cart's
// payment collection.
export const startPayment = async (a: Api, cart: any, providerId: string) => {
  const collectionId =
    cart.payment_collection?.id ??
    (await a.post<{ payment_collection: any }>("/store/payment-collections", { cart_id: cart.id })).payment_collection.id
  const { payment_collection } = await a.post<{ payment_collection: any }>(
    `/store/payment-collections/${encodeURIComponent(collectionId)}/payment-sessions`,
    { provider_id: providerId },
  )
  return (payment_collection.payment_sessions ?? []).find((s: any) => s.provider_id === providerId) ?? null
}

export const activeSession = (cart: any) =>
  (cart?.payment_collection?.payment_sessions ?? []).find((s: any) => s.status !== "canceled") ?? null

export type Completed = { order: any } | { error: string }

// Places the order. On success the cart is gone; on a refusal (a declined card, stock that ran
// out) the cart stays for the customer to try again.
export const complete = async (a: Api, cookies: AstroCookies, cartId: string): Promise<Completed> => {
  try {
    const out = await a.post<{ type: string; order?: any; error?: { message: string } }>(`/store/carts/${encodeURIComponent(cartId)}/complete`, {})
    if (out.type === "order" && out.order) {
      dropCart(cookies)
      return { order: out.order }
    }
    return { error: out.error?.message ?? "The payment didn't go through." }
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) return { error: e.message }
    throw e
  }
}
