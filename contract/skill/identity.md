# Whisk skill §4: Identity in full

The rest of §4 of the Whisk skill (https://skill.whisk.run/), which an agent reads first. Read it when the app signs in its own customers, is a Promoted app, has public pages, serves MCP, or uses feature flags or roles.

**Using an app as your person.** To change what lives in a deployed app (its pages, records,
settings), call the app's own API rather than writing to its database:
`whisk call GET /cms/api/pages?limit=5`, `whisk call PATCH /cms/api/pages/4 --data @page.json`.
It sends the request to the app's address with your Whisk credential, so you never put the token
in a request yourself; prefer it to curl. The edge lets you in as the person you act for, with
their grants, and adds `X-Whisk-Agent` (your name) and `X-Whisk-Agent-Id`; the app records the
change as theirs, made by you. An answer outside 2xx is `CALL_FAILED` with the app's answer in
`details.body`. When the person you act for is an owner of the app's business, your `whisk
login` works as it is: `whisk call` signs each request on this computer, and the call is
recorded in the business's audit log like any agent's. For anyone else (an admin, a member) the
edge refuses a login with `TOKEN_SCOPE`, and you need an agent token: an agent identity's
credential, which needs `change` to write, or one from `whisk agent-token create`, which needs
`deploy`, as `WHISK_TOKEN`. In the site and shop templates the pages are at `/cms/api`. For
many requests (an import, a bulk change), write one JSON request per line, `{"method", "path",
"data"}`, and send them with `whisk call --batch file.ndjson --parallel 4`: one run, one
connection, an answer per line. Keep `--parallel 1` when a line needs an earlier one made first.

`customer` identities are the app's own users (portals, public apps), enabled with
`customer_identity: app | org`. They arrive in the same headers without roles or groups, and
`X-Whisk-User-Id` names the customer. The way in is an invitation until an owner opens
registration; onboard your own with
`POST /v1/orgs/<org>/apps/<app>/customers {email, name}` and your service token, which answers
the link (201) and emails nothing; someone already in the pool answers 200 with them. To have
Whisk email someone their invitation, `POST .../customers/<id>/invite` (once a day per person at
most, `CUSTOMER_INVITE_SENT`, counted as your app's email). Only send one when the owner or the
person asked for it, never on a schedule or for a whole list. `whisk customers list`, `invite` (with `--email` to send it), `block` and `unblock` do the same from
the CLI. An owner can remove a customer: they are signed out and can no longer sign in. Invited
again (or signing up again) within 30 days they come back under the same `X-Whisk-User-Id`, so
keep their records rather than deleting them when a request stops naming them; after 30 days
the platform deletes the person, and a later invitation makes a new id.

Customers sign in on Whisk's pages in the app's display name (set it with `whisk apps
display-name`, or the pages say its slug), with a password, an emailed code or a passkey. Give them a way out: link `/.whisk/account` (their own page, with their password,
passkeys and Sign out) somewhere a signed-in customer sees it, or `/.whisk/logout?return=/`
(W132). With `customer_identity: app`, `sign_in` in `whisk.yaml` makes those pages and the sign-in
emails look like the app: `logo` (a PNG or JPEG in the repository, at most 100 KB), `colour`,
`background`, `text`, `font` (`whisk`, `system` or `serif`) and `corners` (`square`, `rounded`,
`pill`). A logo takes the name's place at the top, so use one that carries the name. Match the
app's own design, keep text and background readable (W131), and leave the
"Secured by Whisk" band alone: it is not the app's to style.

**Promoted apps.** An app a business sells to its own customers can be made a Promoted app by an
owner in the dashboard (`promoted.in_force` on `GET /v1/orgs/<org>/apps/<app>`): it never
sleeps, has 4 GB of memory that is always there for it and at least two processor cores (the
plan's, when it gives more), and is served first when its server is busy. Never ask for promotion to get round
anything in this skill, and never promote an app yourself: it is charged, so it is the owner's
choice. Only the dashboard says whether an app is a Promoted app: whisk.yaml never does, so
leave `promoted:` out of it (doctor's W179), and never change the app's files when an owner
promotes it or stops. An app that uses `workers` or `b2b: true` needs a Promoted app: its
production deploy waits, `blocked` with `PROMOTED_APP_REQUIRED`, until an owner promotes it,
and goes live by itself after; relay the NEEDS_HUMAN block. Previews of it run with a Promoted
app's memory without waiting. Everything else in this skill still applies.

A Promoted app is where an existing product brings what it already has:

- **Its own sign-in.** List every route under `routes.public` (`["/**"]`) and the app's own
  login decides who gets in; doctor leaves out W090 for it. The edge then limits each address as
  it would one signed-in person (600 requests a minute, 3000 on Business); every request counts,
  the files the app serves itself included, so keep a cold load (a service worker's precache
  too) well under that. Build or keep a login of the app's own only on a Promoted app.
- **Its own job processes.** `workers` runs up to ten long-running commands beside the web
  process, in the same container and memory, one copy of each, restarted when it exits:
  ```yaml
  workers:
    - name: jobs                  # WHISK_WORKER=jobs; its log lines start [worker:jobs]
      command: bundle exec sidekiq
    - name: queue
      command: node dist/worker.js
      database: direct            # DATABASE_URL is a session connection: LISTEN, advisory
                                  #  locks and SET work; at most 3 open across the workers
  ```
  Use it to bring a queue an app already has across unchanged (graphile-worker, pg-boss,
  Sidekiq, Oban, Celery, Que, GoodJob); Procfile `worker:` lines map onto it, and `release:`
  onto `migrate`. Size each library's own pool to the three direct connections. Whisk shows a
  worker's logs, restarts and memory, and does not see inside its jobs: no run history, replay
  or failure alerts. New background work is written as functions (§7) on every app.
- **A shop's setup.** A shop made from the shop template goes live closed for orders
  (`SHOP_NOT_READY`) until the business has given who is emailed each order and each enquiry, a
  way to be paid, delivery options with prices and its name. Right after its first deploy, and
  whenever its checkout is closed, read `whisk call GET /admin/whisk/setup` and walk the person
  through each item not done, one at a time, in their words: each has a `fix` and `steps`, from
  signing up for Stripe to pasting its keys on the secrets page. Ask for the details and send them
  with `whisk call POST /admin/whisk/setup --data '{"orders_email": "…"}'`; recommend the rest
  (GST number, phone, address, policy pages, own domain and email) without holding up the launch.
- **Trade ordering.** The shop template runs on any plan with room for it and
  keeps its own customer sign-in. Its trade ordering (companies, trade prices, paying on
  account, the ERP link) comes only with Promoted apps: with `b2b: true` a production deploy
  waits until the app is one, and a preview of an app not yet promoted fails with `B2B_UNAVAILABLE`. Its whisk.yaml says
  `commerce: medusa`: keep it, since Whisk's commerce engine answers its API (`/store`,
  `/admin`, `/auth`, `/hooks`, in Medusa's shape) and the app serves only its pages. The site
  and shop templates' whisk.yaml say `content: payload`: keep it, since Whisk's content engine
  answers their site editor (`/cms`) and its API (`/cms/api`, in Payload's shape). A page block
  of the site's own is a file, `content/blocks/<slug>.json`: `{"label": ..., "fields": [{"name",
  "type", "label", "default", ...}]}`, its types text, textarea, words, picture, check, number,
  select (with options), link, group and rows (with fields); `CONTENT_BLOCK_INVALID` says what is
  wrong with one.

Who may open the app (groups, people, everyone in the business) and who is in the business are
an owner's or admin's choice. Everyone in the business means its members. A guest, someone from
outside such as a contractor, opens only the apps they are given and works on those alone, so
invite one with `whisk members invite <email> --guest --app <app>`. `whisk access show` and
`whisk members list` read them. When the
person asks you to change them, `whisk access grant`/`revoke` and `whisk members invite`/`remove`
do not change anything yourself: they print a link and exit 2 (`NEEDS_HUMAN`, with
`details.request_id`). Show the person the link; they approve exactly that change on it, signed
in, within 24 hours. `whisk requests show <id> --wait` then waits for their answer and prints the
result (an invite's link to send, for instance); a declined or expired request is
`REQUEST_CLOSED`, and nothing changed. When the person is a developer, whose role cannot make that change,
the request goes to the business's owners instead (`details.asked_of` is `owners`): they are
emailed and have 7 days. Tell the person it is waiting on their owners; do not wait for it. The
same holds for `whisk apps restore`.

**Public pages.** The edge rejects a cross-site POST to the app (`CSRF_REJECTED`), so forms work
from the app's own pages with no token. Put a public form that strangers fill in (contact, quote
request, sign-up) under `routes.challenge`: a POST without a solved proof-of-work challenge gets
`CHALLENGE_REQUIRED` carrying one; solve it with the Altcha widget and resend with the solution in
the `altcha` form field. There is no difficulty to set: a person gets one a phone solves in about
a second, and an address that keeps posting to the app gets harder ones (a larger `maxnumber`)
until it slows down, so always solve the challenge the latest `CHALLENGE_REQUIRED` carries.
`routes.headers` adds or replaces response headers on every route, such
as a `Content-Security-Policy`. The edge sends `Cross-Origin-Opener-Policy: same-origin`, so a
link to the app from inside a sandboxed iframe (a CRM tile, a widget on another site) opens to
Chrome's `ERR_BLOCKED_BY_RESPONSE`; when the app is meant to be opened that way, set
`Cross-Origin-Opener-Policy: unsafe-none` in `routes.headers`.

**The Whisk bar.** When the app has an admin (a dashboard, a back office, a CMS), name it in
whisk.yaml so the business's team gets a bar across the top of every page, like WordPress's
toolbar, with `Admin` and `Edit page` links:

```yaml
admin:
  path: /admin                        # the admin's address on this app
  edit: "/admin/edit?path={path}"     # optional: opens the editor at the page with this path
  setup: /admin/setup                 # optional: what the app still needs from the business
```

`admin.path` and `admin.edit` are paths on the app's own address; `{path}` is the page's path,
percent-encoded. The edge adds the bar to team members' pages only, signs them in quietly on
public pages once they have signed in on the app's address, and never shows it to customers,
guests or the public. Build the `admin.edit` route to check the person is on the team (or send
them to `/.whisk/login?return=<this url>`), find what that path shows, and redirect to its edit
screen, or to the list when nothing matches. An app with no admin leaves `admin` out and gets no
bar. The site and shop templates already declare theirs.

`admin.setup` names a route that answers `GET`, for a team member, with what the app still needs
from the business before it works for its customers: `{"ready": false, "items": [{"key",
"label", "required", "done", "fix", "steps"}]}`, `done` null for what the app cannot check
itself. When an app needs the business's own details, accounts or choices to work (where emails
go, how it is paid, its prices), keep them in the app, editable in its admin, and list them there
rather than leaving a person to find them. `whisk deploy` prints each item not done once the
deploy is live, required ones first, and the bar shows `Finish setup` beside `Admin` while a
required one is missing. Walk the person through those items after a deploy.

**An MCP server in the app.** When the person wants to use the app from Claude or ChatGPT,
serve an MCP server (Streamable HTTP, one `POST` per JSON-RPC message) from the app and name
it in whisk.yaml:

```yaml
mcp:
  path: /mcp                          # where the MCP server answers; never under routes.public
  read_tools: [list_invoices, get_invoice]   # the tools that only read
```

The person adds `https://<app address>/mcp` as a custom connector, and the platform does the
sign-in: no OAuth, tokens or login code in the app. Each request arrives with the person's team
headers and `X-Whisk-Agent` naming the assistant, so a tool answers and changes exactly what
that person may, and records changes as theirs, made by it. Give every tool `annotations` with
`readOnlyHint`, and list each read-only tool under `read_tools` too: an agent allowed only to
read may call those and no other, and any tool not listed counts as a change. Keep the server
stateless (answer each request on its own, ignore `Mcp-Session-Id`), because the app can sleep
and restart between requests.

### Feature flags

To ship a change to a few people first, declare a flag in whisk.yaml and read it from
`X-Whisk-Flags` (Starter plan and up):

```yaml
flags:
  new-checkout:
    remove_by: 2027-01-31        # a release flag: gone by this day
  payments-off: {kind: kill-switch}
```

```ts
const flags = new Set((c.req.header("X-Whisk-Flags") ?? "").split(",").filter(Boolean));
if (flags.has("new-checkout")) return newCheckout(c);
return checkout(c);
```

Keep both sides working until the flag is on for everyone. The person decides who sees it, on
the app's Flags page or with `whisk flags set new-checkout --team` (everyone on the team),
`--people a@example.com`, `--groups beta`, `--percent 10`, then `whisk flags on new-checkout`
for everyone and `whisk flags off new-checkout` to go back. None of it needs a deploy. Once a
release flag is on for everyone, delete its off side and the flag in one change. A background
job asks `GET $WHISK_FLAGS_URL?user=<id>` with `WHISK_SERVICE_TOKEN` and gets `{"flags": [...]}`. Never keep a flag's
rollout in code or the database; the platform holds it.

### Who may see and change what

The platform decides who may open the app. The app decides which records each person may see
and change, and that is where most real security holes in business apps are: one customer
opening another's invoice by changing the number in the address. Do it the same way every time.

1. **Store the owner on every record a person creates**: their `X-Whisk-User-Id`, in a column
   such as `owner_id` or `author_id`. Never take the owner from the body, the query string or a
   hidden form field.
2. **Filter in the query, not afterwards.** The template's `whisk` module has `scopeFor`
   (`scope_for`, `ScopeFor` in Go): the business's team sees every record, a customer only their
   own, anyone else nothing. Every list, search, count, export and report goes through it.
3. **Check every record fetched by id** with `canSee`, then `canChange` before changing or
   deleting it (`can_see` and `can_change`; `CanSee` and `CanChange`). A record the person may not
   see answers 404, as if it did not exist; one they may see but not change answers 403.
4. **Change the rules in one place.** When a business wants something different (members see
   only their own records, a group sees a department's), change the helpers, not each route. The
   template's access tests (`access.test.ts`, `test_access.py`, `access_test.go`) check the rules
   over thousands of generated people and records; keep them passing and add a case for the new
   rule.
5. **Test each kind of record as the wrong person.** For every route that reads or changes one,
   add a test that calls it as a second customer, as a team member who is not an owner or admin,
   and with no identity at all, and expects 404, 403 or an empty list, never the record. Ids in
   the address and in the body both count.

The same habits close the other common holes. Send SQL with parameters (the ORM, or `$1`), never
by joining strings. Let the framework escape HTML; never insert text a person typed as raw HTML.
Keep keys on the server, never in code sent to the browser. Fetch only addresses the app chose,
never a URL a person typed without checking its host first. Keep uploads in storage (§9) and serve
them from there. Before pushing, run the tests and `whisk doctor`: W094 warns when an app with
customer sign-in never limits anything to the signed-in person, and W096 to W099 flag SQL built
from text, raw HTML, a list of people's records with no owner filter, and public routes that
write.
