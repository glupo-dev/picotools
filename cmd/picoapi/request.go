package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const timeout = 60 * time.Second

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// parseRequest splits "GET /forecast?city={city}" into the method and the path.
func parseRequest(req string) (method, path string, err error) {
	method, path, _ = strings.Cut(strings.TrimSpace(req), " ")
	method = strings.ToUpper(method)
	path = strings.TrimSpace(path)
	known := false
	for _, m := range methods {
		known = known || m == method
	}
	switch {
	case !known:
		return "", "", fmt.Errorf("start with the method: %s", strings.Join(methods, ", "))
	case path == "":
		path = "/"
	case !strings.HasPrefix(path, "/"):
		return "", "", errors.New("the path starts with / (e.g. GET /users/{id})")
	}
	return method, path, nil
}

// hasBody says whether requests with this method usually carry a body.
func hasBody(method string) bool {
	return method == "POST" || method == "PUT" || method == "PATCH"
}

// variable is a {name} or {name?} in the path or the body of an endpoint.
type variable struct {
	name     string
	optional bool
}

var varPattern = regexp.MustCompile(`\{([A-Za-z0-9_.-]+)(\??)\}`)

// variables lists the variables of the endpoint once each, in the order they appear.
// A variable written once as optional and once not is required.
func variables(e *Endpoint) []variable {
	var out []variable
	index := map[string]int{}
	for _, m := range varPattern.FindAllStringSubmatch(e.Request+"\n"+e.Body, -1) {
		name, optional := m[1], m[2] == "?"
		if i, ok := index[name]; ok {
			out[i].optional = out[i].optional && optional
			continue
		}
		index[name] = len(out)
		out = append(out, variable{name, optional})
	}
	return out
}

// fill replaces the variables in s, each value passed through escape.
func fill(s string, values map[string]string, escape func(string) string) string {
	return varPattern.ReplaceAllStringFunc(s, func(v string) string {
		return escape(values[varPattern.FindStringSubmatch(v)[1]])
	})
}

// fillPath puts the values in the path. In the query string a parameter whose optional
// variable is empty is left out entirely.
func fillPath(path string, values map[string]string) string {
	p, query, hasQuery := splitQuery(path)
	p = fill(p, values, url.PathEscape)
	if !hasQuery {
		return p
	}
	var params []string
	for _, param := range strings.Split(query, "&") {
		drop := false
		for _, m := range varPattern.FindAllStringSubmatch(param, -1) {
			if m[2] == "?" && values[m[1]] == "" {
				drop = true
			}
		}
		if !drop && param != "" {
			params = append(params, fill(param, values, url.QueryEscape))
		}
	}
	if len(params) == 0 {
		return p
	}
	return p + "?" + strings.Join(params, "&")
}

// splitQuery cuts the path at the "?" that starts the query string, skipping the
// "?" of optional variables such as {id?}.
func splitQuery(path string) (string, string, bool) {
	depth := 0
	for i, c := range path {
		switch {
		case c == '{':
			depth++
		case c == '}' && depth > 0:
			depth--
		case c == '?' && depth == 0:
			return path[:i], path[i+1:], true
		}
	}
	return path, "", false
}

// jsonEscape escapes a value for the inside of a JSON string: quotes, backslashes, line breaks.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// callSpec is a request ready to send, without the key: the key is added for the real
// request, hidden with •••• on screen and left as a placeholder in the curl commands.
type callSpec struct {
	method  string
	url     string   // without the key
	headers []Header // in the order they are sent, without the key
	body    string
	keyIn   string
	keyName string
}

// newCall fills the endpoint with the values of its variables.
func newCall(p *Provider, e *Endpoint, values map[string]string) (*callSpec, error) {
	method, path, err := parseRequest(e.Request)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimRight(p.URL, "/") + fillPath(path, values))
	if err != nil {
		return nil, err
	}
	c := &callSpec{method: method, url: u.String(), keyIn: p.KeyIn, keyName: p.KeyName}
	if e.Body != "" {
		c.body = fill(e.Body, values, jsonEscape)
		c.setHeader("Content-Type", "application/json")
	}
	for _, h := range p.Headers {
		c.setHeader(h.Name, h.Value)
	}
	return c, nil
}

// setHeader adds a header, or replaces the one with the same name.
func (c *callSpec) setHeader(name, value string) {
	for i, h := range c.headers {
		if strings.EqualFold(h.Name, name) {
			c.headers[i].Value = value
			return
		}
	}
	c.headers = append(c.headers, Header{name, value})
}

