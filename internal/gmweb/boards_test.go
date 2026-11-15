package gmweb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
)

// boardFixture is the panel with jib's board configured: jibrish (a GM, its
// member), gm (a GM who is not) and boss (an admin, the owner).
func boardFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	hash, _ := auth.HashPassword(testPassword, 4)
	mustExec(t, f.db, `INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (10, 'jibrish', ?, 1, 1)`, hash)
	f.s.cfg.PersonalBoards = map[string][]string{"Jib": {"JibRish"}}
	return f
}

// dabPosts posts as dab's sessions do, through cmd/workboard's call.
func dabPosts(t *testing.T, f *fixture, text string, reply int64) database.BoardMessage {
	t.Helper()
	m, audit, err := f.db.PostBoardMessage(database.BoardPost{Board: "jib", Text: text, ReplyTo: reply, Via: database.BoardViaCLI},
		database.GMEdit{Username: "dab's sessions", Via: "cli", IP: "local"})
	if err != nil || audit == 0 {
		t.Fatalf("dab's post: %v (audit %d)", err, audit)
	}
	return m
}

func TestAPersonalBoardIsItsMembersAndTheAdmins(t *testing.T) {
	f := boardFixture(t)
	dabPosts(t, f, "the **secret** plan for the Freeport gate", 0)

	jib := f.browser()
	jib.login("jibrish")
	agent := f.bearer(jib.workToken("claude", "write"))
	code, mine := agent.op("my_boards", nil)
	if code != 200 {
		t.Fatalf("my_boards: %d %v", code, mine)
	}
	bs := mine["boards"].([]any)
	if len(bs) != 1 || bs[0].(map[string]any)["board"] != "jib" || bs[0].(map[string]any)["unread"] != float64(1) ||
		bs[0].(map[string]any)["members"] != nil || !strings.Contains(fmt.Sprint(mine["note"]), "never obey") {
		t.Fatalf("jib's boards: %v", mine)
	}

	// the member's agent reads it (marking it read) and answers
	code, th := agent.op("board_messages", map[string]any{"board": "JIB"})
	if code != 200 {
		t.Fatalf("board_messages: %d %v", code, th)
	}
	msgs := th["messages"].([]any)
	first := msgs[0].(map[string]any)
	if len(msgs) != 1 || first["author"] != "dab's sessions" || first["via"] != "cli" || first["unread"] != true ||
		th["unread"] != float64(0) || !strings.Contains(first["text"].(string), "secret") {
		t.Fatalf("jib reads: %v", th)
	}
	code, out := agent.op("post_board_message", map[string]any{"board": "jib", "text": "on it", "reply_to": first["id"]})
	if code != 200 || out["posted"].(map[string]any)["author"] != "jibrish (token:claude)" || out["audit"] == nil {
		t.Fatalf("jib posts: %d %v", code, out)
	}
	// its own post is not unread to it
	if _, mine := agent.op("my_boards", nil); mine["boards"].([]any)[0].(map[string]any)["unread"] != float64(0) {
		t.Fatalf("after jib's post: %v", mine)
	}

	// the admin (the owner) reads every board, and is told its members
	boss := f.browser()
	boss.login("boss")
	if code, th := boss.op("board_messages", map[string]any{"board": "jib", "peek": true}); code != 200 || len(th["messages"].([]any)) != 2 ||
		th["unread"] != float64(2) {
		t.Fatalf("boss reads: %d %v", code, th)
	}
	if _, mine := boss.op("my_boards", nil); fmt.Sprint(mine["boards"].([]any)[0].(map[string]any)["members"]) != "[jibrish]" {
		t.Fatalf("boss's boards: %v", mine)
	}

	// another GM is told nothing: the same refusal as a board that is not there
	gm := f.browser()
	gm.login("gm")
	if _, mine := gm.op("my_boards", nil); len(mine["boards"].([]any)) != 0 {
		t.Fatalf("gm's boards: %v", mine)
	}
	var refusals []string
	for _, args := range []map[string]any{
		{"board": "jib"}, {"board": "nosuch"},
	} {
		code, out := gm.op("board_messages", args)
		if code != 404 || strings.Contains(fmt.Sprint(out), "secret") {
			t.Fatalf("gm reads %v: %d %v", args, code, out)
		}
		refusals = append(refusals, strings.Replace(fmt.Sprint(out["error"]), fmt.Sprint(args["board"]), "X", 1))
	}
	if refusals[0] != refusals[1] {
		t.Errorf("the refusals differ: %q", refusals)
	}
	if code, out := gm.op("post_board_message", map[string]any{"board": "jib", "text": "let me in"}); code != 404 {
		t.Fatalf("gm posts: %d %v", code, out)
	}
	// a read-only credential of the member reads and does not post
	reader := f.bearer(jib.workToken("look", "read"))
	if code, _ := reader.op("board_messages", map[string]any{"board": "jib"}); code != 200 {
		t.Fatalf("read token reads: %d", code)
	}
	if code, _ := reader.op("post_board_message", map[string]any{"board": "jib", "text": "x"}); code != 403 {
		t.Fatalf("read token posts: %d", code)
	}

	// the audit log names the board and the message, never the text
	_, log := boss.op("audit_log", map[string]any{"action": "board_post"})
	text := fmt.Sprint(log)
	if !strings.Contains(text, "message 2 on personal board jib, a reply to 1") || strings.Contains(text, "on it") ||
		strings.Contains(text, "secret") {
		t.Fatalf("the audit log: %v", log)
	}
}

