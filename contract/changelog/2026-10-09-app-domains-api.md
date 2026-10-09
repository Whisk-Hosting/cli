An app manages its customers' custom domains with its service token: `GET`/`POST
/v1/orgs/<org>/apps/<app>/domains`, `POST .../domains/<id>/verify` and `DELETE
.../domains/<id>` from production, removing only the domains it added. A Domain carries `status`
and `records`, the DNS records to show the customer, and `added_by`. New codes
`PLAN_LIMIT_DOMAINS` and `DOMAIN_ADDED_BY_TEAM`; `RATE_LIMITED` gains the scope `app_domains`.
Each template's `whisk` module has a `domains` helper and a `PlatformError`, and the stub serves
the routes (names under `.test` or `.example` verify at once). The skill explains it under
"Their customers' domains".
