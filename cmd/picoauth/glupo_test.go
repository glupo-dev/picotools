package main

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"picotools/internal/gui"
)

// fakeGlupo is a picoauth-server in memory, with the same API (picoauth-server, src/web.rs).
type fakeGlupo struct {
	srv *httptest.Server

	mu      sync.Mutex
	logins  map[string][]byte    // user → login_hash
	invites map[string]bool      // code → used
	tokens  map[string]string    // token → user
	files   map[string][2]string // user/path → content (base64), version
	n       int                  // for tokens and versions
	failPut int                  // the next PUTs that fail with 500
	calls   map[string]int       // "POST /login" → how many
}

func newFakeGlupo(t *testing.T, invites ...string) *fakeGlupo {
	f := &fakeGlupo{logins: map[string][]byte{}, invites: map[string]bool{}, tokens: map[string]string{},
		files: map[string][2]string{}, calls: map[string]int{}}
	for _, c := range invites {
		f.invites[c] = false
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGlupo) next() string { f.n++; return fmt.Sprintf("v%d", f.n) }

func (f *fakeGlupo) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[r.Method+" "+r.URL.Path]++
	fail := func(code int, e string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": e})
	}
	var body map[string]string
	json.NewDecoder(r.Body).Decode(&body)
	invite := strings.ToUpper(strings.ReplaceAll(body["invite_code"], "-", ""))
	switch r.URL.Path {
	case "/check", "/accounts":
		used, ok := f.invites[invite]
		switch {
		case !usernameRe.MatchString(body["username"]):
			fail(400, "invalid_username")
		case !ok || used:
			fail(403, "invite_not_valid")
		case f.logins[body["username"]] != nil:
			fail(409, "username_taken")
		case r.URL.Path == "/check":
			w.Write([]byte("{}"))
		default:
			h, _ := base64.StdEncoding.DecodeString(body["login_hash"])
			f.logins[body["username"]], f.invites[invite] = h, true
			tok := "tok-" + f.next()
			f.tokens[tok] = body["username"]
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"token": tok})
		}
		return
	case "/login":
		h, _ := base64.StdEncoding.DecodeString(body["login_hash"])
		want := f.logins[body["username"]]
		if want == nil || subtle.ConstantTimeCompare(h, want) != 1 {
			fail(401, "wrong_login")
			return
		}
		tok := "tok-" + f.next()
		f.tokens[tok] = body["username"]
		json.NewEncoder(w).Encode(map[string]string{"token": tok})
		return
	}
	user, ok := f.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	if !ok {
		fail(401, "unauthorized")
		return
	}
	switch {
	case r.URL.Path == "/logout":
		delete(f.tokens, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		w.WriteHeader(204)
	case r.URL.Path == "/list":
		dir := r.URL.Query().Get("dir")
		type entry struct {
			Name, Path, Version, Type string
		}
		seen, out := map[string]bool{}, []map[string]string{}
		for k, v := range f.files {
			p, _ := strings.CutPrefix(k, user+"/")
			if !strings.HasPrefix(k, user+"/") {
				continue
			}
			d, name, nested := strings.Cut(p, "/")
			switch {
			case dir == "" && !nested:
				out = append(out, map[string]string{"name": p, "path": p, "version": v[1], "type": "file"})
			case dir == "" && nested && !seen[d]:
				seen[d] = true
				out = append(out, map[string]string{"name": d, "path": d, "version": "", "type": "dir"})
			case nested && d == dir:
				out = append(out, map[string]string{"name": name, "path": p, "version": v[1], "type": "file"})
			}
		}
		sort.Slice(out, func(a, b int) bool { return out[a]["path"] < out[b]["path"] })
		json.NewEncoder(w).Encode(out)
	case strings.HasPrefix(r.URL.Path, "/files/"):
		key := user + "/" + strings.TrimPrefix(r.URL.Path, "/files/")
		cur, exists := f.files[key]
		switch r.Method {
		case "GET":
			if !exists {
				fail(404, "not_found")
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"content": cur[0], "version": cur[1]})
		case "PUT":
			if f.failPut > 0 {
				f.failPut--
				fail(500, "internal")
				return
			}
			if (body["version"] == "" && exists) || (body["version"] != "" && (!exists || cur[1] != body["version"])) {
				fail(409, "conflict")
				return
			}
			v := f.next()
			f.files[key] = [2]string{body["content"], v}
			json.NewEncoder(w).Encode(map[string]string{"version": v})
		case "DELETE":
			switch {
			case !exists:
				fail(404, "not_found")
			case cur[1] != r.URL.Query().Get("version"):
				fail(409, "conflict")
			default:
				delete(f.files, key)
				w.WriteHeader(204)
			}
		}
	}
}

