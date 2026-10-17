package main

import (
	"strings"
	"testing"

	"gltools/internal/gui"
)

func TestCurl(t *testing.T) {
	p := &Provider{URL: "https://api.anthropic.com/v1", KeyIn: keyHeader, KeyName: "x-api-key",
		Headers: []Header{{"anthropic-version", "2023-06-01"}}}
	e := &Endpoint{Request: "POST /messages", Body: "{\n  \"content\": \"{prompt}\"\n}"}
	c, err := newCall(p, e, map[string]string{"prompt": "it's \"ok\""})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`curl -X POST 'https://api.anthropic.com/v1/messages' -H 'Content-Type: application/json' -H 'anthropic-version: 2023-06-01' -H 'x-api-key: <API_KEY>' -d '{"content":"it'\''s \"ok\""}'`,
		`curl.exe -X POST "https://api.anthropic.com/v1/messages" -H "Content-Type: application/json" -H "anthropic-version: 2023-06-01" -H "x-api-key: <API_KEY>" -d "{\"content\":\"it's \\"ok\\"\"}"`,
		`curl.exe -X POST 'https://api.anthropic.com/v1/messages' -H 'Content-Type: application/json' -H 'anthropic-version: 2023-06-01' -H 'x-api-key: <API_KEY>' -d '{"content":"it''s \"ok\""}'`,
	}
	for i, sh := range curlShells {
		got := c.curl(sh)
		if got != want[i] {
			t.Errorf("%s:\ngot  %s\nwant %s", sh.name, got, want[i])
		}
		if strings.Contains(got, "\n") {
			t.Errorf("%s: more than one line", sh.name)
		}
	}
}

func TestCurlKeyPlaces(t *testing.T) {
	e := &Endpoint{Request: "GET /f?city={city}"}
	values := map[string]string{"city": "Napoli"}
	bash := curlShells[0]
	for _, c := range []struct {
		p    *Provider
		want string
	}{
		{&Provider{URL: "https://a.com", KeyIn: keyQuery, KeyName: "apikey"}, `curl 'https://a.com/f?city=Napoli&apikey=<API_KEY>'`},
		{&Provider{URL: "https://a.com", KeyIn: keyBearer}, `curl 'https://a.com/f?city=Napoli' -H 'Authorization: Bearer <API_KEY>'`},
		{&Provider{URL: "https://a.com"}, `curl 'https://a.com/f?city=Napoli'`},
	} {
		call, _ := newCall(c.p, e, values)
		if got := call.curl(bash); got != c.want {
			t.Errorf("got  %s\nwant %s", got, c.want)
		}
	}
	call, _ := newCall(&Provider{URL: "https://a.com"}, &Endpoint{Request: "HEAD /"}, nil)
	if got := call.curl(bash); got != `curl -I 'https://a.com/'` {
		t.Errorf("head: %s", got)
	}
	call, _ = newCall(&Provider{URL: "https://a.com"}, &Endpoint{Request: "POST /", Body: "a\nb"}, nil)
	if got := call.curl(bash); got != `curl -X POST 'https://a.com/' -H 'Content-Type: application/json' -d 'a b'` {
		t.Errorf("text body: %s", got)
	}
}

func TestCopyCurlScreen(t *testing.T) {
	tm := newTestModel(t)
	tm.store.Providers = []*Provider{{Name: "p", URL: "http://127.0.0.1:1", KeyIn: keyHeader, KeyName: "X-Key",
		Endpoints: []*Endpoint{{Name: "e", Request: "GET /x"}}}}
	setKey("p", "secret")
	tm.Key(gui.KeyEnter)
	tm.Key(gui.KeyEnter)
	tm.wait()
	tm.Rune('u')
	tm.expect("copy as curl")
	tm.expect("Windows cmd")
	if strings.Contains(tm.screenText(), "secret") {
		t.Error("the key is on screen")
	}
	var copied string
	tm.copy = func(s string) { copied = s }
	tm.Rune('2')
	if copied != `curl.exe "http://127.0.0.1:1/x" -H "X-Key: <API_KEY>"` {
		t.Errorf("copied %s", copied)
	}
	if tm.screen != screenResponse {
		t.Errorf("screen %d", tm.screen)
	}
	tm.expect("Copied: curl for Windows cmd")
}
