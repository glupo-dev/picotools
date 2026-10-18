package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseRequest(t *testing.T) {
	for _, c := range []struct{ in, method, path, err string }{
		{"GET /users/{id}", "GET", "/users/{id}", ""},
		{"post   /alerts", "POST", "/alerts", ""},
		{"GET", "GET", "/", ""},
		{"FETCH /x", "", "", "start with the method"},
		{"GET users", "", "", "starts with /"},
	} {
		method, path, err := parseRequest(c.in)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%q: error %v, want %q", c.in, err, c.err)
			}
			continue
		}
		if err != nil || method != c.method || path != c.path {
			t.Errorf("%q: got %q %q %v", c.in, method, path, err)
		}
	}
}

func TestVariables(t *testing.T) {
	e := &Endpoint{Request: "GET /a/{id}?lang={lang?}&id={id}", Body: `{"q": "{prompt}", "l": "{lang}"}`}
	got := variables(e)
	want := []variable{{"id", false}, {"lang", false}, {"prompt", false}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("variable %d: got %v, want %v", i, got[i], want[i])
		}
	}
	if v := variables(&Endpoint{Request: "GET /x?lang={lang?}"}); !v[0].optional {
		t.Errorf("lang should be optional: %v", v)
	}
}

func TestFillPath(t *testing.T) {
	for _, c := range []struct {
		path   string
		values map[string]string
		want   string
	}{
		{"/forecast?city={city}&lang={lang?}", map[string]string{"city": "San Marco"}, "/forecast?city=San+Marco"},
		{"/forecast?city={city}&lang={lang?}", map[string]string{"city": "Roma", "lang": "it"}, "/forecast?city=Roma&lang=it"},
		{"/x?lang={lang?}", nil, "/x"},
		{"/users/{id}/posts", map[string]string{"id": "a/b"}, "/users/a%2Fb/posts"},
		{"/items/{id?}", nil, "/items/"},
	} {
		if got := fillPath(c.path, c.values); got != c.want {
			t.Errorf("%s: got %s, want %s", c.path, got, c.want)
		}
	}
}

func TestBuildRequest(t *testing.T) {
	p := &Provider{Name: "p", URL: "https://api.example.com/v1/", KeyIn: keyHeader, KeyName: "X-API-Key",
		Headers: []Header{{"anthropic-version", "2023-06-01"}}}
	e := &Endpoint{Request: "POST /messages", Body: `{"content": "{prompt}"}`}
	req, shown, err := buildRequest(context.Background(), p, e, map[string]string{"prompt": "say \"hi\"\nplease"}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.String() != "https://api.example.com/v1/messages" || shown != req.URL.String() {
		t.Errorf("url %s, shown %s", req.URL, shown)
	}
	if req.Header.Get("X-API-Key") != "secret" || req.Header.Get("anthropic-version") != "2023-06-01" || req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers %v", req.Header)
	}
	body, _ := io.ReadAll(req.Body)
	if string(body) != `{"content": "say \"hi\"\nplease"}` {
		t.Errorf("body %s", body)
	}

	p.KeyIn = keyBearer
	req, _, _ = buildRequest(context.Background(), p, &Endpoint{Request: "GET /"}, nil, "tok")
	if req.Header.Get("Authorization") != "Bearer tok" || req.Header.Get("Content-Type") != "" {
		t.Errorf("bearer: %v", req.Header)
	}

	p.KeyIn, p.KeyName = keyQuery, "apikey"
	req, shown, _ = buildRequest(context.Background(), p, &Endpoint{Request: "GET /f?city={city}"}, map[string]string{"city": "Napoli"}, "k&1")
	if req.URL.Query().Get("apikey") != "k&1" || req.URL.Query().Get("city") != "Napoli" {
		t.Errorf("query key: %s", req.URL)
	}
	if strings.Contains(shown, "k&1") || strings.Contains(shown, "k%261") || !strings.HasSuffix(shown, "?city=Napoli&apikey=••••") {
		t.Errorf("shown should hide the key: %s", shown)
	}
	_, shown, _ = buildRequest(context.Background(), p, &Endpoint{Request: "GET /f"}, nil, "k")
	if !strings.HasSuffix(shown, "/v1/f?apikey=••••") {
		t.Errorf("shown without query: %s", shown)
	}
}

func TestExtension(t *testing.T) {
	for in, want := range map[string]string{
		"application/json; charset=utf-8": ".json",
		"application/problem+json":        ".json",
		"text/xml":                        ".xml",
		"text/html":                       ".html",
		"":                                ".txt",
		"image/png":                       ".png",
		"application/octet-stream":        ".bin",
	} {
		if got := extension(in); got != want {
			t.Errorf("%q: got %s, want %s", in, got, want)
		}
	}
}

func TestFileName(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 17, 15, 30, 0, 0, time.Local)
	name := fileName(dir, "meteo test", "città/forecast?", ".json", now)
	if name != "meteo-test-cittforecast-20261017-1530.json" {
		t.Errorf("got %s", name)
	}
	os.WriteFile(filepath.Join(dir, name), nil, 0o644)
	if got := fileName(dir, "meteo test", "città/forecast?", ".json", now); got != "meteo-test-cittforecast-20261017-1530-2.json" {
		t.Errorf("second: got %s", got)
	}
}

func TestSaveFileNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "out.json")
	if err := saveFile(path, []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := saveFile(path, []byte("2")); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("got %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "1" {
		t.Errorf("overwritten: %s", data)
	}
}

func TestRemember(t *testing.T) {
	e := &Endpoint{}
	for _, v := range []string{"a", "b", "a", "", "c"} {
		e.remember("x", v)
	}
	if got := strings.Join(e.Values["x"], ","); got != "c,a,b" {
		t.Errorf("got %s", got)
	}
	for i := range 20 {
		e.remember("x", string(rune('d'+i)))
	}
	if len(e.Values["x"]) != maxValues {
		t.Errorf("kept %d values", len(e.Values["x"]))
	}
}