func (f *fakeGlupo) file(key string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := base64.StdEncoding.DecodeString(f.files[key][0])
	return b
}

func (f *fakeGlupo) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k := range f.files {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeGlupo) revokeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = map[string]string{}
}

func (f *fakeGlupo) tokenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tokens)
}

func newGlupoModel(t *testing.T, dir string, f *fakeGlupo) *syncModel {
	sm := newSyncModel(t, dir, newFakeGitHub(t))
	sm.gl.api = f.srv.URL
	return sm
}

func (sm *syncModel) withAccount() {
	sm.vault.Accounts = []*Account{{Kind: kindTOTP, Name: "GitHub", User: "me", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30}}
	sm.vault.save()
}

// linkGlupo goes through Settings › Sync with glupo.dev with a user name and an invite code.
func (sm *syncModel) linkGlupo(t *testing.T, user, invite string) {
	t.Helper()
	sm.Rune('s')
	sm.Rune('2')
	sm.typ(user)
	sm.typ(invite)
}

func TestLinkToGlupo(t *testing.T) {
	keyring.MockInit()
	f := newFakeGlupo(t, "K7QM3XPA")
	sm := newGlupoModel(t, t.TempDir(), f)
	sm.newVault("master-pw")
	sm.withAccount()

	sm.Rune('s')
	if !strings.Contains(sm.text(), "2  Sync with glupo.dev") || !strings.Contains(sm.text(), "3  Delete this vault") {
		t.Fatal(sm.text())
	}
	sm.Key(gui.KeyEsc)

	// A bad user name is refused on the form, without calling the server.
	sm.linkGlupo(t, "ma", "K7QM-3XPA")
	if sm.screen != screenForm || !strings.Contains(sm.text(), "3 to 32 characters") {
		t.Fatal(sm.text())
	}
	sm.Key(gui.KeyEsc)
	sm.Key(gui.KeyEsc)
	sm.Key(gui.KeyEsc)

	// A wrong invite: back on the form with the message and the values.
	sm.linkGlupo(t, "Mario", "AAAA-BBBB")
	sm.until(t, func() bool { return sm.screen == screenForm && sm.form.err != "" })
	if !strings.Contains(sm.text(), "Invite code not valid or already used") || f.logins["mario"] != nil {
		t.Fatal(sm.text())
	}
	sm.Key(gui.KeyEnter) // keeps "mario"
	sm.Rune('k')         // replaces the invite
	sm.typ("7qm-3xpa")
	sm.until(t, func() bool { return sm.screen == screenSync && !sm.sync.busy() })
	if !strings.Contains(sm.sync.done, "glupo.dev, in the account mario") {
		t.Fatal(sm.text())
	}

	// The same file on glupo.dev and here; the link and the device file.
	up := f.file("mario/" + vaultFile)
	local, _ := os.ReadFile(sm.path)
	if len(up) == 0 || string(up) != string(local) {
		t.Fatal("the file on glupo.dev is not the local one")
	}
	if sm.link.Repo != "glupo.dev/mario" || sm.link.SHA == "" {
		t.Fatalf("%+v", sm.link)
	}
	sm.until(t, func() bool { return sm.link.Computer != "" })
	if f.count("mario/computers/") != 1 {
		t.Fatal("no device file")
	}
	sm.Key(gui.KeyEsc)
	sm.Rune('s')
	if !strings.Contains(sm.text(), "Synced with glupo.dev: mario") || !strings.Contains(sm.text(), "The vault stays on glupo.dev.") {
		t.Fatal(sm.text())
	}
	sm.Key(gui.KeyEsc)

	// A change goes to glupo.dev at once.
	before := sm.link.SHA
	sm.Rune('m')
	sm.typ("Renamed")
	sm.typ("")
	sm.until(t, func() bool { return !sm.saving && sm.link.SHA != before })
	if sm.vault.Accounts[0].Name != "Renamed" || string(f.file("mario/"+vaultFile)) == string(up) {
		t.Fatal(sm.text())
	}

	// The author revokes the tokens: the device logs in again by itself with its master password.
	f.revokeAll()
	before = sm.link.SHA
	sm.Rune('m')
	sm.typ("Again")
	sm.typ("")
	sm.until(t, func() bool { return !sm.saving && sm.link.SHA != before })
	if sm.vault.Accounts[0].Name != "Again" {
		t.Fatal(sm.text())
	}

	// Without the server: nothing is changed, and the message names glupo.dev.
	api := sm.gl.api
	sm.gl.api = "http://127.0.0.1:1"
	sm.Rune('d')
	sm.Rune('y')
	sm.until(t, func() bool { return !sm.saving && sm.status != nil })
	if !strings.Contains(sm.text(), "Cannot reach glupo.dev: the change was not saved. Try again later.") || len(sm.vault.Accounts) != 1 {
		t.Fatal(sm.text())
	}
	sm.gl.api = api

	// Reset this device: the device file goes, the token is revoked, nothing stays here.
	tokens := f.tokenCount()
	sm.Rune('s')
	sm.Rune('1')
	sm.until(t, func() bool { return sm.screen == screenForm })
	if !strings.Contains(sm.text(), "nobody can open the vault on glupo.dev") {
		t.Fatal(sm.text())
	}
	sm.typ("delete")
	sm.typ("master-pw")
	sm.until(t, func() bool { return sm.screen == screenStart })
	if !strings.Contains(sm.text(), "the vault is still on glupo.dev") || f.count("mario/computers/") != 0 {
		t.Fatal(sm.text())
	}
	if f.tokenCount() != tokens-1 || f.count("mario/"+vaultFile) != 1 {
		t.Fatalf("tokens %d → %d", tokens, f.tokenCount())
	}
	if tok, _ := keyring.Get(keyringService, keyringGlupo); tok != "" {
		t.Fatal("the token is still in the keychain")
	}
}

