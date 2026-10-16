package tcp

import (
	"testing"
)

// The SD override is RETIRED, and the test that used to prove it changed the
// served bytes now proves it changes nothing at all.
//
// SD is not served — EQOAHDD.ELF installs it (elfDelivered) — so an override
// could only name a file this server would refuse a moment later. Setting one
// must not resurrect the entry by a side door: an override is applied inside
// scanServeDir, and if the refusal and the override were ordered the other way
// round, `EQOA_SD_FILE=anything` would put SD back in the manifest and hand every
// card a second writer, from a variable an operator set for an unrelated reason.
//
// A payload is swapped by building the tool against a different eqscene/card/SD.
func TestSDFileOverrideIsRefused(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		if sd := find(newTestServer(t), "SD"); sd != nil {
			t.Fatalf("SD is served by default (%d bytes)", len(sd.Content))
		}
	})

	t.Run("override cannot put it back", func(t *testing.T) {
		t.Setenv("EQOA_SD_FILE", "SD-disc8")
		if sd := find(newTestServer(t), "SD"); sd != nil {
			t.Fatalf("EQOA_SD_FILE resurrected the SD entry (%d bytes) — the "+
				"override is applied before the refusal, so a stale config key "+
				"gives the card a second writer", len(sd.Content))
		}
	})
}

// A bad name must fall back to the default, not serve nothing.
func TestSDFileOverrideRejectsPaths(t *testing.T) {
	t.Setenv("EQOA_SD_FILE", "../patches/SD")
	if got := resolvePayload("EQOA_SD_FILE", "", defaultSDFile); got != defaultSDFile {
		t.Fatalf("path-bearing name accepted: %q", got)
	}
}

// Precedence is env > config > built-in default. The environment has to outrank
// config: swapping a payload already costs a restart for the two-pass Delete
// dance, and a bisect must not additionally require editing config.yaml.
func TestPayloadResolutionPrecedence(t *testing.T) {
	t.Run("default when nothing set", func(t *testing.T) {
		if got := resolvePayload("EQOA_SD_FILE", "", defaultSDFile); got != defaultSDFile {
			t.Fatalf("got %q, want the built-in default %q", got, defaultSDFile)
		}
	})

	t.Run("config beats default", func(t *testing.T) {
		if got := resolvePayload("EQOA_SD_FILE", "SD-disc8", defaultSDFile); got != "SD-disc8" {
			t.Fatalf("config value ignored: got %q", got)
		}
	})

	t.Run("env beats config", func(t *testing.T) {
		t.Setenv("EQOA_SD_FILE", "SD-test")
		if got := resolvePayload("EQOA_SD_FILE", "SD-disc8", defaultSDFile); got != "SD-test" {
			t.Fatalf("env did not win over config: got %q", got)
		}
	})

	// A path in config is as dangerous as one in the environment -- both would
	// escape the patch directory -- so it falls back rather than being joined.
	t.Run("path-bearing config rejected", func(t *testing.T) {
		if got := resolvePayload("EQOA_SD_FILE", "../SD", defaultSDFile); got != defaultSDFile {
			t.Fatalf("path-bearing config accepted: %q", got)
		}
	})
}
