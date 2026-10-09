# Signed media links

An app with its own sign-in can show private images and video through `/.whisk/img` and
`/.whisk/media` to people who have no Whisk session. Its server asks for signed, expiring links
with the service token:

- `POST /v1/orgs/<org>/apps/<app>/uploads/links {paths, expires_in}` answers `{expires_at,
  links: [{id, path, url}]}`, one per path in order. Paths are written as the page writes them;
  `expires_in` is 60 to 43200 seconds, 3600 when left out; up to 100 paths per call.
- `POST /v1/orgs/<org>/apps/<app>/uploads/links/rotate {immediately}` gives the app a new key.
- New package `medialink`: the link's format (`exp`, `kid`, `sig`), what it covers, and the
  constant-time check, shared by the platform and the stub.
- New error codes `MEDIA_LINK_INVALID` and `MEDIA_LINK_EXPIRED` (403).
- New wire types `MediaLinks`, `MediaLink` and `MediaLinkKey`; `/uploads/links` is a no-replay
  route.
- Templates: `signMedia` (Python `sign_media`) in every `whisk` module, with tests.
- The stub signs links under a key of the run's own and its edge judges `/.whisk/img/` and
  `/.whisk/media/` as the platform does; an image it lets through is a grey picture of the size
  asked for.
- The skill's §9 gains "Signed links".
