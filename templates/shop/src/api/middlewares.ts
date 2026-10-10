// What sits in front of Medusa's own routes:
// - the admin API is for the business's Whisk team only, signed in through Whisk (/auth/whisk);
// - password sign-ins are rate-limited, since Medusa does not limit them itself;
// - every address that is not Medusa's is the storefront (storefront/, built with Astro).
import { defineMiddlewares, type MedusaNextFunction, type MedusaRequest, type MedusaResponse } from "@medusajs/framework/http"
import { createReadStream, promises as fs } from "node:fs"
import path from "node:path"
import { pathToFileURL } from "node:url"
import { kv } from "../lib/kv"
import { personFrom, sessionHolds } from "../lib/staff"
import { isMedusaPath, limitFor, staticFile, STOREFRONT } from "../lib/routes"
import { storefrontContext } from "../lib/shop"

const refuse = (res: MedusaResponse, status: number, code: string, message: string, fix: string) =>
  res.status(status).json({ error: { code, message, fix } })

// The admin API answers only someone on the business's Whisk team. Medusa has already checked
// the session (made by /auth/whisk); this checks it still belongs to whoever Whisk says is here,
// and ends it when not. A secret API key (for a link to another system) is the business's own
// and passes.
const staffOnly = async (req: MedusaRequest, res: MedusaResponse, next: MedusaNextFunction) => {
  const context = (req as any).auth_context
  // No one signed in: Medusa has already turned this away unless the route is open to all (the
  // admin's feature flags, read before sign-in).
  if (!context || context.actor_type === "api-key") return next()
  if (sessionHolds(context?.app_metadata?.whisk_user_id, personFrom(req.headers))) return next()
  await new Promise<void>((ok) => req.session.destroy(() => ok()))
  return refuse(res, 401, "AUTH_REQUIRED", "The shop's admin is for its team.", "Open /app while signed in to Whisk as a member of the business that runs this shop.")
}

// Password sign-in, sign-up and reset requests, counted per address and per network address.
const rateLimited = async (req: MedusaRequest, res: MedusaResponse, next: MedusaNextFunction) => {
  const email = String((req.body as any)?.email ?? (req.body as any)?.identifier ?? "").trim().toLowerCase()
  const window = Math.floor(Date.now() / 900_000)
  const [byEmail, byIp] = await Promise.all([
    email ? kv().incr(`login:${email}:${window}`, 900) : Promise.resolve(0),
    kv().incr(`login-ip:${req.ip}:${window}`, 900),
  ])
  const limit = limitFor(req.originalUrl.split("?")[0])
  if (byEmail > limit.perEmail || byIp > limit.perIp) {
    res.set("retry-after", "900")
    return refuse(res, 429, "RATE_LIMITED", "Too many tries.", "Wait fifteen minutes, or sign in with an emailed code.")
  }
  next()
}

let astro: Promise<{ handler: (req: unknown, res: unknown, next: unknown, locals?: unknown) => unknown }> | undefined
const storefrontEntry = () =>
  (astro ??= import(pathToFileURL(path.resolve(process.cwd(), "storefront/dist/server/entry.mjs")).href))

const storefrontRoot = () => path.resolve(process.cwd(), "storefront/dist/client")

// A built file of the storefront's, if the path names one; false when it does not.
const sendStatic = async (req: MedusaRequest, res: MedusaResponse, pathname: string): Promise<boolean> => {
  if (req.method !== "GET" && req.method !== "HEAD") return false
  const found = staticFile(storefrontRoot(), pathname)
  if (!found) return false
  const stat = await fs.stat(found.file).catch(() => null)
  if (!stat?.isFile()) return false
  res.status(200).set({ "content-type": found.type, "content-length": String(stat.size), "cache-control": found.cache })
  if (req.method === "HEAD") {
    res.end()
    return true
  }
  await new Promise<void>((ok, fail) => createReadStream(found.file).on("error", fail).on("end", ok).pipe(res))
  return true
}

// A middleware with a pattern is mounted the way express mounts a prefix, which takes the matched
// part off req.url; the storefront needs the address as the browser asked for it.
const storefront = async (req: MedusaRequest, res: MedusaResponse, next: MedusaNextFunction) => {
  const pathname = req.originalUrl.split("?")[0]
  if (isMedusaPath(pathname)) return next()
  req.url = req.originalUrl
  // The storefront signs in and out through the API, whose answers carry the session cookie. This
  // request's own session is left alone, so it neither saves over that session nor sends a
  // cookie of its own after the API's.
  ;(req as any).session = null
  if (await sendStatic(req, res, pathname)) return
  const [{ handler }, shop] = await Promise.all([storefrontEntry(), storefrontContext(req.scope)])
  return handler(req, res, next, { shop })
}

// Express mounts a pattern the way it mounts a prefix, and passes over a match that stops partway
// through a path segment, so each pattern below matches the whole of the path it is for.
export default defineMiddlewares({
  routes: [
    { matcher: /^\/admin(\/.*)?$/, middlewares: [staffOnly] },
    { matcher: /^\/auth\/user\/.*/, middlewares: [(_req, res) => refuse(res, 404, "NOT_FOUND", "Staff sign in through Whisk.", "Open /app while signed in to Whisk.")] },
    { matcher: /^\/auth\/customer\/emailpass(\/register|\/reset-password)?$/, methods: ["POST"], middlewares: [rateLimited] },
    { matcher: STOREFRONT, bodyParser: false, middlewares: [storefront] },
  ],
})
