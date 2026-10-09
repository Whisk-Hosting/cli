# Doctor checks the common security slips

Four new doctor warnings, each with a fix the coding agent can act on:

- W096: SQL built by joining text (a template literal into `.query(`, an f-string, `fmt.Sprintf`)
  instead of sending values as parameters.
- W097: text inserted into a page as raw HTML (`innerHTML`, `dangerouslySetInnerHTML`, `| safe`,
  `template.HTML`).
- W098: with `customer_identity`, a statement that reads or changes a table of people's records
  without its owner column.
- W099: a public route that takes writes without a challenge, or a public route named for staff.

The skill's §4 names them. None fires on the templates.
