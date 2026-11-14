package main

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"picotools/internal/gui"
)

// Each linked computer leaves a small file in the repository, computers/<id>.json, with its name,
// its system and the day it was linked, and deletes it when it is unlinked. So Settings can list
// the linked computers, and Pico Auth knows when the last one is being unlinked: after that nobody
// could open the vault on GitHub. A computer cannot unlink another one (docs/picoauth-sync.md, 5).

const computersDir = "computers"

func computerPath(id string) string { return computersDir + "/" + id + ".json" }

// computer is the file of a linked computer.
type computer struct {
	Name   string    `json:"name"`
	System string    `json:"system"`
	Since  time.Time `json:"since"`
	// LastUsed is when Pico Auth was last opened on this computer, even without changes.
	// It is written once at each opening, not at every check: each write is a commit.
	LastUsed time.Time `json:"last_used,omitempty"`

	id string
}

// thisComputer describes this computer: its network name and its system.
func thisComputer(now time.Time) computer {
	name, _ := os.Hostname()
	name = strings.TrimSuffix(name, ".local")
	if name == "" {
		name = "Unknown"
	}
	system := map[string]string{"darwin": "macOS", "windows": "Windows", "linux": "Linux"}[runtime.GOOS]
	if system == "" {
		system = runtime.GOOS
	}
	return computer{Name: name, System: system, Since: now.UTC(), LastUsed: now.UTC()}
}

// register writes the file of this computer, if it has none yet: after a link, after an approval,
// and for a computer linked before the list existed. A failure is tried again at the next check.
func (m *model) register() {
	if m.link == nil || m.link.Computer != "" || m.registering {
		return
	}
	id, err := newRequestID()
	if err != nil {
		return
	}
	m.registering = true
	body, _ := json.Marshal(thisComputer(m.now()))
	repo, ctx, l := m.link.Repo, m.background, m.link
	go func() {
		_, err := m.rs.putFile(ctx, repo, computerPath(id), body, "", "Pico Auth: a device is linked")
		m.ui(func() {
			m.registering = false
			if err == nil && m.link == l {
				l.Computer = id
				l.save(m.path)
				m.usedWritten = true // the new file has the time already
			}
		})
	}()
}

// markUsed writes in the file of this computer the time Pico Auth was opened, once per opening.
// A failure is not shown: the time is only information, and it is tried again at the next check.
func (m *model) markUsed() {
	if m.link == nil || m.link.Computer == "" || m.usedWritten || m.markingUsed {
		return
	}
	m.markingUsed = true
	repo, path, ctx, now := m.link.Repo, computerPath(m.link.Computer), m.background, m.now().UTC()
	go func() {
		raw, sha, err := m.rs.getFile(ctx, repo, path)
		var c computer
		if err == nil {
			err = json.Unmarshal(raw, &c)
		}
		if err == nil {
			c.LastUsed = now
			body, _ := json.Marshal(c)
			_, err = m.rs.putFile(ctx, repo, path, body, sha, "Pico Auth: opened on a device")
		}
		m.ui(func() {
			m.markingUsed = false
			if err == nil {
				m.usedWritten = true
			}
		})
	}()
}

// fetchComputers reads the files of the linked computers, the oldest first.
// It runs outside the user interface.
func (m *model) fetchComputers(ctx context.Context, repo string) ([]computer, error) {
	es, err := m.rs.listDir(ctx, repo, computersDir)
	if err != nil {
		return nil, err
	}
	var cs []computer
	for _, e := range es {
		id, ok := strings.CutSuffix(e.Name, ".json")
		if e.Type != "file" || !ok {
			continue
		}
		raw, _, err := m.rs.getFile(ctx, repo, e.Path)
		if err != nil {
			return nil, err
		}
		var c computer
		if json.Unmarshal(raw, &c) != nil {
			continue
		}
		c.id = id
		cs = append(cs, c)
	}
	sort.Slice(cs, func(a, b int) bool { return cs[a].Since.Before(cs[b].Since) })
	return cs, nil
}

// computersView is the list of linked computers on screen.
type computersView struct {
	list    []computer
	err     string
	loading bool
}

func (m *model) openComputers() {
	v := &computersView{loading: true}
	m.computers, m.screen = v, screenComputers
	repo, ctx := m.link.Repo, m.background
	go func() {
		cs, err := m.fetchComputers(ctx, repo)
		m.ui(func() {
			v.loading, v.list = false, cs
			if err != nil {
				v.err = "Could not read the list from " + m.where() + ": " + err.Error()
			}
		})
	}()
}

func (m *model) renderComputers() []line {
	v := m.computers
	lines := []line{{gui.Strong("Settings › Linked devices", nil)}, nil}
	switch {
	case v.loading:
		lines = append(lines, line{gui.Colored("Reading the list from "+m.where()+"…", gui.Muted)})
	case v.err != "":
		lines = append(lines, line{gui.Strong("✘ "+v.err, gui.Red)})
	default:
		nameW := 0
		for _, c := range v.list {
			nameW = max(nameW, len([]rune(c.Name)))
		}
		nameW = min(nameW, 40)
		for i, c := range v.list {
			l := line{
				gui.Plain("  "), gui.Strong(strconv.Itoa(i+1), gui.Accent),
				gui.Plain("  " + padRight(gui.Truncate(c.Name, nameW), nameW) + "   " + padRight(c.System, 7) + "   "),
				gui.Colored(padRight("since "+c.Since.Local().Format("2 Jan 2006"), 17), gui.Muted),
			}
			l = append(l, gui.Colored("   last "+m.when(c.LastUsed), gui.Muted))
			if m.link != nil && c.id == m.link.Computer {
				l = append(l, gui.Strong("   (this one)", gui.ValueFg))
			}
			lines = append(lines, l)
		}
		lines = append(lines, nil,
			line{gui.Colored("Each device is removed from itself, with Settings › Reset this device.", gui.Muted)})
	}
	return append(lines, m.footer("esc back")...)
}

// when shows a time briefly: "today 16:52", "21 Nov 09:03", or with the year if not this one.
func (m *model) when(t time.Time) string {
	if t.IsZero() {
		return "–"
	}
	t, now := t.Local(), m.now().Local()
	switch {
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return "today " + t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("2 Jan 15:04")
	default:
		return t.Format("2 Jan 2006 15:04")
	}
}

func padRight(s string, n int) string {
	if k := len([]rune(s)); k < n {
		return s + strings.Repeat(" ", n-k)
	}
	return s
}
