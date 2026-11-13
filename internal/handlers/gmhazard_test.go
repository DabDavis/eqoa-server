package handlers

import "testing"

// Petition 1960: /hazard add places a round area where the GM stands (the thorns' rule by default,
// "still" for standing still too), audited and in the world at once; /hazard list names it;
// /hazard remove with no id takes the one holding the GM. A player's /hazard is no command.
func TestGMHazardCommand(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	stand(c, gp, 25500, 51, 15500, 0)
	c.say(ps, "/hazard add 500 6")
	if l := gmReplies(t, ps); !hasLine(l, `Unrecognized command "/hazard"`) {
		t.Fatalf("a player's: %q", l)
	}
	c.say(gs, "/hazard add 500 6 2.5 Spiky thorns")
	if l := gmReplies(t, gs); !hasLine(l, `"Spiky thorns" here, 500 damage every 6.0 s, radius 2.5, again only once moved`) {
		t.Fatalf("/hazard add: %q", l)
	}
	hs, bad, err := db.Hazards()
	if err != nil || len(bad) != 0 || len(hs) != 1 {
		t.Fatalf("rows %+v %v %v", hs, bad, err)
	}
	h := hs[0]
	if h.X1 != 25500 || h.Z1 != 15500 || h.Y1 != 51 || h.X2 != h.X1 || h.Damage != 500 || h.EveryMS != 6000 ||
		h.Radius != 2.5 || !h.OnMove || h.Source != "gm" || h.Name != "Spiky thorns" {
		t.Fatalf("the row: %+v", h)
	}
	if n := auditLines(t, db, "hazards", "insert"); n != 1 {
		t.Fatalf("%d inserts audited", n)
	}
	if s := c.gw.HazardsSummary(); s != "hazards: 1 in table hazards (world 0: 1)" {
		t.Fatalf("the world: %q", s)
	}
	c.say(gs, "/hazard add 50 2 still")
	stand(c, gp, 25600, 51, 15500, 0)
	c.say(gs, "#hazard list")
	if l := gmReplies(t, gs); !hasLine(l, "2 hazards in this world, 2 within 200:") || !hasLine(l, `"Hazard": 50 every 2.0 s, radius 1.5, standing still too`) {
		t.Fatalf("/hazard list: %q", l)
	}
	c.say(gs, "/hazard remove")
	if l := gmReplies(t, gs); !hasLine(l, "you stand in none") {
		t.Fatalf("/hazard remove off any: %q", l)
	}
	stand(c, gp, 25501, 51, 15500, 0)
	c.say(gs, "/hazard remove")
	if l := gmReplies(t, gs); !hasLine(l, "Hazard 1 removed") {
		t.Fatalf("/hazard remove: %q", l)
	}
	if n := auditLines(t, db, "hazards", "delete"); n != 1 {
		t.Fatalf("%d deletes audited", n)
	}
	c.say(gs, "/hazard add 0 6")
	c.say(gs, "/hazard add 10 0.1")
	c.say(gs, "/hazard add 10 6 500")
	if l := gmReplies(t, gs); !hasLine(l, "damage \"0\"") || !hasLine(l, "every \"0.1\"") || !hasLine(l, "radius \"500\"") {
		t.Fatalf("refusals: %q", l)
	}
	if hs, _, _ := db.Hazards(); len(hs) != 1 || hs[0].OnMove {
		t.Fatalf("rows after: %+v", hs)
	}
}
