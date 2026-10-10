// Finding or making the account behind a sign-in. A customer has one account whichever way they
// sign in: a password and an emailed code both reach the auth identity whose "emailpass"
// provider identity is their email, so a customer who joined with a code can set a password
// later, and one with a password can use a code. Staff are Whisk team members (src/api/staff.ts).
import { Modules } from "@medusajs/framework/utils"
import type { MedusaContainer } from "@medusajs/framework/types"
import { createCustomerAccountWorkflow } from "@medusajs/medusa/core-flows"

export type AuthContext = {
  actor_id: string
  actor_type: "customer" | "user"
  auth_identity_id: string
  app_metadata: Record<string, unknown>
}

// customerFor answers the signed-in context for an email that has just proven it is theirs,
// making the auth identity and the customer when they do not exist yet. A customer the shop
// already holds with that email (one moved in from another platform, say) is linked rather
// than duplicated.
export const customerFor = async (scope: MedusaContainer, email: string): Promise<AuthContext> => {
  const auth = scope.resolve(Modules.AUTH)
  const customers = scope.resolve(Modules.CUSTOMER)

  const [pid] = await auth.listProviderIdentities({ provider: "emailpass", entity_id: email })
  const identity = pid
    ? await auth.retrieveAuthIdentity(pid.auth_identity_id!)
    : await auth.createAuthIdentities({ provider_identities: [{ provider: "emailpass", entity_id: email, provider_metadata: {} }] })

  let customerId = identity.app_metadata?.customer_id as string | undefined
  if (!customerId) {
    const [existing] = await customers.listCustomers({ email, has_account: true }, { take: 1 })
    if (existing) {
      customerId = existing.id
      await auth.updateAuthIdentities({ id: identity.id, app_metadata: { ...(identity.app_metadata ?? {}), customer_id: existing.id } })
    } else {
      const { result } = await createCustomerAccountWorkflow(scope).run({ input: { authIdentityId: identity.id, customerData: { email } } })
      customerId = result.id
    }
  }
  return { actor_id: customerId, actor_type: "customer", auth_identity_id: identity.id, app_metadata: { customer_id: customerId } }
}
