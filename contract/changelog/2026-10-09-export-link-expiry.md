# Export links carry when they stop working

`apitypes.Export` adds `url_expires_at`: the download link in `url` works until then, five
minutes after the export was read, while the archive itself is kept until `expires_at`.
SKILL.md says that `whisk export show <id>` signs a new link.
