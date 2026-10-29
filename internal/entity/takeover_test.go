package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// takeoverFixture is a character in the world in one session, mid-fight and
// buffed, and a second session of the same account about to take it.
type takeoverFixture struct {
	w        *World
	sm       *session.Manager
	told     []*session.Session
	old, new *session.Session
	p        *Player
	npc      *NPC
	pet      *NPC
	mate     *Player
	mateSess *session.Session
	effect   *ActiveEffect
	now      time.Time
}

func newTakeoverFixture(t *testing.T) *takeoverFixture {
	t.Helper()
	f := &takeoverFixture{w: NewWorld(), sm: session.NewManager(), now: time.Now()}
	f.w.SessionManager = f.sm
	f.sm.SetOnReplaced(func(s *session.Session) { f.told = append(f.told, s) })
	window := func(port int, inst uint32) *session.Session {
		s := f.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: port}, inst, inst, uint16(inst), 2, true)
		s.SetAccountID(7)
		s.SetInGame(true)
		s.UpdateActivity()
		return s
	}
	f.old = window(6301, 31)
	f.p = f.w.CreatePlayer(f.old, &database.CharacterRecord{ServerID: 61, CharName: "Jibcro", AccountID: 7, Level: 60,
		Race: 3, TClass: 11, X: 25375, Y: 54.1, Z: 15721})
	f.old.SetCharacterInWorld(true)
	f.mateSess = window(6302, 32)
	f.mate = f.w.CreatePlayer(f.mateSess, &database.CharacterRecord{ServerID: 44, CharName: "Jibrish", AccountID: 7, Level: 60,
		Race: 3, TClass: 12, X: 25380, Y: 54.1, Z: 15721})
	f.mateSess.SetCharacterInWorld(true)

	// what it stands in: hurt, spent, buffed and locked out, a creature
	// fighting it, a group, a pet, a GM's freeze, half its breath
	p := f.p
	p.HP, p.Power = 37, 12
	f.effect = &ActiveEffect{SpellID: 900, Name: "Strength of Earth", Icon: 0x1234, Until: f.now.Add(90 * time.Second),
		NextTick: f.now.Add(3 * time.Second), HPTick: -5}
	p.Effects = append(p.Effects, f.effect)
	p.Buffs = append(p.Buffs, BuffSlot{Icon: 0x1234, Name: "Strength of Earth"})
	p.Lockouts = append(p.Lockouts, castLockout{spellID: 900, displayIndex: 4, until: f.now.Add(40 * time.Second)})
	f.npc = &NPC{ServerID: 5001, Name: "a gnoll pup", HP: 100, MaxHP: 100, Level: 10,
		Threats: map[EntityID]int64{p.ServerID: 500}, CurrentEnemy: p.ServerID, X: 25376, Z: 15722}
	f.w.NPCs[f.npc.ServerID] = f.npc
	f.w.groups = map[uint32]*Group{4: {ID: 4, Members: []EntityID{f.mate.ServerID, p.ServerID},
		lastRoster: map[EntityID]string{p.ServerID: "sent", f.mate.ServerID: "sent"}}}
	p.GroupID, f.mate.GroupID = 4, 4
	f.pet = &NPC{ServerID: 5002, Name: "Jibcro's skeleton", HP: 50, MaxHP: 50, Owner: p.ServerID}
	f.w.NPCs[f.pet.ServerID] = f.pet
	f.w.pets = map[EntityID]*petState{p.ServerID: {owner: p, npc: f.pet, told: 0x7E00001}}
	p.Frozen = true
	p.Breath = 128
	// and what it held of the client that dropped
	p.TargetObjectID = 0x186A1
	p.AutoAttack = true
	p.AttackTarget, p.AttackObjectID = f.npc.ServerID, 0x186A1
	p.PowerBars, p.powerBarsSent = true, "x"
	p.ProfanityFilterOff = true
	p.PendingCoachDests = []CoachDest{{Name: "Highpass"}}
	f.w.announced = map[*Player]bool{p: true, f.mate: true}

	f.new = window(6303, 33)
	return f
}

