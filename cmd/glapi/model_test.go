package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"gltools/internal/gui"
)

// testModel is a model with an empty store in a temporary folder and a fake keychain.
// The work it would do in the user interface goes through uiCh, run by wait.
type testModel struct {
	*model
	t    *testing.T
	uiCh chan func()
}

func newTestModel(t *testing.T) *testModel {
	keyring.MockInit()
	dir := t.TempDir()
	s, err := loadStore(filepath.Join(dir, "glapi", "providers.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(s)
	m.downloads = filepath.Join(dir, "Downloads")
	m.rows, m.cols = 40, 120
	tm := &testModel{model: m, t: t, uiCh: make(chan func(), 1000)}
	m.ui = func(f func()) { tm.uiCh <- f }
	return tm
}

// typ types the text and presses enter.
func (tm *testModel) typ(text string) {
	for _, r := range text {
		tm.Rune(r)
	}
	tm.Key(gui.KeyEnter)
}

// wait runs the work sent to the user interface until the request is over.
func (tm *testModel) wait() {
	deadline := time.After(5 * time.Second)
	for tm.screen == screenBusy {
		select {
		case f := <-tm.uiCh:
			f()
		case <-deadline:
			tm.t.Fatal("no response")
		}
	}
}

// screenText is what the window shows, as plain text.
func (tm *testModel) screenText() string {
	var b strings.Builder
	for _, l := range tm.Render(tm.cols, tm.rows) {
		for _, s := range l {
			b.WriteString(s.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (tm *testModel) expect(want string) {
	tm.t.Helper()
	if text := tm.screenText(); !strings.Contains(text, want) {
		tm.t.Fatalf("screen should show %q:\n%s", want, text)
	}
}

func TestFullFlow(t *testing.T) {
	var got *http.Request
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"city":"` + r.URL.Query().Get("city") + `","temp":21}`))
	}))
	defer srv.Close()

	tm := newTestModel(t)
	tm.expect("No providers yet")

	// New provider with a key in a header and a fixed header.
	tm.Rune('n')
	tm.typ("meteo-test")
	tm.typ("ftp://wrong")
	tm.expect("Write the full address")
	for range len("ftp://wrong") {
		tm.Key(gui.KeyBackspace)
	}
	tm.typ(srv.URL + "/v1/")
	tm.Key(gui.KeyEnter) // 1 header is proposed
	tm.typ("X-API-Key")
	tm.typ("secret123")
	tm.expect("Fixed headers")
	tm.typ("Accept json") // wrong
	tm.expect("Write it as name: value")
	for range len("Accept json") {
		tm.Key(gui.KeyBackspace)
	}
	tm.typ("Accept: application/json")
	tm.Key(gui.KeyEnter) // empty line: done
	if tm.screen != screenProviders {
		t.Fatalf("screen %d", tm.screen)
	}
	tm.expect("Saved meteo-test")
	tm.expect("key · 0 endpoints")

	// The key is in the keychain, not in the file.
	if k, _ := keyring.Get(keyringService, "meteo-test"); k != "secret123" {
		t.Errorf("key in keychain: %q", k)
	}
	data, _ := os.ReadFile(tm.store.path)
	if strings.Contains(string(data), "secret123") {
		t.Errorf("the key is in the file:\n%s", data)
	}
	if info, _ := os.Stat(tm.store.path); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v", info.Mode().Perm())
	}

	// New endpoint with a required and an optional variable.
	tm.Key(gui.KeyEnter)
	tm.expect("No endpoints yet")
	tm.Rune('n')
	tm.typ("city forecast")
	tm.typ("get /forecast?city={city}&lang={lang?}")
	if tm.screen != screenEndpoints {
		t.Fatalf("GET should not ask for a body, screen %d", tm.screen)
	}
	tm.expect("city forecast   GET /forecast?city={city}&lang={lang?}")

	// Run it.
	tm.Key(gui.KeyEnter)
	tm.expect("city:")
	tm.Key(gui.KeyEnter)
	tm.expect("This one cannot be empty")
	tm.typ("Napoli")
	tm.expect("lang (optional):")
	tm.Key(gui.KeyEnter)
	tm.wait()
	if got.URL.String() != "/v1/forecast?city=Napoli" {
		t.Errorf("request %s", got.URL)
	}
	if got.Header.Get("X-API-Key") != "secret123" || got.Header.Get("Accept") != "application/json" {
		t.Errorf("headers %v", got.Header)
	}
	tm.expect("200 OK")
	tm.expect(`"city": "Napoli",`) // formatted
	if strings.Contains(tm.screenText(), "secret123") {
		t.Errorf("the key is on screen")
	}

	// Copy and headers.
	var copied string
	tm.copy = func(s string) { copied = s }
	tm.Rune('c')
	if !strings.Contains(copied, "\"temp\": 21") {
		t.Errorf("copied %q", copied)
	}
	tm.Rune('h')
	tm.expect("Content-Type: application/json")
	tm.Key(gui.KeyEsc)
	tm.expect(`"city": "Napoli",`)

	// Save: proposed name in Downloads, formatted content.
	tm.Rune('s')
	tm.expect("meteo-test-city-forecast-")
	tm.Key(gui.KeyEnter)
	tm.expect("✔ Saved")
	files, _ := filepath.Glob(filepath.Join(tm.downloads, "meteo-test-city-forecast-*.json"))
	if len(files) != 1 {
		t.Fatalf("saved files %v", files)
	}
	saved, _ := os.ReadFile(files[0])
	if !strings.Contains(string(saved), "\n  \"temp\": 21\n") {
		t.Errorf("saved %q", saved)
	}
	if tm.store.SaveDir != tm.downloads {
		t.Errorf("save dir %q", tm.store.SaveDir)
	}

	// Back, run again: the last value is proposed, and typing replaces it.
	tm.Key(gui.KeyEsc)
	tm.Rune('1')
	tm.expect("↑/↓ earlier values (1)")
	tm.expect("Napoli")
	tm.typ("Roma")
	tm.Key(gui.KeyEnter)
	tm.wait()
	if got.URL.Query().Get("city") != "Roma" {
		t.Errorf("request %s", got.URL)
	}
	tm.Key(gui.KeyEsc)
	tm.Rune('1')
	tm.Key(gui.KeyUp) // older value
	tm.Key(gui.KeyEnter)
	tm.Key(gui.KeyEnter)
	tm.wait()
	if got.URL.Query().Get("city") != "Napoli" {
		t.Errorf("history: request %s", got.URL)
	}
	tm.Key(gui.KeyEsc)

	// A POST endpoint with a body variable, escaped for JSON.
	tm.Rune('n')
	tm.typ("ask")
	tm.typ("POST /messages")
	tm.Paste("{\"content\": \"{prompt}\"}\n")
	tm.Key(gui.KeyEnter)
	tm.Rune('2')
	tm.Paste("line one\nsay \"hi\"")
	tm.Key(gui.KeyEnter)
	tm.wait()
	if gotBody != `{"content": "line one\nsay \"hi\""}` {
		t.Errorf("body %s", gotBody)
	}
	if got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("content type %q", got.Header.Get("Content-Type"))
	}
	tm.Key(gui.KeyEsc)

	// The file holds what was set up.
	var s Store
	data, _ = os.ReadFile(tm.store.path)
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	p := s.Providers[0]
	if p.KeyIn != keyHeader || p.KeyName != "X-API-Key" || len(p.Endpoints) != 2 || p.Endpoints[0].Values["city"][0] != "Napoli" {
		t.Errorf("stored %+v", p)
	}

	// Rename the provider: the key moves with it.
	tm.Key(gui.KeyEsc)
	tm.Rune('m')
	tm.expect("header X-API-Key · ••••••••")
	tm.Rune('1')
	tm.Key(gui.KeyBackspace) // the proposed name is cleared at once
	tm.typ("meteo")
	if k, _ := keyring.Get(keyringService, "meteo"); k != "secret123" {
		t.Errorf("key after rename: %q", k)
	}
	if _, err := keyring.Get(keyringService, "meteo-test"); err == nil {
		t.Errorf("old key still there")
	}
	tm.Key(gui.KeyEsc)

	// Delete it: asks first, then the key goes too.
	tm.Rune('d')
	tm.expect("Delete the provider meteo")
	tm.Rune('x')
	if len(tm.store.Providers) != 1 {
		t.Fatal("deleted without y")
	}
	tm.Rune('d')
	tm.Rune('y')
	if len(tm.store.Providers) != 0 {
		t.Fatal("not deleted")
	}
	if _, err := keyring.Get(keyringService, "meteo"); err == nil {
		t.Errorf("key still there")
	}
}

func TestQueryKeyAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != "abc" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("<error><msg>no key</msg></error>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{0x89, 'P', 'N', 'G', 0, 1, 2})
	}))
	defer srv.Close()

	tm := newTestModel(t)
	tm.Rune('n')
	tm.typ("img")
	tm.typ(srv.URL)
	tm.Key(gui.KeyBackspace)
	tm.typ("3")
	tm.typ("apikey")
	tm.typ("wrong")
	tm.Key(gui.KeyEnter) // no fixed headers
	tm.Key(gui.KeyEnter)
	tm.Rune('n')
	tm.typ("logo")
	tm.typ("GET /logo")
	tm.Key(gui.KeyEnter)
	tm.wait()
	tm.expect("401 Unauthorized")
	tm.expect("<msg>no key</msg>") // XML formatted on its own line
	tm.expect("apikey=••••")
	if strings.Contains(tm.screenText(), "wrong") {
		t.Errorf("the key is on screen")
	}

	// Fix the key from the edit screen.
	tm.Key(gui.KeyEsc)
	tm.Key(gui.KeyEsc)
	tm.Rune('m')
	tm.Rune('3')
	tm.Key(gui.KeyEnter) // keep 3 query
	tm.Key(gui.KeyEnter) // keep apikey
	tm.expect("Key (empty keeps the saved one) (optional):")
	tm.typ("abc")
	tm.Key(gui.KeyEsc)
	tm.Key(gui.KeyEnter)
	tm.Key(gui.KeyEnter)
	tm.wait()
	tm.expect("200 OK")
	tm.expect("The body is not text")
	tm.Rune('s')
	tm.Key(gui.KeyEnter)
	files, _ := filepath.Glob(filepath.Join(tm.downloads, "img-logo-*.png"))
	if len(files) != 1 {
		t.Fatalf("saved %v", files)
	}
	if data, _ := os.ReadFile(files[0]); len(data) != 7 {
		t.Errorf("saved %d bytes", len(data))
	}
}

func TestCancelAndUnreachable(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)

	tm := newTestModel(t)
	tm.store.Providers = []*Provider{{Name: "slow", URL: srv.URL, Endpoints: []*Endpoint{{Name: "wait", Request: "GET /"}}}}
	tm.Key(gui.KeyEnter)
	tm.Key(gui.KeyEnter)
	if tm.screen != screenBusy {
		t.Fatalf("screen %d", tm.screen)
	}
	tm.expect("Waiting for the response")
	tm.Key(gui.KeyEsc)
	if tm.screen != screenEndpoints {
		t.Fatalf("esc: screen %d", tm.screen)
	}

	tm.prov.URL = "http://127.0.0.1:1"
	tm.Key(gui.KeyEnter)
	tm.wait()
	tm.expect("✘")
	tm.expect("connection refused")
}
