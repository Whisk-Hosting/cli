// POST /cart/update {line_id, quantity}: changes or removes a line (quantity 0 removes it).
import type { APIRoute } from "astro"
import { api, ApiError } from "../../lib/medusa"
import { cartIdOf, updateLine } from "../../lib/store"

export const POST: APIRoute = async ({ request, locals, cookies, redirect }) => {
  const form = await request.formData()
  const cartId = cartIdOf(cookies)
  const lineId = String(form.get("line_id") ?? "")
  const quantity = Math.min(999, Math.max(0, Math.floor(Number(form.get("quantity")) || 0)))
  if (!cartId || !lineId) return redirect("/cart", 303)
  try {
    await updateLine(api(locals.shop, request), cartId, lineId, quantity)
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) return redirect(`/cart?error=${encodeURIComponent(e.message)}`, 303)
    throw e
  }
  return redirect("/cart", 303)
}
