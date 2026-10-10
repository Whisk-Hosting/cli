// POST /account/password/set {token, email, password}: sets a new password with the link's token,
// then signs in with it.
import type { APIRoute } from "astro"
import { api, ApiError, redirectWith } from "../../../lib/medusa"
import { claimCart } from "../../../lib/store"
import { passwordProblem, signIn } from "../../../lib/account"

export const POST: APIRoute = async ({ request, locals, cookies }) => {
  const form = await request.formData()
  const token = String(form.get("token") ?? "")
  const email = String(form.get("email") ?? "").trim().toLowerCase()
  const password = String(form.get("password") ?? "")
  const a = api(locals.shop, request)
  const back = (error: string) => redirectWith(a.call, `/account/password?${new URLSearchParams({ token, email, error })}`)
  const problem = passwordProblem(password)
  if (problem) return back(problem)
  try {
    await a.post("/auth/customer/emailpass/update", { password }, token)
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) {
      return redirectWith(a.call, `/account/password?error=${encodeURIComponent("That link has expired. Ask for a new one.")}`)
    }
    throw e
  }
  await signIn(a, email, password)
  await claimCart(a, cookies)
  return redirectWith(a.call, "/account")
}
