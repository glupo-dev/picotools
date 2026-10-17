package gui

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
)

// Format gives JSON or XML text back indented, one element per line, with its language
// ("JSON" or "XML"). Text that is neither, or not well formed, comes back as it is with "".
func Format(text string) (lang, out string) {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "<") && wellFormedXML(t) {
		return "XML", IndentXML(t)
	}
	if (strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")) && json.Valid([]byte(t)) {
		var buf bytes.Buffer
		json.Indent(&buf, []byte(t), "", "  ")
		return "JSON", buf.String()
	}
	return "", text
}

// wellFormedXML reads the whole document: tags must be closed in the right order.
func wellFormedXML(text string) bool {
	d := xml.NewDecoder(strings.NewReader(text))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
	}
}

// IndentXML writes a well formed XML document with one tag per line, indented by two spaces.
// An element that holds only text stays on one line: <name>Mario</name>.
// Prefixes are kept as written (RawToken does not resolve namespaces).
func IndentXML(text string) string {
	d := xml.NewDecoder(strings.NewReader(text))
	var toks []xml.Token
	for {
		tok, err := d.RawToken()
		if err != nil {
			break
		}
		toks = append(toks, xml.CopyToken(tok))
	}

	var b strings.Builder
	depth := 0
	write := func(s string) {
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString(s)
		b.WriteByte('\n')
	}
	for i := 0; i < len(toks); i++ {
		switch t := toks[i].(type) {
		case xml.StartElement:
			open := "<" + XMLName(t.Name)
			for _, a := range t.Attr {
				open += " " + XMLName(a.Name) + `="` + XMLEscape(a.Value) + `"`
			}
			// <a/> when nothing is inside, <a>text</a> when only text is inside.
			if i+1 < len(toks) {
				if _, ok := toks[i+1].(xml.EndElement); ok {
					write(open + "/>")
					i++
					continue
				}
			}
			if i+2 < len(toks) {
				cd, isText := toks[i+1].(xml.CharData)
				_, closes := toks[i+2].(xml.EndElement)
				if txt := strings.TrimSpace(string(cd)); isText && closes && txt == "" {
					write(open + "/>")
					i += 2
					continue
				} else if isText && closes && !strings.Contains(txt, "\n") {
					write(open + ">" + XMLEscape(txt) + "</" + XMLName(t.Name) + ">")
					i += 2
					continue
				}
			}
			write(open + ">")
			depth++
		case xml.EndElement:
			depth--
			write("</" + XMLName(t.Name) + ">")
		case xml.CharData:
			for _, l := range strings.Split(string(t), "\n") {
				if l = strings.TrimSpace(l); l != "" {
					write(XMLEscape(l))
				}
			}
		case xml.Comment:
			write("<!--" + string(t) + "-->")
		case xml.ProcInst:
			write("<?" + t.Target + " " + string(t.Inst) + "?>")
		case xml.Directive:
			write("<!" + string(t) + ">")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// XMLName is the name with its prefix, as written.
func XMLName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// XMLEscape escapes the characters that cannot appear as they are in XML text or attributes.
func XMLEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
