package doctor

import (
	"bytes"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAML edits that keep the human's comments and layout: the manifest is a document people
// read, so fixes change one node and marshal the tree back.

// yamlLine returns the line of the node at a JSON pointer ("/functions/0/cron"), or 0.
func yamlLine(root *yaml.Node, pointer string) int {
	if root == nil {
		return 0
	}
	node := root
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	for _, seg := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		if seg == "" {
			continue
		}
		switch node.Kind {
		case yaml.MappingNode:
			next := mappingValue(node, seg)
			if next == nil {
				return node.Line
			}
			node = next
		case yaml.SequenceNode:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(node.Content) {
				return node.Line
			}
			node = node.Content[i]
		default:
			return node.Line
		}
	}
	return node.Line
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mappingSet(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

// strNode is a string scalar. The tag makes the encoder quote a value YAML would otherwise read
// as something else, such as NULL, TRUE, 123 or an empty value.
func strNode(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

func parseDoc(src []byte) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	return &doc, doc.Content[0], nil
}

func marshalDoc(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// manifestAddSecret appends a name to secrets, creating the list when absent.
func manifestAddSecret(src []byte, name string) ([]byte, error) {
	doc, root, err := parseDoc(src)
	if err != nil {
		return nil, err
	}
	list := mappingValue(root, "secrets")
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		mappingSet(root, "secrets", list)
	}
	for _, n := range list.Content {
		if n.Value == name {
			return src, nil
		}
	}
	list.Content = append(list.Content, strNode(name))
	return marshalDoc(doc)
}

// manifestRemovePublic drops exact entries from routes.public.
func manifestRemovePublic(src []byte, entries []string) ([]byte, error) {
	doc, root, err := parseDoc(src)
	if err != nil {
		return nil, err
	}
	routes := mappingValue(root, "routes")
	if routes == nil {
		return src, nil
	}
	list := mappingValue(routes, "public")
	if list == nil || list.Kind != yaml.SequenceNode {
		return src, nil
	}
	drop := map[string]bool{}
	for _, e := range entries {
		drop[e] = true
	}
	kept := list.Content[:0]
	for _, n := range list.Content {
		if !drop[n.Value] {
			kept = append(kept, n)
		}
	}
	list.Content = kept
	return marshalDoc(doc)
}

// manifestSetHealthPath sets health.path, creating the mapping when absent.
func manifestSetHealthPath(src []byte, path string) ([]byte, error) {
	doc, root, err := parseDoc(src)
	if err != nil {
		return nil, err
	}
	health := mappingValue(root, "health")
	if health == nil || health.Kind != yaml.MappingNode {
		health = &yaml.Node{Kind: yaml.MappingNode}
		mappingSet(root, "health", health)
	}
	mappingSet(health, "path", strNode(path))
	return marshalDoc(doc)
}
