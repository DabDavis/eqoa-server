package entity

import (
	"bytes"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Ruggid's duel (2026-11-13, Heirloom, 23:04): his Swarming Death fought a training dummy, died,
// was summoned again and buffed (a proc on it, as Soul Mastery +), and in a duel with Hoofbite it
// joined once Hoofbite struck Ruggid, but did nothing but melee: none of its abilities (the
// damage over time, the tap, the harm touch), its template's Swarming Sting nor its buff's proc.
// Now it casts them at its foe, each landing as duel damage (halved, from the pet), and its hits
// are logged as the pet's.
func TestADuelistsPetCastsAndProcs(t *testing.T) {
	r := swarmRig(t, "pet swarming death")
	var lines bytes.Buffer
	was := log.Writer()
	log.SetOutput(&lines)
	defer log.SetOutput(was)
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	templateIntn = func(n int) int { return 0 } // the template's proc on every hit
	procIntn = func(n int) int { return 0 }     // the buff's proc on every hit, never resisted

	// the dummy fight, the pet's death, the summoning again
	r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
	pet, _ := r.summon(t, swarmingDeath, 0)
	r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(20*time.Second))
	for ms := 20100; ms <= 30000; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	r.w.mu.Lock()
	pet.HP, pet.Dead = 0, true
	r.w.petSystem(r.at(31 * time.Second))
	r.w.mu.Unlock()
	if r.w.petOfLocked(r.p) != nil {
		t.Fatal("the dead pet is still out")
	}
	pet, _ = r.summon(t, swarmingDeath, 40*time.Second)
	if pet.Dead || pet.HP <= 0 {
		t.Fatalf("the new pet: %+v", pet)
	}
	// the owner's buff on it: a proc of 1000
	pet.Effects = append(pet.Effects, &ActiveEffect{SpellID: 900977, Name: "Soul Mastery +", Proc: 1000, ProcMax: 1000,
		Until: r.at(time.Hour), Rules: EffectRules{ProcChance: 100}})

	// Hoofbite, beside Ruggid
	mgr := r.w.SessionManager
	sb := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 77), Port: 7077}, 77, 77, 1, 2, true)
	b := &Player{ServerID: 1077, Name: "Hoofbite", Class: 6, Level: 60, HP: 1_000_000, MaxHP: 1_000_000, Power: 100, MaxPower: 100,
		Speed: 8.833333, X: r.p.X + 3, Y: r.p.Y, Z: r.p.Z, World: r.p.World}
	r.w.Players[b.ServerID] = b
	r.w.SpatialIndex.Insert(b.ServerID, b.X, b.Z)
	sb.SetECSEntity(b)
	sb.SetCharacterInWorld(true)
	sb.SetPlayerObjectID(0x1424A1D1)
	see := func(s *session.Session, slot int, id EntityID, obj uint32) {
		vs := GetOrCreateVisibility(s)
		vs.ChannelEntity[slot], vs.ChannelObjectID[slot] = id, obj
	}
	see(r.sess, 20, b.ServerID, 0x00018801)
	see(sb, 0, r.p.ServerID, 0x00018802)
	see(sb, 1, pet.ServerID, 0x00018803)
	r.p.Level, r.p.HP, r.p.MaxHP = 60, 1_000_000, 1_000_000

	r.w.DuelChallenge(r.sess, 0, "Hoofbite", 0, r.at(50*time.Second))
	duel := r.w.duels.of[r.p.ServerID]
	if duel == nil {
		t.Fatalf("no duel: %s", lines.String())
	}
	r.w.DuelBoxAnswer(sb, DuelBoxID(duel.ID), 0, r.at(51*time.Second))
	r.w.mu.Lock()
	r.w.duelSystem(r.at(51*time.Second + DuelCountdown))
	if duel.Phase != duelOn {
		r.w.mu.Unlock()
		t.Fatalf("the duel did not begin: phase %d", duel.Phase)
	}
	start := 51*time.Second + DuelCountdown
	// Hoofbite strikes Ruggid first; the defensive pet joins
	r.w.duelHurtLocked(duel, b, b.ServerID, r.p, 10, r.at(start), numberMelee)
	r.w.mu.Unlock()
	lines.Reset()
	before := b.HP
	for x := start + 100*time.Millisecond; x <= start+20*time.Second; x += 100 * time.Millisecond {
		r.w.mu.Lock()
		r.w.petSystem(r.at(x))
		r.w.effectSystem(r.at(x))
		r.w.duelSystem(r.at(x))
		r.w.mu.Unlock()
	}
	got := lines.String()
	if !duel.pets[r.p.ServerID] {
		t.Fatalf("the pet did not join:\n%s", got)
	}
	for _, want := range []string{
		`joins the duel`,
		`Kimura's pet "` + pet.Name + `" hits Hoofbite for`,
		`Kimura's pet "` + pet.Name + `"'s Swarming Sting hits Hoofbite for 250`,
		`Kimura's pet "` + pet.Name + `"'s "Soul Mastery +" hits Hoofbite for 1000`,
		`begins "Swarming Plague"`,
		`puts "Swarming Plague" on Hoofbite`,
		`lands "Swarming Harm Touch" on Hoofbite`,
		`lands "Swarming Leech" on Hoofbite`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the log has no %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Duel 1: Kimura hits Hoofbite") {
		t.Errorf("a pet's hit logged as its owner's:\n%s", got)
	}
	// every figure halved: the 1000 proc took 500 off, each 300 tick of the damage over time 150
	if !strings.Contains(got, "for 500 (1000 before the halving") {
		t.Errorf("the proc's 1000 not halved:\n%s", got)
	}
	if n := strings.Count(got, "hits Hoofbite for 150 (300 before the halving"); n < 2 {
		t.Errorf("the damage over time ticked %d times at 150:\n%s", n, got)
	}
	if b.HP >= before || duel.Phase != duelOn {
		t.Fatalf("Hoofbite's HP %d of %d, phase %d", b.HP, before, duel.Phase)
	}
	var dot *ActiveEffect
	for _, e := range b.Effects {
		if e.Name == "Swarming Plague" {
			dot = e
		}
	}
	if dot == nil || dot.duelFoe != r.p.ServerID || dot.pet != pet.ServerID {
		t.Fatalf("the damage over time on Hoofbite: %+v", dot)
	}
	// the duel's end takes it off
	r.w.mu.Lock()
	r.w.endDuelLocked(duel, b.ServerID, duelDefeated, r.at(start+21*time.Second))
	r.w.mu.Unlock()
	for _, e := range b.Effects {
		if e.Name == "Swarming Plague" {
			t.Fatal("the pet's damage over time outlived the duel")
		}
	}
}

