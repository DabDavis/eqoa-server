package entity

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The player's harmful effects on the creatures its client draws (npcdebuffs.go, Heirloom
// petition 2997), on the pet rig (pet_test.go): a level 45 magician and a desert crab on
// its client's slot 0 (object id 0x0133E8A0).

// npcDebuffSets takes the OpcodeNPCDebuffsData bodies queued for the rig's session.
func (r *petRig) npcDebuffSets() [][]byte {
	var out [][]byte
	for _, m := range r.sess.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeNPCDebuffsData {
			out = append(out, d[2:])
		}
	}
	return out
}

// npcDebuffsPass runs the system at t0+d (on a tick it looks at).
func (r *petRig) npcDebuffsPass(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount = 0
	r.w.npcDebuffsSystem(r.at(d))
}

// putOn lays e on n as the cast paths do: in its effects, n in w.affected.
func (r *petRig) putOn(n *NPC, e *ActiveEffect) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	n.Effects = append(n.Effects, e)
	if r.w.affected == nil {
		r.w.affected = make(map[EntityID]*NPC)
	}
	r.w.affected[n.ServerID] = n
}

// decodedDebuff is one effect of a set as a client reads it.
type decodedDebuff struct {
	icon   uint32
	whose  byte
	leftMS uint32
	name   string
}

// decodeNPCDebuffs reads a set's body as a client does: creature ids to their effects, in
// order.
func decodeNPCDebuffs(t *testing.T, b []byte) ([]uint32, map[uint32][]decodedDebuff) {
	t.Helper()
	ids, out := []uint32(nil), map[uint32][]decodedDebuff{}
	n, i := int(b[0]), 1
	for range n {
		id := binary.LittleEndian.Uint32(b[i:])
		c := int(b[i+4])
		i += 5
		ids = append(ids, id)
		for range c {
			d := decodedDebuff{icon: binary.LittleEndian.Uint32(b[i:]), whose: b[i+4], leftMS: binary.LittleEndian.Uint32(b[i+5:])}
			l := int(b[i+9])
			d.name = string(b[i+10 : i+10+l])
			i += 10 + l
			out[id] = append(out[id], d)
		}
	}
	if i != len(b) {
		t.Fatalf("set of %d bytes read to %d", len(b), i)
	}
	return ids, out
}

// The player's own harmful effect goes out with its icon, whose, time left and name;
// another player's effect on the same crab and the player's own beneficial one do not. A
// look that changes nothing sends nothing; the effect run out sends the crab with none.
func TestNPCDebuffsMineOnly(t *testing.T) {
	r := newPetRig(t)
	crab := r.enemy
	if !r.w.SetNPCDebuffs(r.sess, true) {
		t.Fatal("no player")
	}
	r.putOn(crab, &ActiveEffect{SpellID: 501, Name: "Weaken", Icon: 0xAAAA0001, Caster: r.p.ServerID,
		Until: r.at(60 * time.Second), Mods: StatMods{Str: -20}})
	r.putOn(crab, &ActiveEffect{SpellID: 502, Name: "Someone's Snare", Icon: 0xAAAA0002, Caster: 4242,
		Until: r.at(60 * time.Second), Snare: true})
	r.putOn(crab, &ActiveEffect{SpellID: 503, Name: "A Heal", Icon: 0xAAAA0003, Caster: r.p.ServerID,
		Until: r.at(60 * time.Second), HPTick: 10})
	r.npcDebuffsPass(10 * time.Second)
	sets := r.npcDebuffSets()
	if len(sets) != 1 {
		t.Fatalf("%d sets sent, want 1", len(sets))
	}
	ids, fx := decodeNPCDebuffs(t, sets[0])
	if len(ids) != 1 || ids[0] != 0x0133E8A0 {
		t.Fatalf("creatures %x, want the crab's channel id alone", ids)
	}
	if got := fx[0x0133E8A0]; len(got) != 1 || got[0] != (decodedDebuff{0xAAAA0001, npcDebuffMine, 50000, "Weaken"}) {
		t.Fatalf("crab's effects %+v", got)
	}
	r.sess.Queue().Clear()
	r.npcDebuffsPass(11 * time.Second)
	if n := len(r.npcDebuffSets()); n != 0 {
		t.Fatalf("%d sets sent with nothing changed", n)
	}
	r.npcDebuffsPass(61 * time.Second) // Weaken ran out
	sets = r.npcDebuffSets()
	if len(sets) != 1 || sets[0][0] != 0 {
		t.Fatalf("the run-out effect: sets %x, want one empty", sets)
	}
}

