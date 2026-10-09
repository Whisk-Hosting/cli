# The skill says storage is on every plan

The `whisk.yaml` reference in `SKILL.md` marked `storage`, `email` and `customer_identity` with
the same "plan" tag as `kv` and `always_on`, so an agent on the free app read storage as
unavailable and kept files out of it. The tags now say "every plan" for storage, email and
customer identity, and "paid" for `kv` and `always_on`, matching the plans (DESIGN.md §11).
