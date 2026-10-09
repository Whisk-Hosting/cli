// Package contract embeds the public Whisk contract: the manifest and graph schemas, the
// webhook presets, the skill and the reference documents. The CLI ships it, the control plane
// validates against it, and whisk-stub runs it locally. Everything here is MIT and public.
package contract

import _ "embed"

// Version is the conventions version this copy of the contract describes.
const Version = 1

// ManifestSchema is whisk.schema.json, JSON Schema draft 2020-12.
//
//go:embed whisk.schema.json
var ManifestSchema []byte

// GraphSchema is graph.schema.json.
//
//go:embed graph.schema.json
var GraphSchema []byte

// PresetsYAML is webhook-presets.yaml.
//
//go:embed webhook-presets.yaml
var PresetsYAML []byte

// Skill is SKILL.md, the document an agent reads first.
//
//go:embed SKILL.md
var Skill string

// ErrorsDoc is errors.md, one section per error code.
//
//go:embed errors.md
var ErrorsDoc string

// DoctorRulesDoc is doctor-rules.md, one section per rule.
//
//go:embed doctor-rules.md
var DoctorRulesDoc string

// HeadersDoc is headers.md.
//
//go:embed headers.md
var HeadersDoc string

// WorkflowsDoc is workflows.md, what the workflow engine supports.
//
//go:embed workflows.md
var WorkflowsDoc string

// EnvironmentDoc is environment.md.
//
//go:embed environment.md
var EnvironmentDoc string
