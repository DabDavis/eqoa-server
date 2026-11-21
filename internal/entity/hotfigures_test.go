package entity

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// A heal on a full target is told whole (petition 2016): the caster's own on
// itself, its heal on a full friend (to both), a heal on a full pet; nothing
// is healed past the maximum.
func TestHealOnAFullTargetIsToldWhole(t *testing.T) {
	r := newTagRig(t)
	r.p.HP, r.friend.HP, r.pet.HP = r.p.MaxHP, r.friend.MaxHP, r.pet.MaxHP
	heal := SpellEffect{Kind: EffectHP, Amount: 120, AmountMax: 120}
	r.w.mu.Lock()
	r.w.healLocked(r.p, r.sess, r.p, 120, numberHeal)
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.me || got[0].dst != r.me || got[0].amount != 120 || got[0].tag != 6 {
		t.Errorf("a heal on itself at full: told %v, want (me, 120, me) tag 6", got)
	}
	r.w.mu.Lock()
	r.w.healLocked(r.p, r.sess, r.friend, 120, numberHeal)
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].dst != r.fid || got[0].amount != 120 || got[0].tag != 6 {
		t.Errorf("a heal on a full friend: the healer was told %v, want (me, 120, friend) tag 6", got)
	}
	if got := figTags(t, r.fsess); len(got) != 1 || got[0].src != r.meF || got[0].amount != 120 || got[0].tag != -1 {
		t.Errorf("a heal on a full friend: the friend was told %v, want (healer, 120, it) untagged", got)
	}
	cs := &castState{spell: CastSpell{SpellID: 9400, Name: "Test Pet Mend", Level: 20, Def: SpellDef{Effects: []SpellEffect{heal}}},
		caster: r.p.ServerID, targets: []EntityID{r.pet.ServerID}}
	r.w.mu.Lock()
	r.w.landOnPetLocked(cs, r.p, r.sess, r.pet, r.t0)
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].dst != r.petO || got[0].amount != 120 || got[0].tag != 6 {
		t.Errorf("a heal on its full pet: told %v, want (me, 120, pet) tag 6", got)
	}
	if r.p.HP != r.p.MaxHP || r.friend.HP != r.friend.MaxHP || r.pet.HP != r.pet.MaxHP {
		t.Errorf("HP %d/%d, %d/%d, pet %d/%d: healed past the maximum", r.p.HP, r.p.MaxHP, r.friend.HP, r.friend.MaxHP, r.pet.HP, r.pet.MaxHP)
	}
}

// A heal over time on a pet (Soul Mastery's "Increase HP per tic by 75",
// petition 2142) is told to its owner each tick, at full health too, once a
// tick; another player's on it to that player (tagged) and to the owner from
// that player (untagged).
func TestPetHoTTickIsToldToItsOwner(t *testing.T) {
	r := newTagRig(t)
	ae := &ActiveEffect{SpellID: 700684, Name: "Soul Mastery", Caster: r.p.ServerID, Until: r.later, NextTick: r.t0, HPTick: 75}
	r.pet.Effects = []*ActiveEffect{ae}
	r.pet.HP = 790
	r.w.mu.Lock()
	r.w.petEffectsLocked(r.ps, r.t0)
	r.w.petEffectsLocked(r.ps, r.t0.Add(time.Second)) // no tick due
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.me || got[0].dst != r.petO || got[0].amount != 75 || got[0].tag != 7 {
		t.Errorf("its tick: the owner was told %v, want one (me, 75, pet) tag 7", got)
	}
	if r.pet.HP != r.pet.MaxHP {
		t.Errorf("pet HP %d, want %d", r.pet.HP, r.pet.MaxHP)
	}
	r.w.mu.Lock()
	r.w.petEffectsLocked(r.ps, r.t0.Add(effectTick)) // at full health
	r.w.mu.Unlock()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].amount != 75 || got[0].tag != 7 {
		t.Errorf("its tick at full health: told %v, want (me, 75, pet) tag 7", got)
	}

	// the friend's heal over time on the pet
	fv := GetOrCreateVisibility(r.fsess)
	fv.ChannelEntity[2], fv.ChannelObjectID[2] = r.pet.ServerID, 0x0150CCC0
	ae.Caster = r.friend.ServerID
	r.w.mu.Lock()
	r.w.petEffectsLocked(r.ps, r.t0.Add(2*effectTick))
	r.w.mu.Unlock()
	if got := figTags(t, r.fsess); len(got) != 1 || got[0].dst != 0x0150CCC0 || got[0].amount != 75 || got[0].tag != 7 {
		t.Errorf("the friend's tick on the pet: the friend was told %v, want (it, 75, pet) tag 7", got)
	}
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.fid || got[0].dst != r.petO || got[0].tag != -1 {
		t.Errorf("the friend's tick on the pet: the owner was told %v, want (friend, 75, pet) untagged", got)
	}
}

