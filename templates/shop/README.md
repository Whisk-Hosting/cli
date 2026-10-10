# whisk-shop

A shop on Whisk: [Medusa](https://medusajs.com) for products, carts, orders, payments and the
staff admin, and the shop's own pages beside it, in one app. It sells in New Zealand dollars with
GST included, takes cards, Apple Pay, Google Pay and Afterpay through Stripe, cards through
Windcave, PayPal, and bank transfer, and emails customers their orders. Customers sign in with an
emailed code or a password; the business's Whisk team runs the shop at `/app` with their Whisk
sign-in. Any business can run it. Medusa needs about 525 MB of memory, which a Promoted app
has; trade ordering (companies, trade prices, approvals, pay on account) comes only with a
Promoted app (`promoted: true` and `b2b: true`).

The pages in `storefront/` (Astro) are a working start: replace their look, or rewrite them, to
match the business's site. They talk only to Medusa's store API (`storefront/src/lib`), so any
page that does the same works.

```
medusa-config.ts            Medusa on Whisk: database, cache, storage, email, payments
migrate.ts                  the migrate step: Medusa's migrations, then src/scripts/setup.ts
src/scripts/setup.ts        NZD with GST, the New Zealand region, delivery, the storefront's key
src/api/middlewares.ts      staff-only admin, rate limits, the storefront mounted on every page
src/api/auth/               sign-in codes for customers; /auth/whisk signs staff in
src/modules/                Whisk storage and email, and the Windcave and PayPal providers
src/subscribers/, src/jobs/ order and password emails
storefront/                 the shop's pages
scripts/shop.mjs            build, start, migrate, dev
```

## Run it

`whisk dev` needs Docker with the compose plugin. It starts Postgres, the cache and a local edge,
then runs the shop with the environment it has in production:

```
npm install
npm run build
whisk dev --as you@example.com --roles owner -- npm run start
```

These lines work the same in PowerShell, Command Prompt and a Unix shell. `whisk dev` runs the
migration (`node migrate.js` in the build) first. Open http://127.0.0.1:3000 for the shop and
http://127.0.0.1:3000/app for the admin. `npm run dev` instead rebuilds the server as its source
changes.

## Take payments

Bank transfer is on once the business's account is in the admin: Settings, Store, Metadata,
`bank_account_name` and `bank_account_number`. Each other way to pay is on when its secrets are
declared under `secrets` in `whisk.yaml` and a person sets their values in the dashboard (the
deploy waits until they have):

| Provider | Secrets |
|---|---|
| Stripe | `STRIPE_API_KEY`, `STRIPE_PUBLISHABLE_KEY`, `STRIPE_WEBHOOK_SECRET`; its webhook goes to `https://<shop>/hooks/payment/stripe_stripe` |
| Windcave | `WINDCAVE_USERNAME`, `WINDCAVE_API_KEY` |
| PayPal | `PAYPAL_CLIENT_ID`, `PAYPAL_CLIENT_SECRET` |

`*_CAPTURE: manual` in `whisk.yaml` takes payment when an order is sent rather than when it is
placed; `WINDCAVE_ENVIRONMENT` and `PAYPAL_ENVIRONMENT` other than `live` use their test
systems.

## Test it

`npm test` runs the unit tests; `npm run typecheck` checks the types of the server and the pages.

## Ship it

`whisk doctor`, then `whisk deploy -m "<what changed, for the shop's customers and staff>"`.
