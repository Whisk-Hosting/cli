# whisk-shop

A shop on Whisk: [Medusa](https://medusajs.com) for products, carts, orders, payments and the
staff admin, and the shop's own pages beside it, in one app. It sells in New Zealand dollars with
GST included, or in Australia (`SHOP_COUNTRY`, below), takes cards, Apple Pay, Google Pay and Afterpay through Stripe, cards through
Windcave, PayPal, and bank transfer, and emails customers their orders. Customers sign in with an
emailed code or a password; the business's Whisk team runs the shop at `/app` with their Whisk
sign-in. Any business can run it. Medusa needs about 525 MB of memory, which a Promoted app
has; trade ordering (companies, trade prices, approvals, pay on account) comes only with a
Promoted app (`promoted: true` and `b2b: true`).

The business's website and its shop are one app on one address. The website is `site/`: its own
Astro app (pages, images, documents, forms), built to static pages by `npm run build` and served
at every address that is not the shop's. The shop's pages are at `/shop` (its home), `/products`,
`/categories`, `/collections`, `/cart`, `/checkout`, `/account`, `/order` and `/search`, with one
`sitemap.xml` and `robots.txt` for both. `/` is the website's home, or the shop's when there is no
`site/`. Edit the website by editing `site/src/pages`; old addresses go in `site/redirects.json`
(`{"from": "/old", "to": "/new"}`, or `"status": 410` for a page that is gone); a form posting to
`/forms/<name>` is kept in the database's `form_entries` table.

The pages in `storefront/` (Astro) are a working start: replace their look, or rewrite them, to
match the website. They talk only to Medusa's store API (`storefront/src/lib`), so any page that
does the same works.

```
medusa-config.ts            Medusa on Whisk: database, cache, storage, email, payments
migrate.ts                  the migrate step: Medusa's migrations, then src/scripts/setup.ts
src/scripts/setup.ts        NZD with GST, the New Zealand region, delivery, the storefront's key
src/api/middlewares.ts      staff-only admin, rate limits, the website and storefront on every page
src/api/auth/               sign-in codes for customers; /auth/whisk signs staff in
src/modules/                Whisk storage and email, and the Windcave and PayPal providers
src/subscribers/, src/jobs/ order and password emails
storefront/                 the shop's pages
site/                       the business's website, when it has one
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
http://127.0.0.1:3000/app for the admin (the shop's home is /shop once `site/` holds a website). `npm run dev` instead rebuilds the server as its source
changes.

## Where it sells

New Zealand, in New Zealand dollars with 15% GST included in prices, unless `env` in
`whisk.yaml` says otherwise before the first deploy (setup adds the market's region, tax and
delivery once and then leaves them to the admin):

| Setting | Values |
|---|---|
| `SHOP_COUNTRY` | `nz` (default) or `au`: Australian dollars, 10% GST |
| `SHOP_PRICES_INCLUDE_TAX` | `false` when prices are entered before tax and the tax is added at checkout |
| `SHOP_TIME_ZONE` | the time zone order dates are shown in, such as `Australia/Perth` |

With prices before tax, the cart, checkout, order pages and emails show lines and the subtotal
before tax, the tax on its own line, then the total.

## Start with a catalogue

`catalogue/catalogue.json` lists products to start with: `{"products": [{"handle":
"impairment-goggles", "title": "Impairment Goggles", "price": 325, "sku": "IG", "categories":
["Goggles"], "options": [{"title": "Type", "values": ["Low", "High"]}], "images":
["goggles.jpg"]}]}`. Prices are entered as the market enters them (with or without tax);
`inStock: false` lists a product as sold out until staff add stock. A picture is a file in
`catalogue/images`, stored on Whisk, or an address (`/catalogue/goggles.jpg` from
`storefront/public`, or `https://…`) used as it is. A job adds every product and category the
shop does not have yet, by handle, within a minute of each deploy, so products staff change in
the admin are never overwritten.

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
