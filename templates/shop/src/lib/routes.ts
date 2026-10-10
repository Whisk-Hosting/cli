// Which addresses are Medusa's, which are the shop's pages and which are the business's website,
// and how often the password routes may be tried. Pure; table-tested.

// Medusa answers /store, /admin, /auth, /hooks, its admin at /app, /health, and /erp-link (the
// ERP link's calls, when trade ordering is installed). Everything else is the storefront: the
// shop's pages, and the website's pages from site/ beside them.
const MEDUSA = /^\/(store|admin|auth|hooks|app|health|erp-link)(\/|$)/
export const STOREFRONT = /^\/(?!(store|admin|auth|hooks|app|health|erp-link)(\/|$)).*/

export const isMedusaPath = (path: string) => MEDUSA.test(path)

// The shop's own pages, which the website cannot take over: the shop's home at /shop, products,
// categories, collections, cart, checkout, accounts, orders, search, and the sitemap and
// robots.txt, which list and fence both. Every other address is the website's first, and the
// shop's only when the website has no file there (the storefront's built files under /_astro
// carry a hash in their names, as the website's do, so the two never share one).
const SHOP = /^\/(shop|products|categories|collections|cart|checkout|account|order|search)(\/|$)|^\/(sitemap\.xml|robots\.txt)$/
export const isShopPath = (path: string) => SHOP.test(path)

// Tries in fifteen minutes: signing in allows a few typos per address; sign-up and reset
// emails fewer, since each sends mail or makes an account.
export const limitFor = (path: string): { perEmail: number; perIp: number } =>
  path.endsWith("/register") || path.endsWith("/reset-password") ? { perEmail: 3, perIp: 20 } : { perEmail: 10, perIp: 60 }

// The storefront's built files (storefront/dist/client): scripts and styles under /_astro, whose
// names carry a hash and so never change, and the public folder's files (favicon and the like).
const TYPES: Record<string, string> = {
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".webp": "image/webp",
  ".avif": "image/avif",
  ".gif": "image/gif",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".json": "application/json",
  ".webmanifest": "application/manifest+json",
  ".txt": "text/plain; charset=utf-8",
  ".map": "application/json",
}

// staticFile answers the file under root that a path names, with its type and how long it may
// be cached, or null: for a path that is not a plain file name, escapes root, or has a type the
// storefront does not serve.
export const staticFile = (root: string, urlPath: string): { file: string; type: string; cache: string } | null => {
  let decoded: string
  try {
    decoded = decodeURIComponent(urlPath)
  } catch {
    return null
  }
  if (!decoded.startsWith("/") || decoded.includes("\0") || decoded.split("/").some((part) => part === ".." || part.startsWith("."))) return null
  const ext = /\.[a-z0-9]+$/i.exec(decoded)?.[0].toLowerCase()
  const type = ext ? TYPES[ext] : undefined
  if (!type) return null
  const cache = decoded.startsWith("/_astro/") ? "public, max-age=31536000, immutable" : "public, max-age=3600"
  return { file: root.replace(/\/$/, "") + decoded, type, cache }
}

// The website's files (site/dist, built from site/ by the shop's build): its pages, and the
// images, documents, fonts, scripts and styles they use.
const SITE_TYPES: Record<string, string> = {
  ...TYPES,
  ".html": "text/html; charset=utf-8",
  ".htm": "text/html; charset=utf-8",
  ".xml": "application/xml",
  ".pdf": "application/pdf",
  ".ttf": "font/ttf",
  ".otf": "font/otf",
  ".mp4": "video/mp4",
  ".webm": "video/webm",
  ".mp3": "audio/mpeg",
}

const safePath = (urlPath: string): string | null => {
  let decoded: string
  try {
    decoded = decodeURIComponent(urlPath)
  } catch {
    return null
  }
  if (!decoded.startsWith("/") || decoded.includes("\0") || decoded.includes("\\") || decoded.split("/").some((part) => part === ".." || part.startsWith("."))) return null
  return decoded
}

export type SiteFile = { file: string; type: string; cache: string }

