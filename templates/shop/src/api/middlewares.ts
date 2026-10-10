// What sits in front of Medusa's own routes:
// - the admin API is for the business's Whisk team only, signed in through Whisk (/auth/whisk);
// - password sign-ins are rate-limited, since Medusa does not limit them itself;
// - every address that is not Medusa's is the storefront: the business's website (site/, built
//   to site/dist) and the shop's pages (storefront/, built with Astro), on one address.
import { defineMiddlewares, type MedusaNextFunction, type MedusaRequest, type MedusaResponse } from "@medusajs/framework/http"
import { createReadStream, existsSync, promises as fs, readFileSync, statSync } from "node:fs"
import path from "node:path"
import { pathToFileURL } from "node:url"
import { kv } from "../lib/kv"
import { personFrom, sessionHolds } from "../lib/staff"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import {
  anyFilled, formFields, formName, formReturn, isMedusaPath, isShopPath, limitFor, redirectFor, siteFiles, staticFile, STOREFRONT, variantFor,
  type Redirect, type SiteFile,
} from "../lib/routes"
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

// The website: its built pages and files, its old addresses, and its forms.
const siteRoot = () => path.resolve(process.cwd(), "site/dist")
let redirects: Redirect[] | undefined
const siteRedirects = (): Redirect[] =>
  (redirects ??= (() => {
    try {
      const list = JSON.parse(readFileSync(path.resolve(process.cwd(), "site/redirects.json"), "utf8"))
      return Array.isArray(list) ? list : []
    } catch {
      return []
    }
  })())

const isFile = async (file: string) => (await fs.stat(file).catch(() => null))?.isFile() ?? false

const sendSiteFile = async (req: MedusaRequest, res: MedusaResponse, status: number, found: SiteFile) => {
  const v = variantFor(found.file, String(req.headers.accept ?? ""), String(req.headers["accept-encoding"] ?? ""), (f) => existsSync(f) && statSync(f).isFile())
  const stat = await fs.stat(v.file)
  res.status(status).set({
    "content-type": v.type ?? found.type,
    "content-length": String(stat.size),
    "cache-control": status === 200 ? found.cache : "no-cache",
    ...(v.encoding ? { "content-encoding": v.encoding } : {}),
    ...(v.vary ? { vary: v.vary } : {}),
  })
  if (req.method === "HEAD") return void res.end()
  await new Promise<void>((ok, fail) => createReadStream(v.file).on("error", fail).on("end", ok).pipe(res))
}

// The website's answer for a path, if it has one: an old address's redirect, else its page or
// file. A path that is neither is the shop's to answer.
const sendSite = async (req: MedusaRequest, res: MedusaResponse, pathname: string): Promise<boolean> => {
  if (req.method !== "GET" && req.method !== "HEAD") return false
  const search = req.originalUrl.includes("?") ? req.originalUrl.slice(req.originalUrl.indexOf("?")) : ""
  const moved = redirectFor(siteRedirects(), pathname, search)
  if (moved) {
    res.status(moved.status).set({ "cache-control": "public, max-age=300", ...(moved.location ? { location: moved.location } : { "content-type": "text/plain; charset=utf-8" }) })
    res.end(moved.location ? undefined : "This page has been removed.")
    return true
  }
  for (const found of siteFiles(siteRoot(), pathname)) {
    if (await isFile(found.file)) {
      await sendSiteFile(req, res, 200, found)
      return true
    }
  }
  return false
}

// The website's own "not found" page, for an address that is neither the website's nor the shop's.
const sendSiteMissing = async (req: MedusaRequest, res: MedusaResponse): Promise<boolean> => {
  const [missing] = siteFiles(siteRoot(), "/404.html")
  if (!missing || !(await isFile(missing.file))) return false
  await sendSiteFile(req, res, 404, missing)
  return true
}

// The body of a form post, up to 64 KB, read within ten seconds.
const readBody = (req: MedusaRequest, limit = 64 * 1024) =>
  new Promise<string | null>((resolve, reject) => {
    let size = 0
    const chunks: Buffer[] = []
    const timer = setTimeout(() => reject(new Error("the form took too long to arrive")), 10_000)
    req.on("data", (c: Buffer) => {
      size += c.length
      if (size > limit) {
        clearTimeout(timer)
        resolve(null)
        req.resume()
      } else chunks.push(c)
    })
    req.on("end", () => {
      clearTimeout(timer)
      resolve(Buffer.concat(chunks).toString("utf8"))
    })
    req.on("error", (e) => {
      clearTimeout(timer)
      reject(e)
    })
  })

// A website form's entry, kept in the shop's form_entries table (made by src/scripts/setup.ts),
// then back to the page it came from. The edge's challenge (whisk.yaml) keeps robots out.
const formPost = async (req: MedusaRequest, res: MedusaResponse, name: string) => {
  const body = await readBody(req)
  if (body === null) return refuse(res, 413, "BODY_TOO_LARGE", "The form was too large.", "Send less than 64 KB; attach large files by email instead.")
  const fields = formFields(String(req.headers["content-type"] ?? ""), body)
  if (!fields) return refuse(res, 400, "INVALID_REQUEST", "The form could not be read.", "Post the form's fields as a form or as a JSON object.")
  if (!anyFilled(fields)) return refuse(res, 400, "INVALID_REQUEST", "The form was empty.", "Fill in at least one field.")
  const referer = typeof req.headers.referer === "string" ? req.headers.referer : undefined
  const db = req.scope.resolve(ContainerRegistrationKeys.PG_CONNECTION)
  await db("form_entries").insert({ form: name, page: referer ?? null, data: JSON.stringify(fields) }).timeout(5_000, { cancel: true })
  res.status(303).set({ location: formReturn(referer, name) }).end()
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
  const form = formName(pathname)
  if (form && req.method === "POST") return formPost(req, res, form).catch(next)
  // The website answers first, except at the shop's own pages. Its home is the address's home;
  // with no website, the shop's home is.
  const website = !isShopPath(pathname)
  if (website && (await sendSite(req, res, pathname))) return
  if (await sendStatic(req, res, pathname)) return
  if (website && pathname !== "/" && (req.method === "GET" || req.method === "HEAD") && (await sendSiteMissing(req, res))) return
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
