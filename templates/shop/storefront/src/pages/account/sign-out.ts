// POST /account/sign-out: ends the session and forgets the cart, which was the customer's.
import type { APIRoute } from "astro"
import { api, redirectWith } from "../../lib/medusa"
import { dropCart } from "../../lib/store"

export const POST: APIRoute = async ({ request, locals, cookies }) => {
  const a = api(locals.shop, request)
  await a.del("/auth/session").catch(() => undefined)
  dropCart(cookies)
  return redirectWith(a.call, "/")
}
