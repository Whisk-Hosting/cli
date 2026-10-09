# `BUCKET_UNREACHABLE`, and one guarded client for tenant addresses

The error catalogue adds `BUCKET_UNREACHABLE` (502, api and deploy) for a bucket a business
brought that did not answer: refused, reset, timed out, not resolving, or resolving to an address
off the public internet all read the same and name no address. `LOG_DESTINATION_FAILED` says
"did not answer" for a destination that gave no answer, whatever the reason.
`contract/run/egress` gains `Policy.DialContext`, which answers a refused connection without the
`net.OpError` that would name the address, and `Resolve`, which turns the platform's own host:port
settings into a policy's `Allow` at start.
