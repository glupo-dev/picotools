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
	if (r == 'e' || r == 'f' || r == 'd') && !m.canChange() {
		return
	}
	switch r {
	case 'e':
		m.openEdit(it)
	case 'r':
		m.toggleReveal(it.ID)
	case 'f':
		id, fav := it.ID, !it.Favourite
		m.apply(func(items []*Item) ([]*Item, error) {
			i := findItem(items, id)
			if i < 0 {
				return nil, errDeletedElsewhere
			}
			items[i].Favourite = fav
			return items, nil
		}, func() {
			if fav {
				m.say("Added to favourites")
			} else {
				m.say("Removed from favourites")
			}
		})
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
		m.screen = screenItem
		if !m.canChange() {
			return
		}
		id, name := it.ID, it.Name
		m.apply(func(items []*Item) ([]*Item, error) {
			// Already deleted on another device: nothing to do (a delete wins, docs 5.2).
			return slices.DeleteFunc(items, func(x *Item) bool { return x.ID == id }), nil
		}, func() {
			if m.screen == screenItem && m.open == id {
				m.screen = screenList
			}
			m.clampCursor()
			m.say(name + " deleted")
		})
	}
}
