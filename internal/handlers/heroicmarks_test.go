package handlers

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

func seedHeroicMarkPrerequisites(t *testing.T, db *database.SQLiteDB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO cmcatalog (id,name,mastery,rank,cost) VALUES (211, 'God Touched', 'God Touched', 1, 75)`,
		`INSERT INTO spellpattern (spellid,abilitylvl,unk2,unk3,range,casttime,power,iconColor,icon,scope,recast,eqprequire,spellname,spelldesc)
		  VALUES (675,34,0,0,0,0,0,0,0,1,0,0,'Bait',''), (1608,34,0,0,0,0,0,0,0,1,0,0,'Bait','')`,
		`INSERT INTO spelleffect (spellid,line,fxdict,source,effects) VALUES
		  (675,'Target, Increase Hate by 594 ; Increase 16% based on STR',0,'test',
		  '{"v":1,"target_type":"Enemy","harmful":true,"effects":[{"kind":"hate","amount":594,"scale":"str","scale_pct":16}]}'),
		  (1608,'Target, Increase Hate by 594 ; Increase 11% based on STR',0,'test','')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHeroicMarksRaidLoot(t *testing.T) {
	r := newRaidLootRig(t, 4)
	seedHeroicMarkPrerequisites(t, r.f.db)
	if err := r.f.db.InstallHeroicMarks(); err != nil {
		t.Fatal(err)
	}
	set := database.HeroicMarksFor(entity.EncounterMythicNagafen)
	seats := make([]entity.RaidSeat, len(r.p))
	for i, class := range []int{0, 1, 7, 10} {
		r.p[i].Class = class
		seats[i] = entity.RaidSeat{ServerID: r.p[i].CharServerID, Class: class, Name: r.p[i].Name, Level: 60}
	}
	r.dice = []int{2, 0, 1, 2, 3}
	ls := r.f.h.loots()
	npc := r.f.npc
	r.f.h.startRaidLootLocked(ls, npc, entity.KillCredit{NPCID: npc.ServerID, Died: npc.DeathTime, HeroicBoss: set.Boss,
		Raid: &entity.RaidKill{Group: 1, Rule: entity.LootNeedGreed, Leader: seats[0], Seats: seats, Members: seats}})
	c := ls.corpses[npc.ServerID]
	if len(ls.rolls) != 4 || len(c.items) != 0 || len(c.shares) != 0 {
		t.Fatalf("marks should be four individual rolls, got %d rolls %d items %d shares", len(ls.rolls), len(c.items), len(c.shares))
	}
	for _, roll := range ls.rolls {
		pattern := roll.item.rec.Pattern
		if pattern.MaxStack != 99 || pattern.Trade != 1 || roll.item.rec.StackLeft != 1 {
			t.Fatalf("bad mark record: %+v", roll.item.rec)
		}
		for _, seat := range roll.seats {
			if want := set.RoleFor(seat.Class).MarkID == pattern.PatternID; seat.mayNeed != want {
				t.Errorf("class %d Need on %s = %t, want %t", seat.Class, pattern.ItemName, seat.mayNeed, want)
			}
		}
	}
	for count := 2; count <= 4; count++ {
		r.dice = append([]int{count - 2}, make([]int, count)...)
		drops := r.f.h.rollHeroicMarks(set.Boss)
		if len(drops) != count {
			t.Fatalf("got %d drops, want %d", len(drops), count)
		}
		for _, drop := range drops {
			if drop.StackLeft != 1 || drop.Pattern.PatternID != set.Roles[0].MarkID {
				t.Fatalf("an identical drop was combined: %+v", drop)
			}
		}
	}
}

func TestHeroicMarksFromTheInstanceCorpse(t *testing.T) {
	for _, mode := range []struct {
		name string
		need bool
	}{{"free for all", false}, {"raid need before greed", true}} {
		t.Run(mode.name, func(t *testing.T) {
			r, board, _ := mythicPortalRig(t)
			db, err := database.NewSQLite(filepath.Join(t.TempDir(), "corpse.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			seedHeroicMarkPrerequisites(t, db)
			if err := r.gw.LoadHeroicMarks(db); err != nil {
				t.Fatal(err)
			}
			r.pa.Level, r.pc.Level, r.pa.Class, r.pc.Class = 60, 60, 0, 1
			for _, p := range []*entity.Player{r.pa, r.pc} {
				if _, err := db.Exec(`INSERT INTO characters
					(serverid,charName,accountid,modelid,tclass,race,humType,level,haircolor,hairlength,hairstyle,faceoption,sex)
					VALUES (?, ?, 1, 0, ?, 0, 0, 60, 0, 0, 0, 0, 0)`, p.CharServerID, p.Name, p.Class); err != nil {
					t.Fatal(err)
				}
			}
			r.anna.SetInventoryList(newInvList(nil))
			r.cora.SetInventoryList(newInvList(nil))
			inv := database.NewInventoryStore(db)
			h := NewNPCWindowHandler(r.sm, inv)
			h.SetGameWorld(r.gw)
			// The loot window itself drains the pending death. FFA below also
			// runs the worker's drain explicitly before the window is opened.
			h.raids.once.Do(func() {})
			votes := 0
			h.raid.intn = func(n int) int {
				switch n {
				case 3:
					return 2 // four drops
				case 4:
					return 0 // four Steel marks, which must be separate rolls
				}
				votes++
				if votes%2 == 1 {
					return n - 1 // Greed
				}
				return 1 // Need wins despite the smaller roll
			}
			r.ih.heroicMarkInventory = inv
			r.group(t, r.anna, r.pa, r.cora, r.pc)
			if mode.need {
				r.gw.RaidInvite(r.anna, r.pb.Name, 0)
				r.gw.RaidLootRuleTo(r.anna, entity.LootNeedGreed)
				r.gw.SetRaidWindows(r.anna, true)
			}
			r.talk(r.anna, board)
			heroicRaids.mu.Lock()
			box := heroicRaids.asked[r.pa.CharServerID].box
			heroicRaids.mu.Unlock()
			r.ih.answerHeroicRaids(r.anna, r.pa, box, 0)
			r.travel()
			inst := r.pa.InstanceKey()
			state, ok := r.gw.MythicStateOf(inst)
			if !ok || inst == 0 || r.pc.InstanceKey() != inst {
				t.Fatalf("the party did not enter the same Nagafen instance (%d)", inst)
			}
			boss := r.gw.NPCByID(state.Boss)
			if boss == nil || boss.World != 18 || boss.Instance != inst {
				t.Fatalf("no Nagafen in world 18 instance %d: %+v", inst, boss)
			}
			r.gw.UpdatePosition(r.pa.ServerID, boss.X, boss.Y, boss.Z+2, 0)
			r.gw.UpdatePosition(r.pc.ServerID, boss.X, boss.Y, boss.Z+3, 0)
			const objectID = 0x18001ABC
			vis := entity.GetOrCreateVisibility(r.anna)
			vis.ChannelEntity[0], vis.ChannelObjectID[0] = boss.ServerID, objectID
			r.drain()
			if !r.gw.KillNPCNowBy(r.anna, boss.ServerID) {
				t.Fatal("could not exercise the actual instance's credited death")
			}
			if !mode.need {
				h.takeRaidKills()
			}
			if err := h.HandleLootCorpse(r.anna, incoming(protocol.OpcodeLootCorpse, u32b(objectID))); err != nil {
				t.Fatal(err)
			}
			var rolls []int
			corpseItems := int64(-1)
			for _, m := range r.anna.Queue().GetPending() {
				data := m.Data()
				switch opOf(data) {
				case protocol.OpcodeLoot:
					corpseItems = lootCount(t, data)
				case protocol.OpcodeRaidRoll:
					rolls = append(rolls, int(binary.LittleEndian.Uint32(data[2:])))
				}
			}
			if mode.need {
				if len(rolls) != 4 || corpseItems > 0 {
					t.Fatalf("expected four separate rolls and no freely lootable marks, got %v and %d", rolls, corpseItems)
				}
				for _, id := range rolls {
					h.RaidRollChoice(r.cora, id, rollNeed) // the melee character is refused
					h.RaidRollChoice(r.cora, id, rollGreed)
					h.RaidRollChoice(r.anna, id, rollNeed)
				}
				if votes != 8 {
					t.Fatalf("expected two eligible votes on each mark, got %d", votes)
				}
			} else {
				if len(rolls) != 0 || corpseItems != 4 {
					t.Fatalf("expected four marks in the corpse window, got %v and %d", rolls, corpseItems)
				}
				if err := h.HandleLootAll(r.anna, incoming(protocol.OpcodeLootTakeAll, nil)); err != nil {
					t.Fatal(err)
				}
			}
			pack, err := inv.GetPackItems(r.pa.CharServerID)
			if err != nil || len(pack) != 1 || pack[0].Pattern.ItemName != "Nagafen's Mark of Steel" || pack[0].StackLeft != 4 {
				t.Fatalf("expected four marks together in the winner's pack, got %+v (%v)", pack, err)
			}
			pack, err = inv.GetPackItems(r.pc.CharServerID)
			if err != nil || len(pack) != 0 {
				t.Fatalf("Greed received a mark over Need: %+v (%v)", pack, err)
			}
		})
	}
}

func TestHeroicMarksBoardAndChargedPort(t *testing.T) {
	r, board, _ := mythicPortalRig(t)
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "marks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seedHeroicMarkPrerequisites(t, db)
	if err := r.gw.LoadHeroicMarks(db); err != nil {
		t.Fatal(err)
	}
	if _, err := r.gw.LoadSpellGates(db); err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	r.ih.heroicMarkInventory = inv
	r.pa.Class, r.pa.Level = 7, 60
	if _, err := db.Exec(`INSERT INTO characters
		(serverid,charName,accountid,modelid,tclass,race,humType,level,haircolor,hairlength,hairstyle,faceoption,sex)
		VALUES (?, 'Anna', 1, 0, 7, 0, 0, 60, 0, 0, 0, 0, 0)`, r.pa.CharServerID); err != nil {
		t.Fatal(err)
	}
	set := database.HeroicMarksFor(entity.EncounterMythicNagafen)
	role := set.RoleFor(r.pa.Class)
	if _, err := inv.AddCharItem(r.pa.CharServerID, role.MarkID, 4, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	pack, err := inv.GetPackItems(r.pa.CharServerID)
	if err != nil {
		t.Fatal(err)
	}
	r.anna.SetInventoryList(newInvList(pack))
	box := func() uint32 {
		heroicRaids.mu.Lock()
		defer heroicRaids.mu.Unlock()
		return heroicRaids.asked[r.pa.CharServerID].box
	}
	r.talk(r.anna, board)
	if got := askOf(t, r.msgs(r.anna)); !strings.HasSuffix(got, "|Mythic Lord Nagafen|Spend marks") {
		t.Fatal(got)
	}
	r.ih.answerHeroicRaids(r.anna, r.pa, box(), 1)
	if got := askOf(t, r.msgs(r.anna)); !strings.Contains(got, role.MarkName) ||
		!strings.Contains(got, "|Unlock God Touched II (3 marks)|Nagafen's Hearthfire Ring (5 marks)|Solusek Bridge Stone (1 mark)|") {
		t.Fatal(got)
	}
	r.ih.answerHeroicRaids(r.anna, r.pa, box(), 0)
	old := box()
	r.ih.answerHeroicRaids(r.anna, r.pa, old, 0)
	r.ih.answerHeroicRaids(r.anna, r.pa, old, 0)
	if got := sessionInvList(r.anna).held(role.MarkID); got != 1 {
		t.Fatalf("remaining after buying unlock once: %d", got)
	}
	cm := NewClassMasteryHandler(r.sm)
	cm.SetGameWorld(r.gw)
	cm.SetStore(database.NewSQLiteCharacterStore(db))
	if !cm.cmKnowsOf(r.pa).HeroicUnlocks[set.Boss] || cm.cmKnowsOf(r.pc).HeroicUnlocks[set.Boss] {
		t.Fatal("mastery unlock was not character-specific")
	}
	r.talk(r.anna, board)
	r.ih.answerHeroicRaids(r.anna, r.pa, box(), 1)
	r.ih.answerHeroicRaids(r.anna, r.pa, box(), 2)
	confirm := box()
	r.gw.UpdatePosition(r.pa.ServerID, 5100, 50, 5100, 0)
	r.ih.answerHeroicRaids(r.anna, r.pa, confirm, 0)
	if got := sessionInvList(r.anna).held(role.MarkID); got != 1 {
		t.Fatal("a confirmation from afar consumed marks")
	}
	r.gw.UpdatePosition(r.pa.ServerID, 5000, 50, 5000, 0)
	r.talk(r.anna, board)
	r.ih.answerHeroicRaids(r.anna, r.pa, box(), 1)
	r.ih.answerHeroicRaids(r.anna, r.pa, box(), 2)
	r.ih.answerHeroicRaids(r.anna, r.pa, box(), 0)
	pack, err = inv.GetPackItems(r.pa.CharServerID)
	if err != nil || len(pack) != 1 || pack[0].Pattern.PatternID != set.Teleport.ID || pack[0].Charges != 50 {
		t.Fatalf("stone purchase got %+v, %v", pack, err)
	}
	h := &EquipHandler{inventoryStore: inv, gameWorld: r.gw, spells: database.NewSpellbookStore(db)}
	now := time.Now()
	if !h.useItemSpell(r.anna, r.pa, 0, &pack[0], 0) {
		t.Fatal("the bridge stone did not start a cast")
	}
	r.gw.Tick(now.Add(time.Second))
	if r.pa.World != 0 {
		t.Fatal("landed before two seconds")
	}
	r.gw.Tick(now.Add(3 * time.Second))
	to := set.Teleport
	if r.pa.World != uint8(to.World) || r.pa.X != to.X || r.pa.Y != to.Y || r.pa.Z != to.Z {
		t.Fatalf("stone landed world %d at (%g,%g,%g)", r.pa.World, r.pa.X, r.pa.Y, r.pa.Z)
	}
	if r.pc.World != 0 {
		t.Fatal("the stone teleported another character")
	}
	stone, err := inv.GetCharItem(r.pa.CharServerID, pack[0].ItemID)
	if err != nil || stone.Charges != 49 {
		t.Fatalf("after cast: %d charges, %v", stone.Charges, err)
	}
}
