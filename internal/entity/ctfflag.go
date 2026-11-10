package entity

import (
	"fmt"
	"log"
	"math"
	"time"
)

// ctfflag.go — capture the flag's banners and power-ups (ctf.go has the rules): each an NPC of its own (a
// Temporary NPC: no row, never kept over a restart), touched by a player's report of its place (ctfStepLocked,
// from ApplyClientUpdate after the move check, at the place the server holds) or by a talk to it (CTFTouch,
// handlers/ctf.go). A banner stands on its stand, lies where it dropped, or goes with its carrier: its NPC trails
// the carrier a step behind, moved every tick with the carrier's velocity so each client carries it along between
// updates as it does the carrier (ctfFollowLocked), as a standard-bearer's pole; the carrier's name says it to
// every client ("[Banner] Bob", ShownName), gold to the other team (ctfViewLocked), and a layout with a hand
// item (FlagHand) puts it in the carrier's off hand too (the channel's override item, SerializePlayerUpdate).

// ctfFlagLevel is a banner's and a power-up's level: what a con of it says.
const ctfFlagLevel = 1

// ctfTrail is how far behind its carrier a carried banner goes.
const ctfTrail float32 = 1.4

// CTFCarrierTag goes before a carrier's name where every client draws it (ShownName).
const CTFCarrierTag = "[Banner] "

// ctfNPCLocked makes a match's NPC: a banner or a power-up, standing at (x, y, z) facing f, of body model, the NPC
// bit alone (no one fights it, a click talks to it: the touch).
func (w *World) ctfNPCLocked(name string, model uint32, size float32, x, y, z, f float32) EntityID {
	if size <= 0 {
		size = 1
	}
	npc := &NPC{
		ServerID: w.allocFreeIDLocked(),
		Name:     name,
		X:        x, Y: y, Z: z,
		Facing:    f,
		World:     w.ctf.set.World,
		ModelID:   int64(model),
		Size:      size,
		NPCType:   0x80,
		Level:     ctfFlagLevel,
		HP:        100, MaxHP: 100,
		Temporary: true,
	}
	npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing = x, y, z, f
	w.putNPCLocked(npc)
	w.SpatialIndex.Insert(npc.ServerID, x, z)
	return npc.ServerID
}

// ctfDropNPCLocked takes a match's NPC out of the world.
func (w *World) ctfDropNPCLocked(id EntityID) {
	if npc := w.NPCs[id]; npc != nil && npc.Temporary {
		w.removeNPCLocked(npc)
	}
}

// ctfFlagNPCGoneLocked takes the banner's NPC away (going home, or the match over).
func (w *World) ctfFlagNPCGoneLocked(f *ctfFlag) {
	if f.npc != 0 {
		w.ctfDropNPCLocked(f.npc)
		f.npc = 0
	}
}

// ctfBannerLocked makes team t's banner's NPC at (x, y, z) facing face.
func (w *World) ctfBannerLocked(t int, x, y, z, face float32) EntityID {
	lay := w.ctf.lay
	body, size := lay.Teams[t].Body, lay.Teams[t].Size
	if body == 0 {
		body, size = lay.FlagBody, lay.FlagSize
	}
	return w.ctfNPCLocked(lay.Teams[t].Flag, body, size, x, y, z, face)
}

// ctfLetGoLocked is a carrier letting its banner go (dropped, captured, the match over).
func (w *World) ctfLetGoLocked(f *ctfFlag) {
	if c := w.Players[f.carrier]; c != nil {
		c.ctf.carries, c.ctf.hand = false, 0
	}
}

