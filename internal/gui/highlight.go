package gui

import (
	"image/color"
	"strings"
)

// Colors of the syntax: the structure stays in the background, the data stands out (default color).
var (
	syntaxTag    = Accent  // XML tag names, JSON keys
	syntaxAttr   = Amber   // XML attribute names, JSON true/false/null
	syntaxValue  = ValueFg // XML attribute values, JSON numbers
	syntaxPunct  = Muted   // JSON brackets and commas, XML comments
	syntaxPlain  color.Color
	jsonWordRune = "abcdefghijklmnopqrstuvwxyz"
)

// Highlight splits one line of formatted JSON or XML into colored pieces.
// Other languages, or lines it does not understand, come back as plain text.
func Highlight(lang, l string) []Span {
	switch lang {
	case "XML":
		return highlightXML(l)
	case "JSON":
		return highlightJSON(l)
	}
	return []Span{Plain(l)}
}

func highlightXML(l string) []Span {
	var out []Span
	add := func(s string, c color.Color) {
		if s != "" {
			out = append(out, Span{Text: s, Color: c})
		}
	}
	for len(l) > 0 {
		switch {
		case strings.HasPrefix(l, "<!--"):
			end := strings.Index(l, "-->")
			if end < 0 {
				add(l, syntaxPunct)
				return out
			}
			add(l[:end+3], syntaxPunct)
			l = l[end+3:]
		case strings.HasPrefix(l, "<"):
			l = xmlTag(l, add)
		default:
			end := strings.Index(l, "<")
			if end < 0 {
				end = len(l)
			}
			add(l[:end], syntaxPlain)
			l = l[end:]
		}
	}
	return out
}

// xmlTag colors one tag like <a:Name id="5"> and returns what follows it.
func xmlTag(l string, add func(string, color.Color)) string {
	// "<", "</" or "<?" and the name
	i := 1
	for i < len(l) && (l[i] == '/' || l[i] == '?' || l[i] == '!') {
		i++
	}
	for i < len(l) && !strings.ContainsRune(" \t>/?", rune(l[i])) {
		i++
	}
	add(l[:i], syntaxTag)
	l = l[i:]
	for len(l) > 0 {
		switch {
		case l[0] == ' ' || l[0] == '\t':
			add(l[:1], syntaxPlain)
			l = l[1:]
		case l[0] == '>':
			add(">", syntaxTag)
			return l[1:]
		case strings.HasPrefix(l, "/>") || strings.HasPrefix(l, "?>"):
			add(l[:2], syntaxTag)
			return l[2:]
		case l[0] == '"' || l[0] == '\'':
			end := strings.IndexByte(l[1:], l[0])
			if end < 0 {
				add(l, syntaxValue)
				return ""
			}
			add(l[:end+2], syntaxValue)
			l = l[end+2:]
		case l[0] == '=':
			add("=", syntaxPunct)
			l = l[1:]
		default: // attribute name
			j := 0
			for j < len(l) && !strings.ContainsRune(" \t=>/?\"'", rune(l[j])) {
				j++
			}
			if j == 0 {
				j = 1
			}
			add(l[:j], syntaxAttr)
			l = l[j:]
		}
	}
	return l
}

func highlightJSON(l string) []Span {
	var out []Span
	add := func(s string, c color.Color) {
		if s != "" {
			out = append(out, Span{Text: s, Color: c})
		}
	}
	for len(l) > 0 {
		c := l[0]
		switch {
		case c == ' ' || c == '\t':
			j := 0
			for j < len(l) && (l[j] == ' ' || l[j] == '\t') {
				j++
			}
			add(l[:j], syntaxPlain)
			l = l[j:]
		case strings.HasPrefix(l, "//"): // comment added by the skeleton
			add(l, syntaxPunct)
			return out
		case c == '"':
			j := 1
			for j < len(l) && l[j] != '"' {
				if l[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(l))
			// A string followed by ":" is a key.
			if strings.HasPrefix(strings.TrimLeft(l[j:], " "), ":") {
				add(l[:j], syntaxTag)
			} else {
				add(l[:j], syntaxPlain)
			}
			l = l[j:]
		case strings.ContainsRune("{}[],:", rune(c)):
			add(l[:1], syntaxPunct)
			l = l[1:]
		case c == '-' || c >= '0' && c <= '9':
			j := 0
			for j < len(l) && strings.ContainsRune("-+.eE0123456789", rune(l[j])) {
				j++
			}
			add(l[:j], syntaxValue)
			l = l[j:]
		case strings.ContainsRune(jsonWordRune, rune(c)):
			j := 0
			for j < len(l) && strings.ContainsRune(jsonWordRune, rune(l[j])) {
				j++
			}
			add(l[:j], syntaxAttr) // true, false, null
			l = l[j:]
		default:
			add(l[:1], syntaxPlain)
			l = l[1:]
		}
	}
	return out
}

// WrapSpans cuts a line of colored pieces into rows of at most width characters.
func WrapSpans(spans []Span, width int) [][]Span {
	rows := [][]Span{nil}
	used := 0
	for _, s := range spans {
		r := []rune(s.Text)
		for len(r) > 0 {
			if used == width {
				rows = append(rows, nil)
				used = 0
			}
			n := min(len(r), width-used)
			rows[len(rows)-1] = append(rows[len(rows)-1], Span{Text: string(r[:n]), Color: s.Color, Bold: s.Bold})
			used += n
			r = r[n:]
		}
	}
	return rows
}
