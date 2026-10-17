package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gltools/internal/gui"
)

type screen int

const (
	screenProviders    screen = iota // the start: list of providers
	screenEndpoints                  // the endpoints of a provider
	screenProviderEdit               // the fields of a provider
	screenHeaders                    // the fixed headers of a provider
	screenEndpointEdit               // the fields of an endpoint
	screenForm                       // questions, one after the other
	screenConfirm                    // a yes/no question
	screenBusy                       // waiting for the response
	screenResponse                   // the response
	screenCurl                       // the response's request, to copy as a curl command
)

// model holds the state of GL API and reacts to keys.
// It knows nothing about the window: copying, quitting and running in the user
// interface are done through callbacks.
type model struct {
	store  *Store
	screen screen

	provCursor   int // selected provider
	epCursor     int // selected endpoint
	fieldCursor  int // selected field of the provider or endpoint being edited
	headerCursor int // selected fixed header

	prov *Provider // provider opened
	ep   *Endpoint // endpoint opened

	form    *form
	confirm string // the yes/no question
	yes     func() // run on "y"
	after   screen // where a form or a question goes back to

	cancel  context.CancelFunc // stops the request in progress
	runID   int                // identifies the current request, so the late response of a cancelled one is ignored
	started time.Time
	reqLine string    // method and address of the request, the key hidden
	call    *callSpec // the request sent, for the curl commands

	curlCursor int // selected shell in screenCurl

	resp        *response
	err         error
	viewer      gui.Viewer
	showHeaders bool // the response headers are shown instead of the body

	status gui.Line // message shown at the bottom (e.g. "Saved"), cleared by the next key
	cols   int
	rows   int

	copy func(text string)
	quit func()
	// ui runs f in the user interface and redraws; it can be called from any goroutine.
	ui        func(f func())
	now       func() time.Time
	downloads string // where responses are saved the first time
}

func newModel(s *Store) *model {
	home, _ := os.UserHomeDir()
	return &model{
		store:     s,
		cols:      gui.DefaultCols,
		copy:      func(string) {},
		quit:      func() {},
		ui:        func(f func()) { f() },
		now:       time.Now,
		downloads: filepath.Join(home, "Downloads"),
	}
}

// ---- Keys ----

func (m *model) Key(k string) {
	if m.screen != screenForm {
		m.status = nil
	}
	switch m.screen {
	case screenProviders:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.provCursor = move(m.provCursor, k, len(m.store.Providers))
		case gui.KeyEnter:
			m.openProvider(m.provCursor)
		case gui.KeyEsc:
			m.quit()
		}

	case screenEndpoints:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.epCursor = move(m.epCursor, k, len(m.prov.Endpoints))
		case gui.KeyEnter:
			m.runEndpoint(m.epCursor)
		case gui.KeyEsc:
			m.screen = screenProviders
		}

	case screenProviderEdit:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.fieldCursor = move(m.fieldCursor, k, 4)
		case gui.KeyEnter:
			m.editProviderField(m.fieldCursor)
		case gui.KeyEsc:
			m.screen = screenProviders
		}

	case screenHeaders:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.headerCursor = move(m.headerCursor, k, len(m.prov.Headers))
		case gui.KeyEnter:
			if len(m.prov.Headers) > 0 {
				m.headerForm(m.headerCursor)
			}
		case gui.KeyEsc:
			m.screen = screenProviderEdit
		}

	case screenEndpointEdit:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.fieldCursor = move(m.fieldCursor, k, 3)
		case gui.KeyEnter:
			m.editEndpointField(m.fieldCursor)
		case gui.KeyEsc:
			m.screen = screenEndpoints
		}

	case screenForm:
		m.form.key(k)

	case screenConfirm:
		m.screen = m.after // any key other than y is no

	case screenBusy:
		if k == gui.KeyEsc {
			m.stopRequest()
			m.screen = screenEndpoints
		}

	case screenResponse:
		if k == gui.KeyEsc {
			if m.showHeaders {
				m.toggleHeaders()
				return
			}
			m.screen = screenEndpoints
			return
		}
		if m.resp != nil && (m.resp.isText || m.showHeaders) {
			m.viewer.Key(k, m.cols, m.viewRoom())
		}

	case screenCurl:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.curlCursor = move(m.curlCursor, k, len(curlShells))
		case gui.KeyEnter:
			m.copyCurl(m.curlCursor)
		case gui.KeyEsc:
			m.screen = screenResponse
		}
	}
}

