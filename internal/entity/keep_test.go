package entity

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// keepRig is one process's World with a player in the world in a session,
// another player, an NPC of a row and a pet beside it.
type keepRig struct {
	w          *World
	sm         *session.Manager
	sess, mate *session.Session
	p, other   *Player
}

func newKeepRig(t *testing.T, npcName string, pet bool) *keepRig {
	t.Helper()
	r := &keepRig{w: NewWorld(), sm: session.NewManager()}
	r.w.SessionManager = r.sm
	open := func(port int, inst uint32, acct int) *session.Session {
		s := r.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: port}, inst, inst, uint16(inst), 2, true)
		s.SetAccountID(acct)
		s.SetInGame(true)
		s.SetCharacterInWorld(true)
		return s
	}
	r.sess, r.mate = open(6301, 31, 7), open(6302, 32, 8)
	r.sess.SetPlayerObjectID(500000031)
	r.mate.SetPlayerObjectID(500000032)
	r.p = r.w.CreatePlayer(r.sess, &database.CharacterRecord{ServerID: 61, CharName: "Jibcro", AccountID: 7, Level: 60,
		Race: 3, TClass: 11, X: 25375, Y: 54.1, Z: 15721})
	r.other = r.w.CreatePlayer(r.mate, &database.CharacterRecord{ServerID: 44, CharName: "Jibrish", AccountID: 8, Level: 60,
		Race: 3, TClass: 12, X: 25380, Y: 54.1, Z: 15721})
	r.w.NPCs[5001] = &NPC{ServerID: 5001, Name: npcName, HP: 100, MaxHP: 100, Level: 10, X: 25376, Z: 15722}
	if pet {
		r.w.NPCs[900001] = &NPC{ServerID: 900001, Name: "Jibcro's skeleton", HP: 50, MaxHP: 50, Temporary: true, X: 25377, Z: 15722}
	}
	return r
}

// see puts the entities on sess's slots as the visibility system does, their
// state in each channel.
func (r *keepRig) see(t *testing.T, sess *session.Session, ids ...EntityID) {
	t.Helper()
	vs := GetOrCreateVisibility(sess)
	conn := sess.Connection()
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for i, id := range ids {
		vs.nextObjectID++
		vs.ChannelEntity[i], vs.ChannelObjectID[i] = id, vs.nextObjectID
		ch := conn.GetServerObject(i + 1)
		SetEntityState(ch, r.w.serializeEntityForChannel(sess, id, vs.nextObjectID))
		_, seq, _ := ch.GenerateXORDelta()
		ch.UpdateBaseXor(seq)
	}
}

// through is a restart: the session written down (the connection's part and
// the World's), and a new World with the same rows loaded, the session and
// its character put back.
func through(t *testing.T, old *keepRig, npcName string) (*keepRig, *session.Session) {
	t.Helper()
	kv := old.w.KeepView(old.sess)
	b, err := json.Marshal(struct {
		S session.Kept
		V KeptView
	}{old.sess.Keep(), kv})
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		S session.Kept
		V KeptView
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	r := &keepRig{w: NewWorld(), sm: session.NewManager()}
	r.w.SessionManager = r.sm
	sess, err := r.sm.Resume(back.S)
	if err != nil {
		t.Fatal(err)
	}
	r.sess = sess
	r.p = r.w.CreatePlayer(sess, &database.CharacterRecord{ServerID: 61, CharName: "Jibcro", AccountID: 7, Level: 60,
		Race: 3, TClass: 11, X: 25375, Y: 54.1, Z: 15721})
	r.mate = r.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 6302}, 32, 32, 32, 2, true)
	r.other = r.w.CreatePlayer(r.mate, &database.CharacterRecord{ServerID: 44, CharName: "Jibrish", AccountID: 8, Level: 60,
		Race: 3, TClass: 12, X: 25380, Y: 54.1, Z: 15721})
	r.w.NPCs[5001] = &NPC{ServerID: 5001, Name: npcName, HP: 100, MaxHP: 100, Level: 10, X: 25376, Z: 15722}
	r.w.RestoreView(sess, back.V)
	return r, sess
}

