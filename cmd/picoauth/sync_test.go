package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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
		f.access = "t1"
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
		key := strings.TrimSuffix(strings.Replace(strings.TrimPrefix(r.URL.Path, "/repos/"), "/contents/", "/", 1), "/contents")
		data, ok := f.files[key]
		sha := shaOf(data)
		if r.Method == "GET" && !ok {
			// A folder: its files, and its subfolders once.
			var es []map[string]any
			dirs := map[string]bool{}
			for k := range f.files {
				rest, found := strings.CutPrefix(k, key+"/")
				if !found {
					continue
				}
				repoLen := len(strings.Join(strings.SplitN(k, "/", 3)[:2], "/")) + 1
				if dir, _, sub := strings.Cut(rest, "/"); sub {
					if !dirs[dir] {
						dirs[dir] = true
						es = append(es, map[string]any{"name": dir, "path": k[repoLen:len(k)-len(rest)] + dir, "sha": "d", "type": "dir"})
					}
					continue
				}
				es = append(es, map[string]any{"name": rest, "path": k[repoLen:], "sha": shaOf(f.files[k]), "type": "file"})
			}
			if es != nil {
				out(200, es)
				return
			}
		}
		if r.Method == "DELETE" {
			var body struct{ SHA string }
			json.NewDecoder(r.Body).Decode(&body)
			if !ok {
				out(404, map[string]any{"message": "Not Found"})
			} else if body.SHA != sha {
				out(409, map[string]any{"message": "conflict"})
			} else {
				delete(f.files, key)
				out(200, map[string]any{})
			}
			return
		}
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
		out(201, map[string]any{"content": map[string]any{"sha": shaOf(f.files[key])}})
	default:
		out(404, map[string]any{"message": "Not Found"})
	}
}

func shaOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:8])
}

func (f *fakeGitHub) count(prefix string) int {
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
	tm.checkEvery = 10 * time.Millisecond
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

// linkedMac is a Mac with a vault of one account, linked to the fake GitHub.
func linkedMac(t *testing.T, f *fakeGitHub) *syncModel {
	mac := newSyncModel(t, t.TempDir(), f)
	mac.newVault("master-pw")
	mac.vault.Accounts = []*Account{{Kind: kindTOTP, Name: "GitHub", User: "me", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30}}
	mac.vault.save()
	mac.Rune('s')
	mac.Rune('1')
	mac.until(t, func() bool { return !mac.sync.busy() })
	mac.Key(gui.KeyEsc)
	mac.Key(gui.KeyEsc)
	if mac.screen != screenList || mac.link == nil {
		t.Fatal(mac.text())
	}
	mac.until(t, func() bool { return mac.link.Computer != "" })
	return mac
}

// newPC is a new computer, with its own empty keychain, asking for access from GitHub.
func newPC(t *testing.T, f *fakeGitHub) *syncModel {
	keyring.MockInit() // the Mac keeps what it needs in memory
	pc := newSyncModel(t, t.TempDir(), f)
	pc.Rune('2')
	pc.Rune('1')
	pc.until(t, func() bool { return pc.sync.pair != "" })
	return pc
}

func TestAddComputer(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)
	macSecond := mac.vault.second

	pc := newPC(t, f)
	if !strings.Contains(pc.text(), pc.sync.pair) || f.count("genlupo/picoauth-vault/requests/") != 1 {
		t.Fatal(pc.text())
	}

	// The Mac finds the request at its next check, with the same code.
	mac.tick()
	mac.until(t, func() bool { return mac.screen == screenApprove })
	if mac.approving.code != pc.sync.pair || !strings.Contains(mac.text(), pc.sync.pair) {
		t.Fatalf("%s\n%s", mac.approving.code, mac.text())
	}

	// y and the master password: a wrong one is refused.
	mac.Rune('y')
	mac.typ("wrong")
	if !strings.Contains(mac.text(), "Wrong password") {
		t.Fatal(mac.text())
	}
	for range 5 {
		mac.Key(gui.KeyBackspace)
	}
	mac.typ("master-pw")
	mac.until(t, func() bool { return strings.Contains(mac.text(), "Approved") })

	// The PC gets the second key and asks the master password once.
	pc.until(t, func() bool { return pc.screen == screenForm })
	pc.typ("master-pw")
	if pc.screen != screenList || len(pc.vault.Accounts) != 1 || pc.link == nil {
		t.Fatal(pc.text())
	}
	if s, _ := savedSecond(); string(s) != string(macSecond) {
		t.Fatal("the PC has not the same second key")
	}
	if f.count("genlupo/picoauth-vault/requests/")+f.count("genlupo/picoauth-vault/answers/") != 0 {
		t.Fatal("request and answer not deleted")
	}
	pc.until(t, func() bool { return pc.link.Computer != "" })
	if f.count("genlupo/picoauth-vault/computers/") != 2 {
		t.Fatal("the PC is not in the list")
	}

	// Next start on the PC opens by itself.
	again := newTestModel(t, pc.path[:len(pc.path)-len("/picoauth/vault.picoauth")])
	if again.screen != screenList || len(again.vault.Accounts) != 1 {
		t.Fatal(again.text())
	}
}

