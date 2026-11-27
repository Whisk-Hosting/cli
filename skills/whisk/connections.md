# Whisk skill §8: Email in, other apps and outside systems

The rest of §8 of the Whisk skill (https://skill.whisk.run/), which an agent reads first. Read it when the app receives email, calls another app of the business, reaches the internet, or connects to an outside service or an AI model.

**Receiving email.** Declare an inbox and every message to the app's address is stored, then
delivered to the handler like a webhook: verified, retried, listed and replayable. The Team plan
and above include an inbox; on Free and Starter every message is dropped (`W080` says so).

```yaml
inbox:
  handler: /inbound/email
  allow_from: ["@lab.example", "results@other.example"]   # optional; anyone when absent
```

`whisk inbox` prints the address, `<app>.<org>@in.whisk.run`; `<app>.<org>+anything@…` reaches
the app too, with `anything` as `tag`. A business that wants its own address runs `whisk inbox
domains add results.yourbusiness.com`, publishes the MX record it prints, then `whisk inbox
domains verify <id>`: every address at that domain then reaches this app. Use a subdomain that
receives no other mail. The handler receives `POST` with `Content-Type: application/json`, the
same `X-Whisk-Webhook-*` headers as a webhook with source `inbox`, and this body:

```json
{"message_id":"<b7@lab.example>","from":"Lab <results@lab.example>","from_address":"results@lab.example",
 "to":["lab.acme@in.whisk.run"],"cc":[],"recipient":"lab.acme@in.whisk.run","tag":"","subject":"Batch 7",
 "text":"See attached.","html":"","text_truncated":false,"html_truncated":false,
 "authentication":{"spf":"pass","dkim":"pass","dmarc":"pass"},
 "raw":{"upload_id":"01J…","key":"app/01J…/inbox/…/message.eml","bytes":48213,"content_type":"message/rfc822"},
 "attachments":[{"filename":"b7.pdf","content_type":"application/pdf","upload_id":"01J…","key":"app/01J…/inbox/…/1-b7.pdf","bytes":40120}],
 "received_at":"2026-10-09T08:00:00Z"}
```

The original message and each attachment are already in the app's storage: read one with
`GET /v1/orgs/<org>/apps/<app>/uploads/<upload_id>` and the service token, which answers a link
to it, or by `key` with the storage client when the app declares `storage: true`. `text` and
`html` stop at 1 MB each; the original holds the rest. Dedupe on `X-Whisk-Webhook-Id` or
`message_id`. Mail is dropped, never bounced, when it is over 25 MB, from a sender `allow_from`
does not accept (a whole address or `@domain`; a message failing DMARC never matches), past 30 a
minute, 1,000 or 1 GB a day, or unreadable; a dropped message is still listed with its reason
and cannot be replayed (`INBOX_MESSAGE_DROPPED`). `whisk webhooks events inbox` and `whisk
webhooks replay inbox <id>` work on messages as on any source. Locally, with `whisk dev` running,
`whisk inbox send message.eml` (or `--from`, `--subject`, `--text`, `--attach`) delivers a test
message the same way.

**Calling another of the business's apps.** When one app needs an answer from another now (the
website asking the stock app for today's price, a customer portal asking the orders app where
an order is), call it directly instead of copying its data. Both apps say so. The caller names
the apps it calls in its `whisk.yaml`:

```yaml
calls: [orders]
env: { ORDERS_APP_ID: "01J..." }   # whisk apps info orders --json prints it as app.id
```

and the app being called names who may call it in its own `whisk.yaml`, then deploys:

```yaml
accepted_callers: [website]       # the orders app accepts calls from website and nobody else
```

