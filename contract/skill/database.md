# Whisk skill §5: The database in full

The rest of §5 of the Whisk skill (https://skill.whisk.run/), which an agent reads first. Read it when the app keeps one customer's rows from another's, needs schema guards, restores, or brings an existing database.

**Row-level security: one customer never sees another's rows.** When the app serves customers
(`customer_identity`), or people who must not see each other's records, every table holding rows
that belong to someone gets a policy, so a query that forgets its `where` still answers only the
caller's rows. Add it in a migration, naming the column that holds the owner's
`X-Whisk-User-Id`:

```sql
create index orders_owner_id on orders (owner_id);
alter table orders enable row level security;
alter table orders force row level security;
create policy orders_by_audience on orders using (
    current_setting('whisk.audience', true) in ('team', 'system')
    or owner_id = nullif(current_setting('whisk.user_id', true), '')
);
```

`force` matters: the app's role owns the table, and an owner skips policies without it. The
policy lets the team and the app's own work see every row and a customer only their own. Then
every query on such a table goes through the template's helper, which tells Postgres who is
asking at the start of one short transaction: `dbFor(who(c), (tx) => …)` in TypeScript,
`with db_for(identity(request.headers)) as session:` in Python, `db.For(ctx, callerOf(identityFrom(r.Header)), func(tx pgx.Tx)
error {…})` in Go. Functions and webhook deliveries, which run for nobody in particular, use
`asSystem` (`as_system`, `db.AsSystem`); never use it to answer a person's request. A query that
skips the helper sees none of the rows and cannot write them, which is the safe way to fail.
Keep slow work (calls to other services) outside the helper, since the transaction holds a
pooled connection until it ends.

To tailor it: a table every signed-in customer may read but only the team may change (a price
list) needs no policy; a team member who should see only their own rows drops `'team'` from the
list; rows shared by a customer's company use a column holding that company's id instead of
the person's. A migration that reads or changes rows of a policy table starts with
`select set_config('whisk.audience', 'system', true);` (DDL needs nothing). `whisk db query`
runs as `system` and sees every row. Another app reading a shared schema is filtered by the
owner's policies too: it sets `system` the same way when it should see every row. `whisk dev`
connects as a superuser, which policies never filter, so prove isolation on a deploy.

**Keep the app's own queries from changing the schema.** Without `database_role` the app connects
as the role that owns its tables (the default, kept so apps written that way keep running), so a
bug or an injected statement could turn row-level security off, drop a policy or rewrite a table.
The starters set `database_role: restricted`, and every new app should: the app connects as a second
login, `<owner>_run`, that owns nothing: it reads and writes rows, under every policy, and cannot
change a table, a policy or anything else the migrations made. Only the `migrate` step connects
as the owner. Use it for any app whose data is kept apart with row-level security, and for an
app moving to Whisk that already runs as a restricted role.

```yaml
database: app
database_role: restricted          # owner (default) | restricted; needs a migrate command
migrate: "npx prisma migrate deploy"
```

- `DATABASE_URL` in the running app is the run login (pooled, as always); in `migrate` it is the
  owner (direct). Use each as-is. Prisma users point `url` at `DATABASE_URL` and need no
  `directUrl`: the migrate step's `DATABASE_URL` is already direct.
- Every table, view and sequence the owner makes is readable and writable by the run login
  without a grant, now and later (`select`, `insert`, `update`, `delete`; `usage` and `update`
  on sequences). It has no `truncate`, cannot create anything, and cannot `set role` to the
  owner, so drop any `SET ROLE` or `SET LOCAL ROLE` the app used to switch to a restricted role
  itself: the login already is one.
- Schema changes belong in `migrate` alone. An app that creates tables when it starts (`create
  table if not exists` at boot, `prisma migrate deploy` or `db push` in the start command) fails
  under the run login with `permission denied`; move that into `migrate`. `whisk doctor` warns
  (W105, W102).
- Make a table append-only with policies, not with `revoke`: give it a `for select` and a `for
  insert` policy and no `update` or `delete` policy, and the run login can add rows and never
  change them. A `revoke` in a migration also works and lasts across deploys, but a restore that
  swaps the database in gives the run login its grants on every table again; policies are part
  of the schema and come back with it.
- In a shared database, a reader that runs with `database_role: restricted` connects as
  `app_<reader id>_run`: grant that login what you grant `app_<reader id>`, in the same three
  statements. It exists once the reader has deployed with the setting, and a grant to a login
  that does not exist fails, so add it after.
- `whisk db query`, `db schema` and `db url` act as the owner, so a person can still inspect and
  repair anything. `whisk dev` connects as a superuser, so prove the restriction on a deploy.

Turning it on for a live app takes one deploy: it makes the login, grants it everything the
owner already has, migrates as the owner and starts the app as the login. Run `whisk doctor`
first, since anything the app changes in the schema while it runs now fails. Removing the line
starts the app as the owner again on the next deploy.

**Looking at the data from your machine.** The database's own address is inside the platform, so
connecting to it from outside does not work (`whisk db url` says so). Use these instead; they
run on the platform, need nothing installed, and are audited:

- `whisk db schema --json`: the PostgreSQL version and every table with its columns, keys and
  estimated row count. Read it first when planning an import or a migration.
- `whisk db query "select … limit 20" --json`: the rows as JSON, values as PostgreSQL prints
  them. `--file script.sql` runs a script. Statements stop after 30 seconds; at most `--limit`
  rows come back (1000 by default).
- Reads and additions run at once. SQL that would change or remove existing data (`update`,
  `delete`, upserts, `drop`, `alter`, `truncate`) does not run: it comes back as
  `CONFIRM_REQUIRED` saying what it would do and carrying a code. Check that the effect is what
  you meant, then run `whisk db query --confirm <code>`. A restore point is taken first, and
  the result names the `whisk restore --at` that undoes it, on a plan with self-service restore.
  On Free nothing undoes it (`details.restorable` is `false`): copy the rows it changes first
  with `whisk db query "select …" --json`.
- Do not put `begin` or `commit` in the SQL: each call is already one transaction.

**Undoing a mistake.** Backups are continuous, and `whisk restore` returns the database to any
minute in the plan's window: the last 7 days on Starter and Team, 30 on Business; Free
has no self-service restore. `whisk db snapshot` marks a restore point before risky work and
prints the command that returns to it, or says the plan cannot. `whisk restore --at <time or 2h>`
restores into a new database beside the live one, which `whisk db query --database <name>` reads,
so you can check it first; `--swap` puts it live, and `whisk restore discard <id>` drops the copy
once you are done with it (it goes on its own after 30 days). `whisk restore --before <deploy-id>` goes back
to just before that deploy's migration instead. When the live version's migration was the
mistake, `whisk rollback --data` makes the version before it live again on production's data as
it was just before that migration: the app's requests wait while the databases change places and
that version starts, the replaced database is kept for 7 days, and a rollback that cannot go live
puts the database back as it was. A restore runs in the background: add `--wait`
to follow it, or `whisk restore show <id> --wait` later. An owner or admin downloads all of the
business's data with `whisk export --wait`; its link works for five minutes, and
`whisk export show <id>` signs a new one while the archive is kept (seven days).

**Bringing an existing database.** When an app moves to Whisk with data it already has, dump the
old database in custom format with PostgreSQL 18's `pg_dump` or older, `pg_dump -Fc --no-owner
--no-acl -f app.dump "$OLD_DATABASE_URL"`, deploy the app once so its database exists, then run
`whisk db import app.dump --wait`. The dump is uploaded over HTTPS and loaded into a new database
beside the live one, named in the result; check it with `whisk db query --database <name>`, then
`whisk db import app.dump --swap --wait` makes it the live database and keeps the previous one
for 7 days. Owners and grants in the dump are dropped and everything belongs to the app, so the
app's migrations must not recreate tables the dump already has. A plain SQL dump is refused;
there is no database port to connect to and none is needed. An app on a shared database cannot
take an import. Up to 5 GB.
