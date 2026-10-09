# Error catalogue

Every error the platform, the edge, the git hooks and the CLI produce has a stable code. The wire
format is the same everywhere:

```json
{
  "error": {
    "code": "SECRET_IN_COMMIT",
    "message": "A value that looks like a Stripe secret key is in src/config.ts line 12.",
    "fix": "Remove the value, declare STRIPE_SECRET_KEY under secrets in whisk.yaml, read it from the environment, and ask an owner to set it at https://whisk.run/o/acme/secrets.",
    "docs": "https://skill.whisk.run/errors/SECRET_IN_COMMIT",
    "details": { "file": "src/config.ts", "line": 12, "rule": "stripe-secret" }
  }
}
```

`code` is what an agent branches on. `message` says what happened, in one sentence, with the
specific names involved. `fix` says what to do next, in imperative sentences an agent can follow
without reading anything else. `docs` links to the section below. `details` is structured and
differs by code. Codes are upper snake case and never reused for a different meaning; new codes
are additive within a conventions version.

Each section states the HTTP status where one applies (`-` for codes that arrive in a deploy
record, a notification or CLI output rather than an HTTP response), when it occurs, the `fix`
text, and an example. `Surface` names where it comes from: `api`, `edge` (an app hostname),
`hooks` (the webhook ingress), `auth` (the sign-in host), `git` (pre-receive), `cli`, `deploy` (a deploy or build record),
`run` (a workflow run), `container` (whisk-init, in the app's own log), `node` (the node agent's answer
to the control plane, which lands in the deploy record and the operator's view).

## AUTH_REQUIRED

Status: 401 · Surface: api, edge

When: no credential was presented. On the API, no bearer token or session. On an app hostname, a
private route was fetched without a session, or a service route (`queue.endpoint`, a webhook
handler) was requested from the internet.

Fix: For the API, run `whisk login` and retry. For an app route, sign in by navigating to
`/.whisk/login?return=<path>` on the app, or make the route public in `routes.public` if it is
meant for everyone.

```json
{"error":{"code":"AUTH_REQUIRED","message":"This route requires a signed-in user.","fix":"Navigate to /.whisk/login?return=/reports to sign in, or add the route to routes.public in whisk.yaml if it is meant for everyone.","docs":"https://skill.whisk.run/errors/AUTH_REQUIRED","details":{"route":"/reports","audience":"anonymous"}}}
```

## TOKEN_EXPIRED

Status: 401 · Surface: api, git

When: the agent, deploy or service token has passed its expiry.

Fix: Run `whisk login` to obtain a fresh agent token. For CI, create a new token with
`whisk agent-token create` and update the `WHISK_TOKEN` secret in the CI system.

```json
{"error":{"code":"TOKEN_EXPIRED","message":"The agent token expired on 2026-10-07T09:12:00Z.","fix":"Run whisk login to obtain a new token.","docs":"https://skill.whisk.run/errors/TOKEN_EXPIRED","details":{"expired_at":"2026-10-07T09:12:00Z","kind":"agent"}}}
```

## TOKEN_REVOKED

Status: 401 · Surface: api, git

When: a human revoked the token from the dashboard, or the person it acted for left the org.

Fix: Run `whisk login`. If it was revoked deliberately, ask an owner or admin of the org why
before continuing.

```json
{"error":{"code":"TOKEN_REVOKED","message":"This token was revoked by ana@acme.example on 2026-09-30T14:02:11Z.","fix":"Run whisk login to obtain a new token, and ask ana@acme.example whether the revocation was intended.","docs":"https://skill.whisk.run/errors/TOKEN_REVOKED","details":{"revoked_at":"2026-09-30T14:02:11Z","revoked_by":"ana@acme.example"}}}
```

## TOKEN_SCOPE

Status: 403 · Surface: api

When: the token is valid but not scoped to this org, this app, or this action. An agent token
does only what its grant scopes allow, even when the person it acts for may do more: deploying,
creating or changing apps and restoring or opening a database need `deploy`, declaring secrets
needs `secrets:declare`, and reading logs and error groups needs
`logs:read`. `details.required` names the missing scope and `details.token_scopes` lists the
token's own. Agent tokens never carry `secrets:read`, so reading a secret value always fails
with this code. An operator read token (`operator:read`) only reads: any other method, and any
route outside the operator reads and its person's org reads, answers this code. The one write it
may make is setting a piece of feedback's status, and only when it also carries
`feedback:resolve`; without it, that write answers this code with `details.required`
`feedback:resolve`. An app's own token sending an event is refused with this code when it is tied
to no deploy the platform knows, and a workflow registration from an app's SDK is refused with it
when it names another app or any URL, at the top or on a step, other than the app's serve URL
(`details.field` names the first such field, `details.url` the serve URL).

Fix: Use the org and app the token was issued for (`whisk whoami` shows them), or ask a human to
create a token with the needed scope with `whisk agent-token create --scopes`, or run
`whisk login`, which grants `deploy`, `logs:read` and `secrets:declare`. Send events with the
`WHISK_SERVICE_TOKEN` the container received, and leave the workflow SDK's serve origin and path
unset so it registers at the URL the platform gives it. Secret values can never
be read; declare the name and let a human set it. With an operator read token, make the change
signed in, or with an agent token from `whisk login`; to resolve feedback with one, ask the
operator for a read token created with `feedback:resolve` on the operator page.

```json
{"error":{"code":"TOKEN_SCOPE","message":"This token is scoped to app job-tracker and cannot act on app reporting-api.","fix":"Run whisk use acme/job-tracker, or ask an admin for a token scoped to reporting-api.","docs":"https://skill.whisk.run/errors/TOKEN_SCOPE","details":{"required":"app:01J9X4","token_scopes":["org:01J9W2","app:01J9X3","deploy"]}}}
```

## BUSINESS_NOT_IN_LOGIN

Status: 403 · Surface: api, git

When: a `whisk login` was used in a business it does not cover, though the person it acts for
belongs to that business. A login covers the businesses it was approved for: a login for all
the person's businesses covers the ones they belonged to when they approved it, and never one
they joined later; a login for one business covers that business. A person can add a business
to either without a new login. `details.url` is the page where they do it, `details.org` and
`details.org_name` name the business, `details.login` is the login's id and `details.device` the
computer it is bound to. The CLI prints this as `NEEDS_HUMAN` with the link.

Fix: Show the person `details.url`. They open it signed in, check the login is theirs by its
computer and when it was made, and tap Add; then retry. No new login is needed. Do not run
`whisk login` again for this.

```json
{"error":{"code":"BUSINESS_NOT_IN_LOGIN","message":"This login does not cover Northwind Bakery.","fix":"Ask the person to open https://whisk.run/me/logins/01J9Y2?add=northwind, check the login is theirs and tap Add Northwind Bakery, then retry. No new login is needed.","docs":"https://skill.whisk.run/errors/BUSINESS_NOT_IN_LOGIN","details":{"org":"northwind","org_name":"Northwind Bakery","url":"https://whisk.run/me/logins/01J9Y2?add=northwind","login":"01J9Y2","device":"ana-laptop"}}}
```

## TOKEN_SIGNATURE

Status: 401 · Surface: api, git

When: the agent token is bound to the computer that ran `whisk login`, and the request did not
prove it came from there. On the API, the `Whisk-Signature` header is missing, cannot be read,
was made more than five minutes from the platform's clock, or does not match the request and
the token's key: the token was copied to another computer, or sent by something other than the
whisk CLI. On git, the bound token itself was given as the password, which git cannot sign.
`details.reason` is `missing`, `malformed`, `stale`, `mismatch` or (git) `git`, and
`details.device` names the computer the token belongs to.

Fix: Run `whisk login` again on this computer and use the whisk CLI or `whisk mcp`, which sign
every request; never copy the token elsewhere. For `stale`, set this computer's clock to the
right time. For git, use `whisk deploy`, or let `whisk git-credential` answer git (`whisk clone`
sets it up). For CI, use a token from `whisk agent-token create` in `WHISK_TOKEN`, which is not
bound.

```json
{"error":{"code":"TOKEN_SIGNATURE","message":"This token is bound to the computer that signed in, and this request carries no Whisk-Signature.","fix":"Run whisk login again on this computer, and use the whisk CLI (or whisk mcp) rather than sending the token yourself.","docs":"https://skill.whisk.run/errors/TOKEN_SIGNATURE","details":{"reason":"missing","device":"ana-laptop"}}}
```

## FORBIDDEN_ROLE

Status: 403 · Surface: api, git

When: the person is signed in but their org role does not allow the action. The authorisation
matrix is in the control plane specification: deploys need developer or above, secret values
need admin or above, members and access need admin or above, billing needs owner or billing.
An app's own service or container token has no org role: on a route that needs one it gets this
code, a message naming the routes it can call (its queue, uploads, customers and
email) and a fix pointing at `POST $WHISK_QUEUE_URL` for events. A deploy token only
pushes to git. On git, an agent token whose person is not in the app's org, or whose role does
not deploy (billing, member), is refused the push with this code, as the deploy route would
refuse it.

Fix: Ask an owner or admin of the org to perform the action or to change your role. The message
names the role that would be enough.

When the role allows the action and an agent token never may (changing billing, members,
exports), the message says it is the token's limit, `details.token` is `agent`, and the fix
points at the dashboard, where the person does it signed in. An agent acting for an owner or
billing contact reads billing (`whisk billing`).

```json
{"error":{"code":"FORBIDDEN_ROLE","message":"Setting a secret value needs the owner, admin or developer role; you are billing.","fix":"Ask an owner, admin or developer of acme to set STRIPE_SECRET_KEY at https://whisk.run/o/acme/secrets, or to change your role.","docs":"https://skill.whisk.run/errors/FORBIDDEN_ROLE","details":{"required_any":["owner","admin","developer"],"role":"billing"}}}
```

## AGENT_CATEGORY

Status: 403 · Surface: api

When: an agent identity (a coding agent's own standing on the platform, acting for one person)
called a route in a category of change it does not hold. Every route is in one category: `read`,
`operate` (deploy, re-run, cancel, resolve feedback, take a restore point), `change` (add or
change records), `destroy` (replace or remove something the platform can undo) or `human`.
`human` covers who can do what, money, secret values, credentials and everything that cannot be
undone, and no agent identity may ever hold it. Naming a code to run destructive SQL with
`whisk db query` needs `destroy`. `details.required` is the category the route needs and
`details.categories` the identity's own.

Fix: For `human`, ask your person to do it signed in at the dashboard; nothing makes an agent
able to. For any other category, ask your person to add it to the identity on the operator page
(whisk.run/operator, Agent identities). The change applies to your next request; your credential
stays the same.

```json
{"error":{"code":"AGENT_CATEGORY","message":"This agent identity does not hold the Destructive category.","fix":"Ask your person to add Destructive to the identity on the operator page, or to do this signed in.","docs":"https://skill.whisk.run/errors/AGENT_CATEGORY","details":{"required":"destroy","categories":["read","operate","change"]}}}
```

## AGENT_PAUSED

Status: 403 · Surface: api

When: the agent identity this credential belongs to is paused. Its person paused it on the
operator page, and every request it makes is refused until they resume it.

Fix: Stop and tell your person the identity is paused; only they resume it, on the operator
page. Do not look for another credential to carry on with.

```json
{"error":{"code":"AGENT_PAUSED","message":"The agent identity this credential belongs to is paused.","fix":"Stop and tell your person; only they resume it, on the operator page at whisk.run/operator.","docs":"https://skill.whisk.run/errors/AGENT_PAUSED","details":{"identity":"Claude"}}}
```

## ORG_PENDING_LIMIT

Status: 403 · Surface: api

When: the org has not been confirmed by a human yet. Pending orgs can deploy previews but
cannot set custom domains, send email, deploy to production, or exceed free-tier limits.

Fix: Ask the human to confirm the account with a work email at the URL in `details`. Deploy a
preview meanwhile with `whisk deploy --env preview`.

```json
{"error":{"code":"ORG_PENDING_LIMIT","message":"Custom domains are not available until the account is confirmed.","fix":"Ask the account owner to confirm at https://whisk.run/welcome?org=acme, then retry. A preview deploy works now: whisk deploy --env preview.","docs":"https://skill.whisk.run/errors/ORG_PENDING_LIMIT","details":{"expires_at":"2026-09-09T10:00:00Z","confirm_url":"https://whisk.run/welcome?org=acme"}}}
```

## ORG_FROZEN

Status: 403 · Surface: api, git

When: the org is frozen: for non-payment, by an operator, or because nobody used a free org
for 90 days. Apps serve a maintenance page, deploys are refused, export still works.

Fix: For non-payment, ask the billing contact to update the payment method at the billing URL.
For inactivity, any member signing in to the dashboard, or a member's agent calling the API or
pushing, wakes the org first, so this is answered only to an app's visitors. Export is available
at any time with `whisk export`.

```json
{"error":{"code":"ORG_FROZEN","message":"acme is frozen because the invoice for August is unpaid.","fix":"Ask the billing contact to pay at https://whisk.run/o/acme/billing. Apps resume within a minute of payment. whisk export works while frozen.","docs":"https://skill.whisk.run/errors/ORG_FROZEN","details":{"frozen_at":"2026-09-01T00:00:00Z","shred_at":"2026-10-01T00:00:00Z"}}}
```

## ORG_STATUS_REFUSED

Status: 409 · Surface: api

When: the business's status does not allow the change asked for: restoring a business that is
not being deleted, restoring one being deleted because a payment failed or by Whisk, freezing
one that is being deleted, or anything on a business that is already shredded.

Fix: Read the message: a business deleted for a failed payment is restored by paying what is
owed on the billing page; one deleted by Whisk only through support.

```json
{"error":{"code":"ORG_STATUS_REFUSED","message":"This business is being deleted because a payment failed.","fix":"Pay what is owed on the billing page; that restores it.","docs":"https://skill.whisk.run/errors/ORG_STATUS_REFUSED","details":{"status":"shredding"}}}
```

## ADDRESS_BLOCKED

Status: 403 · Surface: edge

When: something on the request's network address asked three times within ten minutes for paths
only an attacker's scanner asks for (WordPress, PHP, secrets or version control files such as
`/.env` and `.git`), or with a known attack tool's user agent, so the edge is not answering
requests from it on any Whisk address for a while: an hour the first time, a day if it does so
again within 30 days, a week after that, and never more than an hour for an address people are
seen using. A browser opening a page from that address is shown a short browser check instead
(the page carries this code) and, once it passes, gets through for a day. Shared and private
networks' own addresses, and coding agents' shared outbound addresses, are never blocked.

Fix: Open the page in a browser, which passes a short check; otherwise try again later or use
another network. Never request those paths.

```json
{"error":{"code":"ADDRESS_BLOCKED","message":"This network address is blocked for a while because something on it asked for addresses only attackers ask for.","fix":"Open the page in a browser, which passes a short check; otherwise try again later or use another network.","docs":"https://skill.whisk.run/errors/ADDRESS_BLOCKED","details":{}}}
```

## ORG_UNDER_REVIEW

Status: 403 · Surface: api, git, edge, hooks, deploy

When: a check found an app of the business that looks like a scam or malware, or the business
was started by a person, address or computer already held, so it is paused until Whisk reviews
it (CONTROL-PLANE.md §6.27). Apps show a review page; deploys, new apps, previews, app email and
webhooks are refused. Export still works.

Fix: Nothing to do: Whisk reviews every held business and releases it if the check was wrong.
Write to support@whisk.run if it is urgent.

```json
{"error":{"code":"ORG_UNDER_REVIEW","message":"acme is paused while Whisk reviews it.","fix":"Nothing to do: Whisk reviews every held business and releases it if the check was wrong. Write to support@whisk.run if it is urgent.","docs":"https://skill.whisk.run/errors/ORG_UNDER_REVIEW","details":{"org":"acme"}}}
```

## ACCOUNT_LAST_OWNER

Status: 409 · Surface: api, cli

When: a person asked to delete their own account (`DELETE /v1/me`, `whisk account delete`) while
they are the only active owner of one or more businesses, which would be left with nobody in
charge. `details.orgs` names each business, with its People and settings pages.

Fix: For each business named, make someone else an owner on its People page, or delete the
business in its settings, then delete your account again.

```json
{"error":{"code":"ACCOUNT_LAST_OWNER","message":"You are the only owner of Acme (acme), so deleting your account would leave it with nobody in charge.","fix":"For each business named, make someone else an owner on its People page, or delete the business in its settings, then delete your account again.","docs":"https://skill.whisk.run/errors/ACCOUNT_LAST_OWNER","details":{"orgs":[{"org":"acme","name":"Acme","people_url":"https://whisk.run/o/acme/people","settings_url":"https://whisk.run/o/acme/settings"}]}}}
```

## PLAN_LIMIT_APPS

Status: 402 · Surface: api

When: creating another app would exceed the plan's app count.

Fix: Delete an unused app with `whisk apps delete`, or ask an owner to upgrade the plan at the
billing URL.

```json
{"error":{"code":"PLAN_LIMIT_APPS","message":"The Team plan allows 10 apps and acme has 10.","fix":"Delete an app you no longer need with whisk apps delete <slug>, or ask an owner to upgrade at https://whisk.run/o/acme/billing.","docs":"https://skill.whisk.run/errors/PLAN_LIMIT_APPS","details":{"limit":10,"current":10,"plan":"team"}}}
```

## FREE_APP_TAKEN

Status: 402 · Surface: api, git, cli

When: the first production deploy of an app in a business on the Free plan that has no free app.
Every person gets one free app, for the first business they set up, and every work email domain
gets one, for the first business confirmed with it. A further business can be created and can
deploy previews, but publishes only on a paid plan or during its Starter trial.
`details.held_by` names the business that has the free app. A push to the default branch that
would be that deploy is refused at pre-receive with this error, so `whisk deploy` fails at once.

Fix: Ask an owner to start the free Starter trial or choose a paid plan on the business's billing
page. Preview deployments keep working meanwhile.

```json
{"error":{"code":"FREE_APP_TAKEN","message":"Acme Labs has no free app: each person and each work email domain gets one, and Acme Ltd already has it.","fix":"Start the free Starter trial or choose a paid plan for Acme Labs on its billing page. Preview deployments remain available.","docs":"https://skill.whisk.run/errors/FREE_APP_TAKEN","details":{"held_by":"Acme Ltd"}}}
```

## PLAN_LIMIT_MEMBERS

Status: 402 · Surface: api

When: inviting another member or guest would exceed the plan's seat count.

Fix: Remove someone who has left, or ask an owner to upgrade the plan.

```json
{"error":{"code":"PLAN_LIMIT_MEMBERS","message":"The Team plan allows 25 people and acme has 25.","fix":"Remove someone who has left with whisk members remove <email>, or ask an owner to upgrade at https://whisk.run/o/acme/billing.","docs":"https://skill.whisk.run/errors/PLAN_LIMIT_MEMBERS","details":{"limit":25,"current":25,"plan":"team"}}}
```

## PLAN_LIMIT_EMAIL_DOMAINS

Status: 402 · Surface: api

When: an app registers a sending domain of its own for a customer (`POST
/v1/orgs/<org>/apps/<app>/email/domains`) while the business's apps already hold as many as the
plan's `app_email_domains` allows together: Starter 25, Team and Agency 250, Business 1,000.

Fix: Remove a domain no customer sends from any more, by its id, or ask an owner to upgrade.

```json
{"error":{"code":"PLAN_LIMIT_EMAIL_DOMAINS","message":"The Starter plan allows 25 sending domains across a business's apps, and this business's apps hold 25.","fix":"Remove a domain no customer sends from any more by its id, or ask an owner to upgrade at https://whisk.run/o/acme/billing.","docs":"https://skill.whisk.run/errors/PLAN_LIMIT_EMAIL_DOMAINS","details":{"limit":25,"current":25,"plan":"starter"}}}
```

## PLAN_LIMIT_PREVIEWS

Status: 402 · Surface: api, cli

When: starting a preview for a branch would give the app more previews than the plan allows: one
per app on the free app, three per app on every paid plan. A branch that already has a preview
never counts against it. `details.previews` names the app's previews.

Fix: Remove a preview nobody needs with `whisk envs delete preview:<branch>` (or delete its
branch), or wait for one to expire; on the free app, an owner can choose a paid plan for three.

```json
{"error":{"code":"PLAN_LIMIT_PREVIEWS","message":"crm already has 1 preview, the most the Free plan allows for one app.","fix":"Remove one with whisk envs delete preview:<branch> (or delete its branch), then start this one again. Paid plans allow 3 previews per app.","docs":"https://skill.whisk.run/errors/PLAN_LIMIT_PREVIEWS","details":{"limit":1,"plan":"free","previews":["preview:new-invoice-screen"]}}}
```

## PRODUCTION_NEEDS_MAIN

Status: 409 · Surface: api, cli

When: a deploy to production named a branch other than `main` (or `master`), or a commit or build
that the app's default branch does not contain. Only the default branch goes live; any other branch
runs as a preview at its own address. Returning to one of production's own earlier deploys (a
rollback) is always allowed.

Fix: Merge the branch into `main`, then run `whisk deploy` from `main`. To show the branch without
putting it live, run `whisk deploy` from the branch, which starts its preview.

```json
{"error":{"code":"PRODUCTION_NEEDS_MAIN","message":"Only main goes live; new-invoice-screen is a branch.","fix":"Merge new-invoice-screen into main, then run whisk deploy from main. To show it without putting it live, run whisk deploy from new-invoice-screen for a preview.","docs":"https://skill.whisk.run/errors/PRODUCTION_NEEDS_MAIN","details":{"branch":"new-invoice-screen"}}}
```

## PREVIEW_NO_WORKFLOWS

Status: 409 · Surface: api

When: a preview's container called the workflow API: sending an event, registering functions or
recording a run's steps. Previews run no workflows, scheduled functions or events, so a branch's
code never starts production's runs or replaces its functions.

