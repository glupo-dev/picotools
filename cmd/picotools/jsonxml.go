package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"picotools/internal/gui"
)

// Shorter names for the XML helpers of the gui package, used by several tools.
var (
	xmlName   = gui.XMLName
	xmlEscape = gui.XMLEscape
)

// validateJSONorXML only says whether the text is well formed: text starting with "<" is
// checked as XML, anything else as JSON. The text is never changed.
func validateJSONorXML(text string) ([]Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("paste the JSON or XML text")
	}
	if strings.HasPrefix(text, "<") {
		if err := checkXML(text); err != nil {
			return nil, fmt.Errorf("not valid XML: %w", err)
		}
		return []Result{{Name: "XML", Value: "✔ Valid XML"}}, nil
	}
	if err := checkJSON(text); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	// A JSON string holding JSON or XML (as stored in many databases, e.g. "{\"a\":1}"):
	// check what is inside, otherwise a broken document would pass as a valid string.
	var inner string
	if json.Unmarshal([]byte(text), &inner) == nil {
		if s := strings.TrimSpace(inner); strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") || strings.HasPrefix(s, "<") {
			res, err := validateJSONorXML(inner)
			if err != nil {
				return nil, fmt.Errorf("inside a string, %w", err)
			}
			res[0].Value += " (inside a string)"
			return res, nil
		}
	}
	return []Result{{Name: "JSON", Value: "✔ Valid JSON"}}, nil
}

// checkJSON returns the syntax error, if any, with its line and column.
func checkJSON(text string) error {
	var v any
	err := json.Unmarshal([]byte(text), &v)
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return err
	}
	ln, col := position(text, int(se.Offset))
	return fmt.Errorf("%s (line %d, column %d)", se.Error(), ln, col)
}

// checkXML reads the whole document: tags must be closed in the right order
// and there must be exactly one root element.
func checkXML(text string) error {
	d := xml.NewDecoder(strings.NewReader(text))
	depth, roots := 0, 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err // already says "XML syntax error on line N: ..."
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					ln, col := d.InputPos()
					return fmt.Errorf("a second root element <%s> on line %d, column %d: only one is allowed", t.Name.Local, ln, col)
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				ln, col := d.InputPos()
				return fmt.Errorf("text outside the root element (line %d, column %d)", ln, col)
			}
		}
	}
	if roots == 0 {
		return errors.New("no root element")
	}
	return nil
}

// position turns a byte offset into a line and column, both starting from 1.
func position(text string, offset int) (int, int) {
	before := text[:min(offset, len(text))]
	ln := strings.Count(before, "\n") + 1
	col := utf8.RuneCountInString(before[strings.LastIndex(before, "\n")+1:])
	return ln, col
}

// formatJSONorXML gives the text back indented, one element per line, to be read in the viewer.
// It is checked first like validateJSONorXML, and JSON or XML packed in a string is unpacked.
func formatJSONorXML(text string) ([]Result, error) {
	if _, err := validateJSONorXML(text); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	var inner string
	if !strings.HasPrefix(text, "<") && json.Unmarshal([]byte(text), &inner) == nil {
		if s := strings.TrimSpace(inner); strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") || strings.HasPrefix(s, "<") {
			return formatJSONorXML(inner)
		}
	}
	if strings.HasPrefix(text, "<") {
		return []Result{{Name: "XML", Value: gui.IndentXML(text)}}, nil
	}
	var buf bytes.Buffer
	json.Indent(&buf, []byte(text), "", "  ")
	return []Result{{Name: "JSON", Value: buf.String()}}, nil
}
