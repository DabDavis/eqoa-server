package entity

import "testing"

// A helping "Target" item use lands on its friendly target (work board 693 step
// 1, 1572 "Pronouns: In/Vis"): at another player it lands on that one, not the
// user, and the user's own invisibility stays on; at the user, on the user; at
// a player's pet, on the pet. At a creature it is refused "Target is not
// friendly.", with no target "No target.", out of range "Out of range.": none
// spends a charge (a refusal is never "").
func TestAHelpingItemUseLandsOnAFriend(t *testing.T) {
	inVis := func() PotionEffect {
		return NewPotionEffect(965009, "Pronouns: In/Vis", "Target, Invis, 10min", "Superior Invisibility", 0x9CD122E0, "", 0, 0)
	}
	sow := func() PotionEffect {
		return NewPotionEffect(990988, "Special thanks, Waldorf.", "Target, Sow, 30min", "Super SoW", 638503769, "attack", 0, 0)
	}
	hasInvis := func(p *Player) bool {
		for _, e := range p.Effects {
			if e.Invis {
				return true
			}
		}
		return false
	}
	hasSpeed := func(p *Player) bool {
		for _, e := range p.Effects {
			if e.Speed {
				return true
			}
		}
		return false
	}
	r := newAreaRig(t)
	box := r.player(3001, "Box", 1005, 1000)
	use := func(pe PotionEffect, target uint32) string {
		t.Helper()
		refusal, err := r.w.UsePotion(r.sess, pe, target, r.t0)
		if err != nil {
			t.Fatal(err)
		}
		return refusal
	}

	// the user hidden first, then a box: both stay hidden
	if got := use(inVis(), r.me); got != "" {
		t.Fatalf("at the user: refused %q", got)
	}
	if !hasInvis(r.p) {
		t.Fatal("at the user: the user is not invisible")
	}
	if got := use(inVis(), r.objectID(box.ServerID)); got != "" {
		t.Fatalf("at another player: refused %q", got)
	}
	if !hasInvis(box) {
		t.Fatal("at another player: that player is not invisible")
	}
	if !hasInvis(r.p) {
		t.Fatal("the user's own invisibility broke on a use at another")
	}

	// a buff at another lands on that one only
	if got := use(sow(), r.objectID(box.ServerID)); got != "" {
		t.Fatalf("SoW at another player: refused %q", got)
	}
	if !hasSpeed(box) || hasSpeed(r.p) {
		t.Fatalf("SoW at another player: on the box %v, on the user %v; want the box alone", hasSpeed(box), hasSpeed(r.p))
	}

	// a pet
	pet := r.petOf(box, 3002, 1004, 1000)
	if got := use(sow(), r.objectID(pet.ServerID)); got != "" {
		t.Fatalf("SoW at a player's pet: refused %q", got)
	}
	landed := false
	for _, e := range pet.Effects {
		landed = landed || e.Speed
	}
	if !landed {
		t.Fatal("SoW at a player's pet: not on the pet")
	}

	// refusals, each spending no charge
	rat := r.creature(3003, "a rat", 1003, 1000)
	if got := use(inVis(), r.objectID(rat.ServerID)); got != castNotFriendly {
		t.Fatalf("at a creature: %q, want %q", got, castNotFriendly)
	}
	if hasInvis(nil2(rat)) {
		t.Fatal("at a creature: it landed")
	}
	if got := use(inVis(), 0xFFFFFFFF); got != castNoTarget {
		t.Fatalf("no target: %q, want %q", got, castNoTarget)
	}
	far := r.player(3004, "Far", 1000+PotionRange+5, 1000)
	if got := use(inVis(), r.objectID(far.ServerID)); got == "" {
		t.Fatal("out of range: used")
	}
	if hasInvis(far) {
		t.Fatal("out of range: it landed")
	}
}

// nil2 stands a creature's effects in for a player's, for hasInvis.
func nil2(n *NPC) *Player { return &Player{Effects: n.Effects} }
