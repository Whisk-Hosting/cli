// POST /account/addresses {action: add|edit|delete|default, id?, ...address}: the customer's
// saved addresses.
import type { APIRoute } from "astro"
import { api, ApiError } from "../../lib/medusa"
import { addressFrom } from "../../lib/format"

export const POST: APIRoute = async ({ request, locals, redirect }) => {
  const form = await request.formData()
  const action = String(form.get("action") ?? "")
  const id = encodeURIComponent(String(form.get("id") ?? ""))
  const a = api(locals.shop, request)
  const back = (error: string) => redirect(`/account?${new URLSearchParams({ error, ...(action === "add" ? { address: "new" } : action === "edit" ? { address: String(form.get("id")) } : {}) })}#addresses`, 303)
  try {
    if (action === "delete") await a.del(`/store/customers/me/addresses/${id}`)
    else if (action === "default") await a.post(`/store/customers/me/addresses/${id}`, { is_default_shipping: true, is_default_billing: true })
    else if (action === "add" || action === "edit") {
      const parsed = addressFrom(form)
      if ("missing" in parsed) return back("Fill in your name and full address.")
      await a.post(action === "add" ? "/store/customers/me/addresses" : `/store/customers/me/addresses/${id}`, parsed.address)
    } else return back("Unknown change.")
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) return redirect("/account", 303)
    if (e instanceof ApiError && e.status < 500) return back(e.message)
    throw e
  }
  return redirect("/account#addresses", 303)
}
