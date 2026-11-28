# Whisk skill §9: Files, email, push, PDFs and key-value

The rest of §9 of the Whisk skill (https://skill.whisk.run/), which an agent reads first. Read it when the app takes uploads, video, images or documents, sends email or push notifications, makes PDFs, or uses key-value.

**Files people send.** A request to your app carries at most the plan's `body_bytes` (8 MiB on
Starter and Team, 32 MiB on Business; a Promoted app has its plan's), and a bigger one is
refused at the edge with 413 `BODY_TOO_LARGE`. So any file a browser sends goes straight to
storage instead: images, video, documents and attachments on a form alike. `POST
/v1/orgs/<org>/apps/<app>/uploads` with the service token and `{filename, content_type,
max_bytes}` answers `{id, key, url, fields, expires_at}`: post the file to `url` as
`multipart/form-data` with every entry of `fields` first and the file last, then send your app
the upload's `id` with the rest of the form. The store refuses anything larger than `max_bytes`
or of another type, and the bytes never pass through your app. One upload may be up to 5 GB
(`max_bytes` 5368709120). `GET /v1/orgs/<org>/apps/<app>/uploads/<id>` gives you a link to read
one back. Every upload is virus-scanned: an infected one moves to `quarantine/` and reading it
answers `OBJECT_QUARANTINED`. What your app writes to storage itself with its key is its own
and is not scanned, so an app that keeps mail or other files it fetched keeps every one of them. The business's storage
allowance counts its files, its databases and its apps' git repositories alike, so images and
media committed to the repository use the allowance too; put what visitors or staff add in
storage, not in git. Files you bring yourself, such as the videos and photos of a website you are
moving to Whisk, go up with `whisk uploads add <file>... [--public] [--wait]`, which prints each
upload's id and the element that shows it; if it stops part way, run the same command again and
it carries on; `whisk uploads list` and `whisk uploads show <id>` read them back, and
`whisk uploads delete <id>` removes one you uploaded yourself (your person removes any other). Your app removes one of its own with
`DELETE /v1/orgs/<org>/apps/<app>/uploads/<id>` and `WHISK_SERVICE_TOKEN`: the file, its converted copies and its record go together. The virus scan reads files up to 100 MB: a larger one
is stored without a scan, with status `unscanned`, and your app can still use it. If a file must
be scanned before it is used, split it into pieces under 100 MB before uploading.

**A delete by mistake.** A file your app deletes with its store key, one by one or a thousand at
a time, is gone for the app at once as with any store, and kept for a week where nothing reaches
it. `whisk restore files --since 2h [--prefix uploads/]` puts back what was deleted since then,
leaving any file written again under the same name as it is. It counts against no allowance. A
removal through the uploads API above takes the upload's record too and is not kept.

**Video and audio.** Never stream video through your app or hand out the 15-minute read link
for it. Authorise the upload as above with its real `content_type` (`video/mp4`,
`video/quicktime`, `audio/mpeg`…) and `"visibility": "public"` if anyone may watch it (a
product video on a website) or leave it `private` (only people signed in to the app). Whisk
converts it for every browser and phone, makes a poster image, and serves it from the app's own
address, which never expires. Store the upload `id`; to show it, load the player and use the
element:

```html
<script src="/.whisk/player.js" defer></script>
<whisk-video media="UPLOAD_ID"></whisk-video>      <!-- or <whisk-audio media="UPLOAD_ID"> -->
```

The player picks a quality for the viewer's screen and connection and plays the original while
the conversion runs. To put a public video on another website, use the absolute script URL
(`https://<app host>/.whisk/player.js`) with the same element, or an iframe of
`https://<app host>/.whisk/media/<id>/embed`. `GET /.whisk/media/<id>` (from the page) says the
status, duration, size and sources; `PATCH /v1/orgs/<org>/apps/<app>/uploads/<id>`
`{"visibility": "public"}` changes who may watch. The largest copy Whisk makes is 1080p, so when a site already has a
1080p encode of a video, upload that rather than a 4K original: it plays the same and goes up
many times faster. Converting is a monthly allowance (minutes of
media); past it, new videos play as uploaded until the month turns.

**Images.** Never resize pictures in your app or serve the original of a photo. Authorise the
upload as above with its real `content_type` (`image/jpeg`, `image/png`, `image/heic`…), `public`
if anyone may see it (a product photo on a website) or left `private` (only people signed in to
the app), store the upload `id`, and put the image's own address in the page:

```html
<img src="/.whisk/img/UPLOAD_ID?w=800" width="800" alt="…">
<img src="/.whisk/img/UPLOAD_ID?w=96&h=96&fit=cover" width="96" height="96" alt="…">  <!-- a square crop -->
<img srcset="/.whisk/img/UPLOAD_ID?w=400 400w, /.whisk/img/UPLOAD_ID?w=800 800w, /.whisk/img/UPLOAD_ID?w=1600 1600w"
     sizes="(max-width: 600px) 100vw, 800px" src="/.whisk/img/UPLOAD_ID?w=800" alt="…">
```

`w` and `h` are 1 to 4096 pixels; it fits inside them, or with `fit=cover` fills both and crops
the edges. It is never made larger than the original. Each browser gets AVIF or WebP when it
shows them; add `format=jpeg` (or `png`, `webp`, `avif`) where something else needs one format,
such as an email or an `og:image`. Each size is made the first time it is asked for and kept, so
use a few fixed sizes rather than the exact width of every screen. The address works the moment
the upload finishes and never expires; on another website use the absolute
`https://<app host>/.whisk/img/<id>?w=800` of a public image. Errors: `IMAGE_NOT_READY` (the
upload has not finished; retry), `IMAGE_UNREADABLE` (not a picture it can read, or over 50
megapixels), `NOT_AN_IMAGE` (the upload is not `image/*`). `GET …/uploads/<id>` gives the address
as `image.path`.

**Documents and other files.** Every upload, whatever its type, opens at `/.whisk/file/<id>` on
the app's own hostname: a PDF, a spec sheet, a CAD or 3D model file, a zip. Upload it `public`
(`whisk uploads add <file> --public`, or `"visibility": "public"`) when anyone may open it, such as
a download on a website, and link the address; it never expires, needs no server code, and opens
under the file's own name on the app's own domain. Whisk sets its type from the file's name (STEP,
GLB, STL, DWG and the rest, from the open mime-db list) and compresses what compresses, so do not
set a content type or zip files yourself. When a file moved from an old address that search
engines know, name that address as canonical: `whisk uploads set <id> --canonical
https://example.com/old/spec-sheet.pdf` (or `PATCH …/uploads/<id> {"canonical": "…"}`), and the
file answers with `Link: <…>; rel="canonical"`. A `private` one opens for people signed in to the
app, or through a signed link. `GET …/uploads/<id>` gives the address as `file_path` (and
`file_url` on the app's hostname). Error: `FILE_NOT_READY` (the upload has not finished or its
scan; retry).

