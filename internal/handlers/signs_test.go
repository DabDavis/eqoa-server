package handlers

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// signRows gives the handler these rows of signtext.
func signRows(h *InteractHandler, rows ...database.SignText) {
	m := make(map[int]database.SignText, len(rows))
	for _, r := range rows {
		m[r.NPCID] = r
	}
	h.SetSignTexts(func() (map[int]database.SignText, error) { return m, nil })
}

// A sign says its row of signtext, its lines in one dialogue box; the
// controls: a town's person beside it still says its town line, and an NPC
// a GM gave a row says it as a sign (never its town line), and without the
// row says its town line again.
func TestASignSaysItsWords(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	const human = 0x70D898C6
	tw.w.SetPlayerModels(map[uint32]bool{human: true})
	tw.w.SetNPCFactions([]database.NPCFactionRow{{Name: "vahji shandun", World: -1, Faction: "Citizens of Freeport"},
		{Name: "dhea crier", World: -1, Faction: "Citizens of Freeport"},
		{Name: "signpost", World: -1, Faction: "Citizens of Freeport"}})
	tw.h.SetDialogLines(func() ([]database.NPCDialogLine, error) {
		return []database.NPCDialogLine{{ID: 1, Town: "Freeport", Line: "Gold talks in %t, %p.", Race: -1, Weight: 1}}, nil
	})
	road := database.SignText{NPCID: 702, Kind: database.SignKindRoad, Text: "North: Highpass Hold\nSouth: Freeport"}
	crier := database.SignText{NPCID: 703, Kind: database.SignKindGM, Text: "Hear ye: the Militia is hiring."}
	signRows(tw.h, road, crier)
	sign := tw.add(1, &entity.NPC{ServerID: 702, Name: "signpost", ModelID: SignBody, SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	vahji := tw.add(2, &entity.NPC{ServerID: 691, Name: "Vahji Shandun", ModelID: human, SpawnX: 25403.2, SpawnY: 54.1, SpawnZ: 15725.3})
	dhea := tw.add(3, &entity.NPC{ServerID: 703, Name: "Dhea Crier", ModelID: human, SpawnX: 25402, SpawnY: 54.1, SpawnZ: 15724})

	tw.talk(sign)
	if m := questMsgs(tw.s); len(m) != 1 || m[0] != "box:North: Highpass Hold\nSouth: Freeport" {
		t.Errorf("the sign said %q", m)
	}
	tw.talk(vahji)
	if m := questMsgs(tw.s); len(m) != 1 || m[0] != "box:Vahji Shandun: Gold talks in Freeport, Talker." {
		t.Errorf("the control, the town's person beside it: %q", m)
	}
	tw.talk(dhea)
	if m := questMsgs(tw.s); len(m) != 1 || m[0] != "box:Hear ye: the Militia is hiring." {
		t.Errorf("a townsperson with a row is a sign: %q", m)
	}
	signRows(tw.h, road)
	tw.talk(dhea)
	if m := questMsgs(tw.s); len(m) != 1 || m[0] != "box:Dhea Crier: Gold talks in Freeport, Talker." {
		t.Errorf("the control, its row taken away: %q", m)
	}
}

// A sign never gives a quest nor has a step done at it: a sign named as a
// quest's giver offers nothing, and one named as a step's NPC does not do
// the step. The controls: the giver and the step's NPC on a body of their
// own offer the quest and do the step.
func TestASignGivesNoQuest(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.h.SetSignTexts(f.db.SignTexts)
	f.exec(`INSERT INTO signtext (npcid, kind, text, source) VALUES (801, 'road', 'East: Freeport', 'ours')`)
	database.InvalidateSignText()
	defer database.InvalidateSignText()
	signMalsis := f.tw.add(1, &entity.NPC{ServerID: 801, Name: "Malsis", ModelID: SignBody, SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	signKellina := f.tw.add(2, &entity.NPC{ServerID: 802, Name: "Kellina", ModelID: SignBody, SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	malsis := f.tw.add(3, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15725})
	kellina := f.tw.add(4, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404.5, SpawnY: 54.1, SpawnZ: 15726.5})

	f.tw.talk(signMalsis)
	m := f.msgs()
	if len(m) != 1 || m[0] != "box:East: Freeport" {
		t.Errorf("the sign named Malsis: %q, want its words alone", m)
	}
	f.tw.talk(malsis)
	m = f.msgs()
	f.want(m, "ask#*")
	f.tw.pick(boxID(m), 0)
	f.msgs()
	if s, _ := f.progress(10010); s != 1 {
		t.Fatalf("taken: step %d, want 1", s)
	}
	f.tw.talk(signKellina)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("the sign named Kellina, with no row and no npc_text: %q, want nothing", m)
	}
	if s, _ := f.progress(10010); s != 1 {
		t.Errorf("the step was done at a sign: step %d", s)
	}
	f.tw.talk(kellina)
	f.want(f.msgs(), "add:Bring Kellina two cracked ant pincers.")
}

// A sign with no row says world.npc_text's words for its name, else
// nothing at all; a board says its words and then the news row's, the news
// alone with no row of its own, and no other sign says the news.
func TestABoardSaysTheNews(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	welcome := database.SignText{NPCID: 462, Kind: database.SignKindBoard, Text: "Welcome to Freeport."}
	road := database.SignText{NPCID: 702, Kind: database.SignKindRoad, Text: "East: Freeport"}
	news := database.SignText{NPCID: database.SignNewsID, Kind: database.SignKindNews, Text: "The bank opens at dawn."}
	board := tw.add(1, &entity.NPC{ServerID: 462, Name: "Freeport Announcements", ModelID: SignBody, SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	sign := tw.add(2, &entity.NPC{ServerID: 702, Name: "signpost", ModelID: SignBody, SpawnX: 25402, SpawnY: 54.1, SpawnZ: 15724})
	bare := tw.add(3, &entity.NPC{ServerID: 2060, Name: "Neriak Announcements", ModelID: SignBody, SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726})
	unlisted := tw.add(4, &entity.NPC{ServerID: 705, Name: "signpost", ModelID: SignBody, SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15724})

	say := func(id uint32) []string { tw.talk(id); return questMsgs(tw.s) }
	signRows(tw.h, welcome, road)
	if m := say(board); len(m) != 1 || m[0] != "box:Welcome to Freeport." {
		t.Errorf("the board with no news: %q", m)
	}
	signRows(tw.h, welcome, road, news)
	if m := say(board); len(m) != 1 || m[0] != "box:Welcome to Freeport.\n\nThe bank opens at dawn." {
		t.Errorf("the board with news: %q", m)
	}
	if m := say(sign); len(m) != 1 || m[0] != "box:East: Freeport" {
		t.Errorf("a road sign said %q, want no news", m)
	}
	if m := say(bare); len(m) != 1 || m[0] != "box:The bank opens at dawn." {
		t.Errorf("a board with no row: %q, want the news", m)
	}
	if m := say(unlisted); len(m) != 0 {
		t.Errorf("a sign with no row and no npc_text: %q, want nothing", m)
	}
	SetNPCText(map[string]string{"signpost": "Turn back."})
	defer SetNPCText(nil)
	if m := say(unlisted); len(m) != 1 || m[0] != "box:Turn back." {
		t.Errorf("a sign with no row, by world.npc_text: %q", m)
	}
	if m := say(sign); len(m) != 1 || m[0] != "box:East: Freeport" {
		t.Errorf("a sign's row comes before world.npc_text: %q", m)
	}
	// a GM's temporary copy is never a row's sign, whatever id the world gave it
	copied := tw.add(5, &entity.NPC{ServerID: 702, Name: "Guard Kelsy", Temporary: true, SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15724})
	if m := say(copied); len(m) != 0 {
		t.Errorf("a temporary NPC with a sign's id: %q, want nothing", m)
	}
}

// What a sign says goes as CLIENT shows it: a '%' doubled (its text expander
// reads %x as a code and %% as a '%'), and a text past a box in boxes of at
// most 217 characters.
func TestASignsWordsOnTheWire(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	long := strings.Repeat("The road east is long and the bandits are many. ", 8)
	signRows(tw.h, database.SignText{NPCID: 702, Kind: database.SignKindGM, Text: "Taxes: 10%p of all goods"},
		database.SignText{NPCID: 703, Kind: database.SignKindGM, Text: long})
	odd := tw.add(1, &entity.NPC{ServerID: 702, Name: "signpost", ModelID: SignBody, SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	wordy := tw.add(2, &entity.NPC{ServerID: 703, Name: "signpost", ModelID: SignBody, SpawnX: 25402, SpawnY: 54.1, SpawnZ: 15724})
	tw.talk(odd)
	if m := questMsgs(tw.s); len(m) != 1 || m[0] != "box:Taxes: 10%%p of all goods" {
		t.Errorf("the box: %q", m)
	}
	tw.talk(wordy)
	m := questMsgs(tw.s)
	if len(m) < 2 {
		t.Fatalf("%d characters in %d boxes", len(long), len(m))
	}
	var said []string
	for _, b := range m {
		text := strings.TrimPrefix(b, "box:")
		if len([]rune(text)) > questBoxChars {
			t.Errorf("a box of %d characters", len([]rune(text)))
		}
		said = append(said, text)
	}
	if strings.Join(said, " ") != strings.TrimSpace(long) {
		t.Errorf("the boxes lost words: %q", said)
	}
}

// tools/seed_signs.py finds the signs by the body handlers stand them on.
func TestSeedSignsKeepsTheSignBody(t *testing.T) {
	src, err := os.ReadFile("../../tools/seed_signs.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^SIGN_BODY = 0x([0-9A-Fa-f]+)$`).FindSubmatch(src)
	if m == nil {
		t.Fatal("tools/seed_signs.py has no SIGN_BODY")
	}
	if v, _ := strconv.ParseUint(string(m[1]), 16, 32); v != SignBody {
		t.Errorf("the tool's SIGN_BODY is 0x%X, the server's 0x%X", v, SignBody)
	}
}