then send ordinary HTTP to `http://<app id>.internal.whisk:8443/<path>` with
`Authorization: Bearer $WHISK_SERVICE_TOKEN`. The
call never leaves the platform. The other app receives it with `X-Whisk-Audience: service` and
`X-Whisk-Service-App` naming the caller's app id, so it can also decide per route. Once the
called app adds the caller and deploys, the caller's calls pass. A call the other app does not accept is `CALLER_NOT_ACCEPTED`, and the
caller's deploy and `whisk doctor` (`W121`) say which line the other app needs. An app with no
`accepted_callers` line accepts only the apps that called it before this rule, for one platform
release; its deploys carry `ACCEPTED_CALLERS_MISSING` and `whisk doctor` (`W120`) names them, so
add the line listing only the callers you expect. Calling an app not listed in `calls` is
`FORBIDDEN_ROLE`, and a call without the token is `AUTH_REQUIRED`. A preview calls the other app's production. Events stay inside the app that
sent them (§7), so between apps: read another app's tables through a shared database (§5); call
it to ask something or to have it act by its own rules; to hand over work that can wait, call a
route that sends an event to the other app's own queue and answers `202` at once.

**Routes only apps may call.** List them under `routes:` as `apps: ["/internal/**"]`. The edge
refuses everyone but another app's service call there, signed-in people included.

