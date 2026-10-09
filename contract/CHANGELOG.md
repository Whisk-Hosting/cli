# Contract changelog

Additive changes by conventions version. A change that would alter the meaning of an existing
field, header or variable is a new version, and every published version is served forever.

Each change adds its own entry as a new file in [`changelog/`](changelog/), named
`YYYY-MM-DD-<short-name>.md` for the day it merges, with the conventions version in its first
line, so changes never edit the same lines. The newest entries are the newest files there; this
file holds the history written before that.

## Version 1

### 2026-10-06

Connected accounts are removed: the token route `GET /v1/orgs/<org>/connections/<id>/token`,
`whisk connections`, and the codes `CONNECTION_EXPIRED`, `CONNECTION_SCOPE` and
`CONNECTION_CLIENT_INVALID`. No app used them. The skill says a key for another system is a
secret, and `WHISK_SERVICE_TOKEN` no longer names a connections route.

### 2026-10-05

An uploaded image answers at `/.whisk/img/<id>` on every app hostname in the size and format
asked for: `w`, `h`, `fit` (`contain` or `cover`) and `format` (`auto`, `avif`, `webp`, `jpeg`,
`png`). Upload records carry `image: {path, url, detail}`. New codes `IMAGE_NOT_READY`,
`IMAGE_UNREADABLE` and `NOT_AN_IMAGE`. The reserved paths in headers.md now list the media and
image paths. The skill's §9 shows how to use it.

Traces. Every container receives `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`,
`OTEL_EXPORTER_OTLP_TRACES_HEADERS` (the service token as a bearer), `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL`,
`OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` and `OTEL_METRICS_EXPORTER` and `OTEL_LOGS_EXPORTER`
set to `none`; a name the manifest sets keeps the manifest's value. Every request carries a W3C
`traceparent` whose trace id is `X-Whisk-Request-Id`'s 16 bytes; one the caller sent is replaced and
`tracestate` removed. The templates start OpenTelemetry when the endpoint is set and tag errors
with the request id.

One error code: `APP_NOT_LIVE`, answered when a package check is asked for an app with no live
production deploy. `PLAN_FEATURE` can name `package_scanning`. The skill tells agents about
package scanning on the Business plan: `whisk status` counts what to fix, `whisk scan` lists it
with the version that fixes each finding, and `whisk scan --now` checks again after a deploy.

The skill opens with "Whisk already does this", a table of each built-in and the outside services
it replaces, and its "Do not" list says not to add one. `SENTRY_DSN` points at Whisk's own error
store; initialise the SDK with the DSN only. Nothing in the contract itself changed.

`POST /orgs/:org/apps/:app/email/send` on Whisk's domain sends from the business's own address
there, `<name>.<org>@<domain>`: `<app>` without `from`, or the mailbox the `from` names.
`shared_from`, added the same day, is the business's pattern, `*.<org>@<domain>`.

The skill now names every feature an app can use. New in it: every `whisk.yaml` key in one
example (`routes.challenge`, `routes.csrf_off`, `routes.headers`, `health.timeout`, `build`,
`static`, `always_on`, `previews`), calling another of the org's apps by its internal name with
`calls`, connected accounts and their token route, restore points, restores and exports,
webhook `ip_allowlist`, `hmac`, delivery lists and replays, and looking after a live app
(status, rollback, error groups, GitHub copy). Nothing in the contract itself changed.

Every app cookie is sent as `Priority=High` unless it names a priority of its own. A GET, HEAD or
OPTIONS from another site's page that is not a navigation (`Sec-Fetch-Mode` other than
`navigate`: a fetch, an image, a script, a websocket) arrives without cookies, so it is answered
as nobody; `routes.csrf_off` routes keep them. Doctor gains W090, a warning for an app's own
password field. Two error codes: `ORG_UNDER_REVIEW`, answered for every change to a business
held for abuse review, and `EMAIL_LINK_BLOCKED`, for an app email linking to an address a
phishing or malware list names.

