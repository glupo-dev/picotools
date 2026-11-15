package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// TestRealGlupoServer runs the link and a change against a real picopassword-server, only when asked:
//
//	PICOPASSWORD_SERVER=http://127.0.0.1:8687 PICOPASSWORD_TEST_INVITE=$(picopassword-server invite) \
//	PICOPASSWORD_TEST_USER=test-$RANDOM go test -run TestRealGlupoServer ./cmd/picopassword
func TestRealGlupoServer(t *testing.T) {
	server, invite, user := os.Getenv("PICOPASSWORD_SERVER"), os.Getenv("PICOPASSWORD_TEST_INVITE"), os.Getenv("PICOPASSWORD_TEST_USER")
	if server == "" || invite == "" || user == "" {
		t.Skip("set PICOPASSWORD_SERVER, PICOPASSWORD_TEST_INVITE and PICOPASSWORD_TEST_USER to run it")
	}
	keyring.MockInit()
	sm := newSyncModel(t, t.TempDir(), newFakeGitHub(t))
	sm.gl.api = server
	sm.newVault("master-pw")
	sm.withItem()

	sm.linkGlupo(t, user, invite)
	sm.until(t, func() bool { return sm.screen == screenSync && !sm.sync.busy() })
	if sm.sync.done == "" {
		t.Fatal(sm.text())
	}
	sm.until(t, func() bool { return sm.link.Computer != "" })

	before := sm.link.SHA
	sm.screen = screenList
	sm.Rune('o')
	editName(sm, "Renamed")
	sm.until(t, func() bool { return !sm.saving && sm.link.SHA != before })
	if sm.vault.Items[0].Name != "Renamed" {
		t.Fatal(sm.text())
	}
	sm.screen = screenList

	// A second device, approved by the first one.
	pc := glupoPC(t, newFakeGlupo(t))
	pc.gl.api = server
	pc.Rune('2')
	pc.Rune('2')
	pc.typ(user)
	pc.typ("master-pw")
	pc.until(t, func() bool { return pc.screen == screenSync && pc.sync.pair != "" })
	sm.lastCheck = time.Time{}
	sm.tick()
	sm.until(t, func() bool { return sm.screen == screenApprove })
	sm.Rune('y')
	sm.typ("master-pw")
	pc.until(t, func() bool { return pc.screen == screenList })
	if pc.vault == nil || pc.vault.Items[0].Name != "Renamed" {
		t.Fatal(pc.text())
	}
	sm.until(t, func() bool { return sm.screen == screenList && !strings.Contains(sm.text(), "Approving") })

	sm.Rune('s')
	sm.Rune('4')
	sm.until(t, func() bool { return sm.screen == screenForm })
	sm.typ("delete")
	sm.typ("master-pw")
	sm.until(t, func() bool { return sm.screen == screenStart })
	if !strings.Contains(sm.text(), "still on glupo.dev") {
		t.Fatal(sm.text())
	}
}
