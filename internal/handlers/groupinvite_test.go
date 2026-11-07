package handlers

import (
	"bytes"
	"testing"
)

// Any member of a group invites into it (the user, 2026-11-06: "lets allow
// any group member to invite to group"): Bren, a member of Alda's group,
// invites Cael, Cael accepts, and the three are one group with Alda still its
// leader. A group of four takes a fifth, whoever invites: it is a raid group
// then (entity/raid.go, raid_test.go).
func TestAnyMemberInvites(t *testing.T) {
	c := newChatWorld(t)
	sa, pa := c.join(1, "Alda", 100, 100, 0)
	sb, pb := c.join(2, "Bren", 102, 100, 0)
	sc, pc := c.join(3, "Cael", 104, 100, 0)
	c.group(t, sa, pa, sb, pb)

	c.gw.GroupInvite(sb, c.gw.IDSeenBy(sb, pc))
	c.gw.GroupAccept(sc, c.gw.IDSeenBy(sc, pb))
	names := func() []string {
		var out []string
		for _, m := range c.gw.GroupMembers(pa) {
			out = append(out, m.Name)
		}
		return out
	}
	if got := names(); len(got) != 3 || got[0] != "Alda" || got[2] != "Cael" {
		t.Fatalf("after Bren's invite: %v, want Alda (leading), Bren, Cael", got)
	}
	if pc.GroupID != pa.GroupID {
		t.Fatalf("Cael is in group %d, Alda's is %d", pc.GroupID, pa.GroupID)
	}

	// the fourth by Cael, then a fifth by Bren
	sd, pd := c.join(4, "Dirn", 106, 100, 0)
	se, pe := c.join(5, "Esk", 108, 100, 0)
	c.gw.GroupInvite(sc, c.gw.IDSeenBy(sc, pd))
	c.gw.GroupAccept(sd, c.gw.IDSeenBy(sd, pc))
	c.gw.GroupInvite(sb, c.gw.IDSeenBy(sb, pe))
	c.gw.GroupAccept(se, c.gw.IDSeenBy(se, pb))
	if got := names(); len(got) != 5 || got[0] != "Alda" || got[3] != "Dirn" || got[4] != "Esk" {
		t.Errorf("after Cael's and Bren's invites: %v, want Alda, Bren, Cael, Dirn, Esk", got)
	}
	if pe.GroupID != pa.GroupID {
		t.Errorf("Esk is in group %d, Alda's is %d", pe.GroupID, pa.GroupID)
	}
}

// The offer is 0x7E17 with a u32 1, the bytes vanilla reads.
func TestTheMemberInvitesOffer(t *testing.T) {
	if got, want := buildMemberInvitesOffer(), []byte{0x17, 0x7E, 1, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Errorf("offer % x, want % x", got, want)
	}
}
