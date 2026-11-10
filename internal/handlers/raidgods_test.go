package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Mr. Moogie's answers of 2026-11-10 (the GM Jibrish's petitions 1441 and 1444) and the trigger's pets dying
// with their caller (the owner's "when the summoned pet's master dies, so does the pet").

// cooldownsSent is each 0x00CD sent to s: its display index and its lockout in ms.
func cooldownsSent(s *session.Session) map[int]int64 {
	out := map[int]int64{}
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 8 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeCooldownTimer {
			continue
		}
		v, n := binary.Uvarint(d[7:])
		if n <= 0 {
			continue
		}
		out[int(d[2])] = int64(v>>1) ^ -int64(v&1)
	}
	return out
}

// Mr. Moogie readies a player's abilities (petition 1444: "reset all of their ability cool downs. This
// cannot be used on players in PvP, in duels and can only be used on a player once every 10 minutes"):
// his box offers it, and picked, every lockout running ends, each badge told 0x00CD with 0, and he says so;
// asked again within ten minutes he refuses with the time left and ends nothing, and after them he readies
// them again. Its own ten minutes are a lockout of no spell, kept as lockouts are over a logout. Flagged
// for PvP, or dueling his metal twin, he will not (the answer is asked again at the pick).
func TestMrMoogieReadiesAbilities(t *testing.T) {
	f := newMetalFixture(t)
	anna, pa := f.join("Anna", 100, 100)
	now := time.Now()
	lock := func() {
		f.w.RestoreLockouts(anna, []database.LockoutRow{{SpellID: 901, Until: now.Add(time.Minute)}, {SpellID: 902, Until: now.Add(time.Hour)}},
			map[int]int{901: 3, 902: 5}, now)
	}
	lock()
	got := f.talk(anna, f.moogie)
	if len(got) != 1 || !strings.Contains(got[0], "|"+moogieResetOpt+"|") {
		t.Fatalf("his talk: %q, want the reset among his answers", got)
	}
	if err := f.ih.HandleDialogueOption(anna, incoming(protocol.OpcodeDialogueBoxOption, append(u32b(boxID(got)), 2))); err != nil {
		t.Fatal(err)
	}
	if told := cooldownsSent(anna); len(told) != 2 || told[3] != 0 || told[5] != 0 {
		t.Errorf("the badges told: %v, want 3 and 5 at 0", told)
	}
	rows, _ := f.w.SavedLockouts(pa.ServerID, now)
	if len(rows) != 1 || rows[0].SpellID != entity.MoogieResetID || rows[0].Until.Sub(now) < 9*time.Minute {
		t.Errorf("the lockouts kept after it: %+v, want his own ten minutes alone", rows)
	}
	// again within the ten minutes: refused, nothing ended
	lock()
	got = f.talk(anna, f.moogie)
	got = f.pick(anna, boxID(got), 2)
	if len(got) != 1 || !strings.HasPrefix(got[0], "colour:"+fmt.Sprintf("Mr. Moogie says, 'Not so soon, Anna. I can ready your abilities again in")) {
		t.Errorf("asked again: %q", got)
	}
	if rows, _ := f.w.SavedLockouts(pa.ServerID, now); len(rows) != 3 {
		t.Errorf("asked again, the lockouts: %+v, want both kept and his own", rows)
	}
	if _, _, wait, ok := f.w.MoogieReset(anna, now.Add(5*time.Minute)); ok || wait <= 0 {
		t.Errorf("at five minutes: ok %v, wait %v", ok, wait)
	}
	if _, n, _, ok := f.w.MoogieReset(anna, now.Add(11*time.Minute)); !ok || n != 1 {
		t.Errorf("past ten minutes: ok %v, %d ended (want the hour's one)", ok, n)
	}
	// flagged for PvP between the box and the pick: refused, nothing ended
	lock()
	got = f.talk(anna, f.moogie)
	f.w.SetPvP(true)
	f.w.PvPCommand(anna, time.Now())
	metalMsgs(anna)
	got = f.pick(anna, boxID(got), 2)
	if len(got) != 1 || got[0] != "colour:"+entity.MoogieLinePvP {
		t.Errorf("flagged: %q", got)
	}
	// dueling his metal twin: refused, no box
	cora, _ := f.join("Cora", 102, 100)
	if _, ok := f.w.MetalMoogieChallenge(cora, f.moogie.ServerID, time.Now()); !ok {
		t.Fatal("no duel for Cora")
	}
	if got := f.talk(cora, f.moogie); len(got) != 1 || got[0] != "colour:"+entity.MetalLineDueling {
		t.Errorf("dueling: %q", got)
	}
}

