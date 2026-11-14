package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

// Pico Auth talks to GitHub over HTTPS, without git, as the GitHub App "Pico Auth"
// (github.com/apps/pico-auth). The user logs in from the browser with the Device Flow:
// Pico Auth shows a short code, the user types it on github.com/login/device, and GitHub
// gives an access that works only on the repositories where the app is installed.
// See docs/picoauth-sync.md, section 2.

// githubClientID tells GitHub which app asks for the login. It is not a secret.
const githubClientID = "Iv23liUsuaDQViHcGo3m"

// vaultFile is the name of the vault in the repository.
const vaultFile = "vault.picoauth"

var (
	errLoginNeeded = errors.New("the GitHub login is needed")
	errConflict    = errors.New("the file on GitHub was changed meanwhile")
	errNotFound    = errors.New("not found on GitHub")
)

// ghToken is the access given by the login. The access lasts about 8 hours and is renewed
// with the refresh token, which lasts months.
type ghToken struct {
	Access         string    `json:"access"`
	AccessExpires  time.Time `json:"access_expires"`
	Refresh        string    `json:"refresh"`
	RefreshExpires time.Time `json:"refresh_expires"`
}

// github is a small client for the few calls Pico Auth needs.
type github struct {
	web    string // https://github.com
	api    string // https://api.github.com
	client *http.Client
	now    func() time.Time

	mu    sync.Mutex // the calls run in several goroutines: one renews the access at a time
	token *ghToken
}

func newGitHub() *github {
	return &github{
		web:    "https://github.com",
		api:    "https://api.github.com",
		client: &http.Client{Timeout: 30 * time.Second},
		now:    time.Now,
	}
}

// ---- Login (Device Flow) ----

// deviceCode is what GitHub answers when the login starts.
type deviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`        // the code the user types, like ABCD-1234
	VerificationURI string `json:"verification_uri"` // github.com/login/device
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"` // seconds between two checks
}

// tokenAnswer is the answer of the token endpoint, for a login or a refresh.
type tokenAnswer struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
}

// form posts a form to github.com and decodes the JSON answer.
func (g *github) form(ctx context.Context, path string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", g.web+path, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// startLogin asks GitHub for the code the user types in the browser.
func (g *github) startLogin(ctx context.Context) (*deviceCode, error) {
	var d deviceCode
	if err := g.form(ctx, "/login/device/code", url.Values{"client_id": {githubClientID}}, &d); err != nil {
		return nil, err
	}
	if d.DeviceCode == "" || d.UserCode == "" {
		return nil, errors.New("GitHub did not give a login code")
	}
	return &d, nil
}

// waitLogin checks GitHub, at the interval it asks, until the user authorizes the code in the
// browser, refuses it, or the code expires.
func (g *github) waitLogin(ctx context.Context, d *deviceCode) error {
	interval := time.Duration(d.Interval) * time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		var t tokenAnswer
		err := g.form(ctx, "/login/oauth/access_token", url.Values{
			"client_id":   {githubClientID},
			"device_code": {d.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &t)
		if err != nil {
			return err
		}
		switch t.Error {
		case "":
			return g.keep(t)
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return errors.New("the code expired: start again")
		case "access_denied":
			return errors.New("the login was cancelled on GitHub")
		default:
			return fmt.Errorf("GitHub login: %s", firstNonEmpty(t.ErrorDescription, t.Error))
		}
	}
}

// keep turns a token answer into the access, and keeps it in the keychain.
func (g *github) keep(t tokenAnswer) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.keepLocked(t)
}

func (g *github) keepLocked(t tokenAnswer) error {
	if t.AccessToken == "" {
		return errors.New("GitHub did not give an access")
	}
	now := g.now()
	tok := &ghToken{Access: t.AccessToken, Refresh: t.RefreshToken}
	if t.ExpiresIn > 0 {
		tok.AccessExpires = now.Add(time.Duration(t.ExpiresIn) * time.Second)
	}
	if t.RefreshTokenExpiresIn > 0 {
		tok.RefreshExpires = now.Add(time.Duration(t.RefreshTokenExpiresIn) * time.Second)
	}
	g.token = tok
	return saveToken(tok)
}

// access is a valid access, renewed with the refresh token if it expired.
// errLoginNeeded means the browser login must be done (again).
func (g *github) access(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token == nil {
		tok, err := savedToken()
		if err != nil {
			return "", err
		}
		if tok == nil {
			return "", errLoginNeeded
		}
		g.token = tok
	}
	now := g.now()
	t := g.token
	if t.AccessExpires.IsZero() || now.Add(time.Minute).Before(t.AccessExpires) {
		return t.Access, nil
	}
	if t.Refresh == "" || (!t.RefreshExpires.IsZero() && now.After(t.RefreshExpires)) {
		return "", errLoginNeeded
	}
	var a tokenAnswer
	err := g.form(ctx, "/login/oauth/access_token", url.Values{
		"client_id":     {githubClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.Refresh},
	}, &a)
	if err != nil {
		return "", err
	}
	if a.Error != "" {
		return "", errLoginNeeded // refresh token refused or expired: log in again
	}
	if err := g.keepLocked(a); err != nil {
		return "", err
	}
	return g.token.Access, nil
}

// forget drops the access, here and in the keychain.
func (g *github) forget() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = nil
	deleteToken()
}