// A character still in the world taken over by another session of its
// account is the SAME Player, handed over: its hit points and power, its
// effect with its own end and next tick, its lockout, the creature fighting it
// and its threat, its group, its pet, a GM's freeze and its breath all as
// they were; the old session let go of it and told it logged in elsewhere,
// its end taking nothing out of the world; what it held of its old client
// (target, attack button, power bars, the filter, an open menu) gone. The
// control is the login it replaces: a Player built anew from the row, as the
// select did once the old session went quiet, has full hit points, none of
// that state, and the pet does not follow it.
func TestTakeOverHandsTheLivePlayer(t *testing.T) {
	f := newTakeoverFixture(t)
	p := f.p

	got, took := f.w.TakeOverPlayer(f.new, 61)
	if got != p {
		t.Fatalf("taken over: %p, want the live player %p", got, p)
	}
	if f.w.Players[p.ServerID] != p || f.w.SessionPlayer(f.new) != p || f.new.CharacterServerID() != 61 {
		t.Fatal("the new session does not have the live player")
	}
	if f.w.SessionPlayer(f.old) != nil {
		t.Fatal("the old session still has it")
	}
	if _, r := f.old.ReplacedFor(); !r || len(f.told) != 1 || f.told[0] != f.old {
		t.Fatalf("the old session was not told it logged in elsewhere (told %d)", len(f.told))
	}
	// as it stood
	if p.HP != 37 || p.Power != 12 || p.Breath != 128 || !p.Frozen {
		t.Fatalf("HP %d power %d breath %d frozen %v: not as it stood", p.HP, p.Power, p.Breath, p.Frozen)
	}
	if len(p.Effects) != 1 || p.Effects[0] != f.effect || !f.effect.Until.Equal(f.now.Add(90*time.Second)) ||
		!f.effect.NextTick.Equal(f.now.Add(3*time.Second)) || len(p.Buffs) != 1 {
		t.Fatalf("its effect: %+v", p.Effects)
	}
	if len(p.Lockouts) != 1 || !p.Lockouts[0].until.Equal(f.now.Add(40*time.Second)) {
		t.Fatalf("its lockouts: %+v", p.Lockouts)
	}
	if f.npc.Threats[p.ServerID] != 500 || f.npc.CurrentEnemy != p.ServerID {
		t.Fatalf("the creature fighting it: threat %d, enemy %d", f.npc.Threats[p.ServerID], f.npc.CurrentEnemy)
	}
	if got := f.w.GroupMembers(p); p.GroupID != 4 || len(got) != 2 || got[1] != p {
		t.Fatalf("its group: %d, %v", p.GroupID, got)
	}
	f.w.mu.RLock()
	pet := f.w.petOfLocked(p)
	f.w.mu.RUnlock()
	if pet != f.pet || f.w.pets[p.ServerID].told != 0 {
		t.Fatalf("its pet: %v (told %#x: the new client is told it again)", pet, f.w.pets[p.ServerID].told)
	}
	// what it held of the old client
	if p.TargetObjectID != 0 || p.AutoAttack || p.AttackTarget != 0 || p.AttackObjectID != 0 || p.PowerBars ||
		p.powerBarsSent != "" || p.ProfanityFilterOff || p.PendingCoachDests != nil {
		t.Fatalf("the old client's state kept: target %#x auto %v attack %d bars %v filter off %v menu %v",
			p.TargetObjectID, p.AutoAttack, p.AttackTarget, p.PowerBars, p.ProfanityFilterOff, p.PendingCoachDests)
	}
	if f.w.announced[p] || !f.w.announced[f.mate] {
		t.Fatal("the new client is not told its guild, or another player's announcement went")
	}
	if _, kept := f.w.groups[4].lastRoster[p.ServerID]; kept {
		t.Fatal("the new client is not sent its group's roster")
	}
	if _, kept := f.w.groups[4].lastRoster[f.mate.ServerID]; !kept {
		t.Fatal("the group mate's roster was reset")
	}
	if !p.StatsDirty {
		t.Fatal("the stat channel does not go out at once")
	}
	if took.HP != 37 || len(took.Effects) != 1 || took.Lockouts != 1 || took.Fighting != 1 || took.Group != 4 ||
		took.Pet != f.pet.Name || took.From != 1 || took.Told != 1 {
		t.Fatalf("the takeover's account: %+v", took)
	}
	// the old session's end, after its grace, takes nothing out of the world
	f.w.RemovePlayerBySession(f.old)
	if f.w.Players[p.ServerID] != p {
		t.Fatal("the old session's end took the character out of the world")
	}
	// and the other window of the account plays on
	if _, r := f.mateSess.ReplacedFor(); r || f.mateSess.PendingTermination() || f.w.SessionPlayer(f.mateSess) != f.mate {
		t.Fatal("the account's other window lost its character")
	}

	// the control: the login the takeover replaces
	c := newTakeoverFixture(t)
	fresh := c.w.CreatePlayer(c.new, &database.CharacterRecord{ServerID: 61, CharName: "Jibcro", AccountID: 7, Level: 60,
		Race: 3, TClass: 11, X: 25375, Y: 54.1, Z: 15721})
	c.w.mu.RLock()
	freshPet := c.w.petOfLocked(fresh)
	c.w.mu.RUnlock()
	if fresh == c.p || fresh.HP == 37 || len(fresh.Effects) != 0 || len(fresh.Lockouts) != 0 || fresh.Frozen || freshPet != nil {
		t.Fatalf("the control: a login built from the row kept the live state (HP %d, %d effects, %d lockouts, frozen %v, pet %v)",
			fresh.HP, len(fresh.Effects), len(fresh.Lockouts), fresh.Frozen, freshPet)
	}
}

