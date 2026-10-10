# PACKAGE_MALICIOUS and PACKAGE_LOOKALIKE

New deploy error `PACKAGE_MALICIOUS` (exit 6, `fault: app`): a lockfile in the commit names a
package version on the OpenSSF list of known malicious packages (an osv.dev advisory whose id
starts `MAL-`). Every deploy on every plan checks before the build, so nothing was installed.
`details.packages` (`package`, `ecosystem`, `version`, `path`, `advisory`, `url`) and
`details.count` name them.

New deploy warning `PACKAGE_LOOKALIKE`: a dependency's name is one slip from a popular npm, PyPI
or Go package's and is not popular itself. `details.packages` (`package`, `ecosystem`,
`version`, `path`, `like`, `why`) and `details.count` name them.

The skill's table of codes names both.
