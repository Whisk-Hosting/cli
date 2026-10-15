# whisk-python

A starting point for a business app on Whisk: FastAPI, SQLAlchemy with Alembic on Postgres, the
Inngest SDK for background work, structlog for logs. It follows every convention in the agent
skill (https://whisk.run/skill) and deploys with nothing for a person to set up.

What is here: a home page that greets whoever is signed in, `/health`, a `notes` table with its
migration, `GET`/`POST /notes` (any signed-in person) and `DELETE /notes/{id}` (owners and
admins), and one function, `note-added`, which runs on the queue after each new note and counts
its words, with its graph in `workflows/`. Replace the notes with the app's own data; keep the
rest.

```
app/main.py        routes, request logging, shutdown
app/whisk.py       identity, logging, tracing, the Inngest client, and the approval, enqueue and
                   webhook delivery helpers (copy this into any app)
app/functions.py   note-added (event note.added)
app/db.py          models and engine · alembic/ migrations
app/sync.py        pull_tree/push_tree, for a job that keeps a tree of files (needs storage: true)
```

## Run it

`whisk dev` needs Docker with the compose plugin. It starts Postgres, the workflow engine and a
local edge that signs you in, then runs the app with the environment it has in production:

```
uv sync
uv run whisk dev --as you@example.com --roles owner -- uvicorn app.main:app --port 8080
```

These lines work the same in PowerShell, Command Prompt and a Unix shell. `uv run` puts the
project's tools on the path, so `whisk dev` can run the migration (`alembic upgrade head`) before
it starts the app. Open http://127.0.0.1:3000, then http://127.0.0.1:3000/notes. Secrets for
local runs, once the app declares any, go in `.whisk/dev/secrets.env` as `NAME=value` lines; the
folder is git-ignored.

## Test it

`uv run pytest` runs the tests of the helpers in `app/whisk.py` and `app/sync.py`.

## Ship it

`whisk doctor`, then `whisk deploy -m "<what changed, for the people who use the app>"`. The image
is Python slim with the locked dependencies; migrations run from `alembic upgrade head` before
traffic switches.

When the app needs a secret (an API key for another service), declare its name under `secrets`
in `whisk.yaml`; a person sets the value in the dashboard. For a webhook, a schedule, storage or
email, see the skill.
