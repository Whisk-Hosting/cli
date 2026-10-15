package whisk

// ReleasePublicKey is the Ed25519 public key, base64, that `make cli-dist` signs SHA256SUMS
// with (the private half lives at WHISK_RELEASE_KEY; `make release-key` generates the pair and
// prints this value). Paste it here, or set it at build time with
// -ldflags "-X github.com/whisk-run/cli/whisk.ReleasePublicKey=...". Empty means a build that
// verifies checksums only, and `whisk update` says so.
var ReleasePublicKey = "59pxX6VivPH9gB4VQBgGzziObWwJggAKb8m5XReH5yc="
