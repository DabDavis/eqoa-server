package handlers

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// testBufferSpells is five made-up buffs in Buffer's spells' shapes (entity's buffer_test.go has them whole): a
// heal with its maximum, an AC and HP buff, a max HP and power buff, an all-resist buff, and the GM Armor's record.
func testBufferSpells() []entity.CastSpell {
	mk := func(id, level int, name, js string) entity.CastSpell {
		sp := &database.SpellPatternRecord{SpellID: id, AbilityLvl: level, Range: 0x41F0, CastTime: 3, Power: 300, Scope: 1,
			Recast: 4, SpellName: name, Icon: int64(id)}
		return entity.SpellOfRow(sp, 0, "", js, uint32(0x0B000000+id))
	}
	stat := func(stack, level int, stats string, amount string) string {
		var parts []string
		for _, s := range strings.Split(stats, ",") {
			parts = append(parts, `{"kind":"stat","stat":"`+s+`","amount":`+amount+`}`)
		}
		return `{"v":1,"scope":"target","harmful":false,"duration_s":1800,"stack":` + strconv.Itoa(stack) + `,"level":` + strconv.Itoa(level) +
			`,"effects":[` + strings.Join(parts, ",") + `]}`
	}
	return []entity.CastSpell{
		mk(9401, 51, "Test Avatar", `{"v":1,"scope":"target","harmful":false,"duration_s":1830,"stack":7,"level":51,"effects":[
			{"kind":"hp","amount":300},{"kind":"stat","stat":"str","amount":40},{"kind":"stat","stat":"maxhp","amount":300}]}`),
		mk(9402, 60, "Test Order", stat(1, 60, "ac,maxhp", "65")),
		mk(9403, 60, "Test Heart", stat(3, 60, "maxhp,maxpower", "120")),
		mk(9404, 60, "Test Ward", stat(1, 60, "fr,cr,lr,pr,dr,ar", "85")),
		mk(BufferGMArmorID, 60, "GM Armor", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":18000,`+
			`"stack":905120,"level":60,"min_level":1,"effects":[{"id":"GMAC","type":"Statistic Change","kind":"stat","stat":"ac","amount":500}]}`),
	}
}

// bufferRig is the portal rig with Buffer beside Anna (talked to on her first slot, seen by Cora on hers) and his
// talk reading testBufferSpells on the rig's clock.
func bufferRig(t *testing.T) (*portalRig, *entity.NPC) {
	t.Helper()
	r := newPortalRig(t)
	buf := &entity.NPC{ServerID: 7790, Name: entity.BufferName, X: 5001, Y: 50, Z: 5000, Level: 60, HP: 5000, MaxHP: 5000, NPCType: 0x80}
	r.gw.NPCs[buf.ServerID] = buf
	r.ih.buffer = &Buffer{gw: r.gw, spells: func() ([]entity.CastSpell, []string) { return testBufferSpells(), nil },
		now: func() time.Time { return r.now }}
	vs := entity.GetOrCreateVisibility(r.cora)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = buf.ServerID, 100002
	return r, buf
}

func countMsgs(msgs []string, want string) int {
	n := 0
	for _, m := range msgs {
		if m == want {
			n++
		}
	}
	return n
}