// A duel's melee hit, the foe's or its pet's, goes through the struck duelist's mitigation as a
// creature's does (the owner, 2026-11-13: Ruggid's Stoneskin stopped nothing of Hoofbite's), at
// its percent of what the duel's share leaves, its most spent as it goes, the buff breaking when
// it is spent. A spell's hit is no melee: it takes nothing off.
func TestADuelsMeleeIsMitigated(t *testing.T) {
	d := newDuelists(t)
	duel := d.start(t)
	skin := &ActiveEffect{SpellID: 900783, Name: "Stoneskin", Until: d.at(time.Hour),
		Rules: EffectRules{MitigatePct: 75, MitigateLeft: 1000, MitigateCapped: true}}
	d.w.mu.Lock()
	defer d.w.mu.Unlock()
	d.w.addEffectLocked(d.a, skin, d.at(0))
	hp := d.a.HP
	d.w.duelHurtLocked(duel, d.b, d.b.ServerID, d.a, 400, d.at(time.Second), numberMelee) // 200, 150 off
	if got := hp - d.a.HP; got != 50 {
		t.Fatalf("a melee hit of 400 (200 halved) took %d, want 50", got)
	}
	hp = d.a.HP
	d.w.duelHurtLocked(duel, d.b, d.b.ServerID, d.a, 400, d.at(time.Second), numberSpell)
	if got := hp - d.a.HP; got != 200 {
		t.Fatalf("a spell's hit of 400 took %d, want 200", got)
	}
	pet := &NPC{ServerID: 4005, Name: "Construct", Level: 50, HP: 4000, MaxHP: 4000, Owner: d.b.ServerID}
	d.w.NPCs[pet.ServerID] = pet
	hp = d.a.HP
	d.w.duelHurtLocked(duel, d.b, pet.ServerID, d.a, 1200, d.at(2*time.Second), numberMelee.byPet()) // 600, 450 off
	if got := hp - d.a.HP; got != 150 || skin.Rules.MitigateLeft != 400 {
		t.Fatalf("the pet's hit of 1200 took %d (%d left to stop), want 150 and 400", got, skin.Rules.MitigateLeft)
	}
	hp = d.a.HP
	d.w.duelHurtLocked(duel, d.b, d.b.ServerID, d.a, 2000, d.at(3*time.Second), numberMelee) // 1000, 750 off but 400 left
	if got := hp - d.a.HP; got != 600 {
		t.Fatalf("the last of the skin: %d taken, want 1000 - 400", got)
	}
	for _, e := range d.a.Effects {
		if e == skin {
			t.Fatal("the spent skin did not break")
		}
	}
}
