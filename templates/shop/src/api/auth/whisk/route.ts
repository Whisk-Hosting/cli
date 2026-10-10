// GET /auth/whisk?return=/app/...: signs a member of the business's Whisk team in to the shop's
// admin. Whisk's edge says who is signed in (CONTRACT.md §4); someone who is not on the team is
// sent to sign in to Whisk first. The admin's own sign-in page comes here at once
// (src/admin/widgets/whisk-sign-in.tsx), so nobody has an admin password.
import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { adminReturn, isStaff, personFrom, staffFor } from "../../../lib/staff"

export const AUTHENTICATE = false

export const GET = async (req: MedusaRequest, res: MedusaResponse) => {
  const back = adminReturn(String(req.query.return ?? ""))
  const person = personFrom(req.headers)
  if (!isStaff(person)) {
    return res.redirect(302, `/.whisk/login?return=${encodeURIComponent(`/auth/whisk?return=${encodeURIComponent(back)}`)}`)
  }
  const context = await staffFor(req.scope, person)
  await new Promise<void>((ok, fail) => req.session.regenerate((e: unknown) => (e ? fail(e) : ok())))
  ;(req.session as any).auth_context = context
  await new Promise<void>((ok, fail) => req.session.save((e: unknown) => (e ? fail(e) : ok())))
  return res.redirect(302, back)
}