// The client keeps its scene through a restart, so each slot goes on with the
// same entity under the same id when it is in the new world (an NPC of its
// row and name, a player of its character), and is taken away when it is not
// (the pet, made for the moment, is gone): the removal names the entity the
// slot showed. The next new entity's id is past every one given.
func TestKeptViewKeepsTheSlotsEntities(t *testing.T) {
	old := newKeepRig(t, "a gnoll pup", true)
	old.see(t, old.sess, 5001, 900001, old.other.ServerID)
	vs := old.sess.VisibilityState().(*VisibilityState)
	wantObj := append([]uint32(nil), vs.ChannelObjectID[:3]...)

	r, sess := through(t, old, "a gnoll pup")
	nvs := sess.VisibilityState().(*VisibilityState)
	if nvs.ChannelEntity[0] != 5001 || nvs.ChannelObjectID[0] != wantObj[0] {
		t.Fatalf("the NPC's slot: %d as %d, want 5001 as %d", nvs.ChannelEntity[0], nvs.ChannelObjectID[0], wantObj[0])
	}
	if nvs.ChannelEntity[2] != r.other.ServerID || nvs.ChannelObjectID[2] != wantObj[2] {
		t.Fatalf("the player's slot: %d as %d", nvs.ChannelEntity[2], nvs.ChannelObjectID[2])
	}
	if nvs.ChannelEntity[1] != 0 {
		t.Fatalf("the pet's slot holds %d", nvs.ChannelEntity[1])
	}
	data, _, ref := sess.Connection().GetServerObject(2).GenerateXORDelta()
	if data == nil || data[0] != 0 || ref != 0 {
		t.Fatalf("the pet's slot sends %x ref %d, want its removal whole", data, ref)
	}
	if id := uint32(data[1]) | uint32(data[2])<<8 | uint32(data[3])<<16 | uint32(data[4])<<24; id != wantObj[1] {
		t.Fatalf("the removal names %d, the pet was %d", id, wantObj[1])
	}
	if nvs.nextObjectID < wantObj[2] {
		t.Fatalf("next id %d, at or below one given (%d)", nvs.nextObjectID, wantObj[2])
	}
	// what is kept is described again whole, on from the old numbers
	if data, _, ref := sess.Connection().GetServerObject(1).GenerateXORDelta(); data == nil || ref != 0 || data[0] == 0 {
		t.Fatalf("the NPC's slot sends %x ref %d", data, ref)
	}
}

// The control: an NPC of the same row but another name now (the row was
// changed in between) is not the entity the client shows: taken away.
func TestKeptViewTakesAnotherEntityAway(t *testing.T) {
	old := newKeepRig(t, "a gnoll pup", false)
	old.see(t, old.sess, 5001)
	_, sess := through(t, old, "a gnoll warder")
	if id := sess.VisibilityState().(*VisibilityState).ChannelEntity[0]; id != 0 {
		t.Fatalf("slot 1 still holds %d", id)
	}
	if data, _, _ := sess.Connection().GetServerObject(1).GenerateXORDelta(); data == nil || data[0] != 0 {
		t.Fatalf("no removal: %x", data)
	}
}