```html
<a href="/.whisk/file/UPLOAD_ID">Installation guide (PDF)</a>
```

**A website's own files at their own paths.** A static site's downloads, drawings, posters and
other large files belong in storage, not in the repository, and they can keep their ordinary
paths: put them in a folder laid out as the site links them and run `whisk uploads add <folder>
--public --keep-paths`, so `<folder>/posters/install.jpg` answers at `/posters/install.jpg` on
every hostname of the app wherever the app itself has nothing at that path. Link them by those
paths; there is no id to keep in the site, no manifest and no redirect list, and old links to a
moved site's files keep working. The app's own pages always win, and a path nothing names gets the
app's 404. Upload the folder again after a change: a file uploaded to a path again replaces the
one there once it is scanned. One file: `whisk uploads add <file> --public --path /specs/a.pdf`,
or `whisk uploads set <id> --path /specs/a.pdf` (`PATCH …/uploads/<id> {"path": "…"}`; `""`
removes it). `GET …/uploads/<id>` gives it as `path`.

**Signed links (an app with its own sign-in).** A `private` upload's addresses answer only
people signed in through Whisk. If the app keeps its own accounts (an app moved onto Whisk with
its own login), do not keep your own image resizing (sharp, Pillow) or stream private photos
and video through the app: have the server sign the addresses. After checking that the person
may see the file, ask with the service token, server side only, and put the answered `path` in
the page:

```ts
const { links } = await signMedia(["/.whisk/img/UPLOAD_ID?w=800", "/.whisk/media/VIDEO_ID", "/.whisk/file/DOC_ID"], 3600);
// Python: sign_media([...], expires_in=3600); Go: signMedia(ctx, []string{...}, time.Hour)
// = POST /v1/orgs/<org>/apps/<app>/uploads/links {"paths": [...], "expires_in": 3600}
```