// A Moogie Tribute spent on Metal Moogie V2 (petition 1441): without a tribute his box offers none (the
// control: no V2 summoned without the token); with one, the tribute's answer asks a second question, his
// favor or V2's duel; V2's picked, Mr. Moogie says so, V2 stands as his original is (his size, his
// template's figures: the data's), Metal Moogie's duel in every rule with his own name in its lines, and the
// tribute is spent. Another's ask while he stands is refused and keeps its tribute.
func TestATributeSummonsMetalMoogieV2(t *testing.T) {
	f := newMetalFixture(t)
	v2 := &entity.NPC{ServerID: 7779, Name: entity.MetalMoogieV2Name, NPCID: 9702, X: 101, Y: 54, Z: 130, Level: 60,
		HP: 54965, MaxHP: 54965, Size: 9, SpawnOnly: true}
	v2.SpawnX, v2.SpawnY, v2.SpawnZ = v2.X, v2.Y, v2.Z
	f.w.NPCs[v2.ServerID] = v2
	f.w.SpatialIndex.Insert(v2.ServerID, v2.X, v2.Z)
	anna, pa := f.join("Anna", 100, 100)
	cora, pc := f.join("Cora", 102, 100)
	if got := f.talk(anna, f.moogie); len(got) != 1 || strings.Contains(got[0], moogieTributeOpt) {
		t.Fatalf("no tribute held: %q", got)
	}
	f.give(anna, pa)
	got := f.talk(anna, f.moogie)
	want := "|" + strings.Join([]string{moogieClearOpt, moogieHealOpt, moogieResetOpt, moogieProveOpt, moogieTributeOpt, moogieNoOpt}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) {
		t.Fatalf("a tribute held: %q, want %q", got, want)
	}
	got = f.pick(anna, boxID(got), 4)
	want = moogieTributeQuestion + "|" + strings.Join([]string{moogieFavorOpt, moogieV2Opt, moogieNoOpt}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) {
		t.Fatalf("the tribute's question: %q, want %q", got, want)
	}
	got = f.pick(anna, boxID(got), 1)
	if !hasMsg(got, "chat:"+fmt.Sprintf(entity.MetalLineRisesV2, "Anna")) || !hasMsg(got, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeRemoveInvItem))) {
		t.Errorf("V2's pick: %q", got)
	}
	var c *entity.NPC
	for _, n := range f.w.NPCs {
		if n.Name == entity.MetalMoogieV2Name && !n.SpawnOnly && !n.Dead {
			c = n
		}
	}
	if c == nil || c.CurrentEnemy != pa.ServerID || !c.DuelSpawn() || c.Size != 9 || c.MaxHP != v2.MaxHP {
		t.Fatalf("no V2 standing for Anna as his original is: %+v", c)
	}
	if n := f.held(pa); n != 0 {
		t.Errorf("%d tributes left after V2 rose, want it spent", n)
	}
	pc.Z = c.Z - 3
	if got := f.talk(cora, c); len(got) != 1 || got[0] != "colour:Metal Moogie V2 is dueling Anna and pays you no mind." {
		t.Errorf("Cora's talk to Anna's V2: %q", got)
	}
	pc.Z = 100
	f.give(cora, pc)
	got = f.talk(cora, f.moogie)
	got = f.pick(cora, boxID(got), 4)
	got = f.pick(cora, boxID(got), 1)
	if len(got) != 1 || got[0] != "chat:"+fmt.Sprintf(entity.MetalLineBusy, "Anna") || f.held(pc) != 1 {
		t.Errorf("Cora's ask while V2 stands: %q, %d tributes kept", got, f.held(pc))
	}
	// the control: no V2 on the server, the tribute's answer is his favor at once, as before
	v2.Name = "Someone Else"
	anna2, pa2 := f.join("Dora", 104, 100)
	f.give(anna2, pa2)
	got = f.talk(anna2, f.moogie)
	got = f.pick(anna2, boxID(got), 4)
	if !hasMsg(got, "chat:"+fmt.Sprintf(moogieFavored, "Dora")) {
		t.Errorf("with no V2, the tribute's answer: %q, want his favor", got)
	}
}

