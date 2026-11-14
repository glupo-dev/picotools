package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
)

// The sync through glupo.dev (docs/picoauth-glupo-sync.md): the same files as on GitHub
// (vault.picoauth, requests/, answers/, computers/), kept by picoauth-server, a separate project.
// The account is a user name; its password is the master password, of which the server only gets
// login_hash. Sign-up needs an invite code from the author. Each device keeps its own token.

// glupoServer is the address of picoauth-server: on the author's VPS, Caddy sends
// vault.glupo.dev/picoauth/* to it (a future Pico Password can have its own /picopassword).
// PICOAUTH_SERVER changes it, only for tests against a server on this computer
// (for example http://127.0.0.1:8686, without /picoauth).
const glupoServer = "https://vault.glupo.dev/picoauth"

// glupoPrefix starts the "repository" of a vault linked to glupo.dev in sync.json and in the
// code: glupo.dev/<username>. A GitHub owner cannot contain a dot, so it never meets a repository.
const glupoPrefix = "glupo.dev/"

func isGlupo(repo string) bool { return strings.HasPrefix(repo, glupoPrefix) }

// glupoUser is the user name of a glupo.dev "repository".
func glupoUser(repo string) string { return strings.TrimPrefix(repo, glupoPrefix) }

var (
	errInviteNotValid = errors.New("invite code not valid or already used")
	errUsernameTaken  = errors.New("this user name is taken")
	errBadUsername    = errors.New("the user name must be 3 to 32 characters: a-z, 0-9, - and _")
	errWrongLogin     = errors.New("wrong user name or master password")
)

// loginHash is what glupo.dev gets instead of the master password: Argon2id of the password,
// salted with "picoauth-login:" and the user name. The costs are fixed for ever: every device
// must compute the same value from user name and password alone.
func loginHash(username, password string) []byte {
	return argon2.IDKey([]byte(password), []byte("picoauth-login:"+username), 3, 64*1024, 4, 32)
}

// normalUsername is the user name as the server wants it: lower case, without spaces around.
func normalUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// glupo is the client of picoauth-server.
type glupo struct {
	api    string
	client *http.Client

	mu       sync.Mutex // the calls run in several goroutines
	token    string     // "" until read from the keychain or given by a login
	user     string     // for a new login when the server refuses the token
	password string
}

func newGlupo() *glupo {
	return &glupo{api: firstNonEmpty(os.Getenv("PICOAUTH_SERVER"), glupoServer), client: &http.Client{Timeout: 30 * time.Second}}
}

// creds are the user name and master password used to log in again by itself when the token
// is refused (for example revoked by the author): the device never asks for them.
func (g *glupo) creds(user, password string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.user, g.password = user, password
}

// ---- Calls without a token ----

// post sends JSON and decodes the answer; an error answer becomes one of the errors above.
func (g *glupo) post(ctx context.Context, path string, body, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", g.api+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return answerError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// answerError turns an error answer of the server into an error.
func answerError(resp *http.Response) error {
	var e struct {
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&e)
	switch e.Error {
	case "invite_not_valid":
		return errInviteNotValid
	case "username_taken":
		return errUsernameTaken
	case "invalid_username":
		return errBadUsername
	case "wrong_login":
		return errWrongLogin
	case "too_many_attempts":
		return errors.New("too many login attempts: wait a minute")
	}
	return fmt.Errorf("glupo.dev answered %d %s", resp.StatusCode, e.Error)
}

// check asks whether the invite is usable and the user name free. The server saves nothing.
func (g *glupo) check(ctx context.Context, user, invite string) error {
	return g.post(ctx, "/check", map[string]string{"username": user, "invite_code": invite}, nil)
}

// createAccount creates the account and keeps its token.
func (g *glupo) createAccount(ctx context.Context, user, invite string, hash []byte) error {
	var out struct {
		Token string `json:"token"`
	}
	err := g.post(ctx, "/accounts", map[string]string{
		"username": user, "invite_code": invite, "login_hash": base64.StdEncoding.EncodeToString(hash),
	}, &out)
	if err != nil {
		return err
	}
	return g.keep(out.Token)
}

// login gives this device a new token and keeps it.
func (g *glupo) login(ctx context.Context, user string, hash []byte) error {
	var out struct {
		Token string `json:"token"`
	}
	err := g.post(ctx, "/login", map[string]string{"username": user, "login_hash": base64.StdEncoding.EncodeToString(hash)}, &out)
	if err != nil {
		return err
	}
	return g.keep(out.Token)
}

// ---- The token ----

const keyringGlupo = "glupo.dev access"

func (g *glupo) keep(token string) error {
	if err := keyring.Set(keyringService, keyringGlupo, token); err != nil {
		return fmt.Errorf("could not keep the glupo.dev access in the keychain: %w", err)
	}
	g.mu.Lock()
	g.token = token
	g.mu.Unlock()
	return nil
}

// current is the token, read from the keychain the first time; "" if there is none.
func (g *glupo) current() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token == "" {
		g.token, _ = keyring.Get(keyringService, keyringGlupo)
	}
	return g.token
}

