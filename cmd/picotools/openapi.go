package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// openAPIExamples reads an OpenAPI specification (YAML or JSON) and gives one result per
// operation, named "GET /path  summary", whose items are the request body and the responses
// with their JSON. Each JSON comes from
// the example written in the specification or, when there is none, is built from the schema.
func openAPIExamples(text string) ([]Result, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("cannot read the specification: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, errors.New("paste an OpenAPI specification")
	}
	spec := &openAPI{root: doc.Content[0]}
	paths := get(spec.root, "paths")
	if paths == nil || paths.Kind != yaml.MappingNode {
		return nil, errors.New("no paths found: paste an OpenAPI specification")
	}

	var results []Result
	for i := 0; i+1 < len(paths.Content); i += 2 {
		path, item := paths.Content[i].Value, spec.resolve(paths.Content[i+1])
		shared := get(item, "parameters")
		for j := 0; j+1 < len(item.Content); j += 2 {
			method := strings.ToUpper(item.Content[j].Value)
			if !httpMethods[method] {
				continue
			}
			op := item.Content[j+1]
			summary := scalar(get(op, "summary"))
			info, items := spec.describe(op, shared)
			results = append(results, Result{
				Name:  fmt.Sprintf("%-6s %s\t%s", method, path, summary), // lined up below
				Value: info,
				Items: items,
			})
		}
	}
	if len(results) == 0 {
		return nil, errors.New("no operations found in the paths")
	}
	// Line up the summaries after the longest path.
	width := 0
	for _, r := range results {
		width = max(width, len(strings.SplitN(r.Name, "\t", 2)[0]))
	}
	for i, r := range results {
		parts := strings.SplitN(r.Name, "\t", 2)
		results[i].Name = fmt.Sprintf("%-*s   %s", width, parts[0], parts[1])
	}
	return results, nil
}

var httpMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true}

type openAPI struct{ root *yaml.Node }

// describe gives what is shown for one operation: Value holds the parameters, shown above
// the list, and Items the request body and each response with its JSON.
func (s *openAPI) describe(op, shared *yaml.Node) (string, []Result) {
	var params []string
	for _, list := range []*yaml.Node{shared, get(op, "parameters")} {
		if list == nil {
			continue
		}
		for _, p := range list.Content {
			p = s.resolve(p)
			desc := scalar(get(p, "name")) + " (" + scalar(get(p, "in"))
			if t := s.typeOf(get(p, "schema")); t != "" {
				desc += ", " + t
			}
			if scalar(get(p, "required")) == "true" {
				desc += ", required"
			}
			params = append(params, desc+")")
		}
	}
	info := "Parameters: none"
	if len(params) > 0 {
		info = "Parameters: " + strings.Join(params, ", ")
	}

	var items []Result
	if body := get(op, "requestBody"); body != nil {
		items = append(items, Result{Name: "Request body", Value: s.contentExample(s.resolve(body))})
	}
	if responses := get(op, "responses"); responses != nil {
		for i := 0; i+1 < len(responses.Content); i += 2 {
			code, resp := responses.Content[i].Value, s.resolve(responses.Content[i+1])
			name := "Response " + code
			if d := scalar(get(resp, "description")); d != "" {
				name += " — " + d
			}
			items = append(items, Result{Name: name, Value: s.contentExample(resp)})
		}
	}
	return info, items
}

// contentExample gives the JSON of a request body or response: its example if written,
// otherwise one built from its schema. JSON media types are preferred.
func (s *openAPI) contentExample(n *yaml.Node) string {
	content := get(n, "content")
	if content == nil || len(content.Content) < 2 {
		return "// (no content)"
	}
	media := content.Content[1]
	for i := 0; i+1 < len(content.Content); i += 2 {
		if strings.Contains(content.Content[i].Value, "json") {
			media = content.Content[i+1]
			break
		}
	}
	if ex := get(media, "example"); ex != nil {
		return toJSON(ex, 0)
	}
	if exs := get(media, "examples"); exs != nil && len(exs.Content) >= 2 {
		if v := get(s.resolve(exs.Content[1]), "value"); v != nil {
			return toJSON(v, 0)
		}
	}
	if schema := get(media, "schema"); schema != nil {
		return toJSON(s.fromSchema(schema, nil), 0)
	}
	return "// (no example and no schema)"
}

// ---- Example from a schema ----

