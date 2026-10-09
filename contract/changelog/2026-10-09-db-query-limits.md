# db query result and concurrency limits

The error catalogue adds `QUERY_RESULT_TOO_LARGE` (422) for a `db query` stopped because the
database sent a row, error or notice past 8 MiB or more than 256 MiB of rows, and `QUERY_BUSY`
(429) for a `db query` or `db schema` that found no place within five seconds: two per org,
sixteen on the platform.