func TestRefuseComputer(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)
	pc := newPC(t, f)

	mac.tick()
	mac.until(t, func() bool { return mac.screen == screenApprove })
	mac.Rune('n')
	if mac.screen != screenList || !strings.Contains(mac.text(), "Request refused") {
		t.Fatal(mac.text())
	}
	pc.until(t, func() bool { return pc.screen == screenStart })
	if !strings.Contains(pc.text(), "Request refused or expired") || pc.vault != nil {
		t.Fatal(pc.text())
	}
	// The refused request is not shown again.
	mac.lastCheck = time.Time{}
	mac.tick()
	time.Sleep(50 * time.Millisecond)
	for len(mac.ui) > 0 {
		(<-mac.ui)()
	}
	if mac.screen != screenList {
		t.Fatal(mac.text())
	}
}

func TestJoinWithoutVault(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	pc := newSyncModel(t, t.TempDir(), f)
	pc.Rune('2')
	pc.Rune('1')
	pc.until(t, func() bool { return !pc.sync.busy() })
	if !strings.Contains(pc.sync.err, "holds no vault") || f.count("genlupo/picoauth-vault/requests/") != 0 {
		t.Fatal(pc.text())
	}
	pc.Key(gui.KeyEsc)
	if pc.screen != screenStart {
		t.Fatal(pc.screen)
	}
}

func TestPairing(t *testing.T) {
	second, _ := newSecond()
	priv, _ := ecdhKey()
	body, err := sealSecond(priv.PublicKey().Bytes(), "abc", second)
	if err != nil {
		t.Fatal(err)
	}
	got, err := openSecond(priv, "abc", body)
	if err != nil || string(got) != string(second) {
		t.Fatal(err)
	}
	if _, err := openSecond(priv, "other", body); err == nil {
		t.Fatal("opened with another id")
	}
	other, _ := ecdhKey()
	if _, err := openSecond(other, "abc", body); err == nil {
		t.Fatal("opened by another computer")
	}
	if c := pairCode(priv.PublicKey().Bytes()); len(c) != 7 || c[3] != ' ' {
		t.Fatal(c)
	}
}

func ecdhKey() (*ecdh.PrivateKey, error) { return ecdh.X25519().GenerateKey(rand.Reader) }

const vaultKey = "genlupo/picoauth-vault/" + vaultFile

// remote opens the vault on the fake GitHub with the keys of the Mac.
func remote(t *testing.T, f *fakeGitHub, mac *syncModel) *Vault {
	t.Helper()
	v, err := decryptVault(f.file(vaultKey), "", mac.vault.password, mac.vault.second)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// otherComputer saves on GitHub a change made by another linked computer.
func otherComputer(t *testing.T, f *fakeGitHub, mac *syncModel, change func(v *Vault)) {
	v := remote(t, f, mac)
	change(v)
	data, _ := v.encrypted()
	f.mu.Lock()
	f.files[vaultKey] = data
	f.mu.Unlock()
}

// inSync checks that the local copy is the file on GitHub, at the version remembered.
func inSync(t *testing.T, f *fakeGitHub, mac *syncModel) {
	t.Helper()
	local, _ := os.ReadFile(mac.path)
	up := f.file(vaultKey)
	if string(local) != string(up) || mac.link.SHA != shaOf(up) || loadLink(mac.path).SHA != mac.link.SHA {
		t.Fatal("this computer and GitHub differ")
	}
}

func TestChangesUploaded(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)

	mac.Rune('n')
	mac.typ("jbsw y3dp ehpk 3pxp")
	mac.typ("Test")
	mac.Key(gui.KeyEnter)
	mac.Rune('y')
	if !strings.Contains(mac.text(), "Saving on GitHub") {
		t.Fatal(mac.text())
	}
	mac.Rune('n') // no other change while saving
	if mac.screen != screenList {
		t.Fatal(mac.text())
	}
	mac.until(t, func() bool { return !mac.saving })
	if len(remote(t, f, mac).Accounts) != 2 || !strings.Contains(mac.text(), "Test saved") {
		t.Fatal(mac.text())
	}
	inSync(t, f, mac)

	mac.Rune('m')
	for range len("Test") {
		mac.Key(gui.KeyBackspace)
	}
	mac.typ("Test 2")
	mac.Key(gui.KeyEnter)
	mac.until(t, func() bool { return !mac.saving })
	if remote(t, f, mac).Accounts[1].Name != "Test 2" {
		t.Fatal(mac.text())
	}
	mac.Rune('d')
	mac.Rune('y')
	mac.until(t, func() bool { return !mac.saving })
	if len(remote(t, f, mac).Accounts) != 1 || len(mac.vault.Accounts) != 1 {
		t.Fatal(mac.text())
	}
	inSync(t, f, mac)
}

