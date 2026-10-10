# Manifest: a product declares the notices its copies may send (`managed.notify`)

A product's `managed:` block may list `notify`, each entry a `code` in the error-code shape and a
one-line `subject` of at most 120 characters. A copy of the product tells its business's owners
about a declared code by posting `{code, occurrence, detail?}` to
`http://connect.internal.whisk:8443/.whisk/notify` with its service token; Whisk emails the
owners once per occurrence. A code declared twice, a code of another shape and a subject over
one line are `MANIFEST_INVALID`; `notify` outside `managed:` is `MANIFEST_UNKNOWN_KEY`.

New error codes for the route's refusals: `NOTIFY_NOT_MANAGED`, `NOTIFY_NOT_DECLARED`,
`NOTIFY_INVALID` and `NOTIFY_LIMIT`. A paused copy's notice is `APP_PAUSED`.
