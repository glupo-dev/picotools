package main

import (
	"strings"
	"testing"
)

// testModel runs the tools at once, without the window.
func testModel() *model {
	m := &model{cols: 100, rows: 30, copy: func(string) {}, quit: func() {}}
	m.background = func(work func() ([]Result, error), done func([]Result, error)) { done(work()) }
	m.ui = func(f func()) { f() }
	return m
}

func screenText(m *model) string {
	var b strings.Builder
	for _, l := range m.Render(m.cols, m.rows) {
		for _, s := range l {
			b.WriteString(s.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// openJSONTool opens the tool with the given name in the JSON/XML menu.
func openJSONTool(t *testing.T, m *model, name string) {
	for i, top := range tools {
		if top.Name != "JSON/XML" {
			continue
		}
		m.openTool(i)
		for j, tool := range top.Menu {
			if tool.Name == name {
				m.openTool(j)
				return
			}
		}
	}
	t.Fatalf("no tool %q", name)
}

func TestViewJSON(t *testing.T) {
	m := testModel()
	openJSONTool(t, m, "View JSON/XML")
	var items []string
	for i := range 40 {
		items = append(items, `{"n":`+string(rune('0'+i%10))+`}`)
	}
	m.Paste("[" + strings.Join(items, ",") + "]")
	m.Key(keyEnter)
	if m.screen != screenView {
		t.Fatalf("screen %d", m.screen)
	}
	text := screenText(m)
	if !strings.Contains(text, "  1  [") || !strings.Contains(text, `"n": 0`) || !strings.Contains(text, "122 lines") {
		t.Errorf("first page:\n%s", text)
	}
	m.Key(keyEnd)
	if text := screenText(m); !strings.Contains(text, "122  ]") || !strings.Contains(text, "100%") {
		t.Errorf("after end:\n%s", text)
	}
	m.Key(keyHome)
	if text := screenText(m); !strings.Contains(text, "  1  [") {
		t.Errorf("after home:\n%s", text)
	}
	m.Key(keyEsc)
	if m.screen != screenInput {
		t.Errorf("esc: screen %d", m.screen)
	}
}

func TestViewXMLPlain(t *testing.T) {
	m := testModel()
	openJSONTool(t, m, "Structure of JSON/XML")
	m.Paste("<a><b>1</b><b>2</b></a>")
	m.Key(keyEnter)
	if m.screen != screenView {
		t.Fatalf("screen %d", m.screen)
	}
	text := screenText(m)
	if strings.Contains(text, "  1  ") || strings.Contains(text, " lines") {
		t.Errorf("plain view should have no line numbers:\n%s", text)
	}
	var copied string
	m.copy = func(s string) { copied = s }
	m.Rune('c')
	if copied == "" || m.screen != screenInput {
		t.Errorf("copy: %q, screen %d", copied, m.screen)
	}
}
