// POST /account/code {email, return}: asks the shop to email a sign-in code, then shows the box
// to type it in. The API's cookie that ties the code to this browser is passed on.
import type { APIRoute } from "astro"
import { api, ApiError, redirectWith } from "../../lib/medusa"
import { safeReturn } from "../../lib/format"

export const POST: APIRoute = async ({ request, locals }) => {
  const form = await request.formData()
  const email = String(form.get("email") ?? "").trim().toLowerCase()
  const ret = safeReturn(String(form.get("return") ?? ""), "/account")
  const a = api(locals.shop, request)
  const to = (q: Record<string, string>) => `/account?${new URLSearchParams({ return: ret, ...q })}`
  try {
    await a.post("/auth/sign-in-code", { email })
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) return redirectWith(a.call, to({ error: e.message }))
    throw e
  }
  return redirectWith(a.call, to({ step: "code", email }))
}
