# Whisk plugin for ChatGPT and Codex

The package Whisk submits to OpenAI's plugin directory, which ChatGPT and Codex share
(CONTROL-PLANE.md §4.7 "In OpenAI's plugin directory"). It is in the Agent Plugins format:

```
plugin.json                      identity, listing text and review cases (extensions.com.openai)
mcp.json                         the connector at https://api.whisk.run/mcp, OAuth by CIMD
skills/build-on-whisk/SKILL.md   how to build, check and change apps with the connector's tools
assets/                          logo (light and dark) and composer icon
```

Build the ZIP from this folder, with the files at its root:

```
cd contract/plugin && zip -r ../../whisk-plugin.zip plugin.json mcp.json skills assets
```

`go test ./contract/...` checks the package against the directory's limits (lengths, counts,
icons, referenced files). Changes to the connector's tools need no new ZIP: OpenAI scans the
server daily. A change to anything in this folder is a new version: raise `version` and upload.

Reviewer credentials, the demo recording and the domain verification token never go in the
package. The token is the live setting `openai_apps_challenge`, which the API shows at
`/.well-known/openai-apps-challenge`.
