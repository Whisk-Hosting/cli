package endpoints

import (
	"encoding"
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Wire is a set of routes written as JSON Schema, field by field: each route's input and answer,
// and every named type they use once, under $defs. A golden copy of it is what pins the wire
// format, so a field renamed, retyped, dropped or made optional fails a test however the change
// was made (TestWireIsCurrent). Pure.
func Wire(specs []Spec) map[string]any {
	defs := map[string]any{}
	routes := map[string]any{}
	for _, s := range specs {
		in, out := s.Wire()
		route := map[string]any{"answer": schemaOf(out, defs)}
		switch {
		case in == reflect.TypeFor[None]():
		case len(QueryNames(in)) > 0:
			route["query"] = querySchema(in)
		default:
			route["body"] = schemaOf(in, defs)
		}
		routes[s.Pattern()] = route
	}
	return map[string]any{"routes": routes, "$defs": defs}
}

var (
	marshaler     = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
	pkgPath       = regexp.MustCompile(`[\w.-]+(/[\w.-]+)+\.`)
)

// typeName is a named type's name with its package's name: apitypes.Page[apitypes.App].
func typeName(t reflect.Type) string {
	return pkgPath.ReplaceAllStringFunc(t.PkgPath()+"."+t.Name(), func(s string) string {
		return s[strings.LastIndex(s[:len(s)-1], "/")+1:]
	})
}

// schemaOf writes a type as JSON Schema, putting each named struct in defs once.
func schemaOf(t reflect.Type, defs map[string]any) map[string]any {
	switch {
	case t == reflect.TypeFor[None]():
		return map[string]any{"type": "null"}
	case t == reflect.TypeFor[time.Time]():
		return map[string]any{"type": "string", "format": "date-time"}
	case t == reflect.TypeFor[json.RawMessage]():
		return map[string]any{}
	case t.Kind() != reflect.Pointer && t.Kind() != reflect.Interface &&
		(t.Implements(marshaler) || reflect.PointerTo(t).Implements(marshaler) || t.Implements(textMarshaler) || reflect.PointerTo(t).Implements(textMarshaler)):
		// It writes itself; its name is what pins it.
		return map[string]any{"x-marshals": typeName(t)}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return schemaOf(t.Elem(), defs)
	case reflect.Bool:
		return named(t, map[string]any{"type": "boolean"})
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return named(t, map[string]any{"type": "integer"})
	case reflect.Float32, reflect.Float64:
		return named(t, map[string]any{"type": "number"})
	case reflect.String:
		return named(t, map[string]any{"type": "string"})
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "contentEncoding": "base64"}
		}
		return map[string]any{"type": "array", "items": schemaOf(t.Elem(), defs)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaOf(t.Elem(), defs)}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Invalid, reflect.Uintptr, reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return map[string]any{"x-unsupported": t.String()}
	case reflect.Struct:
		if t.Name() == "" {
			return structSchema(t, defs)
		}
		name := typeName(t)
		if _, ok := defs[name]; !ok {
			defs[name] = map[string]any{} // a type that refers to itself stops here
			defs[name] = structSchema(t, defs)
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	}
	return map[string]any{"x-unsupported": t.String()}
}

// named adds a named basic type's name, so an enum's type is pinned too.
func named(t reflect.Type, s map[string]any) map[string]any {
	if t.Name() != "" && t.PkgPath() != "" {
		s["x-type"] = typeName(t)
	}
	return s
}

// structSchema writes a struct's JSON fields as encoding/json writes them: by tag name, embedded
// structs' fields in their place, "-" and unexported fields left out, omitempty optional.
func structSchema(t reflect.Type, defs map[string]any) map[string]any {
	props := map[string]any{}
	var required []string
	for i := range t.NumField() {
		fieldProps, fieldRequired := fieldSchema(t.Field(i), defs)
		for k, v := range fieldProps {
			props[k] = v
		}
		required = append(required, fieldRequired...)
	}
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		slices.Sort(required)
		out["required"] = slices.Compact(required)
	}
	return out
}

// fieldSchema is one struct field's properties, with those required: none for a field JSON
// leaves out, an embedded struct's own fields for one it flattens.
func fieldSchema(f reflect.StructField, defs map[string]any) (map[string]any, []string) {
	tag, hasTag := f.Tag.Lookup("json")
	name, opts, _ := strings.Cut(tag, ",")
	if name == "-" && opts == "" {
		return nil, nil
	}
	if ft := derefType(f.Type); f.Anonymous && !hasTag && ft.Kind() == reflect.Struct {
		embedded := structSchema(ft, defs)
		required, _ := embedded["required"].([]string)
		return embedded["properties"].(map[string]any), required
	}
	if !f.IsExported() {
		return nil, nil
	}
	if name == "" {
		name = f.Name
	}
	s := schemaOf(f.Type, defs)
	if strings.Contains(opts, "string") {
		s = map[string]any{"type": "string", "x-encodes": s}
	}
	if strings.Contains(opts, "omitempty") || strings.Contains(opts, "omitzero") {
		return map[string]any{name: s}, nil
	}
	return map[string]any{name: s}, []string{name}
}

// derefType is a type without its pointer.
func derefType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}
	return t
}

// querySchema writes a query input's parameters.
func querySchema(t reflect.Type) map[string]any {
	props := map[string]any{}
	for i := range t.NumField() {
		f := t.Field(i)
		if name, ok := f.Tag.Lookup("query"); ok && name != "-" {
			props[name] = schemaOf(f.Type, map[string]any{})
		}
	}
	return props
}