```html
<img src="/.whisk/img/UPLOAD_ID?w=800&exp=…&kid=…&sig=…" width="800" alt="…">
<script src="/.whisk/player.js" defer></script>
<whisk-video src="/.whisk/media/VIDEO_ID?exp=…&kid=…&sig=…"></whisk-video>
```

Write each path exactly as for a signed-in page, with the size it asks for (one link per size,
up to 100 paths per call); a video's link also covers its `/embed`, `/poster.jpg` and every
quality. A link opens the file for anyone who has it until it expires, an hour unless
`expires_in` says (60 seconds to 12 hours), so sign links each time the page is drawn and never
store or email them; the same path asked for again within a quarter of its lifetime is the same
link, so browsers keep the picture cached. Change nothing in a link: another size, a later
`exp` or another app answers `403 MEDIA_LINK_INVALID`, an old one `403 MEDIA_LINK_EXPIRED`
(draw the page again). The address without its signature still needs a Whisk sign-in. If a link
leaks, `whisk uploads rotate-key --now` stops every link issued; without `--now` old links run
out on their own. `whisk uploads link <path>` prints a link to look at a private file as a
stranger would.

**Files a job keeps between runs.** Do not rebuild a directory tree in the database. Set
`storage: true` and use the template's tree helper: pull the tree into a directory under `/tmp`
at the start of a run, work on ordinary files, and push as you go and at the end. It moves only
what changed, so a tree of thousands of files syncs in seconds.

```ts
const s3 = storageClient()!;
await pullTree(s3, "transcripts", "/tmp/work");   // Go: PullTree, Python: pull_tree
// ... the job reads and writes /tmp/work ...
await pushTree(s3, "transcripts", "/tmp/work");   // push after each batch, not only at the end
```

Take a lease row in the database first (`insert ... on conflict do nothing`, with an expiry) so
two runs never work on the same tree, and release it when done. `/tmp` is memory: the pulled tree and the process
share `WHISK_MEMORY_BYTES` (256 MiB on Free, 512 MiB on Team). Keep a pulled tree under half of
it; for anything bigger, pull only the part a run needs or read and write objects by key
instead of copying the tree. Near the limit (85 %) the app's log shows `APP_MEMORY_HIGH` with
the numbers. An app over its limit is killed mid-run with no SIGTERM: `whisk logs` shows a
`whisk:` line from the platform at that moment and the next run's log starts with
`APP_OUT_OF_MEMORY`, the function runs it cut off are failed with that code, and the org's
owners are told (at most once a day per app). `whisk apps info` shows the app's memory now and its peak
against the limit; check it after a job's first real run. There is no persistent disk, by design: the app can move, sleep and wake
anywhere, and everything in storage is backed up.

