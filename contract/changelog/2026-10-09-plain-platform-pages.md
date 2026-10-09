# Plain platform pages

`platformpage.Page` has `Plain`: the page in a neutral design with the app's name at the top in
place of the wordmark, no footer, and `PlainReloadFlag` (`__reload`) for its one reload where
session storage is blocked. `platformpage.PlainLines(message, status)` gives such a page its
lines: the error's message unless it names Whisk, and one sentence for the status in place of
the developer's fix. The edge draws a Promoted app's pages this way (CADDY.md §5.1).
