# Row-level security

SKILL.md §5 adds row-level security: the policy to put on any table a customer or person owns,
the templates' `dbFor`/`db_for`/`DB.For` and `asSystem`/`as_system`/`DB.AsSystem` helpers that
tell Postgres who is asking, and how to tailor the policy. The templates keep `notes` under it,
the canaries add `GET /diag/rows` and `canary.test` checks it, and doctor adds the warning W095
for a table an app with `customer_identity` creates without it.