// ctfFlagHomeLocked puts the banner on its stand: a new NPC there (the old one gone at once, never walked across
// the field).
func (w *World) ctfFlagHomeLocked(f *ctfFlag) {
	w.ctfLetGoLocked(f)
	w.ctfFlagNPCGoneLocked(f)
	f.state, f.carrier, f.takenAt, f.wounds, f.droppedAt = ctfFlagHome, 0, time.Time{}, 0, time.Time{}
	s := w.ctf.lay.Teams[f.team].Stand
	f.x, f.y, f.z = s.X, s.Y, s.Z
	f.npc = w.ctfBannerLocked(f.team, s.X, s.Y, s.Z, s.Facing)
}

// ctfFlagDropLocked drops the banner at (x, y, z), on the floor there: its NPC stays where it trailed its carrier
// when that is near, else a new one is made there.
func (w *World) ctfFlagDropLocked(f *ctfFlag, x, y, z float32, now time.Time) {
	w.ctfLetGoLocked(f)
	if npc := w.NPCs[f.npc]; npc != nil && f.npc != 0 && distXZ(npc.X, npc.Z, x, z) <= 3*ctfTrail {
		x, y, z = npc.X, npc.Y, npc.Z
	}
	if gy, ok := w.groundFloorLocked(w.ctf.set.World, x, y+1, z); ok && y-gy < 30 && gy-y < 3 {
		y = gy // on the floor, not in the air where a falling carrier was
	}
	f.state, f.carrier, f.x, f.y, f.z, f.droppedAt, f.wounds = ctfFlagDropped, 0, x, y, z, now, 0
	if npc := w.NPCs[f.npc]; npc != nil && f.npc != 0 && npc.X == x && npc.Z == z {
		npc.Y = y
		npc.VelX, npc.VelY, npc.VelZ = 0, 0, 0
		return
	}
	w.ctfFlagNPCGoneLocked(f)
	f.npc = w.ctfBannerLocked(f.team, x, y, z, 0)
}

// ctfFollowLocked moves every carried banner's NPC behind its carrier, a tick at a time, with the carrier's
// velocity (the client carries an NPC along it between updates).
func (w *World) ctfFollowLocked(m *ctfMatch) {
	for _, f := range m.flags {
		if f == nil || f.state != ctfFlagCarried {
			continue
		}
		p, npc := w.Players[f.carrier], w.NPCs[f.npc]
		if p == nil {
			continue
		}
		if npc == nil {
			f.npc = w.ctfBannerLocked(f.team, p.X, p.Y, p.Z, p.Facing)
			npc = w.NPCs[f.npc]
		}
		h := float64(p.Facing)
		x, z := p.X-ctfTrail*float32(math.Sin(h)), p.Z-ctfTrail*float32(math.Cos(h))
		y := p.Y
		if gy, ok := w.groundFloorLocked(w.ctf.set.World, x, y+2, z); ok && y-gy < 4 && gy-y < 2 {
			y = gy
		}
		oldX, oldZ := npc.X, npc.Z
		npc.X, npc.Y, npc.Z, npc.Facing = x, y, z, p.Facing
		npc.VelX, npc.VelY, npc.VelZ = p.VelX, 0, p.VelZ
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, x, z)
		f.x, f.y, f.z = p.X, p.Y, p.Z
	}
}

// groundFloorLocked is GroundFloor under the lock.
func (w *World) groundFloorLocked(world uint8, x, y, z float32) (float32, bool) {
	if w.ground == nil {
		return y, false
	}
	return w.ground.Floor(world, x, y, z)
}

// ctfFlagOfCarrierGoneLocked drops the banner a player carried, if it carried one, where it was: it left the match,
// the arena or the world.
func (w *World) ctfFlagOfCarrierGoneLocked(m *ctfMatch, id EntityID, now time.Time) {
	for _, f := range m.flags {
		if f == nil || f.state != ctfFlagCarried || f.carrier != id {
			continue
		}
		x, y, z := f.x, f.y, f.z
		name := "its carrier"
		if p := w.Players[id]; p != nil {
			name = p.Name
			if p.World == w.ctf.set.World {
				x, y, z = p.X, p.Y, p.Z
			}
		}
		w.ctfFlagDropLocked(f, x, y, z, now)
		log.Printf("CTF: %s is gone with %s: dropped at (%.1f, %.1f, %.1f)", name, w.ctf.lay.Teams[f.team].Flag, f.x, f.y, f.z)
		w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineDropped, name, w.ctfBannerWordsLocked(f.team)), ctfGold)
	}
}

