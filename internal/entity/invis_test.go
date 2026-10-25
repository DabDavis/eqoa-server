package entity

import (
	"net"
	"testing"
	"time"
)

// Casting a spell or swinging at something ends invisibility (byte 154 off
// again): the cast at its start, the attack at its engagement and at every
// round it swings. The control: a spell landing ON the invisible player
// from someone else leaves it.
func TestInvisibilityEndsWithACastOrAnAttack(t *testing.T) {
	invis := CastSpell{SpellID: 509, DisplayIndex: 6, Scope: 1, Name: "Invisibility", Icon: 0x7777,
		CastTime: 2 * time.Second, PowerCost: 10, Range: 30, Def: ParseSpellNamed("Invisibility", "Target, invis, 5.5min", "")}
	seen := func(p *Player) bool { return SerializePlayerUpdate(1, p)[154] == 0 }

	c := newCaster(t)
	c.land(t, invis, c.me, 0)
	if seen(c.p) {
		t.Fatal("Invisibility on oneself: byte 154 not set")
	}
	if !c.w.StartCast(c.sess, minorHeal(), c.me, c.at(10*time.Second)) {
		t.Fatal("the heal did not begin")
	}
	if !seen(c.p) || c.p.Invisible() {
		t.Error("a cast begun left its caster invisible")
	}

	c = newCaster(t)
	c.land(t, invis, c.me, 0)
	c.w.addEffectLocked(c.p, &ActiveEffect{SpellID: 99, Name: "Other's buff", Until: c.at(time.Hour)}, c.at(5*time.Second))
	if seen(c.p) {
		t.Error("the control: a spell landing on the invisible player showed it")
	}
	if !c.w.EngageAttack(c.sess, c.npcO, Swing{Kind: 0, Delay: 3 * time.Second, Reach: 5}, c.at(6*time.Second)) {
		t.Fatal("no fight")
	}
	if !seen(c.p) {
		t.Error("an attack engaged left the attacker invisible")
	}

	// invisible again mid-fight: the next round shows it
	c.land(t, invis, c.me, 7*time.Second)
	c.p.AutoAttack = true
	c.p.NextSwing = c.at(11 * time.Second)
	c.w.mu.Lock()
	c.w.attackSystem(c.at(11 * time.Second))
	c.w.mu.Unlock()
	if !seen(c.p) {
		t.Error("a round swung left the attacker invisible")
	}
}

// Invisibility cast at a friend lands on the friend: another player, or a
// pet, whose own channel's byte 154 then hides it, until the pet swings.
// The control: a creature's channel never had it.
func TestInvisibilityOnAFriendOrAPet(t *testing.T) {
	invis := CastSpell{SpellID: 509, DisplayIndex: 6, Scope: 1, Name: "Invisibility", Icon: 0x7777,
		CastTime: 2 * time.Second, PowerCost: 10, Range: 30, Def: ParseSpellNamed("Invisibility", "Target, invis, 5.5min", "")}
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	if SerializeNPCUpdate(petID, pet)[154] != 0 {
		t.Fatal("the control: a pet's byte 154 before any invisibility")
	}
	if !r.w.StartCast(r.sess, invis, petID, r.at(20*time.Second)) {
		t.Fatalf("Invisibility at the pet did not begin: %x", r.sent())
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(20*time.Second + invis.CastTime + castLand))
	r.w.mu.Unlock()
	if SerializeNPCUpdate(petID, pet)[154] != 1 {
		t.Fatalf("the invisible pet's byte 154 is not 1 (effects %+v)", pet.Effects)
	}
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(30*time.Second))
	for ms := 30100; ms < 40000; ms += 100 {
		r.tick(time.Duration(ms) * time.Millisecond)
	}
	if SerializeNPCUpdate(petID, pet)[154] != 0 {
		t.Error("the pet fought on unseen")
	}

	c := newCaster(t)
	sess2 := c.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 13), Port: 7000}, 3, 3, 3, 4, true)
	friend := &Player{ServerID: 1003, Name: "Friend", HP: 100, MaxHP: 100, X: 104, Y: 10, Z: 100}
	c.w.Players[friend.ServerID] = friend
	sess2.SetECSEntity(friend)
	sess2.SetCharacterInWorld(true)
	sess2.SetPlayerObjectID(0x22220002)
	GetOrCreateVisibility(c.sess).ChannelEntity[2] = friend.ServerID
	GetOrCreateVisibility(c.sess).ChannelObjectID[2] = 0x01312EA0
	c.land(t, invis, 0x01312EA0, 0)
	if SerializePlayerUpdate(1, friend)[154] != 1 || c.p.Invisible() {
		t.Errorf("Invisibility at a friend: the friend's byte 154 %d, the caster invisible %v", SerializePlayerUpdate(1, friend)[154], c.p.Invisible())
	}
}
