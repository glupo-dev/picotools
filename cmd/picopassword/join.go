package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"picotools/internal/gui"
)

// Add another computer (docs/picoauth-sync.md, section 6): the new computer opens the vault
// that is on GitHub, after a linked computer approves it and gives it the second key.

// checkEvery is how often Pico Password reads the repository: the new computer for the answer,
// a linked computer for requests of new computers.
const checkEvery = 10 * time.Second

// ---- On the new computer ----

func (m *model) startJoin() {
	v := m.connect("Open an existing vault › From GitHub", screenStart)
	v.next = func(repo string) { m.join(v, repo) }
}

// join downloads the vault, writes a request and waits for the answer of a linked computer.
func (m *model) join(v *syncView, repo string) {
	ctx, later := v.ctx, v.later
	v.repos, v.msg = nil, "Looking for the vault in "+repo+"…"
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	id, err2 := newRequestID()
	if err = errors.Join(err, err2); err != nil {
		v.err = capitalize(err.Error())
		return
	}
	pub := priv.PublicKey().Bytes()
	now, every := m.now().UTC(), m.checkEvery
	go func() {
		_, _, err := m.rs.getFile(ctx, repo, vaultFile)
		if errors.Is(err, errNotFound) {
			err = errors.New(repo + " holds no vault: link first the device that has it (Settings › Sync with GitHub)")
			if isGlupo(repo) {
				err = errors.New("The account " + glupoUser(repo) + " holds no vault on glupo.dev: link first the device that has it (Settings › Sync with glupo.dev)")
			}
		}
		var reqSHA string
		if err == nil {
			body, _ := json.Marshal(request{PublicKey: pub, Created: now})
			reqSHA, err = m.rs.putFile(ctx, repo, requestPath(id), body, "", "Pico Password: a new device asks for access")
		}
		if err != nil {
			later(func() { v.err = capitalize(err.Error()) })
			return
		}
		later(func() {
			v.pair, v.msg = pairCode(pub), "Waiting for the approval…"
			// Leaving now withdraws the request (with the background context: the view's one is
			// cancelled by esc, before this runs).
			v.onEsc = func() {
				bg := m.background
				go func() {
					m.rs.deleteFile(bg, repo, requestPath(id), reqSHA, "Pico Password: request withdrawn")
					m.leftJoin(bg, repo)
				}()
			}
		})

		// Wait for the answer; the request gone without an answer means refused or expired.
		var ans []byte
		var ansSHA string
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(every):
			}
			data, sha, err := m.rs.getFile(ctx, repo, answerPath(id))
			if err == nil {
				ans, ansSHA = data, sha
				break
			}
			if !errors.Is(err, errNotFound) {
				continue // the network may come back
			}
			if _, _, err := m.rs.getFile(ctx, repo, requestPath(id)); errors.Is(err, errNotFound) {
				m.leftJoin(ctx, repo)
				later(func() {
					v.onEsc = nil
					m.startPage = 0
					m.screen = screenStart
					m.warn("Request refused or expired")
				})
				return
			}
		}
		second, err := openSecond(priv, id, ans)
		var raw []byte
		var sha string
		if err == nil {
			raw, sha, err = m.rs.getFile(ctx, repo, vaultFile) // the newest version
		}
		m.rs.deleteFile(ctx, repo, answerPath(id), ansSHA, "Pico Password: access given")
		m.rs.deleteFile(ctx, repo, requestPath(id), reqSHA, "Pico Password: access given")
		later(func() {
			v.onEsc = nil
			if err != nil {
				v.err = capitalize(err.Error())
				return
			}
			v.pair, v.msg = "", ""
			if v.password != "" {
				// glupo.dev: the master password was typed before the login; it opens the vault.
				if msg := m.finishJoin(repo, raw, sha, second, v.password); msg == "" {
					return
				}
			}
			m.askJoinPassword(repo, raw, sha, second)
		})
	}()
}

// leftJoin forgets the glupo.dev access of a new device that did not get in (refused, expired, or
// esc): its token is revoked on the server and removed from the keychain. Nothing to do on GitHub.
// It runs outside the user interface.
func (m *model) leftJoin(ctx context.Context, repo string) {
	if isGlupo(repo) {
		m.gl.logout(ctx)
		m.gl.forget()
	}
}

// askJoinPassword asks the master password once, opens the vault and keeps everything here.
func (m *model) askJoinPassword(repo string, raw []byte, sha string, second []byte) {
	m.screen = screenStart
	m.ask("Open the vault from "+where(repo), []question{{label: "Master password", secret: true}}, func(a []string) string {
		return m.finishJoin(repo, raw, sha, second, a[0])
	})
}

