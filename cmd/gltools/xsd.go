package main

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// xsdExamples reads an XSD schema and gives one result per global element: the name is the
// element, the value an example XML document built from the schema. Comments tell which
// elements are optional, repeated or alternatives (choice); values are the type names.
func xsdExamples(text string) ([]Result, error) {
	if _, err := validateJSONorXML(text); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "<") {
		return nil, errors.New("paste an XSD schema (XML)")
	}
	root, err := parseXSD(text)
	if err != nil {
		return nil, err
	}
	if root.local() != "schema" {
		return nil, errors.New("this is XML but not an XSD schema: the root should be <xs:schema>")
	}
	x := &xsd{root: root, prefix: root.prefix()}
	var results []Result
	for _, el := range root.kids {
		if el.local() == "element" && el.attr("name") != "" {
			var b strings.Builder
			x.element(&b, el, 0, nil, true)
			results = append(results, Result{Name: el.attr("name"), Value: strings.TrimRight(b.String(), "\n")})
		}
	}
	if len(results) == 0 {
		return nil, errors.New("no global elements found in the schema")
	}
	return results, nil
}

// xsdNode is an XSD node: tag name as written (e.g. "xs:element"), attributes and children.
type xsdNode struct {
	name  string
	attrs []xattr
	kids  []*xsdNode
}

func (n *xsdNode) local() string { return n.name[strings.LastIndex(n.name, ":")+1:] }

func (n *xsdNode) prefix() string {
	if i := strings.Index(n.name, ":"); i >= 0 {
		return n.name[:i]
	}
	return ""
}

func (n *xsdNode) attr(name string) string {
	for _, a := range n.attrs {
		if a.name == name {
			return a.value
		}
	}
	return ""
}

func (n *xsdNode) child(local string) *xsdNode {
	for _, k := range n.kids {
		if k.local() == local {
			return k
		}
	}
	return nil
}

func parseXSD(text string) (*xsdNode, error) {
	var root *xsdNode
	var stack []*xsdNode
	err := walkXML(text, func(e *xelem, start bool) {
		if !start {
			stack = stack[:len(stack)-1]
			return
		}
		n := &xsdNode{name: e.name, attrs: e.attrs}
		if len(stack) > 0 {
			p := stack[len(stack)-1]
			p.kids = append(p.kids, n)
		} else if root == nil {
			root = n
		}
		stack = append(stack, n)
	})
	return root, err
}

type xsd struct {
	root   *xsdNode
	prefix string // prefix of the XML Schema namespace, usually "xs" or "xsd"
}

// global finds a top level definition, e.g. the complexType named "MitgliedType".
func (x *xsd) global(kind, name string) *xsdNode {
	name = name[strings.LastIndex(name, ":")+1:]
	for _, k := range x.root.kids {
		if k.local() == kind && k.attr("name") == name {
			return k
		}
	}
	return nil
}

// occurs writes the comment for minOccurs/maxOccurs: "optional", "0..n", "1..n", "2..5".
func occurs(n *xsdNode) string {
	lo, hi := n.attr("minOccurs"), n.attr("maxOccurs")
	if lo == "" {
		lo = "1"
	}
	if hi == "" {
		hi = "1"
	}
	if hi == "unbounded" {
		hi = "n"
	}
	switch {
	case lo == "1" && hi == "1":
		return ""
	case lo == "0" && hi == "1":
		return "optional"
	}
	return lo + ".." + hi
}

// element writes one element of the example. seen holds the named types being written,
// to stop at a type that contains itself.
func (x *xsd) element(b *strings.Builder, el *xsdNode, depth int, seen []string, top bool, notes ...string) {
	pad := strings.Repeat("  ", depth)
	if ref := el.attr("ref"); ref != "" {
		if g := x.global("element", ref); g != nil {
			// The occurrences are written on the reference, the rest on the global element.
			x.element(b, g, depth, seen, false, append(notes, occurs(el))...)
			return
		}
	}
	name := el.attr("name")
	notes = append(notes, occurs(el))
	addNote := func(n string) { notes = append(notes, n) }

	var attrs []string
	var body strings.Builder
	text := ""
	if top {
		if ns := x.root.attr("targetNamespace"); ns != "" {
			attrs = append(attrs, `xmlns="`+xmlEscape(ns)+`"`)
		}
	}

	ct := el.child("complexType")
	if t := el.attr("type"); t != "" && ct == nil {
		switch {
		case x.global("complexType", t) != nil:
			for _, s := range seen {
				if s == t {
					b.WriteString(pad + "<" + name + "/>   <!-- " + t + ", again -->\n")
					return
				}
			}
			ct = x.global("complexType", t)
			seen = append(seen, t)
		case x.global("simpleType", t) != nil:
			var n string
			text, n = x.simpleValue(x.global("simpleType", t))
			addNote(n)
		default:
			text = t[strings.LastIndex(t, ":")+1:]
		}
	} else if st := el.child("simpleType"); st != nil {
		var n string
		text, n = x.simpleValue(st)
		addNote(n)
	}
	if ct != nil {
		text = x.complexContent(&body, ct, depth+1, seen, &attrs)
	}

	var note string
	for _, n := range notes {
		if n != "" {
			note += ", " + n
		}
	}
	if note != "" {
		note = "   <!-- " + note[2:] + " -->"
	}

	open := "<" + name
	for _, a := range attrs {
		open += " " + a
	}
	switch {
	case body.Len() > 0:
		b.WriteString(pad + open + ">" + note + "\n" + body.String() + pad + "</" + name + ">\n")
	case text != "":
		b.WriteString(pad + open + ">" + xmlEscape(text) + "</" + name + ">" + note + "\n")
	default:
		b.WriteString(pad + open + "/>" + note + "\n")
	}
}

