package gmweb

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// The attack types' labels say what the server's combat code does with each:
// a value the server's melee takes as pierce is labelled Pierce, one it never
// swings says so, a ranged one reaches its Range.
func TestAttackTypeLabelsAreWhatCombatReads(t *testing.T) {
	for _, e := range enums()["attacktype"] {
		if e.Value == 0 {
			continue
		}
		kind := entity.MeleeKindOf(e.Value)
		s := entity.SwingFor(0, 1, [21]bool{}, true, e.Value, 37)
		for _, k := range []entity.Stat{entity.StatPierce, entity.StatSlash, entity.StatBlunt} {
			if strings.Contains(e.Label, entity.StatName(k)) && kind != k && !s.Never {
				t.Errorf("%d %q: labelled %s, its hits are %s", e.Value, e.Label, entity.StatName(k), entity.StatName(kind))
			}
		}
		switch {
		case s.Never:
			if !strings.Contains(e.Help, "never swings") {
				t.Errorf("%d %q never swings, its help: %s", e.Value, e.Label, e.Help)
			}
		case s.Reach == 37:
			if !strings.Contains(e.Help, "reaches its Range") {
				t.Errorf("%d %q reaches its range, its help: %s", e.Value, e.Label, e.Help)
			}
		default:
			if !strings.Contains(e.Help, "reaches 4.5") || !strings.Contains(e.Help, strings.ToLower(entity.StatName(kind))) {
				t.Errorf("%d %q reaches %.1f as %s, its help: %s", e.Value, e.Label, s.Reach, entity.StatName(kind), e.Help)
			}
		}
	}
	// the owner's example: "1hb is 3"; CLIENT's 4 is 2HB and its pierces 5 and 6
	for v, want := range map[int]string{3: "1H Blunt (3)", 4: "2H Blunt (4)", 5: "1H Pierce (5)", 6: "2H Pierce (6)", 7: "Bow (7)", 10: "Thrown (10)", 42: "unknown (42)"} {
		if got := attackTypeName(v); got != want {
			t.Errorf("attack type %d: %q, want %q", v, got, want)
		}
	}
	if entity.MeleeKindOf(5) != entity.StatPierce || entity.MeleeKindOf(3) != entity.StatBlunt || entity.MeleeKindOf(1) != entity.StatSlash {
		t.Error("the melee kinds moved")
	}
}

// wordsLine is a WORDS.TXT id's word: the id is the line's number from 1.
func wordsLine(t *testing.T, text string, id int) string {
	t.Helper()
	exact := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if id < 1 || id > len(exact) {
		t.Fatalf("WORDS.TXT has no line %d", id)
	}
	return strings.TrimSpace(exact[id-1])
}

// The attack type words are CLIENT's: its table gAttackWords (0x00CDF6A0, in
// a live game's EE memory) and the disc's WORDS.TXT, where this machine has
// them (neither is in the repository).
func TestAttackTypeWordsAreClients(t *testing.T) {
	home, _ := os.UserHomeDir()
	mem, err1 := os.ReadFile(home + "/claude-eqoa/memory-dumps/inworld-eqoa.live.eememory")
	words, err2 := os.ReadFile(home + "/claude-eqoa/extracted-assets/data/WORDS.TXT")
	if err1 != nil || err2 != nil {
		t.Skip("no EE dump or WORDS.TXT on this machine")
	}
	const gAttackWords = 0x00CDF6A0
	for i := 0; i < 11; i++ {
		word := int(int32(binary.LittleEndian.Uint32(mem[gAttackWords+8*i:])))
		id := int(int32(binary.LittleEndian.Uint32(mem[gAttackWords+8*i+4:])))
		w := wordsLine(t, string(words), word)
		var got *entity.AttackTypeWord
		for j := range entity.AttackTypeWords {
			if entity.AttackTypeWords[j].Value == id {
				got = &entity.AttackTypeWords[j]
			}
		}
		if got == nil || got.Word != w {
			t.Errorf("CLIENT's attack type %d is %q (word %d); ours %+v", id, w, word, got)
		}
	}
}