// ---- API calls ----

// call makes an API request with the access; a refused access means a new login.
func (g *github) call(ctx context.Context, method, path string, body, out any) (int, error) {
	tok, err := g.access(ctx)
	if err != nil {
		return 0, err
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.api+path, r)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		g.forget()
		return resp.StatusCode, errLoginNeeded
	case resp.StatusCode >= 300:
		return resp.StatusCode, nil
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("GitHub answer not understood: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// repos are the repositories the app can reach: those where the user installed it
// ("Only select repositories"). Each is "owner/name".
func (g *github) repos(ctx context.Context) ([]string, error) {
	var inst struct {
		Installations []struct {
			ID int64 `json:"id"`
		} `json:"installations"`
	}
	code, err := g.call(ctx, "GET", "/user/installations?per_page=100", nil, &inst)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %d for the installations", code)
	}
	var all []string
	for _, in := range inst.Installations {
		var rs struct {
			Repositories []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
		}
		code, err := g.call(ctx, "GET", fmt.Sprintf("/user/installations/%d/repositories?per_page=100", in.ID), nil, &rs)
		if err != nil {
			return nil, err
		}
		if code != http.StatusOK {
			return nil, fmt.Errorf("GitHub answered %d for the repositories", code)
		}
		for _, r := range rs.Repositories {
			all = append(all, r.FullName)
		}
	}
	return all, nil
}

// getFile reads a file of the repository, with its sha (the version, needed to replace it).
// A missing file gives errNotFound.
func (g *github) getFile(ctx context.Context, repo, path string) (data []byte, sha string, err error) {
	var f struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	code, err := g.call(ctx, "GET", "/repos/"+repo+"/contents/"+path, nil, &f)
	if err != nil {
		return nil, "", err
	}
	switch code {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", errNotFound
	default:
		return nil, "", fmt.Errorf("GitHub answered %d reading %s", code, path)
	}
	if f.Encoding != "base64" {
		return nil, "", fmt.Errorf("%s is not a file Pico Auth can read", path)
	}
	data, err = base64.StdEncoding.DecodeString(strings.ReplaceAll(f.Content, "\n", ""))
	return data, f.SHA, err
}

// putFile writes a file. sha is the version read before ("" for a new file): if the file on
// GitHub is not that version any more, GitHub refuses and putFile gives errConflict.
// It gives the sha of the new version.
func (g *github) putFile(ctx context.Context, repo, path string, data []byte, sha, message string) (string, error) {
	body := map[string]string{"message": message, "content": base64.StdEncoding.EncodeToString(data)}
	if sha != "" {
		body["sha"] = sha
	}
	var out struct {
		Content struct {
			SHA string `json:"sha"`
		} `json:"content"`
	}
	code, err := g.call(ctx, "PUT", "/repos/"+repo+"/contents/"+path, body, &out)
	if err != nil {
		return "", err
	}
	switch code {
	case http.StatusOK, http.StatusCreated:
		return out.Content.SHA, nil
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return "", errConflict
	default:
		return "", fmt.Errorf("GitHub answered %d writing %s", code, path)
	}
}

// dirEntry is a file in a folder of the repository.
type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	SHA  string `json:"sha"`
	Type string `json:"type"` // "file" or "dir"
}

// listDir lists a folder of the repository ("" the root); a missing folder is empty.
func (g *github) listDir(ctx context.Context, repo, path string) ([]dirEntry, error) {
	var es []dirEntry
	p := "/repos/" + repo + "/contents"
	if path != "" {
		p += "/" + path
	}
	code, err := g.call(ctx, "GET", p, nil, &es)
	if err != nil {
		return nil, err
	}
	switch code {
	case http.StatusOK:
		return es, nil
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("GitHub answered %d listing %s", code, path)
	}
}

// deleteFile deletes a file at the version sha; a file already gone is fine.
func (g *github) deleteFile(ctx context.Context, repo, path, sha, message string) error {
	code, err := g.call(ctx, "DELETE", "/repos/"+repo+"/contents/"+path, map[string]string{"message": message, "sha": sha}, nil)
	if err != nil {
		return err
	}
	switch code {
	case http.StatusOK, http.StatusNotFound:
		return nil
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return errConflict
	default:
		return fmt.Errorf("GitHub answered %d deleting %s", code, path)
	}
}

// ---- The access in the keychain ----

const keyringToken = "github access"

func savedToken() (*ghToken, error) {
	s, err := keyring.Get(keyringService, keyringToken)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t ghToken
	if json.Unmarshal([]byte(s), &t) != nil {
		return nil, nil // damaged: log in again
	}
	return &t, nil
}

func saveToken(t *ghToken) error {
	b, _ := json.Marshal(t)
	if err := keyring.Set(keyringService, keyringToken, string(b)); err != nil {
		return fmt.Errorf("could not keep the GitHub access in the keychain: %w", err)
	}
	return nil
}

func deleteToken() { keyring.Delete(keyringService, keyringToken) }

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
