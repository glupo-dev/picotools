package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"picotools/internal/gui"
)

// fakeGitHub answers the few calls of Pico Auth like GitHub does.
type fakeGitHub struct {
	mu      sync.Mutex
	srv     *httptest.Server
	repos   []string
	files   map[string][]byte // "owner/name/path" → content
	polls   int
	access  string
	refresh int
}

func newFakeGitHub(t *testing.T, repos ...string) *fakeGitHub {
	f := &fakeGitHub{repos: repos, files: map[string][]byte{}, access: "t1"}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := func(code int, v any) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	r.ParseForm()
	switch {
	case r.URL.Path == "/login/device/code":
		out(200, map[string]any{"device_code": "dc", "user_code": "ABCD-1234", "verification_uri": "https://github.com/login/device", "interval": 0, "expires_in": 900})
		return
	case r.URL.Path == "/login/oauth/access_token" && r.Form.Get("grant_type") == "refresh_token":
		f.refresh++
		f.access = "t2"
		out(200, map[string]any{"access_token": "t2", "expires_in": 28800, "refresh_token": "r2", "refresh_token_expires_in": 15897600})
		return
	case r.URL.Path == "/login/oauth/access_token":
		f.polls++
		if f.polls == 1 {
			out(200, map[string]any{"error": "authorization_pending"})
			return
		}
		out(200, map[string]any{"access_token": "t1", "expires_in": 28800, "refresh_token": "r1", "refresh_token_expires_in": 15897600})
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+f.access {
		out(401, map[string]any{"message": "Bad credentials"})
		return
	}
	switch {
	case r.URL.Path == "/user/installations":
		out(200, map[string]any{"installations": []map[string]any{{"id": 7}}})
	case r.URL.Path == "/user/installations/7/repositories":
		var rs []map[string]any
		for _, n := range f.repos {
			rs = append(rs, map[string]any{"full_name": n})
		}
		out(200, map[string]any{"repositories": rs})
	case strings.HasPrefix(r.URL.Path, "/repos/"):
		key := strings.Replace(strings.TrimPrefix(r.URL.Path, "/repos/"), "/contents/", "/", 1)
		data, ok := f.files[key]
		sha := fmt.Sprintf("sha%d", len(data))
		if r.Method == "GET" {
			if !ok {
				out(404, map[string]any{"message": "Not Found"})
				return
			}
			out(200, map[string]any{"content": base64.StdEncoding.EncodeToString(data), "encoding": "base64", "sha": sha})
			return
		}
		var body struct{ Content, SHA string }
		json.NewDecoder(r.Body).Decode(&body)
		if ok && body.SHA != sha || !ok && body.SHA != "" {
			out(409, map[string]any{"message": "conflict"})
			return
		}
		f.files[key], _ = base64.StdEncoding.DecodeString(body.Content)
		out(201, map[string]any{"content": map[string]any{"sha": fmt.Sprintf("sha%d", len(f.files[key]))}})
	default:
		out(404, map[string]any{"message": "Not Found"})
	}
}

func (f *fakeGitHub) file(key string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[key]
}

// syncModel is a test model talking to the fake GitHub, with the user interface calls queued.
type syncModel struct {
	*testModel
	ui     chan func()
	opened string
}

func newSyncModel(t *testing.T, dir string, f *fakeGitHub) *syncModel {
	tm := newTestModel(t, dir)
	sm := &syncModel{testModel: tm, ui: make(chan func(), 100)}
	tm.ui = func(fn func()) { sm.ui <- fn }
	tm.openURL = func(u string) { sm.opened = u }
	tm.gh.web, tm.gh.api = f.srv.URL, f.srv.URL
	return sm
}

// until runs the queued user interface calls until ok, or fails after a while.
func (sm *syncModel) until(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !ok() {
		select {
		case fn := <-sm.ui:
			fn()
		case <-deadline:
			t.Fatal("timed out:\n" + sm.text())
		}
	}
}

func (sm *syncModel) newVault(pw string) {
	sm.Rune('1')
	sm.typ(pw)
	sm.typ(pw)
}

func TestLinkToGitHub(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	dir := t.TempDir()
	sm := newSyncModel(t, dir, f)
	sm.newVault("master-pw")
	sm.vault.Accounts = []*Account{{Kind: kindTOTP, Name: "GitHub", User: "me", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30}}
	sm.vault.save()

	sm.Rune('s')
	if sm.screen != screenSettings || !strings.Contains(sm.text(), "Sync with GitHub") {
		t.Fatal(sm.text())
	}
	sm.Rune('1')
	sm.until(t, func() bool { return sm.sync.code != "" })
	if !strings.Contains(sm.text(), "ABCD-1234") || sm.opened != "https://github.com/login/device" {
		t.Fatalf("opened %q\n%s", sm.opened, sm.text())
	}
	sm.Key(gui.KeyEnter)
	if sm.clip != "ABCD-1234" || !strings.Contains(sm.text(), "Code copied") {
		t.Fatalf("clip %q\n%s", sm.clip, sm.text())
	}
	sm.until(t, func() bool { return !sm.sync.busy() })
	if sm.sync.done == "" {
		t.Fatal(sm.text())
	}

	// The same file on GitHub and here, version 2, that needs the second key.
	up := f.file("genlupo/picoauth-vault/" + vaultFile)
	local, _ := os.ReadFile(sm.path)
	if len(up) == 0 || string(up) != string(local) {
		t.Fatal("the file on GitHub is not the local one")
	}
	var env envelope
	json.Unmarshal(up, &env)
	if env.Version != 2 {
		t.Fatalf("version %d", env.Version)
	}
	if _, err := decryptVault(up, "", "master-pw", nil); !errors.Is(err, errNoSecondKey) {
		t.Fatalf("opened without the second key: %v", err)
	}
	other, _ := newSecond()
	if _, err := decryptVault(up, "", "master-pw", other); !errors.Is(err, errWrongPassword) {
		t.Fatalf("opened with another second key: %v", err)
	}
	second, _ := savedSecond()
	v, err := decryptVault(up, "", "master-pw", second)
	if err != nil || len(v.Accounts) != 1 {
		t.Fatalf("open with both: %v", err)
	}
	if l := loadLink(sm.path); l == nil || l.Repo != "genlupo/picoauth-vault" || l.SHA == "" {
		t.Fatalf("link %+v", l)
	}

	// A change after the link is saved with the second key.
	sm.Key(gui.KeyEsc)
	if !strings.Contains(sm.text(), "Synced with GitHub: genlupo/picoauth-vault") {
		t.Fatal(sm.text())
	}
	sm.Key(gui.KeyEsc)
	sm.vault.Accounts[0].Name = "GitHub work"
	sm.saveVault()

	// Next start opens by itself with the keychain, and remembers the link.
	tm := newTestModel(t, dir)
	if tm.screen != screenList || tm.vault.Accounts[0].Name != "GitHub work" || tm.link == nil {
		t.Fatalf("screen %v\n%s", tm.screen, tm.text())
	}

	// Without the second key in the keychain it cannot be opened.
	keyring.Delete(keyringService, keyringSecond)
	tm = newTestModel(t, dir)
	if tm.screen != screenFatal || !strings.Contains(tm.text(), "second key") {
		t.Fatal(tm.text())
	}
}

func TestLinkRefused(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	f.files["genlupo/picoauth-vault/"+vaultFile] = []byte("someone else's vault")
	sm := newSyncModel(t, t.TempDir(), f)
	sm.newVault("master-pw")
	before, _ := os.ReadFile(sm.path)

	sm.Rune('s')
	sm.Rune('1')
	sm.until(t, func() bool { return !sm.sync.busy() })
	if !strings.Contains(sm.sync.err, "already holds a vault") {
		t.Fatal(sm.text())
	}
	after, _ := os.ReadFile(sm.path)
	if string(before) != string(after) || loadLink(sm.path) != nil || string(f.file("genlupo/picoauth-vault/"+vaultFile)) != "someone else's vault" {
		t.Fatal("something changed")
	}
	sm.Key(gui.KeyEsc)
	if sm.screen != screenSettings {
		t.Fatal(sm.screen)
	}
}

func TestLinkChooseRepository(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/other", "genlupo/picoauth-vault")
	sm := newSyncModel(t, t.TempDir(), f)
	sm.newVault("master-pw")

	sm.Rune('s')
	sm.Rune('1')
	sm.until(t, func() bool { return sm.sync.repos != nil })
	if !strings.Contains(sm.text(), "2  genlupo/picoauth-vault") {
		t.Fatal(sm.text())
	}
	sm.Rune('2')
	sm.until(t, func() bool { return !sm.sync.busy() })
	if sm.sync.done == "" || len(f.file("genlupo/picoauth-vault/"+vaultFile)) == 0 || len(f.file("genlupo/other/"+vaultFile)) != 0 {
		t.Fatal(sm.text())
	}
}

func TestLinkNoRepository(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t)
	sm := newSyncModel(t, t.TempDir(), f)
	sm.newVault("master-pw")
	sm.Rune('s')
	sm.Rune('1')
	sm.until(t, func() bool { return !sm.sync.busy() })
	if !strings.Contains(sm.sync.err, "cannot reach any repository") {
		t.Fatal(sm.text())
	}
}

func TestAccessRenewed(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	g := newGitHub()
	g.web, g.api = f.srv.URL, f.srv.URL
	now := time.Unix(1700000000, 0)
	g.now = func() time.Time { return now }
	if _, err := g.repos(t.Context()); !errors.Is(err, errLoginNeeded) {
		t.Fatalf("no login yet: %v", err)
	}
	d, err := g.startLogin(t.Context())
	if err == nil {
		err = g.waitLogin(t.Context(), d)
	}
	if err != nil {
		t.Fatal(err)
	}

	// After 9 hours the access expired: it is renewed by itself, and kept in the keychain.
	now = now.Add(9 * time.Hour)
	f.mu.Lock()
	f.access = "t2" // GitHub no longer accepts t1
	f.mu.Unlock()
	if rs, err := g.repos(t.Context()); err != nil || len(rs) != 1 || f.refresh != 1 {
		t.Fatalf("%v %v refresh %d", rs, err, f.refresh)
	}
	if tok, _ := savedToken(); tok == nil || tok.Access != "t2" || tok.Refresh != "r2" {
		t.Fatalf("keychain %+v", tok)
	}
}
