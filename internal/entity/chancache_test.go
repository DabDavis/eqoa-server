package entity

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The channel cache (chancache.go) changes no byte: a scene of many players
// seeing the same creatures, mercs and a pet, each viewer's client asking for
// another look (the disc's, the live game's, armour, every aura layout,
// decorations), players targeting what their own tables number, PvP foes and
// a fight, creatures walking, hurt, dying and lying as corpses, a creature
// that cannot be selected, a dyed one: at every tick's end every channel
// holds what serializeEntityForChannelIn makes of its entity for its viewer
// (the path the cache replaced), and a pass made side by side, the cached and
// the old, gives every channel the same bytes (the old serializer is a
// verbatim copy of it before the cache: chanlegacy_test.go). The control: a pass's cache
// kept into the next (no reset) after the world changed gives channels the
// old path does not.

// chanScene is the scene, ticked to its first view.
type chanScene struct {
	r       *mercRig
	viewers []*session.Session
	crowd   []*NPC
	d       time.Duration
}

func newChanScene(t *testing.T) *chanScene {
	t.Helper()
	r := newMercRig(t)
	c := &chanScene{r: r}
	// each look a client may ask for, every aura layout with the ones it
	// needs (session.go: a layout counts only with those before it)
	auras := func(s *session.Session, layout int) {
		s.SetLiveLook(true)
		s.SetArmorLook(true)
		s.SetPieceAuras(true)
		for i, set := range []func(bool){s.SetBodyAura, s.SetCapeAura, s.SetHandAura, s.SetSecondAura, s.SetHatAura} {
			set(i < layout)
		}
	}
	looks := []func(s *session.Session){
		func(s *session.Session) {}, // a PS2's, the disc's
		func(s *session.Session) { s.SetLiveLook(true) },
		func(s *session.Session) { s.SetLiveLook(true); s.SetArmorLook(true) },
		func(s *session.Session) { auras(s, 0) },
		func(s *session.Session) { auras(s, 1) },
		func(s *session.Session) { auras(s, 2) },
		func(s *session.Session) { auras(s, 3) },
		func(s *session.Session) { auras(s, 4) },
		func(s *session.Session) { auras(s, 5); s.SetDecorLights(DecorPetArtLayout) },
	}
	c.viewers = append(c.viewers, r.sess)
	for i := 1; i < 12; i++ {
		s, p := r.player(fmt.Sprintf("Viewer%02d", i), i%15, i%10)
		p.HP, p.MaxHP = 900, 1000
		p.Dyes[database.DyeChest] = uint32(0x11223300 + i)
		if i%4 == 0 {
			p.pvp.on = true // foes of each other
		}
		if i%5 == 0 {
			p.LastCombatTime = r.at(0) // the battle music's byte
		}
		c.viewers = append(c.viewers, s)
	}
	for i, s := range c.viewers {
		looks[i%len(looks)](s)
	}
	r.p.pvp.on = true
	for i := 0; i < 24; i++ {
		n := r.creature(fmt.Sprintf("a crowd creature %d", i), 10+i, float32(4+i))
		n.Z += float32(i % 5)
		switch i % 6 {
		case 1:
			n.Tint = 3
		case 2:
			n.NoSelect = true
		case 3:
			n.RobeStyle = "2"
			n.Race = "4"
		}
		c.crowd = append(c.crowd, n)
	}
	// dyed creatures (npcdyes.go: by their names' keys)
	dyes := map[string]*Dyes{}
	for i, n := range c.crowd {
		if i%6 == 4 {
			d := &Dyes{}
			d[database.DyeChest], d[database.DyeLegs], d[database.DyeRobe] = 0xC8301EFF, 0x1E8C3CFF, uint32(0x10203000+i)
			dyes[n.classKey()] = d
		}
	}
	old := npcDyes.Load()
	npcDyes.Store(&dyes)
	t.Cleanup(func() { npcDyes.Store(old) })
	// a merc and a pet among them: its owner's dyes, a pet's name colour
	r.p.Dyes[database.DyeChest], r.p.Dyes[database.DyeBoots] = 0x8040C0FF, 0x203040FF
	nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 40, "Target, Decrease Hitpoints by 30")
	r.hire(r.kit(13, 0, 0, nuke), MercAssist)
	r.w.mu.Lock()
	pet := &NPC{ServerID: 4950, Name: "Viewer01`s pet", Level: 20, HP: 500, MaxHP: 500, ModelID: 0x8A416241, Size: 1,
		Owner: r.w.sessionPlayerLocked(c.viewers[1]).ServerID, X: r.p.X + 3, Z: r.p.Z + 2, World: r.p.World}
	r.w.putNPCLocked(pet)
	r.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	// a board and a titled NPC: their name plates (boardplate.go, npctitle.go)
	board := &NPC{ServerID: 4960, Name: "Mercenary Recruitment", X: r.p.X + 5, Y: r.p.Y, Z: r.p.Z, Facing: 1.25,
		ModelID: int64(PaperedBoardBody), Size: 1, NPCType: 0x80, Level: 1, HP: 10, MaxHP: 10, World: r.p.World}
	titled := &NPC{ServerID: 4961, Name: "Guard Soalen", Title: "the Watchful", X: r.p.X + 6, Y: r.p.Y, Z: r.p.Z + 1,
		ModelID: 0x55432169, Size: 1, NPCType: 0x80, Level: 30, HP: 900, MaxHP: 900, World: r.p.World}
	for _, n := range []*NPC{board, titled} {
		r.w.putNPCLocked(n)
		r.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	r.w.mu.Unlock()
	c.tick() // the view's first pass
	return c
}

