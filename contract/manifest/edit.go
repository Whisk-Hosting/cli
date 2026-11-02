package manifest

import (
	"bytes"
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// SetAlwaysOn returns whisk.yaml with always_on set to on, changing as little of the file as it
// can: the top-level always_on line is rewritten in place, keeping its indentation and comment;
// with no such line, always_on: true is added at the end, and turning it off leaves the file as
// it is, since off is the default. The dashboard's switch commits what it returns
// (CONTRACT.md §3). It refuses a file that does not parse as a manifest, a top level written as
// a flow mapping ({...}), and an always_on it cannot rewrite on one line. Pure.
func SetAlwaysOn(src []byte, on bool) ([]byte, error) {
	if _, err := Parse(src); err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Style&yaml.FlowStyle != 0 {
		return nil, errors.New("whisk.yaml's top level is not a block mapping, so always_on cannot be changed in place")
	}
	word := map[bool]string{true: "true", false: "false"}[on]
	top := doc.Content[0].Content
	var out []byte
	for i := 0; i+1 < len(top); i += 2 {
		key, val := top[i], top[i+1]
		if key.Value != "always_on" {
			continue
		}
		if val.Kind != yaml.ScalarNode || val.Line != key.Line {
			return nil, errors.New("whisk.yaml's always_on is not a value on its own line, so it cannot be changed in place")
		}
		lines := strings.SplitAfter(string(src), "\n")
		line := lines[key.Line-1]
		end := strings.TrimRight(line, "\r\n")
		rest := line[len(end):]
		comment := ""
		if val.LineComment != "" {
			comment = " " + val.LineComment
		} else if key.LineComment != "" {
			comment = " " + key.LineComment
		}
		lines[key.Line-1] = end[:key.Column-1] + "always_on: " + word + comment + rest
		out = []byte(strings.Join(lines, ""))
		break
	}
	if out == nil {
		if !on {
			return src, nil
		}
		out = append(bytes.Clone(src), []byte(map[bool]string{true: "", false: "\n"}[len(src) == 0 || bytes.HasSuffix(src, []byte("\n"))]+"always_on: true\n")...)
	}
	m, err := Parse(out)
	if err != nil {
		return nil, err
	}
	if m.AlwaysOn != on {
		return nil, errors.New("whisk.yaml's always_on could not be changed in place")
	}
	return out, nil
}
