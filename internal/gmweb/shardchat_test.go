package gmweb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/shardchat"
)

func TestShardChatPanelConnection(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(key, []byte(testShardKey), 0600); err != nil {
		t.Fatal(err)
	}
	panel := func(cfg config.GMConfig) *Server {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		b := shardchat.New(shardchat.FromConfig(cfg.ShardID), func(m shardchat.Message) shardchat.Receipt {
			return shardchat.Receipt{Name: m.To, Away: cfg.ShardID + " " + m.Text}
		})
		s := &Server{cfg: cfg, d: Deps{Chat: b}, chatCtx: ctx, stopChat: cancel}
		if err := s.initShards(); err != nil {
			t.Fatal(err)
		}
		return s
	}
	live := panel(config.GMConfig{ShardID: "live", ShardKeyFile: key})
	var active atomic.Value
	active.Store(live.Handler())
	httpLive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active.Load().(http.Handler).ServeHTTP(w, r)
	}))
	t.Cleanup(httpLive.Close)
	box := panel(config.GMConfig{ShardID: "heirloom", OtherShards: []config.GMShardPanel{{Name: "live", URL: httpLive.URL, KeyFile: key}}})
	box.startShardChat()
	t.Cleanup(box.stopChat)

	send := func(s *Server, dest, body string) error {
		m := shardchat.Message{Channel: "tell", From: "Sender", To: "Recipient", Text: body}
		r, err := s.d.Chat.Send(m)
		if err == nil && (r.Name != m.To || r.Away != dest+" "+body) {
			return fmt.Errorf("receipt %+v for %q", r, body)
		}
		return err
	}
	ready := func(s *Server, dest string) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for {
			if err := send(s, dest, "ready"); err == nil {
				return
			} else if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	ready(box, "live")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s, dest := range map[*Server]string{box: "live", live: "heirloom"} {
				if err := send(s, dest, fmt.Sprint(i)); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()

	// Restart the accepting shard. It has no outbound peer configuration.
	replacement := panel(live.cfg)
	active.Store(replacement.Handler())
	live.stopChat()
	ready(replacement, "heirloom")
	ready(box, "live")
}

func TestShardChatAcceptsOnlyTheLocalPeer(t *testing.T) {
	s := &Server{d: Deps{Chat: shardchat.New(shardchat.Vanilla, nil)}, shardKey: testShardKey}
	for _, tc := range []struct{ name, address, key, proxy, origin string }{
		{"no credential", "127.0.0.1:1234", "", "", ""},
		{"GM credential", "127.0.0.1:1234", testShardKey, "", ""},
		{"proxy", "127.0.0.1:1234", chatCredential(testShardKey), "192.0.2.1", ""},
		{"nonlocal", "192.0.2.1:1234", chatCredential(testShardKey), "", ""},
		{"browser", "127.0.0.1:1234", chatCredential(testShardKey), "", "http://localhost"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, shardChatPath, nil)
			r.RemoteAddr = tc.address
			r.Header.Set(chatKeyHeader, tc.key)
			r.Header.Set("X-Forwarded-For", tc.proxy)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			s.handleShardChat(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
}