// ctfNear reports (px, py, pz) touching (x, y, z): within reach across, ctfTouchBelow under it to ctfTouchAbove
// over it.
func ctfNear(px, py, pz, x, y, z, reach float32) bool {
	if dy := py - y; dy < -ctfTouchBelow || dy > ctfTouchAbove {
		return false
	}
	return distXZ(px, pz, x, z) <= reach
}

// ctfStepLocked is capture the flag's look at one report of a player's place (ApplyClientUpdate's, after the move
// check): the field's edge, a power-up, a banner or a stand touched. Must be called with the lock held.
func (w *World) ctfStepLocked(p *Player, now time.Time) {
	m := w.ctf.m
	if !w.ctf.on || m == nil || !p.inMatch(m) || p.World != w.ctf.set.World || p.IsDead() || p.respawnPending() {
		return
	}
	if len(w.ctf.lay.Field) >= 3 && !ctfInside(w.ctf.lay.Field, p.X, p.Z) {
		if p.ctf.carries {
			for _, f := range m.flags {
				if f != nil && f.state == ctfFlagCarried && f.carrier == p.ServerID {
					w.ctfFlagDropLocked(f, p.X, p.Y, p.Z, now)
					w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineDropped, p.Name, w.ctfBannerWordsLocked(f.team)), ctfGold)
				}
			}
		}
		log.Printf("CTF: %s strays past the field at (%.1f, %.1f): back to camp", p.Name, p.X, p.Z)
		w.ctfTellLocked(p, ctfLineField, ctfBlue)
		w.ctfToCampLocked(p, 1)
		return
	}
	if m.phase != ctfPlay || !ctfMayTouch(p, now) {
		return
	}
	w.ctfTouchLocked(m, p, p.X, p.Y, p.Z, now)
}

// ctfMayTouch reports p's place one to touch by: not while the move check holds a report of the client's as from
// before a server's move (movecheck.go's pending: in log mode it is taken, so a client's last report from where it
// fell, sent before it heard of its rise at its camp, would put it back on the banner it died by), nor for
// ctfQuietAfterMove after the server put it somewhere (the same with the move check off).
func ctfMayTouch(p *Player, now time.Time) bool {
	return !p.mv.pending && !now.Before(p.ctf.quiet)
}

// ctfTouchAllLocked is every living player of the match touching what it stands at, where the server holds it:
// a client standing still sends no reports (CLIENT's 0x40 goes only with a move), and a server's move (a GM's,
// a camp's) puts a player somewhere with none from there yet.
func (w *World) ctfTouchAllLocked(m *ctfMatch, now time.Time) {
	for t := range m.teams {
		for _, id := range m.teams[t].members {
			p := w.Players[id]
			if p == nil || !p.inMatch(m) || p.World != w.ctf.set.World || p.IsDead() || p.respawnPending() || !ctfMayTouch(p, now) {
				continue
			}
			w.ctfTouchLocked(m, p, p.X, p.Y, p.Z, now)
		}
	}
}

