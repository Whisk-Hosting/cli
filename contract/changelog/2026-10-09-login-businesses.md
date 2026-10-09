# Logins cover chosen businesses, on the wire

- `DeviceCodeRequest` carries optional `org` and `app`, what `whisk login` is working on.
- `DeviceInfo` carries `suggested {scope, org_id?, app?}`, the page's starting choice
  (`DeviceScope`: `app`, `org`, `pick`, `new`).
- `DeviceToken` carries `orgs` for a whole-account login: the businesses it covers.
- New `Login`, `LoginBusiness`, `LoginBusinessRequest` and `LoginScope` for `GET /me/logins/:id`
  and `POST`/`DELETE /me/logins/:id/businesses`.
- New error code `BUSINESS_NOT_IN_LOGIN` (403, api and git), with `details.url`. The CLI prints it
  as `NEEDS_HUMAN` with `details.reason`.