**Email** (`email: true`). `POST /v1/orgs/<org>/apps/<app>/email/send` with the service token
and `{to, subject, html, text, from}`; `to` is one address or a list. A preview sends only to the
business's members: anyone else is left out and listed under `held` in the answer, so test with
your own address; production sends to everyone. Addresses are plain ASCII:
a name before one may hold any letters (`Zoë <zoe@example.com>`), but an address with accents is
refused with `INVALID_REQUEST`, since mail servers refuse it (a domain with accents goes in its
`xn--` form). Leave `from` out and the
mail goes from the business's own address on Whisk's domain, `<app>.<org>@whisk.page`, under the
app's name, with no setup; `from: "Training <training@whisk.page>"` sends as
`training.<org>@whisk.page` instead. To send from the
business's own domain, an owner or admin adds it with `whisk email domains add <domain>` (or under
Settings, Email), creates the DNS records it prints, runs `whisk email domains verify <domain>`,
and `from` names an address on it; any other domain gets `EMAIL_DOMAIN_UNVERIFIED` naming the domains that are. Set `reply_to`
when people should be able to answer. Email is for transactional mail a person asked for or
needs now, never mailings: one business may use only a share of Whisk's shared sending allowance,
and past it sends answer `EMAIL_RATE_LIMITED` until the day or month resets.
`attachments` adds files, up to 10 and 10 MB together: `{filename, content_type, content}` with
the file base64, or `{filename, storage_key}` with the full key of a file the app keeps in
storage (`WHISK_STORAGE_PREFIX` + name), which suits a generated PDF report. For a logo in the
mail, give the image a `content_id` and show it in `html` with `<img src="cid:logo">`; every
inline image must be shown and every `cid:` must have its image. Programs and scripts (`.exe`,
`.js`, `.bat`, …) are refused with `EMAIL_ATTACHMENT_BLOCKED`; the other refusals are
`EMAIL_ATTACHMENT_INVALID`, `EMAIL_ATTACHMENT_TOO_LARGE` and `EMAIL_ATTACHMENT_NOT_FOUND`, each
with a `fix`. Locally `whisk dev` checks the same rules and keeps the mail instead of sending it:
`GET <api>/v1/orgs/<org>/apps/<app>/email/sent` lists it.
When each of your app's customers wants mail from their own domain (a white-label app), register
it with the service token: `POST /v1/orgs/<org>/apps/<app>/email/domains {domain}` answers `id`,
`status: "pending"` and `dns_records` (DKIM, SPF and the return path's MX, full names). Show the
customer those records to publish, keep the `id` against the customer, then
`POST .../email/domains/<id>/verify` until `status` is `verified` (it asks at most every 30
seconds); `from` on that domain then works for your app only. `GET .../email/domains` lists them
and `DELETE .../email/domains/<id>` removes one when the customer leaves. A domain another
business or app already has is `EMAIL_DOMAIN_TAKEN`; the plan caps them
(`PLAN_LIMIT_EMAIL_DOMAINS`) and the free app has none. `whisk dev` answers these routes locally
and verifies at once.
The send answers 202 with an `id`; keep it with the record the mail is about. Send an
`Idempotency-Key` header that names the message (the record's id and what the mail is, such as
`order-1042-confirmation`): the same send under the same key within 24 hours is that message,
sent once, so a send that timed out or answered `PLATFORM_UNAVAILABLE` is safe to make again, and
a different message under a used key is `INVALID_REQUEST`. To learn what became
of the message, `GET /v1/orgs/<org>/apps/<app>/email/sends/<id>` with the service token (or
`whisk email status <id>`) answers `status` and `events`: `sent` (the provider took it, nothing
heard since), `delayed`, `delivered` (the receiving server accepted it), `failed`, `bounced`
(with the reason in `detail`) or `complained` (the recipient marked it as spam): the furthest
the message got, a complaint, bounce or failure for any recipient outranking it, so with several
recipients `delivered` can mean only some of them. Nobody can
know whether a delivered message went to the inbox or the spam folder; only a complaint is
reported, so never tell a person their mail "reached the inbox". Stop mailing an address that
bounced or complained. Delivery is kept 30 days, and an id that is not your app's or is older
answers `EMAIL_SEND_NOT_FOUND`. Under `whisk dev` a kept message stays `sent`.
Transactional only: at most 20 recipients a message and 100 a minute. The plan includes a daily
allowance; past it a paid plan keeps sending and is billed, and the free app stops (it also stops
at 50 a month). `EMAIL_RATE_LIMITED` says which limit with `details.period`.

**Push notifications** (every plan, nothing to declare). Whisk holds your app's signing key,
delivers to every device a person turned notifications on for, retries while a push service is
busy, forgets devices that are gone, and can email the person instead. Do not add `web-push`,
VAPID keys or a push service of your own.

1. On a page, `<script src="/.whisk/push.js"></script>`. `await Whisk.push.state()` is
   `unsupported`, `needs-install`, `denied`, `off` or `on`: show a "Turn on notifications"
   button for `off`, and for `needs-install` say to add the app to the Home Screen first: an
   iPhone or iPad shows notifications only from an app added to the Home Screen. Link Whisk's web
   manifest, made from the app's name and its `sign_in` colours and logo, with `<link
   rel="manifest" href="/.whisk/manifest.webmanifest">`, unless the app serves its own. From the button's tap, `await Whisk.push.enable({ register })`, where
   `register(subscription, { label })` posts to a route of your own. Call
   `Whisk.push.sync({ register })` on each page load so a renewed device reaches you, and
   `Whisk.push.disable({ unregister })` to turn it off. `enable` rejects with `e.code`
   `PUSH_DENIED`, `PUSH_NEEDS_INSTALL` or `PUSH_UNSUPPORTED`; show `e.message`.
2. That route, for the signed-in person, calls `POST /v1/orgs/<org>/apps/<app>/push/devices` with
   the service token and `{user, subscription, label}`, where `user` is your key for the person:
   `X-Whisk-User-Id` with Whisk's sign-in, or your own user id with your own. A person may have
   several devices; the same browser signed in as someone else moves to them.
   `GET …/push/devices?user=` lists them and `DELETE …/push/devices?user=&endpoint=` removes
   a person's devices, or one.
3. To notify, `POST /v1/orgs/<org>/apps/<app>/push/send` with the service token and `{to, title,
   body, url}`: `to` is one user key or up to 500, `url` a path on your app (or an https address)
   that a tap opens. Optional: `tag` (a newer notification with the same tag replaces the older
   one on the device), `icon`, `data` (handed to the page), `ttl` (seconds it stays worth
   delivering, default a day), `urgency` (`very-low`, `low`, `normal`, `high`), and
   `fallback_email`, which Whisk emails the title, body and a link when no device took it (as
   your app's email, counted against its allowance). Send an `Idempotency-Key` header, such as
   the id of what happened, so a retried send notifies once. It answers 202 `{id, devices,
   status}`; `GET …/push/sends/<id>` says what became of each device's copy (`delivered`,
   `retrying`, `gone`, `refused`, `expired`) for 7 days. A notification is at most 3.7 KB of JSON
   (`PUSH_TOO_LARGE`): send an id in `data` and fetch the rest. At most 5,000 devices a minute
   (`PUSH_RATE_LIMITED`). Notify for what the person asked to hear about, never marketing.

An app with its own service worker keeps it: add `importScripts("/.whisk/push-sw.js")` to it
and pass `{ registration: await navigator.serviceWorker.ready }` to `enable`, `sync` and
`disable`. To subscribe with code of your own, `WHISK_PUSH_KEY` is the key to subscribe with
(`applicationServerKey`), and the notification your worker receives is `{title, body, url, tag,
icon, data, send}`. Whisk's worker opens a click's `url` by focusing a window already on it,
else navigating one of the app's windows there, else opening one. To open it your own way (say,
routing in place without a reload), add your `notificationclick` listener before the
`importScripts` line and call `event.stopImmediatePropagation()` in it. Under `whisk dev` notifications are kept, not sent: `GET …/push/sent` lists
them.

**PDFs** (invoices, statements, reports). Typeset them with Typst, one static binary and no
browser: download one exact release in a build stage and copy the binary into the final image,
so an upgrade never changes a document. Not headless Chrome (`puppeteer`, `playwright`), which
takes hundreds of MB, more than a small plan's whole memory, and seconds to start. The fonts are
the app's own: keep the font files in the repository and pass `--font-path` with
`--ignore-system-fonts`, so a document looks the same in `whisk dev` and on Whisk. Read the data,
end the database transaction, then render, so a slow document never holds a connection. Give each
render a deadline shorter than the request's and let only two run at once, queueing the rest, so a
burst of reports cannot use up memory or keep `/health` from answering. A render's memory counts
against `WHISK_MEMORY_BYTES` (256 MiB on Free): render a long pack of many pages with images in a
job (§7), or in parts. Write each finished PDF to storage under `WHISK_STORAGE_PREFIX` and keep
its key: serve it from there rather than rendering it again, and email it by `storage_key`.

**Key-value** (`kv: true`). Any Redis client with `WHISK_KV_URL`. The instance is the app's own,
so no prefix is needed. Use it for counters, short caches, locks and job queues. It is not
durable: it empties when the app sleeps, so keep anything that must survive in the database.
When the cache is full it drops the keys used least recently. A job queue loses jobs that way,
so an app that runs one on the cache (BullMQ, Bull, Bee-Queue, Medusa's Redis event bus or
workflow engine, RQ, Celery, Dramatiq, arq, asynq) sets `kv_evict: false`: the cache then keeps
every key and refuses new writes until there is room. Changing `kv_evict` restarts the cache,
which empties it. Starter and above include it; the free app does not, so there the app deploys
without a cache, `WHISK_KV_URL` is unset, and the push and the app's logs say so with `W080`.
Remove `kv` or ask an owner to move to Starter. A free app that already had a cache keeps it.