// ctfTouchLocked is p touching what is at (x, y, z): a power-up, a banner (taken, taken up, or returned), its own
// stand with the other banner (a capture).
func (w *World) ctfTouchLocked(m *ctfMatch, p *Player, x, y, z float32, now time.Time) {
	reach := max(w.ctf.set.TouchReach, 1)
	for _, u := range m.powerUps {
		if u.npc != 0 && ctfNear(x, y, z, u.lay.At.X, u.lay.At.Y, u.lay.At.Z, reach) {
			w.ctfTakePowerUpLocked(m, u, p, now)
		}
	}
	lay := w.ctf.lay
	own, other := m.flags[p.ctf.team], m.flags[1-p.ctf.team]
	// the other team's banner: on its stand or where it lies
	if other != nil && !p.ctf.carries {
		switch other.state {
		case ctfFlagHome:
			s := lay.Teams[other.team].Stand
			if ctfNear(x, y, z, s.X, s.Y, s.Z, reach) {
				w.ctfTakeLocked(m, other, p, false, now)
			}
		case ctfFlagDropped:
			if ctfNear(x, y, z, other.x, other.y, other.z, reach) {
				w.ctfTakeLocked(m, other, p, true, now)
			}
		}
	}
	// its own banner lying in the field: returned
	if own != nil && own.state == ctfFlagDropped && ctfNear(x, y, z, own.x, own.y, own.z, reach) {
		w.ctfFlagHomeLocked(own)
		p.ctf.returns++
		log.Printf("CTF: %s returns %s", p.Name, lay.Teams[own.team].Flag)
		w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineReturned, p.Name, w.ctfBannerWordsLocked(own.team)), ctfGold)
	}
	// a carrier at its own stand
	if other != nil && other.state == ctfFlagCarried && other.carrier == p.ServerID {
		s := lay.Teams[p.ctf.team].Stand
		if !ctfNear(x, y, z, s.X, s.Y, s.Z, reach) {
			return
		}
		if own == nil || own.state != ctfFlagHome {
			if now.Sub(p.ctf.saidAt) >= ctfSayEvery {
				p.ctf.saidAt = now
				w.ctfTellLocked(p, ctfLineOwnAway, ctfGold)
			}
			return
		}
		w.ctfCaptureLocked(m, other, p, now)
	}
}

// ctfTakeLocked is p taking the other team's banner (up from where it lay, with up).
func (w *World) ctfTakeLocked(m *ctfMatch, f *ctfFlag, p *Player, up bool, now time.Time) {
	lay := w.ctf.lay
	if !up {
		w.ctfFlagNPCGoneLocked(f) // off its stand: made again behind its carrier (ctfFollowLocked)
	}
	f.state, f.carrier, f.takenAt, f.wounds, f.droppedAt = ctfFlagCarried, p.ServerID, now, 0, time.Time{}
	f.x, f.y, f.z = p.X, p.Y, p.Z
	p.ctf.carries, p.ctf.hand = true, lay.FlagHand
	w.ctfFollowLocked(m)
	w.breakInvisLocked(p, "a banner") // invis.go: a carrier is seen
	line := ctfLineTaken
	if up {
		line = ctfLineTakenUp
	}
	log.Printf("CTF: %s takes %s (up from the field: %v)", p.Name, lay.Teams[f.team].Flag, up)
	w.ctfTellMatchLocked(m, fmt.Sprintf(line, p.Name, w.ctfBannerWordsLocked(f.team)), ctfGold)
}

// ctfCaptureLocked is p scoring with f, the other team's banner, at its own stand.
func (w *World) ctfCaptureLocked(m *ctfMatch, f *ctfFlag, p *Player, now time.Time) {
	lay := w.ctf.lay
	t := p.ctf.team
	m.teams[t].score++
	p.ctf.caps++
	w.ctfFlagHomeLocked(f)
	score := w.ctfScoreLocked(m)
	log.Printf("CTF: %s captures %s for %s: %s", p.Name, lay.Teams[f.team].Flag, lay.Teams[t].Short, score)
	w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineCaptured, p.Name, w.ctfBannerWordsLocked(f.team), lay.Teams[t].Name, score+"."), ctfGold)
	if m.teams[t].score >= max(w.ctf.set.ScoreLimit, 1) {
		w.ctfEndLocked(t, "score", now)
	}
}

