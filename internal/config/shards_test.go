package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A shard's config: a port of 0 stays 0 (no such listener), a key left out is its
// default, the list and the off switches read; a config with none of it is as ever.
func TestAShardsConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(`
server:
  game_port: 10071
  auth_port: 0
  patch_port: 0
  account_port: 0
patch:
  worlds_udp_port: 0
auth:
  shared_db: "/srv/one/eqoa.db"
world:
  heirlooms: false
shards:
  - name: "Heirloom Test Server"
    port: 10070
  - name: "Live Server"
    port: 10071
    recommended: true
`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.GamePort != 10071 || c.Server.AuthPort != 0 || c.Server.PatchPort != 0 || c.Server.AccountPort != 0 || c.Patch.WorldsUDPPort != 0 {
		t.Errorf("ports: %+v %d", c.Server, c.Patch.WorldsUDPPort)
	}
	if c.Auth.SharedDB != "/srv/one/eqoa.db" || c.World.Heirlooms == nil || *c.World.Heirlooms {
		t.Errorf("shared db %q, heirlooms %v", c.Auth.SharedDB, c.World.Heirlooms)
	}
	if len(c.Shards) != 2 || c.Shards[1].Name != "Live Server" || !c.Shards[1].Recommended || c.Shards[0].Recommended || c.Shards[0].Port != 10070 {
		t.Errorf("shards: %+v", c.Shards)
	}
	os.WriteFile(path, []byte("server:\n  server_ip: \"192.0.2.1\"\n"), 0o644)
	c, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.AuthPort != 9735 || c.Server.PatchPort != 7000 || c.Server.AccountPort != 8080 || c.Patch.WorldsUDPPort != 7003 ||
		c.Auth.SharedDB != "" || c.World.Heirlooms != nil || len(c.Shards) != 0 {
		t.Errorf("the defaults moved: %+v", c)
	}
}

// world.start_tunar: unset, 5000 (Heirloom as ever); 0, none (Live, the owner 2026-11-18).
func TestStartTunar(t *testing.T) {
	for _, c := range []struct {
		yaml string
		want int
	}{{"world:\n  start_level: 1\n", 5000}, {"world:\n  start_tunar: 0\n", 0}, {"world:\n  start_tunar: 250\n", 250}} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		os.WriteFile(path, []byte(c.yaml), 0o644)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.World.StartingTunar(); got != c.want {
			t.Errorf("%q: %d, want %d", c.yaml, got, c.want)
		}
	}
}