// urlWith is the address with the key in the query string, when it goes there.
// The key is escaped unless raw (for •••• and the curl placeholder).
func (c *callSpec) urlWith(key string, raw bool) string {
	if c.keyIn != keyQuery || key == "" {
		return c.url
	}
	if !raw {
		key = url.QueryEscape(key)
	}
	sep := "?"
	if strings.Contains(c.url, "?") {
		sep = "&"
	}
	return c.url + sep + url.QueryEscape(c.keyName) + "=" + key
}

// headersWith is the headers with the key, when it goes in a header.
func (c *callSpec) headersWith(key string) []Header {
	hs := append([]Header(nil), c.headers...)
	switch {
	case key == "":
	case c.keyIn == keyHeader:
		hs = append(hs, Header{c.keyName, key})
	case c.keyIn == keyBearer:
		hs = append(hs, Header{"Authorization", "Bearer " + key})
	}
	return hs
}

// request makes the HTTP request with the real key.
func (c *callSpec) request(ctx context.Context, key string) (*http.Request, error) {
	var body io.Reader
	if c.body != "" {
		body = strings.NewReader(c.body)
	}
	req, err := http.NewRequestWithContext(ctx, c.method, c.urlWith(key, false), body)
	if err != nil {
		return nil, err
	}
	for _, h := range c.headersWith(key) {
		req.Header.Set(h.Name, h.Value)
	}
	return req, nil
}

// shown is the address to show, with the key hidden.
func (c *callSpec) shown(key string) string {
	if key == "" {
		return c.url
	}
	return c.urlWith("••••", true)
}

// buildRequest makes the HTTP request of an endpoint with the values of its variables,
// and gives the address to show, where a key in the query string is hidden.
func buildRequest(ctx context.Context, p *Provider, e *Endpoint, values map[string]string, key string) (*http.Request, string, error) {
	c, err := newCall(p, e, values)
	if err != nil {
		return nil, "", err
	}
	req, err := c.request(ctx, key)
	if err != nil {
		return nil, "", err
	}
	return req, c.shown(key), nil
}

// response is what came back from a call.
type response struct {
	status  string // e.g. "200 OK"
	code    int
	elapsed time.Duration
	header  http.Header
	body    []byte
	ext     string // file extension from the Content-Type, with the dot
	isText  bool   // the body can be shown as text
	lang    string // "JSON" or "XML" when the body is one of them
	shown   string // the body as shown: formatted for JSON and XML
}

// call sends the request and reads the whole response.
func call(req *http.Request) (*response, error) {
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	r := &response{
		status:  resp.Status,
		code:    resp.StatusCode,
		elapsed: time.Since(start),
		header:  resp.Header,
		body:    body,
	}
	r.ext = extension(resp.Header.Get("Content-Type"))
	return r, nil
}

// extension picks the file extension from the Content-Type of the response.
func extension(contentType string) string {
	t, _, _ := mime.ParseMediaType(contentType)
	switch {
	case t == "application/json" || strings.HasSuffix(t, "+json"):
		return ".json"
	case t == "application/xml" || t == "text/xml" || strings.HasSuffix(t, "+xml"):
		return ".xml"
	case t == "text/html":
		return ".html"
	case t == "text/csv":
		return ".csv"
	case t == "application/pdf":
		return ".pdf"
	case t == "image/png":
		return ".png"
	case t == "image/jpeg":
		return ".jpg"
	case t == "image/gif":
		return ".gif"
	case t == "image/svg+xml":
		return ".svg"
	case strings.HasPrefix(t, "text/") || t == "":
		return ".txt"
	}
	return ".bin"
}

// isText says whether the body can be shown: valid UTF-8 without control characters
// other than line breaks and tabs.
func isText(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

// size writes a number of bytes as B, KB or MB.
func size(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
}

// ---- Saving a response ----

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// fileName is provider-endpoint-YYYYMMDD-HHMM.ext, with spaces turned into "-" and
// the characters not allowed in file names left out. If such a file already exists
// in dir, -2, -3… is added.
func fileName(dir, provider, endpoint, ext string, now time.Time) string {
	clean := func(s string) string {
		s = unsafeChars.ReplaceAllString(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"), "")
		return strings.Trim(s, "-.")
	}
	base := clean(provider) + "-" + clean(endpoint) + "-" + now.Format("20060102-1504")
	name := base + ext
	for i := 2; exists(filepath.Join(dir, name)); i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	return name
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolvePath turns what the user typed into a full path: ~ is the user folder.
func resolvePath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	return filepath.Clean(p)
}

// displayPath shows a path with ~ in place of the user folder.
func displayPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + filepath.ToSlash(p[len(home):])
	}
	return p
}

// saveFile writes data to a new file: an existing file is never overwritten.
func saveFile(path string, data []byte) error {
	if !filepath.IsAbs(path) {
		return errors.New("write the full path, e.g. ~/Downloads/name.json")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return errors.New("a file with this name already exists: " + displayPath(path))
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