// ctfFlagsAloneLocked sends home a banner left lying FlagReturn.
func (w *World) ctfFlagsAloneLocked(m *ctfMatch, now time.Time) {
	for _, f := range m.flags {
		if f == nil || f.state != ctfFlagDropped || now.Sub(f.droppedAt) < w.ctf.set.FlagReturn {
			continue
		}
		w.ctfFlagHomeLocked(f)
		log.Printf("CTF: %s left alone: back on its stand", w.ctf.lay.Teams[f.team].Flag)
		w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineHomeAlone, upFirstWord(w.ctfBannerWordsLocked(f.team))), ctfGold)
	}
}

// ctfWoundPct is how much more a carrier takes from every hit, in percent, having held a banner for held.
func ctfWoundPct(set CTFSettings, held time.Duration) int {
	if set.WoundAfter <= 0 || held < set.WoundAfter || set.WoundStep <= 0 {
		return 0
	}
	steps := 1 + int((held-set.WoundAfter)/time.Minute)
	return min(steps*set.WoundStep, max(set.WoundMax, 0))
}

// ctfWoundsLocked tells the match each step of a carrier's wounds.
func (w *World) ctfWoundsLocked(m *ctfMatch, now time.Time) {
	for _, f := range m.flags {
		if f == nil || f.state != ctfFlagCarried {
			continue
		}
		pct := ctfWoundPct(w.ctf.set, now.Sub(f.takenAt))
		if pct <= f.wounds {
			continue
		}
		f.wounds = pct
		if p := w.Players[f.carrier]; p != nil {
			log.Printf("CTF: %s has carried %s %v: +%d%% from every hit", p.Name, w.ctf.lay.Teams[f.team].Flag, now.Sub(f.takenAt).Round(time.Second), pct)
			w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineWound, p.Name, w.ctfBannerWordsLocked(f.team), pct), ctfGold)
		}
	}
}

// ctfHurtLocked is a PvP hit of dmg on t (duelHurtLocked's, after the share): more on a wounded carrier.
func (w *World) ctfHurtLocked(t *Player, dmg int) int {
	m := w.ctf.m
	if !t.inMatch(m) || !t.ctf.carries {
		return dmg
	}
	for _, f := range m.flags {
		if f != nil && f.state == ctfFlagCarried && f.carrier == t.ServerID && f.wounds > 0 {
			return dmg + int(math.Round(float64(dmg)*float64(f.wounds)/100))
		}
	}
	return dmg
}

// ctfDownLocked is a match's player dying (playerDownLocked): its banner drops where it fell, its death counted.
func (w *World) ctfDownLocked(p *Player, now time.Time) {
	m := w.ctf.m
	if !p.inMatch(m) {
		return
	}
	p.ctf.deaths++
	for _, f := range m.flags {
		if f == nil || f.state != ctfFlagCarried || f.carrier != p.ServerID {
			continue
		}
		w.ctfFlagDropLocked(f, p.X, p.Y, p.Z, now)
		log.Printf("CTF: %s falls with %s: dropped at (%.1f, %.1f, %.1f)", p.Name, w.ctf.lay.Teams[f.team].Flag, f.x, f.y, f.z)
		w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineDropped, p.Name, w.ctfBannerWordsLocked(f.team)), ctfGold)
	}
	if m.phase == ctfPlay {
		wave := w.ctfWaveForLocked(m, now)
		w.ctfTellLocked(p, fmt.Sprintf(ctfLineWaveSoon, int(math.Ceil(wave.Sub(now).Seconds()))), ctfBlue)
	}
}

// ctfKillLocked counts a match's kill (pvpDeathLocked's).
func (w *World) ctfKillLocked(killer, t *Player) {
	if m := w.ctf.m; killer.inMatch(m) && t.inMatch(m) {
		killer.ctf.kills++
	}
}

// ctfPowerUpOutLocked puts a power-up in the field.
func (w *World) ctfPowerUpOutLocked(u *ctfPowerUp) {
	lay := w.ctf.lay
	body := lay.HealBody
	if u.lay.Kind == CTFSpeed {
		body = lay.SpeedBody
	}
	a := u.lay.At
	u.npc = w.ctfNPCLocked(u.lay.Name, body, 1, a.X, a.Y, a.Z, a.Facing)
	u.backAt = time.Time{}
}

