package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"picotools/internal/gui"
)

type screen int

const (
	screenConnections screen = iota // the start: list of saved connections
	screenDatabases                 // the databases of the server, when the string names none
	screenMenu                      // what to look at in the database
	screenTables                    // the tables of the database
	screenTable                     // the page of a table
	screenCreate                    // the CREATE TABLE script
	screenData                      // select all, one page at a time
	screenForm                      // questions, one after the other
	screenConfirm                   // a yes/no question
	screenBusy                      // waiting for the database
)

// Timeouts: connecting and reading the catalog or a page. Exports have none: esc stops them.
const (
	connectTimeout = 30 * time.Second
	queryTimeout   = 60 * time.Second
)

// model holds the state of Pico DB and reacts to keys.
// It knows nothing about the window: copying, quitting and running in the user
// interface are done through callbacks.
type model struct {
	store  *Store
	screen screen

	connCursor  int
	dbCursor    int
	menuCursor  int
	tableCursor int
	number      string // digits typed to pick an entry of a long list

	conn       *Connection
	eng        engine // the kind of database of the connection opened
	connString string
	server     *sql.DB // connected with the string as it is
	db         *sql.DB // the database in use: the server itself, or the one chosen
	dbName     string
	chosen     bool // the database was chosen from the list
	info       string
	dbs        []string
	tables     []table
	table      *tableInfo
	viewer     gui.Viewer
	page       int
	rows       [][]any
	gridTop    int // first row shown of the page
	gridLeft   int // first column shown

	form    *form
	confirm string
	yes     func()
	after   screen

	busy    string // what we are waiting for
	back    screen // where esc goes while busy
	cancel  context.CancelFunc
	runID   int
	started time.Time
	done    func() string // progress of an export, shown while busy

	where map[string]string // server and database of each connection, for the list

	status gui.Line
	cols   int
	height int // rows of the window

	copy func(text string)
	quit func()
	// ui runs f in the user interface and redraws; it can be called from any goroutine.
	ui        func(f func())
	now       func() time.Time
	downloads string
}

func newModel(s *Store) *model {
	home, _ := os.UserHomeDir()
	return &model{
		store:     s,
		cols:      gui.DefaultCols,
		where:     map[string]string{},
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
	if k != gui.KeyEnter && k != gui.KeyBackspace {
		m.number = ""
	}
	switch m.screen {
	case screenConnections:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.connCursor = move(m.connCursor, k, len(m.store.Connections))
		case gui.KeyEnter:
			m.openConnection(m.connCursor)
		case gui.KeyEsc:
			m.quit()
		}

	case screenDatabases:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.dbCursor = move(m.dbCursor, k, len(m.dbs))
		case gui.KeyEnter:
			m.number = ""
			m.useDatabase(m.dbCursor)
		case gui.KeyBackspace:
			m.backspace(&m.dbCursor, len(m.dbs))
		case gui.KeyEsc:
			m.closeConnection()
		}

	case screenMenu:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.menuCursor = move(m.menuCursor, k, 1)
		case gui.KeyEnter:
			m.openMenu(m.menuCursor)
		case gui.KeyEsc:
			m.leaveDatabase()
		}

	case screenTables:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.tableCursor = move(m.tableCursor, k, len(m.tables))
		case gui.KeyEnter:
			m.number = ""
			m.openTable(m.tableCursor)
		case gui.KeyBackspace:
			m.backspace(&m.tableCursor, len(m.tables))
		case gui.KeyEsc:
			m.screen = screenMenu
		}

	case screenTable:
		if k == gui.KeyEsc {
			m.screen = screenTables
			return
		}
		m.viewer.Key(k, m.cols, m.viewRoom())

	case screenCreate:
		if k == gui.KeyEsc {
			m.showTable()
			return
		}
		m.viewer.Key(k, m.cols, m.viewRoom())

	case screenData:
		room := m.gridRoom()
		switch k {
		case gui.KeyEsc:
			m.showTable()
		case gui.KeyUp:
			m.gridTop = max(m.gridTop-1, 0)
		case gui.KeyDown:
			m.gridTop = min(m.gridTop+1, max(len(m.rows)-room, 0))
		case gui.KeyPageUp:
			m.gridTop = max(m.gridTop-room, 0)
		case gui.KeyPageDown:
			m.gridTop = min(m.gridTop+room, max(len(m.rows)-room, 0))
		case gui.KeyLeft:
			m.gridLeft = max(m.gridLeft-1, 0)
		case gui.KeyRight:
			m.gridLeft = min(m.gridLeft+1, len(m.table.Columns)-1)
		case gui.KeyHome:
			m.gridTop, m.gridLeft = 0, 0
		}

	case screenForm:
		m.form.key(k)

	case screenConfirm:
		m.screen = m.after // any key other than y is no

	case screenBusy:
		if k == gui.KeyEsc {
			m.stop()
			m.screen = m.back
		}
	}
}

