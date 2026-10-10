// POST /checkout/details: the customer's email and delivery address on the cart.
import type { APIRoute } from "astro"
import { api, ApiError } from "../../lib/medusa"
import { cartIdOf, me } from "../../lib/store"
import { addressFrom } from "../../lib/format"

const back = (message: string) => `/checkout?edit=details&error=${encodeURIComponent(message)}`

export const POST: APIRoute = async ({ request, locals, cookies, redirect }) => {
  const cartId = cartIdOf(cookies)
  if (!cartId) return redirect("/cart", 303)
  const form = await request.formData()
  const a = api(locals.shop, request)
  const email = String(form.get("email") ?? "").trim().toLowerCase()
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return redirect(back("Check your email address."), 303)

  const customer = await me(a)
  const savedId = String(form.get("saved_address_id") ?? "")
  const saved = savedId ? (customer?.addresses ?? []).find((x: any) => x.id === savedId) : null
  const parsed = saved ? { address: saved } : addressFrom(form)
  if ("missing" in parsed) return redirect(back("Fill in your name and full address."), 303)
  const { id: _id, customer_id: _c, created_at: _ca, updated_at: _ua, deleted_at: _da, address_name: _n, is_default_billing: _b, is_default_shipping: _s, metadata: _m, ...address } = parsed.address as any
  try {
    await a.post(`/store/carts/${encodeURIComponent(cartId)}`, { email, shipping_address: address, billing_address: address })
    if (customer && !saved && form.get("save_address") === "1") {
      await a.post("/store/customers/me/addresses", { ...address, is_default_shipping: (customer.addresses ?? []).length === 0 })
    }
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) return redirect(back(e.message), 303)
    throw e
  }
  return redirect("/checkout", 303)
}
