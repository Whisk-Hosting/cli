# Whisk skill §3: Domains, analytics and running a live app

The rest of §3 of the Whisk skill (https://skill.whisk.run/), which an agent reads first. Read it when the person wants their own domain, redirects, the CDN, analytics or package scanning, or the app is live and needs looking after.

**Their own domain.** On a paid plan, `whisk domains add jobs.acme.com` prints two DNS records
for the human to create: a TXT that proves they own the name and a CNAME to the app's address.
A bare domain (`acme.com`) cannot have a CNAME. If the human's DNS host offers an ALIAS or
ANAME record, or CNAME flattening (Cloudflare flattens a CNAME at the domain itself), point it
that way at the `apex_target` printed, the app's address: it then goes through the app's CDN
while that is on. Otherwise it takes A (and AAAA) records to the addresses printed, which reach
the app directly and never through the CDN; to have the site on the CDN anyway, make
`www.acme.com` the main address (below). Relay the records with their HOST, the name as a DNS provider's form
takes it (`@` is the domain itself), changing the name's existing A record rather than adding a
second one and leaving its MX and other TXT records alone. Then `whisk domains verify jobs.acme.com` once they
are in place; until then it answers `DOMAIN_UNVERIFIED` naming what is missing. The certificate
is issued on verification and the app answers on the name with no deploy. DNS cannot redirect
`www.acme.com` to `acme.com`: add and verify both, then make one the main address (below). Prefer relative
links; `WHISK_PUBLIC_URL` stays the platform address, and the `Host` header is the name the
visitor used.

**One main address.** Once the human's domain answers, ask them whether it should be the app's
main address, and if so run `whisk domains main acme.com`. Every other address of the app (its
Whisk address, `www.acme.com`, an old domain) then sends a browser to the same page on
`acme.com` with a 301, so people and search engines see one site. Anything other than a page
load, such as a webhook's POST, is still answered on the address it was sent to, so integrations
set up with an old address keep working; point new ones at the main address. Previews keep
their own addresses. A domain becomes the main address only once its certificate is issued, and
the Whisk address takes over again by itself if the domain stops working. `whisk domains main`
prints the main address and `whisk domains main --whisk` makes the Whisk address the main one
again; a human can press Make main on the app's domains page.

To put every app on the business's domain at once (crm.acme.com, billing.acme.com, and each new
app with no further DNS), an owner or admin's session runs `whisk domains business add acme.com`
instead: a TXT and one wildcard CNAME (`*.acme.com`), then `whisk domains business verify`. The
human's existing records (www, mail) keep working.

