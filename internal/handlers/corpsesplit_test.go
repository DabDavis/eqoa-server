package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A corpse's coins are shared with the opener's group (petition 1228,
// Waldorf: "During live, the entire group shared a portion of it"): ten coins
// among three members near the kill are three each and the odd one the
// opener's; a member out of the kill's reach takes none. The control: one in
// no group takes them all.
func TestACorpsesCoinsAreSharedWithTheGroup(t *testing.T) {
	c := newChatWorld(t)
	sa, pa := c.join(1, "Alda", 100, 100, 0)
	sb, pb := c.join(2, "Bren", 102, 100, 0)
	sc, pc := c.join(3, "Cael", 104, 100, 0)
	sd, pd := c.join(4, "Dirn", 900, 100, 0) // 800 from the kill
	c.group(t, sa, pa, sb, pb)
	c.gw.GroupInvite(sa, c.gw.IDSeenBy(sa, pc))
	c.gw.GroupAccept(sc, c.gw.IDSeenBy(sc, pa))
	c.gw.GroupInvite(sa, c.gw.IDSeenBy(sa, pd))
	c.gw.GroupAccept(sd, c.gw.IDSeenBy(sd, pa))
	if n := len(c.gw.GroupMembers(pa)); n != 4 {
		t.Fatalf("the group has %d, want 4", n)
	}
	before := map[*entity.Player]int{pa: pa.Tunar, pb: pb.Tunar, pc: pc.Tunar, pd: pd.Tunar}
	npc := &entity.NPC{ServerID: 5000, Name: "beetle", X: 100, Z: 100, Level: 10}
	h := &NPCWindowHandler{gameWorld: c.gw, playerOf: c.gw.SessionPlayer}
	h.giveCorpseCoins(sa, npc, 10)
	for p, want := range map[*entity.Player]int{pa: 4, pb: 3, pc: 3, pd: 0} {
		if got := p.Tunar - before[p]; got != want {
			t.Errorf("%s took %d of the corpse's 10, want %d", p.Name, got, want)
		}
	}

	// the control: alone, all of it
	se, pe := c.join(5, "Esk", 300, 300, 0)
	was := pe.Tunar
	h.giveCorpseCoins(se, &entity.NPC{ServerID: 5001, Name: "beetle", X: 300, Z: 300, Level: 10}, 7)
	if got := pe.Tunar - was; got != 7 {
		t.Errorf("one in no group took %d of 7", got)
	}
}
