Version 1

Platform pages (`contract/platformpage`) can heal themselves: a page with `Retry` set has a
Retry button and, when `After` is set, reloads itself once after that many seconds. It
remembers the reload in the tab's session storage for five minutes, or in a `__whisk_reload`
query parameter where storage is blocked, so it never reloads twice. A waking page's Retry line
starts hidden and is shown by `whiskHeal(0)` once its reload has not helped. `RATE_LIMITED`
from the edge is a page, not JSON, for a browser's page load, and counts a signed-in person by
who they are rather than by address.
