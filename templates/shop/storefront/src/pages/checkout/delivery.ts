// POST /checkout/delivery {option_id}: the delivery method.
import type { APIRoute } from "astro"
import { api, ApiError } from "../../lib/medusa"
import { cartIdOf } from "../../lib/store"

export const POST: APIRoute = async ({ request, locals, cookies, redirect }) => {
  const cartId = cartIdOf(cookies)
  if (!cartId) return redirect("/cart", 303)
  const optionId = String((await request.formData()).get("option_id") ?? "")
  try {
    await api(locals.shop, request).post(`/store/carts/${encodeURIComponent(cartId)}/shipping-methods`, { option_id: optionId })
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) return redirect(`/checkout?edit=delivery&error=${encodeURIComponent(e.message)}`, 303)
    throw e
  }
  return redirect("/checkout", 303)
}
