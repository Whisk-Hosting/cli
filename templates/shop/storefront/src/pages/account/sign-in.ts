// POST /account/sign-in {email, password, return}: signs in with a password. Medusa answers a
// token, which is exchanged for the session cookie the rest of the shop uses.
import type { APIRoute } from "astro"
import { api, ApiError, redirectWith } from "../../lib/medusa"
import { claimCart } from "../../lib/store"
import { safeReturn } from "../../lib/format"
import { signIn } from "../../lib/account"

export const POST: APIRoute = async ({ request, locals, cookies }) => {
  const form = await request.formData()
  const email = String(form.get("email") ?? "").trim().toLowerCase()
  const password = String(form.get("password") ?? "")
  const ret = safeReturn(String(form.get("return") ?? ""), "/account")
  const a = api(locals.shop, request)
  try {
    await signIn(a, email, password)
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) {
      return redirectWith(a.call, `/account?${new URLSearchParams({ return: ret, step: "password", email, error: "Wrong email or password." })}`)
    }
    throw e
  }
  await claimCart(a, cookies)
  return redirectWith(a.call, ret)
}
