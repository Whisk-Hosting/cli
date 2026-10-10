# Doctor W104: the ESM banner's import name

W104's esbuild example imports `createRequire` as `whiskCreateRequire`, as the TypeScript
template does, so the banner cannot clash with a bundled library (Sentry 11) that imports
`createRequire` under its own name.