func TestConflict(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)

	// The PC adds an account; the Mac does not know yet and renames its own.
	otherComputer(t, f, mac, func(v *Vault) {
		v.Accounts = append(v.Accounts, &Account{Kind: kindTOTP, Name: "Other", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30})
	})
	mac.Rune('m')
	for range len("GitHub") {
		mac.Key(gui.KeyBackspace)
	}
	mac.typ("GitHub work")
	mac.Key(gui.KeyEnter)
	mac.until(t, func() bool { return !mac.saving })

	r := remote(t, f, mac)
	if len(r.Accounts) != 2 || r.Accounts[0].Name != "GitHub work" || r.Accounts[1].Name != "Other" {
		t.Fatalf("%+v %+v\n%s", r.Accounts[0], r.Accounts[len(r.Accounts)-1], mac.text())
	}
	inSync(t, f, mac)

	// Renamed here, deleted on the PC: the rename cannot be done.
	otherComputer(t, f, mac, func(v *Vault) { v.Accounts = v.Accounts[1:] })
	mac.Rune('m')
	mac.typ("X")
	mac.Key(gui.KeyEnter)
	mac.until(t, func() bool { return !mac.saving })
	if !strings.Contains(mac.text(), "deleted on another device") || len(mac.vault.Accounts) != 1 {
		t.Fatal(mac.text())
	}
}

func TestChangesArrive(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)
	otherComputer(t, f, mac, func(v *Vault) { v.Accounts[0].Name = "Renamed on the PC" })

	mac.lastCheck = time.Time{}
	mac.tick()
	mac.until(t, func() bool { return mac.vault.Accounts[0].Name == "Renamed on the PC" })
	inSync(t, f, mac)
}

func TestLastUsed(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)
	path := "genlupo/picoauth-vault/" + computerPath(mac.link.Computer)
	lastUsed := func() time.Time {
		var c computer
		json.Unmarshal(f.file(path), &c)
		return c.LastUsed
	}
	if !lastUsed().Equal(mac.at) {
		t.Fatal("the link does not write the time")
	}

	// The next day Pico Auth is opened again, with no change: the time is written once.
	next := newSyncModel(t, mac.path[:len(mac.path)-len("/picoauth/vault.picoauth")], f)
	next.at = next.at.Add(24 * time.Hour)
	opened := next.at
	next.tick()
	next.until(t, func() bool { return next.usedWritten })
	if !lastUsed().Equal(opened) {
		t.Fatalf("last used %v", lastUsed())
	}
	next.at = next.at.Add(time.Minute)
	next.lastCheck = time.Time{}
	next.tick()
	next.until(t, func() bool { return !next.checking })
	if !lastUsed().Equal(opened) {
		t.Fatal("written again in the same opening")
	}

	next.Rune('s')
	next.Rune('2')
	next.until(t, func() bool { return !next.computers.loading })
	if !strings.Contains(next.text(), "last today "+opened.Local().Format("15:04")) || !strings.Contains(next.text(), "since ") {
		t.Fatal(next.text())
	}
}

func TestOffline(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)
	before, _ := os.ReadFile(mac.path)
	mac.gh.api = "http://127.0.0.1:1" // nothing answers there

	mac.Rune('d')
	mac.Rune('y')
	mac.until(t, func() bool { return !mac.saving })
	if !strings.Contains(mac.text(), "Cannot reach GitHub: the change was not saved. Try again later.") || len(mac.vault.Accounts) != 1 {
		t.Fatal(mac.text())
	}
	after, _ := os.ReadFile(mac.path)
	if string(before) != string(after) {
		t.Fatal("changed without GitHub")
	}
	// The codes still work.
	mac.Rune('1')
	if mac.clip == "" {
		t.Fatal("no code copied")
	}
	// The check finds nothing and says nothing.
	mac.lastCheck = time.Time{}
	mac.tick()
	mac.until(t, func() bool { return !mac.checking })
	if mac.screen != screenList {
		t.Fatal(mac.text())
	}
}

func TestLoginAgain(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)
	f.mu.Lock()
	f.access = "revoked" // GitHub no longer accepts the access of the Mac
	f.mu.Unlock()

	mac.Rune('d')
	mac.Rune('y')
	mac.until(t, func() bool { return mac.screen == screenSync && mac.sync.code != "" })
	if !strings.Contains(mac.text(), "log in again") {
		t.Fatal(mac.text())
	}
	mac.until(t, func() bool { return mac.screen == screenList && !mac.saving && len(mac.vault.Accounts) == 0 })
	if len(remote(t, f, mac).Accounts) != 0 {
		t.Fatal("not deleted on GitHub")
	}
	inSync(t, f, mac)
}

