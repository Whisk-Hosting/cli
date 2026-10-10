// POST /auth/sign-in-code {email}: emails a sign-in code and sets the cookie that ties it to
// this browser. It answers the same whether or not the address has an account, so nobody can
// learn who shops here.
import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { Modules } from "@medusajs/framework/utils"
import { CODE_MINUTES, issue, mayRequest, newCode, newNonce, normalEmail, validEmail } from "../../../lib/codes"
import { kv, setJson } from "../../../lib/kv"
import { secretsFrom } from "../../../lib/settings"
import { shopInfo } from "../../../lib/shop"

const NONCE_COOKIE = "shop_code"

export const POST = async (req: MedusaRequest<{ email?: string }>, res: MedusaResponse) => {
  const email = normalEmail(String(req.body?.email ?? ""))
  if (!validEmail(email)) {
    return res.status(400).json({ error: { code: "EMAIL_INVALID", message: "That doesn't look like an email address.", fix: "Check the address and try again." } })
  }
  const hour = Math.floor(Date.now() / 3_600_000)
  const [toEmail, fromIp] = await Promise.all([
    kv().incr(`code:sent:${email}`, CODE_MINUTES * 60),
    kv().incr(`code:ip:${req.ip}:${hour}`, 3600),
  ])
  if (!mayRequest(toEmail - 1, fromIp - 1)) {
    return res.status(429).set("retry-after", "600").json({ error: { code: "RATE_LIMITED", message: "Too many codes asked for.", fix: "Use the last code we sent, or try again in ten minutes." } })
  }
  const code = newCode()
  const nonce = newNonce()
  const { codes } = secretsFrom(process.env)
  await setJson(`code:${email}`, issue(codes, email, code, nonce, Date.now()), CODE_MINUTES * 60)
  const shop = await shopInfo(req.scope)
  await req.scope.resolve(Modules.NOTIFICATION).createNotifications({
    to: email,
    channel: "email",
    template: "signInCode",
    data: { shop: shop.name, shopUrl: shop.url, code, minutes: CODE_MINUTES },
  })
  res.cookie(NONCE_COOKIE, nonce, { httpOnly: true, secure: req.secure, sameSite: "lax", maxAge: CODE_MINUTES * 60_000, path: "/" })
  return res.status(200).json({ sent: true })
}