// Groups form again among the characters back in the world, leader first;
// one with a single member back is not formed, and no id is given twice.
func TestKeptGroupsFormAgain(t *testing.T) {
	old := newKeepRig(t, "a gnoll pup", false)
	old.w.groups = map[uint32]*Group{
		4: {ID: 4, Members: []EntityID{old.other.ServerID, old.p.ServerID}, lastRoster: map[EntityID]string{}},
	}
	old.p.GroupID, old.other.GroupID = 4, 4
	kept := old.w.KeepGroups()
	if len(kept) != 1 || kept[0].ID != 4 || kept[0].Members[0] != 44 || kept[0].Members[1] != 61 {
		t.Fatalf("kept %+v", kept)
	}
	r, _ := through(t, old, "a gnoll pup")
	n, err := r.w.RestoreGroups(append(kept, KeptGroup{ID: 9, Members: []uint32{61, 777}}))
	if err != nil || n != 1 {
		t.Fatalf("%d formed, %v", n, err)
	}
	g := r.w.groups[4]
	if g == nil || len(g.Members) != 2 || g.Members[0] != r.other.ServerID || r.p.GroupID != 4 || r.other.GroupID != 4 {
		t.Fatalf("group %+v, ids %d %d", g, r.p.GroupID, r.other.GroupID)
	}
	if r.w.groups[9] != nil {
		t.Fatal("a group of one was formed")
	}
	if r.w.nextGroupID < 4 {
		t.Fatalf("the next group id %d could be 4 again", r.w.nextGroupID)
	}
}

// What a login does not carry comes through: the target as the client knows
// it, the lockouts with what is left of them, the power bars, the pet's effects
// and duel window the client asked for; and a dead character is said to be
// dead.
func TestKeptPlayerTargetLockoutsOffers(t *testing.T) {
	now := time.Now()
	old := newKeepRig(t, "a gnoll pup", false)
	old.p.TargetObjectID = 100001
	old.p.Lockouts = []castLockout{{spellID: 900, displayIndex: 4, until: now.Add(40 * time.Second)},
		{spellID: 901, displayIndex: 5, until: now.Add(-time.Second)}}
	old.p.PowerBars, old.p.DuelWindow, old.p.GM, old.p.PetEffects = true, true, true, true
	old.p.ProfanityFilterOff = true
	old.p.ProfanityFilterMedium = true            // petition 1625: the tier kept with it
	old.p.LFG, old.p.GroupChatHidden = true, true // petition 1582: a registration over the restart
	kp, ok := old.w.KeepPlayer(old.sess, now)
	if !ok || kp.Target != 100001 || len(kp.Lockouts) != 1 || kp.Lockouts[0].LeftMS < 39000 || !kp.PowerBars || !kp.DuelWindow || kp.Dead || !kp.PetEffects {
		t.Fatalf("kept %+v", kp)
	}
	r, sess := through(t, old, "a gnoll pup")
	later := now.Add(5 * time.Second)
	if !r.w.RestoreKeptPlayer(sess, kp, later) {
		t.Fatal("no player")
	}
	if !r.p.GM {
		t.Error("a GM kept over the restart is no GM")
	}
	if !r.p.ProfanityFilterMedium {
		t.Error("the profanity filter's Medium tier was lost over the restart")
	}
	if !r.p.ProfanityFilterOff {
		t.Error("the profanity filter turned off came back on over the restart")
	}
	if !r.p.LFG || !r.p.GroupChatHidden {
		t.Errorf("the group search's registration %v, the group chat hidden %v: dropped over the restart", r.p.LFG, r.p.GroupChatHidden)
	}
	if r.p.TargetObjectID != 100001 || !r.p.PowerBars || !r.p.powerBarsDue || !r.p.DuelWindow {
		t.Fatalf("target %d power bars %v due %v duel %v", r.p.TargetObjectID, r.p.PowerBars, r.p.powerBarsDue, r.p.DuelWindow)
	}
	if !r.p.PetEffects || !r.p.petEffectsDue {
		t.Fatalf("pet effects %v due %v: its client asked before the restart", r.p.PetEffects, r.p.petEffectsDue)
	}
	if len(r.p.Lockouts) != 1 || r.p.Lockouts[0].spellID != 900 || r.p.Lockouts[0].until.Sub(later) < 39*time.Second {
		t.Fatalf("lockouts %+v", r.p.Lockouts)
	}
	old.p.HP = 0
	if kp, _ := old.w.KeepPlayer(old.sess, now); !kp.Dead {
		t.Fatal("a dead character kept as alive")
	}
}
