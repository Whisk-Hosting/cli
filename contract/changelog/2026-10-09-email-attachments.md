# Email attachments

The email send body takes `attachments`, a list of `{filename, content_type, content,
storage_key, content_id}` (`apitypes.EmailAttachment`): at most 10 files holding at most 10 MB
together decoded, each the file base64 as `content` or the full key of an object under the app's
`WHISK_STORAGE_PREFIX` as `storage_key`. `content_type` left out follows the extension. A
`content_id` makes an image inline; the HTML shows it with `<img src="cid:<content_id>">`, and
every `cid:` in the HTML must name one.

New package `mailattach` holds the rules (`Plan`, `Check`, `MaxFiles`, `MaxBytes`,
`MaxRequestBytes`, `Blocked`, `BlockedTypes`, `ContentTypeFor`), shared by the platform and the
stub.

New error codes: `EMAIL_ATTACHMENT_INVALID` (400), `EMAIL_ATTACHMENT_TOO_LARGE` (413),
`EMAIL_ATTACHMENT_BLOCKED` (422) and `EMAIL_ATTACHMENT_NOT_FOUND` (404).

The stub serves `POST …/email/send`, checking the message and its attachments as the platform
does and keeping it instead of sending it, and `GET …/email/sent`, which lists what it kept.

The skill's email paragraph describes attachments and inline images.
