Version 1

The storage key an app receives reaches its business's own bucket and nothing else
(CONTRACT.md §5); it is no longer described as scoped to the app's prefix. `CSRF_REJECTED`
gains the `api` surface: a state-changing API request with the Whisk session cookie from any page
but the dashboard.
