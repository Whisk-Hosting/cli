// Staff are the business's own Whisk team. Whisk's edge tells the shop who is signed in to
// Whisk (CONTRACT.md §4); a team member opening the shop's admin is given a Medusa user of
// their own, made the first time, and signed in to it. Nobody has a password for the admin.
import { Modules } from "@medusajs/framework/utils"
import type { MedusaContainer } from "@medusajs/framework/types"
import type { AuthContext } from "./accounts"

export type Person = { id: string; email: string; name: string; audience: string; roles: string[] }

const header = (h: Record<string, string | string[] | undefined>, name: string) => {
  const v = h[name]
  return (Array.isArray(v) ? v[0] : v) ?? ""
}

export const personFrom = (h: Record<string, string | string[] | undefined>): Person => ({
  id: header(h, "x-whisk-user-id"),
  email: header(h, "x-whisk-email").toLowerCase(),
  name: header(h, "x-whisk-name"),
  audience: header(h, "x-whisk-audience"),
  roles: header(h, "x-whisk-roles").split(",").map((r) => r.trim()).filter(Boolean),
})

// Who may run the shop: anyone on the business's team except a guest. Billing contacts and
// members included; the admin has no finer roles (Medusa's are a paid licence, not used here).
export const isStaff = (p: Person) => p.audience === "team" && !!p.id && !!p.email && p.roles.some((r) => r !== "guest")

export const splitName = (name: string): { first_name: string; last_name: string } => {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  return { first_name: parts[0] ?? "", last_name: parts.slice(1).join(" ") }
}

// staffFor answers the admin session for a team member, making their Medusa user and auth
// identity (provider "whisk", keyed by their Whisk user id) the first time.
export const staffFor = async (scope: MedusaContainer, p: Person): Promise<AuthContext> => {
  const auth = scope.resolve(Modules.AUTH)
  const users = scope.resolve(Modules.USER)
  const [pid] = await auth.listProviderIdentities({ provider: "whisk", entity_id: p.id })
  const identity = pid
    ? await auth.retrieveAuthIdentity(pid.auth_identity_id!)
    : await auth.createAuthIdentities({ provider_identities: [{ provider: "whisk", entity_id: p.id, user_metadata: { email: p.email } }] })

  let userId = identity.app_metadata?.user_id as string | undefined
  if (userId) {
    const [still] = await users.listUsers({ id: userId }, { take: 1 })
    if (!still) userId = undefined
  }
  if (!userId) {
    const [byEmail] = await users.listUsers({ email: p.email }, { take: 1 })
    const user = byEmail ?? (await users.createUsers({ email: p.email, ...splitName(p.name) }))
    userId = user.id
    await auth.updateAuthIdentities({ id: identity.id, app_metadata: { ...(identity.app_metadata ?? {}), user_id: userId } })
  }
  return { actor_id: userId, actor_type: "user", auth_identity_id: identity.id, app_metadata: { user_id: userId, whisk_user_id: p.id } }
}

// Where the admin sign-in may send someone back to: a page of the admin, never another site.
export const adminReturn = (to: string) => (/^\/app(\/[^/\\]|\/?$|\?)/.test(to) && !to.includes("//") ? to : "/app")

// Whether an admin session still belongs to the person Whisk says is here. A session made for
// one team member is no good once they sign out of Whisk, someone else signs in, or they leave
// the team.
export const sessionHolds = (whiskUserId: unknown, p: Person) => isStaff(p) && whiskUserId === p.id
