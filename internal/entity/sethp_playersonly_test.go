package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Siliskor's poison round himself (work board 708: 600 a tick for 30 s, radius
// 40, as a sethp that sets no one's hit points): players_only reaches the
// players and their side (a pet) alone, never Pomus Magemell or the
// worshippers beside him. The control: without it his own side is reached.
func TestASetHPPlayersOnlySparesItsOwnSide(t *testing.T) {
	for _, only := range []bool{true, false} {
		r := newSetHPRig(t, database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 1000000, Radius: 40, Every: 30,
			Tick: 600, Resist: "PR", TickFor: 30, PlayersOnly: only, Name: "Siliskor's Venom"})
		other, _ := r.player(1002, "Other", 5000, 1020, 1010)
		worshipper := r.f().npc(3201, "dragon worshipper", 53, creationBody, 1015, 1005)
		worshipper.HP, worshipper.MaxHP = 20000, 20000
		pet := r.f().npc(3202, "a pet", 40, creationBody, 1000, 1005)
		pet.HP, pet.MaxHP, pet.Owner = 3000, 3000, other.ServerID
		r.at(0)
		r.at(6 * time.Second)
		if r.p.HP >= 5000 || other.HP >= 5000 || pet.HP >= 3000 {
			t.Fatalf("players_only %v: the players' side untouched: %d, %d, pet %d", only, r.p.HP, other.HP, pet.HP)
		}
		r.w.mu.Lock()
		r.w.effectSystem(r.now.Add(7 * time.Second))
		r.w.mu.Unlock()
		hit := worshipper.HP < 20000 || len(worshipper.Effects) > 0
		if only && hit {
			t.Fatalf("players_only: the worshipper reached (%d, %d effects)", worshipper.HP, len(worshipper.Effects))
		}
		if !only && !hit {
			t.Fatal("without players_only his own side is reached (the control)")
		}
	}
}

// players_only is a sethp's with a radius: refused elsewhere.
func TestPlayersOnlyIsASetHPAreasAlone(t *testing.T) {
	for _, a := range []database.TemplateAbility{
		{Kind: database.AbilitySetHP, SetHP: 700, Every: 30, PlayersOnly: true},
		{Kind: database.AbilityNuke, Every: 30, PlayersOnly: true},
	} {
		if err := database.CheckTemplateAbilities([]database.TemplateAbility{a}); err == nil {
			t.Errorf("%+v passed", a)
		}
	}
	ok := database.TemplateAbility{Kind: database.AbilitySetHP, SetHP: 700, Radius: 25, Every: 30, PlayersOnly: true}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{ok}); err != nil {
		t.Errorf("a sethp area's players_only refused: %v", err)
	}
}
