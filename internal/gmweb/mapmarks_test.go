package gmweb

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/mapmarks"
)

// suggestMark keeps a map suggestion as the public route does, from the sender
// whose hash is given.
func suggestMark(t *testing.T, f *fixture, world int, x, z float64, hash, name, note string) int64 {
	t.Helper()
	id, _, err := f.db.AddMapMarkSuggestion(database.MapMarkSuggestion{World: world, X: x, Z: z, Name: name, Note: note,
		Client: "dev-test", SenderHash: hash}, time.Now(), mapmarks.ReplaceWithin, 20, 5000)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// publicMarks is the public route's list as a map reads it.
func publicMarks(t *testing.T, s *mapmarks.Service) []map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeMarks(w, httptest.NewRequest("GET", "/api/mapmarks", nil))
	var out struct{ Marks []map[string]any }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("the public list: %v %s", err, w.Body)
	}
	return out.Marks
}

// A suggestion is listed (its sender by its hash's first 8 characters, the marks
// near it), accepted with the GM's edits into mapmarks through the audited row
// edit (on the public list at once; its note held to the route's rule, a line
// break a space), reverted off it again, refused when it is
// not waiting; dismissed once; a sender's waiting ones cleared, never with fewer
// than 8 characters or a wildcard; a read token lists and never decides; the row
// editor holds a mark to the route's rules.
func TestCommunityMapMarksAreAcceptedAndRevertible(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	pub := mapmarks.New(f.db)
	well := suggestMark(t, f, 0, 25100.4, 15200.6, "aaaaaaaa11111111", "the old well", "drink here\nlater")
	cave := suggestMark(t, f, 0, 25110, 15210, "bbbbbbbb22222222", "<b>Cave</b>", "kept as typed")
	odus := suggestMark(t, f, 2, 100, 100, "aaaaaaaa11111111", "Odus Rock", "")

	code, out := c.op("map_suggestions", nil)
	if code != 200 {
		t.Fatalf("map_suggestions: %d %v", code, out)
	}
	list := out["suggestions"].([]any)
	if len(list) != 3 || out["waiting"].(float64) != 3 {
		t.Fatalf("listed %d, %v waiting: %v", len(list), out["waiting"], out)
	}
	if s := fmt.Sprint(out); !strings.Contains(s, "aaaaaaaa") || strings.Contains(s, "11111111") || !strings.Contains(s, "<b>Cave</b>") {
		t.Errorf("senders by 8 characters, names as typed: %v", s)
	}
	if code, out = c.op("map_suggestions", map[string]any{"world": 2}); code != 200 || len(out["suggestions"].([]any)) != 1 {
		t.Fatalf("world 2: %d %v", code, out)
	}
	if code, _ = c.op("map_suggestions", map[string]any{"status": "new"}); code != 400 {
		t.Errorf("a status that is not one: %d", code)
	}

	// a read token lists, never decides
	_, tok := c.do("POST", "/api/tokens", map[string]any{"action": "create", "name": "ro", "scope": "read", "days": 1})
	ro := f.bearer(tok["token"].(string))
	if code, _ := ro.op("map_suggestions", nil); code != 200 {
		t.Fatalf("a read token's map_suggestions: %d", code)
	}
	if code, _ := ro.op("accept_map_suggestion", map[string]any{"id": well}); code == 200 {
		t.Fatalf("a read token accepted a suggestion")
	}

	if len(publicMarks(t, pub)) != 0 {
		t.Fatalf("a mark on the public list before any accept")
	}
	// the GM's edits: a name a player would be refused, then a kind that is not one
	if code, _ = c.op("accept_map_suggestion", map[string]any{"id": well, "name": "the ass well"}); code != 400 {
		t.Errorf("accepting a profane name: %d", code)
	}
	if code, _ = c.op("accept_map_suggestion", map[string]any{"id": well, "kind": "tavern"}); code != 400 {
		t.Errorf("accepting a kind that is not one: %d", code)
	}
	code, out = c.op("accept_map_suggestion", map[string]any{"id": well, "name": "The Old Well", "kind": "camp", "x": 25101})
	if code != 200 {
		t.Fatalf("accept: %d %v", code, out)
	}
	ch := out["change"].(map[string]any)
	audit := ch["audit"]
	mark := out["mark"].(map[string]any)
	if audit == nil || mark["name"] != "The Old Well" || mark["kind"] != "camp" || mark["x"].(float64) != 25101 || mark["z"].(float64) != 15200.6 ||
		mark["note"] != "drink here later" || mark["source"] != "suggestion" || mark["suggestion_id"].(float64) != float64(well) {
		t.Fatalf("accept's answer: %v", out)
	}
	marks := publicMarks(t, pub)
	if len(marks) != 1 || marks[0]["name"] != "The Old Well" || marks[0]["kind"] != "camp" || marks[0]["x"].(float64) != 25101 {
		t.Fatalf("the public list after the accept: %v", marks)
	}
	if sg, _ := f.db.MapMarkSuggestionByID(well); sg.Status != database.MapMarkAccepted || sg.MarkID != int64(mark["id"].(float64)) || sg.DecidedBy != "gm" {
		t.Errorf("the suggestion after the accept: %+v", sg)
	}
	if code, _ = c.op("accept_map_suggestion", map[string]any{"id": well}); code != 400 {
		t.Errorf("accepting it twice: %d", code)
	}
	// the cave, 14 off the well: the mark is near it
	code, out = c.op("map_suggestions", map[string]any{"world": 0})
	near := ""
	for _, s := range out["suggestions"].([]any) {
		if m := s.(map[string]any); m["id"].(float64) == float64(cave) {
			near = fmt.Sprint(m["near"])
		}
	}
	if !strings.Contains(near, "The Old Well") {
		t.Errorf("the cave's near marks: %q", near)
	}
	// the map's mark is a row edit: revert_change takes it off every map
	if code, out = c.op("revert_change", map[string]any{"id": audit}); code != 200 {
		t.Fatalf("revert: %d %v", code, out)
	}
	if marks := publicMarks(t, pub); len(marks) != 0 {
		t.Errorf("the public list after the revert: %v", marks)
	}

	// the row editor: the route's rules hold a GM's edit too
	code, out = c.op("accept_map_suggestion", map[string]any{"id": cave, "name": "Hidden Cave"})
	if code != 200 {
		t.Fatalf("accept the cave: %d %v", code, out)
	}
	key := out["change"].(map[string]any)["key"]
	if code, _ = c.op("update_row", map[string]any{"table": "mapmarks", "key": key, "values": map[string]any{"kind": "tavern"}}); code != 400 {
		t.Errorf("a row edit with a kind that is not one: %d", code)
	}
	if code, _ = c.op("update_row", map[string]any{"table": "mapmarks", "key": key, "values": map[string]any{"x": 300000}}); code != 400 {
		t.Errorf("a row edit off every map: %d", code)
	}
	if code, out = c.op("update_row", map[string]any{"table": "mapmarks", "key": key, "values": map[string]any{"name": "Hidden Grotto"}}); code != 200 {
		t.Fatalf("a row edit: %d %v", code, out)
	}
	if marks := publicMarks(t, pub); len(marks) != 1 || marks[0]["name"] != "Hidden Grotto" {
		t.Errorf("the public list after the row edit: %v", marks)
	}

	// dismiss, once
	if code, out = c.op("dismiss_map_suggestion", map[string]any{"id": odus}); code != 200 || out["dismissed"].(float64) != 1 {
		t.Fatalf("dismiss: %d %v", code, out)
	}
	if code, _ = c.op("dismiss_map_suggestion", map[string]any{"id": odus}); code != 400 {
		t.Errorf("dismissing what is no longer waiting: %d", code)
	}

	// a spammer's waiting rows, by the hash's first 8 characters; never fewer
	suggestMark(t, f, 1, 10, 10, "aaaaaaaa11111111", "Spam One", "")
	suggestMark(t, f, 1, 900, 900, "aaaaaaaa11111111", "Spam Two", "")
	if code, _ = c.op("clear_map_suggestions", map[string]any{"sender": "aaaa"}); code != 400 {
		t.Errorf("a 4-character sender: %d", code)
	}
	if code, _ = c.op("clear_map_suggestions", map[string]any{"sender": "aaaaaaa%"}); code != 400 {
		t.Errorf("a wildcard sender: %d", code)
	}
	if code, out = c.op("clear_map_suggestions", map[string]any{"sender": "AAAAAAAA"}); code != 200 || out["deleted"].(float64) != 2 {
		t.Fatalf("clear: %d %v", code, out)
	}
	// the sender's decided rows stay (the well accepted, Odus dismissed)
	var left int
	f.db.QueryRow(`SELECT COUNT(*) FROM mapmark_suggestions WHERE sender_hash LIKE 'aaaaaaaa%'`).Scan(&left)
	if left != 2 {
		t.Errorf("%d of the sender's rows left, want its 2 decided", left)
	}
	if code, out = c.op("map_marks", map[string]any{"world": 0}); code != 200 || len(out["marks"].([]any)) != 1 {
		t.Errorf("map_marks: %d %v", code, out)
	}
}

// The senders' salt stays out of every page: the table is hidden from rows.
func TestTheMapMarkSaltIsNotATable(t *testing.T) {
	f := newFixture(t)
	if _, err := f.db.MapMarkSalt(); err != nil {
		t.Fatal(err)
	}
	c := f.browser()
	c.login("boss")
	if code, out := c.op("query_rows", map[string]any{"table": "mapmark_meta"}); code == 200 {
		t.Fatalf("mapmark_meta read through rows: %v", out)
	}
	if code, out := c.op("list_tables", nil); code != 200 || strings.Contains(fmt.Sprint(out), "mapmark_meta") {
		t.Fatalf("list_tables names mapmark_meta: %d", code)
	}
	if code, out := c.op("query_rows", map[string]any{"table": "mapmark_suggestions"}); code != 200 {
		t.Errorf("the suggestions themselves are a table: %d %v", code, out)
	}
}

// Names and notes are what a player typed: the page and the map's layer put
// them in as text, never as markup.
func TestMapSuggestionsShowTextAsText(t *testing.T) {
	for _, file := range []string{"static/views/mapmarks.js", "static/views/map.js"} {
		src, err := fs.ReadFile(staticFS, file)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s uses %s", file, bad)
			}
		}
	}
}
