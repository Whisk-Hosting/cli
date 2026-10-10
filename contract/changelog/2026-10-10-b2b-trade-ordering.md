# Manifest: b2b, and the error B2B_UNAVAILABLE

`b2b: true` asks for trade ordering, the shop template's private plugin, which Whisk gives only to
Promoted apps; the manifest accepts it only beside `promoted: true`. `B2B_UNAVAILABLE` (409) is
the build's refusal when Whisk will not give it: the app is not a Promoted app in force and the
deploy is a preview (`promotion`), the build has no Dockerfile (`dockerfile`), or this Whisk does
not carry it (`not_carried`).
