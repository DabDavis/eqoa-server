package gmweb

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// castsTo is every 0x00B1 queued for s: caster, target, ms, visual.
func castsTo(s *session.Session) [][4]uint32 {
	var out [][4]uint32
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 19 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeCastingCombat {
			le := binary.LittleEndian
			out = append(out, [4]uint32{le.Uint32(d[2:]), le.Uint32(d[6:]), le.Uint32(d[11:]), le.Uint32(d[15:])})
		}
	}
	return out
}

// A GM plays a visual in the game from the panel (only with their own
// character in the world; a read token may not), gives a spell one through
// the audited row edit, reads what spells play and which use which; and a
// visual set in the game (#spellfx) is in the panel's log and reverts there.
func TestSpellVisualsFromThePanel(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")

	// not in the game: refused, nothing played
	if code, out := c.op("play_visual", map[string]any{"visual": "0x1DAEB5B6"}); code != 400 || !strings.Contains(out["error"].(string), "not in the game") {
		t.Fatalf("play_visual out of the game: %d %v", code, out)
	}

	// the GM's character (account 2) in the world
	sess := f.world.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 5009}, 9, 9, 9, 1, true)
	sess.SetAccountID(2)
	sess.SetPlayerObjectID(500000009)
	f.world.CreatePlayer(sess, &database.CharacterRecord{ServerID: 9, CharName: "Gamemaster", X: 25000, Y: 50, Z: 15000})
	sess.SetCharacterInWorld(true)

	code, out := c.op("play_visual", map[string]any{"visual": "1DAEB5B6", "seconds": 1.5})
	if code != 200 || out["on"] != "Gamemaster" {
		t.Fatalf("play_visual: %d %v", code, out)
	}
	if got := castsTo(sess); len(got) != 1 || got[0] != [4]uint32{500000009, 500000009, 1500, 0x1DAEB5B6} {
		t.Fatalf("the 0x00B1 played: %v", got)
	}
	sess.Queue().GetPending()

	// Minor Healing has no effect row: what a cast of it plays, the
	// stand-in of the kind its name reads as
	code, out = c.op("spell_visual", map[string]any{"spell": "Minor Healing"})
	kind, _ := out["standIn"].(string)
	if code != 200 || kind == "" || out["own"] != nil || out["plays"] != handlers.Hex32(entity.StandInVisuals()[kind]) {
		t.Fatalf("spell_visual: %d %v", code, out)
	}
	code, out = c.op("set_spell_visual", map[string]any{"spell": 100, "visual": "0x12AB34CD"})
	if code != 200 {
		t.Fatalf("set_spell_visual: %d %v", code, out)
	}
	if ch := out["change"].(map[string]any); ch["action"] != "insert" || ch["audit"] == nil {
		t.Fatalf("set_spell_visual's change: %v", ch)
	}
	if v := out["visual"].(map[string]any); v["own"] != "0x12AB34CD" || v["plays"] != "0x12AB34CD" {
		t.Fatalf("set_spell_visual's visual: %v", v)
	}
	if code, out = c.op("set_spell_visual", map[string]any{"spell": "Minor Healing", "visual": 0x0000BEEF}); code != 200 || out["change"].(map[string]any)["action"] != "update" {
		t.Fatalf("set_spell_visual again: %d %v", code, out)
	}
	code, out = c.op("play_visual", map[string]any{"spell": 100})
	if code != 200 {
		t.Fatalf("play_visual spell: %d %v", code, out)
	}
	if got := castsTo(sess); len(got) != 1 || got[0][3] != 0xBEEF {
		t.Fatalf("play_visual spell played: %v", got)
	}
	sess.Queue().GetPending()
	// a visual with a spell: the spell's cast time (Root's 2000 s here, held
	// to the 30 s most)
	if code, out = c.op("play_visual", map[string]any{"visual": "0x1DAEB5B6", "spell": "Root"}); code != 200 {
		t.Fatalf("play_visual visual+spell: %d %v", code, out)
	}
	if got := castsTo(sess); len(got) != 1 || got[0][3] != 0x1DAEB5B6 || got[0][2] != 30000 {
		t.Fatalf("play_visual visual+spell played: %v", got)
	}
	sess.Queue().GetPending()

	code, out = c.op("visual_uses", nil)
	if code != 200 {
		t.Fatalf("visual_uses: %d %v", code, out)
	}
	used := out["used"].([]any)
	if len(used) != 1 || used[0].(map[string]any)["visual"] != "0x0000BEEF" || !strings.Contains(fmt.Sprint(used[0]), "Minor Healing") {
		t.Fatalf("visual_uses' used: %v", used)
	}
	// Root (101) alone plays a stand-in now
	total := 0.0
	for _, s := range out["standIns"].([]any) {
		total += s.(map[string]any)["spells"].(float64)
	}
	if total != 1 || out["spells"].(float64) != 2 {
		t.Errorf("visual_uses' stand-ins: %v of %v spells", out["standIns"], out["spells"])
	}

	// a read token: the reads, never the playback or the write
	_, tok := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "ro", "scope": "read", "days": 1})
	ro := f.bearer(tok["token"].(string))
	if code, _ := ro.op("spell_visual", map[string]any{"spell": 101}); code != 200 {
		t.Fatalf("a read token's spell_visual: %d", code)
	}
	if code, _ := ro.op("play_visual", map[string]any{"visual": "0x1DAEB5B6"}); code == 200 {
		t.Fatal("a read token played a visual")
	}
	if code, _ := ro.op("set_spell_visual", map[string]any{"spell": 101, "visual": "0x1"}); code == 200 {
		t.Fatal("a read token set a visual")
	}
	if got := castsTo(sess); len(got) != 0 {
		t.Fatalf("the read token's play_visual played: %v", got)
	}

	// #spellfx in the game: in the panel's log, and reverted there
	before, _, err := database.NewSpellbookStore(f.db).SetSpellVisual(100, 0xCAFEF00D, database.GMEdit{AccountID: 2, Via: "game", IP: "203.0.113.9"}, "#spellfx in the game")
	if err != nil || before != 0xBEEF {
		t.Fatalf("SetSpellVisual: %#x %v", before, err)
	}
	code, out = c.op("audit_log", map[string]any{"table": "spelleffect", "key": map[string]any{"spellid": 100}})
	if code != 200 {
		t.Fatalf("audit_log: %d %v", code, out)
	}
	entries := out["entries"].([]any)
	if len(entries) != 3 {
		t.Fatalf("spell 100's history: %d entries, want 3: %v", len(entries), entries)
	}
	game := entries[0].(map[string]any)
	if game["via"] != "game" || game["username"] != "gm" || game["action"] != "update" {
		t.Fatalf("the game's entry: %v", game)
	}
	code, out = c.op("revert_change", map[string]any{"id": game["id"]})
	if code != 200 {
		t.Fatalf("revert of the game's change: %d %v", code, out)
	}
	if code, out = c.op("spell_visual", map[string]any{"spell": 100}); out["own"] != "0x0000BEEF" {
		t.Fatalf("after the revert: %d %v", code, out)
	}
	// the control: a revert of the same entry again is refused
	if code, _ := c.op("revert_change", map[string]any{"id": game["id"]}); code == 200 {
		t.Fatal("an entry reverted twice")
	}
}
