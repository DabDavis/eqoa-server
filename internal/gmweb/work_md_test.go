package gmweb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The work board's markdown (static/lib/md.js) in a real browser, when one
// is installed: what people write comes out as elements, a script or a
// javascript: link as text, and the board's own constructs (lists, task
// boxes, tables, code, links) as they read.
func TestTheBoardsMarkdownInABrowser(t *testing.T) {
	var browser string
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			browser = p
			break
		}
	}
	if browser == "" {
		t.Skip("no Chromium to render the page in")
	}
	f := newFixture(t)
	const sample = "## Conventions\n\n- [x] measure the gate\n- [ ] the paths\n  - east first\n\n" +
		"| Guard | Path |\n|---|---:|\n| Dahvel | east |\n\n" +
		"**bold** and _em_ and `work_board` and snake_case_word; see https://example.org/x. and [the board](#/work/3)\n\n" +
		"<script>window.pwned = 1</script> [bad](javascript:alert(1)) <img src=x onerror=alert(1)>\n\n" +
		"```\ncode <b>kept</b>\n```\n\n> a quote\n\n1. one\n2. two\n"
	// the sample as a JSON string, its < escaped: it holds a </script> of its own
	quoted, _ := json.Marshal(sample)
	harness := `<!doctype html><meta charset="utf-8"><div id="out"></div><script type="module">
import { markdown } from '/static/lib/md.js';
document.getElementById('out').appendChild(markdown(` + string(quoted) + `));
document.getElementById('out').dataset.done = String(!window.pwned);
</script>`
	mux := http.NewServeMux()
	mux.HandleFunc("/harness.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(harness))
	})
	mux.Handle("/static/", f.s.Handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser, "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--user-data-dir="+t.TempDir(), "--dump-dom", ts.URL+"/harness.html")
	// no session bus to look for, and its temporary files (it leaves some
	// behind) in a directory of their own, with a short path: its socket's
	// path must fit a unix socket's 108 bytes
	tmp, err := os.MkdirTemp("", "wbmd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS=disabled:", "TMPDIR="+tmp)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	dom := string(out)
	if err != nil || !strings.Contains(dom, `id="out"`) {
		t.Skipf("the browser did not load the page (%v): %.300s %.600s", err, out, stderr.String())
	}
	if !strings.Contains(dom, `data-done=`) {
		t.Fatalf("the page loaded and md.js did not render:\n%s", dom)
	}
	for _, want := range []string{
		`<h4>Conventions</h4>`,
		`<span class="md-task done" aria-label="done">☑ </span>measure the gate`,
		`<li>east first</li>`,
		`<table class="plain md-table">`, `<td style="text-align: right;">east</td>`,
		`<strong>bold</strong>`, `<em>em</em>`, `<code>work_board</code>`, `snake_case_word`,
		`<a href="https://example.org/x" target="_blank" rel="noopener noreferrer">https://example.org/x</a>.`,
		`<a href="#/work/3">the board</a>`,
		`&lt;script&gt;window.pwned = 1&lt;/script&gt;`, `[bad](javascript:alert(1))`, `&lt;img src=x onerror=alert(1)&gt;`,
		`<pre class="md-code"><code>code &lt;b&gt;kept&lt;/b&gt;</code></pre>`,
		`<blockquote><p>a quote</p></blockquote>`, `<ol><li>one</li><li>two</li></ol>`,
		`data-done="true"`,
	} {
		if !strings.Contains(dom, want) {
			t.Errorf("the rendered markdown has no %s", want)
		}
	}
	if strings.Contains(dom, `href="javascript:`) || strings.Contains(dom, "<script>window") || strings.Contains(dom, "<img") {
		t.Errorf("what was written became markup:\n%s", dom)
	}
}