**Apps Whisk runs for the business.** Some apps in a business are managed apps: Whisk supplies
their code and keeps them up to date (`whisk apps` marks them `managed`; the ERP link is one). You
cannot push to, clone or deploy one (`APP_MANAGED`). Add one with `whisk managed add <product>
--link <role>=<app>`, change what it lets you set with `whisk managed set <app> NAME=value`, and
pause or resume it with `whisk pause` and `whisk resume`. A copy linked to your app can call it,
and your app finds the copy's id in `WHISK_LINKED_<PRODUCT>` (for the ERP link,
`WHISK_LINKED_ERP_LINK`) after its next start; a product whose copies differ by a setting (the ERP
link's `MARKET`, one per market) may have several on one app, listed there as `id,au=id`. When a copy reports something only a person can fix
(for the ERP link, signing in to the ERP again), Whisk emails the business's owners once. The
`managed:` block (`product:`, `name:`, `settings:`, `links:`, `link_key:`, `variants:`, `notify:`) only means
something in Whisk's own source apps; elsewhere it is ignored.

**Reaching the internet.** By default (`egress: open`) an app calls any public internet address
itself, on any port but the mail ports (25, 465, 587: send mail with `email: true`), over IPv4.
Private and internal addresses are unreachable, and an app opens at most 200 new connections a
second. So an app can deliver its customers' webhooks to the addresses they give it and call
public APIs directly; for a service a person grants access to, use a connection (below), so the
app never holds the key. `egress: closed` lets the app reach only its own database and cache,
the apps it calls, its connections and email, never the rest of the internet: use it for an app
that holds data it must never send anywhere else.

**Services outside Whisk: connections.** Reach an outside API (an ERP, accounting, CRM, a
payment provider) through a connection, so the app never holds its key. Whisk's broker makes
each call, checks it against what a person granted, adds the credential and sends it on.

```yaml
connections:
  erp:                                     # the app gets WHISK_CONNECTION_ERP_URL
    url: https://api.example-erp.com/v2    # https, no query, no credentials
    auth:                                  # how a request is signed; empty for an API with no key
      headers:
        Authorization: "Bearer {secret.ERP_API_KEY}"
    operations:                            # only these calls pass; ask for the least you need
      - { name: Read stock levels, method: GET, path: "/stock/**" }
      - { name: Create sales orders, method: POST, path: /orders }
```

Call `$WHISK_CONNECTION_ERP_URL/stock/42` with the header `Whisk-Service-Token:
$WHISK_SERVICE_TOKEN` (or `Authorization: Bearer $WHISK_SERVICE_TOKEN` when the API's own
credential goes in another header); a client library pointed at that address needs only that
default header. Never put the API's key in code or under `secrets`: the keys a recipe reads
(`secret.NAME`) are declared for you, a person sets them, and they never reach the app. A key is
the business's: when another app of it already holds one the recipe names (`whisk connections
list` says which app), share it with `whisk secrets share NAME --from <app>` and never ask the
person to paste it again; their one tap granting the connection then lets the app use it.

The recipe is text with `{expression}` placeholders built from fixed functions, so any API's
sign-in can be described: `basic(secret.USER, secret.PASSWORD)`; a signature such as
`{base64(hmac_sha256(secret.KEY, concat(request.method, request.path, time.unix)))}` with
`{time.unix}` in another header; a token step that posts a client id and secret and places
`{token}`; a signed JWT, `{jwt('RS256', secret.KEY_PEM, json('iss', secret.EMAIL, 'iat',
time.unix, 'exp', add(time.unix, 3600)))}`.

References: `secret.NAME`; `request.method` (upper case), `request.url` (the whole upstream
address), `request.host`, `request.path` (escaped, without the query), `request.query` (the raw
query string exactly as sent, without `?`, and empty when there is none) and `request.body`;
`time.unix`, `time.unix_ms`, `time.rfc3339`, `time.http`, `time.amz` and `time.date`; `nonce`
and `uuid` (fixed for one call, so a header and the signature over it agree); `token` (with a
token step or a sign-in); `vendor.client_id` and `vendor.client_secret`. Functions:
`hmac_sha1`, `hmac_sha256`, `hmac_sha512` (key, message); `sha1`, `sha256`, `sha512`; `base64`,
`base64url` (no padding), `hex`, `base64_decode`, `hex_decode`; `urlencode`, `upper`, `lower`,
`trim`, `sort_query` (pairs ordered by name, encoding kept); `concat`; `basic(user, password)`;
`add(a, b)` on whole numbers; `json('name', value, ...)`; `jwt('ALG', key, claims[, kid])`; and
`sign('ALG', key_pem, message)` (RS256, RS384, RS512, PS256, ES256, EdDSA; `jwt` also takes
HS256, HS384, HS512). Hashes, HMACs and signatures are raw bytes, so wrap them in `base64`,
`base64url` or `hex`. The broker sends on the query the app sent, byte for byte, so build it once
and keep a client library from encoding it again. Unleashed, for example, signs the query string:

```yaml
    auth:
      headers:
        api-auth-id: "{secret.UNLEASHED_API_ID}"
        api-auth-signature: "{base64(hmac_sha256(secret.UNLEASHED_API_KEY, request.query))}"
        client-type: your-business/your-app
        Accept: application/json
```

A person grants each connection, freshly signed in, on the app's Keys tab. The
first production deploy, and any deploy that changes the address or recipe or adds an
operation, waits with `GRANT_NEEDED`: relay its names and link to the human, and say plainly
what each operation lets the app do. Previews are refused until a person grants previews too.
Use a connection for every outside API whose key the app should not hold; one with no key can
still be a connection, with an empty `auth`. The broker's own refusals carry the header
`Whisk-Broker: refused` and an error code (`CONNECTION_NOT_GRANTED`, `CONNECTION_PAUSED`,
`CONNECTION_LIMIT`, `CONNECTION_REQUEST_AMBIGUOUS`, `CONNECTION_BUSY`, ...); anything else is the
API's own answer. A connection with `auth.body` (Odoo 16 to 18) needs a JSON body with a placeholder
at each of its pointers; the broker fills in the key. A system with its own certificate takes `pin: sha256/…`
(ask the human for its public key's hash). When the outside system wants a certificate you upload
(NetSuite), name `keypair: {secret: NAME}` and sign with that secret: never make or ask for a
private key; the human presses "Make key pair" on the connections page and uploads the
certificate it offers. When the system wants a person to sign in and allow the app (Xero,
MYOB, QuickBooks, Google, Microsoft: "OAuth", "Connect to Xero"), name `auth.sign_in` with a `label` (the
system's name, for its "Sign in to …" button), its `authorize` and `token` addresses, the client ID and secret as `{secret.NAME}` and the scopes
(include the one that keeps the sign-in, such as `offline_access`), and put `Bearer {token}` in a
header; tell the human to register an app with the system whose redirect URI is
`https://auth.whisk.run/connections/callback`, set its ID and secret, and press "Sign in" on
the connections page once they allow it. Until then calls are `CONNECTION_SIGN_IN_NEEDED`;
relay its link. Anything else the system wants on each call (Xero's `Xero-Tenant-Id`) the app
reads through the connection and sends itself. Send plain paths with the real method: dot segments, `;` parameters and method
override headers are refused. Redirects come back to you unfollowed, and a credential the API
echoes back arrives as `[redacted by Whisk]`. Each connection may make 600 calls a minute and
100,000 a day unless someone changed that. When the app's work needs more, raise them with
`whisk connections limit NAME --calls-per-minute <calls>` or `--calls-per-day <calls>`, which
holds from the next call with no push, and keep them below what the outside system itself
allows. If the app may be misbehaving, `whisk connections pause NAME` stops its calls at once. A system that is not HTTP (a database, SFTP) still uses a plain secret (§6).

**AI models: a connection with a spend cap.** Every call to an AI model's API (OpenAI,
Anthropic, Gemini, OpenRouter, Mistral, Groq, xAI, DeepSeek and the rest) goes through a
connection with `ai:`. Never put an AI provider's key under `secrets`: a key set there is saved
on hold and never reaches the app (`SECRET_HELD`). Whisk reads what each answer cost from the fields you name, adds it
up, and stops the connection's calls when the cap is reached, with an email to the business.

```yaml
connections:
  openai:
    url: https://api.openai.com/v1
    auth: {headers: {Authorization: "Bearer {secret.OPENAI_API_KEY}"}}
    operations:
      - {name: Write replies, method: POST, path: /chat/completions}
    ai:
      spend_cap: {per_month: 50}   # optional: the limit your person chose, in US dollars
      cost:
        - {at: /usage/prompt_tokens, usd_per_million: 2.50}
        - {at: /usage/completion_tokens, usd_per_million: 10.00}
```

The spending limit is the person's money, so talk it through with them. Say what the app uses
the model for, roughly what one use costs at the model's prices, how many uses a day you expect,
and suggest a limit from that (`per_day`, `per_week`, `per_month`, `total`, in US dollars, any
mix). Tell them that when it is reached the app's calls to the model stop until the window ends
(or, for `total`, until it is raised), and that they get an email saying so. Write the numbers
they chose as `spend_cap`: the person sees them filled in when they allow the connection and can
change them there. Later, set a new limit they ask for with `whisk connections limit NAME
--per-month <dollars>`; it is in force from the next call, with no push. The person can also
change it themselves on the app's page. Take each `cost` part's price from the provider's pricing page for the model
the app uses, and list every place the count appears, streamed and not. OpenAI's chat
completions and the APIs that copy it (Mistral, Groq, xAI, DeepSeek) count `/usage/prompt_tokens`
and `/usage/completion_tokens`; OpenAI's responses API and Anthropic `/usage/input_tokens` and
`/usage/output_tokens` (Anthropic's stream also starts with `/message/usage/input_tokens`);
Gemini `/usageMetadata/promptTokenCount` and `/usageMetadata/candidatesTokenCount`; OpenRouter
states its cost as `/usage/cost`, priced `usd_each: 1`. Ask for usage in streams where the provider needs it
(OpenAI: `stream_options: {include_usage: true}`). An answer that names no cost part stops that
deploy's calls (`AI_COST_UNREADABLE`) until a deploy fixes the parts. Over a cap, calls answer
`429 AI_SPEND_CAP`: show the person a calm message that the AI feature is paused, never an error
page, and keep the rest of the app working.

A deploy that carries `SECRET_HELD` names an AI key Whisk found under `secrets` and put on hold.
Move it: declare an AI connection whose recipe reads `{secret.NAME}`, take the name out of
`secrets`, point the app's client at `$WHISK_CONNECTION_<NAME>_URL` with the service token as
above, walk the person through the limit, and push. The value stays as it is, so nobody sets
it again; the person grants the connection and the app's AI features work again.
