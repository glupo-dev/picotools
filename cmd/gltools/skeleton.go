package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// skeletonOf rewrites a JSON or XML document with one record for every repeated element
// (or list item), holding all the fields found in all the records, and the type of data
// in place of each value. It shows how the document is built.
func skeletonOf(text string) ([]Result, error) {
	if _, err := validateJSONorXML(text); err != nil {
		return nil, err
	}
	text = unpackString(strings.TrimSpace(text))
	if strings.HasPrefix(text, "<") {
		out, err := xmlSkeleton(text)
		if err != nil {
			return nil, err
		}
		return []Result{{Name: "XML", Value: out}}, nil
	}
	out, err := jsonSkeleton(text)
	if err != nil {
		return nil, err
	}
	return []Result{{Name: "JSON", Value: out}}, nil
}

var (
	reDate     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reDateTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}`)
	reTime     = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?$`)
	reNumber   = regexp.MustCompile(`^[+-]?\d+([.,]\d+)?$`)
)

// valueType guesses the kind of data held in a text value.
func valueType(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case reDate.MatchString(s):
		return "date"
	case reDateTime.MatchString(s):
		return "datetime"
	case reTime.MatchString(s):
		return "time"
	case reNumber.MatchString(s):
		return "number"
	case strings.EqualFold(s, "true") || strings.EqualFold(s, "false"):
		return "true/false"
	}
	return "text"
}

// typeSet keeps the types found for one value, in the order they were first found.
type typeSet []string

func (t *typeSet) add(typ string) {
	for _, x := range *t {
		if x == typ {
			return
		}
	}
	*t = append(*t, typ)
}

func (t typeSet) String() string { return strings.Join(t, " | ") }

// orderedNames merges the field names of all records: a name seen for the first time
// goes right after the field that came before it in its record.
type orderedNames []string

func (o *orderedNames) merge(names []string) {
	prev := -1
	for _, n := range names {
		i := o.index(n)
		if i < 0 {
			i = prev + 1
			*o = append((*o)[:i], append([]string{n}, (*o)[i:]...)...)
		}
		prev = i
	}
}

func (o orderedNames) index(n string) int {
	for i, x := range o {
		if x == n {
			return i
		}
	}
	return -1
}

// ---- XML ----

type xattr struct{ name, value string }

// xelem is a parsed XML element with its values.
type xelem struct {
	name  string
	attrs []xattr
	text  string
	kids  []*xelem
}

// xskel merges all the elements with the same path.
type xskel struct {
	name     string
	attrs    orderedNames
	attrType map[string]*typeSet
	nsValue  map[string]string // xmlns declarations keep their value
	text     typeSet
	kids     orderedNames
	kid      map[string]*xskel
	maxTimes int // most times it appears in one parent
}

func newXSkel(name string) *xskel {
	return &xskel{name: name, attrType: map[string]*typeSet{}, nsValue: map[string]string{}, kid: map[string]*xskel{}}
}

func xmlSkeleton(text string) (string, error) {
	d := xml.NewDecoder(strings.NewReader(text))
	var stack []*xelem
	var root *xelem
	var prolog []string // <?xml ...?> and other lines before the root
	for {
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			e := &xelem{name: xmlName(t.Name)}
			for _, a := range t.Attr {
				e.attrs = append(e.attrs, xattr{xmlName(a.Name), a.Value})
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, e)
			} else if root == nil {
				root = e
			}
			stack = append(stack, e)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		case xml.ProcInst:
			if root == nil {
				prolog = append(prolog, "<?"+t.Target+" "+string(t.Inst)+"?>")
			}
		}
	}

	s := newXSkel(root.name)
	s.add(root)
	s.maxTimes = 1
	var b strings.Builder
	for _, p := range prolog {
		b.WriteString(p + "\n")
	}
	s.write(&b, 0)
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *xskel) add(e *xelem) {
	var names []string
	for _, a := range e.attrs {
		names = append(names, a.name)
		if a.name == "xmlns" || strings.HasPrefix(a.name, "xmlns:") {
			s.nsValue[a.name] = a.value
			continue
		}
		if s.attrType[a.name] == nil {
			s.attrType[a.name] = &typeSet{}
		}
		s.attrType[a.name].add(valueType(a.value))
	}
	s.attrs.merge(names)
	if strings.TrimSpace(e.text) != "" && len(e.kids) == 0 {
		s.text.add(valueType(e.text))
	}

	names = nil
	times := map[string]int{}
	for _, k := range e.kids {
		if times[k.name] == 0 {
			names = append(names, k.name)
		}
		times[k.name]++
		if s.kid[k.name] == nil {
			s.kid[k.name] = newXSkel(k.name)
		}
		s.kid[k.name].add(k)
	}
	s.kids.merge(names)
	for n, t := range times {
		s.kid[n].maxTimes = max(s.kid[n].maxTimes, t)
	}
}