// The whole of it through the handlers (petition 1390): a talk to Buffer is his line and each spell's 0x00B1 (to
// Cora too, who sees him), nothing landed before the second; at it, all five on Anna until five hours on, her slots
// showing them and his line naming them; Cora, beside her, has none. The control: another NPC's talk at the same
// spot casts and gives nothing.
func TestBufferTalkBuffsTheSpeakerASecondLater(t *testing.T) {
	r, buf := bufferRig(t)
	r.talk(r.anna, buf)
	got := r.msgs(r.anna)
	if countMsgs(got, "op:00B1") != 5 || !hasMsg(got, "chat:"+entity.BufferLineBegins) || len(got) != 6 {
		t.Fatalf("his talk: %q", got)
	}
	if seen := r.msgs(r.cora); countMsgs(seen, "op:00B1") != 5 {
		t.Errorf("Cora, who sees him, was sent %q", seen)
	}
	start := r.now
	r.gw.Tick(start.Add(999 * time.Millisecond))
	if fx := r.gw.PlayerEffects(r.anna); len(fx) != 0 {
		t.Fatalf("before the second: %+v", fx)
	}
	r.gw.Tick(start.Add(time.Second))
	var names []string
	for _, s := range r.gw.PlayerEffects(r.anna) {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "Test Order,Test Heart,Test Ward,GM Armor,Test Avatar" { // the heal goes on last
		t.Fatalf("her effects at the second: %v", names)
	}
	for _, e := range r.pa.Effects {
		if want := start.Add(time.Second + entity.BufferLasts); !e.Until.Equal(want) {
			t.Errorf("%s until %v, want %v", e.Name, e.Until, want)
		}
	}
	if r.pa.Mods.AC != 565 {
		t.Errorf("her AC +%d, want 565 (Test Order's and the GM Armor's)", r.pa.Mods.AC)
	}
	if !hasMsg(r.msgs(r.anna), "chat:Buffer says, 'There you are, Anna: Test Avatar, Test Order, Test Heart, Test Ward and GM Armor, for five hours of play.'") {
		t.Error("his line at the landing")
	}
	if len(r.pc.Effects) != 0 {
		t.Errorf("Cora beside her: %d effects", len(r.pc.Effects))
	}

	// the control: another NPC where he stands
	r, _ = bufferRig(t)
	other := &entity.NPC{ServerID: 7791, Name: "Bufferd", X: 5001, Y: 50, Z: 5000, Level: 60, HP: 5000, MaxHP: 5000, NPCType: 0x80}
	r.gw.NPCs[other.ServerID] = other
	r.talk(r.anna, other)
	if got := r.msgs(r.anna); countMsgs(got, "op:00B1") != 0 {
		t.Errorf("another NPC's talk: %q", got)
	}
	r.gw.Tick(r.now.Add(2 * time.Second))
	if len(r.pa.Effects) != 0 {
		t.Errorf("another NPC's talk gave %d effects", len(r.pa.Effects))
	}
}

// A second talk renews (five effects at their whole five hours again); with no spell to give he says so; flagged
// for PvP he refuses in red and casts nothing.
func TestBufferTalkRenewsAndRefuses(t *testing.T) {
	r, buf := bufferRig(t)
	r.talk(r.anna, buf)
	r.gw.Tick(r.now.Add(time.Second))
	r.now = r.now.Add(90 * time.Minute)
	r.gw.Tick(r.now)
	r.drain()
	r.talk(r.anna, buf)
	r.gw.Tick(r.now.Add(time.Second))
	if len(r.pa.Effects) != 5 || len(r.gw.PlayerEffects(r.anna)) != 5 {
		t.Fatalf("renewed: %d effects, %d slots", len(r.pa.Effects), len(r.gw.PlayerEffects(r.anna)))
	}
	for _, e := range r.pa.Effects {
		if want := r.now.Add(time.Second + entity.BufferLasts); !e.Until.Equal(want) {
			t.Errorf("%s until %v, want %v", e.Name, e.Until, want)
		}
	}

	r.ih.buffer.spells = func() ([]entity.CastSpell, []string) { return nil, []string{"Avatar"} }
	r.drain()
	r.talk(r.anna, buf)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "chat:"+entity.BufferLineNone {
		t.Errorf("no spells: %q", got)
	}

	r, buf = bufferRig(t)
	r.gw.SetPvP(true)
	r.gw.PvPCommand(r.anna, r.now)
	r.drain()
	r.talk(r.anna, buf)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "colour:"+entity.BufferLinePvP {
		t.Errorf("flagged: %q", got)
	}
	r.gw.Tick(r.now.Add(2 * time.Second))
	if len(r.pa.Effects) != 0 {
		t.Errorf("flagged, given %d effects", len(r.pa.Effects))
	}
}

// His spells and his lines: five, the GM Armor among them by its own id; each name fits the effects channel, and his
// longest line (every name, a long character name) fits a chat line.
func TestBufferSpellsAndWords(t *testing.T) {
	if len(BufferSpells) != 5 || BufferSpells[4] != (BufferSpell{BufferGMArmorID, "GM Armor"}) {
		t.Fatalf("his spells: %+v", BufferSpells)
	}
	var names []string
	for _, s := range BufferSpells {
		if len(s.Name) > entity.MaxEffectName {
			t.Errorf("%q is past the effects channel's %d", s.Name, entity.MaxEffectName)
		}
		names = append(names, s.Name)
	}
	line := strings.Replace(strings.Replace(entity.BufferLineDone, "%s", strings.Repeat("W", 24), 1), "%s", joinAnd(names), 1)
	if n := len([]rune(line)); n > 1023 || strings.ContainsAny(line, "|%") {
		t.Errorf("his line is %d characters (or holds a code): %q", n, line)
	}
}
