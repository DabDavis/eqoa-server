package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A heal shield: below its share of hit points a melee hit heals the creature and 1 lands;
// above it, and with no template, the hit is the hit.
func TestHealShieldTurnsMeleeToHealing(t *testing.T) {
	n := &NPC{MaxHP: 1000, HP: 900, tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{TakenPct: 100}, healBelow: 50}}
	if got := n.meleeTaken(100); got != 100 || n.HP != 900 {
		t.Fatalf("above its share: %d lands, HP %d", got, n.HP)
	}
	n.HP = 400
	if got := n.meleeTaken(100); got != 1 || n.HP != 500 {
		t.Fatalf("below its share: %d lands, HP %d, want 1 and healed to 500", got, n.HP)
	}
	n.HP = 490
	n.tmpl.healBelow = 100
	n.HP = 999
	if got := n.meleeTaken(100); got != 1 || n.HP != 1000 {
		t.Fatalf("a heal stops at the most: %d lands, HP %d", got, n.HP)
	}
	bare := &NPC{MaxHP: 1000, HP: 10}
	if got := bare.meleeTaken(100); got != 100 || bare.HP != 10 {
		t.Fatalf("no template: %d, HP %d", got, bare.HP)
	}
}

// A kind of weapon a template names lands at its share, the others whole.
func TestMeleeKindShare(t *testing.T) {
	n := &NPC{tmpl: &ClassTemplate{kindTaken: map[Stat]int{StatSlash: 50, StatPierce: 150}}}
	for kind, want := range map[Stat]int{StatSlash: 50, StatPierce: 150, StatBlunt: 100} {
		if got := n.kindTakenOf(kind, 100); got != want {
			t.Errorf("kind %v: %d, want %d", kind, got, want)
		}
	}
	if got := (&NPC{}).kindTakenOf(StatSlash, 100); got != 100 {
		t.Errorf("no template: %d", got)
	}
}

// A template's resist_kind may name several: a spell of one of them is cut, another whole.
func TestResistKindsSeveral(t *testing.T) {
	n := &NPC{tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{ResistKind: "FR, CR,LR", ResistPct: 80}}}
	if !resistKindHas(n.tmpl.ResistKind, StatCR) || resistKindHas(n.tmpl.ResistKind, StatAR) {
		t.Fatal("the list")
	}
	if got := creatureResists(StatLR, n, -1000, time.Now()); got != -200 {
		t.Errorf("LR: %d, want -200", got)
	}
	if got := creatureResists(StatAR, n, -1000, time.Now()); got != -1000 {
		t.Errorf("AR: %d, want whole", got)
	}
}