func (m *model) Rune(r rune) {
	if m.screen != screenForm {
		m.status = nil
	}
	if r < '0' || r > '9' {
		m.number = ""
	}
	switch m.screen {
	case screenConnections:
		switch {
		case r == 'n':
			m.newConnection()
		case r == 'm' && len(m.store.Connections) > 0:
			m.editConnection()
		case r == 'd' && len(m.store.Connections) > 0:
			m.deleteConnection()
		case r == 'q':
			m.quit()
		case r >= '1' && r <= '9':
			m.openConnection(int(r - '1'))
		}

	case screenDatabases:
		if i, ok := m.pick(r, &m.dbCursor, len(m.dbs)); ok {
			m.useDatabase(i)
		}

	case screenMenu:
		if r == '1' {
			m.openMenu(0)
		}

	case screenTables:
		if i, ok := m.pick(r, &m.tableCursor, len(m.tables)); ok {
			m.openTable(i)
		}

	case screenTable:
		switch r {
		case '1':
			m.showCreate()
		case '2':
			m.selectAll(0)
		}

	case screenCreate:
		switch r {
		case 'c':
			m.copy(m.eng.createScript(m.table))
			m.say("Copied")
		case 's':
			m.saveCreate()
		}

	case screenData:
		switch r {
		case 'n':
			if len(m.rows) == pageSize {
				m.selectAll(m.page + 1)
			}
		case 'p':
			if m.page > 0 {
				m.selectAll(m.page - 1)
			}
		case 'e':
			m.exportForm(exportCSV)
		case 'i':
			m.exportForm(exportInsert)
		case 'd':
			m.exportForm(exportDropCreate)
		}

	case screenForm:
		m.form.rune(r)

	case screenConfirm:
		m.screen = m.after
		if r == 'y' || r == 'Y' {
			m.yes()
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

// pick handles a digit typed in a list of n entries. Up to 9 entries the digit opens
// its entry at once (ok); in longer lists the digits typed so far move the cursor to
// that number and enter opens it.
func (m *model) pick(r rune, cursor *int, n int) (int, bool) {
	if r < '0' || r > '9' {
		return 0, false
	}
	if n <= 9 {
		i := int(r - '1')
		return i, i >= 0 && i < n
	}
	number := m.number + string(r)
	var i int
	fmt.Sscan(number, &i)
	if i < 1 || i > n {
		return 0, false // ignore a digit that leads nowhere
	}
	m.number, *cursor = number, i-1
	return 0, false
}

// backspace takes back the last digit typed in a long list.
func (m *model) backspace(cursor *int, n int) {
	if m.number == "" {
		return
	}
	m.number = m.number[:len(m.number)-1]
	var i int
	fmt.Sscan(m.number, &i)
	if i >= 1 && i <= n {
		*cursor = i - 1
	}
}

// ---- Forms and messages ----

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
		m.fail("Could not save: " + err.Error())
		return false
	}
	return true
}

func (m *model) say(msg string) { m.status = gui.Line{gui.Strong("✔ "+msg, gui.Amber)} }

func (m *model) fail(msg string) { m.status = gui.Line{gui.Strong("✘ "+msg, gui.Red)} }

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---- Waiting for the database ----

// run does work in the background showing what it waits for; esc stops it and goes
// back to the current screen. When work ends, its result is applied in the user
// interface: on error the error is shown on the screen we came from.
func (m *model) run(what string, timeout time.Duration, work func(ctx context.Context) (func(), error)) {
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	m.runID++
	id := m.runID
	m.cancel, m.busy, m.back, m.started, m.done = cancel, what, m.screen, m.now(), nil
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
		apply, err := work(ctx)
		timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
		cancel()
		m.ui(func() {
			if id != m.runID || m.screen != screenBusy {
				return // stopped with esc
			}
			m.screen = m.back
			if err != nil {
				if timedOut {
					err = fmt.Errorf("no answer within %d seconds", int(timeout.Seconds()))
				}
				m.fail(capitalize(err.Error()))
				return
			}
			apply()
		})
	}()
}

