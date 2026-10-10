// POST /cart/add {variant_id, quantity, return}: puts a product in the cart.
import type { APIRoute } from "astro"
import { api, ApiError } from "../../lib/medusa"
import { addLine, ensureCart } from "../../lib/store"
import { safeReturn } from "../../lib/format"

export const POST: APIRoute = async ({ request, locals, cookies, redirect }) => {
  const form = await request.formData()
  const back = safeReturn(String(form.get("return") ?? ""), "/cart")
  const variantId = String(form.get("variant_id") ?? "")
  const quantity = Math.min(999, Math.max(1, Math.floor(Number(form.get("quantity")) || 1)))
  const a = api(locals.shop, request)
  const join = (path: string, k: string, v: string) => `${path}${path.includes("?") ? "&" : "?"}${k}=${encodeURIComponent(v)}`
  if (!variantId) return redirect(join(back, "error", "Choose an option first."), 303)
  try {
    const cart = await ensureCart(a, locals.shop, cookies)
    await addLine(a, cart.id, variantId, quantity)
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) return redirect(join(back, "error", e.message), 303)
    throw e
  }
  return redirect(join(back.replace(/[?&](added|error)=[^&]*/g, ""), "added", "1"), 303)
}