// relogin gets a new token with the user name and master password of this device.
func (g *glupo) relogin(ctx context.Context) error {
	g.mu.Lock()
	user, password := g.user, g.password
	g.token = ""
	g.mu.Unlock()
	if user == "" || password == "" {
		return errLoginNeeded
	}
	return g.login(ctx, user, loginHash(user, password))
}

// forget deletes the token of this device from the keychain.
func (g *glupo) forget() {
	g.mu.Lock()
	g.token, g.user, g.password = "", "", ""
	g.mu.Unlock()
	keyring.Delete(keyringService, keyringGlupo)
}

// logout revokes the token of this device on the server; it is fine if that fails.
func (g *glupo) logout(ctx context.Context) {
	if t := g.current(); t != "" {
		g.do(ctx, "POST", "/logout", nil, nil, t)
	}
}

// ---- Calls with the token ----

// do makes one request with a token; it gives the status code.
func (g *glupo) do(ctx context.Context, method, path string, body, out any, token string) (int, error) {
	var r *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	var req *http.Request
	var err error
	if r != nil {
		req, err = http.NewRequestWithContext(ctx, method, g.api+path, r)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, g.api+path, nil)
	}
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 && out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("glupo.dev answer not understood: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// call makes a request with the token; a refused token means a new login, done by itself, then
// the request once more.
func (g *glupo) call(ctx context.Context, method, path string, body, out any) (int, error) {
	token := g.current()
	if token == "" {
		if err := g.relogin(ctx); err != nil {
			return 0, err
		}
		token = g.current()
	}
	code, err := g.do(ctx, method, path, body, out, token)
	if err != nil || code != http.StatusUnauthorized {
		return code, err
	}
	if err := g.relogin(ctx); err != nil {
		if errors.Is(err, errWrongLogin) {
			return 0, errLoginNeeded
		}
		return 0, err
	}
	return g.do(ctx, method, path, body, out, g.current())
}

// The four file operations, with the same meaning as on GitHub (github.go): repo is ignored,
// the account is the token's. The version plays the role of the sha.

func (g *glupo) getFile(ctx context.Context, _ string, path string) ([]byte, string, error) {
	var f struct {
		Content string `json:"content"`
		Version string `json:"version"`
	}
	code, err := g.call(ctx, "GET", "/files/"+path, nil, &f)
	if err != nil {
		return nil, "", err
	}
	switch code {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", errNotFound
	default:
		return nil, "", fmt.Errorf("glupo.dev answered %d reading %s", code, path)
	}
	data, err := base64.StdEncoding.DecodeString(f.Content)
	return data, f.Version, err
}

func (g *glupo) putFile(ctx context.Context, _ string, path string, data []byte, version, _ string) (string, error) {
	body := map[string]string{"content": base64.StdEncoding.EncodeToString(data)}
	if version != "" {
		body["version"] = version
	}
	var out struct {
		Version string `json:"version"`
	}
	code, err := g.call(ctx, "PUT", "/files/"+path, body, &out)
	if err != nil {
		return "", err
	}
	switch code {
	case http.StatusOK:
		return out.Version, nil
	case http.StatusConflict:
		return "", errConflict
	default:
		return "", fmt.Errorf("glupo.dev answered %d writing %s", code, path)
	}
}

func (g *glupo) listDir(ctx context.Context, _ string, path string) ([]dirEntry, error) {
	var es []struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Version string `json:"version"`
		Type    string `json:"type"`
	}
	code, err := g.call(ctx, "GET", "/list?dir="+url.QueryEscape(path), nil, &es)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("glupo.dev answered %d listing %s", code, path)
	}
	out := make([]dirEntry, len(es))
	for i, e := range es {
		out[i] = dirEntry{Name: e.Name, Path: e.Path, SHA: e.Version, Type: e.Type}
	}
	return out, nil
}