On a paid plan apps live under their business's address: an app is at `<app>.<org>.whisk.page`
and a preview at `<app>--<branch>.<org>.whisk.page`. On the free plan they stay at
`<app>--<org>.whisk.page`. When the plan crosses between the two, an app moves and keeps the
address it had, listed in its domains with kind `former`: a browser's GET or HEAD there is sent on
with a 308, and anything else, such as a webhook, is answered as before. `WHISK_PUBLIC_URL` names
the new address from the app's next deploy. Business slugs that are the
platform's names or hold a well-known service's name are no longer given out (`INVALID_REQUEST`
with `details.reserved` for the second).

The edge binds every app cookie to the app's own address with the `__Host-` prefix: in the browser
a cookie the app sets as `sid` is `__Host-sid` (`Path=/`, `Secure`, no `Domain`) and it reaches
the app as `sid`. A cookie without the prefix no longer reaches the app and is deleted in the
response. A request other than GET, HEAD or OPTIONS carrying the app's own cookies, not only the
platform session, now needs to come from the app's own pages (`CSRF_REJECTED`). Every app response
carries `Origin-Agent-Cluster: ?1`. Server-side names, headers and variables keep their meaning;
only page script that reads or writes cookies by their bare name sees the difference.

`GET /orgs/:org/email` gains `monthly_limit`, `sent_month` and `billed_past_daily`.
`EMAIL_RATE_LIMITED` for an allowance gains `details.period` (`day` or `month`). On a plan that
prices email, a send past the day's allowance now goes out and is billed rather than refused.

New routes for Whisk's own payment pages: `GET /orgs/:org/billing/pay/:id`, `POST
/orgs/:org/billing/pay/:id/start {name, address, tax_id}`, `POST /orgs/:org/billing/pay/:id/finish`
and `POST /orgs/:org/billing/invoices/:invoice/pay`. `PUT /operator/payments/stripe` takes
`publishable_key` beside `api_key`, and `GET /operator/payments` answers `own_pages`.

New operator routes `GET /operator/payments`, `PUT /operator/payments/stripe {api_key}` and
`DELETE /operator/payments/stripe`: the platform's Stripe account. New errors `STRIPE_KEY_INVALID`
(400, `details.reason`) for a key that is malformed, refused by Stripe or may not make the webhook,
and `STRIPE_ACCOUNT_IN_USE` (409) for a key for another account than the one businesses pay
through.

New operator routes `GET /operator/email`, `PUT /operator/email/resend {api_key}` and `DELETE
/operator/email/resend`: the platform's own mail sender. New error `EMAIL_SENDER_INVALID` (400,
`details.reason`, `details.tried`): a Resend key that is malformed, refused by Resend, or cannot
send from any of the platform's domains.

`POST /orgs/:org/apps/:app/email/send` no longer needs `from`: without it, or with a `from` on
Whisk's own sending domain, the mail goes from Whisk's shared address under the app's name. A
`from` on any other domain still needs that domain verified. No existing field, header or variable
changes meaning.

A cron function run by `whisk cron run` receives `inngest/scheduled.timer` with `data.cron`, as
a scheduled run does, instead of `whisk/cron.run.<function id>`. Runs still list as
`trigger: cron`, `manual: true`. No other field, header or variable changes meaning.

### 2026-10-04

`POST /orgs/:org/apps/:app/restart` is served: it starts production's live build in a new
container and answers the app, or `INVALID_REQUEST` when nothing is live.

Doctor reads a `fuzz/` folder as test code, like `test/` and `tests/`: environment reads there
(W030) are not reported.

`WHISK_STORAGE_ENDPOINT` is reachable from the app's container and from browsers, and clients
use path-style addressing (the bucket in the path). `whisk init` and `whisk apps create` no
longer take `--region`, and the API ignores a `region` sent when creating a business or an app.
Responses keep `region`. No other field, header or variable changes
meaning.

New operator routes `PUT /operator/orgs/:org/comp {plan, ends_at?, reason}` and `DELETE
/operator/orgs/:org/comp`, and CLI commands `whisk operator comp start|end|list`: a business on a
paid plan free of charge. `GET /operator/orgs` gains `comp {plan, ends_at, reason, by, comped_at}`
and `GET /orgs/:org/billing` gains `comp {ends_at}` while one is in force. No error codes are added
and no existing field changes meaning.

A JSON request body is one JSON value: anything after it but white space answers
`INVALID_REQUEST` instead of being dropped. The workflow engine reads field names without regard
to case, so the platform reads an app's function registration and its `WaitForEvent` answers the
same way (`eventnames.PrefixWaits`): a key spelled in another case is scoped and named like its
own spelling, a registration that spells one field two ways is refused, and a wait opcode that
does is dropped. No field, header or variable changes meaning for an app that spells them as
documented.

### 2026-10-03

Agent tokens from `whisk login` are bound to the computer that signed in. `POST /device/code`
takes `{public_key, device_name?, os?}` and refuses a request without `public_key`
(`INVALID_REQUEST`, fix: `whisk update`); `GET /device/:code` gains `requested_from {device_name,
os, ip, country, network}`. Every request with a bound token carries `Whisk-Signature: t=<unix>,
sig=<base64>` (the `tokensig` package builds and checks it); one without a valid signature answers
the new `TOKEN_SIGNATURE` (401, `details.reason`, `details.device`), which git also answers for a
bound token given as a password. New route `POST /tokens/git` trades a bound token for a
ten-minute git password. `GET /whoami` and token lists gain `bound` and `device_name`. Unbound
tokens keep working as plain bearer tokens. No other field, header or variable changes meaning.

New error `CONNECTION_CLIENT_INVALID` (400, `details.provider`, `details.field`): the operator
set Whisk's own client ID or secret for a sign-in provider with one that is empty, holds spaces
or line breaks, or is too long. No existing field, header or variable changes meaning.

Log forwarding: `GET` and `POST /orgs/:org/logs/destinations`, `POST
/orgs/:org/logs/destinations/:id/test` and `DELETE /orgs/:org/logs/destinations/:id` send every line
of an org's apps to its own HTTPS endpoint, Datadog or OpenTelemetry collector on a plan with the
new `log_forwarding` limit; `whisk logs forwarding [test <id>]`. A new error,
`LOG_DESTINATION_FAILED` (422, `details.status`, `details.answer`), `PLAN_FEATURE` names the
setting `log_forwarding`, and a new notification kind, `logs.failing`. No existing field, header
or variable changes meaning.

SKILL.md no longer points at "this repository": the files it names are at
`https://skill.whisk.run/<file>`, and the starter apps come from `whisk init --template ts|py|go`.
Wording only; nothing an agent does changes.

