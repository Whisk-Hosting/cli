# whisk-go

A starting point for a business app on Whisk: net/http with chi, pgx with goose on Postgres, the
Inngest Go SDK for background work, slog for logs. It follows every convention in the agent
skill (https://whisk.run/skill) and deploys with nothing for a person to set up.

What is here: a home page that greets whoever is signed in, `/health`, a `notes` table with its
migration, `GET`/`POST /notes` (any signed-in person) and `DELETE /notes/{id}` (owners and
admins), and one function, `note-added`, which runs on the queue after each new note and counts
its words, with its graph in `workflows/`. Replace the notes with the app's own data; keep the
rest.

```
main.go            routes, request logging, shutdown, the migrate subcommand
whisk.go           identity, logging, tracing, the Inngest client, and the approval, enqueue and
                   webhook delivery helpers (copy this into any app)
functions.go       note-added (event note.added)
db.go              pool and queries · migrations/ goose SQL, embedded
sync.go            PullTree/PushTree, for a job that keeps a tree of files (needs storage: true)
```

## Run it

`whisk dev` needs Docker with the compose plugin. It starts Postgres, the workflow engine and a
local edge that signs you in, then runs the app with the environment it has in production:

```
go build -o server .
whisk dev --as you@example.com --roles owner -- go run .
```

`whisk dev` runs the migration (`./server migrate`, which the build makes) before it starts the
app. On Windows, run `whisk dev` for this template inside WSL: Windows runs the migrate command
through the Command Prompt, which does not take the Unix path `./server`. Open
http://127.0.0.1:3000, then http://127.0.0.1:3000/notes. Secrets for local runs, once the app
declares any, go in `.whisk/dev/secrets.env` as `NAME=value` lines; the folder is git-ignored.

## Test it

`go test ./...` runs the tests of the helpers in `whisk.go` and `sync.go`.

## Ship it

`whisk doctor`, then `whisk deploy -m "<what changed, for the people who use the app>"`. The image
is distroless with one static binary; migrations run from `./server migrate` before traffic
switches.

When the app needs a secret (an API key for another service), declare its name under `secrets`
in `whisk.yaml`; a person sets the value in the dashboard. For a webhook, a schedule, storage or
email, see the skill.
