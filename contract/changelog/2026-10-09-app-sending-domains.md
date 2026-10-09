# An app's own sending domains

The skill (§9, Email) teaches a white-label app to register a sending domain for each customer
with its service token, show the customer the DNS records, check until it is verified, send from
it and remove it by its id when the customer leaves.

The error catalogue gains `PLAN_LIMIT_EMAIL_DOMAINS`; `EMAIL_DOMAIN_TAKEN` names
`details.held_by` (`business` or `app`) for a domain held inside the business; `PLAN_FEATURE`
covers `app_email_domains`; `RATE_LIMITED` covers an app's hourly registrations
(`details.scope: email_domains`).

The stub serves `/v1/orgs/<org>/apps/<app>/email/domains` with the service token: records in
the provider's shape, verified when checked, listed, read and removed by id. `GET /v1/stub` names
the route as `email_domains`. The wire type `EmailDomain` gains `app` and `checked_at`.