func (m *model) stop() {
	m.runID++
	if m.cancel != nil {
		m.cancel()
	}
}

// ---- Connections ----

func (m *model) checkName(except *Connection) func(string, []string) string {
	return func(v string, _ []string) string {
		if c := m.store.find(v); c != nil && c != except {
			return "There is already a connection named " + v
		}
		return ""
	}
}

// kindLabel is the question of the database kind, with its answers by number.
const kindLabel = "Database (1 SQL Server · 2 PostgreSQL)"

func checkKind(v string, _ []string) string {
	if len(v) != 1 || v < "1" || v > fmt.Sprint(len(kinds)) {
		return "Type 1 or 2"
	}
	return ""
}

func kindNumber(kind string) string {
	for i, k := range kinds {
		if k == kind {
			return fmt.Sprint(i + 1)
		}
	}
	return "1"
}

func kindFrom(answer string) string { return kinds[answer[0]-'1'] }

// checkConnString checks the string for the kind chosen in the answer at index kindAt.
func checkConnString(kindAt int) func(string, []string) string {
	return func(v string, a []string) string {
		if v == "" {
			return ""
		}
		if err := engineOf(kindFrom(a[kindAt])).check(v); err != nil {
			return capitalize(err.Error())
		}
		return ""
	}
}

// whereOf is the kind, server and database of a saved connection, read once from the keychain.
func (m *model) whereOf(c *Connection) string {
	if w, ok := m.where[c.Name]; ok {
		return w
	}
	e := engineOf(c.Type)
	w := e.name() + " · no connection string saved"
	if s, err := getConnString(c.Name); err != nil {
		w = "could not read the keychain"
	} else if s != "" {
		w = e.name() + " · " + e.where(s)
	}
	m.where[c.Name] = w
	return w
}

func (m *model) sortConnections() {
	sort.SliceStable(m.store.Connections, func(i, j int) bool {
		return strings.ToLower(m.store.Connections[i].Name) < strings.ToLower(m.store.Connections[j].Name)
	})
}

func (m *model) indexOf(c *Connection) int {
	for i, x := range m.store.Connections {
		if x == c {
			return i
		}
	}
	return 0
}

func (m *model) newConnection() {
	qs := []question{
		{label: "Name", check: m.checkName(nil)},
		{label: kindLabel, value: "1", check: checkKind},
		{label: "Connection string", secret: true, check: checkConnString(1)},
	}
	m.ask("New connection", qs, func(a []string) string {
		m.testAndSave(nil, a[0], kindFrom(a[1]), a[2])
		return ""
	})
}

func (m *model) editConnection() {
	c := m.store.Connections[m.connCursor]
	qs := []question{
		{label: "Name", value: c.Name, check: m.checkName(c)},
		{label: kindLabel, value: kindNumber(engineKind(c)), check: checkKind},
		{label: "Connection string (empty keeps the saved one)", secret: true, optional: true, check: checkConnString(1)},
	}
	m.ask("Edit "+c.Name, qs, func(a []string) string {
		kind, s := kindFrom(a[1]), a[2]
		if s == "" {
			var err error
			if s, err = getConnString(c.Name); err != nil {
				return "Could not read the keychain: " + err.Error()
			}
			if kind == engineKind(c) {
				if a[0] != c.Name {
					m.saveConnection(c, a[0], kind, s)
				}
				return "" // the string did not change: no need to try it again
			}
		}
		m.testAndSave(c, a[0], kind, s)
		return ""
	})
}

// engineKind is the kind of a connection: the ones saved before there was a choice are SQL Server.
func engineKind(c *Connection) string {
	if c.Type == "" {
		return kindSQLServer
	}
	return c.Type
}

// testAndSave tries the connection string; if it works it saves it, otherwise it shows
// why and asks whether to save it anyway.
func (m *model) testAndSave(c *Connection, name, kind, connString string) {
	e := engineOf(kind)
	m.run("Testing the connection…", connectTimeout, func(ctx context.Context) (func(), error) {
		db, err := e.open(connString, "")
		var info, dbName string
		if err == nil {
			defer db.Close()
			if info, err = e.serverInfo(ctx, db); err == nil {
				dbName, err = e.currentDatabase(ctx, db)
			}
		}
		if err != nil {
			msg := "Could not connect: " + err.Error()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				msg = fmt.Sprintf("Could not connect: no answer within %d seconds", int(connectTimeout.Seconds()))
			}
			return func() {
				m.askYes(msg+" · Save it anyway?", func() { m.saveConnection(c, name, kind, connString) })
			}, nil
		}
		return func() {
			if m.saveConnection(c, name, kind, connString) {
				m.say("Saved " + name + " · " + info + " · database " + dbName)
			}
		}, nil
	})
}