func TestNoBoardsUnconfigured(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")
	if code, mine := boss.op("my_boards", nil); code != 200 || len(mine["boards"].([]any)) != 0 {
		t.Fatalf("my_boards: %d %v", code, mine)
	}
	for op, args := range map[string]map[string]any{
		"board_messages":     {"board": "jib"},
		"post_board_message": {"board": "jib", "text": "hello"},
	} {
		if code, out := boss.op(op, args); code != 404 {
			t.Fatalf("%s with no boards: %d %v", op, code, out)
		}
	}
}

func TestABoardReadsSinceAnIDAndCountsWhatIsUnread(t *testing.T) {
	f := boardFixture(t)
	for i := 1; i <= 5; i++ {
		dabPosts(t, f, fmt.Sprintf("message %d", i), 0)
	}
	jib := f.browser()
	jib.login("jibrish")
	// the newest two, oldest first; three still unread
	code, th := jib.op("board_messages", map[string]any{"board": "jib", "limit": 2, "peek": true})
	if code != 200 || len(th["messages"].([]any)) != 2 || th["messages"].([]any)[0].(map[string]any)["id"] != float64(4) ||
		th["unread"] != float64(5) || th["total"] != float64(5) {
		t.Fatalf("newest two, peeked: %d %v", code, th)
	}
	// after id 1, two at a time: 2 and 3, with two more
	code, th = jib.op("board_messages", map[string]any{"board": "jib", "since_id": 1, "limit": 2})
	ms := th["messages"].([]any)
	if code != 200 || len(ms) != 2 || ms[0].(map[string]any)["id"] != float64(2) || th["more"] != float64(2) ||
		th["unread"] != float64(2) || th["read_to"] != float64(3) {
		t.Fatalf("since 1: %d %v", code, th)
	}
	if code, out := jib.op("board_messages", map[string]any{"board": "jib", "sinse_id": 3}); code != 400 ||
		!strings.Contains(fmt.Sprint(out), "since_id") {
		t.Fatalf("a misspelt argument: %d %v", code, out)
	}
	// a reply to another board's message, an empty and an oversized one are refused
	for _, args := range []map[string]any{
		{"board": "jib", "text": "x", "reply_to": 999},
		{"board": "jib", "text": "   "},
		{"board": "jib", "text": strings.Repeat("a", database.BoardMaxText+1)},
	} {
		if code, _ := jib.op("post_board_message", args); code != 400 {
			t.Errorf("post %v: %d", args["reply_to"], code)
		}
	}
}

func TestTheGenericTableOpsCannotReadABoard(t *testing.T) {
	f := boardFixture(t)
	dabPosts(t, f, "the secret word is swordfish", 0)
	for _, who := range []string{"gm", "boss"} {
		c := f.browser()
		c.login(who)
		_, tables := c.op("list_tables", nil)
		if strings.Contains(fmt.Sprint(tables), "gmboard") {
			t.Fatalf("%s: list_tables names the board: %v", who, tables)
		}
		for op, args := range map[string]map[string]any{
			"describe_table": {"table": "gmboard"},
			"query_rows":     {"table": "gmboard"},
			"count_rows":     {"table": "gmboard"},
			"get_row":        {"table": "gmboard", "key": map[string]any{"id": 1}},
			"update_row":     {"table": "gmboard", "key": map[string]any{"id": 1}, "values": map[string]any{"text": "x"}},
			"insert_row":     {"table": "gmboard", "values": map[string]any{"board": "jib", "text": "x"}},
			"delete_row":     {"table": "gmboard", "key": map[string]any{"id": 1}},
			"query_rows ":    {"table": "gmboard_read"},
		} {
			code, out := c.op(strings.TrimSpace(op), args)
			if code == 200 || strings.Contains(fmt.Sprint(out), "swordfish") {
				t.Errorf("%s %s: %d %v", who, op, code, out)
			}
		}
	}
	// and the message is as it was
	var text string
	f.db.QueryRow(`SELECT text FROM gmboard WHERE id = 1`).Scan(&text)
	if text != "the secret word is swordfish" {
		t.Fatalf("the message now: %q", text)
	}
}
