package entity

import (
	"testing"
	"time"
)

// The mob stays on a tank outside the pet owner's group. An area hit on a
// bystander must work even when the victim has never generated threat.
func TestAggressivePetsAssistPlayerDamage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		actor    string
		outgoing bool
		stance   petStance
		held     bool
		ownerX   float32
		want     bool
	}{
		{"owner takes damage", "owner", false, petAggressive, false, 5, true},
		{"groupmate takes damage", "groupmate", false, petAggressive, false, 18, true},
		{"owner deals damage", "owner", true, petAggressive, false, 18, true},
		{"groupmate deals damage", "groupmate", true, petAggressive, false, 18, true},
		{"stranger takes damage", "stranger", false, petAggressive, false, 18, false},
		{"stranger deals damage", "stranger", true, petAggressive, false, 18, false},
		{"passive pet", "groupmate", false, petPassive, false, 18, false},
		{"backed-off pet", "groupmate", false, petAggressive, true, 18, false},
		{"outside assist range", "groupmate", false, petAggressive, false, 35, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, boss, tank, _ := rampageRig(t, `[{"kind":"rampage","chance":100}]`)
			boss.Threats[tank.ServerID] = 1_000_000
			owner, sess := r.rampagePlayer("Owner", boss.X+tc.ownerX, boss.Z)
			actor := owner
			if tc.actor != "owner" {
				actor, sess = r.rampagePlayer("Other player", boss.X+5, boss.Z)
				if tc.actor == "groupmate" {
					r.group([]*Player{owner, actor})
				}
			}
			// Both companions stand outside the rampage and within assist range
			// of the boss. Only the human player takes or deals the damage.
			pet := r.petOf(owner, 0x00300001, "Pet", false, boss.X+18, boss.Z)
			ps := &petState{owner: owner, npc: pet, stance: tc.stance}
			r.w.pets = map[EntityID]*petState{owner.ServerID: ps}
			merc := r.petOf(owner, 0x00300002, "Merc", true, boss.X+18, boss.Z).merc
			merc.stance = MercAggressive
			now := r.at(time.Second)
			if tc.held {
				ps.heldUntil = now.Add(petBackOffHold)
			}
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			if tc.outgoing {
				before := boss.HP
				r.w.spellHitLocked(actor, sess, boss, 100, numberSpell)
				if boss.HP >= before {
					t.Fatal("the player's spell did not damage the mob")
				}
			} else {
				before := actor.HP
				r.w.areaRampageLocked(boss, tank.ServerID, now)
				if actor.HP >= before {
					t.Fatal("the area attack did not damage the player")
				}
				if _, listed := boss.Threats[actor.ServerID]; listed {
					t.Fatal("the bystander appeared on the mob's threat list")
				}
				r.w.mercChooseTargetLocked(merc)
				if merc.target != 0 {
					t.Fatal("the bystander's incoming hit called a mercenary")
				}
			}
			if boss.CurrentEnemy != tank.ServerID {
				t.Fatal("the mob changed targets")
			}
			r.w.petChooseTargetLocked(ps, now)
			if got := ps.target == boss.ServerID; got != tc.want {
				t.Errorf("pet assisted = %v, want %v", got, tc.want)
			}
		})
	}
}

// A dummy accounts for damage separately and never adds the player to a mob
// threat list. Both normal player damage paths must still call the pet.
func TestAggressivePetAssistsPlayerHitsWithoutMobThreat(t *testing.T) {
	for _, kind := range []string{"melee", "spell"} {
		t.Run(kind, func(t *testing.T) {
			r := newPetRig(t)
			r.summon(t, fireElemental(), 0)
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			ps := r.w.pets[r.p.ServerID]
			ps.stance = petAggressive
			r.enemy.Dummy = TrainingDummy
			if kind == "melee" {
				r.w.playerHitLocked(r.sess, r.p, r.enemy, r.w.objectIDForEntityLocked(r.sess, r.enemy.ServerID))
			} else {
				r.w.spellHitLocked(r.p, r.sess, r.enemy, 100, numberSpell)
			}
			if r.enemy.dummyHate[r.p.ServerID] <= 0 || len(r.enemy.Threats) != 0 || r.p.Attacking() {
				t.Fatal("expected a dummy hit without normal threat or auto-attack")
			}
			r.w.petChooseTargetLocked(ps, r.at(10*time.Second))
			if ps.target != r.enemy.ServerID {
				t.Fatal("the pet missed the player's hit")
			}
		})
	}
}

// Neither an NPC's spell nor its DoT needs the victim to be its current enemy.
// A pet gets a turn to answer a hit that arrived after its previous turn.
func TestAggressivePetAssistsLandedSpellsAndTicks(t *testing.T) {
	for _, kind := range []string{"spell", "damage over time"} {
		t.Run(kind, func(t *testing.T) {
			r := newPetRig(t)
			r.summon(t, fireElemental(), 0)
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			ps := r.w.pets[r.p.ServerID]
			ps.stance = petAggressive
			now := r.at(10 * time.Second)
			r.w.damagePlayerFromNPCLocked(r.enemy, r.p, 0)
			r.w.petChooseTargetLocked(ps, now)
			if ps.target != 0 {
				t.Fatal("a hit for zero called the pet")
			}
			before := r.p.HP
			if kind == "spell" {
				cast := &npcCast{npc: r.enemy, spell: &CastSpell{Name: "Blast"}, fixed: 20}
				r.w.templateHitLocked(cast, r.p.ServerID, nil, now)
			} else {
				dot := &ActiveEffect{Name: "Poison", HPTick: -20, npcFoe: r.enemy.ServerID}
				r.w.npcTickLocked(r.p, dot, now)
			}
			if r.p.HP >= before || r.enemy.CurrentEnemy != 0 || len(r.enemy.Threats) != 0 {
				t.Fatal("expected damage without an attack target or threat")
			}
			r.w.TickCount++
			r.w.petSystem(now.Add(100 * time.Millisecond))
			if ps.target != r.enemy.ServerID {
				t.Fatal("the pet missed damage from the previous tick")
			}
			ps.target = 0
			r.w.TickCount++
			r.w.petSystem(now.Add(200 * time.Millisecond))
			if ps.target != 0 {
				t.Fatal("the pet reacquired a mob from an expired hit")
			}
		})
	}
}
