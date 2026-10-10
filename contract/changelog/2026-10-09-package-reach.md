# Package findings say whether the app's code calls them

`PackageFinding` gains `reach` (`PackageReach`: `called`, `not_called` or `unknown`), whether
call analysis found the app's code calling the vulnerable part, with
`PackageReach.NotCalled()` deciding what is set aside: only `not_called`, never `unknown` or an
empty value. `PackageCounts` gains `not_called`, and its severity totals, `fixable` and
`attention` now count only the findings the app's code may call. The skill says a
`not_called` finding is listed apart and is not one to fix now, and doctor rule `W063` leaves
it out.