Fix: Test functions locally with `whisk dev`, or merge the branch into `main` and deploy it when
it is ready. In code, skip sending events when `WHISK_ENV` starts with `preview:`.

```json
{"error":{"code":"PREVIEW_NO_WORKFLOWS","message":"Previews run no workflows, scheduled functions or events; this call was not sent.","fix":"Test functions locally with whisk dev, or merge the branch into main and deploy it. Skip sending events when WHISK_ENV starts with preview:.","docs":"https://skill.whisk.run/errors/PREVIEW_NO_WORKFLOWS","details":{"environment":"preview:new-invoice-screen"}}}
```

## PLAN_LIMIT_RUNS

Status: 402 · Surface: api, run

When: the org has used its monthly workflow run allowance on a plan without run overage. The
event is kept, not lost: the platform holds it and delivers it, in order, when the month turns or
the plan changes, and the answer is 402 so the sender knows its run has not started. A scheduled
function is started by the engine on its own clock and is counted but never held.

Fix: Ask an owner to upgrade or to enable overage at the billing URL. Reduce runs by batching
events or removing a chatty cron.

```json
{"error":{"code":"PLAN_LIMIT_RUNS","message":"acme has used 50,000 of 50,000 runs this month; new runs are queued.","fix":"Ask an owner to enable overage or upgrade at https://whisk.run/o/acme/billing. Runs resume on 2026-10-01T00:00:00Z otherwise.","docs":"https://skill.whisk.run/errors/PLAN_LIMIT_RUNS","details":{"limit":50000,"used":50000,"resets_at":"2026-10-01T00:00:00Z"}}}
```

## PLAN_DOWNGRADE_BLOCKED

Status: 409 · Surface: api, cli

When: an owner asked to move to a plan the org does not fit: more apps, members or storage than
it includes, or a feature switched on that it does not include (single sign-on, Promoted apps).
`details.over` names each limit with what the org has and what the plan allows;
`details.features` names each feature to switch off first (`sso`, `promoted`).

Fix: Delete what the smaller plan does not include and switch off the features it lacks (single
sign-on in the business's settings, Promoted apps on each app's page), then change the plan
again.

```json
{"error":{"code":"PLAN_DOWNGRADE_BLOCKED","message":"acme has 12 apps and the Starter plan includes 10.","fix":"Delete 2 apps you no longer need with whisk apps delete <slug>, then change the plan again.","docs":"https://skill.whisk.run/errors/PLAN_DOWNGRADE_BLOCKED","details":{"plan":"starter","over":[{"limit":"apps","has":12,"allows":10}]}}}
```

```json
{"error":{"code":"PLAN_DOWNGRADE_BLOCKED","message":"Acme has single sign-on on and the Starter plan does not include it.","fix":"Turn single sign-on off at https://whisk.run/o/acme/settings (members then sign in with their own email), then change the plan again.","docs":"https://skill.whisk.run/errors/PLAN_DOWNGRADE_BLOCKED","details":{"plan":"starter","over":[],"features":["sso"]}}}
```

## PLAN_FEATURE

Status: 402 · Surface: api

When: the action needs a plan setting the org's plan does not include: a custom domain
(`custom_domains`), keeping an app awake (`always_on`), or a manifest setting the plan lacks
(`storage`, `email`, `customer_identity`), company sign-in (`sso`), forwarding logs to the
org's own log service (`log_forwarding`), setting or changing the org's own storage bucket,
which also receives the backup copy (`own_bucket`, Business only; sending the bucket the org
already has again with new keys is allowed on any plan), checking an app's packages now
(`package_scanning`), serving an app through the CDN (`cdn`), making an app a Promoted app
(`promoted`), an app registering sending domains of its own for its customers
(`app_email_domains`, on every plan with custom domains), or an app's inbox receiving at the
business's own domain (`inbox`, Team and above). `details.setting` names it and `details.plan`
the plan. `kv` on a plan without it is not refused: the app deploys without a cache and `W080`
says so; nor is `inbox`: the app deploys and every message to it is dropped as `not_on_plan`
(doctor-rules.md).

Fix: Ask an owner to upgrade the plan at the billing URL, or stop using the setting.

```json
{"error":{"code":"PLAN_FEATURE","message":"Custom domains are not included in the Free plan.","fix":"Ask an owner to upgrade at https://whisk.run/o/acme/billing.","docs":"https://skill.whisk.run/errors/PLAN_FEATURE","details":{"setting":"custom_domains","plan":"free"}}}
```

## PROMOTE_UNAVAILABLE

Status: 409 · Surface: api

When: an owner or billing contact asked to make an app a Promoted app for a business that does
not pay its own extras, where a Promoted app is charged: a client business its agency pays for
(`details.reason: client`, with `details.agency` naming the agency), or a business inside its
free trial (`details.reason: trial`, with `details.trial_ends_at`).

Fix: For a client business, hand it over and let it choose its own plan first. During the trial,
end the trial or choose a plan on the billing page, then promote the app.

```json
{"error":{"code":"PROMOTE_UNAVAILABLE","message":"Acme is inside its free trial, and a Promoted app is charged with the month's extras.","fix":"End the trial or choose a plan at https://whisk.run/o/acme/billing, then promote the app.","docs":"https://skill.whisk.run/errors/PROMOTE_UNAVAILABLE","details":{"reason":"trial","trial_ends_at":"2026-11-08T00:00:00Z"}}}
```

## TRIAL_USED

Status: 409 · Surface: api

When: an owner asked to start the free Starter trial for an org that has already had one, or
had their own trial on another business. Each business gets one trial, ever, and so does each
person, whether it ended by being cancelled, by a payment that did not go through, or by becoming
a paid plan. `details.trial_used_at` is when the first one started; when it was the person's
trial on another business, `details.used_on` names that business. Also the answer when an
agency asks for its first client business's free month (`trial: true`) and has had it
(`details.client_trial_org` names the client business that had it) or the person asking has had
their one trial (`details.used_on`).

Fix: Choose a plan from the billing page instead, or add the client business without the free
month: it is billed from the day it is added.

```json
{"error":{"code":"TRIAL_USED","message":"acme has already had its free trial, started on 2 September 2026.","fix":"Choose a plan at https://whisk.run/o/acme/billing; each business gets one trial.","docs":"https://skill.whisk.run/errors/TRIAL_USED","details":{"trial_used_at":"2026-09-02T10:00:00Z"}}}
```

## DEMO_NOT_FOUND

Status: 404 · Surface: api, dashboard

When: a demo business's handover link names a code no business has. The link was mistyped or
cut short, or the business it named was deleted.

Fix: Ask the person who sent the link to copy it again from the operator page.

```json
{"error":{"code":"DEMO_NOT_FOUND","message":"No business is waiting to be handed over at this link.","fix":"Ask the person who sent the link to copy it again.","docs":"https://skill.whisk.run/errors/DEMO_NOT_FOUND","details":{}}}
```

## DEMO_CLAIMED

Status: 409 · Surface: api, dashboard

When: someone tried to claim a demo business that has already been claimed. A demo business
is handed over once; `details.org` is its slug.

Fix: The person who claimed it is its owner and can invite anyone else from the People page.

```json
{"error":{"code":"DEMO_CLAIMED","message":"Harbour Building has already been claimed.","fix":"Ask its owner to invite you from https://whisk.run/o/harbour-building/people.","docs":"https://skill.whisk.run/errors/DEMO_CLAIMED","details":{"org":"harbour-building"}}}
```

## DEMO_WRONG_DOMAIN

Status: 403 · Surface: api, dashboard

When: a person signed in with an email outside the company's domain tried to claim a demo
business made for that company. Only someone with an email at `details.domain` can claim it,
so a demo cannot become a business for someone else.

Fix: Sign out and sign in again with your email at the company's domain.

```json
{"error":{"code":"DEMO_WRONG_DOMAIN","message":"Harbour Building can only be claimed with an email at harbourbuilding.com.","fix":"Sign out, then sign in with your harbourbuilding.com email and open the link again.","docs":"https://skill.whisk.run/errors/DEMO_WRONG_DOMAIN","details":{"domain":"harbourbuilding.com"}}}
```

## HANDOVER_WRONG_EMAIL

Status: 403 · Surface: api, dashboard

When: a person tried to accept a client business an agency is handing over while signed in with
an email other than the contact the agency named. Only `details.contact` can accept it, so a
handover link cannot give the business to someone else.

Fix: Sign out and sign in again with the contact's email, or ask the agency to make the link for
your email.

```json
{"error":{"code":"HANDOVER_WRONG_EMAIL","message":"Harbour Building is being handed to sam@harbourbuilding.com.","fix":"Sign out, then sign in as sam@harbourbuilding.com and open the link again, or ask the agency to make the link for your email.","docs":"https://skill.whisk.run/errors/HANDOVER_WRONG_EMAIL","details":{"contact":"sam@harbourbuilding.com"}}}
```

## PAYOUTS_UNAVAILABLE

Status: 409 · Surface: api, dashboard

When: an agency asked to set up payouts for its rebate, and Whisk cannot pay out yet: payments
are not configured, or Stripe would not make the payout account. Nothing is lost: the whole
rebate comes off the agency's own bill as credit until payouts work.

Fix: Nothing to do now; the rebate keeps coming off your bill. Try again later, or contact
support.

```json
{"error":{"code":"PAYOUTS_UNAVAILABLE","message":"Payouts are not available yet.","fix":"Nothing to do now: your rebate comes off your Whisk bill as credit until payouts are available. Try again later, or contact support.","docs":"https://skill.whisk.run/errors/PAYOUTS_UNAVAILABLE","details":{}}}
```

## DEMO_UNCLAIMED

Status: 409 · Surface: api, dashboard

When: someone tried to choose a plan or start a trial for a demo business nobody has claimed
yet. A demo runs on its one free app until its company claims it; the plan and the trial are
theirs to choose.

Fix: Hand the business over first; the person who claims it chooses a plan.

```json
{"error":{"code":"DEMO_UNCLAIMED","message":"Harbour Building is a demo nobody has claimed yet, so it stays on its free app.","fix":"Send the handover link from https://whisk.run/operator; the person who claims it chooses a plan.","docs":"https://skill.whisk.run/errors/DEMO_UNCLAIMED","details":{}}}
```

## DOCTOR_FAILED

Status: - · Surface: cli

When: `whisk doctor` found at least one error, or `whisk deploy` ran doctor first and it found
one. `details` carries the whole report: `errors`, `warnings` and `findings`, each finding with
its rule id, message, fix and, where it has one, the file and line. With `--json` the report is
inside this one error object, not printed beside it. The CLI exits 3.

Fix: Fix each finding with an error severity; `whisk doctor --fix` applies the ones that are
safe to apply. Then run `whisk doctor` again. `whisk deploy --force` pushes anyway, for when a
finding is wrong about this app.

```json
{"error":{"code":"DOCTOR_FAILED","message":"1 error(s) must be fixed before deploying.","fix":"Fix each finding (whisk doctor shows them; --fix applies the safe ones), or pass --force to push anyway.","docs":"https://skill.whisk.run/errors/DOCTOR_FAILED","details":{"errors":1,"warnings":0,"findings":[{"rule":"W001","severity":"error","message":"whisk.yaml is missing.","fix":"Run whisk init."}]}}}
```

## MANIFEST_INVALID

Status: 422 · Surface: api, git, cli

When: `whisk.yaml` does not parse, fails the schema for its version, or breaks a manifest rule.
`details.problems` lists every problem with a path so all can be fixed in one pass. A deploy
also fails with it when a `static` folder is not in the commit or holds no files (problem path
`/static/<i>/dir`): static folders are read from the commit, so build output such as `dist/`,
which is git-ignored, is not there.

Fix: Fix each problem listed; `whisk doctor` shows the same list locally with the schema
description of the field. `whisk schema` prints the schema.

```json
{"error":{"code":"MANIFEST_INVALID","message":"whisk.yaml has 2 problems.","fix":"Fix each listed problem and run whisk doctor. Problems: /functions/0: needs exactly one of cron or event; /webhooks/0/secret: STRIPE_WEBHOOK_SECRET is not listed under secrets.","docs":"https://skill.whisk.run/errors/MANIFEST_INVALID","details":{"problems":[{"path":"/functions/0","message":"needs exactly one of cron or event"},{"path":"/webhooks/0/secret","message":"STRIPE_WEBHOOK_SECRET is not listed under secrets"}]}}}
```

## MANIFEST_UNKNOWN_KEY

Status: 422 · Surface: api, git, cli

When: `whisk.yaml` contains a key the schema does not know. Usually a typo (`function:` for
`functions:`) or a key from a later conventions version.

Fix: Remove or rename the key; the message suggests the nearest known key. Keys from a newer
version need `whisk: <version>` and the platform to support it.

```json
{"error":{"code":"MANIFEST_UNKNOWN_KEY","message":"whisk.yaml has an unknown key function at the top level.","fix":"Rename function to functions, or remove it. Known keys at this level are listed in whisk schema.","docs":"https://skill.whisk.run/errors/MANIFEST_UNKNOWN_KEY","details":{"path":"/function","suggestion":"functions"}}}
```

## CONVENTIONS_VERSION_UNSUPPORTED

Status: 422 · Surface: api, git, cli

When: `whisk:` names a version this platform does not serve. Every published version is
supported forever, so this means a version that does not exist yet or a malformed value.

Fix: Set `whisk: 1`, the current version, or update the CLI with `whisk update` if the message
says the CLI is behind.

```json
{"error":{"code":"CONVENTIONS_VERSION_UNSUPPORTED","message":"whisk.yaml declares conventions version 3; this platform serves versions 1.","fix":"Set whisk: 1 in whisk.yaml.","docs":"https://skill.whisk.run/errors/CONVENTIONS_VERSION_UNSUPPORTED","details":{"declared":3,"supported":[1]}}}
```

## SECRET_IN_COMMIT

Status: 422 · Surface: git, cli

When: a pushed commit contains a value shaped like a secret (an API key, a private key, a
connection string with a password). The push is rejected before it is stored.

Fix: Remove the value from the file, declare a name for it under `secrets` in `whisk.yaml`, read
it from the environment, amend or rewrite the commit so the value is not in history, and ask a
human to set the value in the dashboard. Rotate the leaked value with the provider.

```json
{"error":{"code":"SECRET_IN_COMMIT","message":"A value that looks like a Stripe secret key is in src/config.ts line 12.","fix":"Remove the value, declare STRIPE_SECRET_KEY under secrets in whisk.yaml, read it from the environment, rewrite the commit, and ask an owner to set it at https://whisk.run/o/acme/secrets. Rotate the key in Stripe.","docs":"https://skill.whisk.run/errors/SECRET_IN_COMMIT","details":{"file":"src/config.ts","line":12,"rule":"stripe-secret","commit":"a1b2c3d"}}}
```

## SECRET_VALUE_NEEDS_HUMAN

Status: 403 · Surface: api, cli

When: an agent token tried to set a secret's value. Agents may declare names; only a human
session may set values.

Fix: Tell the human the secret name and the URL where they set it. Do not ask them for the
value and do not put it in a file.

```json
{"error":{"code":"SECRET_VALUE_NEEDS_HUMAN","message":"Agent tokens cannot set secret values.","fix":"Ask an owner, admin or developer to set XERO_CLIENT_SECRET at https://whisk.run/o/acme/secrets/XERO_CLIENT_SECRET. The deploy will restart automatically once it is set.","docs":"https://skill.whisk.run/errors/SECRET_VALUE_NEEDS_HUMAN","details":{"name":"XERO_CLIENT_SECRET","url":"https://whisk.run/o/acme/secrets/XERO_CLIENT_SECRET"}}}
```

## SECRET_SHREDDED

Status: 410 · Surface: api

When: a secret of an org that has been shredded was asked for. Its data key was destroyed, so
nothing sealed under it can be opened by anyone.

Fix: Nothing can bring the value back. If the org is to run again, it is a new org with new
values.

```json
{"error":{"code":"SECRET_SHREDDED","message":"acme was shredded on 2026-11-07; its secrets cannot be read.","fix":"Nothing can bring the value back. If the org is to run again, it is a new org with new values.","docs":"https://skill.whisk.run/errors/SECRET_SHREDDED","details":{"org":"acme","shredded_at":"2026-11-07T00:00:00Z"}}}
```

## SECRET_UNSET

Status: 409 · Surface: api, deploy, cli

When: a deploy needs a declared secret that has no value yet. The deploy is `live` if the app
starts without it, otherwise it is blocked; the CLI prints a `NEEDS_HUMAN` block either way. A
`build.secrets` name with no value blocks the deploy before the build starts (`details.build`
is `true`), because the build cannot run without it.

When another app of the business already has its own value under a missing name,
`details.elsewhere` maps that name to those apps' slugs.

Fix: Relay the names and the URL to the human. Nothing else is needed; the platform restarts the
app when the value arrives. For a name in `details.elsewhere`, first ask the human whether this
app may use the same value as that app; if they agree, run
`whisk secrets share NAME --from <app>` instead of asking for the value.

```json
{"error":{"code":"SECRET_UNSET","message":"2 declared secrets have no value: XERO_CLIENT_SECRET, SLACK_WEBHOOK_URL.","fix":"XERO_CLIENT_SECRET is already set for crm. Ask the human whether this app may use the same value; if so, run whisk secrets share XERO_CLIENT_SECRET --from crm. Ask an owner, admin or developer to set the rest at https://whisk.run/o/acme/secrets. The app restarts automatically when they are set.","docs":"https://skill.whisk.run/errors/SECRET_UNSET","details":{"names":["XERO_CLIENT_SECRET","SLACK_WEBHOOK_URL"],"elsewhere":{"XERO_CLIENT_SECRET":["crm"]},"url":"https://whisk.run/o/acme/secrets"}}}
```

## SECRET_SHARED_EXISTS

Status: 409 · Surface: api, cli, dashboard

When: an app's secret was to be shared with every app of the business, but the business already
has a shared secret of that name with its own value, which sharing would replace.

Fix: Keep both: the app reads its own value and the other apps the shared one. To have every app
use one value, delete the one that should not win and share or set the other.

```json
{"error":{"code":"SECRET_SHARED_EXISTS","message":"acme already has a shared XERO_CLIENT_SECRET with its own value.","fix":"Apps that name XERO_CLIENT_SECRET without their own value already read the shared one. To use crm's value everywhere instead, delete the shared one at https://whisk.run/o/acme/keys and share again.","docs":"https://skill.whisk.run/errors/SECRET_SHARED_EXISTS","details":{"name":"XERO_CLIENT_SECRET","app":"crm"}}}
```

## SECRET_TOO_SHORT

Status: 422 · Surface: api, cli

When: a value under 8 characters was set, from the dashboard or the API. Short values cannot be
scrubbed from logs reliably, so the platform refuses them.

Fix: Use the real value; if the real value is genuinely short, put it in `env` instead because
it is not being treated as a secret.

```json
{"error":{"code":"SECRET_TOO_SHORT","message":"Values must be at least 8 characters so they can be scrubbed from logs.","fix":"Set the full value. If it is genuinely short and not sensitive, declare it under env in whisk.yaml instead.","docs":"https://skill.whisk.run/errors/SECRET_TOO_SHORT","details":{"name":"REGION_CODE","minimum":8}}}
```

## BUILD_FAILED

Status: - · Surface: deploy, cli

When: the image build exited non-zero. The last 200 lines of build output are in
`details.log` and the CLI prints the last 40.

Fix: Read the log excerpt. Typical causes: a dependency install failed, a compile error, a
Dockerfile step failed, a build-time secret is missing from `build.secrets`. Fix and redeploy.

```json
{"error":{"code":"BUILD_FAILED","message":"The build failed at step 5 of 9 (npm ci).","fix":"Read the log excerpt, fix the cause, and run whisk deploy again. The previous deploy is still live.","docs":"https://skill.whisk.run/errors/BUILD_FAILED","details":{"build_id":"01J9","step":"npm ci","exit_code":1,"log":["npm ERR! code E404","npm ERR! 404 Not Found - GET https://registry.npmjs.org/left-padd"]}}}
```

## SECRET_IN_IMAGE

Status: - · Surface: deploy, cli

When: the built image, or one of the manifest's static folders, holds the value of one of the
app's secrets: a build secret a step wrote into a file, or a value pasted into code or a
committed file. Whisk looks for every distinctive secret value before the image is stored, so
the build stopped and nothing was pushed; the previous deploy is still live.
`details.secrets` names the secrets and `details.files` each file (`{secret, path, browser}`);
`browser: true` means the file is sent to browsers, where anyone who opens the app could read
it. The value itself is never shown.

