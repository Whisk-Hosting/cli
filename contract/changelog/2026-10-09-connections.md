Version 1

`whisk.yaml` gains `connections` (CONTRACT.md §3.1): each names an outside system's address, a
recipe of templates built from a fixed set of references and functions, and the operations the
app calls. The app reaches each through `WHISK_CONNECTION_<NAME>_URL` and never holds the secrets
its recipe reads; a name may not appear both there and under `secrets` or `build.secrets`. The
error catalogue adds `GRANT_NEEDED`, `CONFIRMATION_NEEDED` and the `CONNECTION_*` codes the broker
answers with, and the doctor adds W032 for code that reads a connection's secret from the
environment. SKILL.md §8 says how to declare and call a connection, and environment.md lists
`WHISK_CONNECTION_<NAME>_URL`.

The broker adds `CONNECTION_REQUEST_AMBIGUOUS` (a call another server might read differently)
and `CONNECTION_BUSY` (too many calls at once for one business), and replaces any credential an
outside system echoes back with `[redacted by Whisk]`.
