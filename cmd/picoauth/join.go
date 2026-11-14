package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"picotools/internal/gui"
)

// Add another computer (docs/picoauth-sync.md, section 6): the new computer opens the vault
// that is on GitHub, after a linked computer approves it and gives it the second key.

// checkEvery is how often Pico Auth reads the repository: the new computer for the answer,
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
		_, _, err := m.gh.getFile(ctx, repo, vaultFile)
		if errors.Is(err, errNotFound) {
			err = errors.New(repo + " holds no vault: link first the computer that has it (Settings › Sync with GitHub)")
		}
		var reqSHA string
		if err == nil {
			body, _ := json.Marshal(request{PublicKey: pub, Created: now})
			reqSHA, err = m.gh.putFile(ctx, repo, requestPath(id), body, "", "Pico Auth: a new computer asks for access")
		}
		if err != nil {
			later(func() { v.err = capitalize(err.Error()) })
			return
		}
		later(func() {
			v.pair, v.msg = pairCode(pub), "Waiting for the approval…"
			// Leaving now withdraws the request.
			v.onEsc = func() {
				go m.gh.deleteFile(ctx, repo, requestPath(id), reqSHA, "Pico Auth: request withdrawn")
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
			data, sha, err := m.gh.getFile(ctx, repo, answerPath(id))
			if err == nil {
				ans, ansSHA = data, sha
				break
			}
			if !errors.Is(err, errNotFound) {
				continue // the network may come back
			}
			if _, _, err := m.gh.getFile(ctx, repo, requestPath(id)); errors.Is(err, errNotFound) {
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
			raw, sha, err = m.gh.getFile(ctx, repo, vaultFile) // the newest version
		}
		m.gh.deleteFile(ctx, repo, answerPath(id), ansSHA, "Pico Auth: access given")
		m.gh.deleteFile(ctx, repo, requestPath(id), reqSHA, "Pico Auth: access given")
		later(func() {
			v.onEsc = nil
			if err != nil {
				v.err = capitalize(err.Error())
				return
			}
			v.pair, v.msg = "", ""
			m.askJoinPassword(repo, raw, sha, second)
		})
	}()
}

// askJoinPassword asks the master password once, opens the vault and keeps everything here.
func (m *model) askJoinPassword(repo string, raw []byte, sha string, second []byte) {
	m.screen = screenStart
	m.ask("Open the vault from GitHub", []question{{label: "Master password", secret: true}}, func(a []string) string {
		v, err := decryptVault(raw, m.path, a[0], second)
		if errors.Is(err, errWrongPassword) {
			return "Wrong password"
		}
		if err == nil {
			err = writeFile(m.path, raw)
		}
		if err == nil {
			err = errors.Join(saveSecond(second), savePassword(a[0]))
		}
		l := &linkState{Repo: repo, SHA: sha}
		if err == nil {
			err = l.save(m.path)
		}
		if err != nil {
			return capitalize(err.Error())
		}
		m.vault, m.link, m.screen = v, l, screenList
		m.say("This computer is now linked to " + repo)
		return ""
	})
}

// ---- On a linked computer ----

// pendingRequest is a request of a new computer, shown for approval.
type pendingRequest struct {
	id   string
	sha  string
	pub  []byte
	code string
}

// checkRequests reads the requests of new computers, when this computer is linked and the
// account list is on screen. Old requests are deleted.
func (m *model) checkRequests() {
	if m.link == nil || m.vault == nil || m.vault.second == nil || m.checking || m.screen != screenList {
		return
	}
	m.checking = true
	repo, ctx, now := m.link.Repo, m.background, m.now()
	go func() {
		es, err := m.gh.listDir(ctx, repo, requestsDir)
		var found []*pendingRequest
		var created []time.Time
		for _, e := range es {
			if err != nil || e.Type != "file" || len(e.Name) < 6 || e.Name[len(e.Name)-5:] != ".json" {
				continue
			}
			raw, sha, err := m.gh.getFile(ctx, repo, e.Path)
			var r request
			if err != nil || json.Unmarshal(raw, &r) != nil {
				continue
			}
			id := e.Name[:len(e.Name)-5]
			if now.Sub(r.Created) > requestLife {
				m.gh.deleteFile(ctx, repo, e.Path, sha, "Pico Auth: request expired")
				continue
			}
			found = append(found, &pendingRequest{id: id, sha: sha, pub: r.PublicKey, code: pairCode(r.PublicKey)})
			created = append(created, r.Created)
		}
		// The oldest first.
		idx := make([]int, len(found))
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(a, b int) bool { return created[idx[a]].Before(created[idx[b]]) })
		m.ui(func() {
			m.checking = false
			if m.screen != screenList {
				return
			}
			for _, i := range idx {
				if r := found[i]; !m.handled[r.id] {
					m.approving, m.screen = r, screenApprove
					return
				}
			}
		})
	}()
}

func (m *model) approveRune(r rune) {
	req := m.approving
	switch r {
	case 'y', 'Y':
		m.ask("Approve the new computer", []question{{label: "Master password of this vault", secret: true}}, func(a []string) string {
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
			m.say("Approving the new computer…")
			repo, ctx := m.link.Repo, m.background
			go func() {
				_, err := m.gh.putFile(ctx, repo, answerPath(req.id), body, "", "Pico Auth: access given to a new computer")
				m.ui(func() {
					if err != nil {
						m.warn("Could not approve: " + err.Error())
						return
					}
					m.say("Approved: the new computer can now open the vault")
				})
			}()
			return ""
		})
	case 'n', 'N':
		m.handled[req.id] = true
		m.screen = screenList
		m.say("Request refused")
		repo, ctx := m.link.Repo, m.background
		go m.gh.deleteFile(ctx, repo, requestPath(req.id), req.sha, "Pico Auth: request refused")
	}
}

// ---- Screens ----

func (m *model) renderApprove() []line {
	r := m.approving
	return []line{
		{gui.Strong("A new computer asks for access to this vault", gui.Amber)},
		nil,
		{gui.Plain("Code:  "), gui.Strong(r.code, gui.ValueFg)},
		nil,
		{gui.Plain("Is it the same code shown on the new computer?")},
		{gui.Colored("If it is different, or you did not ask, refuse: someone else wrote the request.", gui.Muted)},
		nil,
		{gui.Colored("y approve (asks the master password) · n refuse", gui.Muted)},
	}
}
