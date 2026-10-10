// Medusa on Whisk. Everything comes from the platform's environment (CONTRACT.md §5): the
// pooled database, the app's own cache for events, workflows, locks and the cache, the
// business's storage, Whisk's email, and secrets derived from the app's own key.
import { defineConfig, Modules } from "@medusajs/framework/utils"
import { databaseFrom, paymentProvidersFrom, secretsFrom, tradeOrdering } from "./src/lib/settings"

const env = process.env
const db = databaseFrom(env.DATABASE_URL ?? "postgres://localhost/shop")
// `medusa build` reads this file too, with none of the app's environment.
const secrets = secretsFrom(env, process.argv.includes("build"))
const kv = env.WHISK_KV_URL
// Trade ordering (companies, branches, trade prices, approvals, paying on account, the ERP link)
// is the private @whisk/shop-b2b plugin, which the platform installs for a Promoted app whose
// whisk.yaml says b2b: true. Without it this is a plain public shop.
const trade = tradeOrdering()
const publicUrl = (env.WHISK_PUBLIC_URL ?? "http://localhost:8080").replace(/\/$/, "")

// With a cache, Medusa's events, workflows and locks are shared through it; without one (a
// laptop) they stay in the process. Locks never use Postgres advisory locks, which the pooled
// connection cannot hold.
const sharedModules = kv
  ? [
      { key: Modules.EVENT_BUS, resolve: "@medusajs/medusa/event-bus-redis", options: { redisUrl: kv } },
      { key: Modules.WORKFLOW_ENGINE, resolve: "@medusajs/medusa/workflow-engine-redis", options: { redis: { redisUrl: kv } } },
      {
        key: Modules.LOCKING,
        resolve: "@medusajs/medusa/locking",
        options: { providers: [{ resolve: "@medusajs/medusa/locking-redis", id: "locking-redis", is_default: true, options: { redisUrl: kv } }] },
      },
      {
        key: Modules.CACHING,
        resolve: "@medusajs/medusa/caching",
        options: { providers: [{ resolve: "@medusajs/caching-redis", id: "caching-redis", is_default: true, options: { redisUrl: kv } }] },
      },
    ]
  : []

const files = env.WHISK_STORAGE_ENDPOINT
  ? {
      key: Modules.FILE,
      resolve: "@medusajs/medusa/file",
      options: {
        providers: [
          {
            resolve: "./src/modules/whisk-files",
            id: "whisk",
            options: {
              public_url: publicUrl,
              file_url: `${env.WHISK_STORAGE_ENDPOINT.replace(/\/$/, "")}/${env.WHISK_STORAGE_BUCKET}`,
              access_key_id: env.WHISK_STORAGE_ACCESS_KEY,
              secret_access_key: env.WHISK_STORAGE_SECRET_KEY,
              region: env.WHISK_STORAGE_REGION || "auto",
              bucket: env.WHISK_STORAGE_BUCKET,
              prefix: env.WHISK_STORAGE_PREFIX ?? "",
              endpoint: env.WHISK_STORAGE_ENDPOINT,
              additional_client_config: { forcePathStyle: true },
            },
          },
        ],
      },
    }
  : null

const email = env.WHISK_SERVICE_TOKEN
  ? {
      key: Modules.NOTIFICATION,
      resolve: "@medusajs/medusa/notification",
      options: { providers: [{ resolve: "./src/modules/whisk-email", id: "whisk-email", options: { channels: ["email"] } }] },
    }
  : {
      key: Modules.NOTIFICATION,
      resolve: "@medusajs/medusa/notification",
      options: { providers: [{ resolve: "@medusajs/medusa/notification-local", id: "local", options: { channels: ["email"] } }] },
    }

module.exports = defineConfig({
  projectConfig: {
    databaseUrl: db.url,
    databaseDriverOptions: db.driverOptions,
    redisUrl: kv,
    workerMode: "shared",
    http: {
      // The storefront, the admin and the API share one address, so nothing cross-origin is
      // allowed.
      storeCors: publicUrl,
      adminCors: publicUrl,
      authCors: publicUrl,
      jwtSecret: secrets.jwt,
      cookieSecret: secrets.cookie,
      authMethodsPerActor: { user: ["whisk"], customer: ["emailpass"] },
    },
    // Customers stay signed in for thirty days from their last visit.
    sessionOptions: { ttl: 30 * 24 * 3600 * 1000, rolling: true, name: "shop_session" },
  },
  plugins: trade ? [{ resolve: "@whisk/shop-b2b", options: {} }] : [],
  admin: {
    path: "/app",
    backendUrl: "/",
  },
  modules: [
    ...sharedModules,
    ...(files ? [files] : []),
    email,
    {
      key: Modules.PAYMENT,
      resolve: "@medusajs/medusa/payment",
      options: { providers: paymentProvidersFrom(env, trade) },
    },
  ],
})
