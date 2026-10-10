# Manifest: `promoted`, error PROMOTED_APP_REQUIRED, the shop template

`whisk.yaml` takes `promoted` (boolean, default false): the app runs only as a Promoted app, so
its production deploy waits, `blocked` with the new code `PROMOTED_APP_REQUIRED` (409), until an
owner or billing contact promotes it. The templates gain `shop` (`whisk init --template shop`),
a Medusa shop that says `promoted: true`. Doctor leaves out W005, W090 and W099 for such an app,
W022 finds a Medusa route at `src/api/<path>/route.ts`, and W052 counts the queue endpoint only
when the app declares functions.