**A new address.** The app's platform address is made from its slug. To change it, run `whisk
apps rename <new-slug>` on the same app: never create a new app and delete the old one. The app
keeps its data, secrets, history and domains, the old address sends visitors to the new one, and
the old slug still names the app. Commit the `.whisk/app.json` and `whisk.yaml` it updates. The
name people see is the app's display name, which starts as its slug: `whisk apps display-name
"Job Tracker"` sets it and leaves the address as it is.

**Redirects.** When a page is renamed or a site moves to Whisk, keep every old address
working, or search engines drop the pages. List each old path under `redirects` with the new one (`from`, `to`, `status` 301 by
default, 308, 302, 307 or 410 for a page that is gone). `from: /blog/*` with `to: /news/*`
moves a whole section, carrying the rest of the path; `from: /blog/:year/:slug` with `to:
/posts/:slug` reshapes addresses; `from: /product.php?id=12` matches only that query. Short
links and campaigns are rules too (`/go/zoom` to an outside address, status 302 when it will
change). The request's query is kept unless `query: drop`, or the rule's `from` names one.
More than a few dozen rules go in a file named by `redirects_file`, one rule a line:
`/old-page /new-page`, `/blog/* /news/* 308`, `/old-promo 410`; at most 10,000 rules in all.
The edge answers them on every hostname of the app without waking it, ahead of the app's own
routes. Point each rule straight at the final address: `whisk redirects check` (and `whisk
doctor`, `W111`) reports a rule whose target another rule redirects again, and a loop fails the
push. `whisk redirects test /old-page` prints what a request gets, and `whisk redirects list`
what is live. To move the old domain itself, add it to the app (`whisk domains add
old-acme.com`, verify it), then `whisk domains redirect old-acme.com acme.com`: every request
to the old name gets a 301 to the same path on the new one, and a path a rule names goes
straight to the rule's target there. A human can also set it on the app's domains page in the
dashboard. To send `www.acme.com` and every other address to `acme.com`, make `acme.com` the main
address instead (above).

**Their customers' domains.** An app that serves other businesses under their own names (a
white-label app) lets each bring a domain of its own from inside the app, with no person running
the CLI: the template's `domains` helper (`domains.add`, `list`, `verify`, `remove`; Go
`domains.Add` and so on) calls `/v1/orgs/<org>/apps/<app>/domains` with `WHISK_SERVICE_TOKEN`
from production. `add(hostname)` answers the domain with its `id`, `status` and `records`; show
the customer `records` as they are (a TXT, then a CNAME, or A and AAAA records for a bare domain
like `lab.com`) and store the `id` with the customer. Let them press a button that calls
`verify(id)` once they have created the records: `DOMAIN_UNVERIFIED` says in `details.missing`
what is not in place yet, and DNS can take an hour, so never verify in a loop. `status` is then
`pending_certificate` until the certificate is served and `active` after; the app answers on the
name with no deploy and reads the name the visitor used from `Host`. Remove with `remove(id)`,
never by hostname. The app removes only the domains it added; the business's own
(`added_by: team`) answer `DOMAIN_ADDED_BY_TEAM`. A name another app holds is `DOMAIN_TAKEN`,
one app holds at most the plan's `custom_domains_per_app` (`PLAN_LIMIT_DOMAINS`), and more than
30 adds and removes or 120 verifies an hour is `RATE_LIMITED`. Only the customer's own admins
should reach these screens: check their role in the app first. `whisk dev` answers the same
routes; names under `.test` or `.example` verify there at once.

**Serving from near the visitors.** On Team and Business, `whisk cdn on` puts the app's address
and its CNAME'd domains behind a CDN (`whisk cdn` shows the status and each hostname's route).
It helps public pages, sites and downloads. A public page is cached only as long as the app's
`Cache-Control` says, so set it on public pages and assets you want cached; a response that sets
a cookie is never cached. Each production deploy empties the cache. To show a change sooner, the app drops
the pages it changed: `POST /v1/orgs/<org>/apps/<app>/cdn/purge` with `WHISK_SERVICE_TOKEN` and
`{"paths": ["/entries/42", "/entries/*"]}` (a trailing `*` takes everything under it, at most 20
paths, `{}` for everything), or `whisk cdn purge` by hand. Then a public page can carry a long
max-age and still change at once. An app the CDN does not serve answers `"on": false`, and so
does `whisk dev`, so purging on every change is safe anywhere.

**Looking after a live app.** `whisk status` says whether it is running, sleeping or broken,
which deploy is live and which secrets are unset, and, when it stopped answering, why: the exit
code and last lines of a crash, or a wake that failed. `whisk rollback` puts the previous deploy back
(or a named one from `whisk deploys list`). `whisk errors` lists the app's grouped exceptions and
`whisk traces` shows where its requests' time went, once the app sends them (§3, "Errors and
traces"). `SENTRY_DSN` points at Whisk's own error store: initialise the Sentry SDK with only the
DSN, without Sentry's tracing, profiling or session replay, which cost memory and start time and
which Whisk already covers. A log line's `request_id` opens its trace. Traces are kept 7 days,
100,000 spans per app per day. `whisk open <page>` prints the dashboard link for the human (`logs`, `runs`, `secrets`,
`domains`, `access`, `errors`, `analytics`). `whisk github` shows the app's optional two-way copy on the
business's GitHub; a person links it in the app's settings.

**Analytics.** Every app has analytics on every plan. Put `<script defer
src="/.whisk/analytics.js"></script>` in the `<head>` of every page layout (the templates do), and
each page view is counted. The script and what it sends stay on the app's own address, it sets no
cookie and keeps no IP address. The business's own people signed in to the app are left out
(`analytics: { team: count }` in whisk.yaml counts them), and so is a browser asking not to be
tracked (`analytics: { do_not_track: ignore }` counts it). Send an event from the page with `umami.track("signup", { plan: "team" })`
(check `window.umami` first, since a blocker can stop the script), or mark a button with
`data-umami-event="signup"`. Crawlers run no script, so Whisk counts them from its own request log:
search engines, AI assistants and crawlers, link previews and other bots, with nothing to add to the
app. `whisk analytics` prints views, visitors, top pages, sources, events and crawlers over
`--period 24h|7d|30d|90d` or `--from 2026-11-01 --to 2026-11-07`. `--source google.com`, `--page /pricing`
and `--event signup` narrow it to those visits, and `--by` breaks it down by `day` (or `hour`, `month`,
with `--tz`), `page`, `entry`, `exit`, `source`, `event`, `event-property` (the properties `--event`
carried), `country`, `browser`, `os` or `device`. `whisk analytics visitors` lists the visitors and
`whisk analytics visitor <id>` what one did, page by page. `whisk analytics traffic --by day,path,agent`
counts every request, robots, scripts and what the CDN answered from its copy included, grouped by any
of its parts (`query`, `user_agent`, `referer`, `user_id`, `address`, `country`, `network`, `edge_cache`,
`hour`…) and narrowed with `--status 404`, `--kind ai_crawler`, `--country NZ` or `--user-id <id>`; `--list`
prints the latest requests one by one, with the time the app and a wake took, and `--json` every field. `whisk analytics crawlers [--crawler Googlebot]`
prints each crawler's pages, answers and journeys, and `whisk analytics journey <id>` one journey request
by request; the human sees everything with `whisk open analytics`. Visits are kept
7 days on Free, 13 months on paid plans and 25 months on Business. If the app has a privacy notice, say there
that it counts visits with Whisk's analytics, without cookies, and for how long. Do not add another analytics
tool unless the human names one.

**Known vulnerabilities in packages (Business plan).** Whisk checks the live app's packages after
every production deploy and daily: the image (base image packages and everything installed in it)
and every lockfile in the commit. `whisk status` says how many findings need fixing now; `whisk
doctor` lists them as `W063`, one line per package: it checks the local lockfiles as they are,
before you deploy, so run it after each change and the list shrinks as you fix; `whisk scan` lists
each one with its severity, the installed version, the version that fixes it (`fixed`) and where it
was found (a lockfile, or a path in the image). Fix the critical and high ones that have a fix: move
the package to that version or newer in the lockfile (a base image package by moving to a newer base
image), deploy, then `whisk scan --now` to check again. A finding with no `fixed` has no fix yet;
leave it. `whisk doctor` also sums up the person's other apps (where they are an owner, admin or
developer) that have packages to upgrade, under `other_apps`; `whisk scan --all` lists each of those
apps with its packages and fixed versions. Tell your human about them, and fix them in each app's own
repository. For Go, Whisk also checks whether the app's code calls the vulnerable part: a finding
with `reach: not_called` is listed apart under "Not called by your code" and is not counted as
one to fix now; update it when convenient, never ahead of the ones to fix. Every other language
is `unknown` and counts as called. Python's slim image ships pip, which bundles its own copies of urllib3, msgpack and setuptools that the scan reports; the Python template uninstalls pip after `uv sync`, since the app runs from uv's virtualenv, and an app with its own Dockerfile does the same. Most image findings come from the operating system in a full base image: the TypeScript
and Go templates run on a distroless image (`gcr.io/distroless/nodejs24-debian13:nonroot` for Node,
the Python one on `python:3.14-slim`), which has no shell or package manager, so keep `migrate` a plain command such as `node dist/migrate.js`, not `npm run`
or shell syntax (`whisk doctor` warns, `W062`). An app that needs a system program (ffmpeg,
ImageMagick, a browser) moves the Dockerfile's last stage to `FROM node:24-trixie-slim`, installs it
with `RUN apt-get update && apt-get install -y --no-install-recommends <package> && rm -rf
/var/lib/apt/lists/*`, and sets `USER node` and `CMD ["node", "dist/index.js"]`, since only the
distroless image starts node itself. A slim image has no CA certificates, so add `ca-certificates`
to that install whenever a build step or the app makes HTTPS calls (a postinstall download, a
`fetch` at start), or they fail with a certificate error. Deploys are never blocked by findings. On another plan `whisk
scan` says scanning is on the Business plan.