// Every column of the item record is named, and every coded one is a list.
func TestEveryItemColumnIsNamed(t *testing.T) {
	f := newFixture(t)
	tb, err := f.s.table("itempattern")
	if err != nil {
		t.Fatal(err)
	}
	coded := map[string]string{"attacktype": "attacktype", "procanim": "itemtype", "equipslot": "wearslot", "trade": "notrade",
		"rent": "rent", "lore": "lore", "craft": "craftmat", "unk3": "ammokind"}
	en := enums()
	for _, c := range tb.Columns {
		if c.Label == "" {
			t.Errorf("itempattern.%s has no label", c.Name)
		}
		if strings.HasPrefix(c.Label, "unk") {
			t.Errorf("itempattern.%s is labelled %q", c.Name, c.Label)
		}
		if want, ok := coded[c.Name]; ok {
			if c.Enum != want || len(en[want]) == 0 {
				t.Errorf("itempattern.%s: enum %q (%d values), want %q", c.Name, c.Enum, len(en[c.Enum]), want)
			}
		}
	}
	// the wear slots: CLIENT's -1 and 1-27 (sWearSlotWords), the cape's 28
	ws := map[int]bool{}
	for _, e := range en["wearslot"] {
		ws[e.Value] = true
	}
	for v := -1; v <= 28; v++ {
		if !ws[v] {
			t.Errorf("wear slot %d is not named", v)
		}
	}
	// No Rent is rent 0, backwards as CLIENT has it
	if en["rent"][0].Value != 0 || en["rent"][0].Label != "NO RENT" {
		t.Errorf("rent: %v", en["rent"])
	}
	// the races: the froglok's bit and each custom race beside the race it passes as
	bits := flagSets()["racemask"]
	if len(bits) != 11 || bits[10].Bit != 1024 {
		t.Errorf("race bits: %v", bits)
	}
}

// item_reading says a weapon's proc as the server reads it, an armour piece's
// hit effect, the coded columns by name, and reads an edit before it is saved.
func TestItemReadingSaysWhatTheServerReads(t *testing.T) {
	f := newFixture(t)
	mustExec(t, f.db, `INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent,
		unk4, attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore,
		unk6, craft, itemname, itemdesc, model, color)
		VALUES (20, 5, 0, 0, 0, 12, 0, 1, 1, 0, 5, 30, 0, 1, 1, 100, 0, 32767, 1023, 0, 1, 0, 0, 'Frostfang Dirk', 'Procs 150 Ice Damage.', 0, 0),
		       (21, 5, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, 'Crown of the Jaguar', '', 0, 0),
		       (22, 5, 0, 0, 0, 16, 0, 0, 0, 0, 7, 20, 40, 1, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, 'Plain Bow', '', 0, 0)`)
	c := f.browser()
	c.login("gm")
	code, out := c.op("item_reading", map[string]any{"item": 20})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	at := out["attackType"].(map[string]any)
	if at["name"] != "1H Pierce (5)" || at["word"] != "1HP" || !strings.Contains(at["reads"].(string), "pierce") {
		t.Errorf("attack type: %v", at)
	}
	proc, _ := out["proc"].(map[string]any)
	if proc == nil || !strings.Contains(proc["text"].(string), "hits for 150 cold damage") || proc["resist"] != "Cold" ||
		!strings.Contains(proc["text"].(string), "3% a landed hit") {
		t.Errorf("proc: %v", out)
	}
	if tags := out["tags"].([]any); len(tags) != 2 || tags[0] != "NO TRADE" || tags[1] != "LORE" {
		t.Errorf("tags: %v", tags)
	}
	// an edit read before it is saved: its own chance, a range, worn where it never procs
	_, out = c.op("item_reading", map[string]any{"item": 20, "values": map[string]any{
		"itemdesc": "Procs 400-600 fire, a 10% chance each hit.", "equipslot": "18", "attacktype": "3"}})
	proc, _ = out["proc"].(map[string]any)
	if proc == nil || !strings.Contains(proc["text"].(string), "hits for 400-600 fire damage, 10% a landed hit") ||
		!strings.Contains(proc["note"].(string), "never procs") || out["attackType"].(map[string]any)["name"] != "1H Blunt (3)" {
		t.Errorf("an edit: %v", out)
	}
	// a description that speaks of a proc the server cannot read
	_, out = c.op("item_reading", map[string]any{"item": 20, "values": map[string]any{"itemdesc": "A mighty proc."}})
	if out["proc"] != nil || out["procUnread"] == nil {
		t.Errorf("an unread proc: %v", out)
	}
	_, out = c.op("item_reading", map[string]any{"item": 21})
	if w, _ := out["worn"].(string); !strings.Contains(w, "30 damage back") || !strings.Contains(w, "15 health back") {
		t.Errorf("the crown: %v", out)
	}
	_, out = c.op("item_reading", map[string]any{"item": 22})
	if out["range"] != float64(40) || !strings.Contains(out["attackType"].(map[string]any)["reads"].(string), "reaches its Range") ||
		len(out["tags"].([]any)) != 1 {
		t.Errorf("the bow: %v", out)
	}
}