// tick runs the whole server tick, the view's pass due, 100 ms on.
func (c *chanScene) tick() {
	w := c.r.w
	w.mu.Lock()
	if w.TickCount%VisibilityTickInterval != VisibilityTickInterval-1 {
		w.TickCount = VisibilityTickInterval*(w.TickCount/VisibilityTickInterval+1) - 1
	}
	w.mu.Unlock()
	c.d += 100 * time.Millisecond
	w.Tick(c.r.at(c.d))
}

// change is the world going on: the creatures walk, fight a viewer, are hurt
// and die; viewers target what their own tables number.
func (c *chanScene) change(step int) {
	w := c.r.w
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, n := range c.crowd {
		if n.Dead {
			continue
		}
		ox := n.X
		n.X += 0.4 * float32(i%3)
		n.Facing += 0.1
		n.VelX = 0.4 * float32(i%3)
		w.SpatialIndex.Update(n.ServerID, ox, n.Z, n.X, n.Z)
		switch (i + step) % 7 {
		case 0:
			n.HP = max(1, n.HP-n.MaxHP/5)
		case 3:
			if step > 2 && i%4 == 1 {
				n.HP, n.Dead = 0, true
				w.wakeNPCLocked(n)
			}
		case 5:
			n.CurrentEnemy = c.r.p.ServerID // a fight: its target, as each viewer numbers it
			w.wakeNPCLocked(n)
		}
	}
	for i, s := range c.viewers {
		p := w.sessionPlayerLocked(s)
		vis := GetOrCreateVisibility(s)
		for k := (i + step) % len(vis.ChannelEntity); k < len(vis.ChannelEntity); k++ {
			if vis.ChannelEntity[k] != 0 {
				p.TargetObjectID = vis.ChannelObjectID[k] // in its own table
				break
			}
		}
	}
}

// compare holds every channel to the old path (the file's doc), side by side
// and as the tick left it; it returns the channels compared.
func (c *chanScene) compare(t *testing.T, when string) int {
	t.Helper()
	w := c.r.w
	w.mu.Lock()
	defer w.mu.Unlock()
	var cache chanCache
	cache.reset()
	var scratch chanScratch
	n := 0
	for _, s := range c.viewers {
		vis := GetOrCreateVisibility(s)
		conn := s.Connection()
		for i, id := range vis.ChannelEntity {
			if id == 0 {
				continue
			}
			obj := vis.ChannelObjectID[i]
			want := w.legacySerializeEntityForChannelIn(nil, s, id, obj)
			got := w.serializeEntityChannelCached(&cache, &scratch, s, id, obj)
			if string(got) != string(want) || (got == nil) != (want == nil) {
				t.Fatalf("%s: slot %d (entity %d, id %#x) to %s: cached %x\nwant %x", when, i+1, id, obj,
					w.sessionPlayerLocked(s).Name, got, want)
			}
			if one := w.serializeEntityForChannelIn(nil, s, id, obj); string(one) != string(want) || (one == nil) != (want == nil) {
				t.Fatalf("%s: slot %d (entity %d, id %#x) to %s: uncached %x\nwant %x", when, i+1, id, obj,
					w.sessionPlayerLocked(s).Name, one, want)
			}
			if disc, p := w.serializeEntityDiscIn(nil, s, id, obj); true {
				wantDisc, wantP := w.legacySerializeEntityDiscIn(nil, s, id, obj)
				if string(disc) != string(wantDisc) || p != wantP {
					t.Fatalf("%s: slot %d (entity %d) to %s: disc %x (%p)\nwant %x (%p)", when, i+1, id,
						w.sessionPlayerLocked(s).Name, disc, p, wantDisc, wantP)
				}
			}
			if want != nil {
				if put := conn.GetServerObject(i + 1).GetData(); string(put) != string(want) {
					t.Fatalf("%s: slot %d (entity %d) to %s holds %x\nwant %x", when, i+1, id, w.sessionPlayerLocked(s).Name, put, want)
				}
			}
			n++
		}
	}
	return n
}

