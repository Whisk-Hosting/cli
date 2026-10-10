// GET /robots.txt: everything but the cart, checkout and account, and where the sitemap is.
import type { APIRoute } from "astro"

export const GET: APIRoute = ({ url }) =>
  new Response(
    ["User-agent: *", "Disallow: /cart", "Disallow: /checkout", "Disallow: /account", "Disallow: /order/", "Disallow: /app", "", `Sitemap: ${url.origin}/sitemap.xml`, ""].join("\n"),
    { headers: { "content-type": "text/plain; charset=utf-8", "cache-control": "public, max-age=3600" } },
  )
