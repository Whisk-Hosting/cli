# Manifest: managed, routes.apps and egress; environment: WHISK_PAUSED and WHISK_LINKED_*

- `managed:` declares a product: its slug, name, the settings a business may set, the roles it
  links to and its variants (each with env and connections).
- `routes.apps` lists routes only linked apps may call; the public listener refuses them.
- `egress: closed` keeps outbound traffic to the hosts the app's connections name.
- `WHISK_PAUSED`, `WHISK_LINKED_<ROLE>` and `WHISK_LINKED_<PRODUCT>` are new variables.
- Errors: APP_MANAGED, APP_PAUSED, MANAGED_UNAVAILABLE, MANAGED_NO_RELEASE, MANAGED_LINK_TAKEN,
  MANAGED_SOURCE_IN_USE.
- A variant may list `settings` of its own, which a business may set only on a copy of that
  variant.