An approval is decided by an owner or a person its `to` names (CONTRACT.md §6): a role covers that
role and every role above it, `group:<name>` its members, `user:<email>` that person, and several
subjects may be listed separated by commas. Anyone else gets `FORBIDDEN_ROLE`, which a git push
from a token whose person cannot deploy now also answers. No field, event or helper changes.

Previews start on request. Pushing a branch other than the default only stores it; `POST
/orgs/:org/apps/:app/previews {branch}` starts the branch's preview (202 `{environment,
deploy}`), and from then on every push to the branch deploys it. `whisk deploy` from a branch
calls it after the push, and `whisk envs start <branch>` calls it alone. `GET
/orgs/:org/apps/:app/branches` lists the repository's branches with their previews. An
environment gains `status` (`active` or `sleeping`): previews sleep when idle and wake on the
next request, and `POST /apps/:app/sleep` takes `{environment}` to sleep one. New error
`PLAN_LIMIT_PREVIEWS` (402, `details.limit`, `details.plan`, `details.previews`): one preview
per app on the free app, three on paid plans. `previews.ttl_days` is now applied, counts days
since the last push, start or visit, and defaults to 3; before, every preview lasted 7 days after
its last push whatever it said. Previews run no workflows, scheduled functions or events: a call
to the workflow API from a preview's container answers the new `PREVIEW_NO_WORKFLOWS` (409), where
it used to reach the app's own functions. No other field, header or variable changes meaning.

### 2026-10-02

Four new errors for demo businesses, which an operator makes for another company and hands over
with a link: `DEMO_NOT_FOUND` (404), `DEMO_CLAIMED` (409, `details.org`), `DEMO_WRONG_DOMAIN`
(403, `details.domain`) and `DEMO_UNCLAIMED` (409), the answer to a plan change or trial on a demo
nobody has claimed yet. New routes: `GET /demos/:code` and `POST /demos/:code/claim`. No existing
field, header or variable changes meaning.

A business's own domain: `GET/POST /orgs/:org/domains`, `POST /orgs/:org/domains/:id/verify` and
`DELETE /orgs/:org/domains/:id` put every app at `<app-slug>.<domain>` once a TXT and a wildcard
CNAME are in place; `whisk domains business add|verify|show|remove`. A new error,
`ORG_DOMAIN_EXISTS` (409, `details.domain`). An app's domain list gains the kind `business`. No
existing field, header or variable changes meaning.

New error `DATABASE_CREDENTIALS_REJECTED` (503) for `db query` and `db schema` when an
environment's database refuses the password Whisk holds for it, which used to answer
`NODE_DOWN`; on a deploy it is the `cause_code` of a `PLATFORM_DEPLOY_FAILED` with step code
`MIGRATE_FAILED`. No existing field, header or variable changes meaning.

An app may be linked to a GitHub repository: `GET /orgs/:org/github`, `POST
/orgs/:org/github/installations`, and `GET`, `PUT`, `DELETE /orgs/:org/apps/:app/github` with
`POST .../github/sync`. New errors: `GITHUB_NOT_SET_UP` (503), `GITHUB_NOT_INSTALLED` (409),
`GITHUB_INSTALLATION_UNVERIFIED` (403), `GITHUB_INSTALLATION_TAKEN` (409),
`GITHUB_REPO_UNAVAILABLE` (422), `GITHUB_REPO_TAKEN` (409) and `GITHUB_REPO_NOT_EMPTY` (409). A
new notification kind, `github.broken`. No existing field, header or variable changes meaning.

New errors for the operator's Google Search Console connection on the weekly numbers:
`SEARCH_KEY_INVALID` (400) for a key that is not a service account's JSON key, and
`SEARCH_KEY_REJECTED`, `SEARCH_ACCESS_DENIED`, `SEARCH_API_DISABLED` and `SEARCH_UNAVAILABLE`,
which an import records for the operator to read. No existing field, header or variable changes
meaning.

The skill lists `whisk clone <org>/<app>`, which copies an existing app's code into a new
directory where plain git then works.

Custom domains: a bare domain verifies with A or AAAA records to the app hostname's addresses
instead of a CNAME. A domain gains `addresses` while unverified, `DOMAIN_UNVERIFIED` gains
`details.records`, `details.addresses` and `details.missing` as documented (replacing the
undocumented `txt_ok` and `cname_ok`), and `cert_status` becomes `issued` once the certificate is
served. Adding a name under the platform's own domains is refused with `INVALID_REQUEST`.

Agencies add client businesses themselves: `GET`/`POST /orgs/:org/clients`, `POST
/orgs/:org/clients/:client/billing` and `DELETE /orgs/:org/clients/:client`. `TRIAL_USED` is
also the answer to a second free client month (`details.client_trial_org`). `GET /billing/plans`
gains `client_monthly_usd`, `client_annual_usd` and `client_intervals` on every plan, and the
Agency plan's `monthly_usd` and `annual_usd` are now 0. `GET /orgs/:org/billing` gains
`client_of` on a client business. No existing field, header or variable changes meaning.

A new error: `FREE_APP_TAKEN` (402, `details.held_by`), the answer to the first production deploy
of an app in a business on the Free plan whose free app another business has: each person and
each work email domain gets one free app. `TRIAL_USED` is also the answer when the person asking
has had their trial on another business (`details.used_on`). `GET /orgs/:org/billing` gains
`free_app` (`included`, `held_by`) and `trial.used_elsewhere`. `POST /orgs` answers an `active`
org when the person creating it already runs a confirmed business. No existing field, header or
variable changes meaning.

A new error: `TRIAL_USED` (409, `details.trial_used_at`), the answer to a second request for the
free Starter trial; each business gets one. `GET /orgs/:org/billing` gains `trial` (`plan`,
`days`, `available`, `active`, `ends_at`, `used_at`). No existing field, header or variable
changes meaning.

The edge caches answers the app marks cacheable: a `200` to a `GET` with `Cache-Control`
`max-age` or `s-maxage` above zero, no `no-store`, `no-cache` or `Set-Cookie`, and a body of at
most 1 MiB, for at most an hour, per person and per live deploy, honouring `Vary`. A repeat
request is answered by the edge without reaching or waking the app. A new response header,
`X-Whisk-Cache` (`hit` or `miss`), says when an answer was eligible (`headers.md`, edge response
cache). The skill (§2) tells agents to send `Cache-Control: private, max-age=<seconds>` on pages
and reads that may be a little old. No field, variable or error changes.

§2 of the skill gains "Keep it light": stream large files and results, work in small steps, let
the database filter, prefer events over polling schedules. §13 gains the matching two lines.

§7 of the skill says an app woken only for a function run sleeps again about 30 seconds after
the run's last step when nobody visits it meanwhile. No field, header, variable or error
changes.

### 2026-10-01

A new error: `FREE_CAPACITY_BUSY` (503 with `Retry-After`, `details.retry_after`). A Free plan
app waits for it when every free app on its node is in use; a browser sees the starting page,
which tries again by itself. No field, header or variable changes.

A new error: `APP_CPU_SLEEP`, the log line written when a Free plan app kept its whole processor
share busy for ten minutes and was put to sleep; the next request wakes it. A function's trigger
in `GET /orgs/:org/apps/:app/functions` gains `runs_as`, the schedule it actually runs on when the
plan spaces a cron more widely than written (Free: at most every 10 minutes). The skill says so.
`manifest.SpacedCron` is the helper that computes it. No existing field changes meaning.

### 2026-09-30

`platformpage` renders the page the edge and the platform answer a browser navigation with when
there is no app page to show, in the Workbench Mustard design. It changes no field, header or
error body; the JSON answers are as before.

§1 of the skill says what an app is to the people an agent works for and gives the rules for
adding to an existing app or starting a new one. No field, header or variable changes.

### 2026-09-29

An agent identity is a coding agent's own standing on the platform, acting for one person, who
chooses the categories of change it may make (`read`, `operate`, `change`, `destroy`; never
`human`). Its credential is an agent token (`whsk_agent_…`) with no org; `GET /whoami` answers
`identity {id, name, categories}` for it. Two new errors: `AGENT_CATEGORY` (403, the route is in a
category the identity does not hold, `details.required` and `details.categories`) and
`AGENT_PAUSED` (403, its person paused it). The CLI exits with the auth code for both.

Senders read their own feedback: `GET /v1/feedback` lists what the caller and its org sent,
every status by default, and `GET /v1/feedback/:id` shows one, to anyone for feedback sent
without signing in. Each carries `status`, `sent_by_you` and `note`, the Whisk team's word to the
sender, which `POST /v1/operator/feedback/:id` now takes as `note`. The CLI adds
`whisk feedback show <id>` and `--note` on resolve and reopen; `whisk feedback list` lists the
caller's own, and `--everyone` lists every sender's for operators. The skill (§12) tells agents to
check back.

The skill (§5) says how an app in a shared database gives another app access to its schema:
the schema and role names, and the `GRANT` statements the owning app runs in a migration.

`priority.run` works: the platform makes the expression a whole number, which the engine needs,
so `"event.data.boost"` reorders waiting runs. `workflows.md` lists it as supported.

An operator read token may carry `feedback:resolve`, which the operator names when creating it
on the operator page; with it the token resolves and reopens feedback
(`POST /v1/operator/feedback/:id`, `whisk feedback resolve|reopen <id>`) and still changes
nothing else. Without it that write answers `TOKEN_SCOPE` with `details.required`
`feedback:resolve`. An operator may also name `feedback:resolve` for an agent token.

When an app is killed for going over its memory limit, `whisk logs` shows a line from the
platform at that moment (stream `platform`, starting `whisk:`, naming `APP_OUT_OF_MEMORY` and the
limit), and the org's owners get the new notification `app.out_of_memory` for the first kill of
an app and then at most once per app per 24 hours.

`whisk cron run <function>` starts one run of a cron function now, the way its schedule starts
it, and prints the run id to follow with `whisk runs show`; the API route is
`POST /orgs/:org/apps/:app/cron/:function/run`. The run is listed with `trigger: cron` and the new
field `manual: true`, and its event is `whisk/cron.run.<function id>` with `data.cron`: the
platform gives every cron function that event trigger at registration, and an app may no longer
send, trigger on or cancel on an event in the `whisk/cron.run.` family. New code
`FUNCTION_NOT_LIVE` (the function's app has no live deploy, its container stopped, the engine
does not hold the function with its cron-run trigger yet, or the function has no room for one).

`whisk doctor` W031 counts a secret as read when its name is a whole string literal anywhere in
code, so secrets read through a table of names (`env[KEYS.hubspot]`) are no longer reported as
unused.

Every failed deploy and failed run says whose side it failed on in `details.fault` (`app` or
`platform`), and a code starting `PLATFORM_` is always Whisk's. New codes:
`PLATFORM_DEPLOY_FAILED` (a deploy that failed on Whisk's side; the part that failed is
`details.cause_code`, and `IMAGE_PULL_FAILED`, `POLICY_APPLY_FAILED`, `DATABASE_FAILED` and
`NODE_DOWN` reach a deploy that way), `PLATFORM_RUN_INTERRUPTED` (Whisk cut the run off and runs
it again once when no step had finished; the new run is `rerun_run_id`), `RUN_APP_UNREACHABLE` and `RUN_FAILED`. A run
carries `error`, `rerun_run_id` and `rerun_of`. `whisk deploy` exits 5 for a failure on Whisk's
side.

Event names are plain everywhere: write `"po.created"` in triggers, `cancelOn`,
`step.waitForEvent` and sends, and the platform keeps it to your app. The templates' `eventName`,
`event_name` and `eventTrigger` helpers are gone; code that still uses them keeps working
unchanged. The approval helper waits for `whisk/approval.decided`, which now resumes the run.
`workflows.md` documents what the workflow engine supports, feature by feature: everything
except `priority`. A trigger on `inngest/function.*` never fires, and `step.invoke` of another
app's function fails with the new code `INVOKE_FOREIGN`.

An agent token does only what its grant scopes allow: deploying, creating or changing apps and
restoring or opening a database need `deploy`, declaring secrets and starting connections need
`secrets:declare`, and logs and error groups need `logs:read`. A token without the scope is
`TOKEN_SCOPE` with `details.required` and `details.token_scopes`. `whisk login` grants all three,
so an agent that logged in is unaffected.

`TOKEN_SCOPE` also answers an operator read token (`operator:read`) that tries anything but a
read. `whisk operator logs|units|nodes|deploys` read the platform for its operators; tenant
commands are unchanged.

Sign-in is Whisk's own on `auth.<domain>`: passkeys, passwords, emailed six-digit codes and, for
orgs whose plan includes it, company SSO. Nothing an app reads changes: the identity headers, the handoff,
`/.whisk/login` and `/.whisk/logout` are as they were. `SSO_DOMAIN_UNVERIFIED` (409) answers
enabling SSO for an email domain whose `_whisk-sso` TXT record is not in place, and `DOMAIN_TAKEN`
also answers a domain another org already uses for SSO. `CSRF_REJECTED` and `RATE_LIMITED` can
come from the sign-in host (`auth`). A customer invitation link is the pool's sign-in page with
the email filled in.

`POST /v1/feedback {kind?, message, org?, app?, context?}` sends feedback to Whisk, with or
without a credential, and answers `{id, kind, org, app, redacted, created_at, message}`. `kind`
is `bug`, `difficulty` (default), `idea` or `praise`. More than the hourly allowance answers
`RATE_LIMITED` with `details.scope` `feedback`. The skill gains section 12, "Tell Whisk what got
in your way", and "Do not" becomes section 13.

Doctor's W030 treats every `WHISK_` name as the platform's, so a variable newer than the CLI is
never reported as undeclared. `whisk deploy`'s `--json` summary carries `environment` and, for a
preview, `note`, the sentence saying production is unchanged. `whisk secrets set` takes several
names and its `NEEDS_HUMAN` link, like the deploy's for unset secrets, is the secrets page with
`?set=<names>`, which opens a paste form ready for them. `whisk runs list <function>` names the
function as an argument. Logs scrub the values of declared secrets and platform credentials;
values under `env` are configuration and appear as printed.

Doctor's W022 and W050 read a plain `node:http` server's path comparisons (`req.url ===
"/path"`, `case "/path":`) as routes, and a skipped route check says which route went
unconfirmed and which forms doctor reads.

The templates ship a tree helper for files a job keeps between runs: a tree lives under
`<WHISK_STORAGE_PREFIX>trees/<name>/` with an index of SHA-256 sums at
`<WHISK_STORAGE_PREFIX>trees/<name>.index.json`, the same layout in all three languages, and
`GET /diag/sync?tree=&file=&value=` round-trips one. Rule 4 says `/tmp` is emptied on every
restart and points at storage for files that must last.

API responses carry `X-Whisk-CLI-Version`, the CLI the platform serves. CLI `--json` output
gains `cli_update` `{current, latest, fix}` when the running CLI differs from it, and an unknown
command or flag is `UNKNOWN_COMMAND`, whose fix is `whisk update`.

### 2026-09-11

`SECRET_SHREDDED` (410) answers any read of a secret whose org has been shredded: its data key
was destroyed, so the value cannot be opened by anyone.

`GET /v1/billing/plans` lists every plan with its limits and overage prices, and
`GET /v1/orgs/<org>/billing` an org's plan, usage and payment timeline; an org's JSON carries
`timeline` with the dates that matter next. `PUT /v1/orgs/<org>/billing/plan {plan, interval}`
answers the org, or 202 `{checkout_url}` when a card is needed first, and refuses a plan the org
does not fit with `PLAN_DOWNGRADE_BLOCKED`. On a plan without run overage, an event sent after the
month's runs are used is kept and answered with `PLAN_LIMIT_RUNS`, and delivered when the month
turns or the plan changes.

`POST /v1/orgs/<org>/export` starts an export of the whole org and answers 202 with the export;
`GET /v1/orgs/<org>/export/<id>` follows it and carries a link once it is `done`. One runs at a
time (`EXPORT_IN_PROGRESS`), only a person may start one, and a failed export records
`EXPORT_FAILED` with the piece it stopped at. `whisk export --wait` does both and prints the link.

### 2026-09-10 (later)

`customer_identity: app | org` gives an app a second audience. Its people reach it with
`X-Whisk-Audience: customer`, `X-Whisk-User-Id` naming the customer and no `X-Whisk-Roles` or
`X-Whisk-Groups`, because a customer is a member of nothing. An app onboards its own with
`POST /v1/orgs/<org>/apps/<app>/customers {email, name}` and its service token, which answers
the invitation link.

`POST /v1/orgs/<org>/apps/<app>/email/send` takes `to` as one address or a list, and answers
202 with what the platform took. A message may name at most 20 recipients and an org may reach
at most 100 a minute: more is a mailing, and the platform sends transactional mail. `from` must
be on a domain the org has verified, which is `EMAIL_DOMAIN_UNVERIFIED` when it is not.

An upload is authorised as a form rather than a URL: `POST /uploads {filename, content_type,
max_bytes}` answers `{id, key, url, fields, expires_at}` to post as `multipart/form-data`, which
is what lets the object store itself enforce the size and the type.
`GET /uploads/<id>` gives a link to read one back, and answers `OBJECT_QUARANTINED` for a file
the scanner flagged. `UPLOAD_TOO_LARGE` is now about the 100 MB an object may be, since the
per-form limit is the store's to refuse. No field changed shape, so version 1 stands.

### 2026-09-10

`kv: true` gives an app a Valkey of its own rather than a share of the node's behind an ACL, so
keys need no prefix, one tenant can never evict another's, and the cache empties when the app
sleeps; `WHISK_KV_URL` and the key-value paragraphs say so. `customer_identity` gained the
sentence that registration is a setting an owner turns on, not something the manifest decides.
No field changed shape, so version 1 stands.

### 2026-09-07 (later)

The stub became package `stub` with `cmd/whisk-stub` as its command, so `whisk dev` can embed it;
`manifest.NextCron` computes cron next-run times for `whisk cron list`. Doctor idioms gained the
shared env-helper form (`env("NAME")`), W022's safe fix is the manifest edit, W042 ignores names
built at runtime, and W070 assumes the Free plan when the platform is not asked.

### 2026-09-07

Added container startup diagnostics: INIT_CONFIG, INIT_PROCESS, INIT_EXEC, INIT_SECRETS and
INIT_SECRET_FILES. Existing token error codes remain unchanged.

First publication: manifest schema, graph schema, headers, environment, webhook presets
(Stripe, GitHub, Shopify, Xero, Slack, HubSpot, Twilio, Zoom, Linear, Standard Webhooks, plus
`hmac` and `token`), error catalogue, doctor rules, the skill, the Go module and `whisk-stub`,
and the three templates.
