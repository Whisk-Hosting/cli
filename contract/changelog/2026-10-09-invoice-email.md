# Settings carry the business's invoice email

`GET /orgs/:org/settings` returns `invoice_email` when one is set, and `PUT /orgs/:org/settings`
takes `invoice_email`: one address, stored lowercased, or an empty string to clear it. Anything
else is `INVALID_REQUEST`. Receipts are emailed to that address instead of the owners.
