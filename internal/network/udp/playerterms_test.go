package udp

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/session"
)

func TestPlayerTermsRechecksKeptSessionsBeforeWorldEntry(t *testing.T) {
	sm := session.NewManager()
	l := NewListener(config.DefaultConfig(), sm, nil, nil)
	l.PlayerTermsAllowed = func(id int) bool { return id == 2 }
	snap := Snapshot{Sessions: []KeptSession{
		{Session: session.Kept{Addr: "127.0.0.1:9021", InstanceID: 1, AccountID: 1, InGame: true, CharacterInWorld: true}},
		{Session: session.Kept{Addr: "127.0.0.1:9022", InstanceID: 2, AccountID: 2}},
	}}
	resumed, told := l.ResumeKept(&snap, time.Now())
	if resumed != 1 || told != 1 || sm.ActiveCount() != 1 {
		t.Fatalf("resumed %d, refused %d, active %d", resumed, told, sm.ActiveCount())
	}
}