// finishJoin opens the vault with the master password and keeps everything here; it gives the
// message of what went wrong, or "".
func (m *model) finishJoin(repo string, raw []byte, sha string, second []byte, password string) string {
	v, err := decryptVault(raw, m.path, password, second)
	if errors.Is(err, errWrongPassword) {
		return "Wrong password"
	}
	if err == nil {
		err = writeFile(m.path, raw)
	}
	if err == nil {
		err = errors.Join(saveSecond(second), savePassword(password))
	}
	l := &linkState{Repo: repo, SHA: sha}
	if err == nil {
		err = l.save(m.path)
	}
	if err != nil {
		return capitalize(err.Error())
	}
	m.vault, m.link, m.screen = v, l, screenList
	m.glupoCreds()
	m.register()
	if isGlupo(repo) {
		m.say("This device is now linked to glupo.dev, account " + glupoUser(repo))
	} else {
		m.say("This device is now linked to " + repo)
	}
	return ""
}

// ---- On a linked computer ----

// pendingRequest is a request of a new computer, shown for approval.
type pendingRequest struct {
	id   string
	sha  string
	pub  []byte
	code string
}

// fetchRequests reads the requests of new computers, the oldest first, and deletes the old ones.
// It runs outside the user interface.
func (m *model) fetchRequests(ctx context.Context, repo string, now time.Time) []*pendingRequest {
	es, err := m.rs.listDir(ctx, repo, requestsDir)
	if err != nil {
		return nil
	}
	var found []*pendingRequest
	created := map[*pendingRequest]time.Time{}
	for _, e := range es {
		name, ok := strings.CutSuffix(e.Name, ".json")
		if e.Type != "file" || !ok {
			continue
		}
		raw, sha, err := m.rs.getFile(ctx, repo, e.Path)
		var r request
		if err != nil || json.Unmarshal(raw, &r) != nil {
			continue
		}
		if now.Sub(r.Created) > requestLife {
			m.rs.deleteFile(ctx, repo, e.Path, sha, "Pico Password: request expired")
			continue
		}
		p := &pendingRequest{id: name, sha: sha, pub: r.PublicKey, code: pairCode(r.PublicKey)}
		found = append(found, p)
		created[p] = r.Created
	}
	sort.Slice(found, func(a, b int) bool { return created[found[a]].Before(created[found[b]]) })
	return found
}

// showRequest shows the first request not yet approved or refused, if the list is on screen.
func (m *model) showRequest(found []*pendingRequest) {
	if m.screen != screenList {
		return
	}
	for _, r := range found {
		if !m.handled[r.id] {
			m.approving, m.screen = r, screenApprove
			return
		}
	}
}

func (m *model) approveRune(r rune) {
	req := m.approving
	switch r {
	case 'y', 'Y':
		m.ask("Approve the new device", []question{{label: "Master password of this vault", secret: true}}, func(a []string) string {
			if subtle.ConstantTimeCompare([]byte(a[0]), []byte(m.vault.password)) != 1 {
				return "Wrong password"
			}
			m.handled[req.id] = true
			m.screen = screenList
			body, err := sealSecond(req.pub, req.id, m.vault.second)
			if err != nil {
				m.warn(capitalize(err.Error()))
				return ""
			}
			m.say("Approving the new device…")
			repo, ctx := m.link.Repo, m.background
			go func() {
				_, err := m.rs.putFile(ctx, repo, answerPath(req.id), body, "", "Pico Password: access given to a new device")
				m.ui(func() {
					if err != nil {
						m.warn("Could not approve: " + err.Error())
						return
					}
					m.say("Approved: the new device can now open the vault")
				})
			}()
			return ""
		})
	case 'n', 'N':
		m.handled[req.id] = true
		m.screen = screenList
		m.say("Request refused")
		repo, ctx := m.link.Repo, m.background
		go m.rs.deleteFile(ctx, repo, requestPath(req.id), req.sha, "Pico Password: request refused")
	}
}

// ---- Screens ----

func (m *model) renderApprove() []line {
	r := m.approving
	return []line{
		{gui.Strong("A new device asks for access to this vault", gui.Amber)},
		nil,
		{gui.Plain("Code:  "), gui.Strong(r.code, gui.ValueFg)},
		nil,
		{gui.Plain("Is it the same code shown on the new device?")},
		{gui.Colored("If it is different, or you did not ask, refuse: someone else wrote the request.", gui.Muted)},
		nil,
		{gui.Colored("y approve (asks the master password) · n refuse", gui.Muted)},
	}
}
