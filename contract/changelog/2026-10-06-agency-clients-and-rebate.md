Version 1.

New error `HANDOVER_WRONG_EMAIL` (403, `details.contact`): accepting a client business an agency is
handing over needs the contact's email. Agencies get `POST /orgs/:org/clients/:client/handover`,
`GET /orgs/:org/clients/overview`, `contact` when adding a client, and `kind` on `GET
/demos/:code`. New error `PAYOUTS_UNAVAILABLE` (409): an agency asked to set up
rebate payouts while Whisk cannot pay out; the rebate stays as credit on its bill. Agencies get
`GET /orgs/:org/billing/rebates` and `POST /orgs/:org/billing/payouts`. `PLAN_FEATURE` can name
`clients`: an agency adds client businesses once its own business is on a paid plan.
