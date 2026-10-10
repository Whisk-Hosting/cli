// Which addresses are Medusa's and which are the storefront's, and how often the password
// routes may be tried. Pure; table-tested.

// Medusa answers /store, /admin, /auth, /hooks, its admin at /app, and /health. Everything else
// is the storefront.
const MEDUSA = /^\/(store|admin|auth|hooks|app|health)(\/|$)/
export const STOREFRONT = /^\/(?!(store|admin|auth|hooks|app|health)(\/|$)).*/

export const isMedusaPath = (path: string) => MEDUSA.test(path)

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