// saveConnection saves a new connection (c nil) or changes c: name, kind and string.
func (m *model) saveConnection(c *Connection, name, kind, connString string) bool {
	if c != nil && c.Name != name {
		if err := deleteConnString(c.Name); err != nil {
			m.fail("Could not change the keychain: " + err.Error())
			return false
		}
	}
	if err := setConnString(name, connString); err != nil {
		m.fail("Could not save the connection string in the keychain: " + err.Error())
		return false
	}
	if c == nil {
		c = &Connection{}
		m.store.Connections = append(m.store.Connections, c)
	} else {
		delete(m.where, c.Name)
	}
	c.Name, c.Type = name, kind
	delete(m.where, name)
	m.sortConnections()
	m.connCursor = m.indexOf(c)
	if !m.saveStore() {
		return false
	}
	m.say("Saved " + name)
	return true
}

func (m *model) deleteConnection() {
	c := m.store.Connections[m.connCursor]
	m.askYes("Delete the connection "+c.Name+" and its connection string?", func() {
		if err := deleteConnString(c.Name); err != nil {
			m.fail("Could not delete the connection string from the keychain: " + err.Error())
			return
		}
		cs := m.store.Connections
		i := m.indexOf(c)
		m.store.Connections = append(cs[:i:i], cs[i+1:]...)
		delete(m.where, c.Name)
		m.connCursor = min(m.connCursor, max(len(m.store.Connections)-1, 0))
		if m.saveStore() {
			m.say("Deleted " + c.Name)
		}
	})
}

// ---- Opening a database ----

func (m *model) openConnection(i int) {
	if i < 0 || i >= len(m.store.Connections) {
		return
	}
	m.connCursor = i
	c := m.store.Connections[i]
	e := engineOf(c.Type)
	s, err := getConnString(c.Name)
	if err != nil {
		m.fail("Could not read the keychain: " + err.Error())
		return
	}
	if s == "" {
		m.fail("No connection string saved for " + c.Name + ": press m to set it")
		return
	}
	choose := !e.namesDatabase(s)
	m.run("Connecting to "+c.Name+"…", connectTimeout, func(ctx context.Context) (func(), error) {
		db, err := e.open(s, "")
		if err != nil {
			return nil, err
		}
		info, err := e.serverInfo(ctx, db)
		var dbName string
		var dbs []string
		if err == nil && choose {
			dbs, err = e.databases(ctx, db)
		} else if err == nil {
			dbName, err = e.currentDatabase(ctx, db)
		}
		if err != nil {
			db.Close()
			return nil, err
		}
		return func() {
			m.conn, m.eng, m.connString, m.server, m.info = c, e, s, db, info
			if choose {
				m.dbs, m.dbCursor, m.screen = dbs, 0, screenDatabases
				return
			}
			m.db, m.dbName, m.chosen = db, dbName, false
			m.menuCursor, m.screen = 0, screenMenu
		}, nil
	})
}

// useDatabase connects to database i of the server, with the same connection string.
func (m *model) useDatabase(i int) {
	if i < 0 || i >= len(m.dbs) {
		return
	}
	m.dbCursor = i
	name := m.dbs[i]
	m.run("Opening "+name+"…", connectTimeout, func(ctx context.Context) (func(), error) {
		db, err := m.eng.open(m.connString, name)
		if err != nil {
			return nil, err
		}
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, err
		}
		return func() {
			m.db, m.dbName, m.chosen = db, name, true
			m.menuCursor, m.screen = 0, screenMenu
		}, nil
	})
}

// leaveDatabase goes back from the menu: to the list of databases or to the connections.
func (m *model) leaveDatabase() {
	m.tables, m.table = nil, nil
	if m.chosen {
		m.db.Close()
		m.db, m.screen = nil, screenDatabases
		return
	}
	m.closeConnection()
}