func (g *glupo) deleteFile(ctx context.Context, _ string, path, version, _ string) error {
	code, err := g.call(ctx, "DELETE", "/files/"+path+"?version="+url.QueryEscape(version), nil, nil)
	if err != nil {
		return err
	}
	switch code {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	case http.StatusConflict:
		return errConflict
	default:
		return fmt.Errorf("glupo.dev answered %d deleting %s", code, path)
	}
}

// ---- One door for both services ----

// remotes sends each file operation to GitHub or to glupo.dev, by the "repository" of the link:
// glupo.dev/<username> goes to glupo.dev, owner/name to GitHub. It never changes, so the
// goroutines can use it without locks.
type remotes struct {
	gh *github
	gl *glupo
}

func (r remotes) getFile(ctx context.Context, repo, path string) ([]byte, string, error) {
	if isGlupo(repo) {
		return r.gl.getFile(ctx, repo, path)
	}
	return r.gh.getFile(ctx, repo, path)
}

func (r remotes) putFile(ctx context.Context, repo, path string, data []byte, sha, message string) (string, error) {
	if isGlupo(repo) {
		return r.gl.putFile(ctx, repo, path, data, sha, message)
	}
	return r.gh.putFile(ctx, repo, path, data, sha, message)
}

func (r remotes) listDir(ctx context.Context, repo, path string) ([]dirEntry, error) {
	if isGlupo(repo) {
		return r.gl.listDir(ctx, repo, path)
	}
	return r.gh.listDir(ctx, repo, path)
}

func (r remotes) deleteFile(ctx context.Context, repo, path, sha, message string) error {
	if isGlupo(repo) {
		return r.gl.deleteFile(ctx, repo, path, sha, message)
	}
	return r.gh.deleteFile(ctx, repo, path, sha, message)
}

// where is the name of the service of a link, for the messages.
func where(repo string) string {
	if isGlupo(repo) {
		return "glupo.dev"
	}
	return "GitHub"
}

// where is the name of the service this device is linked to, for the messages.
func (m *model) where() string {
	if m.link == nil {
		return "GitHub"
	}
	return where(m.link.Repo)
}

// glupoCreds gives the glupo.dev client the user name and master password of this device, so it
// can log in again by itself when its token is refused. It runs in the user interface.
func (m *model) glupoCreds() {
	if m.link != nil && isGlupo(m.link.Repo) && m.vault != nil {
		m.gl.creds(glupoUser(m.link.Repo), m.vault.password)
	}
}

// loginNeeded handles a refused login: on GitHub the browser login again, then then; on
// glupo.dev the device already tried again with its master password, so it only says so.
func (m *model) loginNeeded(then func()) {
	if m.link != nil && isGlupo(m.link.Repo) {
		m.warn("glupo.dev refused the login of this device: nothing was changed")
		return
	}
	m.relogin(then)
}