// A character whose session is gone altogether (a client's reset took the
// session and left its Player in the world) is taken over too, from no one;
// one with no Player in the world is not taken over (the login builds it).
func TestTakeOverAGhostAndNothing(t *testing.T) {
	f := newTakeoverFixture(t)
	f.sm.RemoveSession(f.old)
	got, took := f.w.TakeOverPlayer(f.new, 61)
	if got != f.p || took.From != 0 || took.Told != 0 || f.w.SessionPlayer(f.new) != f.p {
		t.Fatalf("a character its session lost: %v, %+v", got, took)
	}
	if got, _ := f.w.TakeOverPlayer(f.new, 62); got != nil {
		t.Fatal("a character with no player in the world was taken over")
	}
	if got, _ := f.w.TakeOverPlayer(nil, 61); got != nil {
		t.Fatal("no session took a character")
	}
}

// The new client is told each lockout still running with what is left of it
// (0x00CD), the ability by its place in the book; one over is not sent.
func TestSendTakenOverTellsTheLockouts(t *testing.T) {
	f := newTakeoverFixture(t)
	f.p.Lockouts = append(f.p.Lockouts, castLockout{spellID: 901, displayIndex: 6, until: f.now.Add(-time.Second)})
	f.w.TakeOverPlayer(f.new, 61)
	if n := f.w.SendTakenOver(f.new, f.now); n != 1 {
		t.Fatalf("%d lockouts told, want 1", n)
	}
	var sent [][]byte
	for _, m := range f.new.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeCooldownTimer {
			sent = append(sent, d)
		}
	}
	if len(sent) != 1 || sent[0][2] != 4 {
		t.Fatalf("0x00CD sent: %x", sent)
	}
	r := &vims{b: sent[0][7:]}
	if ms := r.signed(); ms != 40000 {
		t.Fatalf("the lockout left: %d ms, want 40000", ms)
	}
	// the control: the old session, which has no player now, is told nothing
	if n := f.w.SendTakenOver(f.old, f.now); n != 0 {
		t.Fatalf("the old session was told %d lockouts", n)
	}
}

// A character that died and dropped is taken over dead: no new client may
// skip the death (the old login built it anew from its row, alive and full).
// Its respawn comes on the death's own time, to the session that has it now;
// the old session gets none.
func TestADeadCharacterIsTakenOverDead(t *testing.T) {
	f := newTakeoverFixture(t)
	died := f.now.Add(-PlayerRespawnDelay + 2*time.Second)
	f.w.mu.Lock()
	f.w.playerDiedLocked(nil, f.p, died)
	f.w.mu.Unlock()
	var to []*session.Session
	f.w.OnPlayerRespawn(func(s *session.Session, p *Player) { to = append(to, s) })

	got, took := f.w.TakeOverPlayer(f.new, 61)
	if got != f.p || !took.Dead || got.HP != 0 || !got.DiedAt.Equal(died) {
		t.Fatalf("taken over: dead %v, HP %d, died at %v (want %v)", took.Dead, got.HP, got.DiedAt, died)
	}
	f.new.SetCharacterInWorld(true) // its client in the world (0x0014)
	f.w.Tick(f.now.Add(time.Second))
	if len(to) != 0 || !f.p.respawnPending() {
		t.Fatal("the respawn came before its time")
	}
	f.w.Tick(f.now.Add(3 * time.Second))
	if len(to) != 1 || to[0] != f.new || f.p.HP != f.p.MaxHP {
		t.Fatalf("the respawn went to %v (the new session %p), HP %d/%d", to, f.new, f.p.HP, f.p.MaxHP)
	}
}