func (m *model) closeConnection() {
	if m.db != nil && m.db != m.server {
		m.db.Close()
	}
	if m.server != nil {
		m.server.Close()
	}
	m.server, m.db, m.conn, m.eng, m.connString = nil, nil, nil, nil, ""
	m.dbs, m.tables, m.table = nil, nil, nil
	m.screen = screenConnections
}

// ---- Tables ----

func (m *model) openMenu(i int) {
	if i != 0 {
		return
	}
	m.run("Reading the tables…", queryTimeout, func(ctx context.Context) (func(), error) {
		list, err := m.eng.tables(ctx, m.db)
		if err != nil {
			return nil, err
		}
		return func() {
			m.tables, m.tableCursor, m.screen = list, 0, screenTables
		}, nil
	})
}

func (m *model) openTable(i int) {
	if i < 0 || i >= len(m.tables) {
		return
	}
	m.tableCursor = i
	t := m.tables[i]
	m.run("Reading "+t.full()+"…", queryTimeout, func(ctx context.Context) (func(), error) {
		info, err := m.eng.describe(ctx, m.db, t)
		if err != nil {
			return nil, err
		}
		return func() {
			m.table = info
			m.tables[i].Rows = info.Rows
			m.showTable()
		}, nil
	})
}

func (m *model) showTable() {
	m.viewer.Set(tablePage(m.table), "", false)
	m.screen = screenTable
}

func (m *model) showCreate() {
	m.viewer.Set(strings.TrimRight(m.eng.createScript(m.table), "\n"), "", true)
	m.screen = screenCreate
}

// saveForm asks where to save a file, proposing a name in the last folder used.
func (m *model) saveForm(title, base, ext string, save func(path string) string) {
	dir := m.store.SaveDir
	if dir == "" {
		dir = m.downloads
	}
	name := fileName(dir, base, ext, m.now())
	m.ask(title, []question{{label: "File", value: displayPath(filepath.Join(dir, name))}}, func(a []string) string {
		path := resolvePath(a[0])
		if msg := save(path); msg != "" {
			return msg
		}
		m.store.SaveDir = filepath.Dir(path)
		m.saveStore()
		return ""
	})
}

func (m *model) saveCreate() {
	t := m.table
	m.saveForm(m.crumbs()+" › save", t.full()+"-create", ".sql", func(path string) string {
		if err := saveFile(path, []byte(m.eng.createScript(t))); err != nil {
			return capitalize(err.Error())
		}
		m.say("Saved " + displayPath(path))
		return ""
	})
}

// ---- Select all ----

func (m *model) selectAll(page int) {
	t := m.table
	m.run(fmt.Sprintf("Reading page %d of %s…", page+1, t.full()), queryTimeout, func(ctx context.Context) (func(), error) {
		rows, err := readPage(ctx, m.eng, m.db, t, page)
		if err != nil {
			return nil, err
		}
		return func() {
			if page > 0 && len(rows) == 0 {
				m.say("No more rows")
				return
			}
			m.page, m.rows, m.gridTop = page, rows, 0
			if m.screen != screenData {
				m.gridLeft = 0
			}
			m.screen = screenData
		}, nil
	})
}

// exportForm asks the file and then writes all rows of the table in it.
func (m *model) exportForm(kind int) {
	t := m.table
	base, ext := t.full(), ".csv"
	switch kind {
	case exportInsert:
		base, ext = t.full()+"-insert", ".sql"
	case exportDropCreate:
		base, ext = t.full()+"-drop-create-insert", ".sql"
	}
	m.saveForm(m.crumbs()+" › export", base, ext, func(path string) string {
		f, err := createFile(path)
		if err != nil {
			return capitalize(err.Error())
		}
		m.export(kind, f, path)
		return ""
	})
}

func (m *model) export(kind int, f *os.File, path string) {
	t := m.table
	var written int64
	m.run("Exporting "+t.full()+" to "+displayPath(path)+"…", 0, func(ctx context.Context) (func(), error) {
		n, err := exportRows(ctx, m.eng, m.db, t, kind, f, func(n int64) {
			m.ui(func() { written = n })
		})
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(path) // never leave half a file
			if errors.Is(ctx.Err(), context.Canceled) {
				return func() {}, nil
			}
			return nil, err
		}
		return func() { m.say(fmt.Sprintf("Exported %s rows to %s", thousands(n), displayPath(path))) }, nil
	})
	// After run: the export goes on until the end, esc stops it and removes the file.
	m.done = func() string { return thousands(written) + " rows written" }
}

func thousands(n int64) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return s
	}
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}
