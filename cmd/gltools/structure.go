package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// structureOf describes the fields of a JSON or XML document: their type, how often they
// appear, whether they are sometimes missing, empty or of a different type. In a list of
// records (or repeated XML elements) all the records are merged into one description.
func structureOf(text string) ([]Result, error) {
	if _, err := validateJSONorXML(text); err != nil {
		return nil, err
	}
	text = unpackString(strings.TrimSpace(text))
	var lines []string
	var err error
	if strings.HasPrefix(text, "<") {
		lines, err = xmlStructure(text)
	} else {
		lines, err = jsonStructure(text)
	}
	if err != nil {
		return nil, err
	}
	return []Result{{Name: "Structure", Value: strings.Join(lines, "\n")}}, nil
}

// unpackString returns the JSON or XML held in a JSON string, or the text itself.
func unpackString(text string) string {
	var inner string
	if strings.HasPrefix(text, `"`) && json.Unmarshal([]byte(text), &inner) == nil {
		if s := strings.TrimSpace(inner); strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") || strings.HasPrefix(s, "<") || strings.HasPrefix(s, `"`) {
			return unpackString(s)
		}
	}
	return text
}

// fieldRow is one line of the description: name (indented), type, "x of y", notes.
type fieldRow struct{ name, typ, count, notes string }

// formatRows lines up the columns.
func formatRows(rows []fieldRow) []string {
	if len(rows) == 0 {
		return []string{"(no fields)"}
	}
	nameW, typW, countW := 0, 0, 0
	for _, r := range rows {
		nameW, typW, countW = max(nameW, len([]rune(r.name))), max(typW, len(r.typ)), max(countW, len(r.count))
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %*s  %s", nameW, r.name, typW, r.typ, countW, r.count, r.notes), " ")
	}
	return lines
}

func timesRange(lo, hi int, unit string) string {
	if lo == hi {
		return plural(hi, unit)
	}
	return fmt.Sprintf("%d–%d %ss", lo, hi, unit)
}

// ---- JSON ----

// jnode is a parsed JSON value that keeps the fields of objects in their original order.
type jnode struct {
	kind   string // "object", "list", "text", "number", "true/false", "null"
	keys   []string
	fields []*jnode
	items  []*jnode
}

// jshape sums up every value found at one place of the document: in all records,
// the same field is merged into the same jshape.
type jshape struct {
	seen    int            // values found here
	nulls   int            // ...of which null
	kinds   map[string]int // ...by kind, null excluded
	objects int            // how many were objects: the base for "x of y" of the fields
	names   []string       // fields in order of first appearance
	fields  map[string]*jshape
	lists   int // how many were lists
	minLen  int
	maxLen  int
	item    *jshape // all the elements of those lists
}

func newJShape() *jshape { return &jshape{kinds: map[string]int{}, fields: map[string]*jshape{}} }

func jsonStructure(text string) ([]string, error) {
	root, err := parseJSON(json.NewDecoder(strings.NewReader(text)))
	if err != nil {
		return nil, err
	}
	s := newJShape()
	s.add(root)

	var rows []fieldRow
	switch root.kind {
	case "list":
		head := "Structure: JSON list of " + countOf(len(root.items), s.item)
		if s.item == nil {
			return []string{head}, nil
		}
		if s.item.objects == 0 || len(s.item.kinds) > 1 {
			rows = append(rows, fieldRow{"[ ]", s.item.typeName(), "", s.item.notes(s.item.seen)})
		}
		s.item.addRows(&rows, 0)
		return append([]string{head, ""}, formatRows(rows)...), nil
	case "object":
		s.addRows(&rows, 0)
		head := "Structure: JSON object with " + plural(len(root.keys), "field")
		return append([]string{head, ""}, formatRows(rows)...), nil
	}
	return []string{"Structure: a single JSON " + root.kind + " value"}, nil
}

func parseJSON(d *json.Decoder) (*jnode, error) {
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			n := &jnode{kind: "object"}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return nil, err
				}
				v, err := parseJSON(d)
				if err != nil {
					return nil, err
				}
				n.keys, n.fields = append(n.keys, k.(string)), append(n.fields, v)
			}
			_, err = d.Token() // }
			return n, err
		}
		n := &jnode{kind: "list"}
		for d.More() {
			v, err := parseJSON(d)
			if err != nil {
				return nil, err
			}
			n.items = append(n.items, v)
		}
		_, err = d.Token() // ]
		return n, err
	case string:
		return &jnode{kind: "text"}, nil
	case float64:
		return &jnode{kind: "number"}, nil
	case bool:
		return &jnode{kind: "true/false"}, nil
	case nil:
		return &jnode{kind: "null"}, nil
	}
	return nil, io.ErrUnexpectedEOF
}

func (s *jshape) add(n *jnode) {
	s.seen++
	if n.kind == "null" {
		s.nulls++
		return
	}
	s.kinds[n.kind]++
	switch n.kind {
	case "object":
		s.objects++
		for i, k := range n.keys {
			f, ok := s.fields[k]
			if !ok {
				f = newJShape()
				s.fields[k] = f
				s.names = append(s.names, k)
			}
			f.add(n.fields[i])
		}
	case "list":
		if s.lists == 0 || len(n.items) < s.minLen {
			s.minLen = len(n.items)
		}
		s.maxLen = max(s.maxLen, len(n.items))
		s.lists++
		if s.item == nil {
			s.item = newJShape()
		}
		for _, it := range n.items {
			s.item.add(it)
		}
	}
}

// typeName is "text", "number / null", "object"... in a fixed order.
func (s *jshape) typeName() string {
	var parts []string
	for _, k := range []string{"text", "number", "true/false", "object", "list"} {
		if s.kinds[k] > 0 {
			parts = append(parts, k)
		}
	}
	if s.nulls > 0 {
		parts = append(parts, "null")
	}
	return strings.Join(parts, " / ")
}

// countOf says "1000 objects", "5 numbers", or "12 values" when they are mixed.
func countOf(n int, item *jshape) string {
	if item == nil || len(item.kinds) != 1 || item.nulls > 0 {
		return plural(n, "value")
	}
	for k := range item.kinds {
		switch k {
		case "text":
			return plural(n, "text")
		case "true/false":
			return plural(n, "true/false value")
		}
		return plural(n, k)
	}
	return ""
}

// addRows adds one row per field of the objects found in s, followed by what is inside it.
func (s *jshape) addRows(rows *[]fieldRow, depth int) {
	pad := strings.Repeat("  ", depth)
	for _, name := range s.names {
		f := s.fields[name]
		*rows = append(*rows, fieldRow{pad + name, f.typeName(), fmt.Sprintf("%d of %d", f.seen, s.objects), f.notes(s.objects)})
		if f.objects > 0 {
			f.addRows(rows, depth+1)
		}
		// For a list, a "[ ]" row describes its elements, then their fields.
		if f.item != nil && f.item.seen > 0 {
			*rows = append(*rows, fieldRow{pad + "  [ ]", f.item.typeName(), "", f.item.notes(f.item.seen)})
			if f.item.objects > 0 {
				f.item.addRows(rows, depth+2)
			}
		}
	}
}

// notes points out what is worth knowing: missing values, nulls, mixed types, list sizes.
func (s *jshape) notes(total int) string {
	var n []string
	if missing := total - s.seen; missing > 0 {
		n = append(n, fmt.Sprintf("%d missing", missing))
	}
	if s.nulls > 0 && s.nulls < s.seen {
		n = append(n, fmt.Sprintf("%d empty (null)", s.nulls))
	}
	if s.lists > 0 {
		n = append(n, timesRange(s.minLen, s.maxLen, "element"))
	}
	if len(s.kinds) > 1 {
		n = append(n, "⚠ mixed types")
	}
	return strings.Join(n, "   ")
}

// ---- XML ----

// xnode is a parsed XML element.
type xnode struct {
	name  string
	attrs []string
	text  bool // has some text of its own (not only spaces)
	kids  []*xnode
}

// xshape sums up all the elements with the same path, e.g. every <Ort> inside <Kunde>.
type xshape struct {
	seen     int // elements found
	withText int // ...that have text
	withKids int // ...that have child elements
	attrs    []string
	attrSeen map[string]int
	names    []string // child elements in order of first appearance
	kids     map[string]*xshape
	parents  int // parent elements that contain at least one of these
	minTimes int // how many times it appears in one parent
	maxTimes int
}

func newXShape() *xshape { return &xshape{attrSeen: map[string]int{}, kids: map[string]*xshape{}} }

func xmlStructure(text string) ([]string, error) {
	root, err := parseXML(xml.NewDecoder(strings.NewReader(text)))
	if err != nil {
		return nil, err
	}
	s := newXShape()
	s.add(root)
	var rows []fieldRow
	s.addRows(&rows, 0)
	head := "Structure: XML, root element <" + root.name + ">"
	// The usual list of records: the root holds only one kind of element, many times.
	if len(s.names) == 1 && s.kids[s.names[0]].maxTimes > 1 {
		k := s.kids[s.names[0]]
		head = fmt.Sprintf("Structure: XML, <%s> with %d <%s> elements", root.name, k.seen, s.names[0])
	}
	return append([]string{head, ""}, formatRows(rows)...), nil
}

// parseXML builds the element tree. Prefixes are kept as written; xmlns declarations are left out.
func parseXML(d *xml.Decoder) (*xnode, error) {
	var stack []*xnode
	var root *xnode
	for {
		tok, err := d.RawToken()
		if err == io.EOF {
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{name: xmlName(t.Name)}
			for _, a := range t.Attr {
				if a.Name.Space != "xmlns" && a.Name.Local != "xmlns" {
					n.attrs = append(n.attrs, xmlName(a.Name))
				}
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 && strings.TrimSpace(string(t)) != "" {
				stack[len(stack)-1].text = true
			}
		}
	}
}

func (s *xshape) add(n *xnode) {
	s.seen++
	if n.text {
		s.withText++
	}
	if len(n.kids) > 0 {
		s.withKids++
	}
	for _, a := range n.attrs {
		if s.attrSeen[a] == 0 {
			s.attrs = append(s.attrs, a)
		}
		s.attrSeen[a]++
	}
	times := map[string]int{}
	for _, k := range n.kids {
		ks, ok := s.kids[k.name]
		if !ok {
			ks = newXShape()
			s.kids[k.name] = ks
			s.names = append(s.names, k.name)
		}
		times[k.name]++
		ks.add(k)
	}
	for name, t := range times {
		ks := s.kids[name]
		if ks.parents == 0 || t < ks.minTimes {
			ks.minTimes = t
		}
		ks.maxTimes = max(ks.maxTimes, t)
		ks.parents++
	}
}

// typeName: "text" for an element with only text, "element" when it holds other elements
// or attributes, "empty" when it holds nothing.
func (s *xshape) typeName() string {
	var parts []string
	if s.withKids > 0 || len(s.attrs) > 0 {
		parts = append(parts, "element")
	}
	if s.withText > 0 {
		parts = append(parts, "text")
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, " + ")
}

// addRows adds the attributes (as @name) and the child elements of s, then what is inside each.
func (s *xshape) addRows(rows *[]fieldRow, depth int) {
	pad := strings.Repeat("  ", depth)
	for _, a := range s.attrs {
		var notes string
		if missing := s.seen - s.attrSeen[a]; missing > 0 {
			notes = fmt.Sprintf("%d missing", missing)
		}
		*rows = append(*rows, fieldRow{pad + "@" + a, "attribute", fmt.Sprintf("%d of %d", s.attrSeen[a], s.seen), notes})
	}
	for _, name := range s.names {
		k := s.kids[name]
		var notes []string
		if missing := s.seen - k.parents; missing > 0 {
			notes = append(notes, fmt.Sprintf("%d missing", missing))
		}
		if k.maxTimes > 1 {
			notes = append(notes, "repeated "+timesRange(k.minTimes, k.maxTimes, "time"))
		}
		if k.withText > 0 && k.withText < k.seen && k.withKids == 0 {
			notes = append(notes, fmt.Sprintf("%d empty", k.seen-k.withText))
		}
		*rows = append(*rows, fieldRow{pad + name, k.typeName(), fmt.Sprintf("%d of %d", k.parents, s.seen), strings.Join(notes, "   ")})
		k.addRows(rows, depth+1)
	}
}
