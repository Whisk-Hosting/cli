// Pure decisions about how the shop runs, from its environment. medusa-config.ts calls these and
// test/settings.test.ts table-tests them; nothing here reads the process or the network.
import { createHmac } from "node:crypto"

export type Env = Record<string, string | undefined>

// The platform's DATABASE_URL ends in sslmode=require, which in libpq means "encrypt, but do not
// check the certificate": the database sits behind the app network's gateway with a certificate
// from the platform's own authority. node-postgres reads require as verify-full, so the mode is
// taken off the address and handed to the driver as what libpq means by it.
export type Database = { url: string; driverOptions: Record<string, unknown> }

const NO_VERIFY = new Set(["require", "prefer", "allow"])

export const databaseFrom = (raw: string): Database => {
  const url = new URL(raw)
  const mode = url.searchParams.get("sslmode") ?? ""
  url.searchParams.delete("sslmode")
  if (mode === "disable" || mode === "") return { url: url.toString(), driverOptions: { connection: { ssl: false } } }
  // nosemgrep: problem-based-packs.insecure-transport.js-node.bypass-tls-verification.bypass-tls-verification -- libpq's sslmode=require, as the platform's DATABASE_URL asks (CONTRACT.md §5)
  const ssl = NO_VERIFY.has(mode) ? { rejectUnauthorized: false } : { rejectUnauthorized: true }
  return { url: url.toString(), driverOptions: { connection: { ssl } } }
}

// Medusa signs its tokens and session cookies with two secrets, and the shop keys its sign-in
// codes with a third. A shop needs nobody to set them: each is derived from WHISK_DELIVERY_KEY,
// the app's own key that the platform derives and never shares, under a label of its own, so
// none reveals the key or another. Without the key (a build, which reads the config but signs
// nothing, or a laptop) they are fixed values that only work there; a server without it stops.
export type Secrets = { jwt: string; cookie: string; codes: string }

const derive = (key: Buffer, label: string) => createHmac("sha256", key).update(`whisk-shop:${label}`).digest("hex")

export const secretsFrom = (env: Env, building = false): Secrets => {
  const key = Buffer.from(env.WHISK_DELIVERY_KEY ?? "", "base64")
  if (key.length < 32) {
    if (building || env.WHISK_DEV === "1" || env.NODE_ENV === "test") return { jwt: "development-only", cookie: "development-only", codes: "development-only" }
    throw new Error("WHISK_DELIVERY_KEY is missing: the shop derives its token secrets from it.")
  }
  return { jwt: derive(key, "jwt"), cookie: derive(key, "cookie"), codes: derive(key, "codes") }
}

// A payment provider is on when its credentials are set. The shop's own bank transfer (Medusa's
// system provider) is always on, so the first deploy can take orders with no keys at all. Each
// name is written as a string literal, which is how whisk doctor sees a secret that is read.
export type PaymentProvider = { resolve: string; id: string; options: Record<string, unknown> }

// The keys the storefront's pages hand the browser (Stripe's publishable key, PayPal's client
// id), which are public by design.
export const browserKeysFrom = (env: Env): { stripe: string | null; paypal: string | null } => ({
  stripe: env["STRIPE_PUBLISHABLE_KEY"] || null,
  paypal: env["PAYPAL_CLIENT_ID"] || null,
})

export const paymentProvidersFrom = (env: Env): PaymentProvider[] => {
  const out: PaymentProvider[] = []
  if (env["STRIPE_API_KEY"]) {
    out.push({
      resolve: "@medusajs/medusa/payment-stripe",
      id: "stripe",
      options: {
        apiKey: env["STRIPE_API_KEY"],
        webhookSecret: env["STRIPE_WEBHOOK_SECRET"] ?? "",
        // Cards, Apple Pay, Google Pay and Afterpay come from the methods turned on in the
        // Stripe dashboard; the storefront's Payment Element shows whichever apply.
        automaticPaymentMethods: true,
        capture: env["STRIPE_CAPTURE"] !== "manual",
      },
    })
  }
  if (env["WINDCAVE_USERNAME"] && env["WINDCAVE_API_KEY"]) {
    out.push({
      resolve: "./src/modules/windcave",
      id: "windcave",
      options: {
        username: env["WINDCAVE_USERNAME"],
        apiKey: env["WINDCAVE_API_KEY"],
        environment: env["WINDCAVE_ENVIRONMENT"] === "live" ? "live" : "test",
        publicUrl: env["WHISK_PUBLIC_URL"] ?? "",
        capture: env["WINDCAVE_CAPTURE"] !== "manual",
      },
    })
  }
  if (env["PAYPAL_CLIENT_ID"] && env["PAYPAL_CLIENT_SECRET"]) {
    out.push({
      resolve: "./src/modules/paypal",
      id: "paypal",
      options: {
        clientId: env["PAYPAL_CLIENT_ID"],
        clientSecret: env["PAYPAL_CLIENT_SECRET"],
        environment: env["PAYPAL_ENVIRONMENT"] === "live" ? "live" : "sandbox",
        capture: env["PAYPAL_CAPTURE"] !== "manual",
      },
    })
  }
  return out
}

// The provider ids Medusa gives them, pp_<module id>_<provider id>: what a region lists.
export const providerIds = (providers: PaymentProvider[]): string[] => [
  "pp_system_default",
  ...providers.map((p) => (p.id === "stripe" ? "pp_stripe_stripe" : `pp_${p.id}_${p.id}`)),
]