// A spell's HoT stat is told at the regen tick, its part alone, on a player
// and on a pet; natural regeneration (a 50th, a meal's) shows nothing.
func TestStatHoTIsToldNaturalRegenIsNot(t *testing.T) {
	r := newTagRig(t)
	regen := func() {
		r.w.mu.Lock()
		r.w.TickCount = 0
		r.w.regenSystem(r.t0)
		r.w.npcRegenSystem(r.t0)
		r.w.mu.Unlock()
	}
	r.p.HP, r.pet.HP = 100, 100
	r.p.Effects = []*ActiveEffect{{SpellID: FoodEffectID, Name: "Test Meal", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 30}}}
	r.p.Mods = sumEffectMods(r.p.Effects)
	regen()
	if got := figTags(t, r.sess); len(got) != 0 {
		t.Errorf("natural regeneration and a meal: told %v, want nothing", got)
	}
	if r.p.HP <= 100 || r.pet.HP <= 100 {
		t.Fatalf("no regen ran: HP %d, pet %d", r.p.HP, r.pet.HP)
	}

	r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 9401, Name: "Test Regrowth", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 40}})
	r.p.Mods = sumEffectMods(r.p.Effects)
	r.pet.Effects = []*ActiveEffect{{SpellID: 9402, Name: "Test Pet Regrowth", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 50}}}
	r.p.HP = r.p.MaxHP // at full health too
	regen()
	got := figTags(t, r.sess)
	if len(got) != 2 {
		t.Fatalf("a spell's HoT on the player and the pet: told %v, want two", got)
	}
	if f := got[0]; f.src != r.me || f.dst != r.me || f.amount != 40 || f.tag != 7 {
		t.Errorf("the player's: %v, want (me, 40, me) tag 7", f)
	}
	if f := got[1]; f.src != r.me || f.dst != r.petO || f.amount != 50 || f.tag != 7 {
		t.Errorf("the pet's: %v, want (me, 50, pet) tag 7", f)
	}
}

// Heirloom petitions 2496 and 2499 (2026-11-20): a heal over time recast on a pet takes over the running one's next
// tick, no tick at once (it had landed its first, doubled, tick at every recast); with none ticking it ticks at once;
// a weaker one does not go on.
func TestAPetsHoTRecastKeepsItsTick(t *testing.T) {
	r := newTagRig(t)
	first := &ActiveEffect{SpellID: 700771, Name: "Celestial Healing", Caster: r.p.ServerID, Until: r.t0.Add(24 * time.Second), HPTick: 775}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if !r.w.placePetHoTLocked(r.pet, first, r.t0) || !first.NextTick.Equal(r.t0) {
		t.Fatalf("the first: placed at %v, want at once %v", first.NextTick, r.t0)
	}
	r.pet.Effects = append(r.pet.Effects, first)
	r.pet.HP = 10
	r.w.petEffectsLocked(r.ps, r.t0) // its first tick
	next := first.NextTick
	at := r.t0.Add(3 * time.Second)
	recast := &ActiveEffect{SpellID: 700771, Name: "Celestial Healing", Caster: r.p.ServerID, Until: at.Add(24 * time.Second), HPTick: 775}
	if !r.w.placePetHoTLocked(r.pet, recast, at) || !recast.NextTick.Equal(next) {
		t.Errorf("the recast: next tick %v, want the running one's %v", recast.NextTick, next)
	}
	weak := &ActiveEffect{SpellID: 700001, Name: "Minor Healing", Caster: r.p.ServerID, Until: at.Add(24 * time.Second), HPTick: 50}
	if r.w.placePetHoTLocked(r.pet, weak, at) {
		t.Error("a weaker heal over time went on over a stronger one ticking")
	}
}

