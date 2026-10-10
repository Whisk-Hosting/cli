// POST /account/register {first_name, last_name, email, password, return}: opens an account
// with a password and signs in.
import type { APIRoute } from "astro"
import { api, ApiError, redirectWith } from "../../lib/medusa"
import { claimCart } from "../../lib/store"
import { safeReturn } from "../../lib/format"
import { registerProblem, signIn } from "../../lib/account"

export const POST: APIRoute = async ({ request, locals, cookies }) => {
  const form = await request.formData()
  const input = {
    email: String(form.get("email") ?? "").trim().toLowerCase(),
    password: String(form.get("password") ?? ""),
    first_name: String(form.get("first_name") ?? "").trim(),
    last_name: String(form.get("last_name") ?? "").trim(),
  }
  const ret = safeReturn(String(form.get("return") ?? ""), "/account")
  const a = api(locals.shop, request)
  const back = (error: string) =>
    redirectWith(a.call, `/account?${new URLSearchParams({ return: ret, step: "register", email: input.email, first_name: input.first_name, last_name: input.last_name, error })}`)
  const problem = registerProblem(input)
  if (problem) return back(problem)
  try {
    const { token } = await a.post<{ token: string }>("/auth/customer/emailpass/register", { email: input.email, password: input.password })
    await a.post("/store/customers", { email: input.email, first_name: input.first_name, last_name: input.last_name }, token)
    await signIn(a, input.email, input.password)
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) {
      return back(/exist/i.test(e.message) ? "That email already has an account. Sign in with a code, or reset the password." : e.message)
    }
    throw e
  }
  await claimCart(a, cookies)
  return redirectWith(a.call, ret)
}
