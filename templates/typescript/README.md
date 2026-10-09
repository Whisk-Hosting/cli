# whisk-typescript

A starting point for a business app on Whisk: Hono on Node, Drizzle on Postgres, the Inngest SDK
for background work, pino for logs. It follows every convention in the agent skill
(https://whisk.run/skill) and deploys with nothing for a person to set up.

What is here: a home page that greets whoever is signed in, `/health`, a `notes` table with its
migration, `GET`/`POST /notes` (any signed-in person) and `DELETE /notes/:id` (owners and admins),
and one function, `note-added`, which runs on the queue after each new note and counts its words,
with its graph in `workflows/`. Replace the notes with the app's own data; keep the rest.

```
src/index.ts       routes, request logging, shutdown
src/whisk.ts       identity, logging, tracing, the Inngest client, and the approval, enqueue and
                   webhook delivery helpers (copy this into any app)
src/functions.ts   note-added (event note.added)
src/schema.ts      tables · src/db.ts client · src/migrate.ts runs drizzle/
src/sync.ts        pullTree/pushTree, for a job that keeps a tree of files (needs storage: true)
```

## Run it

`whisk dev` needs Docker with the compose plugin. It starts Postgres, the workflow engine and a
local edge that signs you in, then runs the app with the environment it has in production:

```
npm install
npm run build
whisk dev --as you@example.com --roles owner -- npm run dev
```

These lines work the same in PowerShell, Command Prompt and a Unix shell. `whisk dev` runs the
migration (`node dist/migrate.js`, which the build makes) before it starts the app. Open
http://127.0.0.1:3000, then http://127.0.0.1:3000/notes. Secrets for local runs, once the app
declares any, go in `.whisk/dev/secrets.env` as `NAME=value` lines; the folder is git-ignored.

## Test it

`npm test` runs the unit tests of the helpers in `src/whisk.ts`. `npm run typecheck` checks the
types.

## Ship it

`whisk doctor`, then `whisk deploy -m "<what changed, for the people who use the app>"`. The build
bundles each entry point into one file with esbuild, so the image is distroless Node with no
`node_modules`; migrations run from `node dist/migrate.js` before traffic switches.

When the app needs a secret (an API key for another service), declare its name under `secrets`
in `whisk.yaml`; a person sets the value in the dashboard. For a webhook, a schedule, storage or
email, see the skill.
