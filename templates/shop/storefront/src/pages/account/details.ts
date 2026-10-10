// POST /account/details {first_name, last_name, phone}: the customer's name and phone.
import type { APIRoute } from "astro"
import { api, ApiError } from "../../lib/medusa"

export const POST: APIRoute = async ({ request, locals, redirect }) => {
  const form = await request.formData()
  const value = (k: string) => String(form.get(k) ?? "").trim()
  if (!value("first_name") || !value("last_name")) return redirect(`/account?error=${encodeURIComponent("Fill in your name.")}#details`, 303)
  try {
    await api(locals.shop, request).post("/store/customers/me", { first_name: value("first_name"), last_name: value("last_name"), phone: value("phone") || null })
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) return redirect("/account", 303)
    if (e instanceof ApiError && e.status < 500) return redirect(`/account?error=${encodeURIComponent(e.message)}#details`, 303)
    throw e
  }
  return redirect("/account#details", 303)
}