// ctfPowerUpGoneLocked takes a power-up out of the field.
func (w *World) ctfPowerUpGoneLocked(u *ctfPowerUp) {
	if u.npc != 0 {
		w.ctfDropNPCLocked(u.npc)
		u.npc = 0
	}
}

// ctfPowerUpsLocked brings back the power-ups whose time has come.
func (w *World) ctfPowerUpsLocked(m *ctfMatch, now time.Time) {
	for _, u := range m.powerUps {
		if u.npc == 0 && !u.backAt.IsZero() && !now.Before(u.backAt) {
			w.ctfPowerUpOutLocked(u)
		}
	}
}

// ctfTakePowerUpLocked is p touching a power-up: its gift, and the power-up gone for PowerUpEvery.
func (w *World) ctfTakePowerUpLocked(m *ctfMatch, u *ctfPowerUp, p *Player, now time.Time) {
	w.ctfPowerUpGoneLocked(u)
	u.backAt = now.Add(w.ctf.set.PowerUpEvery)
	switch u.lay.Kind {
	case CTFSpeed:
		// Spirit of the Wolf's run speed (boardwolf.go's effect, under a key of its own) for SpeedFor
		w.addEffectLocked(p, &ActiveEffect{SpellID: CTFSpeedEffectID, Name: u.lay.Name, Icon: BoardWolfIcon,
			Caster: p.ServerID, Until: now.Add(w.ctf.set.SpeedFor), Speed: true}, now)
		w.ctfTellLocked(p, fmt.Sprintf(ctfLineSpeed, u.lay.Name), ctfBlue)
	default:
		p.HP, p.Power = p.MaxHPNow(), p.MaxPowerNow()
		p.StatsDirty = true
		w.ctfTellLocked(p, fmt.Sprintf(ctfLineHeal, u.lay.Name), ctfBlue)
	}
	log.Printf("CTF: %s takes %s (%s); back in %v", p.Name, u.lay.Name, u.lay.Kind, w.ctf.set.PowerUpEvery)
}

// CTFSpeedEffectID keys the speed power-up among a player's effects (the boards' Spirit of the Wolf is -1004).
const CTFSpeedEffectID = -1010

// CTFTouch is a talk to a match's NPC (a banner, a power-up), the same as touching it: false when id is none of
// them (the talk goes on as any other's).
func (w *World) CTFTouch(playerID, npcID EntityID, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	m := w.ctf.m
	if m == nil {
		return false
	}
	npc := w.NPCs[npcID]
	if npc == nil {
		return false
	}
	ours := false
	for _, f := range m.flags {
		ours = ours || (f != nil && f.npc == npcID)
	}
	for _, u := range m.powerUps {
		ours = ours || u.npc == npcID
	}
	if !ours {
		return false
	}
	p := w.Players[playerID]
	if p == nil || !p.inMatch(m) || m.phase != ctfPlay || p.IsDead() || p.respawnPending() {
		return true
	}
	if distXZ(p.X, p.Z, npc.X, npc.Z) <= InteractRange {
		w.ctfTouchLocked(m, p, npc.X, npc.Y, npc.Z, now)
	}
	return true
}

// ctfInside reports (x, z) inside the polygon (even-odd).
func ctfInside(poly [][2]float32, x, z float32) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a[1] > z) != (b[1] > z) && x < (b[0]-a[0])*(z-a[1])/(b[1]-a[1])+a[0] {
			in = !in
		}
	}
	return in
}

// ctfBannerWordsLocked is team t's banner in a sentence: "the Ember Banner".
func (w *World) ctfBannerWordsLocked(t int) string {
	return "the " + w.ctf.lay.Teams[t].Flag
}
