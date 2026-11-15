package main

import (
	"slices"
	"strconv"
	"strings"

	"picotools/internal/gui"
)

// The page of one item (docs/picopassword.md, section 4.4): its data, and Edit, favourite,
// History and Delete. Every change of the vault is made here or in New, never in the list.

// itemActions are the entries of the item page, with their keys.
var itemActions = []rune{'e', 'f', 'h', 'd'}

func (m *model) openItem(it *Item) {
	m.open, m.itemCursor, m.revealed, m.screen = it.ID, 0, "", screenItem
}

// current is the item whose page is open, nil if it is gone.
func (m *model) current() *Item {
	for _, it := range m.vault.Items {
		if it.ID == m.open {
			return it
		}
	}
	return nil
}

func (m *model) itemKey(k string) {
	switch k {
	case gui.KeyUp, gui.KeyDown:
		m.itemCursor = move(m.itemCursor, k, len(itemActions))
	case gui.KeyEnter:
		m.itemRune(itemActions[m.itemCursor])
	case gui.KeyEsc:
		m.backToList()
	}
}

func (m *model) itemRune(r rune) {
	it := m.current()
	if it == nil {
		m.backToList()
		return
	}
	if i := slices.Index(itemActions, r); i >= 0 {
		m.itemCursor = i
	}
	switch r {
	case 'e':
		m.openEdit(it)
	case 'r':
		m.toggleReveal(it.ID)
	case 'f':
		it.Favourite = !it.Favourite
		if m.save(func() { it.Favourite = !it.Favourite }) {
			if it.Favourite {
				m.say("Added to favourites")
			} else {
				m.say("Removed from favourites")
			}
		}
	case 'h':
		m.histCursor, m.revealed, m.screen = 0, "", screenHistory
	case 'd':
		m.delInput, m.revealed, m.screen = nil, "", screenDelete
	}
}

// backToList goes back to the list, with the cursor on the item that was open.
func (m *model) backToList() {
	m.screen, m.revealed = screenList, ""
	if it := m.current(); it != nil {
		m.selectItem(it)
	} else {
		m.cursor = min(m.cursor, max(len(m.shown())-1, 0))
	}
}

// ---- History ----

func historyID(i int) string { return "history:" + strconv.Itoa(i) }

func (m *model) historyKey(k string) {
	it := m.current()
	if it == nil {
		m.backToList()
		return
	}
	switch k {
	case gui.KeyUp, gui.KeyDown:
		i := move(m.histCursor, k, len(it.History))
		if i != m.histCursor {
			m.revealed = ""
		}
		m.histCursor = i
	case gui.KeyEnter:
		if m.histCursor < len(it.History) {
			m.copy(it.History[m.histCursor].Password)
			m.say("Old password copied")
		}
	case gui.KeyEsc:
		m.revealed, m.screen = "", screenItem
	}
}

// ---- Delete ----

// deleteKey: yes and enter deletes the item; anything else and enter, or esc, goes back.
func (m *model) deleteKey(k string) {
	switch k {
	case gui.KeyBackspace:
		if len(m.delInput) > 0 {
			m.delInput = m.delInput[:len(m.delInput)-1]
		}
	case gui.KeyEsc:
		m.screen = screenItem
	case gui.KeyEnter:
		if strings.ToLower(strings.TrimSpace(string(m.delInput))) != "yes" {
			m.screen = screenItem
			return
		}
		it := m.current()
		if it == nil {
			m.backToList()
			return
		}
		before := m.vault.Items
		m.vault.Items = slices.DeleteFunc(slices.Clone(before), func(x *Item) bool { return x == it })
		if !m.save(func() { m.vault.Items = before }) {
			m.screen = screenItem
			return
		}
		m.screen = screenList
		m.cursor = min(m.cursor, max(len(m.shown())-1, 0))
		m.say(it.Name + " deleted")
	}
}