func (s *xskel) write(b *strings.Builder, depth int) {
	pad := strings.Repeat("  ", depth)
	open := "<" + s.name
	for _, a := range s.attrs {
		v, ok := s.nsValue[a]
		if !ok {
			v = s.attrType[a].String()
		}
		open += " " + a + `="` + xmlEscape(v) + `"`
	}
	note := ""
	if s.maxTimes > 1 {
		note = fmt.Sprintf("   <!-- repeated: %d times -->", s.maxTimes)
	}
	switch {
	case len(s.kids) > 0:
		b.WriteString(pad + open + ">" + note + "\n")
		for _, k := range s.kids {
			s.kid[k].write(b, depth+1)
		}
		b.WriteString(pad + "</" + s.name + ">\n")
	case len(s.text) > 0:
		b.WriteString(pad + open + ">" + s.text.String() + "</" + s.name + ">" + note + "\n")
	default:
		b.WriteString(pad + open + "/>" + note + "\n")
	}
}

// ---- JSON ----

// jskel merges all the values found at the same place: the fields of all objects
// and the items of all lists.
type jskel struct {
	types  typeSet // types of the plain values (text, number, null...)
	object bool
	names  orderedNames
	field  map[string]*jskel
	list   bool
	item   *jskel
	minLen int // shortest and longest list found here
	maxLen int
}

func newJSkel() *jskel { return &jskel{field: map[string]*jskel{}} }

func jsonSkeleton(text string) (string, error) {
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	root, err := parseJSONValues(d)
	if err != nil {
		return "", err
	}
	s := newJSkel()
	s.add(root)
	var b strings.Builder
	s.write(&b, 0)
	return b.String(), nil
}

// jvalue is a parsed JSON value that keeps the order of the fields and the plain values.
type jvalue struct {
	kind   string // "object", "list", "text", "number", "true/false", "null"
	text   string
	keys   []string
	fields []*jvalue
	items  []*jvalue
}

func parseJSONValues(d *json.Decoder) (*jvalue, error) {
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		n := &jvalue{kind: "list"}
		if t == '{' {
			n.kind = "object"
		}
		for d.More() {
			if n.kind == "object" {
				k, err := d.Token()
				if err != nil {
					return nil, err
				}
				n.keys = append(n.keys, k.(string))
			}
			v, err := parseJSONValues(d)
			if err != nil {
				return nil, err
			}
			if n.kind == "object" {
				n.fields = append(n.fields, v)
			} else {
				n.items = append(n.items, v)
			}
		}
		_, err = d.Token() // } or ]
		return n, err
	case string:
		return &jvalue{kind: "text", text: t}, nil
	case json.Number:
		return &jvalue{kind: "number"}, nil
	case bool:
		return &jvalue{kind: "true/false"}, nil
	case nil:
		return &jvalue{kind: "null"}, nil
	}
	return nil, io.ErrUnexpectedEOF
}

func (s *jskel) add(v *jvalue) {
	switch v.kind {
	case "object":
		s.object = true
		s.names.merge(v.keys)
		for i, k := range v.keys {
			if s.field[k] == nil {
				s.field[k] = newJSkel()
			}
			s.field[k].add(v.fields[i])
		}
	case "list":
		if !s.list || len(v.items) < s.minLen {
			s.minLen = len(v.items)
		}
		s.maxLen = max(s.maxLen, len(v.items))
		s.list = true
		if s.item == nil {
			s.item = newJSkel()
		}
		for _, it := range v.items {
			s.item.add(it)
		}
	case "text":
		s.types.add(valueType(v.text)) // dates and numbers written as text are told apart
	default:
		s.types.add(v.kind)
	}
}

// write prints an object or a list as such; plain values become a string with their types.
// After "[" a comment says how many items the lists have.
// A value that is sometimes an object and sometimes something else is shown as the object.
func (s *jskel) write(b *strings.Builder, depth int) {
	pad := strings.Repeat("  ", depth)
	switch {
	case s.object:
		if len(s.names) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, n := range s.names {
			key, _ := json.Marshal(n)
			b.WriteString(pad + "  " + string(key) + ": ")
			s.field[n].write(b, depth+1)
			if i < len(s.names)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad + "}")
	case s.list:
		if s.item == nil || (!s.item.object && !s.item.list && len(s.item.types) == 0) {
			b.WriteString("[]")
			return
		}
		// Not valid JSON, but this text is only to be read: say how many items the lists have.
		b.WriteString("[   // " + timesRange(s.minLen, s.maxLen, "item") + "\n" + pad + "  ")
		s.item.write(b, depth+1)
		b.WriteString("\n" + pad + "]")
	default:
		v, _ := json.Marshal(s.types.String())
		b.WriteString(string(v))
	}
}
