package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Stone of Guenhwyvar from the item menu: she comes with no cast, named so,
// and the stone is kept; while she is out it does nothing; after she falls she
// rests. The control: another gem brings no one.
func TestTheStoneCallsGuenhwyvar(t *testing.T) {
	f := newScribeFixture(t)
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.player.HP, f.player.MaxHP, f.player.Level = 100, 100, 60
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.gameWorld = w
	w.NPCs[900] = &entity.NPC{ServerID: 900, Name: "Guenhwyvar", Level: 60, HP: 100, MaxHP: 100, ModelID: 0xAC4818AA, Size: 1, NPCType: 0x80, X: 20000, Z: 20000}
	if _, err := f.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power,
		iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (?, 60, 0, 0, 16528, 0, 0, 0, 0, 0, 0, -1, 'Summon Guenhwyvar', '')`, GuenhwyvarSpell); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?, 'Summons Guenhwyvar to aid you', 0, 'test',
		'{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Summon Pet","effects":[{"id":"F001","type":"Summon Pet","kind":"summon","pet":{"sheet_pet_id":"GUEN","name":"Guenhwyvar"}}]}')`, GuenhwyvarSpell); err != nil {
		t.Fatal(err)
	}
	insert := func(name string) int {
		res, err := f.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 7, 0, 0, 0, ?, '', 0, 0)`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	gem, stone := insert("Spitestone"), insert(GuenhwyvarStoneName)
	f.own(gem, 1)
	f.own(stone, 1)
	f.loadList()
	key := func(pattern int) uint32 {
		t.Helper()
		rows, err := f.inv.GetCharacterItems(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			if rows[i].Pattern.PatternID == pattern {
				return uint32(i)
			}
		}
		t.Fatalf("no item %d", pattern)
		return 0
	}
	pet := func() *entity.NPC {
		for _, n := range w.NPCs {
			if n.Owner == f.player.ServerID {
				return n
			}
		}
		return nil
	}
	said := func(sent [][]byte, words string) bool {
		var wide []byte
		for _, c := range words {
			wide = append(wide, byte(c), 0)
		}
		for _, m := range sent {
			if opOf(m) == protocol.OpcodeClientMessage && bytes.Contains(m, wide) {
				return true
			}
		}
		return false
	}
	f.use(key(gem))
	if pet() != nil {
		t.Fatal("another gem brought a pet")
	}
	sent := f.use(key(stone))
	p := pet()
	if p == nil || p.Name != "Guenhwyvar" {
		t.Fatalf("the stone brought %+v", p)
	}
	if !said(sent, "answers the stone") {
		t.Errorf("its line: %s", ops(sent))
	}
	if f.stackOf(GuenhwyvarStoneName) != 1 {
		t.Error("the stone was spent")
	}
	id := p.ServerID
	if sent = f.use(key(stone)); !said(sent, "already at your side") || pet().ServerID != id {
		t.Errorf("with her out: %s, pet %d (was %d)", ops(sent), pet().ServerID, id)
	}
	p.Dead = true
	if sent = f.use(key(stone)); !said(sent, "resting") {
		t.Errorf("after she fell: %s", ops(sent))
	}
	if IsGuenhwyvarStone(&database.ItemPatternRecord{EquipSlot: 18, ItemName: GuenhwyvarStoneName}) ||
		!IsGuenhwyvarStone(&database.ItemPatternRecord{EquipSlot: -1, ItemName: " stone of guenhwyvar"}) {
		t.Error("IsGuenhwyvarStone")
	}
}