// A recast (a later end) goes out again; so does its pet's debuff (whose 1) and a merc's
// is none of the player's unless it hired it.
func TestNPCDebuffsRecastAndPet(t *testing.T) {
	r := newPetRig(t)
	crab := r.enemy
	r.w.SetNPCDebuffs(r.sess, true)
	e := &ActiveEffect{SpellID: 501, Name: "Weaken", Icon: 0xAAAA0001, Caster: r.p.ServerID,
		Until: r.at(30 * time.Second), Mods: StatMods{Str: -20}}
	r.putOn(crab, e)
	r.npcDebuffsPass(0)
	r.sess.Queue().Clear()
	r.w.mu.Lock()
	e.Until = r.at(40 * time.Second) // cast again 10 s later
	r.w.mu.Unlock()
	r.npcDebuffsPass(10 * time.Second)
	if sets := r.npcDebuffSets(); len(sets) != 1 {
		t.Fatalf("the recast: %d sets, want 1", len(sets))
	}
	r.sess.Queue().Clear()
	r.putOn(crab, &ActiveEffect{SpellID: 9901, Name: "Pet Rot", Icon: 0xAAAA0009, Caster: r.p.ServerID, pet: 3000,
		Until: r.at(40 * time.Second), HPTick: -12})
	r.putOn(crab, &ActiveEffect{SpellID: 9902, Name: "Merc Slow", Icon: 0xAAAA000A, Caster: 3101, merc: 3101,
		Until: r.at(40 * time.Second), Snare: true})
	r.npcDebuffsPass(11 * time.Second)
	sets := r.npcDebuffSets()
	if len(sets) != 1 {
		t.Fatalf("the pet's debuff: %d sets, want 1", len(sets))
	}
	_, fx := decodeNPCDebuffs(t, sets[0])
	got := fx[0x0133E8A0]
	if len(got) != 2 || got[1] != (decodedDebuff{0xAAAA0009, npcDebuffPet, 29000, "Pet Rot"}) {
		t.Fatalf("crab's effects %+v: want Weaken, then the pet's Pet Rot, no one else's merc", got)
	}
}

// A creature off the client's slots is left out; the target goes first; a set never
// outgrows a message, the farthest left out.
func TestNPCDebuffsSlotsTargetBudget(t *testing.T) {
	r := newPetRig(t)
	r.w.SetNPCDebuffs(r.sess, true)
	vs := GetOrCreateVisibility(r.sess)
	long := "A Very Long Debuff Name That Goes On And On For Ever More"
	for i := range 20 {
		n := &NPC{ServerID: EntityID(5000 + i), Name: "gnoll", Level: 30, HP: 100, MaxHP: 100,
			X: r.p.X + float32(i+1), Z: r.p.Z, World: 2}
		r.w.NPCs[n.ServerID] = n
		if i < 19 { // the last is on no slot
			vs.ChannelEntity[i+1] = n.ServerID
			vs.ChannelObjectID[i+1] = uint32(0x02000000 + i)
		}
		for k := range NPCDebuffsMost + 2 {
			r.putOn(n, &ActiveEffect{SpellID: 600 + k, Name: fmt.Sprintf("%s %d", long, k), Icon: uint32(0xBB000000 + k),
				Caster: r.p.ServerID, Until: r.at(time.Minute), Root: true})
		}
	}
	r.p.TargetObjectID = 0x02000000 + 18 // the farthest on a slot
	r.npcDebuffsPass(0)
	sets := r.npcDebuffSets()
	if len(sets) != 1 {
		t.Fatalf("%d sets", len(sets))
	}
	if len(sets[0])+2 > protocol.DefaultMessageSize-64 {
		t.Fatalf("a set of %d bytes", len(sets[0]))
	}
	ids, fx := decodeNPCDebuffs(t, sets[0])
	if len(ids) == 0 || ids[0] != 0x02000000+18 {
		t.Fatalf("creatures %x: the target not first", ids)
	}
	if len(ids) > 1 && ids[1] != 0x02000000 {
		t.Fatalf("creatures %x: the nearest not after the target", ids)
	}
	for _, id := range ids {
		if len(fx[id]) != NPCDebuffsMost {
			t.Fatalf("creature %x carries %d effects, want %d", id, len(fx[id]), NPCDebuffsMost)
		}
	}
}

// A keep restart carries the client's ask; a takeover's new client has not asked.
func TestNPCDebuffsKept(t *testing.T) {
	now := time.Now()
	old := newKeepRig(t, "a gnoll pup", false)
	old.p.NPCDebuffs = true
	kp, ok := old.w.KeepPlayer(old.sess, now)
	if !ok || !kp.NPCDebuffs {
		t.Fatalf("kept %+v", kp)
	}
	r, sess := through(t, old, "a gnoll pup")
	if !r.w.RestoreKeptPlayer(sess, kp, now) {
		t.Fatal("no player")
	}
	if !r.p.NPCDebuffs || !r.p.npcDebuffsDue {
		t.Fatalf("npc debuffs %v due %v: its client asked before the restart", r.p.NPCDebuffs, r.p.npcDebuffsDue)
	}
}
