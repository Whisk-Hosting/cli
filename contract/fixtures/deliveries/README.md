# A signed delivery

One webhook delivery as the platform sends it (CONTRACT.md §7), for a handler's test in any
language. `key.b64` is the app's `WHISK_DELIVERY_KEY`; `headers.json` the platform's delivery
headers; `body` the raw bytes, no trailing newline; `signature` the expected
`X-Whisk-Delivery-Signature`.

A verifier computes HMAC-SHA256 under the decoded key over
`<X-Whisk-Webhook-Id> "\n" <X-Whisk-Webhook-Received-At> "\n" <body>`, hex-encodes it,
prefixes `v1=` and compares in constant time. Flipping any byte of the body, either header or
the key must fail.
