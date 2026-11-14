package main

import (
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestRealGlupoServer runs the link and a change against a real picoauth-server, only when asked:
//
//	PICOAUTH_SERVER=http://127.0.0.1:8686 PICOAUTH_TEST_INVITE=$(picoauth-server invite) \
//	PICOAUTH_TEST_USER=test-$RANDOM go test -run TestRealGlupoServer ./cmd/picoauth
func TestRealGlupoServer(t *testing.T) {
	server, invite, user := os.Getenv("PICOAUTH_SERVER"), os.Getenv("PICOAUTH_TEST_INVITE"), os.Getenv("PICOAUTH_TEST_USER")
	if server == "" || invite == "" || user == "" {
		t.Skip("set PICOAUTH_SERVER, PICOAUTH_TEST_INVITE and PICOAUTH_TEST_USER to run it")
	}
	keyring.MockInit()
	sm := newSyncModel(t, t.TempDir(), newFakeGitHub(t))
	sm.gl.api = server
	sm.newVault("master-pw")
	sm.withAccount()

	sm.linkGlupo(t, user, invite)
	sm.until(t, func() bool { return sm.screen == screenSync && !sm.sync.busy() })
	if sm.sync.done == "" {
		t.Fatal(sm.text())
	}
	sm.until(t, func() bool { return sm.link.Computer != "" })

	before := sm.link.SHA
	sm.screen = screenList
	sm.Rune('m')
	sm.typ("Renamed")
	sm.typ("")
	sm.until(t, func() bool { return !sm.saving && sm.link.SHA != before })
	if sm.vault.Accounts[0].Name != "Renamed" {
		t.Fatal(sm.text())
	}

	sm.Rune('s')
	sm.Rune('1')
	sm.until(t, func() bool { return sm.screen == screenForm })
	sm.typ("delete")
	sm.typ("master-pw")
	sm.until(t, func() bool { return sm.screen == screenStart })
	if !strings.Contains(sm.text(), "still on glupo.dev") {
		t.Fatal(sm.text())
	}
}