// siteFiles lists the files under root that may answer a path, in the order to try them: a page
// keeps the address form it was built with (src/pages/about.astro is /about, about/index.astro
// is /about/), so /about tries about.html then about/index.html, /about/ tries
// about/index.html, and / tries index.html. A path naming a file of a type the website serves
// tries that file alone. Pages may be cached five minutes, so an edit shows soon after a
// deploy; the built scripts and styles under /_astro never change.
export const siteFiles = (root: string, urlPath: string): SiteFile[] => {
  const decoded = safePath(urlPath)
  if (!decoded) return []
  const base = root.replace(/\/$/, "")
  const page = (rel: string): SiteFile => ({ file: base + rel, type: SITE_TYPES[".html"], cache: "public, max-age=300" })
  if (decoded.endsWith("/")) return [page(decoded + "index.html")]
  const ext = /\.[a-z0-9]+$/i.exec(decoded.slice(decoded.lastIndexOf("/")))?.[0].toLowerCase()
  if (ext) {
    const type = SITE_TYPES[ext]
    if (!type) return []
    const cache = decoded.startsWith("/_astro/") ? "public, max-age=31536000, immutable" : ext === ".html" || ext === ".htm" ? "public, max-age=300" : "public, max-age=3600"
    return [{ file: base + decoded, type, cache }]
  }
  return [page(decoded + ".html"), page(decoded + "/index.html")]
}

// The website's old addresses (site/redirects.json, written by the mover): each answers with one
// hop to where it went, or 410 for a page that is gone. A redirect for an exact query wins over
// one for the path alone.
export type Redirect = { from: string; to?: string; status?: number }
export const redirectFor = (redirects: Redirect[], path: string, search: string): { status: number; location?: string } | null => {
  const hit = redirects.find((r) => search && r.from === path + search) ?? redirects.find((r) => r.from === path)
  if (!hit) return null
  if (hit.status === 410) return { status: 410 }
  if (!hit.to) return null
  return { status: hit.status === 302 || hit.status === 307 || hit.status === 308 ? hit.status : 301, location: hit.to }
}

// A smaller copy of a website file, when the build wrote one beside it and the browser takes it:
// a WebP for a PNG or JPEG, else Brotli, else gzip for text. `has` says which copies exist.
// The address never changes, so the answer varies on the header that chose it.
const accepts = (header: string, token: string) =>
  header.split(",").some((part) => {
    const [name, ...params] = part.trim().toLowerCase().split(";")
    const q = params.map((p) => p.trim()).find((p) => p.startsWith("q="))
    return name.trim() === token && (q === undefined || Number(q.slice(2)) > 0)
  })

export type Variant = { file: string; type?: string; encoding?: string; vary?: string }
export const variantFor = (file: string, accept: string, acceptEncoding: string, has: (file: string) => boolean): Variant => {
  if (/\.(png|jpe?g)$/i.test(file) && has(`${file}.webp`)) {
    return accepts(accept, "image/webp") ? { file: `${file}.webp`, type: "image/webp", vary: "Accept" } : { file, vary: "Accept" }
  }
  const encoded = ([["br", ".br"], ["gzip", ".gz"]] as const).find(([enc, suffix]) => accepts(acceptEncoding, enc) && has(file + suffix))
  if (encoded) return { file: file + encoded[1], encoding: encoded[0], vary: "Accept-Encoding" }
  return has(`${file}.br`) || has(`${file}.gz`) ? { file, vary: "Accept-Encoding" } : { file }
}

// A website form posts to /forms/<name>; the entry is kept in the shop's database.
export const formName = (path: string): string | null => /^\/forms\/([a-z0-9-]{1,40})$/.exec(path)?.[1] ?? null

// A form's fields from its body, or null when the body is not one; and whether any is filled
// (the website's form script first posts empty to fetch the edge's challenge, which must not
// become an entry).
export const formFields = (contentType: string, body: string): Record<string, unknown> | null => {
  try {
    const fields = contentType.includes("application/json") ? JSON.parse(body) : Object.fromEntries(new URLSearchParams(body))
    return fields && typeof fields === "object" && !Array.isArray(fields) ? fields : null
  } catch {
    return null
  }
}
export const anyFilled = (fields: Record<string, unknown>) => Object.values(fields).some((v) => String(v ?? "").trim() !== "")

// Where a form post returns: the page it came from, told which form was sent.
export const formReturn = (referer: string | undefined, name: string): string => {
  try {
    const u = new URL(referer ?? "/", "http://x")
    u.searchParams.set("sent", name)
    return u.pathname.startsWith("//") ? `/?sent=${encodeURIComponent(name)}` : u.pathname + u.search
  } catch {
    return `/?sent=${encodeURIComponent(name)}`
  }
}
