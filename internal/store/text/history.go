package text

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
	"movement.local/movement/internal/app"
	"movement.local/movement/internal/contract"
	"movement.local/movement/internal/domain"
)

const MaxRecordBytes = 1024 * 1024

func encodeEvent(ev domain.Event) ([]byte, error) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err = yaml.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	// Block mappings are readable, while strings/timestamps remain explicitly strings.
	var style func(*yaml.Node)
	style = func(n *yaml.Node) {
		n.Style = 0
		if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
			n.Style = yaml.DoubleQuotedStyle
		}
		for _, c := range n.Content {
			style(c)
		}
	}
	style(&node)
	var b bytes.Buffer
	b.WriteString("---\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err = enc.Encode(&node); err != nil {
		return nil, err
	}
	if err = enc.Close(); err != nil {
		return nil, err
	}
	b.WriteString("---\n\n")
	b.WriteString(ev.Summary)
	b.WriteByte('\n')
	if b.Len() > MaxRecordBytes {
		return nil, domain.Err("invalid_input", "event exceeds 1 MiB")
	}
	return b.Bytes(), nil
}
func parseEvent(b []byte) (domain.Event, error) {
	var ev domain.Event
	if len(b) > MaxRecordBytes || !bytes.HasPrefix(b, []byte("---\n")) {
		return ev, fmt.Errorf("invalid history frontmatter")
	}
	end := bytes.Index(b[4:], []byte("\n---\n"))
	if end < 0 {
		return ev, fmt.Errorf("missing history delimiter")
	}
	front := b[4 : 4+end]
	body := strings.TrimSpace(string(b[4+end+5:]))
	dec := yaml.NewDecoder(bytes.NewReader(front))
	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		return ev, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return ev, fmt.Errorf("multiple YAML documents")
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return ev, fmt.Errorf("frontmatter must be a mapping")
	}
	var walk func(*yaml.Node, int) error
	walk = func(n *yaml.Node, depth int) error {
		if depth > 32 {
			return fmt.Errorf("YAML nesting exceeds limit")
		}
		if n.Kind == yaml.AliasNode || n.Anchor != "" {
			return fmt.Errorf("YAML aliases/anchors forbidden")
		}
		switch n.Tag {
		case "!!map", "!!seq", "!!str", "!!int", "!!bool", "!!null", "!!float":
		default:
			return fmt.Errorf("unsupported YAML tag %s", n.Tag)
		}
		if n.Kind == yaml.MappingNode {
			seen := map[string]bool{}
			for i := 0; i < len(n.Content); i += 2 {
				k := n.Content[i]
				if k.Kind != yaml.ScalarNode || k.Tag != "!!str" || seen[k.Value] {
					return fmt.Errorf("duplicate or non-string YAML key")
				}
				seen[k.Value] = true
			}
		}
		for _, c := range n.Content {
			if err := walk(c, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root.Content[0], 0); err != nil {
		return ev, err
	}
	var value any
	if err := root.Decode(&value); err != nil {
		return ev, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ev, err
	}
	if err = checkJSONSchema(raw, "history", MaxRecordBytes); err != nil {
		return ev, err
	}
	if err = app.Decode(raw, &ev); err != nil {
		return ev, err
	}
	if ev.SchemaVersion != 1 {
		return ev, domain.Err("unsupported_schema", "unsupported history schema")
	}
	if err = contract.ValidateEvent(raw); err != nil {
		return ev, fmt.Errorf("history schema: %w", err)
	}
	if strings.TrimSpace(ev.Summary) != body {
		return ev, fmt.Errorf("history body does not match recorded summary")
	}
	return ev, nil
}

// Check the version before decoding into today's typed model: future records
// may legitimately have additional fields. Duplicate keys and bounds still fail.
func checkJSONSchema(raw []byte, kind string, limit int) error {
	var fields map[string]json.RawMessage
	if err := app.DecodeWithLimit(raw, &fields, limit); err != nil {
		return err
	}
	var version int
	if err := json.Unmarshal(fields["schema_version"], &version); err != nil {
		return fmt.Errorf("invalid %s schema version", kind)
	}
	if version != 1 {
		return domain.Err("unsupported_schema", "unsupported "+kind+" schema")
	}
	return nil
}