Fix: Read the secret from the environment on the server at run time, never in code sent to the
browser or written into the image (a build secret is for the build's own steps). If the value is
in the repository, remove it and set a new value for the secret, because the old one stays in the
history. A value that is meant to be public takes a public name, such as `NEXT_PUBLIC_` or
`VITE_`.

```json
{"error":{"code":"SECRET_IN_IMAGE","message":"The value of STRIPE_KEY is in /app/dist/assets/index-a1.js, a file sent to browsers, so anyone who opens the app could read it. The build was stopped before the image was stored.","fix":"Read the secret from the environment on the server at run time, never in code sent to the browser or written into the image (a build secret is for the build's own steps). If the value is in the repository, remove it and set a new value for the secret, because the old one stays in the history. A value that is meant to be public takes a public name, such as NEXT_PUBLIC_ or VITE_.","docs":"https://skill.whisk.run/errors/SECRET_IN_IMAGE","details":{"build_id":"01J9","fault":"app","secrets":["STRIPE_KEY"],"files":[{"secret":"STRIPE_KEY","path":"/app/dist/assets/index-a1.js","browser":true}]}}}
```

## SOURCE_MAPS_PUBLISHED

Status: - · Surface: deploy (a warning), cli

When: the deploy went live, but it sends source maps to browsers: `.js.map` or `.css.map` files
in a static folder or in a folder of the image that build tools write browser files into. Anyone
can read the app's original source code from them, comments included. It is a warning on the
deploy (`warnings`), not a failure. `details.count` is how many, `details.files` up to five.

Fix: Turn browser source maps off for production builds (Vite: `build.sourcemap: false`;
Next.js: `productionBrowserSourceMaps: false`; esbuild: no `--sourcemap`), or send them to your
error tracker instead of serving them, then deploy again.

```json
{"error":{"code":"SOURCE_MAPS_PUBLISHED","message":"The app sends source maps to browsers (3, such as /app/dist/assets/index-a1.js.map), so anyone can read its original source code.","fix":"Turn browser source maps off for production builds (Vite: build.sourcemap false; Next.js: productionBrowserSourceMaps false; esbuild: no --sourcemap), or send them to your error tracker instead of serving them, then deploy again.","details":{"count":3,"files":["/app/dist/assets/index-a1.js.map"]},"docs":"https://skill.whisk.run/errors/SOURCE_MAPS_PUBLISHED"}}
```

## BUILD_TIMEOUT

Status: - · Surface: deploy, cli

When: the build ran past 20 minutes.

Fix: Make the build smaller: a lockfile so installs are cached, a multi-stage Dockerfile, no
tests in the build. Check for a step waiting on input.

```json
{"error":{"code":"BUILD_TIMEOUT","message":"The build was stopped after 20 minutes.","fix":"Add a lockfile so dependency installs cache, move tests out of the build, and check that no step waits for input. Redeploy.","docs":"https://skill.whisk.run/errors/BUILD_TIMEOUT","details":{"build_id":"01J9","limit_seconds":1200}}}
```

## IMAGE_PULL_FAILED

Status: - · Surface: deploy

When: the node could not pull the built image from the registry. A platform fault, not the
app's: a deploy that meets it fails as `PLATFORM_DEPLOY_FAILED` with this code as
`details.cause_code`.

Fix: Retry `whisk deploy`. If it fails twice, the platform has already paged the operator;
check `https://whisk.run/status`.

```json
{"error":{"code":"IMAGE_PULL_FAILED","message":"The node could not pull image sha256:9f3c… from the registry.","fix":"Run whisk deploy again. If it fails twice the operator has been paged; see https://whisk.run/status.","docs":"https://skill.whisk.run/errors/IMAGE_PULL_FAILED","details":{"digest":"sha256:9f3c","node":"node1"}}}
```

## HEALTH_CHECK_FAILED

Status: - · Surface: deploy, cli

When: the new container kept running but did not answer 200 on `health.path` within
`health.timeout` seconds. The previous deploy stays live and the new container is stopped.
`details.log` has the container's last 200 lines, `details.path` the path, `details.timeout_seconds`
the limit and `details.last_status` the last HTTP status it answered (0 when nothing answered).
A container that exited instead is `CONTAINER_CRASHED`.

Fix: Read the log. Typical causes: not listening on `PORT` (8080) on all interfaces, the health
route missing or answering non-200, a crash at start, a slow start beyond the timeout. Run the
app locally with `whisk dev` and fetch the health path.

```json
{"error":{"code":"HEALTH_CHECK_FAILED","message":"GET /health did not answer 200 within 90 seconds; nothing answered.","fix":"Check the app listens on PORT (8080) on 0.0.0.0 and that GET /health returns 200. The log excerpt shows the last lines before the timeout. Redeploy after fixing.","docs":"https://skill.whisk.run/errors/HEALTH_CHECK_FAILED","details":{"path":"/health","timeout_seconds":90,"last_status":0,"log":["Listening on 127.0.0.1:3000"],"fault":"app"}}}
```

## CONTAINER_CRASHED

Status: - · Surface: deploy, cli

When: the new container exited during start, before it answered its health check (a deploy
fails; `details.log` has its last lines and `details.path` the health path), or a live app's
container exited and its three restarts did too (the app stops; `whisk status` and the app's
`problem` carry the exit code and last lines). The exit code is known only for the second.

Fix: Read the log lines. A crash at start is usually a missing environment variable or secret,
a missing file or a syntax error; run the app with `whisk dev` to see it. Exit code 137 is the
memory limit; reduce memory use or ask for a larger plan. Other codes are the app's own exit.

```json
{"error":{"code":"CONTAINER_CRASHED","message":"The app exited during start, before GET /health answered.","fix":"Read the log lines and fix the cause; run it with whisk dev to see it start. Exit code 137 would mean the memory limit was hit.","docs":"https://skill.whisk.run/errors/CONTAINER_CRASHED","details":{"path":"/health","log":["Error: DATABASE_URL is not set"],"fault":"app"}}}
```

## MIGRATE_FAILED

Status: - · Surface: deploy, cli

When: the `migrate` command exited non-zero, or ran longer than 600 seconds and was stopped
(`details.timed_out`, `details.timeout_seconds`); the previous deploy is still live.
The live app keeps writing while a migration runs, so the database is rolled back only when the
migration changed its definitions (tables, columns, constraints, indexes, views, functions,
types): it is then restored to the restore point taken before the migration, the message names
the window whose writes are not in the live database, `details.kept_copy` names the replaced
database (kept for 7 days and readable with `whisk db query --database <kept_copy>`), and
`details.writes_not_kept` holds the window. A migration that changed no definitions (one in a
transaction undoes itself) leaves the database as it is, with `details.restored` `left as it
was`; data it changed outside a transaction stays changed. A restore point that did not reach
the backup archive fails the deploy before the migration runs, with `details.step` `snapshot`.

Fix: Read the output in `details.log`. Fix the migration and redeploy. Write migrations to run
in one transaction, so a failure leaves nothing to roll back. A migration that needs longer than
600 seconds is split into smaller ones, or its long backfill moves to a function. If
`details.kept_copy` is set and users wrote during the window, read their rows from the kept copy
with `whisk db query --database` and write them back.

```json
{"error":{"code":"MIGRATE_FAILED","message":"The migrate command failed (exit 1); the migration changed none of the database's tables, columns, indexes or functions, so the database was left as it was and nothing the app's users wrote was undone.","fix":"Fix the migration using the output below and redeploy. The previous deploy is still live.","docs":"https://skill.whisk.run/errors/MIGRATE_FAILED","details":{"command":"node dist/migrate.js","exit_code":1,"restored":"left as it was","snapshot":"deploy_01J9","log":["error: relation \"notes\" already exists"]}}}
```

## PLATFORM_DEPLOY_FAILED

Status: - · Surface: deploy, cli

When: a deploy failed on Whisk's side, not in the app: the builder or the registry failed, no
node was ready, a node could not prepare the database or cache, apply its network rules, pull
the image, start the container, take a restore point or finish running the migration, or the
edge could not be updated. `details.fault` is `platform`, `details.cause_code` names the part
that failed (`NODE_DOWN`, `IMAGE_PULL_FAILED`, `POLICY_APPLY_FAILED`, `DATABASE_FAILED`,
`BUILD_FAILED` for the builder, `MIGRATE_FAILED` for the node, and so on), `details.step_code`
the deploy step when that differs, and `details.cause` the node's own words where it gave
them. The operator is told the cause when it happens. The CLI exits 5, not 6.

Fix: Do not change the code. Run `whisk deploy` again; if it fails the same way, wait a few
minutes and run it again.

```json
{"error":{"code":"PLATFORM_DEPLOY_FAILED","message":"This deploy failed on Whisk's side, not in your code: the migration could not start on node node1: the node's network rules would not load (network policy could not be applied). It did not run, so the database is as it was and the previous deploy is still live.","fix":"Do not change your code for this. Run whisk deploy again; if it fails the same way, wait a few minutes and run it again, since Whisk's operator has already been told the cause.","docs":"https://skill.whisk.run/errors/PLATFORM_DEPLOY_FAILED","details":{"fault":"platform","cause_code":"POLICY_APPLY_FAILED","step_code":"POLICY_APPLY_FAILED","step":"migrate","node":"node1","cause":"network policy could not be applied"}}}
```

## DEPLOY_FAILED

Status: - · Surface: cli

When: a deploy `whisk deploy` was following ended `failed` and the platform recorded no error
code for it, so the CLI cannot name a more specific one. `details` names `deploy_id` and
`status`, and `details.log` holds the last lines of the build or app log when there are any.
The CLI exits 6.

Fix: Run `whisk deploys info <id>` for the phases and `whisk logs` for the app's output, fix
the cause, and run `whisk deploy` again. If nothing in either points at the app, report it with
`whisk feedback --kind bug --code DEPLOY_FAILED`.

```json
{"error":{"code":"DEPLOY_FAILED","message":"Deploy 01J9 ended with status failed and no error was recorded.","fix":"Run whisk deploys info 01J9 for the phases, fix the cause, and run whisk deploy again.","docs":"https://skill.whisk.run/errors/DEPLOY_FAILED","details":{"deploy_id":"01J9","status":"failed"}}}
```

## DEPLOY_CANCELLED

Status: - · Surface: deploy, cli

When: someone cancelled the deploy, or a newer push superseded it.

Fix: Nothing, if a newer deploy replaced it. Otherwise run `whisk deploy` again.

```json
{"error":{"code":"DEPLOY_CANCELLED","message":"Deploy 01J9 was cancelled by ana@acme.example.","fix":"Run whisk deploy again if the cancellation was not intended.","docs":"https://skill.whisk.run/errors/DEPLOY_CANCELLED","details":{"deploy_id":"01J9","cancelled_by":"ana@acme.example"}}}
```

## DEPLOY_IN_PROGRESS

Status: - · Surface: cli

When: `whisk deploy` would wait on a deploy in the same environment that has sat `queued` or
`building` for 25 minutes or more, longer than any build may run (`BUILD_TIMEOUT`): the remote
already had the commit and that commit's deploy is the one stuck, or `git push` lost its
connection to the platform while such a deploy was there. `details` names `deploy_id`,
`status`, `environment`, `since` (when it entered that status), `waited_seconds`, and
`commit_sha`; after a push, git's last lines in `details.git`. The platform itself ends a
deploy nothing is working on within a few minutes, so this is rare. The CLI exits 1.

Fix: Wait for it (`whisk deploys info <id>`), or cancel it with `whisk deploys cancel <id>` and
run `whisk deploy` again, which builds the commit again when its build never finished.

```json
{"error":{"code":"DEPLOY_IN_PROGRESS","message":"Deploy 01J9 has been building for 47 minutes without finishing.","fix":"Wait for it with whisk deploys info 01J9, or cancel it with whisk deploys cancel 01J9 and run whisk deploy again.","docs":"https://skill.whisk.run/errors/DEPLOY_IN_PROGRESS","details":{"deploy_id":"01J9","status":"building","environment":"production","since":"2026-10-06T09:13:00Z","waited_seconds":2820,"commit_sha":"3f9c2a1b7d0e4c55a1b2c3d4e5f60718293a4b5c"}}}
```

## NEEDS_DEPLOY

Status: 503 · Surface: edge

When: a sleeping app was asked to wake but its container no longer exists on the node, for
example after a node rebuild. The platform redeploys the last live build automatically.

Fix: Wait a minute and retry. If it persists, run `whisk deploy`.

```json
{"error":{"code":"NEEDS_DEPLOY","message":"job-tracker is being redeployed and will answer shortly.","fix":"Retry in a minute. If the app still does not answer, run whisk deploy.","docs":"https://skill.whisk.run/errors/NEEDS_DEPLOY","details":{"app":"job-tracker"}}}
```

## POLICY_APPLY_FAILED

Status: - · Surface: deploy

When: the node could not apply the network policy for the container or the migration, so it was
not started. A platform fault; the database is untouched and the previous deploy stays live. A
deploy that meets it fails as `PLATFORM_DEPLOY_FAILED` with this code as `details.cause_code`.
`details` carries `step`, `node`, `cause` (nft's own words) and `fault: platform`, and the operator
is paged.

Fix: Retry `whisk deploy`. The operator has been paged if it fails twice.

```json
{"error":{"code":"POLICY_APPLY_FAILED","message":"The node could not apply network policy for the container and refused to start it.","fix":"Run whisk deploy again. If it fails twice the operator has been paged.","docs":"https://skill.whisk.run/errors/POLICY_APPLY_FAILED","details":{"node":"node1"}}}
```

## CSRF_REJECTED

Status: 403 · Surface: edge, auth, api

When: a non-GET request with cookies (the platform session or the app's own) came from anywhere
but the app's own pages: `Sec-Fetch-Site` was `cross-site` or `same-site` (another app on the apps
domain), or the `Origin` host differed from the request host. On the sign-in host, any POST whose
`Origin` is not that host. On the API, a request that changes state with the Whisk session cookie
whose `Origin` is not the dashboard, or, with no `Origin`, whose `Sec-Fetch-Site` is `cross-site`
or `same-site`.

Fix: Make the request from the app's own pages (on the API, from the dashboard). For a deliberate
cross-site embed, list the route under `routes.csrf_off`. API clients should send a bearer token
instead of a cookie.

```json
{"error":{"code":"CSRF_REJECTED","message":"A POST to /notes with cookies came from another site or another app.","fix":"Send the request from the app's own pages, or list /notes under routes.csrf_off in whisk.yaml if a cross-site POST is intended.","docs":"https://skill.whisk.run/errors/CSRF_REJECTED","details":{"route":"/notes","sec_fetch_site":"cross-site"}}}
```

## REAUTH_REQUIRED

Status: 403 · Surface: auth

When: on the sign-in host, a signed-in person asked to add or remove a passkey, or to set, change
or remove their password, more than ten minutes after they last proved who they are: the sign-in
that made their session, or a confirmation since. Nothing was changed. The pages show the confirm
page instead; the passkey ceremony's script gets this answer.

Fix: Open `/passkeys/confirm` on the sign-in host and confirm it's you with a passkey, your
password or an emailed code (or your company's sign-in, when your company signs you in), then
make the change within ten minutes.

```json
{"error":{"code":"REAUTH_REQUIRED","message":"Confirm it's you before you add a passkey.","fix":"Open https://auth.whisk.run/passkeys/confirm, confirm with a passkey, your password or an emailed code, then try again within ten minutes.","docs":"https://skill.whisk.run/errors/REAUTH_REQUIRED"}}
```

## PASSKEY_REQUIRED

Status: 403 · Surface: api, dashboard, edge

When: a member of a business acted in it through a session that began with an emailed code or a
password, and an owner of the business turned on "Everyone signs in with a passkey or company
sign-in". Only that business refuses the session: the person stays signed in and acts in their
other businesses as before. An app of the business answers its private routes with this too, and
serves its public routes as to anyone. An owner turning the setting on from such a session gets
it as well, so nobody shuts themselves out.

Fix: Sign out, then sign in on the sign-in host with your passkey (or your company's sign-in). If
you have no passkey yet, add one at `/passkeys` on the sign-in host first, then sign in with it.

```json
{"error":{"code":"PASSKEY_REQUIRED","message":"Acme asks everyone to sign in with a passkey or company sign-in, and this session began with an emailed code or a password.","fix":"Sign out, then sign in at https://auth.whisk.run/session/login with your passkey. No passkey yet? Add one at https://auth.whisk.run/passkeys first, then sign in with it.","docs":"https://skill.whisk.run/errors/PASSKEY_REQUIRED","details":{"method":"code","org":"acme"}}}
```

## SSO_REQUIRED

Status: 403 · Surface: api, dashboard, edge

When: a member of a business that requires company sign-in (single sign-on enabled and its
domain verified) acted in it through a session its own identity provider did not make: an
emailed code, a password, a passkey, or another business's provider. It applies to every member,
whatever the domain of their email. Only that business refuses the session; an app of the
business answers its private routes with this too.

Fix: Sign out, then sign in on the sign-in host with your email at the business's domain, which
sends you to its provider. If you are in the business under another email, ask an owner to
invite your address at the business's domain.

```json
{"error":{"code":"SSO_REQUIRED","message":"Acme signs its people in through its own provider, and this session did not come from it.","fix":"Sign out, then sign in at https://auth.whisk.run/session/login with your @acme.com email. If you are in Acme under another email, ask an owner to invite your @acme.com address.","docs":"https://skill.whisk.run/errors/SSO_REQUIRED","details":{"domain":"acme.com","method":"code","org":"acme"}}}
```

## PATH_AMBIGUOUS

Status: 400 · Surface: edge, auth

When: the request path has a `.` or `..` segment, counted after percent-decoding and splitting
on `/` and `\` (so `%2e%2e`, `..%2f` and `..\` count), or does not percent-decode. The edge
classifies routes on the path as sent, and an app that resolves dot segments could serve a
different route than the one the edge classified, so it refuses the path instead of guessing
(CADDY.md §5.1 step 1). Browsers resolve dot segments before sending, so pages never see this.

Fix: Send the path already resolved, as a browser does: `/a/b/../c` is `/a/c`.

```json
{"error":{"code":"PATH_AMBIGUOUS","message":"The request path has a . or .. segment, so the app could serve a different route than the one it names.","fix":"Send the path already resolved, as a browser does: /a/b/../c is /a/c.","docs":"https://skill.whisk.run/errors/PATH_AMBIGUOUS","details":{}}}
```

## CDN_ORIGIN_REQUIRED

Status: 403 · Surface: edge

When: a request reached the origin of a hostname that a CDN fronts, and it neither carried the
shared `X-Whisk-Edge-Key` nor came from the CDN's published ranges (CADDY.md §3).

Fix: Send the request to the hostname's CDN address rather than to the origin. If the CDN itself
is refused, its configured origin header or the platform's ranges are stale; an operator fixes
either.

```json
{"error":{"code":"CDN_ORIGIN_REQUIRED","message":"This hostname accepts requests only through its CDN.","fix":"Send the request to the hostname's CDN address, not to the origin; a CDN must present X-Whisk-Edge-Key or come from its published ranges.","docs":"https://skill.whisk.run/errors/CDN_ORIGIN_REQUIRED","details":{"hostname":"app.acme.com"}}}
```

## CHALLENGE_REQUIRED

Status: 403 · Surface: edge

When: a POST to a route in `routes.challenge` carried no solved Altcha payload, or an invalid
one. The body contains a fresh challenge.

Fix: Solve `details.challenge` with the Altcha widget or library and resend with the solution in
the `altcha` form field or the `X-Altcha` header.

```json
{"error":{"code":"CHALLENGE_REQUIRED","message":"POST /contact needs a solved challenge.","fix":"Solve the challenge in details with the Altcha widget and resend with the payload in the altcha form field or the X-Altcha header.","docs":"https://skill.whisk.run/errors/CHALLENGE_REQUIRED","details":{"route":"/contact","challenge":{"algorithm":"SHA-256","challenge":"…","salt":"…","signature":"…","maxnumber":100000}}}}
```

## RATE_LIMITED

Status: 429 · Surface: edge, api, auth

When: a client exceeded a rate limit: per IP for a visitor who is not signed in, per user for
one who is, per app, or per token on the API; or sent more feedback in an hour than `POST /v1/feedback` takes
(`details.scope` is `feedback`); or an app's own service token added or removed more than 30
custom domains, or asked to verify more than 120, in an hour (`details.scope` is
`app_domains`); or sent more than ten browser error reports in a minute from
one network address to `POST /v1/client-errors` (`details.scope` is `client_errors`); or asked
whisk.run's Whisk On-Premise release routes more than thirty times in a minute from one network
address (`details.scope` is `onpremise_releases`); or started more than ten device logins (`POST /v1/device/code`)
or polled more than sixty times (`POST /v1/device/token`) in a minute from one network address;
or an app registered more than twenty sending domains of its own in an hour (`details.scope` is
`email_domains`).
On the sign-in host, more than five emailed codes for one address
or thirty from one network address in an hour, or ten wrong passwords for one address or fifty
from one network address. `Retry-After` says when to try again.

Fix: Wait `Retry-After` seconds. If a legitimate client hits the limit, the plan's limits are
in the message; ask an owner about a larger plan.

```json
{"error":{"code":"RATE_LIMITED","message":"More than 120 requests per minute from this address.","fix":"Wait 23 seconds and retry. Limits per plan are listed at https://skill.whisk.run/limits.","docs":"https://skill.whisk.run/errors/RATE_LIMITED","details":{"scope":"anonymous_ip","limit":120,"window_seconds":60,"retry_after":23}}}
```

## BODY_TOO_LARGE

Status: 413 · Surface: edge, api, hooks

When: a request body exceeded the limit: 8 MB on app routes (32 MB on Business), 5 MB on the
webhook ingress, 1 MB on the API.

Fix: For file uploads, request a signed upload URL and send the file directly to storage. For
anything else, send less.

```json
{"error":{"code":"BODY_TOO_LARGE","message":"The request body is 12.4 MB; the limit on this route is 8 MB.","fix":"For files, POST /v1/orgs/acme/apps/job-tracker/uploads to get a signed URL and upload directly to storage.","docs":"https://skill.whisk.run/errors/BODY_TOO_LARGE","details":{"limit_bytes":8388608,"received_bytes":13002342}}}
```

## WEBHOOK_UNVERIFIED

Status: 401 · Surface: hooks

When: a delivery to `hooks.whisk.run` failed signature verification, or its timestamp was
outside the preset's tolerance. The event is stored with `verified: false`, visible in the
dashboard, and never delivered to the app.

Fix: Check that the secret set for the source matches the provider's signing secret, and that
the preset matches the provider. Replay from the dashboard is not possible for unverified events;
ask the provider to resend after fixing the secret.

```json
{"error":{"code":"WEBHOOK_UNVERIFIED","message":"The signature on this delivery to source stripe did not verify.","fix":"Confirm STRIPE_WEBHOOK_SECRET matches the signing secret shown in the Stripe dashboard for this endpoint and that the source uses the stripe preset. Then resend the event from Stripe.","docs":"https://skill.whisk.run/errors/WEBHOOK_UNVERIFIED","details":{"source":"stripe","reason":"signature_mismatch","event_id":"01J9"}}}
```

## WEBHOOK_SOURCE_UNKNOWN

Status: 404 · Surface: hooks

When: the URL names an org, app or source that does not exist, or a `token` preset URL is
missing its token.

Fix: Copy the URL printed by `whisk webhooks add` or shown in the dashboard exactly.

```json
{"error":{"code":"WEBHOOK_SOURCE_UNKNOWN","message":"No webhook source named stripe-live exists on app job-tracker.","fix":"Use the exact URL from whisk webhooks list, or declare the source under webhooks in whisk.yaml and deploy.","docs":"https://skill.whisk.run/errors/WEBHOOK_SOURCE_UNKNOWN","details":{"org":"acme","app":"job-tracker","source":"stripe-live"}}}
```

## WEBHOOK_DEAD

Status: - · Surface: run, cli

When: a verified event could not be delivered to the handler after every retry (1m, 5m, 30m,
2h, 12h). It is dead-lettered and can be replayed.

Fix: Fix the handler so it answers 2xx within 30 seconds, deploy, then replay with
`whisk webhooks replay <source> <event-id>`.

```json
{"error":{"code":"WEBHOOK_DEAD","message":"Event 01J9 from source stripe was not delivered after 5 attempts; the last response was 500.","fix":"Fix the handler at /hooks/stripe so it answers 2xx within 30 seconds, deploy, then run whisk webhooks replay stripe 01J9.","docs":"https://skill.whisk.run/errors/WEBHOOK_DEAD","details":{"source":"stripe","event_id":"01J9","attempts":5,"last_status":500}}}
```

## INBOX_NOT_DECLARED

Status: 404 · Surface: api, cli, stub

When: a request needs the app's inbox (`GET`'s domains, `POST .../inbox/domains`, `whisk inbox
send`, the stub's `POST /v1/stub/inbox`) and `whisk.yaml` declares no `inbox`. `GET .../inbox`
itself answers `declared: false` instead.

Fix: Add `inbox:` with `handler:` naming the route that receives each message to `whisk.yaml`,
write that route, and push (or restart `whisk dev`).

```json
{"error":{"code":"INBOX_NOT_DECLARED","message":"lab-results declares no inbox, so it receives no email.","fix":"Add inbox: with handler: /inbound/email (the route that receives each message) to whisk.yaml and push.","docs":"https://skill.whisk.run/errors/INBOX_NOT_DECLARED","details":{"app":"lab-results"}}}
```

## INBOX_DOMAIN_TAKEN

Status: 409 · Surface: api, cli

When: a receiving domain cannot be added to this app: another app already receives at it, the
business sends from it (a sending domain cannot also be a receiving one), another business holds
it at the provider, or it is one of Whisk's own domains. `details.reason` says which.

Fix: Use a subdomain only this app receives at, such as `results.yourbusiness.com`. If another app
of the business has it, remove it there first (`whisk inbox domains remove <id>`).

```json
{"error":{"code":"INBOX_DOMAIN_TAKEN","message":"results.acme.example cannot receive mail for this app: another app receives mail at it.","fix":"Use a subdomain only this app receives at, such as results.yourbusiness.com. If another app of yours has it, remove it there first.","docs":"https://skill.whisk.run/errors/INBOX_DOMAIN_TAKEN","details":{"domain":"results.acme.example","reason":"another app receives mail at it"}}}
```

## INBOX_UNAVAILABLE

Status: 503 · Surface: api, cli

When: a receiving domain was added or verified while the platform has no receiving provider set
up (the operator has not connected Resend). Mail sent to the app's address in the meantime is not
kept.

Fix: Try again later. The platform's operator connects receiving on the operator page.

```json
{"error":{"code":"INBOX_UNAVAILABLE","message":"Whisk is not set up to receive email yet.","fix":"Try again later; the operator sets receiving up on the operator page. Messages sent before then are not kept.","docs":"https://skill.whisk.run/errors/INBOX_UNAVAILABLE","details":{}}}
```

## INBOX_MESSAGE_DROPPED

Status: 409 · Surface: api, cli, stub

When: a replay named an inbox message that was dropped rather than delivered: it was too large,
over the inbox's rate or daily limits, from a sender `allow_from` does not accept, unreadable, or
it arrived while the business was paused. Its `reason` says which. Nothing of it was stored but
the record, so there is nothing to send.

Fix: Nothing to replay. Ask the sender to send it again once the cause is fixed: add them to
`inbox.allow_from` and push, wait for the limit to reset, or ask for a smaller message.

```json
{"error":{"code":"INBOX_MESSAGE_DROPPED","message":"Message 01J9 was dropped (sender_not_allowed), so there is nothing to replay.","fix":"Ask the sender to send it again once the cause is fixed; for sender_not_allowed, add them to inbox.allow_from in whisk.yaml and push first.","docs":"https://skill.whisk.run/errors/INBOX_MESSAGE_DROPPED","details":{"event_id":"01J9","reason":"sender_not_allowed"}}}
```

## DELIVERY_UNVERIFIED

Status: 401 · Surface: container

When: a request to a webhook handler did not carry a valid `X-Whisk-Delivery-Signature`
under the app's `WHISK_DELIVERY_KEY`, or carried `X-Whisk-Service-App`, so it is not a
delivery from the platform. The app's own handler answers this, through the templates'
delivery helper; the platform never sends an unsigned delivery.

Fix: If a genuine delivery is refused, the handler is checking the wrong bytes: verify
`v1=hex(HMAC-SHA256(key, id + "\n" + received_at + "\n" + raw_body))` against
`fixtures/deliveries/`. Anything else is another caller reaching the handler and the refusal
is correct.

```json
{"error":{"code":"DELIVERY_UNVERIFIED","message":"This request is not a delivery from the platform.","fix":"Webhook handlers accept only signed platform deliveries; call the app through its public routes instead.","docs":"https://skill.whisk.run/errors/DELIVERY_UNVERIFIED","details":{"handler":"/hooks/stripe"}}}
```

## INVOKE_FOREIGN

Status: 403 · Surface: run

When: a function called `step.invoke` on a function of another app. One workflow engine serves
every app, so the edge refuses the engine's call into the invoked app unless the invocation came
from that same app. The invoking step fails with this error and the invoked function never runs.

Fix: Invoke only your own app's functions. To use another app, call its service route with
`WHISK_SERVICE_TOKEN` (app-to-app calls).

```json
{"error":{"code":"INVOKE_FOREIGN","message":"Another app's function tried to invoke a function of this app.","fix":"A function can invoke only functions of its own app. To use another app, call its service route with your service token.","docs":"https://skill.whisk.run/errors/INVOKE_FOREIGN"}}
```

## RUN_PARKED

Status: 409 · Surface: api, run, cli

When: a function run failed after its retries, or a run guard stopped it (`RUN_STEP_LIMIT`,
`RUN_CONCURRENCY_LIMIT`). The run is parked with its error and the owner is notified;
`details.step` names the step that failed. A function a guard parked refuses the events that
would start it with this code, from the API and from the SDK's own event calls alike, until
the next deploy to go live declares it again.

Fix: Read the step error with `whisk runs show <id>`, fix the code, deploy, and replay with
`whisk runs replay <id>`. For a parked function, fix it so its runs stay within the limit and
deploy: the deploy lifts the parking when it goes live.

```json
{"error":{"code":"RUN_PARKED","message":"Run 01J9 of nightly-margin failed at step compute-margins after 3 retries.","fix":"Run whisk runs show 01J9 to see the step error, fix it, deploy, then whisk runs replay 01J9.","docs":"https://skill.whisk.run/errors/RUN_PARKED","details":{"run_id":"01J9","function":"nightly-margin","step":"compute-margins","attempts":4,"error":"TypeError: cannot read properties of undefined"}}}
```

## RUN_STEP_LIMIT

Status: - · Surface: run

When: a single run executed more than 1,000 steps, which almost always means a loop with no
exit. The run is parked.

Fix: Bound the loop, or split the work across events so each run stays small.

```json
{"error":{"code":"RUN_STEP_LIMIT","message":"Run 01J9 of sync-contacts executed 1,000 steps and was stopped.","fix":"Bound the loop in sync-contacts or emit one event per page of work so each run stays under 1,000 steps.","docs":"https://skill.whisk.run/errors/RUN_STEP_LIMIT","details":{"run_id":"01J9","function":"sync-contacts","limit":1000}}}
```

## RUN_STEP_FAILED

Status: - · Surface: run

When: a step of a workflow run threw. The run's page carries it on the step, with the step's
input and output beside it; the run's own `error` is its first failed step's.

Fix: Read the step's error and the app's logs, fix the code, push, and replay the run.

```json
{"error":{"code":"RUN_STEP_FAILED","message":"Step charge-card failed: Error: card declined.","fix":"Read the step's error and the app's logs, fix the code, push, and replay the run.","docs":"https://skill.whisk.run/errors/RUN_STEP_FAILED","details":{"step":"charge-card","stack":"Error: card declined\n    at charge (/app/dist/steps.js:12:9)"}}}
```

## RUN_FAILED

Status: - · Surface: run

When: a workflow run failed with an error the app's code threw, in the function body or in a
step, and no step of the run names it (a failed step is `RUN_STEP_FAILED`). The message is the
error's name and message, `details.stack` its stack, and `details.fault` is `app`.

Fix: Read the error and the app's logs, fix the code, push, and replay the run.

```json
{"error":{"code":"RUN_FAILED","message":"The function failed: TypeError: Cannot read properties of undefined (reading 'id').","fix":"Read the error and the app's logs, fix the code, push, and replay the run.","docs":"https://skill.whisk.run/errors/RUN_FAILED","details":{"fault":"app","stack":"TypeError: Cannot read properties of undefined (reading 'id')\n    at sync (/app/dist/sync.js:14:22)"}}}
```

## RUN_APP_UNREACHABLE

Status: - · Surface: run

When: Whisk's edge answered for the app while a run was in progress because the app no longer
did: its process exited or crashed, or a deploy replaced its container mid-run. The engine
records the edge's 502 with no step output. A memory kill is `APP_OUT_OF_MEMORY` instead, and
an edge that could not wake the app is `PLATFORM_RUN_INTERRUPTED`. `details.fault` is `app`.

Fix: Read the app's logs around the run's end for an exit or a crash, fix it, and replay the
run.

```json
{"error":{"code":"RUN_APP_UNREACHABLE","message":"The app stopped answering at 06:33:31 UTC while this run was in progress: its process exited or crashed, or a deploy replaced its container. Your server returned HTTP 502 before the SDK responded.","fix":"Read the app's logs around that time (whisk logs --since) for an exit or a crash, fix it, and replay the run. A memory kill would say APP_OUT_OF_MEMORY instead.","docs":"https://skill.whisk.run/errors/RUN_APP_UNREACHABLE","details":{"fault":"app","engine_error":"Your server returned HTTP 502 before the SDK responded."}}}
```

## PLATFORM_RUN_INTERRUPTED

Status: - · Surface: run

When: Whisk cut a workflow run off: the engine lost its connection to the app, which it reaches
only through Whisk's edge (in practice the edge restarting during a Whisk release), or the edge
could not wake the app (`details.cause_code`, such as `WAKE_TIMEOUT`). Nothing in the app caused
it, and `details.fault` is `platform`. Whisk runs the interrupted run again once, from the same
trigger, within a minute and while the failure is under two hours old; the new run's id is
`details.rerun_run_id` and the run's `rerun_run_id`, and the new run carries `rerun_of`. A run
that is itself a re-run is not run again, and nor is a scheduled run that a later run of its
function has already followed. Only a run that had finished no step is re-run, so nothing that
completed runs twice; a run with a finished step is left to replay. A function with `retries`
has the lost step retried inside the same run by the engine instead.

Fix: Do not change the code. If `details.rerun_run_id` is set, follow that run instead; otherwise
replay the run once its finished steps can safely run again.

```json
{"error":{"code":"PLATFORM_RUN_INTERRUPTED","message":"Whisk lost its connection to the app at 06:33:31 UTC while this run was in progress, most likely because Whisk's edge restarted during a Whisk release. Nothing in your code caused it.","fix":"Do not change your code for this. When no step of the run had finished, Whisk runs it again once by itself and details.rerun_run_id names the new run. Otherwise replay it with whisk runs replay once you know its finished steps can safely run again.","docs":"https://skill.whisk.run/errors/PLATFORM_RUN_INTERRUPTED","details":{"fault":"platform","engine_error":"Unable to reach SDK URL","rerun_run_id":"01M3P163Z5V68KBQ34J4FD51EA"}}}
```

## RUN_CONCURRENCY_LIMIT

Status: - · Surface: run

When: more runs of one function were in flight at once than the plan allows (the plan's
`concurrent_runs`, 20 by default), which almost always means an event sent in a loop. The
function is parked; the runs already started finish.

Fix: Send one event carrying a batch instead of one per item, or let the runs drain and push
again; a push declares the function again and lifts the parking.

```json
{"error":{"code":"RUN_CONCURRENCY_LIMIT","message":"sync-contacts had 41 runs in flight at once, over the limit of 20, and was parked.","fix":"Send one event carrying a batch instead of one per item, then push to lift the parking.","docs":"https://skill.whisk.run/errors/RUN_CONCURRENCY_LIMIT","details":{"function":"sync-contacts","limit":20,"observed":41}}}
```

## EVENT_RATE_LIMITED

Status: 429 · Surface: api

When: the org sent more events per minute than the plan allows. The event was not accepted.

Fix: Wait `Retry-After` and resend, or batch: one event carrying a list is one event.

```json
{"error":{"code":"EVENT_RATE_LIMITED","message":"acme sent more than 600 events in a minute.","fix":"Wait 17 seconds and resend, or send one event carrying a batch instead of one per item.","docs":"https://skill.whisk.run/errors/EVENT_RATE_LIMITED","details":{"limit":600,"window_seconds":60,"retry_after":17}}}
```

## GRAPH_DRIFT

Status: - · Surface: run, cli

When: observed runs of a function executed a step that is not in its declared graph, or a
declared step has never run. Reported by `whisk functions graph` and the dashboard; runs are not
affected.

Fix: Update the graph file to match the code, or the code to match the graph. `whisk doctor`
flags step names that appear in one but not the other before you deploy.

```json
{"error":{"code":"GRAPH_DRIFT","message":"po-approval ran step notify-buyer, which is not in workflows/po-approval.graph.yaml.","fix":"Add notify-buyer to workflows/po-approval.graph.yaml where it belongs, or rename the step in code to a declared id.","docs":"https://skill.whisk.run/errors/GRAPH_DRIFT","details":{"function":"po-approval","unexpected":["notify-buyer"],"never_ran":[]}}}
```

## FUNCTIONS_NOT_REGISTERED

Status: - · Surface: deploy (a warning), cli

When: a deploy went live but its functions could not be registered with the workflow engine, so
their crons and events will not start runs. It is a warning on the deploy (`warnings`), not a
failure: the app serves. `details.functions` names each function `whisk.yaml` declares that the
app's code does not serve (the manifest's name and the code's function id differ), or
`details.cause` gives the engine's error when the app's function route did not answer (the SDK
route not mounted, or the app erroring on it).

Fix: Make each name in `whisk.yaml` match the id the code gives the function (`createFunction`
or its equivalent), and check the app serves the SDK's route; `whisk doctor` compares the names.
Then run `whisk deploy` again, which registers the functions afresh.

```json
{"error":{"code":"FUNCTIONS_NOT_REGISTERED","message":"whisk.yaml declares nightly-report, which the app's code does not serve under that name, so it never runs.","fix":"Make each name under functions in whisk.yaml the id the code gives the function (createFunction's id, fn_id, the ID option), then deploy again.","details":{"functions":["nightly-report"]},"docs":"https://skill.whisk.run/errors/FUNCTIONS_NOT_REGISTERED"}}
```

## FUNCTION_NOT_LIVE

Status: 409 · Surface: api, cli

When: `whisk cron run` (or `POST /orgs/:org/apps/:app/cron/:function/run`) named a cron function
that cannot run yet. `details.reason` says why: `not_deployed` (no deploy of the app has gone
live, so nothing serves the function), `app_stopped` (the app's container stopped and the
platform marked it broken), `not_registered` (the workflow engine does not yet hold the function
with the trigger `whisk cron run` sends, which it gains moments after traffic switches or when
the platform re-registers the app), or `too_many_triggers` (the function already has the
engine's most triggers, 10, so there is no room for that one). No run was started.

Fix: For `not_deployed`, push and wait for `whisk deploy` to report it live. For `app_stopped`,
read why with `whisk logs`, push a fix or run `whisk deploy`. For `not_registered`, wait a minute
and run it again; if it persists, push again, since a deploy registers the function afresh. For
`too_many_triggers`, remove one trigger from the function and push. Then run
`whisk cron run <function>` again.

```json
{"error":{"code":"FUNCTION_NOT_LIVE","message":"nightly-margin cannot run yet: no deploy of crm has gone live.","fix":"Push with whisk deploy and wait for it to go live, then run whisk cron run nightly-margin again.","docs":"https://skill.whisk.run/errors/FUNCTION_NOT_LIVE","details":{"app":"crm","function":"nightly-margin","reason":"not_deployed"}}}
```

## APP_NOT_LIVE

Status: 409 · Surface: api, cli

When: something that reads the app's live production deploy was asked of an app that has none,
such as `whisk scan --now` (or `POST /orgs/:org/apps/:app/packages/scan`) before the app's first
production deploy went live. Nothing was started.

Fix: Run `whisk deploy` and wait for it to report live, then ask again.

```json
{"error":{"code":"APP_NOT_LIVE","message":"crm has no live production deploy to check.","fix":"Run whisk deploy and wait for it to report live, then ask again.","docs":"https://skill.whisk.run/errors/APP_NOT_LIVE","details":{"app":"crm"}}}
```

## DOMAIN_UNVERIFIED

Status: 409 · Surface: api, cli

When: a custom domain's DNS records are not in place yet. The TXT proves ownership; the CNAME
routes traffic. A bare domain such as `acme.example`, which cannot have a CNAME, uses A and AAAA
records to `details.addresses` instead. `details.missing` lists `TXT`, `CNAME` (meaning the
name does not point at the app either way), or both.

Fix: Create the records in `details.records` at the DNS provider, wait for propagation, and
run `whisk domains verify <hostname>`. An app verifying with its own service token shows its
customer `details.records` and calls `POST .../domains/<id>/verify` again later.

```json
{"error":{"code":"DOMAIN_UNVERIFIED","message":"jobs.acme.example is not verified: the TXT record was not found yet.","fix":"Add TXT _whisk-verify.jobs.acme.example = whisk-verify-9f3c and CNAME jobs.acme.example -> job-tracker.acme.whisk.page (for a bare domain like example.com, which cannot have a CNAME, A or AAAA records to 95.217.38.236 instead), wait for DNS to update, then run whisk domains verify jobs.acme.example.","docs":"https://skill.whisk.run/errors/DOMAIN_UNVERIFIED","details":{"hostname":"jobs.acme.example","records":[{"type":"TXT","name":"_whisk-verify.jobs.acme.example","value":"whisk-verify-9f3c"},{"type":"CNAME","name":"jobs.acme.example","value":"job-tracker.acme.whisk.page"}],"addresses":["95.217.38.236"],"missing":["TXT"]}}}
```

## SSO_DOMAIN_UNVERIFIED

Status: 409 · Surface: api

When: SSO was enabled for an email domain whose TXT record is not in place yet. Company sign-in
takes over every email at the domain, so the org must show it holds the domain first.

Fix: Create the record in `details.records` at the DNS provider, wait for propagation, and save
the SSO settings again.

```json
{"error":{"code":"SSO_DOMAIN_UNVERIFIED","message":"acme.example is not verified for company sign-in: the TXT record was not found.","fix":"Add TXT _whisk-sso.acme.example = whisk-sso-4b1e9c, then save the SSO settings again at https://whisk.run/o/acme/settings.","docs":"https://skill.whisk.run/errors/SSO_DOMAIN_UNVERIFIED","details":{"domain":"acme.example","records":[{"type":"TXT","name":"_whisk-sso.acme.example","value":"whisk-sso-4b1e9c"}]}}}
```

## DOMAIN_TAKEN

Status: 409 · Surface: api, cli

When: the hostname is already attached to another app, possibly in another org, or is an app's
address on another business's own domain, or a business domain is already another business's,
or an email domain is already used for company sign-in by another org (`details.kind` is `sso`).

Fix: Remove it from the other app first, or use a different hostname. If another org holds
your domain, contact support with proof of ownership.

```json
{"error":{"code":"DOMAIN_TAKEN","message":"jobs.acme.example is already attached to another app.","fix":"Run whisk domains remove jobs.acme.example on the app that holds it, or choose another hostname. Contact support@whisk.run if you own the domain and do not control that app.","docs":"https://skill.whisk.run/errors/DOMAIN_TAKEN","details":{"hostname":"jobs.acme.example"}}}
```

## PLAN_LIMIT_DOMAINS

Status: 402 · Surface: api, cli

When: adding a custom domain would give the app more than its plan allows one app
(`custom_domains_per_app`: Starter 25, Team and Agency 100, Business 500), counting every custom
domain of the app, verified or not, whoever added it. `details.limit` is the plan's number and
`details.current` how many the app holds.

Fix: Remove a domain nobody uses any more by its id (`DELETE .../domains/<id>`, or `whisk
domains remove <hostname>`), such as one whose records were never created, or ask an owner to
upgrade the plan.

```json
{"error":{"code":"PLAN_LIMIT_DOMAINS","message":"The Starter plan allows 25 custom domains on one app and results has 25.","fix":"Remove a domain nobody uses any more by its id, such as one whose records were never created, or ask an owner to upgrade at https://whisk.run/o/acme/billing.","docs":"https://skill.whisk.run/errors/PLAN_LIMIT_DOMAINS","details":{"limit":25,"current":25,"plan":"starter"}}}
```

## DOMAIN_ADDED_BY_TEAM

Status: 403 · Surface: api

When: an app's own service token tried to remove a custom domain the business's people or their
agents added (`added_by: team`). The app's token removes only the domains it added itself, so
the business's own names stay out of reach of the app's code. `details.domain_id` names the
domain.

Fix: Leave it, or have a person with deploy rights remove it with `whisk domains remove
<hostname>` or from the app's Domains page.

```json
{"error":{"code":"DOMAIN_ADDED_BY_TEAM","message":"results.acme.example was added by the business's team, so the app's own token cannot remove it.","fix":"Leave it, or have a person with deploy rights remove it with whisk domains remove results.acme.example.","docs":"https://skill.whisk.run/errors/DOMAIN_ADDED_BY_TEAM","details":{"domain_id":"01J9ZQ4X7T8V2M5N6P3R1S0W9Y","hostname":"results.acme.example"}}}
```

## ORG_DOMAIN_EXISTS

Status: 409 · Surface: api, cli

When: the business already has its own domain (every app at `<app>.<domain>`); a business has
one. `details.domain` is the one it has.

Fix: Keep using it, or have an owner or admin remove it in the dashboard (Settings, Your
domain) and add the new one.

```json
{"error":{"code":"ORG_DOMAIN_EXISTS","message":"This business already uses acme.example.","fix":"Remove it first (owner, in the dashboard), then add the new one.","docs":"https://skill.whisk.run/errors/ORG_DOMAIN_EXISTS","details":{"domain":"acme.example"}}}
```

## EMAIL_DOMAIN_UNVERIFIED

Status: 403 · Surface: api

When: `from` in an email send names a domain the org has not verified. A send without `from`
goes from Whisk's shared address and never answers this.

Fix: Leave `from` out to send from Whisk's address, send from a verified domain, or ask an owner
to add and verify the domain in the dashboard (the DNS records are shown there).

```json
{"error":{"code":"EMAIL_DOMAIN_UNVERIFIED","message":"acme.example is not a verified sending domain for acme.","fix":"Leave from out to send from Whisk's address, send from a verified domain, or ask an owner to verify acme.example at https://whisk.run/o/acme/settings.","docs":"https://skill.whisk.run/errors/EMAIL_DOMAIN_UNVERIFIED","details":{"from":"noreply@acme.example","verified":["mail.acme.example"]}}}
```

## EMAIL_PAUSED

Status: 403 · Surface: api

When: the org's sending is paused because its bounce or complaint rate crossed the threshold.
Owners were notified.

Fix: Stop sending to addresses that bounced, then ask an owner to request a resume from the
email settings page.

```json
{"error":{"code":"EMAIL_PAUSED","message":"Sending for acme is paused: the bounce rate over the last 24 hours was 7.2%.","fix":"Remove bouncing addresses from your lists, then ask an owner to resume sending at https://whisk.run/o/acme/settings.","docs":"https://skill.whisk.run/errors/EMAIL_PAUSED","details":{"domain":"mail.acme.example"}}}
```

## EMAIL_ATTACHMENT_INVALID

Status: 400 · Surface: api

When: an email send's `attachments` cannot be used as written: more than 10 files, an entry with
no `filename` or one holding a path or a line break, neither or both of `content` and
`storage_key`, `content` that is not base64 or is empty, a `content_type` that is not
`type/subtype`, a `content_id` on a file that is not an image, a `content_id` the HTML never shows
as `cid:<content_id>` or one used twice, or a `cid:` in the HTML that no attachment carries.
`details.attachment` is the entry's index.

Fix: Do what `fix` says for that entry: each attachment is `{filename, content_type, content}` or
`{filename, content_type, storage_key}`, with `content_id` only on an image the HTML shows with
`<img src="cid:<content_id>">`.

```json
{"error":{"code":"EMAIL_ATTACHMENT_INVALID","message":"Attachment logo.png has content_id logo, and the HTML never shows cid:logo.","fix":"Show it with <img src=\"cid:logo\">, or leave content_id out to attach the file.","docs":"https://skill.whisk.run/errors/EMAIL_ATTACHMENT_INVALID","details":{"attachment":1,"filename":"logo.png","content_id":"logo"}}}
```

## EMAIL_ATTACHMENT_TOO_LARGE

Status: 413 · Surface: api

When: an email's attachments hold more than 10 MB together, decoded, counting files read from
storage; or the send body is over 16 MB.

Fix: Send smaller files or fewer of them, or put the file in storage and send a link to it
instead of the file.

```json
{"error":{"code":"EMAIL_ATTACHMENT_TOO_LARGE","message":"The attachments hold 12.4 MB together; a message may carry 10.0 MB.","fix":"Send smaller files, fewer of them, or a link to the file in storage instead of the file.","docs":"https://skill.whisk.run/errors/EMAIL_ATTACHMENT_TOO_LARGE","details":{"bytes":13002342,"limit_bytes":10485760}}}
```

## EMAIL_ATTACHMENT_BLOCKED

Status: 422 · Surface: api

When: an email's attachment is a program, script, shortcut, installer or disk image: its name
ends in an extension Gmail refuses (`.exe`, `.js`, `.bat`, `.iso` and the rest of
`mailattach.Blocked`), its `content_type` is a program's, its first bytes are a Windows, Linux or
macOS executable whatever it is called, or it is a ZIP archive naming such a file.

Fix: Send documents, images and data files, not programs. To share a program, put it in storage
and send a link to it.

```json
{"error":{"code":"EMAIL_ATTACHMENT_BLOCKED","message":"Attachment setup.exe cannot be sent: its type, .exe, is a program or script mail servers refuse.","fix":"Send documents, images and data files, not programs. To share a program, put it in storage and send a link to it.","docs":"https://skill.whisk.run/errors/EMAIL_ATTACHMENT_BLOCKED","details":{"attachment":0,"filename":"setup.exe","extension":".exe"}}}
```

## EMAIL_ATTACHMENT_NOT_FOUND

Status: 404 · Surface: api

When: an email's attachment names a `storage_key` that is not under the app's own
`WHISK_STORAGE_PREFIX`, or that the app's storage does not hold (never written, deleted, or
moved to quarantine by the scan).

Fix: Send the object's full key, the app's `WHISK_STORAGE_PREFIX` followed by its name, for a file
the app wrote; or send the file itself as `content`.

```json
{"error":{"code":"EMAIL_ATTACHMENT_NOT_FOUND","message":"Attachment report.pdf names app/01J9ZQ/reports/october.pdf, which is not in this app's storage.","fix":"Send storage_key as the object's full key, the app's WHISK_STORAGE_PREFIX followed by its name, such as app/01J9ZQ/reports/october.pdf.","docs":"https://skill.whisk.run/errors/EMAIL_ATTACHMENT_NOT_FOUND","details":{"attachment":0,"filename":"report.pdf","storage_key":"app/01J9ZQ/reports/october.pdf"}}}
```

## EMAIL_LINK_BLOCKED

Status: 403 · Surface: api

When: the message links to an address a malware or phishing list names, or to an app Whisk is
reviewing. The message is not sent, and the business is held for review (`ORG_UNDER_REVIEW`).

Fix: Remove the link. If the address is yours and the list is wrong, write to support@whisk.run.

```json
{"error":{"code":"EMAIL_LINK_BLOCKED","message":"The message links to bad.example, which a phishing list names.","fix":"Remove the link. If the address is yours and the list is wrong, write to support@whisk.run.","docs":"https://skill.whisk.run/errors/EMAIL_LINK_BLOCKED","details":{"host":"bad.example"}}}
```

## EMAIL_RATE_LIMITED

Status: 429 · Surface: api

When: the org has sent its daily allowance (on a plan that does not bill past it) or its monthly
allowance, or 100 recipients in a minute. `details.period` is `day` or `month` for an allowance;
`Retry-After` gives the seconds until the window resets.

Fix: Wait, or ask an owner about a larger allowance. Transactional email should rarely hit
this; a burst usually means a loop.

```json
{"error":{"code":"EMAIL_RATE_LIMITED","message":"acme has sent 1,000 emails today, the plan's daily limit.","fix":"Wait 5h12m for the window to reset, and check for a loop if the volume was unexpected.","docs":"https://skill.whisk.run/errors/EMAIL_RATE_LIMITED","details":{"limit":1000,"sent":1000,"retry_after":18720}}}
```

## EMAIL_SENDER_INVALID

Status: 400 · Surface: api

When: the operator set the platform's mail sender (`PUT /v1/operator/email/resend`) with a key
that cannot be used: it is empty, holds spaces or line breaks, does not start with `re_`, Resend
does not accept it, or Resend will not send from any of the platform's domains with it.
`details.reason` is `format`, `refused` or `domain`; for `domain`, `details.tried` lists the
domains tried.

Fix: Create a key with full access at https://resend.com/api-keys and paste it whole. For
`domain`, verify one of the domains in `details.tried` at https://resend.com/domains first.

```json
{"error":{"code":"EMAIL_SENDER_INVALID","message":"Resend will not send from whisk.run or whisk.page.","fix":"Verify one of those domains at https://resend.com/domains, then paste the key again.","docs":"https://skill.whisk.run/errors/EMAIL_SENDER_INVALID","details":{"reason":"domain","tried":["whisk.run","whisk.page"]}}}
```

## STRIPE_KEY_INVALID

Status: 400 · Surface: api, dashboard

When: the operator set the platform's Stripe account (`PUT /v1/operator/payments/stripe`) with a
key that cannot be used: it is empty, holds spaces or line breaks, is not a secret key (`sk_live_`,
`sk_test_`, or a restricted `rk_` key), Stripe does not accept it, or it may not make the webhook
endpoint billing needs. `details.reason` is `format`, `refused` or `permissions`.

Fix: Copy the secret key from https://dashboard.stripe.com/apikeys and paste it whole. For
`permissions`, paste the account's standard secret key rather than a restricted key.

```json
{"error":{"code":"STRIPE_KEY_INVALID","message":"That is the publishable key. Paste the secret key, which starts with sk_live_.","fix":"Copy the secret key from https://dashboard.stripe.com/apikeys and paste it whole.","docs":"https://skill.whisk.run/errors/STRIPE_KEY_INVALID","details":{"reason":"format"}}}
```

## STRIPE_ACCOUNT_IN_USE

Status: 409 · Surface: api, dashboard

When: the operator pasted a Stripe key for another account, or the other mode (test or live), than
the one businesses already pay through. Their customers, cards and subscriptions live in that
account, so switching would stop their billing.

Fix: Paste a key for the same account and mode as `details.account_id` and `details.mode`.

```json
{"error":{"code":"STRIPE_ACCOUNT_IN_USE","message":"3 businesses already pay through the Stripe account in use (acct_1Abc, live mode).","fix":"Paste a key for that same account and mode; moving businesses to another Stripe account is not something Whisk does.","docs":"https://skill.whisk.run/errors/STRIPE_ACCOUNT_IN_USE","details":{"account_id":"acct_1Abc","customers":3,"mode":"live"}}}
```

## CDN_UNAVAILABLE

Status: 503 · Surface: api, cli, dashboard

When: an app's CDN was turned on (`PUT /orgs/:org/apps/:app/cdn {"on": true}`) on a platform whose
operator has not set a CDN provider yet (CONTROL-PLANE.md §6.29).

Fix: Leave the CDN off for now; the app keeps being served directly. Try again once the
platform's operator has set the CDN up.

```json
{"error":{"code":"CDN_UNAVAILABLE","message":"The CDN is not set up on this platform yet.","fix":"Leave the CDN off for now; the app keeps being served directly. Try again once the platform's operator has set the CDN up.","docs":"https://skill.whisk.run/errors/CDN_UNAVAILABLE","details":{}}}
```

## CDN_KEY_INVALID

Status: 400 · Surface: api, dashboard

When: the operator set the platform's CDN provider (`PUT /v1/operator/cdn/bunny`) with a key that
cannot be used: it is empty, holds spaces, line breaks or characters an API key never has, or
Bunny does not accept it. `details.reason` is `format` or `refused`.

Fix: Copy the API key from https://dash.bunny.net/account/api-key and paste it whole.

```json
{"error":{"code":"CDN_KEY_INVALID","message":"Bunny did not accept that API key.","fix":"Copy the API key from https://dash.bunny.net/account/api-key and paste it whole.","docs":"https://skill.whisk.run/errors/CDN_KEY_INVALID","details":{"reason":"refused"}}}
```

## CDN_IN_USE

Status: 409 · Surface: api, dashboard

When: the operator tried to remove the CDN provider's key while apps are still served through it.
Their addresses point at the provider, so removing the key would leave Whisk unable to move them
back.

Fix: Turn the CDN off for the apps in `details.apps`, wait until each shows off, then remove the
key.

```json
{"error":{"code":"CDN_IN_USE","message":"2 apps are still served through the CDN.","fix":"Turn the CDN off for those apps, wait until each shows off, then remove the key.","docs":"https://skill.whisk.run/errors/CDN_IN_USE","details":{"apps":["acme/crm","acme/site"]}}}
```

## GITHUB_NOT_SET_UP

Status: 503 · Surface: api, cli, dashboard

When: an app's copy on GitHub was asked for on a platform whose operator has not set up Whisk's
GitHub App yet (CONTROL-PLANE.md §6.26).

Fix: Nothing to change in the app. Apps run as before without GitHub; the operator sets the App
up on the operator page.

```json
{"error":{"code":"GITHUB_NOT_SET_UP","message":"Copying apps to GitHub is not available on this platform yet.","fix":"The operator sets up Whisk's GitHub App on the operator page first.","docs":"https://skill.whisk.run/errors/GITHUB_NOT_SET_UP"}}
```

## GITHUB_NOT_INSTALLED

Status: 409 · Surface: api, cli, dashboard

When: an app was linked to a GitHub repository, but Whisk is not installed on any GitHub account
of the business yet.

Fix: An owner or admin chooses Connect GitHub in the app's settings and installs Whisk on the
GitHub account that holds the repository, then links it.

```json
{"error":{"code":"GITHUB_NOT_INSTALLED","message":"Whisk is not installed on any GitHub account of this business yet.","fix":"An owner or admin chooses Connect GitHub in the app's settings and installs Whisk on the account that holds the repository.","docs":"https://skill.whisk.run/errors/GITHUB_NOT_INSTALLED"}}
```

## GITHUB_INSTALLATION_UNVERIFIED

Status: 403 · Surface: dashboard

When: GitHub sent a person back from installing Whisk, but their own GitHub account cannot see
the installation named, so it may not be theirs to connect.

Fix: Start again from Connect GitHub in Whisk, signed in to GitHub as someone who can manage the
account Whisk is installed on.

```json
{"error":{"code":"GITHUB_INSTALLATION_UNVERIFIED","message":"Your GitHub account cannot see that installation of Whisk.","fix":"Install Whisk on GitHub from the app's settings in Whisk, signed in to GitHub as someone who can manage that account.","docs":"https://skill.whisk.run/errors/GITHUB_INSTALLATION_UNVERIFIED","details":{"installation_id":51234567}}}
```

## GITHUB_INSTALLATION_TAKEN

Status: 409 · Surface: dashboard

When: the GitHub account Whisk was installed on is already connected to another business on
Whisk.

Fix: Use another GitHub account or organisation, or remove the connection from the other
business first.

```json
{"error":{"code":"GITHUB_INSTALLATION_TAKEN","message":"That GitHub account is already connected to another business on Whisk.","fix":"Use another GitHub account or organisation, or remove the connection from the other business first.","docs":"https://skill.whisk.run/errors/GITHUB_INSTALLATION_TAKEN","details":{"account":"acme"}}}
```

## GITHUB_REPO_UNAVAILABLE

Status: 422 · Surface: api, cli, dashboard

When: the repository to link does not exist, or none of the business's installations of Whisk
on GitHub may reach it.

Fix: Check the name, add the repository to Whisk's installation on GitHub (the app's settings
link to the page), then link again.

```json
{"error":{"code":"GITHUB_REPO_UNAVAILABLE","message":"Whisk cannot reach acme/crm on GitHub.","fix":"Check the name, and add the repository to Whisk's installation on GitHub (the app's settings link to the page), then link again.","docs":"https://skill.whisk.run/errors/GITHUB_REPO_UNAVAILABLE","details":{"repo":"acme/crm"}}}
```

## GITHUB_REPO_TAKEN

Status: 409 · Surface: api, cli, dashboard

When: the repository is already linked to another app.

Fix: Pick another repository, or unlink it from the other app first.

```json
{"error":{"code":"GITHUB_REPO_TAKEN","message":"acme/crm is already linked to the app crm.","fix":"Pick another repository, or unlink it from crm first.","docs":"https://skill.whisk.run/errors/GITHUB_REPO_TAKEN","details":{"repo":"acme/crm","app":"crm"}}}
```

## GITHUB_REPO_NOT_EMPTY

Status: 409 · Surface: api, cli, dashboard

When: the repository already holds code that shares no history with the app, so linking would
mix two unrelated projects.

Fix: Link an empty repository (created on GitHub without a README), or one that already holds
this app's code.

```json
{"error":{"code":"GITHUB_REPO_NOT_EMPTY","message":"acme/website already holds other code.","fix":"Link an empty repository (create one on GitHub without a README), or one that already holds this app's code.","docs":"https://skill.whisk.run/errors/GITHUB_REPO_NOT_EMPTY","details":{"repo":"acme/website"}}}
```

## LOG_DESTINATION_FAILED

Status: 422 · Surface: api

When: a log destination did not accept the test line Whisk sends when an owner adds one, or
when someone asks for a test: it answered with a status outside 200 to 299, did not answer within
10 seconds, does not resolve, or resolves to an address that is not public. `details.status` is the
status it answered (0 when it did not answer) and `details.answer` the start of what it said. A
destination that did not answer reads "did not answer" whatever the reason, naming no address.
Nothing is saved when adding fails.

Fix: Check the URL (or the Datadog site), the headers and the key with the log service, then
add the destination again.

```json
{"error":{"code":"LOG_DESTINATION_FAILED","message":"logs.example.com answered 401 to a test line.","fix":"Check the URL, the headers and the key with your log service, then add the destination again.","docs":"https://skill.whisk.run/errors/LOG_DESTINATION_FAILED","details":{"status":401,"answer":"{\"error\":\"invalid token\"}"}}}
```

## STORAGE_QUOTA

Status: 402 · Surface: api

When: the org's storage is at its plan limit on a plan that does not bill storage past it (the
free app, an Agency client business, a business with no card on file, or a paid plan during its
free trial); new uploads are refused, reads continue.

Fix: Delete objects you no longer need, or ask an owner to upgrade.

```json
{"error":{"code":"STORAGE_QUOTA","message":"acme is using 1.0 GB of its 1.0 GB storage allowance.","fix":"Delete objects under the app's prefix you no longer need, or ask an owner to upgrade at https://whisk.run/o/acme/billing.","docs":"https://skill.whisk.run/errors/STORAGE_QUOTA","details":{"limit_bytes":1073741824,"used_bytes":1073741824}}}
```

## BUCKET_UNREACHABLE

Status: 502 · Surface: api, deploy, edge

When: a bucket the business brought did not answer the platform: the connection was refused,
reset or timed out, the name did not resolve, or it resolved to an address that is not on the
public internet. Every one of these reads the same, so the answer names no address and does not
say which happened. A deploy that writes static folders to the bucket stops with this code, and an
export that reads it names it as the cause. On the edge: a static file of an app whose business
brought its bucket could not be read from it (the bucket did not answer or refused), which the
control plane reads for the edge; the answer carries none of the bucket's own words or status.

Fix: Check that the endpoint is your provider's public https:// S3 address and that it is up,
then try again.

```json
{"error":{"code":"BUCKET_UNREACHABLE","message":"The bucket this business brought did not answer.","fix":"Check that the endpoint is your provider's public https:// S3 address and that it is up, then try again.","docs":"https://skill.whisk.run/errors/BUCKET_UNREACHABLE","details":{}}}
```

## UPLOAD_TOO_LARGE

Status: 413 · Surface: api

When: an upload form was asked for with a `max_bytes` above the 5 GB one object may be. A file
larger than the `max_bytes` a form was issued for is refused by the object store itself, which
answers `EntityTooLarge`.

Fix: Ask for a form with `max_bytes` at or under the limit in `details`, or split the file.

```json
{"error":{"code":"UPLOAD_TOO_LARGE","message":"An upload may be at most 5 GB; this form asked for 6.0 GB.","fix":"Ask for a form with max_bytes up to 5368709120, or split the file.","docs":"https://skill.whisk.run/errors/UPLOAD_TOO_LARGE","details":{"max_bytes":6442450944,"limit_bytes":5368709120}}}
```

## OBJECT_QUARANTINED

Status: 410 · Surface: api

When: the virus scan flagged an uploaded object; it was moved to `quarantine/` under the app's
prefix and a notification was sent.

Fix: Treat the upload as rejected in the app. An admin can inspect or delete it from the
dashboard.

```json
{"error":{"code":"OBJECT_QUARANTINED","message":"uploads/invoice-4412.pdf was quarantined: Eicar-Test-Signature.","fix":"Show the uploader that the file was rejected. An admin can review it at https://whisk.run/o/acme/apps/job-tracker/settings.","docs":"https://skill.whisk.run/errors/OBJECT_QUARANTINED","details":{"key":"uploads/invoice-4412.pdf","signature":"Eicar-Test-Signature","quarantine_key":"quarantine/uploads/invoice-4412.pdf"}}}
```


## MEDIA_NOT_READY

Status: 409 · Surface: api, edge

When: a converted copy of an uploaded video or audio file was asked for at
`/.whisk/media/<id>/<file>` before the conversion made it, or the conversion failed or is held
until the month's conversion allowance renews. The original plays meanwhile at
`/.whisk/media/<id>/original`.

Fix: Read `/.whisk/media/<id>` for the status and the sources that exist, or use the player
(`/.whisk/player.js`), which plays the original until the copies are ready.

```json
{"error":{"code":"MEDIA_NOT_READY","message":"site-walkthrough.mov is still being converted; 720.mp4 is not ready.","fix":"Read /.whisk/media/01J9ABC for what exists, or play /.whisk/media/01J9ABC/original until it is ready.","docs":"https://skill.whisk.run/errors/MEDIA_NOT_READY","details":{"id":"01J9ABC","status":"converting","file":"720.mp4"}}}
```
## IMAGE_NOT_READY

Status: 409 · Surface: edge

When: an uploaded image was asked for at `/.whisk/img/<id>` before its file reached the store:
the upload's form was issued but the browser's POST has not finished. The answer carries
`Retry-After: 5` and is never cached.

Fix: Ask again once the upload has finished; the address is right and stays right.

```json
{"error":{"code":"IMAGE_NOT_READY","message":"team-photo.jpg has not finished uploading.","fix":"Ask again once the upload's POST to the store has finished; the address is right.","docs":"https://skill.whisk.run/errors/IMAGE_NOT_READY","details":{"id":"01J9ABC"}}}
```

## IMAGE_UNREADABLE

Status: 422 · Surface: edge

When: an upload authorised as an image could not be read as one by the resizer: the file is
damaged, is in a format imgproxy does not read, or is larger than 50 megapixels. It is
recorded on the upload, so later requests answer at once, and `GET …/uploads/<id>` shows it as
`image.detail`.

Fix: Show the uploader that the picture could not be used and ask for it again as a JPEG, PNG,
WebP, AVIF, GIF or HEIC file under 50 megapixels.

```json
{"error":{"code":"IMAGE_UNREADABLE","message":"scan.tiff: The file could not be read as an image, or is larger than 50 megapixels.","fix":"Upload the picture again as a JPEG, PNG, WebP, AVIF, GIF or HEIC file under 50 megapixels.","docs":"https://skill.whisk.run/errors/IMAGE_UNREADABLE","details":{"id":"01J9ABC","content_type":"image/tiff"}}}
```

## NOT_AN_IMAGE

Status: 415 · Surface: edge

When: `/.whisk/img/<id>` names an upload whose `content_type` is not `image/*`, such as a PDF
or a video.

Fix: Resize only uploads authorised with an image content type. Play video and audio at
`/.whisk/media/<id>`, and give other files out with `GET …/uploads/<id>`.

```json
{"error":{"code":"NOT_AN_IMAGE","message":"invoice-4412.pdf is application/pdf, not an image.","fix":"Use /.whisk/img/ for uploads authorised with an image/* content_type; play video and audio at /.whisk/media/<id>.","docs":"https://skill.whisk.run/errors/NOT_AN_IMAGE","details":{"id":"01J9ABC","content_type":"application/pdf"}}}
```

## MEDIA_LINK_EXPIRED

Status: 403 · Surface: edge

When: a signed link to an image or video (`/.whisk/img/<id>` or `/.whisk/media/<id>` with
`exp`, `kid` and `sig`, from `POST …/uploads/links`) is used after its `exp`. The signature is
checked first, so only a genuine link is told it expired. The answer is never cached.

Fix: Draw the page again so the app issues a fresh link. Issue links when the page is drawn,
for as long as the page needs them (an hour unless `expires_in` says otherwise, twelve hours at
most), rather than storing them.

```json
{"error":{"code":"MEDIA_LINK_EXPIRED","message":"This link expired at 2026-10-09T05:00:00Z.","fix":"Draw the page again: the app issues a fresh link with POST …/uploads/links each time it shows the file.","docs":"https://skill.whisk.run/errors/MEDIA_LINK_EXPIRED","details":{"expired_at":"2026-10-09T05:00:00Z"}}}
```

## MEDIA_LINK_INVALID

Status: 403 · Surface: edge

When: a request to `/.whisk/img/<id>` or `/.whisk/media/<id>` carries a `sig` that does not
verify: its query was changed (another size, a later `exp`), it was made for another upload or
another app, its `exp`, `kid` or `sig` is malformed, or it was signed with a key the app has
since rotated. A request with a `sig` is judged by it alone, so a wrong one is refused even for
someone signed in. The answer is never cached.

Fix: Use the link exactly as `POST …/uploads/links` answered it. For another size, ask for
another link; after a rotation, issue new links.

```json
{"error":{"code":"MEDIA_LINK_INVALID","message":"The link's signature does not match what it asks for: it was changed, or it was made for another size, file or app.","fix":"Use the link exactly as POST …/uploads/links answered it, changing nothing in its query; ask for another size with a new link.","docs":"https://skill.whisk.run/errors/MEDIA_LINK_INVALID"}}
```
## EXPORT_IN_PROGRESS

Status: 409 · Surface: api, cli

When: an export job is already running for the org.

Fix: Wait for it; `whisk export --wait` follows the existing job.

```json
{"error":{"code":"EXPORT_IN_PROGRESS","message":"An export started at 2026-09-07T10:02:00Z is still running.","fix":"Run whisk export --wait to follow it; a new export can start when it finishes.","docs":"https://skill.whisk.run/errors/EXPORT_IN_PROGRESS","details":{"job_id":"01J9","started_at":"2026-09-07T10:02:00Z"}}}
```

## EXPORT_FAILED

Status: - · Surface: api (the export's record), cli

When: an export could not be written: a node did not answer, a dump failed, or the org's bucket
refused the archive. `details.piece` names where it stopped, `details.node` the node when one was
involved, and `details.cause_code` the node's own code when it gave one.

Fix: Start the export again. If it stops at the same piece, report it with `whisk feedback
--kind bug --code EXPORT_FAILED`, naming the export's id and `details.piece`.

```json
{"error":{"code":"EXPORT_FAILED","message":"The export stopped at apps/notes/databases/production.dump on node node1: pg_dump app_01: exited 1","fix":"Start the export again. If it stops at the same piece, report it with whisk feedback --kind bug --code EXPORT_FAILED --message \"export <id> stopped at <piece>\".","docs":"https://skill.whisk.run/errors/EXPORT_FAILED","details":{"piece":"apps/notes/databases/production.dump","node":"node1","cause":"pg_dump app_01: exited 1","cause_code":"DATABASE_FAILED"}}}
```

## RESTORE_NOT_AVAILABLE

Status: 402 · Surface: api, cli

When: self-service point-in-time restore was requested on a plan without it, or for a time
outside the retention window.

Fix: Choose a time within the last 30 days, or ask an owner to upgrade. The platform's own
backups still cover the org.

```json
{"error":{"code":"RESTORE_NOT_AVAILABLE","message":"Self-service restore is not included in the Starter plan.","fix":"Ask an owner to upgrade at https://whisk.run/o/acme/billing, or contact support@whisk.run for an operator-assisted restore.","docs":"https://skill.whisk.run/errors/RESTORE_NOT_AVAILABLE","details":{"plan":"starter","retention_days":30}}}
```

## RESTORE_IN_PROGRESS

Status: 409 · Surface: api, cli

When: a self-service restore was requested for an app that already has one queued or running.
Two restores of one database race for its name, so one runs at a time.

Fix: Wait for the running restore to finish: `whisk restore show <id> --wait` follows it
(`GET /orgs/:org/apps/:app/restores/:id`), and `whisk restore list` lists the app's restores.

```json
{"error":{"code":"RESTORE_IN_PROGRESS","message":"A restore started at 2026-09-16T10:02:00Z is still running.","fix":"Wait for it to finish: whisk restore show 01J9 --wait follows it (GET /orgs/:org/apps/:app/restores/01J9).","docs":"https://skill.whisk.run/errors/RESTORE_IN_PROGRESS","details":{"restore":"01J9","started_at":"2026-09-16T10:02:00Z"}}}
```

## APP_NOT_RESTORABLE

Status: 409 · Surface: api, cli

When: `whisk apps restore <id>` named an app that is not deleted, whose 7 days are up and whose
data was released, or whose slug another app in the business now uses.

Fix: `whisk apps deleted` lists the apps that can still be restored. For a slug in use, delete or
rename the other app first. A released app cannot be brought back from here; if it was deleted
by mistake within the last 30 days, ask with `whisk feedback`.

```json
{"error":{"code":"APP_NOT_RESTORABLE","message":"crm is not a deleted app that can still be restored: an app is restorable for 7 days after it is deleted, until its data is released.","fix":"Nothing can bring it back from here. If it was deleted by mistake less than 30 days ago, ask Whisk's support with whisk feedback.","docs":"https://skill.whisk.run/errors/APP_NOT_RESTORABLE","details":{"app":"01J9","status":"deleted"}}}
```

## RESOURCE_TAKEN

Status: 409 · Surface: api

When: a provider answered with a thing the platform's record already holds for another business:
the same id at the same provider and account (a bucket, a sending domain, a GitHub installation).
The platform never records one thing as two businesses', because releasing it for one would
delete it for the other (`CONTROL-PLANE.md §6.32`).

Fix: Use another one, or ask its business to remove it first.

```json
{"error":{"code":"RESOURCE_TAKEN","message":"That github installation (51234) already belongs to another business on Whisk.","fix":"Use another one, or ask its business to remove it first.","docs":"https://skill.whisk.run/errors/RESOURCE_TAKEN","details":{"kind":"github_installation","id":"51234"}}}
```

## EMAIL_DOMAIN_TAKEN

Status: 409 · Surface: api, cli

When: a business adds a sending domain another business on Whisk already sends from, or one of
Whisk's own domains or a name under one. A sending domain belongs to one business, so removing it
from one never stops another's mail or Whisk's. Within a business, an app registering a domain
the whole business already has (`details.held_by` is `business`: every app sends from it
already) or another of its apps registered (`held_by` is `app`, `details.app` names it), and a
business adding in Settings a domain one of its apps registered, are refused the same way.

Fix: Send from a domain only your business uses, such as a subdomain like
mail.yourbusiness.com. If the domain is yours, remove it from the other business first. An app
sends from a domain of the whole business without registering it.

```json
{"error":{"code":"EMAIL_DOMAIN_TAKEN","message":"mail.acme.com is already a sending domain of another business on Whisk.","fix":"Send from a domain only your business uses, such as a subdomain like mail.yourbusiness.com. If the domain is yours, remove it from the other business first.","docs":"https://skill.whisk.run/errors/EMAIL_DOMAIN_TAKEN","details":{"domain":"mail.acme.com"}}}
```

## RESOURCES_NOT_RELEASED

Status: 409 · Surface: api (operator), cli

When: a business is being shredded, or an app's release ran, and something the business owns
outside the platform's database is not released yet: a provider did not answer or refused. The
business is not marked shredded until every such thing is released or retained on purpose. A
thing a lookup found, which waits on an operator's confirmation, never causes it: the release
finishes without it and the thing stays listed.

Fix: Nothing to do now: the release is tried again on its own, with backoff, and every hour
after that. `GET /v1/operator/orgs/:org/resources` (`whisk operator resources <business>`) lists
each one with its last error.

```json
{"error":{"code":"RESOURCES_NOT_RELEASED","message":"2 of the business's outside things are not released yet (bucket, ses_identity).","fix":"Nothing to do now: the release is tried again on its own, with backoff, and every hour after that. GET /v1/operator/orgs/:org/resources lists each one with its last error.","docs":"https://skill.whisk.run/errors/RESOURCES_NOT_RELEASED","details":{"open":2,"stuck":0,"kinds":"bucket, ses_identity"}}}
```

## RESTORE_POINT_NOT_ARCHIVED

Status: 503 · Surface: api, cli

When: `whisk db snapshot`, or `whisk db query --confirm` on production, took a restore point
that did not reach the backup archive within 90 seconds, so a restore could not reach it.
Nothing was changed: the confirmed SQL did not run. A deploy whose migration meets the same
fails with `MIGRATE_FAILED` (`details.step` `snapshot`) before the migration runs.

Fix: Wait a few minutes and run the command again (a confirmed query needs a fresh preview and
code). This is on Whisk's side; if it repeats, send it with `whisk feedback`.

```json
{"error":{"code":"RESTORE_POINT_NOT_ARCHIVED","message":"The restore point did not reach the backup archive, so it could not be restored to. Nothing was changed.","fix":"Wait a few minutes and run the command again; a confirmed query needs a fresh preview and code. If it repeats, send it with whisk feedback.","docs":"https://skill.whisk.run/errors/RESTORE_POINT_NOT_ARCHIVED","details":{"restore_point":"snapshot-01J9-20261006T101500Z","lsn":"0/3000060"}}}
```

## DB_ACCESS_OFF

Status: 403 · Surface: api, cli

When: `whisk db query`, `whisk db schema`, `whisk db shell` or `whisk db url` was used in an org
whose owners turned database access from outside the app off (`settings.db_access = false`).

Fix: Run the query from inside the app, or ask an owner or admin to turn database access on
under the org's settings.

```json
{"error":{"code":"DB_ACCESS_OFF","message":"Database access from laptops is turned off for acme.","fix":"An owner or admin can turn it on under settings, or run the query from inside the app.","docs":"https://skill.whisk.run/errors/DB_ACCESS_OFF","details":{"setting":"db_access"}}}
```

## CONFIRM_REQUIRED

Status: 409 · Surface: api, cli

When: `whisk db query` was given SQL that would change or remove existing data: update or
delete rows (upserts and cascades included), drop, alter, truncate or revoke something, or
`create or replace` a definition. The platform ran it in a transaction, recorded its effect and
rolled it back, so nothing was changed. `details` carries the `effect` (per statement, and
rows inserted, updated and deleted per table), the `code` and when it expires.

Fix: Check the effect is what you meant, then run `whisk db query --confirm <code>` within ten
minutes. A restore point is taken first, and the SQL commits only if it does the same again.
Whether that restore point can be returned to depends on the plan: the fix says so, and
`details.restorable` is `true` where `whisk restore` reaches it (within the plan's restore window,
7 days on Starter, Team and Agency and 30 on Business) and `false` on Free, where nothing undoes
the change and the rows it touches should be copied first with `whisk db query "select ..." --json`.

```json
{"error":{"code":"CONFIRM_REQUIRED","message":"Nothing was changed yet. This would delete 48 rows in public.orders in the production database.","fix":"Check that this is what you meant, then run whisk db query --confirm wq_mfrgg4dfmzxw2 before 2026-09-29T12:10:00Z to do it; a restore point is taken first, and whisk restore --at <its time> returns to it for the next 7 days.","docs":"https://skill.whisk.run/errors/CONFIRM_REQUIRED","details":{"code":"wq_mfrgg4dfmzxw2","expires_at":"2026-09-29T12:10:00Z","environment":"production","database":"app_a1","restorable":true,"effect":{"statements":[{"command":"DELETE 48","rows":48}],"tables":[{"table":"public.orders","inserted":0,"updated":0,"deleted":48}]}}}}
```

## CONFIRM_INVALID

Status: 409 · Surface: api, cli

When: `whisk db query --confirm` named a code that does not exist, was already used, is more
than ten minutes old, or was issued to someone else.

Fix: Run the SQL again without `--confirm` for a fresh preview and code.

```json
{"error":{"code":"CONFIRM_INVALID","message":"That code is unknown, used, expired or someone else's.","fix":"Run the SQL again without --confirm for a fresh preview and code.","docs":"https://skill.whisk.run/errors/CONFIRM_INVALID","details":{}}}
```

## CONFIRM_CHANGED

Status: 409 · Surface: api, cli

When: a confirmed `whisk db query` would now do something other than its preview said, because
the data changed in between. It was rolled back; nothing was changed. The code is used up.

Fix: Run the SQL again without `--confirm` for a fresh preview and code.

```json
{"error":{"code":"CONFIRM_CHANGED","message":"Nothing was changed: the SQL would now delete 50 rows in public.orders, not delete 48 rows in public.orders as the preview said.","fix":"Run the SQL again without --confirm for a fresh preview and code.","docs":"https://skill.whisk.run/errors/CONFIRM_CHANGED","details":{}}}
```

## QUERY_FAILED

Status: 422 · Surface: api, cli

When: PostgreSQL refused the SQL `whisk db query` sent: a syntax error, a missing table, a
constraint, a statement past the 30-second limit (`57014`), a lock held for more than five
seconds (`55P03`), or a statement that cannot run inside a transaction. `details.sqlstate` is
PostgreSQL's own code. The transaction was rolled back; nothing was changed.

Fix: Read PostgreSQL's message, fix the SQL and run it again.

```json
{"error":{"code":"QUERY_FAILED","message":"PostgreSQL refused the SQL: relation \"ordrs\" does not exist.","fix":"Fix the SQL and run it again. Nothing was changed.","docs":"https://skill.whisk.run/errors/QUERY_FAILED","details":{"sqlstate":"42P01","position":15}}}
```

## QUERY_RESULT_TOO_LARGE

Status: 422 · Surface: api, cli

When: `whisk db query` was stopped because the database sent more than the control plane reads
for one call: a single row, error or notice larger than 8 MiB (`details.kind` is `message`), or
more than 256 MiB of rows across the SQL's results (`details.kind` is `rows`), counting rows
past `limit` that are counted but not answered. The connection was dropped, which rolls the
transaction back; nothing was changed.

Fix: Narrow what the SQL returns: a `where` clause or a `limit`, `count(*)` to count, and
`left(col, 1000)` or `length(col)` for a very large value.

```json
{"error":{"code":"QUERY_RESULT_TOO_LARGE","message":"The database sent a single row or message larger than 8 MiB, so the query was stopped. Nothing was changed.","fix":"Select fewer or shorter columns, for example left(body, 1000) or length(body) instead of body, and run it again.","docs":"https://skill.whisk.run/errors/QUERY_RESULT_TOO_LARGE","details":{"limit_bytes":8388608,"kind":"message"}}}
```

## QUERY_BUSY

Status: 429 · Surface: api, cli

When: `whisk db query` or `whisk db schema` waited five seconds for a place and did not start:
the org already had two of them running, or the platform sixteen. Nothing ran.

Fix: Wait for the running queries to finish and send it again (`details.retry_after` seconds);
run queries one after another rather than at once.

```json
{"error":{"code":"QUERY_BUSY","message":"The org already has 2 database queries running, or the platform has 16; this one waited 5s and did not start.","fix":"Wait for the running queries to finish, then send it again; run queries one after another rather than at once.","docs":"https://skill.whisk.run/errors/QUERY_BUSY","details":{"per_org":2,"at_once":16,"retry_after":5}}}
```

## DB_URL_PRIVATE

Status: - · Surface: cli

When: `whisk db url` was run where the platform runs no public database proxy, so the
connection string's address is inside the app's network and does not answer from outside.

Fix: Use `whisk db query "<sql>"` or `whisk db schema`, which run on the platform. `--private`
prints the inside address anyway, for use from inside the platform.

```json
{"error":{"code":"DB_URL_PRIVATE","message":"The production database's address is inside the platform, so it does not answer from this machine.","fix":"Use whisk db query \"<sql>\" for rows as JSON or whisk db schema for the tables; both run on the platform.","docs":"https://skill.whisk.run/errors/DB_URL_PRIVATE","details":{"environment":"production"}}}
```

## PLATFORM_UNAVAILABLE

Status: 503 · Surface: api, edge, cli

When: a platform component is down. The response is temporary and safe to retry. The CLI
answers with it when it could not reach the platform, when a download or stream was cut off,
or when a successful answer was not the API's JSON (a proxy or captive portal answered
instead); it has already retried a call that was safe to (CLI.md §1) and exits 5.

Fix: Retry with backoff. Status is at `https://whisk.run/status`.

```json
{"error":{"code":"PLATFORM_UNAVAILABLE","message":"The platform is temporarily unavailable.","fix":"Retry with backoff. See https://whisk.run/status for the current status.","docs":"https://skill.whisk.run/errors/PLATFORM_UNAVAILABLE","details":{"retry_after":10}}}
```

## NODE_DOWN

Status: 503 · Surface: api, edge, cli

When: the node hosting the app is not answering. The operator has been paged; apps on other
nodes are unaffected. From `whisk db query` and `whisk db schema` it means the environment's
database did not answer the connection at all; a database that answers and refuses the
password is `DATABASE_CREDENTIALS_REJECTED` instead.

Fix: Wait. Nothing the app can do; deploys are refused until the node is back or the app has
been moved.

```json
{"error":{"code":"NODE_DOWN","message":"The node hosting job-tracker has not been seen for 3 minutes.","fix":"Wait; the operator has been paged. See https://whisk.run/status.","docs":"https://skill.whisk.run/errors/NODE_DOWN","details":{"node":"node1","last_seen_at":"2026-09-07T10:00:00Z"}}}
```

## NODE_DRAIN_NO_TARGET

Status: 409 · Surface: api

When: the operator drains a node and at least one app live on it has no other ready node to
move to. Nothing is moved and the node's status is unchanged: a half-drained node would still
need the machine.

Fix: Enrol another node, or bring a down node back, then drain again.

```json
{"error":{"code":"NODE_DRAIN_NO_TARGET","message":"No other node is ready to take job-tracker, so node1 cannot be drained.","fix":"Enrol or bring back another ready node, then drain again.","docs":"https://skill.whisk.run/errors/NODE_DRAIN_NO_TARGET","details":{"node":"node1","apps":["job-tracker"]}}}
```

## LICENCE_MISSING

Status: 403 · Surface: api, git, deploy

When: Whisk On-Premise has no licence installed, so a deploy or a push is refused.
Running apps keep running. The hosted platform never answers it.

Fix: Ask Whisk for your licence, then install it on the operator page or with
`whiskd licence install < licence.txt`.

```json
{"error":{"code":"LICENCE_MISSING","message":"No Whisk licence is installed, so nothing new can deploy.","fix":"Ask Whisk for a current licence, then install it on the operator page or with `whiskd licence install < licence.txt`.","docs":"https://skill.whisk.run/errors/LICENCE_MISSING","details":{}}}
```

## LICENCE_INVALID

Status: 403 · Surface: api, git, deploy

When: the installed Whisk On-Premise licence cannot be used: its signature does not match its
terms (the file was changed after Whisk issued it), it was signed with a key this version does
not know, or its start date is still ahead. Installing such a file is refused with the same
code. whisk.run answers it too when a licence file sent to the support relay or the release
downloads does not verify.

Fix: Install the licence file exactly as Whisk sent it, from its first line to its signature. If
it is unchanged, ask Whisk for a current licence.

```json
{"error":{"code":"LICENCE_INVALID","message":"The installed Whisk licence cannot be used: The licence signature does not match its terms. The file has been changed since Whisk issued it.","fix":"Ask Whisk for a current licence, then install it on the operator page or with `whiskd licence install < licence.txt`.","docs":"https://skill.whisk.run/errors/LICENCE_INVALID","details":{"problem":"The licence signature does not match its terms. The file has been changed since Whisk issued it."}}}
```

## LICENCE_EXPIRED

Status: 403 · Surface: api, git, deploy

When: the Whisk On-Premise licence has ended. Running apps keep running and keep their data; new
deploys and pushes wait for a current licence.

Fix: Ask Whisk to renew the licence, then install the new file on the operator page or with
`whiskd licence install < licence.txt`. Deploys work again at once.

```json
{"error":{"code":"LICENCE_EXPIRED","message":"The Whisk licence ended on 2027-10-08. Running apps keep running; new deploys wait for a current licence.","fix":"Ask Whisk for a current licence, then install it on the operator page or with `whiskd licence install < licence.txt`.","docs":"https://skill.whisk.run/errors/LICENCE_EXPIRED","details":{"licence":"lic_2026_0001","ended":"2027-10-08"}}}
```

## SUPPORT_DOOR_CLOSED

Status: 409 · Surface: api

When: one of Whisk's operators sends a call to a Whisk On-Premise install whose support door is
not open: the company never opened it, closed it, its time ran out, or the install has not asked
for calls in the last two minutes. Nothing is sent. Only whisk.run answers it.

Fix: Ask the company to open the support door on their operator page, then send the call again.

```json
{"error":{"code":"SUPPORT_DOOR_CLOSED","message":"The support door of lic_2026_0001 is not open.","fix":"Ask the company to open the support door on their operator page, then send the call again.","docs":"https://skill.whisk.run/errors/SUPPORT_DOOR_CLOSED","details":{"licence":"lic_2026_0001"}}}
```

## SUPPORT_DOOR_TIMEOUT

Status: 504 · Surface: api

When: a call sent through a Whisk On-Premise install's support door had no answer within 60
seconds. The install may still run it and answer later; the answer is kept for a day. Only
whisk.run answers it.

Fix: Read what the call would have changed before sending it again; a read is safe to repeat.

```json
{"error":{"code":"SUPPORT_DOOR_TIMEOUT","message":"lic_2026_0001 did not answer the call within 60 seconds.","fix":"Check what the call would have changed before sending it again; a read is safe to repeat.","docs":"https://skill.whisk.run/errors/SUPPORT_DOOR_TIMEOUT","details":{"licence":"lic_2026_0001","call":"6f2c…"}}}
```

## RELEASE_NOT_COVERED

Status: 403 · Surface: api

When: a company asks whisk.run for a Whisk On-Premise release bundle its licence does not cover:
the release was published after the licence's last day, or the licence's first day has not come.
A licence that has ended still downloads every release published on or before its last day.
Only whisk.run answers it.

Fix: Download a release the licence covers (`POST /v1/onpremise/releases` lists them), or ask
Whisk for a licence that covers this one.

```json
{"error":{"code":"RELEASE_NOT_COVERED","message":"onpremise-2027.11 was published on 2027-11-02, after 2027-10-08, the last day of licence lic_2026_0001.","fix":"Download a release your licence covers (POST /v1/onpremise/releases lists them), or ask Whisk for a licence that covers this one.","docs":"https://skill.whisk.run/errors/RELEASE_NOT_COVERED","details":{"licence":"lic_2026_0001","version":"onpremise-2027.11","starts":"2026-10-09","ends":"2027-10-08"}}}
```

## PACKAGE_CHECK_OFF

Status: 409 · Surface: api

When: lockfiles are sent to be checked before a deploy (`whisk doctor`) on a platform that sends
no lockfile to osv.dev. Whisk On-Premise does not unless its operator turns it on. Nothing was
checked or sent.

Fix: Deploy as usual: once the app is live the platform's registry checks the packages in its
image. An operator can turn the lockfile check on with `WHISK_OSV=on`.

```json
{"error":{"code":"PACKAGE_CHECK_OFF","message":"This platform does not send lockfiles to osv.dev, so they cannot be checked before a deploy.","fix":"Deploy as usual: once the app is live the platform's registry checks the packages in its image. An operator can turn the lockfile check on with WHISK_OSV=on.","docs":"https://skill.whisk.run/errors/PACKAGE_CHECK_OFF","details":{"setting":"WHISK_OSV"}}}
```

## REPO_TOO_LARGE

Status: 422 · Surface: git, cli

When: the repository exceeds the plan's size limit after the push.

Fix: Remove large files from history (build output, media, archives) and use storage for
files. `whisk doctor` reports the size before pushing.

```json
{"error":{"code":"REPO_TOO_LARGE","message":"The repository would be 1.4 GB; the Team plan allows 500 MB.","fix":"Remove build output and media from the repository and its history, ignore them in .gitignore, and use storage for files. Then push again.","docs":"https://skill.whisk.run/errors/REPO_TOO_LARGE","details":{"limit_bytes":524288000,"size_bytes":1503238553,"largest":[{"path":"assets/video.mp4","bytes":812000000}]}}}
```

## SOURCE_CHANGED

Status: 409 · Surface: api

When: files were sent to deploy (`POST /apps/:app/source`, the assistant connector's
`deploy_app`) with a `base_commit` that is no longer the tip of the app's default branch, or
another push landed while this one was being made. Someone else changed the app since its files
were read.

Fix: Read the app's files again (`read_app_files`), apply the change to what is there now, and
send it with the new commit as `base_commit`.

```json
{"error":{"code":"SOURCE_CHANGED","message":"The app changed since its files were read: the default branch is now at 3f9c2a1b7d0e.","fix":"Read the app's files again, apply your change to them, and deploy with base_commit 3f9c2a1b7d0e.","docs":"https://skill.whisk.run/errors/SOURCE_CHANGED","details":{"commit":"3f9c2a1b7d0e4c55a1b2c3d4e5f60718293a4b5c","base_commit":"9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a0b"}}}
```

## INVALID_REQUEST

Status: 400 · Surface: api, hooks

When: the request body or parameters do not match what the endpoint expects: malformed JSON, a
missing field, a wrong type.

Fix: Fix the request as described in `details.problems`.

```json
{"error":{"code":"INVALID_REQUEST","message":"The event is missing name.","fix":"Send {\"name\": \"<event name>\", \"data\": {...}} with an optional dedupe_key.","docs":"https://skill.whisk.run/errors/INVALID_REQUEST","details":{"problems":[{"path":"/name","message":"required"}]}}}
```

## CONFLICT

Status: 409 · Surface: api

When: another request created or changed the same thing at the same moment, so this one could not
be applied as sent.

Fix: Read the current state and retry the request if it still applies.

```json
{"error":{"code":"CONFLICT","message":"Another request changed this at the same moment.","fix":"Read the current state and retry the request if it still applies.","docs":"https://skill.whisk.run/errors/CONFLICT"}}
```

## ROLLBACK_UNAVAILABLE

Status: 409 · Surface: api

When: an operator asked to roll the control plane back, and the copy it replaced cannot take over
now: the 10 minutes it is kept warm after a switch have passed, it is not running idle and
healthy, or it did not start serving and the switch was undone. `details.reason` says which.

Fix: Read GET /v1/operator/control-plane for each copy's state. After the 10 minutes, release the
earlier version instead.

```json
{"error":{"code":"ROLLBACK_UNAVAILABLE","message":"The control plane cannot be rolled back now: the 10 minutes after the switch have passed.","fix":"Read GET /v1/operator/control-plane for each copy's state. After the 10 minutes, release the earlier version instead.","docs":"https://skill.whisk.run/errors/ROLLBACK_UNAVAILABLE","details":{"reason":"expired","live":"green","epoch":5}}}
```

## NOT_FOUND

Status: 404 · Surface: api, edge

When: the org, app, deploy, run, secret or other resource in the URL does not exist or is not
visible to this token. On the edge: the hostname belongs to no app (a removed preview, a deleted
app, a typo under the apps domain); every name under a wildcard certificate reaches the edge, so
the edge answers this rather than an empty response. `details.kind` is `hostname`. Also on the
edge: a path only an attacker's scanner asks for (WordPress, PHP, secrets or version control files such as
`/.env` and `.git`), which nothing on Whisk serves; `details.kind` is `path`. Also on the edge: a static file
of an app whose business brought its bucket that the bucket does not have, or a request for one
that is not a GET or HEAD.

Fix: Check the identifiers with the matching `list` command; `whisk use <org>/<app>` fixes a
directory bound to the wrong app.

```json
{"error":{"code":"NOT_FOUND","message":"No app named job-trakcer in org acme.","fix":"Run whisk apps list to see the app slugs, then whisk use acme/<slug>.","docs":"https://skill.whisk.run/errors/NOT_FOUND","details":{"kind":"app","org":"acme","slug":"job-trakcer"}}}
```

## NEEDS_HUMAN

Status: - · Surface: cli

When: a CLI command reached a point only a human can complete: login approval, setting a secret
value, connecting an account, confirming the org. The CLI prints the URL and exits 2. After a
deploy, a `NEEDS_HUMAN` block lists unset secrets even though the deploy succeeded.

Fix: Show the human the URL and the names exactly as printed, then wait or continue with work
that does not depend on it.

```json
{"error":{"code":"NEEDS_HUMAN","message":"2 secrets need a value before the app can use them: XERO_CLIENT_SECRET, SLACK_WEBHOOK_URL.","fix":"Ask an owner, admin or developer to set them at https://whisk.run/o/acme/secrets. The app restarts automatically when they are set.","docs":"https://skill.whisk.run/errors/NEEDS_HUMAN","details":{"url":"https://whisk.run/o/acme/secrets","names":["XERO_CLIENT_SECRET","SLACK_WEBHOOK_URL"]}}}
```

## UPDATE_FAILED

Status: - · Surface: cli

When: `whisk update` could not install the release the platform serves: the download does not
match `SHA256SUMS`, `SHA256SUMS.sig` does not verify under the release key the build embeds,
the release has no binary for this platform, or the running binary could not be replaced.
`details.asset` names the file; `details.path` the binary.

Fix: Run `whisk update` again. If the checksum or signature fails the same way, do not install
it (the binary you have keeps working) and report it with `whisk feedback --kind bug --code
UPDATE_FAILED`; if the binary cannot be replaced, make its directory writable or reinstall with
the install script.

```json
{"error":{"code":"UPDATE_FAILED","message":"The downloaded whisk_linux_amd64 does not match SHA256SUMS.","fix":"Run whisk update again; if it fails the same way the release on the platform is inconsistent: do not install it, and report it with whisk feedback --kind bug --code UPDATE_FAILED.","docs":"https://skill.whisk.run/errors/UPDATE_FAILED","details":{"asset":"whisk_linux_amd64","want":"9f3c…","got":"1a2b…"}}}
```

## LOCAL_DOCKER_UNAVAILABLE

Status: - · Surface: cli

When: `whisk dev` needs Docker with the compose plugin to start Postgres, the workflow engine and
the local edge, and Docker is not installed, not running, or has no compose plugin. Nothing was
started. The CLI exits 1.

Fix: Install Docker Desktop or Docker Engine with the compose plugin and start it, then run
`whisk dev` again. Without Docker, run the app under `whisk-stub` (`contract/cmd/whisk-stub`)
with a Postgres of your own. Retrying without Docker does not help.

```json
{"error":{"code":"LOCAL_DOCKER_UNAVAILABLE","message":"docker compose version: exec: \"docker\": executable file not found in $PATH","fix":"Install Docker Desktop or Docker Engine with the compose plugin and start it, then run whisk dev again; or run the app under whisk-stub with your own Postgres. Retrying without Docker will not help.","docs":"https://skill.whisk.run/errors/LOCAL_DOCKER_UNAVAILABLE","details":{}}}
```

## DEV_STACK_FAILED

Status: - · Surface: cli

When: `whisk dev` found Docker but `docker compose up` for the local stack in `.whisk/dev/`
failed: a port it needs is taken, an image could not be pulled, or a container would not start.
`message` carries compose's own words. The CLI exits 1.

Fix: Check that Docker is running and that the ports in `.whisk/dev/compose.yaml` are free, then
run `whisk dev` again. `whisk dev --down` resets the stack; `--down --wipe` also deletes its
database.

```json
{"error":{"code":"DEV_STACK_FAILED","message":"docker compose up: Bind for 127.0.0.1:5432 failed: port is already allocated","fix":"Check that Docker is running and the ports in .whisk/dev/compose.yaml are free, then run whisk dev again; whisk dev --down resets the stack.","docs":"https://skill.whisk.run/errors/DEV_STACK_FAILED","details":{}}}
```

## DEV_NOT_RUNNING

Status: - · Surface: cli

When: `whisk inbox send` found nothing answering as the local stub at `127.0.0.1:<port+1>`:
`whisk dev` is not running, or was started with another `--port`. Nothing was sent.

Fix: Start the app with `whisk dev` in another terminal, or pass `--port` with the port it was
started on.

```json
{"error":{"code":"DEV_NOT_RUNNING","message":"Nothing answered at http://127.0.0.1:3001/v1/stub/inbox: connection refused","fix":"Start the app with whisk dev in another terminal, or pass --port with the port it was started on.","docs":"https://skill.whisk.run/errors/DEV_NOT_RUNNING","details":{"url":"http://127.0.0.1:3001/v1/stub/inbox"}}}
```

## GIT_TLS_UNTRUSTED

Status: - · Surface: cli

When: `whisk deploy` pushed and git refused the certificate of the platform's git host ("SSL
certificate problem: unable to get local issuer certificate" and the like). On Windows the CLI
first retries the push once with `-c http.sslBackend=schannel`, so Git for Windows verifies with
the Windows certificate store instead of its bundled OpenSSL roots; this code means that retry
failed too, or the machine is not Windows. `details.host` names the host and `details.git` holds
git's last lines.

Fix: On Windows, run `git config --global http.sslBackend schannel` and deploy again. If that
does not help, or elsewhere, an HTTPS-inspecting proxy or antivirus is presenting its own
certificate: add its root to the certificate store git uses (or point `http.sslCAInfo` at a
bundle that includes it), then deploy again.

```json
{"error":{"code":"GIT_TLS_UNTRUSTED","message":"git does not trust the certificate of git.whisk.run.","fix":"Update the system CA certificates. If an HTTPS-inspecting proxy or antivirus is in the path, point git's http.sslCAInfo at a bundle that includes its root certificate, then run whisk deploy again.","docs":"https://skill.whisk.run/errors/GIT_TLS_UNTRUSTED","details":{"host":"git.whisk.run","git":["fatal: unable to access 'https://git.whisk.run/01J/a1.git/': SSL certificate problem: unable to get local issuer certificate"]}}}
```

## UNKNOWN_COMMAND

Status: - · Surface: cli

When: the CLI was given a command or flag this build does not have. The usual cause is a CLI
installed before the command shipped while the docs or skill describe the current one.
`details.whisk_cli` is the running version.

Fix: Run `whisk update` to install the CLI the platform serves, then retry. If the command is
still unknown, check the spelling against `whisk --help`.

```json
{"error":{"code":"UNKNOWN_COMMAND","message":"This whisk (pilot-66313c9) has no such command or flag: unknown command \"db\" for \"whisk\".","fix":"Run whisk update to install the platform's current CLI, then retry; run whisk --help for what this build offers.","docs":"https://skill.whisk.run/errors/UNKNOWN_COMMAND","details":{"whisk_cli":"pilot-66313c9"}}}
```

## CLI_ERROR

Status: - · Surface: cli

When: the CLI failed on this computer for a reason no other code names: a file it could not
read or write, a directory it could not create, a git command that could not start. The
message is the underlying error as the system gave it. A failure to reach the platform, or an
answer that did not come from it, is `PLATFORM_UNAVAILABLE` instead. The CLI exits 1.

Fix: Read the message and fix what it names on this computer (a permission, a full disk, a
missing program); if it names a platform problem, retry with backoff. If it names neither, report
it with `whisk feedback --kind bug --code CLI_ERROR` and the message.

```json
{"error":{"code":"CLI_ERROR","message":"open whisk.yaml: permission denied","fix":"Read the message; if it names a platform problem, retry with backoff.","docs":"https://skill.whisk.run/errors/CLI_ERROR"}}
```

## APP_OUT_OF_MEMORY

Status: - · Surface: container, run

When: the app's previous run was killed with SIGKILL that no stop preceded, which on Whisk means
it used more memory than its container's limit (files in `/tmp` count as memory). Under gVisor
the kernel kills the whole sandbox, so the app itself prints nothing more. At the moment of the
kill the platform writes a line into the app's logs (`whisk logs`, stream `platform`) starting
`whisk: killed at ... for going over its ... memory limit`; the node agent restarts the
container a second later and whisk-init prints this as the first line of the new run. A function
run in progress when it was killed is lost and fails: its run carries this code, with
`details.killed_at`, on `whisk runs` and the run's page. The org's owners are notified of the
first kill of an app, and then at most once per app per 24 hours.

Fix: Reduce the app's peak memory: stream large downloads and uploads instead of buffering them,
keep less in `/tmp` (hydrate only what a run needs, or read it from storage on demand), and
process work in smaller batches. Otherwise move the org to a plan with more memory.

```json
{"error":{"code":"APP_OUT_OF_MEMORY","message":"whisk-init: the previous run was killed with SIGKILL without a stop request, which means it used more than its memory limit of 256 MiB (files in /tmp count as memory).","fix":"Reduce its peak memory (stream large files, keep less in /tmp) or move to a plan with more memory.","docs":"https://skill.whisk.run/errors/APP_OUT_OF_MEMORY","details":{}}}
```

## APP_CPU_SLEEP

Status: - · Surface: container

When: an app on a plan that puts busy apps to sleep (Free) used all of its processor share (half
a core on Free) for ten minutes. The platform stops it as the idle sleep does and writes a line
into the app's logs (`whisk logs`, stream `platform`) starting `whisk: put to sleep at ... for
using all of its ... processor share`. The next request wakes it. A function run in progress is
cut off and fails as any interrupted run does. On paid plans a busy app keeps running and the
signal is only recorded for the operator.

Fix: Find the loop or heavy job that keeps the app busy, and spread the work out (smaller
batches, a scheduled function that does a little each time). Otherwise move the org to a paid
plan.

```json
{"error":{"code":"APP_CPU_SLEEP","message":"whisk: put to sleep at 09:30:05 UTC for using all of its 0.5 core processor share for ten minutes (APP_CPU_SLEEP). The next request wakes it.","fix":"Find the loop or heavy job that keeps it busy, spread the work out, or move to a paid plan, where a busy app keeps running.","docs":"https://skill.whisk.run/errors/APP_CPU_SLEEP","details":{}}}
```

## APP_MEMORY_HIGH

Status: - · Surface: container

When: the app's memory, or its peak since it started, reached 85 % of its container's limit
(files in `/tmp` count as memory). whisk-init prints it into the app's log with the numbers, at
most every ten minutes. Nothing is stopped yet; above the limit the app is killed at once with
`APP_OUT_OF_MEMORY`.

Fix: Reduce the app's peak memory before it reaches the limit: stream large downloads and
uploads instead of buffering them, keep less in `/tmp` (hydrate only what a run needs, or read it
from storage on demand), and process work in smaller batches. Otherwise move the org to a plan
with more memory.

```json
{"error":{"code":"APP_MEMORY_HIGH","message":"whisk-init: the app is using 230 MiB (peak 240 MiB, 93%) of its memory limit of 256 MiB, and files in /tmp count as memory.","fix":"Reduce its peak memory (stream large files, keep less in /tmp) or move to a plan with more memory.","docs":"https://skill.whisk.run/errors/APP_MEMORY_HIGH","details":{}}}
```

## INIT_CONFIG

Status: - · Surface: container

When: whisk-init has no app command, an invalid option, or a non-positive timeout or grace.

Fix: Correct the node-generated invocation; durations use Go syntax such as `28s`.

```json
{"error":{"code":"INIT_CONFIG","message":"whisk-init: invalid startup configuration.","fix":"Correct the command and positive timeout/grace durations.","docs":"https://skill.whisk.run/errors/INIT_CONFIG","details":{}}}
```

## INIT_PROCESS

Status: - · Surface: container

When: the runtime cannot enable child subreaping.

Fix: Check runtime support for PR_SET_CHILD_SUBREAPER; do not start without child reaping.

```json
{"error":{"code":"INIT_PROCESS","message":"whisk-init: cannot enable child reaping.","fix":"Check the container runtime configuration.","docs":"https://skill.whisk.run/errors/INIT_PROCESS","details":{}}}
```

## INIT_EXEC

Status: - · Surface: container

When: the image command is missing, cannot be resolved in PATH, or cannot be executed.

Fix: Correct the image command, interpreter, PATH and executable permissions.

```json
{"error":{"code":"INIT_EXEC","message":"whisk-init: cannot execute app.","fix":"Check the image command, PATH and executable permissions.","docs":"https://skill.whisk.run/errors/INIT_EXEC","details":{}}}
```

## INIT_SECRETS

Status: - · Surface: container

When: the secrets response is malformed, oversized, has an unsupported version or contains an
unrecognised error. Values, paths and upstream error text are not echoed.

Fix: Check node/init protocol compatibility and restart with a fresh token.

```json
{"error":{"code":"INIT_SECRETS","message":"whisk-init: invalid secrets response.","fix":"Check node/init protocol compatibility and issue a fresh start token.","docs":"https://skill.whisk.run/errors/INIT_SECRETS","details":{}}}
```

## INIT_SECRET_FILES

Status: - · Surface: container

When: a declared secret file cannot be created privately under /run/secrets.

Fix: Provide an app-owned 0700 tmpfs directory with no existing destination files.

```json
{"error":{"code":"INIT_SECRET_FILES","message":"whisk-init: cannot write private secret files.","fix":"Check ownership, permissions and available space under /run/secrets.","docs":"https://skill.whisk.run/errors/INIT_SECRET_FILES","details":{}}}
```

## INIT_NO_TOKEN

Status: - · Surface: container

When: whisk-init found no container token at start. A platform fault; the container is
restarted with a fresh token.

Fix: Nothing. If it repeats in the log, the operator has been paged.

```json
{"error":{"code":"INIT_NO_TOKEN","message":"whisk-init: no container token was provided; refusing to start.","fix":"No action; the platform restarts the container with a new token.","docs":"https://skill.whisk.run/errors/INIT_NO_TOKEN","details":{}}}
```

## TOKEN_INVALID

Status: - · Surface: container

When: the container's secrets token was not recognised by the node, for example after it
expired during a very slow image start.

Fix: Nothing; the container restarts with a fresh token. If the app's image takes minutes to
start, reduce its start time.

```json
{"error":{"code":"TOKEN_INVALID","message":"whisk-init: the secrets token was not accepted.","fix":"No action; the platform restarts the container with a new token.","docs":"https://skill.whisk.run/errors/TOKEN_INVALID","details":{}}}
```

## TOKEN_ALREADY_USED

Status: - · Surface: container

When: a second secrets fetch was attempted with a container token that was already used. This
is either a restart race or something inside the container trying to fetch secrets again, which
is reported to the operator as a security signal.

Fix: If the app itself connects to `/run/whisk/secrets.sock`, stop: secrets are already in the
environment. Otherwise no action.

```json
{"error":{"code":"TOKEN_ALREADY_USED","message":"whisk-init: secrets token already used; refusing to start.","fix":"Read secrets from the environment. Nothing in the app should connect to the secrets socket.","docs":"https://skill.whisk.run/errors/TOKEN_ALREADY_USED","details":{}}}
```

## UPSTREAM_UNAVAILABLE

Status: - · Surface: container

When: the node could not reach the control plane to fetch secrets within 30 seconds. whisk-init
exits so the platform retries.

Fix: Nothing; the container is restarted automatically. Persistent occurrences page the
operator.

```json
{"error":{"code":"UPSTREAM_UNAVAILABLE","message":"whisk-init: platform unavailable, exiting so the platform can retry.","fix":"No action; the container restarts automatically.","docs":"https://skill.whisk.run/errors/UPSTREAM_UNAVAILABLE","details":{}}}
```

## DISK_CRITICAL

Status: - · Surface: deploy, node

When: the node's disk is above 90% and the node agent refused to start a container or pull an image until space is freed. The deploy that hit it is failed with this code; the previous deploy stays live.

Fix: Nothing on the tenant's side; the operator is paged. Retry the deploy once the node reports space again.

```json
{"error":{"code":"DISK_CRITICAL","message":"The node is out of disk and refused to start job-tracker.","fix":"Retry in a few minutes; the operator has been paged.","docs":"https://skill.whisk.run/errors/DISK_CRITICAL","details":{"node":"node1","disk_used_percent":93}}}
```

## POSTGRES_DOWN

Status: - · Surface: deploy, node

When: the node's Postgres cluster did not answer, so a container start, a migration or a database creation was refused. Running containers are untouched.

Fix: Nothing on the tenant's side; the operator is paged. Retry the deploy once the node reports Postgres up.

```json
{"error":{"code":"POSTGRES_DOWN","message":"The database cluster on node1 is not answering, so job-tracker was not started.","fix":"Retry in a few minutes; the operator has been paged.","docs":"https://skill.whisk.run/errors/POSTGRES_DOWN","details":{"node":"node1"}}}
```

## IMAGE_NO_COMMAND

Status: - · Surface: deploy, node

When: the built image has neither an ENTRYPOINT nor a CMD, so there is nothing for whisk-init to run.

Fix: Add a CMD (or ENTRYPOINT) to the Dockerfile that starts the server on PORT, or remove the Dockerfile so Railpack builds one.

```json
{"error":{"code":"IMAGE_NO_COMMAND","message":"The image for job-tracker defines no command to run.","fix":"Add a CMD line, such as CMD node dist/index.js or your server start command, to the Dockerfile and deploy again.","docs":"https://skill.whisk.run/errors/IMAGE_NO_COMMAND","details":{"image":"01J.../01J...@sha256:..."}}}
```

## COMMAND_NOT_RUNNABLE

Status: - · Surface: deploy, node

When: the container's command could not be run at all: the `run` or `migrate` command in whisk.yaml, or the image's ENTRYPOINT or CMD, is not in the image, is not executable, or is built for another CPU. Nothing of the app ran; a migration changed nothing. The runtime's words are in message.

Fix: Make the command exist in the image and be executable: install the tool the command names (for example add prisma to dependencies rather than devDependencies, or call it through npx), use its full path, chmod +x a script, or build for linux/amd64. Deploy again.

```json
{"error":{"code":"COMMAND_NOT_RUNNABLE","message":"The migration did not run: the image has no \"prisma\" on its PATH, so the command could not be started. Nothing ran, so the database is as it was.","fix":"Make the command exist in the image and be executable, then deploy again.","docs":"https://skill.whisk.run/errors/COMMAND_NOT_RUNNABLE","details":{"fault":"app","step":"migrate","command":"prisma migrate deploy"}}}
```

## CONTAINER_START_FAILED

Status: - · Surface: deploy, node

When: Docker or the sandbox runtime refused to start the container. The node's diagnosis is in message; the container's first log lines, when any, are in details.

Fix: Read the message and the log lines: this is a runtime error on Whisk's side, and the operator has been told. A command the image cannot run is COMMAND_NOT_RUNNABLE instead.

```json
{"error":{"code":"CONTAINER_START_FAILED","message":"runsc could not start job-tracker: exec /app/start: permission denied.","fix":"Make the start command executable in the image (chmod +x in the Dockerfile) and deploy again.","docs":"https://skill.whisk.run/errors/CONTAINER_START_FAILED","details":{"log_tail":["exec /app/start: permission denied"]}}}
```

## CONTAINER_NOT_FOUND

Status: - · Surface: node

When: the control plane referred to a container the node no longer has, for example after a node rebuild pruned it.

Fix: The control plane redeploys the last live build; retry the operation once the deploy is live.

```json
{"error":{"code":"CONTAINER_NOT_FOUND","message":"The container for job-tracker no longer exists on node1.","fix":"Wait for the redeploy, then retry.","docs":"https://skill.whisk.run/errors/CONTAINER_NOT_FOUND","details":{"container_id":"3f2a..."}}}
```

## TOKEN_MALFORMED

Status: - · Surface: node, container

When: a container presented something on the secrets socket that is not a 32-byte base64url token, so the node answered without looking anything up.

Fix: Nothing to do in the app: whisk-init writes the token. If it recurs, the token file was altered inside the container; report it.

```json
{"error":{"code":"TOKEN_MALFORMED","message":"The token presented on the secrets socket was not a Whisk token.","fix":"Restart the container; if it recurs, report it to Whisk.","docs":"https://skill.whisk.run/errors/TOKEN_MALFORMED","details":{}}}
```

## WAKE_TIMEOUT

Status: 504 · Surface: edge, node

When: a sleeping app was asked to wake and its container did not reach a healthy state within the wake timeout (60 seconds). The edge shows the platform's timeout page; the node reports the wake.

Fix: Retry in a moment. An app that regularly takes longer than a few seconds to answer its health route should start faster (CONTRACT.md §2, rule 9) or be always_on.

```json
{"error":{"code":"WAKE_TIMEOUT","message":"Starting job-tracker took too long.","fix":"Retry in a moment. If it keeps timing out, make the app start faster or set always_on in whisk.yaml.","docs":"https://skill.whisk.run/errors/WAKE_TIMEOUT","details":{"app":"job-tracker","retry_after":5}}}
```

## WAKE_FAILED

Status: 500 · Surface: edge, node

When: a sleeping app was asked to wake and its container could not be started at all (the
image is gone from the node, the container would not create, or its network or secrets could
not be prepared), as opposed to starting and not becoming healthy (`WAKE_TIMEOUT`). The edge
shows the platform's error page; `message` carries the node's own words.

Fix: Run `whisk status` and `whisk logs`; if the app stays unreachable, run `whisk deploy`
again, which places a fresh container. The operator sees the node's cause.

```json
{"error":{"code":"WAKE_FAILED","message":"The container for job-tracker could not be started: image not present on the node.","fix":"Run whisk deploy if the app stays unreachable.","docs":"https://skill.whisk.run/errors/WAKE_FAILED","details":{}}}
```

## FORBIDDEN

Status: 403 · Surface: node

When: something other than the edge on the same node, or other than the platform's mesh, called
one of the node agent's own endpoints (the wake call, the metrics, a test endpoint). An app or
an agent never meets it: the node agent listens only inside the platform.

Fix: Nothing to do in an app. Call the platform through the API or the app's hostname; the
node agent's endpoints are for the edge and the control plane only.

```json
{"error":{"code":"FORBIDDEN","message":"Only the edge on this node may call the node agent.","fix":"Call the platform through the API or the app's hostname; the node agent answers only the edge and the mesh.","docs":"https://skill.whisk.run/errors/FORBIDDEN","details":{}}}
```

## FREE_CAPACITY_BUSY

Status: 503 · Surface: edge, node, deploy

When: an app on the Free plan had to start (a wake, or a deploy's new container) and the free
apps on its node already use all the memory set aside for them. To make room the node puts to
sleep the free apps used least recently, but none that had a request in the last minute; this
answer means every one of them is busy right now. Nothing is wrong with the app. A browser sees
the app's starting page, which tries again by itself every few seconds; any other client gets
this body with `Retry-After`. A deploy that meets it fails with this code, and the live deploy
keeps serving. Apps on a paid plan never wait for this.

Fix: Retry in a few seconds, or move the app to a paid plan so it never waits.

```json
{"error":{"code":"FREE_CAPACITY_BUSY","message":"Free apps are busy right now, so job-tracker has to wait a few seconds to start.","fix":"Retry in a few seconds, or move the app to a paid plan so it never waits.","docs":"https://skill.whisk.run/errors/FREE_CAPACITY_BUSY","details":{"app":"job-tracker","retry_after":5}}}
```

## INVALID_ARGUMENT

Status: - · Surface: node

When: the control plane sent the node a request it could not act on: a missing request_id, an empty image, an app id that is not a ULID, a malformed restore target. This is a platform bug, never a tenant's.

Fix: Nothing a tenant sends causes this. If it reaches you, report it with `whisk feedback
--kind bug --code INVALID_ARGUMENT`, naming the request id or the deploy id it came with.

```json
{"error":{"code":"INVALID_ARGUMENT","message":"StartContainer was called without an image.","fix":"Report it with whisk feedback --kind bug --code INVALID_ARGUMENT and the request id.","docs":"https://skill.whisk.run/errors/INVALID_ARGUMENT","details":{"field":"image"}}}
```

## CONTROL_PLANE_SUPERSEDED

Status: - · Surface: node

When: a control plane copy that is no longer live sent this command, so the node refused it.
Every command to a node carries the epoch of the copy that sent it (NODE-AGENT.md §A13); the
node refuses one older than the highest epoch it has seen. A tenant never causes this.

Fix: Nothing to do: the live control plane carries the work out. If it persists, check which
copy is live with `whiskd live`.

```json
{"error":{"code":"CONTROL_PLANE_SUPERSEDED","message":"A control plane copy that is no longer live sent this command, so node1 refused it.","fix":"Nothing to do: the live control plane carries the work out. If it persists, check which copy is live with whiskd live.","docs":"https://skill.whisk.run/errors/CONTROL_PLANE_SUPERSEDED","details":{"node":"node1","epoch":4,"highest_seen":5}}}
```

## DATABASE_FAILED

Status: - · Surface: deploy, node

When: the node could not prepare, rotate or drop an app's database, role or cache. On a deploy
it arrives as `PLATFORM_DEPLOY_FAILED` with this code as `details.step_code`; the message names the node and the part of it that failed, and `details` carries `step`
(`database` or `cache`), `node`, `cause_code` (the node's own code: `POLICY_APPLY_FAILED` for
its network rules, `DOCKER_FAILED`, `POSTGRES_DOWN`, `DISK_CRITICAL`, or `DATABASE_FAILED` for
Postgres itself), `cause` (the node's own words) and `fault: platform`. No password is ever
included.

Fix: Nothing in the app causes this, and the operator has been paged with the cause. Run
`whisk deploy` again later; if it fails the same way, report it with `whisk feedback --kind bug
--code DATABASE_FAILED`, naming the deploy id and `details.cause`.

```json
{"error":{"code":"DATABASE_FAILED","message":"The app's database could not be prepared on node node1: the node's network rules would not load (nft: exit status 1: /dev/stdin:14:40-58: Error: Could not process rule: Invalid argument).","fix":"Nothing in the app causes this, and the operator has been paged with the cause. Run whisk deploy again later; if it fails the same way, report the deploy id with details.cause.","docs":"https://skill.whisk.run/errors/DATABASE_FAILED","details":{"step":"database","node":"node1","cause_code":"POLICY_APPLY_FAILED","cause":"nft: exit status 1: /dev/stdin:14:40-58: Error: Could not process rule: Invalid argument","fault":"platform"}}}
```

## DATABASE_CREDENTIALS_REJECTED

Status: 503 · Surface: api, deploy

When: an environment's database refused the password Whisk holds for it. The password is the
platform's, so nothing in the app causes this. From `whisk db query` and `whisk db schema` it is
the answer itself, with `details.node` and `fault: platform`; on a deploy it is the
`details.cause_code` of a `PLATFORM_DEPLOY_FAILED` whose step code is `MIGRATE_FAILED`. Every
deploy checks the password before it migrates and puts it back in step when it is refused.

Fix: Deploy the app again (`whisk deploy`); the deploy puts the password back in step. If the
answer comes back after that, send it with `whisk feedback`.

```json
{"error":{"code":"DATABASE_CREDENTIALS_REJECTED","message":"The production database refused the password Whisk holds for this environment. This is on Whisk's side, not the app's.","fix":"Deploy the app again: a deploy puts the password back in step. If this answer comes back after that, send it with whisk feedback.","docs":"https://skill.whisk.run/errors/DATABASE_CREDENTIALS_REJECTED","details":{"node":"node1","fault":"platform"}}}
```

## RESTORE_FAILED

Status: - · Surface: deploy, node

When: a point-in-time restore did not complete: the base backup could not be fetched, the temporary instance did not promote, or the dump into the target failed. The temporary instance is stopped and removed and a half-restored database is dropped, so nothing is left behind.

Fix: Read the step and the instance's log tail in details. Retry the restore; the live database
is untouched. If it fails the same way, report it with `whisk feedback --kind bug --code
RESTORE_FAILED` and the restore id.

```json
{"error":{"code":"RESTORE_FAILED","message":"The restore of job-tracker to 2026-09-07T14:13:00Z failed while promoting the temporary instance.","fix":"Retry the restore. If it fails the same way, report it with whisk feedback --kind bug --code RESTORE_FAILED and the restore id; the live database is untouched.","docs":"https://skill.whisk.run/errors/RESTORE_FAILED","details":{"step":"promote","log_tail":["FATAL: recovery target not reached"]}}}
```

## NODE_STATE_FAILED

Status: - · Surface: node

When: the node could not write to its own working directory (WHISK_STATE), so it could not keep
the file it was handed while scanning an upload, converting a video or resizing an image: the
disk is full or read-only, or the directory's permissions are wrong. The upload itself is
untouched in the store.

Fix: Nothing on the tenant's side; the operator reads the node log. Retry in a few minutes.

```json
{"error":{"code":"NODE_STATE_FAILED","message":"The node could not write to its working directory, so team-photo.jpg was not resized.","fix":"Retry in a few minutes; the operator has been notified.","docs":"https://skill.whisk.run/errors/NODE_STATE_FAILED","details":{"node":"node1"}}}
```

## DOCKER_FAILED

Status: - · Surface: deploy, node

When: the Docker Engine returned an error the node could not classify further (network creation, inspect, stop, remove). The daemon's message is in message.

Fix: The operator reads the node log. Retry the deploy once fixed.

```json
{"error":{"code":"DOCKER_FAILED","message":"Docker refused to create the network for job-tracker.","fix":"Retry in a few minutes; the operator has been notified.","docs":"https://skill.whisk.run/errors/DOCKER_FAILED","details":{"operation":"network create"}}}
```

## SEARCH_KEY_INVALID

Status: 400 · Surface: api

When: the operator connected Google Search Console to the weekly numbers (`PUT
/v1/operator/numbers/search`) with something that is not a service account's JSON key: not
JSON, a `type` other than `service_account`, no `client_email`, or a `private_key` that is not
an RSA key in PEM; or with no key while nothing is connected. The import's own `search.error`
carries it too when the stored key can no longer be opened.

Fix: In Google Cloud, open IAM and admin, Service accounts, choose the account, then Keys, Add
key, Create new key, JSON, and paste the whole file that downloads.

```json
{"error":{"code":"SEARCH_KEY_INVALID","message":"That is not a service account's JSON key: its \"type\" is not \"service_account\".","fix":"In Google Cloud, open IAM and admin, Service accounts, choose the account, then Keys, Add key, Create new key, JSON, and paste the whole file that downloads.","docs":"https://skill.whisk.run/errors/SEARCH_KEY_INVALID"}}
```

## SEARCH_KEY_REJECTED

Status: - · Surface: api

When: Google refused to sign the service account in with the key Search Console was connected
with: the key or the account was deleted or disabled. It is the weekly numbers' `search.error`;
the import is tried again in 30 minutes.

Fix: In Google Cloud, create a new JSON key for the service account and connect again with it on
the weekly numbers page.

```json
{"error":{"code":"SEARCH_KEY_REJECTED","message":"Google refused the service account's key: Invalid JWT Signature.","fix":"The key or the service account numbers@whisk-numbers.iam.gserviceaccount.com may have been deleted or disabled. In Google Cloud, create a new JSON key for the service account and paste it here.","docs":"https://skill.whisk.run/errors/SEARCH_KEY_REJECTED"}}
```

## SEARCH_ACCESS_DENIED

Status: - · Surface: api

When: Google signed the service account in but will not let it read the Search Console property:
the account's email is not a user on the property, or the property is named differently (a
domain property is `sc-domain:<domain>`, a URL-prefix property its address with a trailing
slash). It is the weekly numbers' `search.error`; the import is tried again in 30 minutes.

Fix: In Search Console, open the property, then Settings, Users and permissions, Add user, and
add the service account's email (Restricted is enough). If the property is named differently,
connect again with its exact name.

```json
{"error":{"code":"SEARCH_ACCESS_DENIED","message":"Google says numbers@whisk-numbers.iam.gserviceaccount.com cannot read the Search Console property sc-domain:whisk.run: User does not have sufficient permission for site 'sc-domain:whisk.run'.","fix":"In Search Console, open the property sc-domain:whisk.run, then Settings, Users and permissions, Add user, and add numbers@whisk-numbers.iam.gserviceaccount.com (Restricted is enough). If the property is named differently, connect again with its exact name.","docs":"https://skill.whisk.run/errors/SEARCH_ACCESS_DENIED"}}
```

## SEARCH_API_DISABLED

Status: - · Surface: api

When: the Google Search Console API is not turned on in the Google Cloud project the service
account belongs to. It is the weekly numbers' `search.error`; the import is tried again in 30
minutes.

Fix: In Google Cloud, open APIs and services, Library, find Google Search Console API and choose
Enable, then wait a few minutes.

```json
{"error":{"code":"SEARCH_API_DISABLED","message":"The Google Search Console API is not turned on for the service account's project.","fix":"In Google Cloud, open APIs and services, Library, find Google Search Console API and choose Enable, then wait a few minutes; Whisk tries again in 30 minutes.","docs":"https://skill.whisk.run/errors/SEARCH_API_DISABLED"}}
```

## SEARCH_UNAVAILABLE

Status: - · Surface: api

When: Google did not answer the import, answered with an error on its side (a 5xx or too many
requests), or answered something the import could not read. It is the weekly numbers'
`search.error`; what was imported before stays.

Fix: Nothing to do: the import is tried again in 30 minutes. If it keeps failing, check
`https://status.cloud.google.com`.

```json
{"error":{"code":"SEARCH_UNAVAILABLE","message":"Google answered 503: Backend Error.","fix":"Nothing to do now: Whisk tries again in 30 minutes. If it keeps failing, check https://status.cloud.google.com.","docs":"https://skill.whisk.run/errors/SEARCH_UNAVAILABLE"}}
```
