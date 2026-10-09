Version 1

`NOT_FOUND` on the edge also answers WordPress, PHP, secrets and version control paths, which
nothing on Whisk serves, with `details.kind: path`. The edge answers them before any app, so an app never sees
a scanner's probes. New error `ADDRESS_BLOCKED` (403, edge): the edge refuses, for an hour, an
address that asked for one of those paths.
