# SECRET_IN_IMAGE and SOURCE_MAPS_PUBLISHED

New deploy error `SECRET_IN_IMAGE` (exit 6): the built image or a static folder holds the value
of one of the app's secrets. `details.secrets` and `details.files` (`{secret, path, browser}`)
name them; the value is never shown.

New deploy warning `SOURCE_MAPS_PUBLISHED`: the app sends source maps to browsers.
`details.count` and `details.files` name them. A deploy's `warnings` now hold what needs fixing
without stopping the deploy, from the build as well as after the switch.