func (m *model) Rune(r rune) {
	if m.screen != screenForm {
		m.status = nil
	}
	switch m.screen {
	case screenProviders:
		switch {
		case r == 'n':
			m.newProvider()
		case r == 'm' && len(m.store.Providers) > 0:
			m.prov, m.fieldCursor, m.screen = m.store.Providers[m.provCursor], 0, screenProviderEdit
		case r == 'd' && len(m.store.Providers) > 0:
			m.deleteProvider()
		case r == 'q':
			m.quit()
		case r >= '1' && r <= '9':
			m.openProvider(int(r - '1'))
		}

	case screenEndpoints:
		switch {
		case r == 'n':
			m.newEndpoint()
		case r == 'm' && len(m.prov.Endpoints) > 0:
			m.ep, m.fieldCursor, m.screen = m.prov.Endpoints[m.epCursor], 0, screenEndpointEdit
		case r == 'd' && len(m.prov.Endpoints) > 0:
			m.deleteEndpoint()
		case r >= '1' && r <= '9':
			m.runEndpoint(int(r - '1'))
		}

	case screenProviderEdit:
		if r >= '1' && r <= '4' {
			m.fieldCursor = int(r - '1')
			m.editProviderField(m.fieldCursor)
		}

	case screenHeaders:
		switch {
		case r == 'n':
			m.headerForm(-1)
		case r == 'd' && len(m.prov.Headers) > 0:
			m.deleteHeader()
		}

	case screenEndpointEdit:
		if r >= '1' && r <= '3' {
			m.fieldCursor = int(r - '1')
			m.editEndpointField(m.fieldCursor)
		}

	case screenForm:
		m.form.rune(r)

	case screenConfirm:
		m.screen = m.after
		if r == 'y' || r == 'Y' {
			m.yes()
		}

	case screenResponse:
		switch r {
		case 'h':
			if m.resp != nil {
				m.toggleHeaders()
			}
		case 'c':
			m.copyShown()
		case 's':
			if m.resp != nil {
				m.saveForm()
			}
		case 'u':
			m.screen = screenCurl
		}

	case screenCurl:
		if r >= '1' && int(r-'1') < len(curlShells) {
			m.copyCurl(int(r - '1'))
		}
	}
}

func (m *model) Paste(text string) {
	if m.screen == screenForm {
		m.form.paste(text)
	}
}

func (m *model) CanPaste() bool { return m.screen == screenForm }

// move takes the cursor one step up or down in a list of n entries, going round.
func move(cursor int, k string, n int) int {
	if n == 0 {
		return 0
	}
	if k == gui.KeyUp {
		return (cursor - 1 + n) % n
	}
	return (cursor + 1) % n
}

// ---- Forms ----

// ask opens a form; when it ends (done or esc) the screen goes back to the current one,
// unless done moved somewhere else.
func (m *model) ask(title string, qs []question, done func([]string) string) {
	back := m.screen
	m.form = newForm(title, qs, func(a []string) string {
		m.screen = back
		msg := done(a)
		if msg != "" {
			m.screen = screenForm
		}
		return msg
	}, func() { m.screen = back })
	if !m.form.finished() {
		m.screen = screenForm
	}
}

func (m *model) askYes(question string, yes func()) {
	m.confirm, m.yes, m.after, m.screen = question, yes, m.screen, screenConfirm
}

func (m *model) saveStore() bool {
	if err := m.store.save(); err != nil {
		m.status = gui.Line{gui.Strong("✘ Could not save: "+err.Error(), gui.Red)}
		return false
	}
	return true
}

func (m *model) say(msg string) { m.status = gui.Line{gui.Strong("✔ "+msg, gui.Amber)} }

func (m *model) fail(msg string) { m.status = gui.Line{gui.Strong("✘ "+msg, gui.Red)} }

