package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"picotools/internal/gui"
)

// Sync with GitHub, from the Settings page (key s in the account list).
// This first part links the vault of this computer to a repository with no vault:
// browser login, the repository found by itself, a new second key, the vault uploaded.
// See docs/picoauth-sync.md, section 5.

// linkState is what this computer remembers of the link, next to the vault (sync.json):
// the repository and the version of the vault last uploaded or downloaded.
type linkState struct {
	Repo string `json:"repo"` // owner/name
	SHA  string `json:"sha"`
}

func linkPath(vault string) string { return filepath.Join(filepath.Dir(vault), "sync.json") }

// loadLink is the link of this computer, or nil if it is not linked.
func loadLink(vault string) *linkState {
	raw, err := os.ReadFile(linkPath(vault))
	if err != nil {
		return nil
	}
	var l linkState
	if json.Unmarshal(raw, &l) != nil || l.Repo == "" {
		return nil
	}
	return &l
}

func (l *linkState) save(vault string) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(linkPath(vault), append(b, '\n'))
}

// syncView is the state of the sync screen while it works.
type syncView struct {
	run    int // which run this is: an answer of an older, cancelled run is dropped
	cancel context.CancelFunc
	code   string   // the login code to type on GitHub, while waiting for it
	url    string   // where to type it
	msg    string   // what Pico Auth is doing now
	repos  []string // the repositories to choose from, when there is more than one
	err    string
	done   string
}

func (v *syncView) busy() bool { return v.err == "" && v.done == "" && v.repos == nil }

// ---- Settings ----

func (m *model) openSettings() { m.screen = screenSettings }

func (m *model) settingsRune(r rune) {
	if r == '1' && m.link == nil {
		m.startLink()
	}
}

// ---- Link this vault to GitHub ----

func (m *model) startLink() {
	ctx, cancel := context.WithCancel(context.Background())
	run := 1
	if m.sync != nil {
		run = m.sync.run + 1
	}
	v := &syncView{run: run, cancel: cancel, msg: "Connecting to GitHub…"}
	m.sync, m.screen = v, screenSync
	// later runs f in the user interface, only if this run was not cancelled.
	later := func(f func()) {
		m.ui(func() {
			if m.sync == v && m.screen == screenSync {
				f()
			}
		})
	}
	go func() {
		repos, err := m.gh.repos(ctx)
		if errors.Is(err, errLoginNeeded) {
			var d *deviceCode
			d, err = m.gh.startLogin(ctx)
			if err == nil {
				later(func() {
					v.code, v.url, v.msg = d.UserCode, d.VerificationURI, "Waiting for GitHub…"
					m.openURL(d.VerificationURI)
				})
				err = m.gh.waitLogin(ctx, d)
			}
			if err == nil {
				later(func() { v.code, v.msg = "", "Looking for the repository…" })
				repos, err = m.gh.repos(ctx)
			}
		}
		later(func() {
			switch {
			case err != nil:
				v.err = capitalize(err.Error())
			case len(repos) == 0:
				v.err = "Pico Auth cannot reach any repository: install it on a private repository of yours " +
					"(github.com/apps/pico-auth → Install → Only select repositories)"
			case len(repos) == 1:
				m.linkTo(ctx, v, later, repos[0])
			default:
				v.repos, v.msg = repos, ""
			}
		})
	}()
}

// linkTo checks that the repository holds no vault, then uploads this one, encrypted with
// master password + a new second key. It runs in the user interface.
func (m *model) linkTo(ctx context.Context, v *syncView, later func(func()), repo string) {
	v.repos, v.msg = nil, "Checking "+repo+"…"
	go func() {
		_, _, err := m.gh.getFile(ctx, repo, vaultFile)
		later(func() {
			switch {
			case err == nil:
				v.err = repo + " already holds a vault: nothing was done. A vault goes only to a repository with no vault."
				return
			case !errors.Is(err, errNotFound):
				v.err = capitalize(err.Error())
				return
			}
			// The second key goes in the keychain before the upload: a vault on GitHub without
			// its key anywhere could not be opened. If the upload fails, the key is just unused.
			second, err := newSecond()
			if err == nil {
				err = saveSecond(second)
			}
			if err != nil {
				v.err = capitalize(err.Error())
				return
			}
			m.vault.setSecond(second)
			data, err := m.vault.encrypted()
			m.vault.setSecond(nil) // the local copy changes only after the upload
			if err != nil {
				v.err = capitalize(err.Error())
				return
			}
			v.msg = "Uploading the vault to " + repo + "…"
			go func() {
				sha, err := m.gh.putFile(ctx, repo, vaultFile, data, "", "Pico Auth: first upload of the vault")
				later(func() { m.linked(v, repo, sha, second, data, err) })
			}()
		})
	}()
}

