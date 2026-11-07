package main

import (
	"os"
	"strings"
	"testing"
)

// The start's wiring of a shard whose accounts are another's (auth.shared_db), a
// source check as the start is not a function a test can run; the rig's probes
// (claude-notes/shards-20261107.md) ran it whole. The sign-in must hold whatever
// auth.require_tcp_login says, leave no ticket (a ticket names a username alone,
// and nothing here ever saw that username's password but the handshake that asks),
// and the mirror must stop before a restart holds the database.
func TestMainWiresASharedShard(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{
		"if cfg.Auth.RequireTCPLogin || sharedAccounts {\n\t\thandlerRegistry.SetLoginTickets(loginTickets)",
		"if cfg.Auth.RequireTCPLogin || sharedAccounts {\n\t\t// a client back at the server list",
		"if sharedAccounts {\n\t\tauthGrants = nil\n\t}",
		"tcp.NewAuthServer(cfg, accountStore, authGrants)",
		"authHandler.SetAccountRefresh(mirror.PullOne)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("main.go does not say %q", want)
		}
	}
	hold, restart := strings.Index(s, "mirror.Hold(true)"), strings.Index(s, "udpListener.KeepRestart(")
	if hold < 0 || restart < 0 || hold > restart {
		t.Errorf("the mirror is not held before a keep restart (%d, %d)", hold, restart)
	}
	// a port of 0 starts no listener: each start behind its own test
	for _, want := range []string{
		"if cfg.Server.AuthPort != 0 {\n\t\tauthListener :=",
		"if cfg.Server.PatchPort != 0 {\n\t\tpatchServer :=",
		"if cfg.Server.AccountPort != 0 {\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("main.go does not say %q", want)
		}
	}
}
