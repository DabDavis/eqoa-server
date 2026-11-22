package entity

import (
	"testing"
	"time"
)

// crusadersAuraJSON is board 1659's §3 effects for spell 701122.
const crusadersAuraJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":90,"stack":2,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Offensive Proc","effects":[{"id":"0525","type":"Offensive Proc","kind":"proc","procs":[{"id":"0D57","type":"Dmg/Heal","kind":"hp","amount":200,"scale":"cha","scale_pct":60,"on":"offense","scope":"group","target_type":"Friendly","radius":30},{"id":"0D58","type":"Dmg/Heal","kind":"hp","amount":-200,"scale":"cha","scale_pct":50,"on":"offense","scope":"target","target_type":"Enemy - Undead"}]}]}`

// Crusader's Aura (board 1659), procIntn forced to 0: a melee hit on a creature
// not undead heals the bearer and a group member 20 off by 200 + 60% of the
// bearer's CHA, not one 40 off or one in another world, and hurts nothing; on
// an undead creature the proc hits once and the group is healed once; a bearer
// in no group is healed alone.
func TestCrusadersAuraHealsTheGroupOnAnyHit(t *testing.T) {
	old := procIntn
	procIntn = func(int) int { return 0 }
	t.Cleanup(func() { procIntn = old })
	aura := CastSpell{SpellID: 701122, DisplayIndex: 3, Scope: 0, Name: "Crusader's Aura", Range: 30,
		Def: SpellDefFrom("Crusader's Aura", "", "", crusadersAuraJSON)}
	setup := func(grouped bool) (*caster, map[string]*Player) {
		c := newCaster(t)
		c.p.Level, c.p.Cha, c.p.MaxHP, c.p.HP = 60, 200, 5000, 1000
		if !c.w.StartCast(c.sess, aura, c.me, c.at(0)) {
			t.Fatal("the aura was not cast")
		}
		c.tick(time.Second)
		if len(c.p.Effects) != 1 {
			t.Fatalf("the aura is not on: %+v", c.p.Effects)
		}
		m := map[string]*Player{}
		for i, s := range []struct {
			name  string
			dx    float32
			world uint8
		}{{"near", 20, 0}, {"far", 40, 0}, {"away", 5, 1}} {
			p := &Player{ServerID: EntityID(1100 + i), Name: s.name, Level: 60, MaxHP: 5000, HP: 1000, X: c.p.X + s.dx, Y: c.p.Y, Z: c.p.Z, World: s.world}
			c.w.Players[p.ServerID] = p
			m[s.name] = p
		}
		if grouped {
			if c.w.groups == nil {
				c.w.groups = map[uint32]*Group{}
			}
			c.w.groups[9] = &Group{ID: 9, Members: []EntityID{c.p.ServerID, 1100, 1101, 1102}}
			c.p.GroupID = 9
			for _, p := range m {
				p.GroupID = 9
			}
		}
		return c, m
	}
	for _, undead := range []bool{false, true} {
		c, m := setup(true)
		c.npc.HP, c.npc.MaxHP, c.npc.Level = 50000, 50000, 60
		c.npc.Name = "a desert crab"
		if undead {
			c.npc.Name = "a restless skeleton"
		}
		c.p.Cha = 200
		c.w.mu.Lock()
		c.w.playerProcsLocked(c.sess, c.p, c.npc, c.at(2*time.Second))
		c.w.mu.Unlock()
		got := c.p.StatNow(StatCha)
		h := 200 + got*60/100
		if c.p.HP != 1000+h || m["near"].HP != 1000+h || m["far"].HP != 1000 || m["away"].HP != 1000 {
			t.Errorf("undead %v: healed the bearer %d, near %d, far %d, away %d; want %d, %d, 0, 0",
				undead, c.p.HP-1000, m["near"].HP-1000, m["far"].HP-1000, m["away"].HP-1000, h, h)
		}
		if took := 50000 - c.npc.HP; undead != (took > 0) {
			t.Errorf("undead %v: the proc hit for %d", undead, took)
		}
	}
	c, m := setup(false)
	c.npc.Name = "a desert crab"
	c.w.mu.Lock()
	c.w.playerProcsLocked(c.sess, c.p, c.npc, c.at(2*time.Second))
	c.w.mu.Unlock()
	if c.p.HP <= 1000 || m["near"].HP != 1000 {
		t.Errorf("no group: the bearer %d, near %d", c.p.HP-1000, m["near"].HP-1000)
	}
}
