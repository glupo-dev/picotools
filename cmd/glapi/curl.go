package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

// The key is never written in a curl command: whoever runs it puts it in place of this.
const keyPlaceholder = "<API_KEY>"

// curlShell is one of the shells a curl command can be copied for: they differ in
// the program name and in how text is quoted.
type curlShell struct {
	name  string
	exe   string
	quote func(string) string
}

var curlShells = []curlShell{
	{"Mac / Linux", "curl", func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }},
	{"Windows cmd", "curl.exe", func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }},
	{"Windows PowerShell", "curl.exe", func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }},
}

// curl writes the call as a curl command on one line, with the key placeholder.
func (c *callSpec) curl(sh curlShell) string {
	args := []string{sh.exe}
	switch c.method {
	case "GET":
	case "HEAD":
		args = append(args, "-I")
	default:
		args = append(args, "-X", c.method)
	}
	key := ""
	if c.keyIn != keyNone {
		key = keyPlaceholder
	}
	args = append(args, sh.quote(c.urlWith(key, true)))
	for _, h := range c.headersWith(key) {
		args = append(args, "-H", sh.quote(h.Name+": "+h.Value))
	}
	if c.body != "" {
		args = append(args, "-d", sh.quote(oneLine(c.body)))
	}
	return strings.Join(args, " ")
}

// oneLine puts the body on one line: JSON compacted, any other text with spaces
// in place of the line breaks. A body already on one line stays as it is.
func oneLine(body string) string {
	if !strings.ContainsAny(body, "\r\n") {
		return body
	}
	var buf bytes.Buffer
	if json.Compact(&buf, []byte(body)) == nil {
		return buf.String()
	}
	return strings.Join(strings.Fields(strings.ReplaceAll(body, "\r\n", "\n")), " ")
}