// A link that stopped halfway (account created, upload failed) is finished by doing it again:
// the invite is used, but the login with the master password finds the account.
func TestLinkToGlupoAgain(t *testing.T) {
	keyring.MockInit()
	f := newFakeGlupo(t, "K7QM3XPA")
	sm := newGlupoModel(t, t.TempDir(), f)
	sm.newVault("master-pw")
	sm.withAccount()

	f.failPut = 1
	sm.linkGlupo(t, "mario", "K7QM-3XPA")
	sm.until(t, func() bool { return sm.screen == screenSync && !sm.sync.busy() })
	if sm.sync.err == "" || sm.link != nil || f.logins["mario"] == nil {
		t.Fatal(sm.text())
	}
	sm.Key(gui.KeyEsc)
	sm.Key(gui.KeyEsc)

	sm.linkGlupo(t, "mario", "K7QM-3XPA")
	sm.until(t, func() bool { return sm.screen == screenSync && !sm.sync.busy() })
	if sm.sync.done == "" || sm.link == nil || f.count("mario/"+vaultFile) != 1 {
		t.Fatal(sm.text())
	}

	// Someone else's name with a used invite: the login fails, the form says why.
	other := newGlupoModel(t, t.TempDir(), f)
	other.newVault("other-pw")
	other.linkGlupo(t, "mario", "K7QM-3XPA")
	other.until(t, func() bool { return other.screen == screenForm && other.form.err != "" })
	if !strings.Contains(other.text(), "Invite code not valid") || other.link != nil {
		t.Fatal(other.text())
	}
}

