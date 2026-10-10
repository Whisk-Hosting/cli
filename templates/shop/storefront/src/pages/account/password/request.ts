// POST /account/password/request {email}: emails a link to set a new password. It answers the
// same whether or not the address has an account.
import type { APIRoute } from "astro"
import { api, ApiError } from "../../../lib/medusa"

export const POST: APIRoute = async ({ request, locals, redirect }) => {
  const form = await request.formData()
  const email = String(form.get("email") ?? "").trim().toLowerCase()
  try {
    await api(locals.shop, request).post("/auth/customer/emailpass/reset-password", { identifier: email })
  } catch (e) {
    if (e instanceof ApiError && e.status === 429) return redirect(`/account/password?error=${encodeURIComponent(e.message)}`, 303)
    if (!(e instanceof ApiError) || e.status >= 500) throw e
  }
  return redirect("/account/password?sent=1", 303)
}
