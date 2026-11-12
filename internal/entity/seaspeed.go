package entity

// seaspeed.go — a sea folk race swims at Spirit of the Wolf's speed (the GM
// Jibrish's petition 1650, for the Mermaid race of petition 1620: "When the new
// mermaid race is added, it should detect if its in water and swim at SoW
// speed by default").
//
// A race whose own body is a sea creature's (swimmerbody.go: the Mermaid's
// merman and mermaid; it goes by the racedefs row's body, so a GM's new race on
// those bodies is one too) moves at the movement rate Spirit of the Wolf gives
// (MoveRate's 100, SpeedNow's 1.3 times the run speed) while it is in water:
// its client's walk flags say it is in a liquid or at its surface (the walk's
// own, as the breath gauge goes by; a client that has sent no flags yet, its
// swim animation). On land its own run speed. It does not stack with Spirit of
// the Wolf or a speed mastery: the rate stops at 100 as every speed does
// (MoveRate), so a mermaid with SoW on swims at SoW's speed and walks at it on
// land. The client is told the new run speed (0x00F8) when it changes, as an
// effect's start and end tell it, and the move judge (movecheck.go) reads the
// same SpeedNow. A form or an illusion changes nothing of it: it goes by the
// race, not the body worn.

// seaRace reports whether race's own body (either sex's) is a sea creature's.
func seaRace(race int) bool {
	if _, ok := customRace(race); !ok {
		return false
	}
	return swimmerBodies[uint32(raceOwnBody(race, 0))] || swimmerBodies[uint32(raceOwnBody(race, 1))]
}

// inLiquid reports whether the player's client says it is in a liquid or at
// its surface.
func (p *Player) inLiquid() bool {
	if p.HasWalkFlags {
		return p.WalkFlags&(walkFlagsSubmerged|walkFlagsAtSurface) != 0
	}
	return isSwimAnimation(p.Animation)
}

// seaSpeedLocked sets the player's water speed by where it is now, and tells
// its client the run speed when that changed.
func (w *World) seaSpeedLocked(p *Player) {
	want := p.HP > 0 && seaRace(p.Race) && p.inLiquid()
	if want == p.seaSpeed {
		return
	}
	before := p.SpeedNow()
	p.seaSpeed = want
	if after := p.SpeedNow(); after != before && w.SessionManager != nil {
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			sendPlayerSpeed(sess, after)
		}
	}
}