// glupoMac is a Mac with one account, linked to glupo.dev as mario, back on the account list.
func glupoMac(t *testing.T, f *fakeGlupo) *syncModel {
	t.Helper()
	mac := newGlupoModel(t, t.TempDir(), f)
	mac.newVault("master-pw")
	mac.withAccount()
	mac.linkGlupo(t, "mario", "K7QM-3XPA")
	mac.until(t, func() bool { return mac.screen == screenSync && !mac.sync.busy() })
	if mac.sync.done == "" {
		t.Fatal(mac.text())
	}
	mac.Key(gui.KeyEsc)
	mac.Key(gui.KeyEsc)
	mac.until(t, func() bool { return mac.link.Computer != "" })
	if mac.screen != screenList {
		t.Fatal(mac.text())
	}
	return mac
}

// glupoPC is a new device, with its own empty keychain, at the first start.
func glupoPC(t *testing.T, f *fakeGlupo) *syncModel {
	t.Helper()
	keyring.MockInit() // the Mac keeps what it needs in memory
	return newGlupoModel(t, t.TempDir(), f)
}

func TestAddDeviceFromGlupo(t *testing.T) {
	keyring.MockInit()
	f := newFakeGlupo(t, "K7QM3XPA")
	mac := glupoMac(t, f)
	macSecond := mac.vault.second
	pc := glupoPC(t, f)

	pc.Rune('2')
	if !strings.Contains(pc.text(), "2  From glupo.dev") {
		t.Fatal(pc.text())
	}
	pc.Rune('2')

	// A wrong password: back on the form, with the user name kept.
	pc.typ("Mario")
	pc.typ("wrong-pw")
	pc.until(t, func() bool { return pc.screen == screenForm && pc.form.err != "" })
	if !strings.Contains(pc.text(), "Wrong user name or master password") {
		t.Fatal(pc.text())
	}
	pc.Key(gui.KeyEnter) // keeps "mario"
	pc.typ("master-pw")
	pc.until(t, func() bool { return pc.screen == screenSync && pc.sync.pair != "" })
	if f.count("mario/requests/") != 1 {
		t.Fatal("no request on glupo.dev")
	}

	// The Mac sees the request with the same code and approves it.
	mac.tick()
	mac.until(t, func() bool { return mac.screen == screenApprove })
	if mac.approving.code != pc.sync.pair {
		t.Fatalf("%s %s", mac.approving.code, pc.sync.pair)
	}
	mac.Rune('y')
	mac.typ("master-pw")
	mac.until(t, func() bool { return strings.Contains(mac.text(), "Approved") })

	// The PC opens the vault with the password typed at the login: no question.
	pc.until(t, func() bool { return pc.screen == screenList })
	if pc.vault == nil || len(pc.vault.Accounts) != 1 || pc.link.Repo != "glupo.dev/mario" {
		t.Fatal(pc.text())
	}
	if !strings.Contains(pc.text(), "linked to glupo.dev, account mario") {
		t.Fatal(pc.text())
	}
	if s, _ := savedSecond(); string(s) != string(macSecond) {
		t.Fatal("the PC has not the same second key")
	}
	if p, _ := savedPassword(); p != "master-pw" {
		t.Fatal("the master password is not in the keychain")
	}
	if f.count("mario/requests/")+f.count("mario/answers/") != 0 {
		t.Fatal("request and answer not deleted")
	}
	pc.until(t, func() bool { return pc.link.Computer != "" })
	if f.count("mario/computers/") != 2 {
		t.Fatal("the PC is not in the list")
	}

	// A change on the PC reaches the Mac at its next check.
	before := pc.link.SHA
	pc.Rune('m')
	pc.typ("From the PC")
	pc.typ("")
	pc.until(t, func() bool { return !pc.saving && pc.link.SHA != before })
	mac.lastCheck = time.Time{}
	mac.tick()
	mac.until(t, func() bool { return mac.vault.Accounts[0].Name == "From the PC" })
}