func TestDeleteVault(t *testing.T) {
	keyring.MockInit()
	tm := newTestModel(t, t.TempDir())
	tm.Rune('1')
	tm.typ("master-pw")
	tm.typ("master-pw")
	tm.Rune('s')
	if !strings.Contains(tm.text(), "3  Delete this vault") {
		t.Fatal(tm.text())
	}
	tm.Rune('3')
	tm.typ("yes")
	if !strings.Contains(tm.text(), "Type delete") {
		t.Fatal(tm.text())
	}
	for range 3 {
		tm.Key(gui.KeyBackspace)
	}
	tm.typ("delete")
	tm.typ("wrong")
	if !strings.Contains(tm.text(), "Wrong password") {
		t.Fatal(tm.text())
	}
	for range 5 {
		tm.Key(gui.KeyBackspace)
	}
	tm.typ("master-pw")
	if tm.screen != screenStart || !strings.Contains(tm.text(), "Vault deleted") {
		t.Fatal(tm.text())
	}
	if _, err := os.Stat(tm.path); err == nil {
		t.Fatal("the file is still there")
	}
	if pw, _ := savedPassword(); pw != "" {
		t.Fatal("the password is still in the keychain")
	}
	// esc cancels it.
	tm.Rune('1')
	tm.typ("master-pw")
	tm.typ("master-pw")
	tm.Rune('s')
	tm.Rune('3')
	tm.Key(gui.KeyEsc)
	if tm.screen != screenSettings || tm.vault == nil {
		t.Fatal(tm.text())
	}
}

func TestUnlink(t *testing.T) {
	keyring.MockInit()
	f := newFakeGitHub(t, "genlupo/picoauth-vault")
	mac := linkedMac(t, f)
	up := f.file(vaultKey)
	if f.count("genlupo/picoauth-vault/computers/") != 1 {
		t.Fatal("the Mac is not in the list")
	}

	// The list of linked computers.
	mac.Rune('s')
	if !strings.Contains(mac.text(), "1  Reset this device") || strings.Contains(mac.text(), "Delete this vault") {
		t.Fatal(mac.text())
	}
	mac.Rune('2')
	mac.until(t, func() bool { return !mac.computers.loading })
	name := thisComputer(time.Now()).Name
	if !strings.Contains(mac.text(), name) || !strings.Contains(mac.text(), "(this one)") {
		t.Fatal(mac.text())
	}
	mac.Key(gui.KeyEsc)

	// Without GitHub it cannot be unlinked.
	api := mac.gh.api
	mac.gh.api = "http://127.0.0.1:1"
	mac.Rune('1')
	mac.until(t, func() bool { return mac.status != nil && strings.Contains(mac.text(), "No connection") })
	mac.gh.api = api

	// The only linked computer: a clear warning.
	mac.Rune('1')
	mac.until(t, func() bool { return mac.screen == screenForm })
	if !strings.Contains(mac.text(), "only linked device") {
		t.Fatal(mac.text())
	}
	mac.Key(gui.KeyEsc)

	// With another computer, the usual words.
	f.mu.Lock()
	f.files["genlupo/picoauth-vault/computers/pc.json"] = []byte(`{"name":"PC-UFFICIO","system":"Windows","since":"2026-11-20T09:00:00Z"}`)
	f.mu.Unlock()
	mac.Rune('1')
	mac.until(t, func() bool { return mac.screen == screenForm })
	if strings.Contains(mac.text(), "only linked device") {
		t.Fatal(mac.text())
	}
	mac.typ("delete")
	mac.typ("master-pw")
	mac.until(t, func() bool { return mac.screen == screenStart })
	if !strings.Contains(mac.text(), "This device is reset") {
		t.Fatal(mac.text())
	}
	for _, p := range []string{mac.path, linkPath(mac.path)} {
		if _, err := os.Stat(p); err == nil {
			t.Fatal(p + " is still there")
		}
	}
	if s, _ := savedSecond(); s != nil {
		t.Fatal("the second key is still in the keychain")
	}
	if tok, _ := savedToken(); tok != nil {
		t.Fatal("the GitHub access is still in the keychain")
	}
	if string(f.file(vaultKey)) != string(up) {
		t.Fatal("the vault on GitHub changed")
	}
	if f.count("genlupo/picoauth-vault/computers/") != 1 || f.file("genlupo/picoauth-vault/computers/pc.json") == nil {
		t.Fatal("the list on GitHub is wrong")
	}
	// No more checks of the repository.
	mac.lastCheck = time.Time{}
	mac.tick()
	if mac.checking {
		t.Fatal("still checking")
	}
}
