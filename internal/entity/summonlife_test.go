package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A summon's life (board 1660): an add called by a summon with life 2 is in
// the world just before 2 s after it rose and gone just after, with no corpse,
// and #stats says it goes; the control, a summon with no life, keeps its add
// past that.
func TestASummonsAddsLeaveAfterTheirLife(t *testing.T) {
	for _, life := range []string{`,"life":2`, ``} {
		r := newTemplateRig(t)
		r.godTemplate("summoner", ", class, hp_pct", ", 0, 200",
			`[{"kind":"summon","below":25,"count":1,"npc":"Gnoll Pup","share":50`+life+`}]`, "gnoll guard")
		guard := r.npc("Gnoll Guard")
		p, _ := r.player("Tank", guard.X+1, guard.Z)
		p.HP, p.MaxHP = 10_000_000, 10_000_000
		r.fight(guard, p.ServerID)
		guard.HP = guard.MaxHP * 24 / 100
		var add *NPC
		var rose time.Duration
		for d := time.Duration(0); d <= time.Second && add == nil; d += 100 * time.Millisecond {
			r.godTick(d)
			for _, n := range r.w.NPCs {
				if n.add != nil && n.add.by == guard {
					add, rose = n, d
				}
			}
		}
		if add == nil {
			t.Fatalf("life %q: no add", life)
		}
		if words := addWords(add); (life != "") != (strings.Contains(words, "it goes in")) {
			t.Errorf("life %q: #stats %q", life, words)
		}
		r.run(rose+100*time.Millisecond, rose+1900*time.Millisecond)
		if r.w.NPCs[add.ServerID] != add {
			t.Fatalf("life %q: gone before 2 s", life)
		}
		r.run(rose+2000*time.Millisecond, rose+2200*time.Millisecond)
		gone := r.w.NPCs[add.ServerID] != add
		if gone != (life != "") || add.Dead {
			t.Errorf("life %q: gone %v after 2 s (dead %v)", life, gone, add.Dead)
		}
	}
}

// life is a summon's alone, 0 or 2 to 3600 seconds.
func TestASummonsLifeIsChecked(t *testing.T) {
	for js, ok := range map[string]bool{
		`[{"kind":"summon","below":25,"count":1,"npc":"Pup","life":90}]`:   true,
		`[{"kind":"summon","below":25,"count":1,"npc":"Pup","life":1}]`:    false,
		`[{"kind":"summon","below":25,"count":1,"npc":"Pup","life":4000}]`: false,
		`[{"kind":"heal","heal_pct":10,"below":50,"every":60,"life":90}]`:  false,
	} {
		if _, err := database.ParseTemplateAbilities(js); (err == nil) != ok {
			t.Errorf("%s: %v", js, err)
		}
	}
}