func TestRefuseDeviceFromGlupo(t *testing.T) {
	keyring.MockInit()
	f := newFakeGlupo(t, "K7QM3XPA")
	mac := glupoMac(t, f)
	tokens := f.tokenCount()

	pc := glupoPC(t, f)
	pc.Rune('2')
	pc.Rune('2')
	pc.typ("mario")
	pc.typ("master-pw")
	pc.until(t, func() bool { return pc.screen == screenSync && pc.sync.pair != "" })
	if f.tokenCount() != tokens+1 {
		t.Fatal("no token for the PC")
	}

	mac.tick()
	mac.until(t, func() bool { return mac.screen == screenApprove })
	mac.Rune('n')
	pc.until(t, func() bool { return pc.screen == screenStart })
	if !strings.Contains(pc.text(), "Request refused or expired") || pc.vault != nil {
		t.Fatal(pc.text())
	}
	// The PC's token is revoked and gone from its keychain.
	if f.tokenCount() != tokens {
		t.Fatalf("tokens %d, want %d", f.tokenCount(), tokens)
	}
	if tok, _ := keyring.Get(keyringService, keyringGlupo); tok != "" {
		t.Fatal("the token is still in the keychain")
	}
}

func TestJoinGlupoWithoutVault(t *testing.T) {
	keyring.MockInit()
	f := newFakeGlupo(t, "K7QM3XPA")
	// An account whose first link stopped before the upload.
	f.failPut = 1
	first := newGlupoModel(t, t.TempDir(), f)
	first.newVault("master-pw")
	first.linkGlupo(t, "mario", "K7QM-3XPA")
	first.until(t, func() bool { return first.screen == screenSync && !first.sync.busy() })

	pc := glupoPC(t, f)
	pc.Rune('2')
	pc.Rune('2')
	pc.typ("mario")
	pc.typ("master-pw")
	pc.until(t, func() bool { return pc.screen == screenSync && !pc.sync.busy() })
	if !strings.Contains(pc.sync.err, "holds no vault on glupo.dev") || f.count("mario/requests/") != 0 {
		t.Fatal(pc.text())
	}
}

// Esc while waiting withdraws the request (it used to be sent with an already cancelled context).
func TestWithdrawRequestFromGlupo(t *testing.T) {
	keyring.MockInit()
	f := newFakeGlupo(t, "K7QM3XPA")
	glupoMac(t, f)
	tokens := f.tokenCount()
	pc := glupoPC(t, f)
	pc.Rune('2')
	pc.Rune('2')
	pc.typ("mario")
	pc.typ("master-pw")
	pc.until(t, func() bool { return pc.screen == screenSync && pc.sync.pair != "" })
	pc.Key(gui.KeyEsc)
	deadline := time.Now().Add(5 * time.Second)
	gone := func() bool {
		pc.gl.mu.Lock()
		forgotten := pc.gl.token == ""
		pc.gl.mu.Unlock()
		return f.count("mario/requests/") == 0 && f.tokenCount() == tokens && forgotten
	}
	for !gone() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !gone() {
		t.Fatalf("requests %d, tokens %d (want %d)", f.count("mario/requests/"), f.tokenCount(), tokens)
	}
	if tok, _ := keyring.Get(keyringService, keyringGlupo); tok != "" {
		t.Fatal("the token is still in the keychain")
	}
}
