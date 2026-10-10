// Talking to Medusa's store API from the storefront's server. The API is the same process, so
// calls go to this machine's own address with the customer's cookies, and any cookie Medusa
// sets (the signed-in session) is handed back to the browser.

// What the server knows about the shop, handed in with every request (src/api/middlewares.ts).
export type ShopContext = {
  api: string // the API's own address on this machine, e.g. http://127.0.0.1:8080
  publishableKey: string
  regionId: string
  currency: string
  name: string
  bank: { accountName: string; accountNumber: string } | null // for bank transfer (store metadata)
  stripeKey: string | null // STRIPE_PUBLISHABLE_KEY
  paypalClientId: string | null
  providers: string[] // payment providers the region takes and the server has loaded
  market: { country: string; currency: string; name: string; locale: string; timeZone: string; taxRate: number; taxName: string; taxInclusive: boolean } // src/lib/market.ts
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly body: unknown,
  ) {
    super(message)
  }
}

export type Call = { setCookies: string[] }

const errorMessage = (body: any, status: number) =>
  body?.error?.message ?? body?.message ?? (typeof body === "string" && body ? body : `The shop answered ${status}.`)

export const api = (shop: ShopContext, request: Request, call: Call = { setCookies: [] }) => {
  const send = async <T>(method: string, path: string, body?: unknown, bearer?: string): Promise<T> => {
    const headers: Record<string, string> = {
      accept: "application/json",
      "x-publishable-api-key": shop.publishableKey,
      // The API sets Secure cookies only on a request it knows came over HTTPS; the edge
      // terminated it.
      "x-forwarded-proto": request.headers.get("x-forwarded-proto") ?? new URL(request.url).protocol.replace(":", ""),
    }
    const cookie = mergedCookies(request.headers.get("cookie") ?? "", call.setCookies)
    if (cookie) headers.cookie = cookie
    const forwarded = request.headers.get("x-forwarded-for")
    if (forwarded) headers["x-forwarded-for"] = forwarded
    if (bearer) headers.authorization = `Bearer ${bearer}`
    if (body !== undefined) headers["content-type"] = "application/json"
    const res = await fetch(shop.api + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(20_000),
    })
    call.setCookies.push(...res.headers.getSetCookie())
    const text = await res.text()
    const parsed = (() => {
      try {
        return text ? JSON.parse(text) : {}
      } catch {
        return text
      }
    })()
    if (!res.ok) throw new ApiError(res.status, errorMessage(parsed, res.status), parsed)
    return parsed as T
  }
  return {
    get: <T>(path: string) => send<T>("GET", path),
    post: <T>(path: string, body: unknown = {}, bearer?: string) => send<T>("POST", path, body, bearer),
    del: <T>(path: string) => send<T>("DELETE", path),
    call,
  }
}

// The browser's cookies with any the API has just set in this request laid over them, so a
// call made after sign-in carries the new session.
export const mergedCookies = (header: string, setCookies: string[]): string => {
  const jar = new Map<string, string>()
  for (const part of header.split(/;\s*/).filter(Boolean)) {
    const i = part.indexOf("=")
    if (i > 0) jar.set(part.slice(0, i), part.slice(i + 1))
  }
  for (const sc of setCookies) {
    const first = sc.split(";")[0]
    const i = first.indexOf("=")
    if (i <= 0) continue
    const name = first.slice(0, i)
    const value = first.slice(i + 1)
    const expired = /;\s*(max-age=0|expires=thu, 01 jan 1970)/i.test(sc) || value === ""
    if (expired) jar.delete(name)
    else jar.set(name, value)
  }
  return [...jar].map(([k, v]) => `${k}=${v}`).join("; ")
}

export const query = (params: Record<string, string | number | undefined | null | (string | number)[]>) => {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === "") continue
    if (Array.isArray(v)) v.forEach((x) => q.append(k, String(x)))
    else q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `?${s}` : ""
}

// A redirect that carries the cookies the API set during this request (a new session, or the
// sign-in code's nonce) to the browser, since those calls were made from this server.
export const redirectWith = (call: Call, location: string, status = 303) => {
  const headers = new Headers({ location })
  for (const c of call.setCookies) headers.append("set-cookie", c)
  return new Response(null, { status, headers })
}
