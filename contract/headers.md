# Headers

The platform sets these request headers on every request that reaches an app. Any header
beginning `X-Whisk-` that arrives from the internet is removed before these are set, so an app
may trust them without checking anything else. The only way to reach an app is through the
platform edge; there is no other path that could set them.

## Identity headers

| Header | Present when | Value |
|---|---|---|
| `X-Whisk-Audience` | always | `anonymous`, `team`, `customer` or `service` |
| `X-Whisk-User-Id` | team, customer | stable ULID of the person; the key to store |
| `X-Whisk-Email` | team, customer | verified email, lower case |
| `X-Whisk-Name` | team, customer | display name; may be empty |
| `X-Whisk-Org` | team, customer, service | org ULID |
| `X-Whisk-Groups` | team | comma-separated group names; absent when the person is in no group |
| `X-Whisk-Roles` | team | comma-separated org roles: `owner`, `admin`, `developer`, `billing`, `member`; `guest` appended for guests |
| `X-Whisk-Session-Id` | team, customer | opaque; for log correlation only |
| `X-Whisk-Service-App` | service, app-to-app | ULID of the calling app |
| `X-Whisk-Request-Id` | always | ULID; echo it in every log line. Also returned on the response so a client can quote it. |
| `traceparent` | always | W3C trace context, `00-<trace id>-<parent id>-01`. The trace id is the request id's 16 bytes in hex, so an OpenTelemetry SDK that continues it records the request under its request id. Whatever the caller sent is replaced, and `tracestate` removed. |

Audiences:

- **anonymous**: a public route with no session. Only `X-Whisk-Audience` and
  `X-Whisk-Request-Id` are present.
- **team**: a member or guest of the org that owns the app, with a grant to it.
- **customer**: a user of the app's own customer pool (`customer_identity: app | org`). No groups,
  no roles. Authorise customers by `X-Whisk-User-Id` against your own tables.
- **service**: the platform itself (function runs, webhook deliveries) or another app in the org
  calling through `<app>.internal.whisk`. `X-Whisk-Service-App` names the caller for app-to-app
  calls and is absent for platform deliveries.

Public routes: when the visitor has a session and a grant, the identity headers are present on a
public route too, so a public page can greet a signed-in person. Without a session the route is
served as `anonymous`. A public route never redirects to login.

Private routes without a session: a browser navigation receives a 302 to the platform login; any
other request receives 401 with the standard error body and `WWW-Authenticate: Whisk
realm="<hostname>"`.

Service routes: `queue.endpoint` and every `webhooks[].handler` are reached only by the platform.
They need not be listed in `routes.public`. A request to them from the internet is answered 401
`AUTH_REQUIRED` by the edge and never reaches the app.

## Delivery headers

Set in addition to the identity headers, which read `X-Whisk-Audience: service`.

Function runs (`queue.endpoint`): `X-Whisk-Job-Id` (the run ID), `X-Whisk-Job-Attempt`
(0-based). The Inngest SDK verifies the request with `WHISK_INNGEST_SIGNING_KEY`; an app that
implements the protocol itself must do the same.

Webhook deliveries (`webhooks[].handler`):

| Header | Value |
|---|---|
| `X-Whisk-Webhook-Id` | ULID of the stored event; the same on every retry and replay, so dedupe on it |
| `X-Whisk-Webhook-Source` | the source name from the manifest |
| `X-Whisk-Webhook-Received-At` | RFC 3339 UTC time the platform received it |
| `X-Whisk-Webhook-Orig-<Name>` | every header the provider sent, prefixed; `X-Whisk-Webhook-Orig-Stripe-Signature` for example |
| `X-Whisk-Delivery-Signature` | `v1=<hex>`: HMAC-SHA256 under the app's `WHISK_DELIVERY_KEY` (base64-decoded) over `<X-Whisk-Webhook-Id> "\n" <X-Whisk-Webhook-Received-At> "\n" <raw body>`; present on every delivery and replay |

The body is the provider's raw body, byte for byte. `Content-Type` is the provider's. The
provider's signature has already been verified; an app never sees an unverified body. The
handler verifies `X-Whisk-Delivery-Signature` in constant time and refuses a request that
lacks it or carries `X-Whisk-Service-App` with 401 `DELIVERY_UNVERIFIED`;
`fixtures/deliveries/` holds a signed delivery and its key for a test.

The delivery headers are platform-only. The internal listener strips every inbound `X-Whisk-*`
header; `X-Whisk-Webhook-*`, `X-Whisk-Delivery-Signature` and `X-Whisk-Job-*` survive only on a
request the platform itself makes, never on an app-to-app call, and the queue endpoint answers
401 to anything that is not the platform. A delivery therefore never carries
`X-Whisk-Service-App`: a handler that sees it is looking at another app's call and refuses it.
The templates' `deliveries` helper does all of this before the app's own code runs, and doctor
warns (`W053`) when a handler does not use it.

