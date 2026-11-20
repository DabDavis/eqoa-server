package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// modesProcess is one process over db as a GM's (account 3) restart sees it:
// the character handler a resume goes through, the chat handler its commands
// and its GM channel go through, the stores both keep the modes in.
func modesProcess(t *testing.T, db *database.SQLiteDB) (*chatWorld, *CharacterHandler) {
	t.Helper()
	h, gw, sm := keepProcess(t, db)
	c := &chatWorld{t: t, sm: sm, gw: gw, h: NewChatHandler(sm)}
	c.h.SetGameWorld(gw)
	c.h.SetAccessLevels(levels{3: 1})
	c.h.SetSafeModeStore(db)
	c.h.SetGMTagHiddenStore(db)
	c.h.SetGMNoAggroStore(db)
	h.gmChannel = c.h.SendGMChannel
	prev := keepChat.Swap(c.h)
	t.Cleanup(func() { keepChat.Store(prev) })
	return c, h
}

// modesSession is the session of account 3's Hero, in the world.
func modesSession(t *testing.T, sm *session.Manager) *session.Session {
	t.Helper()
	k := session.Kept{Addr: "203.0.113.7:5007", InstanceID: 7, SessionID: 500000007, ClientEndpoint: 7, ServerEndpoint: 1,
		State: session.StateInGame, HasInstance: true, DidServerInitiate: true, CharacterInWorld: true, InGame: true,
		AccountID: 3, CharServerID: 7, PlayerObjectID: 500000007}
	s, err := sm.Resume(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The owner, 2026-11-16: "also keep gm safemode on across restarts". A GM in
// the TEST menu's Safe Mode (#aggro off), with /testinvis and every other mode
// a GM can have on (Toggle Invis, Change Sprite, Freeze, Silence, #testmode,
// #tankprofile, #speed, #size), through a restart that keeps the players: the
// new process has every one on, and tells the player nothing of any (its
// client never saw them go). Before the fix only /testinvis came through.
func TestTheModesComeThroughAKeepRestart(t *testing.T) {
	SetTestInvis(true)
	t.Cleanup(func() { SetTestInvis(false) })
	db := keepDB(t)

	// the old process: in the world, every mode turned on
	c, _ := modesProcess(t, db)
	s := modesSession(t, c.sm)
	p, err := ResumeCharacter(s, KeptExtras{}, entity.KeptPlayer{CharServerID: 7, Name: "Hero"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c.h.SendGMChannel(s)
	for _, cmd := range []string{"/testinvis", "#testmode", "#tankprofile2", "#speed 2", "#size 2"} {
		c.say(s, cmd)
	}
	c.h.gmCommand(s, p, gmRequest{Sub: gmSubSafeMode})
	c.h.gmCommand(s, p, gmRequest{Sub: gmSubInvis})
	c.h.gameWorld.SetGMForm(p.ServerID, 0xA99C6B15, 1.5)
	c.h.gmFreeze(s, p, "Hero")
	c.h.gmSilence(s, p, "Hero")
	speed, size := p.Speed, p.Size
	if on, _ := c.gw.SafeMode(s); !on {
		t.Fatal("/testinvis did not turn safe mode on")
	}
	if ign, _, _ := c.gw.AggroIgnored(s); !ign || !p.GMTestMode || p.GMTankProfile != 2 || !p.GMHidden || !p.Frozen ||
		p.GMForm != 0xA99C6B15 || speed == p.GMSpeedBase() || !c.h.silencedChar(7) {
		t.Fatalf("the modes were not all set: no aggro %v, test %v, tank %d, hidden %v, frozen %v, form %#x, speed %v, silenced %v",
			ign, p.GMTestMode, p.GMTankProfile, p.GMHidden, p.Frozen, p.GMForm, speed, c.h.silencedChar(7))
	}
	kp, ok := c.gw.KeepPlayer(s, time.Now())
	if !ok {
		t.Fatal("nothing kept")
	}
	ex := KeepExtras(s, p)

	// the new process: the same row, a World that knows nothing
	c2, _ := modesProcess(t, db)
	s2 := modesSession(t, c2.sm)
	p2, err := ResumeCharacter(s2, ex, kp, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// on before its dump: a creature it passes before its client reports
	// must leave it alone already
	if ign, _, _ := c2.gw.AggroIgnored(s2); !ign {
		t.Error("the GM's Safe Mode (#aggro off) was lost")
	}
	if !ResumeDump(s2, false) {
		t.Fatal("no dump")
	}
	if on, _ := c2.gw.SafeMode(s2); !on {
		t.Error("/testinvis was lost")
	}
	if ign, _, _ := c2.gw.AggroIgnored(s2); !ign {
		t.Error("the GM's Safe Mode (#aggro off) was lost at the dump")
	}
	if !p2.GMTestMode {
		t.Error("#testmode was lost")
	}
	if p2.GMTankProfile != 2 {
		t.Errorf("#tankprofile %d, want 2", p2.GMTankProfile)
	}
	if !p2.GMHidden {
		t.Error("Toggle Invis was lost")
	}
	if p2.GMForm != 0xA99C6B15 || p2.GMFormSize != 1.5 {
		t.Errorf("Change Sprite: %#x at %v", p2.GMForm, p2.GMFormSize)
	}
	if !p2.Frozen || p2.SpeedNow() != 0 {
		t.Errorf("Freeze was lost: frozen %v, speed %v", p2.Frozen, p2.SpeedNow())
	}
	if p2.Speed != speed {
		t.Errorf("#speed: %v, it ran at %v", p2.Speed, speed)
	}
	if p2.Size != size {
		t.Errorf("#size: %v, it stood %v", p2.Size, size)
	}
	if !c2.h.silencedChar(7) {
		t.Error("Silence was lost")
	}
	// and the player is told nothing new of any of them
	for _, l := range heard(t, s2) {
		for _, said := range []string{safeLoginLine, safeOnLine, "Safe Mode", "Creatures", "Test mode", "Tank profile", "invisible", "frozen", "silenced"} {
			if strings.Contains(l.text, said) {
				t.Errorf("told %q at the resume", l.text)
			}
		}
	}

	// #speed 1 and #size 1 still give its own back
	c2.say(s2, "#speed 1")
	if p2.Speed != p2.GMSpeedBase() || p2.Speed == speed {
		t.Errorf("#speed 1 after the resume: %v", p2.Speed)
	}
}

// A GM's Safe Mode outlives a restart that drops the players too (a stop and
// a start) and a logout, as /testinvis does: kept in table chargmnoaggro and
// given back at its world entry (SendGMChannel). Off, the row goes, and the
// next login is off. An account no longer a GM's is given nothing (its row
// waits, harmless, until it is a GM's again or it is turned off).
func TestAGMsSafeModeOutlivesALogin(t *testing.T) {
	db := keepDB(t)
	c, _ := modesProcess(t, db)
	s := modesSession(t, c.sm)
	if _, err := ResumeCharacter(s, KeptExtras{}, entity.KeptPlayer{CharServerID: 7, Name: "Hero"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	c.say(s, "#aggro off")
	if on, err := db.GMNoAggro(7); err != nil || !on {
		t.Fatalf("#aggro off not kept: %v %v", on, err)
	}

	// a login in a new process (no keep file)
	login := func() (*chatWorld, *session.Session) {
		c2, h2 := modesProcess(t, db)
		s2 := modesSession(t, c2.sm)
		rec, err := h2.store.GetByServerID(7)
		if err != nil {
			t.Fatal(err)
		}
		if h2.enterFromRecord(s2, rec) == nil {
			t.Fatal("no player")
		}
		return c2, s2
	}
	c2, s2 := login()
	c2.h.SendGMChannel(s2)
	if ign, _, _ := c2.gw.AggroIgnored(s2); !ign {
		t.Fatal("#aggro off was lost over a restart and a login")
	}
	// no longer a GM: nothing
	c3, s3 := login()
	c3.h.SetAccessLevels(levels{3: 0})
	c3.h.SendGMChannel(s3)
	if ign, _, _ := c3.gw.AggroIgnored(s3); ign {
		t.Fatal("an account no GM's kept the GM's Safe Mode")
	}
	// turned off: the row goes, the next login is off
	c2.say(s2, "#aggro on")
	if on, _ := db.GMNoAggro(7); on {
		t.Fatal("#aggro on left the row")
	}
	c4, s4 := login()
	c4.h.SendGMChannel(s4)
	if ign, _, _ := c4.gw.AggroIgnored(s4); ign {
		t.Fatal("#aggro on did not hold over a login")
	}
}

// The owner, 2026-11-20: "make /safemode for GMs same as the ingame GM menu
// works also for sub GMs". A sub GM's /safemode is the TEST menu's Safe Mode,
// kept in its row as a GM's: a world entry after it (SendGMChannel, a login's)
// has it on again; /safemode again turns it off, and the next entry leaves it off.
func TestASubGMsSafeModeIsKept(t *testing.T) {
	db := keepDB(t)
	c, _ := modesProcess(t, db)
	c.h.SetAccessLevels(subs{levels{3: 0}, map[int]bool{3: true}})
	s := modesSession(t, c.sm)
	if _, err := ResumeCharacter(s, KeptExtras{}, entity.KeptPlayer{CharServerID: 7, Name: "Hero"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	c.say(s, "/safemode")
	if ign, _, _ := c.gw.AggroIgnored(s); !ign {
		t.Fatal("the sub GM's /safemode did not turn Safe Mode on")
	}
	c.gw.SetAggroIgnored(s, false) // a new login's world knows nothing of it
	c.h.SendGMChannel(s)
	if ign, _, _ := c.gw.AggroIgnored(s); !ign {
		t.Fatal("the sub GM's world entry did not put its Safe Mode back")
	}
	c.say(s, "/safemode")
	c.h.SendGMChannel(s)
	if ign, _, _ := c.gw.AggroIgnored(s); ign {
		t.Fatal("Safe Mode turned off came back at the next entry")
	}
}
