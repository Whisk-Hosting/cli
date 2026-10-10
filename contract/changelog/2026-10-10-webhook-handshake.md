# Webhooks: handshake

`webhooks[].handshake: {query, method}` makes the platform answer a provider's check of the
address (Business Central's `?validationToken=`) with the value as `text/plain` and a 200. The
check needs the source's URL token when it has one and is never stored or delivered.
`webhook.Handshake` is the shared rule the platform and the stub use.
