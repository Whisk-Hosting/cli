# Inbound email

- **Manifest.** `inbox: {handler, allow_from}` declares that the app receives email at its own
  address (CONTRACT.md §7, "Inbound email"). A webhook may not be named `inbox` while it is
  declared, and the handler may not be the queue endpoint. Doctor's W050 and W053 cover the inbox
  handler as they cover webhook handlers.
- **Delivery.** Each message reaches the handler as a webhook delivery of source `inbox`, with a
  JSON body (`inbound.Delivery`): sender, recipients, subject, text and html (1 MB each), the
  sender checks, and the upload id and key of the stored original and each attachment.
- **Package `inbound`.** The address, routing, `allow_from`, the limits and the delivery are pure
  functions, with a lenient MIME reader (`inbound.Parse`) the stub and the platform share.
- **Plan.** Team and above include an inbox (`inbound.PlanFeature`). On Free and Starter every
  message is dropped as `not_on_plan`, `GET …/inbox` answers `included: false`, and W080 lists
  `inbox`.
- **Errors.** `INBOX_NOT_DECLARED`, `INBOX_DOMAIN_TAKEN`, `INBOX_UNAVAILABLE`,
  `INBOX_MESSAGE_DROPPED` and `DEV_NOT_RUNNING`.
- **Stub.** `POST /v1/stub/inbox` takes a raw message and delivers it as the platform does,
  keeping the files in memory behind `GET /v1/orgs/<org>/apps/<app>/uploads/<id>`.
- **Skill and templates.** SKILL.md §8 teaches the inbox; the canaries declare one at
  `/inbound/email` and `canary.test` sends a message through the stub.
