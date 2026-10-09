Version 1

`whisk.yaml` adds `redirects` (a list of `from`, `to`, `status`, `query`) and `redirects_file`
(more rules, one a line), defined in CONTRACT.md §3.1 and implemented once in
`contract/redirects`. Doctor adds W110 (the file or the rules together are invalid; also run by
the push) and W111 (a chain). The Domain object adds `redirect_to`. SKILL.md §3 says when and
how to use them.
