Version 1. `kv: true` on a plan without a cache (the free app) is no longer described as
refused: the app deploys without a cache and without `WHISK_KV_URL`, unless it already had one,
and the push and its logs carry a `W080` line with the fix. `PLAN_FEATURE` gains
`details.setting` `own_bucket` for setting or changing the org's own storage bucket off the
Business plan.
