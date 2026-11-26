package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A heal of its kin (kinheal.go): a creature's instant heal of its share (15%
// of its own hit points, no spell) every 4 s lands on the worst off of itself
// and its kin near it, below the heal's 85%: here the pup beside it, of its
// social faction (npcsocial), not the mage of no faction far off, nor the
// priest itself while it is whole enough; the cast's 0x00B1 names the pup and
// the player fighting the pup sees the figure over the pup, from its own id
// (CLIENT draws any other attacker's figure over the player: petition 3205).
// With none of its kin hurt it waits; hurt itself and worse off than the pup
// it heals itself, the figure over it. With no social faction its kin is its
// own name alone.
func TestAKinHealLandsOnTheWorstOffOfItsKin(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES ('kin healer', -1,
		'[{"kind": "heal", "every": 4, "first": 1, "heal_pct": 15, "below": 85, "target": "kin"}]')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll necro", "kin healer") // at x 100; the pup at 103, the mage at 1000
	healer, pup, mage := r.npc("Gnoll Necro"), r.npc("Gnoll Pup"), r.npc("Gnoll Mage")
	r.w.SetSocialRules(map[string]SocialRule{
		"gnoll necro": {Social: true, Faction: "Gnolls"},
		"gnoll pup":   {Social: true, Faction: "Gnolls"},
	})
	p, sess := r.player("Hero", 101.5, 100)
	r.fight(healer, p.ServerID)
	r.fight(pup, p.ServerID)
	for _, n := range []*NPC{healer, pup, mage} {
		n.MaxHP, n.HP = 10000, 10000
	}
	me := sess.PlayerObjectID()
	var selfFigures []int // on the healer itself, from the player's id
	heals := func(from, to time.Duration) (casts []b1, figures []int) {
		for d := from; d <= to; d += 100 * time.Millisecond {
			r.tick(d)
			for _, m := range drain(sess) {
				if opOfMsg(m) == protocol.OpcodeDamageIndication {
					src := binary.LittleEndian.Uint32(m[2:])
					v, n := binary.Uvarint(m[6:])
					amount := int(v>>1) ^ -int(v&1) // zigzag
					if src == r.objectID(healer.ServerID) && amount > 0 {
						t.Errorf("a heal's figure from the healer's id, %d: CLIENT draws it over the player", amount)
					}
					on := binary.LittleEndian.Uint32(m[6+n:])
					if src == me && amount > 0 && on == r.objectID(pup.ServerID) {
						figures = append(figures, amount)
					}
					if src == me && amount > 0 && on == r.objectID(healer.ServerID) {
						selfFigures = append(selfFigures, amount)
					}
				}
				for _, c := range castsIn([][]byte{m}) {
					if c.caster == r.objectID(healer.ServerID) && c.fx == fxHeal {
						casts = append(casts, c)
					}
				}
			}
		}
		return casts, figures
	}
	if c, _ := heals(0, 3*time.Second); len(c) != 0 {
		t.Fatalf("%d heals with no one hurt", len(c))
	}
	pup.HP, mage.HP, healer.HP = 5000, 1000, 9000 // the healer at 90%: whole enough
	c, f := heals(3100*time.Millisecond, 6*time.Second)
	if len(c) != 1 || c[0].target != r.objectID(pup.ServerID) || pup.HP != 6500 || mage.HP != 1000 || healer.HP != 9000 {
		t.Fatalf("heals %+v; pup %d (want 6500), mage %d, healer %d", c, pup.HP, mage.HP, healer.HP)
	}
	if len(f) != 1 || f[0] != 1500 {
		t.Errorf("figures told the player: %v, want one of 1500 on the pup", f)
	}
	healer.HP = 2000 // worse off than the pup's 65%
	c, _ = heals(6100*time.Millisecond, 10*time.Second)
	if len(c) != 1 || c[0].target != c[0].caster || healer.HP != 3500 || pup.HP != 6500 {
		t.Fatalf("heals %+v; healer %d (want 3500), pup %d", c, healer.HP, pup.HP)
	}
	if len(selfFigures) != 1 || selfFigures[0] != 1500 {
		t.Errorf("figures of its heal on itself told the player: %v, want one of 1500 over the healer", selfFigures)
	}

	// no social faction: its kin is its own name; the pup is not
	r.w.SetSocialRules(nil)
	r.w.mu.Lock()
	target := r.w.kinHealTargetLocked(healer, &healer.tmpl.abilities[0])
	r.w.mu.Unlock()
	if target != healer {
		t.Errorf("with no faction the heal goes to %v, want the healer itself", target.Name)
	}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{{Kind: database.AbilityNuke, Every: 4, Target: database.AbilityOnKin}}); err == nil {
		t.Error("a nuke on its kin passed the check")
	}
}

// Out of its own fight (kinheal.go's kinHealSystem) a healer standing by heals
// one of its kin that is fighting, not one that is not, and not itself; it does
// not join the fight.
func TestAKinHealOutOfItsOwnFight(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES ('kin healer', -1,
		'[{"kind": "heal", "every": 15, "first": 1, "heal_pct": 15, "below": 85, "target": "kin"}]')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll necro", "kin healer")
	healer, pup := r.npc("Gnoll Necro"), r.npc("Gnoll Pup")
	r.w.SetSocialRules(map[string]SocialRule{
		"gnoll necro": {Social: true, Faction: "Gnolls"},
		"gnoll pup":   {Social: true, Faction: "Gnolls"},
	})
	p, _ := r.player("Hero", 101.5, 100)
	for _, n := range []*NPC{healer, pup} {
		n.MaxHP, n.HP = 10000, 10000
	}
	idle := func(d time.Duration) {
		r.w.mu.Lock()
		r.w.TickCount = 0
		r.w.kinHealSystem(r.at(d))
		r.w.mu.Unlock()
		for e := d; e <= d+3*time.Second; e += 100 * time.Millisecond {
			r.tick(e)
		}
	}
	healer.HP, pup.HP = 2000, 5000 // the pup out of a fight; the healer hurt
	idle(0)
	if healer.HP != 2000 || pup.HP != 5000 {
		t.Fatalf("with no one fighting: healer %d, pup %d (want 2000, 5000)", healer.HP, pup.HP)
	}
	r.fight(pup, p.ServerID)
	r.w.mu.Lock()
	r.w.wakeNPCLocked(pup)
	r.w.mu.Unlock()
	idle(4 * time.Second)
	if pup.HP != 6500 || healer.HP != 2000 || healer.CurrentEnemy != 0 {
		t.Fatalf("the pup fighting: pup %d (want 6500), healer %d, its enemy %d", pup.HP, healer.HP, healer.CurrentEnemy)
	}
	idle(8 * time.Second) // its 15 s recast: nothing yet
	if pup.HP != 6500 {
		t.Fatalf("healed again inside its recast: pup %d", pup.HP)
	}
	idle(20 * time.Second)
	if pup.HP != 8000 {
		t.Fatalf("after its recast: pup %d (want 8000)", pup.HP)
	}
}
