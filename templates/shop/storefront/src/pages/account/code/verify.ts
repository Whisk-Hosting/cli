// POST /account/code/verify {email, code, return}: signs the customer in with the code they were
// emailed, then makes the cart they were filling theirs.
import type { APIRoute } from "astro"
import { api, ApiError, redirectWith } from "../../../lib/medusa"
import { claimCart } from "../../../lib/store"
import { safeReturn } from "../../../lib/format"

export const POST: APIRoute = async ({ request, locals, cookies }) => {
  const form = await request.formData()
  const email = String(form.get("email") ?? "").trim().toLowerCase()
  const code = String(form.get("code") ?? "").replace(/\s+/g, "")
  const ret = safeReturn(String(form.get("return") ?? ""), "/account")
  const a = api(locals.shop, request)
  try {
    await a.post("/auth/sign-in-code/verify", { email, code })
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) {
      return redirectWith(a.call, `/account?${new URLSearchParams({ return: ret, step: "code", email, error: e.message })}`)
    }
    throw e
  }
  await claimCart(a, cookies)
  return redirectWith(a.call, ret)
}