// linked ends the link: the local copy becomes the same file as the one on GitHub.
func (m *model) linked(v *syncView, repo, sha string, second, data []byte, err error) {
	if errors.Is(err, errConflict) {
		v.err = repo + " got a vault in the meantime: nothing was done."
		return
	}
	if err != nil {
		v.err = capitalize(err.Error())
		return
	}
	if err := writeFile(m.path, data); err != nil {
		v.err = "The vault is on GitHub, but this computer could not save it: " + err.Error()
		return
	}
	m.vault.setSecond(second)
	l := &linkState{Repo: repo, SHA: sha}
	if err := l.save(m.path); err != nil {
		v.err = "The vault is on GitHub, but this computer could not remember the link: " + err.Error()
		return
	}
	m.link = l
	v.msg, v.done = "", "This vault is now on GitHub, in "+repo
}

func (m *model) syncKey(k string) {
	if k == gui.KeyEnter && m.sync.code != "" {
		m.copy(m.sync.code)
		m.say("Code copied: paste it on GitHub")
		return
	}
	if k != gui.KeyEsc {
		return
	}
	if m.sync.busy() || m.sync.repos != nil {
		m.sync.cancel()
	}
	m.screen = screenSettings
}

func (m *model) syncRune(r rune) {
	v := m.sync
	if v.repos == nil || r < '1' || r > '9' {
		return
	}
	i := int(r - '1')
	if i >= len(v.repos) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	later := func(f func()) {
		m.ui(func() {
			if m.sync == v && m.screen == screenSync {
				f()
			}
		})
	}
	m.linkTo(ctx, v, later, v.repos[i])
}

// ---- Screens ----

func (m *model) renderSettings() []line {
	lines := []line{{gui.Strong("Settings", nil)}, nil}
	if m.link != nil {
		lines = append(lines,
			line{gui.Plain("Synced with GitHub: "), gui.Strong(m.link.Repo, gui.ValueFg)})
		return append(lines, m.footer("esc back")...)
	}
	lines = append(lines,
		line{gui.Plain("  "), gui.Strong("1", gui.Accent), gui.Plain("  Sync with GitHub")},
		line{gui.Colored("     Put this vault in a private GitHub repository of yours, to have it on your other computers.", gui.Muted)},
	)
	return append(lines, m.footer("number choose · esc back")...)
}

func (m *model) renderSync() []line {
	v := m.sync
	lines := []line{{gui.Strong("Settings › Sync with GitHub", nil)}, nil}
	if v.code != "" {
		lines = append(lines,
			line{gui.Plain("1. In the browser, on "), gui.Strong(v.url, gui.ValueFg), gui.Colored("  (opened now)", gui.Muted)},
			line{gui.Plain("2. type this code:   "), gui.Strong(v.code, gui.ValueFg)},
			line{gui.Plain("3. and click "), gui.Strong("Authorize", nil), gui.Plain(".")},
			nil,
		)
	}
	if v.msg != "" {
		lines = append(lines, line{gui.Colored(v.msg, gui.Muted)})
	}
	for i, r := range v.repos {
		if i == 0 {
			lines = append(lines, line{gui.Plain("Pico Auth can reach more than one repository: which one holds the vault?")}, nil)
		}
		if i < 9 {
			lines = append(lines, line{gui.Plain("  "), gui.Strong(fmt.Sprint(i+1), gui.Accent), gui.Plain("  " + r)})
		}
	}
	if v.err != "" {
		lines = append(lines, line{gui.Strong("✘ "+v.err, gui.Red)})
	}
	if v.done != "" {
		lines = append(lines, line{gui.Strong("✔ "+v.done, gui.Amber)})
	}
	hint := "esc back"
	switch {
	case v.repos != nil:
		hint = "number choose · esc cancel"
	case v.code != "":
		hint = "enter copy the code · esc cancel"
	case v.busy():
		hint = "esc cancel"
	}
	return append(lines, m.footer(hint)...)
}
