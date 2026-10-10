// POST /auth/sign-in-code/verify {email, code}: signs the customer in when the code is right,
// making their account the first time. The session cookie is what signs them in.
import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { customerFor } from "../../../../lib/accounts"
import { check, normalEmail, type Stored } from "../../../../lib/codes"
import { getJson, kv, setJson } from "../../../../lib/kv"
import { secretsFrom } from "../../../../lib/settings"

const cookieOf = (header: string | undefined, name: string) =>
  (header ?? "").split(/;\s*/).map((p) => p.split("=")).find(([k]) => k === name)?.[1] ?? ""

const REFUSED = { code: "CODE_INVALID", message: "That code didn't work.", fix: "Check the code in the email, or ask for a new one." }

export const POST = async (req: MedusaRequest<{ email?: string; code?: string }>, res: MedusaResponse) => {
  const email = normalEmail(String(req.body?.email ?? ""))
  const code = String(req.body?.code ?? "").replace(/\s/g, "")
  const key = `code:${email}`
  const stored = await getJson<Stored>(key)
  const verdict = check(secretsFrom(process.env).codes, stored, email, code, cookieOf(req.headers.cookie, "shop_code"), Date.now())
  if (!verdict.ok) {
    if (verdict.reason === "wrong" && verdict.stored) {
      await setJson(key, verdict.stored, Math.max(1, Math.ceil((verdict.stored.expires - Date.now()) / 1000)))
    } else if (verdict.reason === "expired" || verdict.reason === "tries") {
      await kv().del(key)
    }
    return res.status(401).json({ error: REFUSED })
  }
  await kv().del(key)
  const context = await customerFor(req.scope, email)
  // A new session for the signed-in customer, so an id fixed before sign-in is worth nothing.
  await new Promise<void>((ok, fail) => req.session.regenerate((e: unknown) => (e ? fail(e) : ok())))
  ;(req.session as any).auth_context = context
  res.clearCookie("shop_code", { path: "/" })
  return res.status(200).json({ customer_id: context.actor_id })
}
