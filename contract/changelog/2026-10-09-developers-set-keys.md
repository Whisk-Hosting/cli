# Developers set secret values

`PUT /v1/orgs/{org}/secrets/{name}/value`, `POST …/rollback` and `DELETE /v1/orgs/{org}/secrets/{name}`
accept the developer role. Fixes that send an agent to a person (`SECRET_VALUE_NEEDS_HUMAN`,
`SECRET_UNSET`, `NEEDS_HUMAN`) now ask an owner, admin or developer. Agent tokens still cannot
set values, and `PUT …/previews` is still an owner's or admin's.
