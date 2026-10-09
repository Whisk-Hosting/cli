# A restricted database role

The manifest takes `database_role: owner | restricted` (default `owner`). With `restricted` the
running app's `DATABASE_URL` is a run login, `<owner>_run`, that reads and writes rows and owns
nothing, and only the `migrate` step connects as the owner, so the app's own queries cannot turn
row-level security off, drop a policy or change a table. It needs a database and a `migrate`
command (`MANIFEST_INVALID` otherwise). SKILL.md §5 says how to use it, how to make a table
append-only with policies, and what to grant a restricted reader in a shared database. Doctor
adds W105 (a schema change outside the migrations, or `SET ROLE`, in a restricted app's code),
and W102 says a migration at start stops a restricted app from starting. The Go canary adds
`POST /diag/pg/guard`.
