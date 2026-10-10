// POST /checkout/complete: places the order once PayPal's window has approved the payment.
// Answers JSON for the page's script: where to go, or what went wrong.
import type { APIRoute } from "astro"
import { api } from "../../lib/medusa"
import { cartIdOf } from "../../lib/store"
import { complete } from "../../lib/checkout"

export const POST: APIRoute = async ({ request, locals, cookies }) => {
  const cartId = cartIdOf(cookies)
  if (!cartId) return Response.json({ error: "Your cart has expired." }, { status: 400 })
  const done = await complete(api(locals.shop, request), cookies, cartId)
  return "order" in done ? Response.json({ location: `/order/${done.order.id}` }) : Response.json({ error: done.error }, { status: 402 })
}
