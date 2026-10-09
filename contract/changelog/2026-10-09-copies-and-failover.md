# Copies and failover

The operator's node record (`GET /operator/nodes`) carries `copy` {holder, state, lag_ms,
behind_bytes, line} and, while a failover is not yet fenced, `failover` {started_at, holder,
apps, stranded, reason, fenced_at}, naming apps by slug. New error codes: `DATABASE_MOVE_FAILED` (a deploy's step code
when a database could not be moved), `COPY_UNAVAILABLE`, `COPY_BEHIND` and `COPY_VERSION_MISMATCH` (node codes; the last when a
copy's source runs another Postgres major version), and
`NODE_DRAIN_NO_COPY` (409, a drain refused because a database has no usable copy to move to).
