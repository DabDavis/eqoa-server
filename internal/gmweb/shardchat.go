package gmweb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/eqoa/server/internal/shardchat"
)

const shardChatPath = "/api/shard/chat/v1"
const chatKeyHeader = "X-EQOA-Chat-Key"

// A chat credential cannot authorize a GM operation.
func chatCredential(key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(shardChatPath))
	return hex.EncodeToString(mac.Sum(nil))
}

// Heirloom already knows Live's loopback panel and key. One connection over
// that route carries player chat both ways, including replies after a restart.
func (s *Server) startShardChat() {
	if s.d.Chat == nil || s.d.Chat.Home != shardchat.Sandbox {
		return
	}
	for _, p := range s.peers {
		if shardchat.FromConfig(p.id) != shardchat.Vanilla || p.key == "" {
			continue
		}
		u, err := url.Parse(p.url)
		if err != nil || !(strings.EqualFold(u.Hostname(), "localhost") || net.ParseIP(u.Hostname()).IsLoopback()) {
			log.Print("Shard chat: the peer panel must have a loopback URL")
			return
		}
		go s.connectShardChat(p)
		return
	}
}

func (s *Server) connectShardChat(p *shardPeer) {
	opts := &websocket.DialOptions{
		HTTPClient: shardClient, // direct, no proxy or redirects
		HTTPHeader: http.Header{chatKeyHeader: {chatCredential(p.key)}},
	}
	ctx := s.chatCtx
	reported := false
	for ctx.Err() == nil {
		dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		c, _, err := websocket.Dial(dialCtx, p.url+shardChatPath, opts)
		cancel()
		if err == nil {
			log.Print("Shard chat: connected to Vanilla")
			err = s.d.Chat.Serve(ctx, c)
			reported = false
		}
		if !reported && ctx.Err() == nil {
			log.Printf("Shard chat: disconnected, reconnecting: %v", err)
			reported = true
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (s *Server) handleShardChat(w http.ResponseWriter, r *http.Request) {
	if s.d.Chat == nil || s.d.Chat.Home != shardchat.Vanilla || s.shardKey == "" {
		http.NotFound(w, r)
		return
	}
	if !handedFromHere(r) || r.Header.Get("Origin") != "" ||
		!hmac.Equal([]byte(r.Header.Get(chatKeyHeader)), []byte(chatCredential(s.shardKey))) {
		http.Error(w, "shard chat unavailable", http.StatusForbidden)
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err == nil {
		_ = s.d.Chat.Serve(s.chatCtx, c)
	}
}