// Petition 2593 (a ranger: "Forest's Succor still doesn't seem to heal anybody
// but me, or at least I can't see the numbers even on mercs in my group"): a
// spell's HoT stat cast on another player is told its caster each regen tick,
// its own part over the bearer, whole at full health; the bearer's own figure
// is the whole, as before; a player with no part in it is told nothing.
func TestStatHoTIsToldItsCaster(t *testing.T) {
	r := newTagRig(t)
	other := r.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 11), Port: 7003}, 4, 4, 4, 5, true)
	op := &Player{ServerID: 1003, Name: "Onlooker", Level: 20, HP: 900, MaxHP: 900, X: 102, Y: 10, Z: 100}
	r.w.Players[op.ServerID] = op
	other.SetECSEntity(op)
	other.SetCharacterInWorld(true)
	other.SetPlayerObjectID(0x0140DDD0)
	other.SetNumberTags(true)
	ov := GetOrCreateVisibility(other)
	ov.ChannelEntity[1], ov.ChannelObjectID[1] = r.friend.ServerID, 0x0140DDD1
	ov.ChannelEntity[2], ov.ChannelObjectID[2] = r.pet.ServerID, 0x0140DDD2
	ov.ChannelEntity[3], ov.ChannelObjectID[3] = r.p.ServerID, 0x0140DDD3
	regen := func() {
		r.w.mu.Lock()
		r.w.TickCount = 0
		r.w.regenSystem(r.t0)
		r.w.npcRegenSystem(r.t0)
		r.w.mu.Unlock()
	}

	// Forest's Succor (701142) from the caster on the friend, at full health,
	// beside the friend's own HoT stat
	succor := &ActiveEffect{SpellID: 701142, Name: "Forest's Succor", Caster: r.p.ServerID, Until: r.later, Mods: StatMods{HoT: 205}}
	own := &ActiveEffect{SpellID: 9403, Name: "Test Own Regrowth", Caster: r.friend.ServerID, Until: r.later, Mods: StatMods{HoT: 30}}
	r.friend.Effects = []*ActiveEffect{succor, own}
	r.friend.Mods = sumEffectMods(r.friend.Effects)
	r.friend.HP = r.friend.MaxHP
	regen()
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.me || got[0].dst != r.fid || got[0].amount != 205 || got[0].tag != 7 {
		t.Errorf("its HoT stat on a full friend: the caster was told %v, want one (me, 205, friend) tag 7", got)
	}
	if got := figTags(t, r.fsess); len(got) != 1 || got[0].src != 0x0140AAA0 || got[0].dst != 0x0140AAA0 || got[0].amount != 235 || got[0].tag != -1 {
		t.Errorf("the friend was told %v, want its whole (it, 235, it) untagged, as before", got)
	}
	if got := figTags(t, other); len(got) != 0 {
		t.Errorf("an onlooker was told %v, want nothing", got)
	}
	if r.friend.HP != r.friend.MaxHP {
		t.Errorf("friend HP %d, want capped at %d", r.friend.HP, r.friend.MaxHP)
	}

	// the friend's on the caster's full pet: the friend told its part over the
	// pet, the owner the whole, from itself, as before
	r.friend.Effects, r.friend.Mods = nil, StatMods{}
	fv := GetOrCreateVisibility(r.fsess)
	fv.ChannelEntity[2], fv.ChannelObjectID[2] = r.pet.ServerID, 0x0150CCC0
	r.pet.HP = r.pet.MaxHP
	r.pet.Effects = []*ActiveEffect{{SpellID: 701142, Name: "Forest's Succor", Caster: r.friend.ServerID, Until: r.later, Mods: StatMods{HoT: 205}}}
	regen()
	if got := figTags(t, r.fsess); len(got) != 1 || got[0].src != 0x0140AAA0 || got[0].dst != 0x0150CCC0 || got[0].amount != 205 || got[0].tag != 7 {
		t.Errorf("its HoT stat on a full pet: the caster was told %v, want one (it, 205, pet) tag 7", got)
	}
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.me || got[0].dst != r.petO || got[0].amount != 205 || got[0].tag != -1 {
		t.Errorf("the pet's owner was told %v, want (me, 205, pet) untagged, as before", got)
	}
	if got := figTags(t, other); len(got) != 0 {
		t.Errorf("an onlooker was told %v, want nothing", got)
	}
	if r.pet.HP != r.pet.MaxHP {
		t.Errorf("pet HP %d, want capped at %d", r.pet.HP, r.pet.MaxHP)
	}
}

