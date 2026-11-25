package entity

import (
	"math/bits"
	"testing"
	"time"
)

// Petition 3069 (mercwere.go): a merc given a were form wears it as a player does, its
// body and its stats, until it is taken away; another form takes the old one's place.

// wolfwereJSON is the wolf line's own form as the workbook gives it (spelleffect.effects).
const wolfwereJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":30,"min_level":1,"req_equipment":"Any","sheet_type":"Alter Appearance","effects":[{"id":"0950","type":"Alter Appearance","kind":"illusion","sprite":"0x23A3EDDB"},{"id":"0951","type":"Statistic Change","kind":"stat","stat":"str","amount":45},{"id":"0951","type":"Statistic Change","kind":"stat","stat":"sta","amount":45},{"type":"Statistic Change","kind":"stat","stat":"maxhp","amount":250},{"id":"0952","type":"Statistic Change","kind":"stat","stat":"int","amount":-16,"resist":"ar"}]}`

const direWolfJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":36000,"stack":0,"level":30,"min_level":1,"req_equipment":"Any","sheet_type":"Alter Appearance","effects":[{"id":"0920","type":"Alter Appearance","kind":"illusion","sprite":"0x63AA64AE"},{"id":"0921","type":"Statistic Change","kind":"stat","stat":"str","amount":120},{"id":"0921","type":"Statistic Change","kind":"stat","stat":"sta","amount":120},{"id":"0921","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":350},{"id":"0922","type":"Statistic Change","kind":"stat","stat":"int","amount":-50,"resist":"ar"}]}`

func mercWereTestSpell(id int, name, line, effects string) *CastSpell {
	return &CastSpell{SpellID: id, Level: 30, Name: name, Icon: 0x1234, Def: SpellDefFrom(name, line, "Assume the form of a wolf.", effects)}
}

func TestMercTakesItsWereForm(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	m := ms.npc
	r.run(0, time.Second)
	own, _ := m.Body()
	r.w.mu.Lock()
	hp, str := m.MaxHP, ms.body.StatNow(StatStr)
	r.w.mu.Unlock()

	wolf := mercWereTestSpell(901824, "Wolfwere", "Self, Gives Form, Increase STR by 45, Increase STA by 45, Increase Maximum HP by 250, Decrease INT by 16", wolfwereJSON)
	r.w.SetMercKit(r.p.CharServerID, ms.slot, withWere(ms.kit, wolf))
	r.run(1100*time.Millisecond, 2*time.Second)
	r.w.mu.Lock()
	body, _ := m.Body()
	want := bits.ReverseBytes32(0x23A3EDDB) // the workbook's sprite is byte-reversed (spelljson.go)
	if body != want || body == own {
		t.Errorf("its body %#x in the wolfwere form, want %#x (its own %#x)", body, want, own)
	}
	if got := ms.body.StatNow(StatStr); got != str+45 {
		t.Errorf("STR %d in the form, want %d", got, str+45)
	}
	if m.MaxHP < hp+250 { // its 250 and what its STA adds
		t.Errorf("most health %d in the form, want at least %d", m.MaxHP, hp+250)
	}
	n := 0
	for _, e := range m.Effects {
		if e.SpellID == wolf.SpellID {
			n++
			if e.Until.Sub(r.at(2*time.Second)) < 24*time.Hour {
				t.Errorf("the form ends in %v: a were form lasts until dropped", e.Until.Sub(r.at(2*time.Second)))
			}
		}
	}
	if n != 1 {
		t.Errorf("%d wolfwere effects on it, want 1", n)
	}
	r.w.mu.Unlock()

	// another form in its place
	dire := mercWereTestSpell(4412, "Dire Wolf Form", "Self, Gives Form, Increase STR by 120, Increase STA by 120, Increase Maximum HP by 350, Decrease INT by 50", direWolfJSON)
	r.w.SetMercKit(r.p.CharServerID, ms.slot, withWere(ms.kit, dire))
	r.run(2100*time.Millisecond, 3*time.Second)
	r.w.mu.Lock()
	body, _ = m.Body()
	if body != bits.ReverseBytes32(0x63AA64AE) {
		t.Errorf("its body %#x as a dire wolf", body)
	}
	for _, e := range m.Effects {
		if e.SpellID == wolf.SpellID {
			t.Error("the wolfwere form stayed on under the dire wolf's")
		}
	}
	if got := ms.body.StatNow(StatStr); got != str+120 {
		t.Errorf("STR %d as a dire wolf, want %d", got, str+120)
	}
	r.w.mu.Unlock()

	// none: itself again
	r.w.SetMercKit(r.p.CharServerID, ms.slot, withWere(ms.kit, nil))
	r.run(3100*time.Millisecond, 4*time.Second)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if body, _ := m.Body(); body != own {
		t.Errorf("its body %#x with no form, want its own %#x", body, own)
	}
	if m.MaxHP != hp || ms.body.StatNow(StatStr) != str {
		t.Errorf("most health %d, STR %d with no form: want %d, %d", m.MaxHP, ms.body.StatNow(StatStr), hp, str)
	}
}

func withWere(k MercKit, s *CastSpell) MercKit {
	k.Were = s
	return k
}