## Forwarding headers

Standard proxy headers, set by the edge: `X-Forwarded-For` (client IP first), `X-Forwarded-Proto`
(`https`), `X-Forwarded-Host`, `X-Real-IP`. The `Host` header is the hostname the visitor used,
which on a custom domain is the custom domain.

## Endpoints the platform answers in front of the app

These paths are reserved on every app hostname and never reach the app:

| Path | Behaviour |
|---|---|
| `/.whisk/login?return=<path>` | Forces login, then redirects to `return`. Use it to sign someone in from a public route. |
| `/.whisk/logout?return=<path>` | Ends the session on this hostname, then redirects to `return`. |
| `/.whisk/ready` | 200 when the app is awake; used by the waking page. |
| `/.whisk/media/<id>`, `/.whisk/player.js` | An uploaded video or audio file and the player that plays it. |
| `/.whisk/img/<id>?w=&h=&fit=&format=` | An uploaded image in the size and format asked for. |
| either, with `exp`, `kid` and `sig` | A private upload for someone without a Whisk sign-in, through a link the app's server signed with `POST …/uploads/links` (CONTRACT.md §8). |

`return` must be a path on the same hostname; anything else is replaced with `/`.

## Response headers

The edge adds security defaults to every response and the manifest's `routes.headers` overrides
or adds named headers. Defaults: `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`,
`Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: SAMEORIGIN`,
`Permissions-Policy: camera=(), microphone=(), geolocation=()`,
`Cross-Origin-Opener-Policy: same-origin`, `Server: whisk`, and `Cache-Control: private,
no-store` on private routes when the app set none. Previews add `X-Robots-Tag: noindex,
nofollow`. There is no default `Content-Security-Policy`; set one in `routes.headers`.

The edge compresses text answers (HTML, CSS, JavaScript, JSON, SVG and the like) with zstd or gzip
for browsers that accept them. An answer the app already sends with a `Content-Encoding` passes
unchanged.

### Edge response cache

The edge keeps an answer the app marks cacheable and answers the same request again itself,
without reaching the app or waking it. It keeps a `200` to a `GET` whose `Cache-Control` has
`max-age` or `s-maxage` above zero, without `no-store` or `no-cache`, with no `Set-Cookie`, no
`Vary: *`, and a body of at most 1 MiB, for that many seconds and never longer than an hour. The
answer is kept per person: the key includes the hostname, the path and query, the identity
headers above and the app's live deploy, so a new deploy starts empty, and it honours `Vary` on
the request headers it names. `HEAD` is answered from the `GET`'s entry. Requests to
`/.whisk/*`, to the health path, with `Authorization`, `Range` or `Upgrade`, and every other
method always reach the app; a request with `Cache-Control: no-cache` or `Pragma: no-cache` (a
hard refresh) reaches the app and replaces the entry. Preview hosts and app-to-app calls are
never cached.

| Header | Present when | Value |
|---|---|---|
| `X-Whisk-Cache` | the answer was eligible for the edge cache | `hit`: served from the edge cache, with `Age` in seconds since the app gave it; `miss`: the app answered and the edge kept the answer (when its body was within the limit) |

Send `Cache-Control: private, max-age=<seconds>` on pages and reads that may be a little old, and
`public` on a page anyone may see. Never put a `max-age` on an answer that must be current, and
set a cookie only on answers that are not meant to be cached.

Every app on `whisk.page` is a neighbour of every other, so the platform binds each cookie to the
app's own address with the browser's `__Host-` prefix, which no other host can set. A cookie the
app sets as `sid` is stored in the browser as `__Host-sid` (any `Domain` removed, `Path=/`,
`Secure` added) and reaches the app as `sid` again. A cookie in the browser without the prefix
(one set from page script by its bare name, or planted by another site) never reaches the app and
is deleted in the response; page script that sets a cookie for the server sets
`__Host-<name>=<value>; Secure; Path=/`. The names `__Host-whisk_session` and `whisk_session` are
the platform's and are dropped if an app sets them. A request other than GET, HEAD or OPTIONS
that carries the app's cookies is accepted only from the app's own pages (`CSRF_REJECTED`
otherwise; `routes.csrf_off` opts a route out), and a background request from another app's
page (a fetch, an image, a websocket; not a link the visitor follows) arrives without cookies,
so call another app server to server with a service token. Every app cookie is sent as
`Priority=High` unless it names a priority. A `Clear-Site-Data` response header loses its
`"cookies"` directive, which would clear every app's cookies on `whisk.page`; clear your own by
expiring them with `Set-Cookie`. Apps own every other cookie.

## What an app must never do

- Read identity from anything other than these headers: not from a cookie, a query parameter,
  a JSON body or an `Authorization` header a client sent.
- Forward `X-Whisk-*` headers to another service as if they were credentials. They are facts
  about this request only.
- Build a login page. The platform owns sign-in; `/.whisk/login` is the whole integration.
