New operator route `PUT /operator/email/allowance {daily_limit, monthly_limit}`: the limits of the
platform's Resend plan, 0 for none. `GET /operator/email` answers `allowance`: `daily` and
`monthly` (`used`, `limit`, `resets_at`), `refused_today`, `checked_at`, `set`, `set_by` and
`set_at`. No new error codes.
