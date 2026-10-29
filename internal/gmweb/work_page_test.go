package gmweb

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"
)

// What agents are told of the work board (the MCP instructions, the guide,
// the tool list and a custom GPT's short list), and the page.

func TestAgentsAreToldOfTheBoard(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	agent := f.bearer(gm.makeToken("agent", "write"))
	instr := agent.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18"})["result"].(map[string]any)["instructions"].(string)
	for _, want := range []string{"work_board", "post_work", "without asking", "work board"} {
		if !strings.Contains(instr, want) {
			t.Errorf("the MCP instructions do not say %q: %s", want, instr)
		}
	}
	names := map[string]bool{}
	for _, tl := range agent.rpc("tools/list", nil)["result"].(map[string]any)["tools"].([]any) {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	for _, n := range []string{"work_board", "post_work", "update_work_overview"} {
		if !names[n] {
			t.Errorf("MCP does not list %s", n)
		}
	}
	// a custom GPT's short list has the board
	resp, _ := http.Get(f.ts.URL + "/api/openapi.json?core=1")
	var doc map[string]any
	json.NewDecoder(resp.Body).Decode(&doc)
	resp.Body.Close()
	paths := doc["paths"].(map[string]any)
	if paths["/api/op/work_board"] == nil || paths["/api/op/post_work"] == nil || len(paths) > 30 {
		t.Fatalf("the short list: %d operations, work_board %v", len(paths), paths["/api/op/work_board"] != nil)
	}
	// the guide says how each agent connects, at this panel's own address
	resp, _ = http.Get(f.ts.URL + "/api/guide")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	guide := string(body)
	for _, want := range []string{"## Before you change anything: the work board", f.ts.URL + "/mcp", "codex mcp add",
		"openapi.json", "post_work"} {
		if !strings.Contains(guide, want) {
			t.Errorf("the guide does not say %q", want)
		}
	}
	if strings.Contains(guide, "{{PANEL}}") {
		t.Error("the guide's address was not filled in")
	}
	code, out := gm.op("guide", nil)
	if code != 200 || !strings.Contains(fmt.Sprint(out["guide"]), f.ts.URL+"/mcp") {
		t.Fatalf("the guide op: %d", code)
	}
}

// The page: in the navigation, its files served, and what people wrote put
// in as text (the markdown is built as elements, never parsed as HTML).
func TestTheWorkBoardPageIsThere(t *testing.T) {
	f := newFixture(t)
	app, err := fs.ReadFile(staticFS, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(app), "['work', 'Work board'") || !strings.Contains(string(app), "work: () => import('./views/work.js')") {
		t.Fatal("app.js has no Work board page")
	}
	for _, file := range []string{"static/views/work.js", "static/lib/md.js"} {
		src, err := fs.ReadFile(staticFS, file)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval("} {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s uses %s", file, bad)
			}
		}
		resp, err := http.Get(f.ts.URL + "/" + file)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
			t.Errorf("%s: %d %s", file, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
}