// fromSchema builds an example value as a YAML node, so it is written like a written example.
// seen holds the schemas being built, to stop at a schema that contains itself.
func (s *openAPI) fromSchema(n *yaml.Node, seen []string) *yaml.Node {
	if n == nil {
		return str("string")
	}
	if ref := scalar(get(n, "$ref")); ref != "" {
		for _, r := range seen {
			if r == ref {
				return str("(" + ref[strings.LastIndex(ref, "/")+1:] + ", again)")
			}
		}
		return s.fromSchema(s.resolve(n), append(seen, ref))
	}
	if ex := get(n, "example"); ex != nil {
		return ex
	}
	if c := get(n, "const"); c != nil {
		return c
	}
	if e := get(n, "enum"); e != nil && len(e.Content) > 0 {
		return e.Content[0]
	}
	if all := get(n, "allOf"); all != nil {
		merged := &yaml.Node{Kind: yaml.MappingNode}
		var last *yaml.Node
		for _, part := range all.Content {
			last = s.fromSchema(part, seen)
			if last.Kind == yaml.MappingNode {
				merged.Content = append(merged.Content, last.Content...)
			}
		}
		if len(merged.Content) == 0 && last != nil {
			return last
		}
		return merged
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		if opts := get(n, key); opts != nil {
			for _, o := range opts.Content {
				if s.typeOf(o) != "null" {
					return s.fromSchema(o, seen)
				}
			}
		}
	}

	switch s.typeOf(n) {
	case "object":
		obj := &yaml.Node{Kind: yaml.MappingNode}
		if props := get(n, "properties"); props != nil {
			for i := 0; i+1 < len(props.Content); i += 2 {
				obj.Content = append(obj.Content, str(props.Content[i].Value), s.fromSchema(props.Content[i+1], seen))
			}
		} else if add := get(n, "additionalProperties"); add != nil && add.Kind == yaml.MappingNode {
			obj.Content = append(obj.Content, str("key"), s.fromSchema(add, seen))
		}
		return obj
	case "array":
		return &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{s.fromSchema(get(n, "items"), seen)}}
	case "integer", "number":
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "0"}
	case "boolean":
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
	case "null":
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	if f := scalar(get(n, "format")); f != "" {
		return str(f) // e.g. "date", "date-time", "email"
	}
	return str("string")
}

// typeOf is the schema type; OpenAPI 3.1 allows a list like [string, "null"]: the first
// non-null one is taken. Without a type, properties mean an object and items an array.
func (s *openAPI) typeOf(n *yaml.Node) string {
	n = s.resolve(n)
	t := get(n, "type")
	switch {
	case t != nil && t.Kind == yaml.SequenceNode:
		for _, x := range t.Content {
			if x.Value != "null" {
				return x.Value
			}
		}
		return "null"
	case t != nil:
		return t.Value
	case get(n, "properties") != nil:
		return "object"
	case get(n, "items") != nil:
		return "array"
	}
	return ""
}

// ---- YAML helpers ----

// resolve follows a local "$ref" such as "#/components/schemas/User".
func (s *openAPI) resolve(n *yaml.Node) *yaml.Node {
	for range 20 { // a ref can point to another ref
		ref := scalar(get(n, "$ref"))
		if !strings.HasPrefix(ref, "#/") {
			return n
		}
		target := s.root
		for _, part := range strings.Split(ref[2:], "/") {
			part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
			target = get(target, part)
		}
		if target == nil {
			return n
		}
		n = target
	}
	return n
}

// get returns the value of a key in a mapping, or nil.
func get(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func scalar(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

func str(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

// toJSON writes a YAML value as indented JSON, keeping the order of the keys.
func toJSON(n *yaml.Node, depth int) string {
	pad := strings.Repeat("  ", depth)
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) > 0 {
			return toJSON(n.Content[0], depth)
		}
	case yaml.AliasNode:
		return toJSON(n.Alias, depth)
	case yaml.MappingNode:
		if len(n.Content) == 0 {
			return "{}"
		}
		var b strings.Builder
		b.WriteString("{\n")
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, _ := json.Marshal(n.Content[i].Value)
			b.WriteString(pad + "  " + string(key) + ": " + toJSON(n.Content[i+1], depth+1))
			if i+2 < len(n.Content) {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		return b.String() + pad + "}"
	case yaml.SequenceNode:
		if len(n.Content) == 0 {
			return "[]"
		}
		var b strings.Builder
		b.WriteString("[\n")
		for i, it := range n.Content {
			b.WriteString(pad + "  " + toJSON(it, depth+1))
			if i < len(n.Content)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		return b.String() + pad + "]"
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!int", "!!float":
			if json.Valid([]byte(n.Value)) {
				return n.Value
			}
		case "!!bool":
			return strings.ToLower(n.Value)
		case "!!null":
			return "null"
		}
		v, _ := json.Marshal(n.Value)
		return string(v)
	}
	return "null"
}