// A resummoned pet dies with its caller (the owner, 2026-11-10: Tunare's Grace and Wrath of the Mother):
// the chief killed, at the next second its pawn lies dead too, a corpse of nothing; the control, a pup
// resummoned by a shaman who lives, stands.
func TestTriggerResummonedPetsDieWithTheirCaller(t *testing.T) {
	w := newTrigWorld(t)
	gp := w.gp
	for _, n := range []*entity.NPC{
		{ServerID: 701, Name: "Orc Chief", HP: 500, MaxHP: 500, Level: 20, NPCType: 0x80, X: 100, Y: 50, Z: 120},
		{ServerID: 703, Name: "Orc Shaman", HP: 500, MaxHP: 500, Level: 20, NPCType: 0x80, X: 100, Y: 50, Z: 130},
		{ServerID: 704, Name: "Orc Pup", HP: 100, MaxHP: 100, Level: 5, NPCType: 0x80, X: 100, Y: 50, Z: 140},
	} {
		w.gw.NPCs[n.ServerID] = n
	}
	target := &entity.NPC{ServerID: 702, Name: "Mr. Moogie", HP: 100000, MaxHP: 100000, Level: 50, X: gp.X + 10, Y: gp.Y, Z: gp.Z, World: gp.World}
	w.gw.NPCs[target.ServerID] = target
	w.set(31, "Chief, shaman and theirs", "near", 60, `[{"do":"spawn","npc":"Orc Chief","attackable":true,"minutes":60,"spread":1},
		{"do":"spawn","npc":"Orc Pawn","attackable":true,"minutes":60,"spread":1},
		{"do":"spawn","npc":"Orc Shaman","attackable":true,"minutes":60,"spread":1},
		{"do":"spawn","npc":"Orc Pup","attackable":true,"minutes":60,"spread":1},
		{"do":"resummon","npc":"Orc Pawn","by":"Orc Chief","seconds":10,"minutes":60},
		{"do":"resummon","npc":"Orc Pup","by":"Orc Shaman","seconds":10,"minutes":60},
		{"do":"awaitdead","npc":"Orc Chief","seconds":3600}]`)
	if _, err := w.fire(31); err != nil {
		t.Fatal(err)
	}
	of := func(name string) *entity.NPC {
		for _, n := range w.gw.NPCs {
			if n.Temporary && n.Name == name {
				return n
			}
		}
		return nil
	}
	chief, pawn, pup := of("Orc Chief"), of("Orc Pawn"), of("Orc Pup")
	if chief == nil || pawn == nil || pup == nil {
		t.Fatalf("the spawns: chief %v, pawn %v, pup %v", chief != nil, pawn != nil, pup != nil)
	}
	w.at(time.Second)
	if pawn.Dead || pup.Dead {
		t.Fatal("a pet died with its caller alive")
	}
	w.gw.KillNPCNow(chief.ServerID)
	w.at(2 * time.Second) // the chief seen dead
	w.at(3 * time.Second)
	if !pawn.Dead || pawn.HP != 0 || !pawn.CorpseOfNothing() {
		t.Errorf("the chief's pawn: dead %v, HP %d, nothing on it %v; want dead with it", pawn.Dead, pawn.HP, pawn.CorpseOfNothing())
	}
	if pup.Dead {
		t.Error("the control: the shaman's pup died with the chief")
	}
	w.at(30 * time.Second)
	if of("Orc Pawn") != nil && !of("Orc Pawn").Dead {
		t.Error("a pawn called back with its chief dead")
	}
}