func TestChannelCacheChangesNoByte(t *testing.T) {
	c := newChanScene(t)
	total, shared := 0, 0
	for step := 0; step < 30; step++ {
		c.change(step)
		c.tick()
		total += c.compare(t, fmt.Sprintf("step %d", step))
	}
	w := c.r.w
	w.mu.Lock()
	seen := map[EntityID]int{}
	for _, s := range c.viewers {
		for _, id := range GetOrCreateVisibility(s).ChannelEntity {
			if id != 0 {
				seen[id]++
			}
		}
	}
	dead, fighting := 0, 0
	for _, n := range c.crowd {
		if n.Dead {
			dead++
		} else if n.CurrentEnemy != 0 {
			fighting++
		}
	}
	w.mu.Unlock()
	for _, k := range seen {
		if k > 1 {
			shared++
		}
	}
	if total == 0 || shared < 10 || dead == 0 || fighting == 0 {
		t.Fatalf("the scene tests little: %d channels compared, %d entities on several viewers' channels, %d dead, %d fighting",
			total, shared, dead, fighting)
	}
	t.Logf("%d channels compared over 30 ticks; %d entities seen by more than one viewer; %d dead, %d fighting", total, shared, dead, fighting)
}

// The control: a pass's cache kept into the next after the world changed
// (no reset) gives channels the old path does not, so the reset is what
// keeps each pass's bytes its own.
func TestChannelCacheKeptPastItsPassIsStale(t *testing.T) {
	c := newChanScene(t)
	w := c.r.w
	var cache chanCache
	cache.reset()
	var scratch chanScratch
	pass := func(stale bool) (differ int) {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, s := range c.viewers {
			vis := GetOrCreateVisibility(s)
			for i, id := range vis.ChannelEntity {
				if id == 0 {
					continue
				}
				want := w.legacySerializeEntityForChannelIn(nil, s, id, vis.ChannelObjectID[i])
				if got := w.serializeEntityChannelCached(&cache, &scratch, s, id, vis.ChannelObjectID[i]); string(got) != string(want) {
					differ++
				}
			}
		}
		return differ
	}
	if d := pass(false); d != 0 {
		t.Fatalf("the first pass: %d channels differ", d)
	}
	c.change(4)
	if d := pass(true); d == 0 {
		t.Fatal("a cache kept past its pass gave every channel the old path's bytes: the control shows nothing")
	}
	cache.reset()
	if d := pass(false); d != 0 {
		t.Fatalf("after the reset: %d channels differ", d)
	}
}

// The whole system as the tick runs it: every channel entityChannelSyncSystem
// puts holds the old serializer's bytes (chanlegacy_test.go) for its viewer,
// pass after pass of a changing world, and a pass on an unchanged world
// marks no channel again (its bytes are the same).
func TestEntityChannelSyncSystemIsTheOld(t *testing.T) {
	c := newChanScene(t)
	w := c.r.w
	for step := 0; step < 10; step++ {
		c.change(step)
		w.mu.Lock()
		w.entityChannelSyncSystem()
		n := 0
		for _, s := range c.viewers {
			conn := s.Connection()
			vis := GetOrCreateVisibility(s)
			for i, id := range vis.ChannelEntity {
				if id == 0 {
					continue
				}
				want := w.legacySerializeEntityForChannelIn(nil, s, id, vis.ChannelObjectID[i])
				ch := conn.GetServerObject(i + 1)
				if got := ch.GetData(); want != nil && string(got) != string(want) {
					w.mu.Unlock()
					t.Fatalf("step %d, %s slot %d: %x\nwant %x", step, w.sessionPlayerLocked(s).Name, i+1, got, want)
				}
				ch.ClearDirty()
				n++
			}
		}
		w.entityChannelSyncSystem() // again, nothing changed
		for _, s := range c.viewers {
			conn := s.Connection()
			for i, id := range GetOrCreateVisibility(s).ChannelEntity {
				if id != 0 && conn.GetServerObject(i+1).IsDirty() {
					w.mu.Unlock()
					t.Fatalf("step %d, %s slot %d: marked again with nothing changed", step, w.sessionPlayerLocked(s).Name, i+1)
				}
			}
		}
		w.mu.Unlock()
		if n == 0 {
			t.Fatal("no channels")
		}
		c.tick()
	}
}
