# Promoted apps

An app a business sells to its own customers can be made a Promoted app. The app record carries
`promoted {since, by, in_force}`, `POST /apps/:app/validate` answers `promoted`, and the new error
`PROMOTE_UNAVAILABLE` (409) refuses promoting an app of a client business or of a business inside
its trial; `PLAN_FEATURE` names `promoted` on a plan without it. SKILL.md §4 says a Promoted app
may keep its own sign-in by listing every route as public, and doctor's W090 is skipped for one.