// complexContent writes the child elements of a complex type into b, adds its attributes,
// and returns the text value for a type with simple content.
func (x *xsd) complexContent(b *strings.Builder, ct *xsdNode, depth int, seen []string, attrs *[]string) string {
	text := ""
	for _, k := range ct.kids {
		switch k.local() {
		case "sequence", "all", "choice", "group":
			x.particle(b, k, depth, seen)
		case "attribute", "attributeGroup":
			x.attribute(k, attrs)
		case "complexContent", "simpleContent":
			ext := k.child("extension")
			if ext == nil {
				ext = k.child("restriction")
			}
			if ext == nil {
				continue
			}
			if base := ext.attr("base"); base != "" {
				if bt := x.global("complexType", base); bt != nil && k.local() == "complexContent" {
					text = x.complexContent(b, bt, depth, seen, attrs)
				} else if st := x.global("simpleType", base); st != nil {
					text, _ = x.simpleValue(st)
				} else {
					text = base[strings.LastIndex(base, ":")+1:]
				}
			}
			if t := x.complexContent(b, ext, depth, seen, attrs); t != "" {
				text = t
			}
		}
	}
	return text
}

// particle writes the elements of a sequence, all, choice or group.
func (x *xsd) particle(b *strings.Builder, p *xsdNode, depth int, seen []string) {
	if p.local() == "group" {
		if ref := p.attr("ref"); ref != "" {
			if g := x.global("group", ref); g != nil {
				p = g
			}
		}
	}
	for _, k := range p.kids {
		switch k.local() {
		case "element":
			if p.local() == "choice" {
				x.element(b, k, depth, seen, false, "choice: one of these")
			} else {
				x.element(b, k, depth, seen, false)
			}
		case "sequence", "all", "choice", "group":
			x.particle(b, k, depth, seen)
		case "any":
			b.WriteString(strings.Repeat("  ", depth) + "<!-- any element -->\n")
		}
	}
}

func (x *xsd) attribute(a *xsdNode, attrs *[]string) {
	if a.local() == "attributeGroup" {
		g := x.global("attributeGroup", a.attr("ref"))
		if g == nil {
			return
		}
		for _, k := range g.kids {
			x.attribute(k, attrs)
		}
		return
	}
	name := a.attr("name")
	if ref := a.attr("ref"); name == "" && ref != "" {
		name = ref
	}
	if name == "" || a.attr("use") == "prohibited" {
		return
	}
	value := "string"
	if t := a.attr("type"); t != "" {
		if st := x.global("simpleType", t); st != nil {
			value, _ = x.simpleValue(st)
		} else {
			value = t[strings.LastIndex(t, ":")+1:]
		}
	} else if st := a.child("simpleType"); st != nil {
		value, _ = x.simpleValue(st)
	}
	if f := a.attr("fixed"); f != "" {
		value = f
	}
	*attrs = append(*attrs, name+`="`+xmlEscape(value)+`"`)
}

// simpleValue is the base type name, or the first allowed value of an enumeration;
// note then lists all the allowed values.
func (x *xsd) simpleValue(st *xsdNode) (value, note string) {
	r := st.child("restriction")
	if r == nil {
		if l := st.child("list"); l != nil {
			return "list of " + strings.TrimPrefix(l.attr("itemType"), x.prefix+":"), ""
		}
		if st.child("union") != nil {
			return "union", ""
		}
		return "string", ""
	}
	var values []string
	for _, k := range r.kids {
		if k.local() == "enumeration" {
			values = append(values, k.attr("value"))
		}
	}
	if len(values) > 0 {
		if len(values) > 1 {
			note = "one of: " + strings.Join(values, ", ")
		}
		return values[0], note
	}
	base := r.attr("base")
	if st := x.global("simpleType", base); st != nil {
		return x.simpleValue(st)
	}
	return base[strings.LastIndex(base, ":")+1:], ""
}

// walkXML calls fn for every start tag (start true) and end tag (start false, e nil).
func walkXML(text string, fn func(e *xelem, start bool)) error {
	d := xml.NewDecoder(strings.NewReader(text))
	for {
		tok, err := d.RawToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			e := &xelem{name: xmlName(t.Name)}
			for _, a := range t.Attr {
				e.attrs = append(e.attrs, xattr{xmlName(a.Name), a.Value})
			}
			fn(e, true)
		case xml.EndElement:
			fn(nil, false)
		}
	}
}
