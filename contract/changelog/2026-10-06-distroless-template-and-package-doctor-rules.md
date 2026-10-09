Version 1

The TypeScript template runs on `gcr.io/distroless/nodejs24-debian13:nonroot`, built with
`node:24-trixie-slim`. The skill says most image findings come from a full base image and that
`migrate` stays a plain command on a shell-less image, and how to move to a fuller image. Two
doctor rules: `W062` (migrate needs a shell the final image lacks) and `W063` (serious package
findings with a fix in the local lockfiles, checked before a deploy through `POST
/v1/orgs/<org>/apps/<app>/packages/check`, or in the live image; Business plan).
