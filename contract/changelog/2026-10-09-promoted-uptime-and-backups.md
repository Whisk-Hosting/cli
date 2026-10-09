# Promoted apps: backup points, status page and uptime alerts

The new error `PROMOTED_ONLY` (409, `details.feature`, `details.app`, `details.in_force`)
refuses setting up or verifying a status page for an app that is not a Promoted app in force
(CONTROL-PLANE.md §6.34). `RESTORE_NOT_AVAILABLE` now applies a 30-day window to a Promoted app
in force whatever its plan's `restore_days` (§6.18).
