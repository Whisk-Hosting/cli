// GET /robots.txt: everything but the cart, checkout, account and admin, and where the sitemap is;
// with a website in site/, the website's own robots.txt first, then the shop's lines.
import type { APIRoute } from "astro"
import { readFile } from "node:fs/promises"
import path from "node:path"
import { robotsTxt } from "../lib/format"

export const GET: APIRoute = async ({ url }) => {
  const site = await readFile(path.resolve(process.cwd(), "site/dist/robots.txt"), "utf8").catch(() => undefined)
  return new Response(robotsTxt(url.origin, site), { headers: { "content-type": "text/plain; charset=utf-8", "cache-control": "public, max-age=3600" } })
}
