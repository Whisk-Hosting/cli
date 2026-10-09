# Errors: PACKAGE_CHECK_OFF

A lockfile check before a deploy (`whisk doctor`, `POST …/packages/check`) on a platform that sends
no lockfile to osv.dev answers `PACKAGE_CHECK_OFF` (409). Whisk On-Premise does not send them
unless its operator turns it on (`WHISK_OSV=on`); the live app's scan still reads its image.
`whisk doctor` reports it as a skipped check.