// Petition 2593's merc: a ranger's HoT stat on a whole merc in its group keeps
// the merc in the regeneration's set and is told the ranger and the merc's
// owner each regen tick; its hit points stay at its maximum.
func TestStatHoTOnAWholeMercIsToldItsCasterAndOwner(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(classWarrior, 0, 0), MercAssist)
	rsess, ranger := r.player("Ranger", classRanger, 0)
	osess, _ := r.player("Onlooker", classWarrior, 0)
	for i, s := range []*session.Session{r.sess, rsess, osess} {
		s.SetNumberTags(true)
		v := GetOrCreateVisibility(s)
		v.ChannelEntity[5], v.ChannelObjectID[5] = ms.npc.ServerID, 0x0160E000+uint32(i)
		s.Queue().GetPending()
	}
	r.w.mu.Lock()
	ms.npc.Effects = append(ms.npc.Effects, &ActiveEffect{SpellID: 701142, Name: "Forest's Succor", Caster: ranger.ServerID,
		Until: r.at(time.Hour), Mods: StatMods{HoT: 205}})
	syncMercBody(ms)
	ms.npc.HP = ms.npc.MaxHP
	r.w.wakeNPCLocked(ms.npc)
	r.w.TickCount = 0
	r.w.npcRegenSystem(r.at(0))
	_, kept := r.w.sets.regen.m[ms.npc.ServerID]
	hp, most := ms.npc.HP, ms.npc.MaxHP
	r.w.mu.Unlock()
	if !kept {
		t.Error("the whole merc with a HoT stat left the regeneration's set")
	}
	if got := figTags(t, rsess); len(got) != 1 || got[0].src != rsess.PlayerObjectID() || got[0].dst != 0x0160E001 || got[0].amount != 205 || got[0].tag != 7 {
		t.Errorf("the ranger was told %v, want one (it, 205, merc) tag 7", got)
	}
	if got := figTags(t, r.sess); len(got) != 1 || got[0].src != r.sess.PlayerObjectID() || got[0].dst != 0x0160E000 || got[0].amount != 205 || got[0].tag != -1 {
		t.Errorf("the owner was told %v, want (it, 205, merc) untagged", got)
	}
	if got := figTags(t, osess); len(got) != 0 {
		t.Errorf("an onlooker was told %v, want nothing", got)
	}
	if hp != most {
		t.Errorf("merc HP %d, want capped at %d", hp, most)
	}
}
