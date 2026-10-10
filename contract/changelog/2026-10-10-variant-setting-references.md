# Manifest: a managed variant's connections may read {setting.NAME}

A variant's connection may name one of the product's or the variant's settings as
`{setting.NAME}` anywhere in its text. The platform fills in the copy's value before the
connection is checked or granted. The manifest refuses a reference to a setting neither declares.
