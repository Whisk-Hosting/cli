---
name: build-on-whisk
description: Build a web app or website with AI and get it live fast on Whisk, with a Postgres database, sign-in, file storage, scheduled jobs and secrets built in; or check on, fix, change or query an app already on Whisk.
---

# Build and run apps on Whisk

Whisk hosts the apps a business builds with AI. Each app gets its own web address, sign-in for
the people the business chooses, a database, backups, scheduled jobs, secrets and logs. The
person's own instructions come first: where they ask for something different from this guide,
do what they ask, and say so if Whisk cannot do it.

## Build a new app

1. Call `list_apps` first. If an existing app already covers the request, change that app
   rather than making a new one, and say which app you are changing.
2. Call `whisk_build_guide` once in the conversation, before writing any code. Follow it: it says
   how an app must be built to run on Whisk.
3. Ask a question only when the answer changes what you build, such as who should be able to
   open the app. Otherwise pick a plain default, say which, and build.
4. Write the app, then call `deploy_app` with its files. Use a short lower-case name, such as
   `leave-requests`.
5. Call `get_deploy` until the deploy is live or has failed. When it is live, give the person the
   web address and one sentence on what the app does. When it fails, read the error, fix the
   code and deploy again; do not ask the person to fix it.

Use what Whisk already provides (sign-in, the database, scheduled jobs, secrets, file storage,
email, logs) rather than adding an outside service, unless the person asks for that service by
name.

## Connect software the business already uses

To pass data between systems (a shop and an accounting package, a form and a CRM), build an app
that receives the other system's webhooks through Whisk's verified webhooks and does the work in
a background job that retries. Keep keys and passwords out of the code: declare each secret the
app needs, then either set it with `set_secret_value`, which shows the person the change to
confirm, or tell them where to set it on the Whisk dashboard. Give them the webhook address to
enter in the other system.

## Move or fix an existing website

When the person's website is slow, hacked or out of date, or they want to move a WordPress, Wix,
Squarespace or other website onto Whisk, rebuild it as an Astro site (the build guide covers Astro). Keep every page, its content and its address,
so links and search results still work. Ask for the site's address if they have not given it, and
deploy it like any new app. Pointing their domain at Whisk is their step; say what to change.

## Check on or fix an app

- "Is it working?": call `get_app`, then `get_logs` for recent errors. Answer in a sentence or
  two; quote a log line only when it explains the problem.
- A failed scheduled job or workflow: `list_runs`, then `get_run` for the failure. Fix the code
  if the failure is in the app, deploy, and only then `replay_run`.
- A broken release: `roll_back_app` puts back the previous version. Say which version is live
  afterwards.
- To change an app, read its code with `read_app_files` first, then deploy the changed files.

## Answer questions from an app's data

Call `get_database_schema` first, then `query_database` with a read-only query. Answer with the
result, not the query. Never guess a number you did not read. A query that changes or removes
existing data returns a confirmation step: show it to the person and run it only after they say
yes.

## Changes the person confirms

`set_secret_value`, `invite_member` and `give_app_access` show the person what will change and
wait for their yes. Never confirm on their behalf, and never invite anyone as an owner.

## What stays on the Whisk dashboard

Deleting an app, removing a person, changing someone's role, billing and plans are done on the
Whisk dashboard at whisk.run, not from the chat. When asked, say so and give the address; do
not try to work around it.

When Whisk itself gets in the way, tell the Whisk team with `send_feedback`.