// ---- Checks of the answers ----

func (m *model) checkProviderName(except *Provider) func(string, []string) string {
	return func(v string, _ []string) string {
		if p := m.store.find(v); p != nil && p != except {
			return "There is already a provider named " + v
		}
		return ""
	}
}

func checkURL(v string, _ []string) string {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "Write the full address, e.g. https://api.example.com/v1"
	}
	return ""
}

func checkKeyIn(v string, _ []string) string {
	if v < "1" || v > "4" || len(v) != 1 {
		return "Type 1, 2, 3 or 4"
	}
	return ""
}

func checkHeaderName(v string, _ []string) string {
	if strings.ContainsAny(v, " :\t") {
		return "A name without spaces or colons, e.g. X-API-Key"
	}
	return ""
}

func checkHeaderLine(v string, _ []string) string {
	name, value, ok := strings.Cut(v, ":")
	if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" || checkHeaderName(strings.TrimSpace(name), nil) != "" {
		return "Write it as name: value, e.g. anthropic-version: 2023-06-01"
	}
	return ""
}

func checkRequest(v string, _ []string) string {
	if _, _, err := parseRequest(v); err != nil {
		return capitalize(err.Error())
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// keyInChoice turns where the key goes into the number of the question, and back.
var keyInChoices = []string{keyHeader, keyBearer, keyQuery, keyNone}

func keyInNumber(keyIn string) string {
	for i, k := range keyInChoices {
		if k == keyIn {
			return fmt.Sprint(i + 1)
		}
	}
	return "4"
}

func keyInFrom(answer string) string { return keyInChoices[answer[0]-'1'] }

const keyInLabel = "Key goes in (1 header · 2 Authorization: Bearer · 3 query · 4 none)"

// keyQuestions ask where the key goes, the name of its header or parameter and the key.
// They are the questions from index first of a form.
func keyQuestions(first int, p *Provider, keySaved bool) []question {
	keyName := func(a []string) string { return a[first] }
	keyLabel, keyOptional := "Key", false
	if keySaved {
		keyLabel, keyOptional = "Key (empty keeps the saved one)", true
	}
	name := ""
	if p != nil && (p.KeyIn == keyHeader || p.KeyIn == keyQuery) {
		name = p.KeyName
	}
	keyIn := "1"
	if p != nil {
		keyIn = keyInNumber(p.KeyIn)
	}
	return []question{
		{label: keyInLabel, value: keyIn, check: checkKeyIn},
		{label: "Header name", value: name, check: checkHeaderName,
			skip: func(a []string) bool { return keyName(a) != "1" }},
		{label: "Parameter name", value: name, check: checkHeaderName,
			skip: func(a []string) bool { return keyName(a) != "3" }},
		{label: keyLabel, secret: true, optional: keyOptional,
			skip: func(a []string) bool { return keyName(a) == "4" }},
	}
}

// applyKey sets where the key goes from the answers of keyQuestions and saves the key.
func applyKey(p *Provider, a []string) error {
	p.KeyIn = keyInFrom(a[0])
	p.KeyName = ""
	switch p.KeyIn {
	case keyHeader:
		p.KeyName = a[1]
	case keyQuery:
		p.KeyName = a[2]
	case keyNone:
		return deleteKey(p.Name)
	}
	if a[3] != "" {
		return setKey(p.Name, a[3])
	}
	return nil
}

// ---- Providers ----

func (m *model) openProvider(i int) {
	if i < 0 || i >= len(m.store.Providers) {
		return
	}
	m.provCursor = i
	m.prov, m.epCursor, m.screen = m.store.Providers[i], 0, screenEndpoints
}

func (m *model) newProvider() {
	qs := []question{
		{label: "Name", check: m.checkProviderName(nil)},
		{label: "Base URL", check: checkURL},
	}
	qs = append(qs, keyQuestions(2, nil, false)...)
	qs = append(qs, question{label: "Fixed headers (name: value, empty line to finish)", repeat: true, check: checkHeaderLine})
	m.ask("New provider", qs, func(a []string) string {
		p := &Provider{Name: a[0], URL: strings.TrimRight(a[1], "/")}
		if err := applyKey(p, a[2:6]); err != nil {
			return "Could not save the key in the keychain: " + err.Error()
		}
		if a[6] != "" {
			for _, l := range strings.Split(a[6], "\n") {
				name, value, _ := strings.Cut(l, ":")
				p.Headers = append(p.Headers, Header{strings.TrimSpace(name), strings.TrimSpace(value)})
			}
		}
		m.store.Providers = append(m.store.Providers, p)
		m.sortProviders()
		m.provCursor = m.indexOfProvider(p)
		if m.saveStore() {
			m.say("Saved " + p.Name)
		}
		return ""
	})
}

func (m *model) sortProviders() {
	sort.SliceStable(m.store.Providers, func(i, j int) bool {
		return strings.ToLower(m.store.Providers[i].Name) < strings.ToLower(m.store.Providers[j].Name)
	})
}

func (m *model) indexOfProvider(p *Provider) int {
	for i, q := range m.store.Providers {
		if q == p {
			return i
		}
	}
	return 0
}

func (m *model) deleteProvider() {
	p := m.store.Providers[m.provCursor]
	m.askYes("Delete the provider "+p.Name+", its endpoints and its key?", func() {
		if err := deleteKey(p.Name); err != nil {
			m.fail("Could not delete the key from the keychain: " + err.Error())
			return
		}
		ps := m.store.Providers
		i := m.indexOfProvider(p)
		m.store.Providers = append(ps[:i:i], ps[i+1:]...)
		m.provCursor = min(m.provCursor, max(len(m.store.Providers)-1, 0))
		if m.saveStore() {
			m.say("Deleted " + p.Name)
		}
	})
}

// editProviderField changes one field of the provider: 0 name, 1 URL, 2 key, 3 headers.
func (m *model) editProviderField(i int) {
	p := m.prov
	switch i {
	case 0:
		m.ask("Edit "+p.Name, []question{{label: "Name", value: p.Name, check: m.checkProviderName(p)}}, func(a []string) string {
			if a[0] == p.Name {
				return ""
			}
			// The key is saved under the provider name: move it to the new one.
			key, err := getKey(p.Name)
			if err == nil && key != "" {
				if err = setKey(a[0], key); err == nil {
					err = deleteKey(p.Name)
				}
			}
			if err != nil {
				return "Could not move the key in the keychain: " + err.Error()
			}
			p.Name = a[0]
			m.sortProviders()
			m.provCursor = m.indexOfProvider(p)
			if m.saveStore() {
				m.say("Saved")
			}
			return ""
		})
	case 1:
		m.ask("Edit "+p.Name, []question{{label: "Base URL", value: p.URL, check: checkURL}}, func(a []string) string {
			p.URL = strings.TrimRight(a[0], "/")
			if m.saveStore() {
				m.say("Saved")
			}
			return ""
		})
	case 2:
		key, _ := getKey(p.Name)
		m.ask("Edit "+p.Name, keyQuestions(0, p, key != ""), func(a []string) string {
			if err := applyKey(p, a); err != nil {
				return "Could not save the key in the keychain: " + err.Error()
			}
			if m.saveStore() {
				m.say("Saved")
			}
			return ""
		})
	case 3:
		m.headerCursor, m.screen = 0, screenHeaders
	}
}

// headerForm adds a fixed header (i < 0) or changes header i.
func (m *model) headerForm(i int) {
	p := m.prov
	value, title := "", "New header for "+p.Name
	if i >= 0 {
		value, title = p.Headers[i].Name+": "+p.Headers[i].Value, "Edit header of "+p.Name
	}
	m.ask(title, []question{{label: "Header (name: value)", value: value, check: checkHeaderLine}}, func(a []string) string {
		name, v, _ := strings.Cut(a[0], ":")
		h := Header{strings.TrimSpace(name), strings.TrimSpace(v)}
		if i >= 0 {
			p.Headers[i] = h
		} else {
			p.Headers = append(p.Headers, h)
			m.headerCursor = len(p.Headers) - 1
		}
		if m.saveStore() {
			m.say("Saved")
		}
		return ""
	})
}

func (m *model) deleteHeader() {
	p, i := m.prov, m.headerCursor
	m.askYes("Delete the header "+p.Headers[i].Name+"?", func() {
		p.Headers = append(p.Headers[:i:i], p.Headers[i+1:]...)
		m.headerCursor = min(m.headerCursor, max(len(p.Headers)-1, 0))
		if m.saveStore() {
			m.say("Deleted")
		}
	})
}

// ---- Endpoints ----

func (m *model) checkEndpointName(except *Endpoint) func(string, []string) string {
	return func(v string, _ []string) string {
		for _, e := range m.prov.Endpoints {
			if e.Name == v && e != except {
				return "There is already an endpoint named " + v
			}
		}
		return ""
	}
}

func bodyQuestion(value string) question {
	return question{label: "Body (paste it, variables as {name})", value: value, optional: true}
}

func (m *model) newEndpoint() {
	qs := []question{
		{label: "Name", check: m.checkEndpointName(nil)},
		{label: "Request (method and path, e.g. GET /users/{id}?lang={lang?})", check: checkRequest},
		bodyQuestion(""),
	}
	qs[2].skip = func(a []string) bool {
		method, _, _ := parseRequest(a[1])
		return !hasBody(method)
	}
	m.ask("New endpoint for "+m.prov.Name, qs, func(a []string) string {
		e := &Endpoint{Name: a[0], Request: normalRequest(a[1]), Body: a[2]}
		m.prov.Endpoints = append(m.prov.Endpoints, e)
		m.epCursor = len(m.prov.Endpoints) - 1
		if m.saveStore() {
			m.say("Saved " + e.Name)
		}
		return ""
	})
}

// normalRequest writes the method in capitals and one space before the path.
func normalRequest(r string) string {
	method, path, _ := parseRequest(r)
	return method + " " + path
}

func (m *model) deleteEndpoint() {
	p, e := m.prov, m.prov.Endpoints[m.epCursor]
	m.askYes("Delete the endpoint "+e.Name+"?", func() {
		i := m.epCursor
		p.Endpoints = append(p.Endpoints[:i:i], p.Endpoints[i+1:]...)
		m.epCursor = min(m.epCursor, max(len(p.Endpoints)-1, 0))
		if m.saveStore() {
			m.say("Deleted " + e.Name)
		}
	})
}

// editEndpointField changes one field of the endpoint: 0 name, 1 request, 2 body.
func (m *model) editEndpointField(i int) {
	e := m.ep
	title := "Edit " + e.Name
	saved := func() string {
		if m.saveStore() {
			m.say("Saved")
		}
		return ""
	}
	switch i {
	case 0:
		m.ask(title, []question{{label: "Name", value: e.Name, check: m.checkEndpointName(e)}}, func(a []string) string {
			e.Name = a[0]
			return saved()
		})
	case 1:
		m.ask(title, []question{{label: "Request", value: e.Request, check: checkRequest}}, func(a []string) string {
			e.Request = normalRequest(a[0])
			return saved()
		})
	case 2:
		m.ask(title, []question{bodyQuestion(e.Body)}, func(a []string) string {
			e.Body = a[0]
			return saved()
		})
	}
}

// ---- Running an endpoint ----

// runEndpoint asks the variables of endpoint i, if any, and sends the request.
func (m *model) runEndpoint(i int) {
	if i < 0 || i >= len(m.prov.Endpoints) {
		return
	}
	m.epCursor = i
	e := m.prov.Endpoints[i]
	m.ep = e
	vars := variables(e)
	if len(vars) == 0 {
		m.send(map[string]string{})
		return
	}
	qs := make([]question, len(vars))
	for j, v := range vars {
		qs[j] = question{label: v.name, optional: v.optional, history: e.Values[v.name]}
	}
	m.ask(m.prov.Name+" › "+e.Name, qs, func(a []string) string {
		values := map[string]string{}
		for j, v := range vars {
			values[v.name] = a[j]
			e.remember(v.name, a[j])
		}
		m.saveStore()
		m.send(values)
		return ""
	})
}

// send makes the request in the background; esc stops it.
func (m *model) send(values map[string]string) {
	p, e := m.prov, m.ep
	key := ""
	if p.KeyIn != keyNone {
		var err error
		if key, err = getKey(p.Name); err != nil {
			m.fail("Could not read the key from the keychain: " + err.Error())
			return
		}
	}
	c, err := newCall(p, e, values)
	if err != nil {
		m.fail(capitalize(err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	req, err := c.request(ctx, key)
	if err != nil {
		cancel()
		m.fail(capitalize(err.Error()))
		return
	}
	m.call = c
	m.runID++
	id := m.runID
	m.cancel = cancel
	m.started = m.now()
	m.reqLine = req.Method + " " + c.shown(key)
	m.resp, m.err, m.showHeaders = nil, nil, false
	m.screen = screenBusy

	// Redraw now and then, so the seconds waited go on.
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.ui(func() {})
			}
		}
	}()
	go func() {
		resp, err := call(req)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("no response within %d seconds", int(timeout.Seconds()))
		}
		m.ui(func() {
			if id != m.runID || m.screen != screenBusy {
				return // cancelled with esc
			}
			m.showResponse(resp, err)
		})
	}()
}

func (m *model) stopRequest() {
	m.runID++
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *model) showResponse(resp *response, err error) {
	m.resp, m.err, m.screen = resp, err, screenResponse
	if err != nil {
		return
	}
	resp.isText = isText(resp.body)
	if !resp.isText {
		return
	}
	switch {
	case len(resp.body) == 0:
		resp.shown = ""
		m.viewer.Set("(empty body)", "", false)
	default:
		resp.lang, resp.shown = gui.Format(string(resp.body))
		resp.shown = strings.ReplaceAll(resp.shown, "\r\n", "\n")
		m.viewer.Set(resp.shown, resp.lang, true)
	}
}

// toggleHeaders switches between the body and the response headers.
func (m *model) toggleHeaders() {
	m.showHeaders = !m.showHeaders
	if m.showHeaders {
		m.viewer.Set(headerText(m.resp), "", false)
		return
	}
	if m.resp.isText {
		m.showResponse(m.resp, nil)
	}
}

// headerText lists the response headers, one "Name: value" per line, sorted by name.
func headerText(r *response) string {
	names := make([]string, 0, len(r.header))
	for n := range r.header {
		names = append(names, n)
	}
	sort.Strings(names)
	var lines []string
	for _, n := range names {
		for _, v := range r.header[n] {
			lines = append(lines, n+": "+v)
		}
	}
	return strings.Join(lines, "\n")
}

// copyShown copies the body as shown, or the headers when they are shown.
func (m *model) copyShown() {
	switch {
	case m.resp == nil:
	case m.showHeaders:
		m.copy(headerText(m.resp))
		m.say("Copied the headers")
	case !m.resp.isText:
		m.fail("The body is not text: save it with s")
	case m.resp.shown == "":
		m.fail("Nothing to copy: the body is empty")
	default:
		m.copy(m.resp.shown)
		m.say("Copied")
	}
}

// copyCurl copies the request as a curl command for shell i, with the key placeholder.
func (m *model) copyCurl(i int) {
	m.copy(m.call.curl(curlShells[i]))
	m.curlCursor = i
	m.screen = screenResponse
	m.say("Copied: curl for " + curlShells[i].name)
}

// saveForm asks where to save the body: JSON and XML formatted as shown, the rest as it came.
func (m *model) saveForm() {
	dir := m.store.SaveDir
	if dir == "" {
		dir = m.downloads
	}
	name := fileName(dir, m.prov.Name, m.ep.Name, m.resp.ext, m.now())
	m.ask(m.prov.Name+" › "+m.ep.Name+" › save", []question{{label: "File", value: displayPath(filepath.Join(dir, name))}}, func(a []string) string {
		path := resolvePath(a[0])
		data := m.resp.body
		if m.resp.lang != "" {
			data = []byte(m.resp.shown + "\n")
		}
		if err := saveFile(path, data); err != nil {
			return capitalize(err.Error())
		}
		m.store.SaveDir = filepath.Dir(path)
		m.saveStore()
		m.say("Saved " + displayPath(path))
		return ""
	})
}
