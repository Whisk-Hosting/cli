// POST /checkout/payment {provider_id}: starts paying. Bank transfer places the order at once;
// Windcave sends the customer to its payment page; Stripe and PayPal show their own fields on
// the checkout page.
import type { APIRoute } from "astro"
import { api, ApiError } from "../../lib/medusa"
import { cart as getCart } from "../../lib/store"
import { complete, paymentMethods, startPayment } from "../../lib/checkout"

export const POST: APIRoute = async ({ request, locals, cookies, redirect }) => {
  const a = api(locals.shop, request)
  const cart = await getCart(a, cookies)
  if (!cart) return redirect("/cart", 303)
  const providerId = String((await request.formData()).get("provider_id") ?? "")
  const method = paymentMethods(locals.shop).find((m) => m.id === providerId)
  if (!method) return redirect("/checkout?error=Choose%20how%20to%20pay.", 303)
  try {
    const session = await startPayment(a, cart, providerId)
    if (method.kind === "bank_transfer") {
      const done = await complete(a, cookies, cart.id)
      return "order" in done ? redirect(`/order/${done.order.id}`, 303) : redirect(`/checkout?error=${encodeURIComponent(done.error)}`, 303)
    }
    if (method.kind === "windcave") {
      const url = session?.data?.redirect_url
      if (typeof url !== "string" || !url.startsWith("https://")) return redirect("/checkout?error=Card%20payments%20are%20unavailable%20right%20now.", 303)
      return redirect(url, 303)
    }
    return redirect("/checkout?pay=1", 303)
  } catch (e) {
    if (e instanceof ApiError && e.status < 500) return redirect(`/checkout?error=${encodeURIComponent(e.message)}`, 303)
    throw e
  }
}
