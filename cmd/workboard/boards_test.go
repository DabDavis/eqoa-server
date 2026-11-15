package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

func TestTheBoxPostsAndReadsAPersonalBoard(t *testing.T) {
	db := board(t)
	cfg := filepath.Join(filepath.Dir(db), "config.yaml")
	os.WriteFile(cfg, []byte("server:\n  db_path: eqoa.db\ngm:\n  personal_boards:\n    jib: [\"jibrish\"]\n"), 0o644)

	if code, out, errs := wb(t, "", "-config", cfg, "post", "jib", "the", "gate", "is", "**moved**"); code != 0 ||
		!strings.Contains(out, "posted #1 to board jib as dab's sessions") {
		t.Fatalf("post: %d %q %q", code, out, errs)
	}
	// a board config does not have is refused, naming the ones it has
	if code, _, errs := wb(t, "", "-config", cfg, "post", "jibs", "hello"); code != exitUsage || !strings.Contains(errs, "it has jib") {
		t.Fatalf("a typo's board: %d %q", code, errs)
	}
	// jib's agent answers through the panel's call
	sq, err := database.OpenWorkBoard(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sq.PostBoardMessage(database.BoardPost{Board: "jib", Text: "thanks\nlooks right", ReplyTo: 1, Via: database.BoardViaPanel},
		database.GMEdit{AccountID: 10, Username: "jibrish", Via: "oauth:Claude"}); err != nil {
		t.Fatal(err)
	}
	sq.Close()

	if code, out, _ := wb(t, "", "-config", cfg, "boards"); code != 0 ||
		!strings.Contains(out, "jib: 2 messages, 1 unread to dab's sessions · members jibrish") {
		t.Fatalf("boards: %d %q", code, out)
	}
	code, out, _ := wb(t, "", "-config", cfg, "board", "jib")
	if code != 0 || !strings.Contains(out, "#2 NEW jibrish (oauth:Claude) (panel)") || !strings.Contains(out, "reply to #1\n  thanks\n  looks right") ||
		strings.Contains(out, "#1 NEW") {
		t.Fatalf("board:\n%s", out)
	}
	// read now: nothing unread, and -since shows what came after
	if code, out, _ := wb(t, "", "-config", cfg, "board", "jib", "-since", "1"); code != 0 || strings.Contains(out, "NEW") ||
		strings.Contains(out, "#1 ") || !strings.Contains(out, "#2 ") {
		t.Fatalf("board -since 1:\n%s", out)
	}
	// stdin, and a reply
	if code, out, errs := wb(t, "line one\nline two\n", "-config", cfg, "post", "jib", "-reply", "2", "-file", "-"); code != 0 ||
		!strings.Contains(out, "posted #3") {
		t.Fatalf("post from stdin: %d %q %q", code, out, errs)
	}
	if code, _, errs := wb(t, "", "-config", cfg, "post", "jib", "-reply", "99", "x"); code != exitUsage || !strings.Contains(errs, "reply_to 99") {
		t.Fatalf("a reply to nothing: %d %q", code, errs)
	}
	// post -title is still add's old name
	if code, out, errs := wb(t, "", "-config", cfg, "post", "-title", "Move the guards"); code != 0 || !strings.Contains(out, "posted #1: Move the guards") {
		t.Fatalf("post -title: %d %q %q", code, out, errs)
	}
}
