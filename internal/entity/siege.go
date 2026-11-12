package entity

import (
	"fmt"
	"log"
	"math"
	"time"
)

// siege.go — the siege of Zentar's Keep (the owner, 2026-11-12: "lets add a pvp zone for pvp events. zentaurs keep
// castle, it is siege mode, team inside has king with high hp. entrances have a door/wood prop as a mob that doesnt
// attack with high hp, when dead way is opened for attacking team to kill king, when king dies the attackers win").
// The siege is the second arena game (ctfgames.go): the engine of capture the flag (ctf.go) runs it, its queue,
// countdown, split, camps, prep, waves, backfill, forfeit, time limit, end and way back the same, in its own world
// (world.siege; tools/worlds/siege.json, the castle of Zentars Keep composed where it stands). What it fights over
// is this file's (claude-notes/siege-world-20261112.md has the rules whole, with every number):
//
//   - Two teams: the defenders (team 0, SiegeDefenders) rise in the castle's courtyard, the attackers (team 1,
//     SiegeAttackers) outside its east gatehouse. The split is the engine's, as even as it comes.
//   - The King: a copy of the spawn-only NPC its layout names (tools/siege_20261112.py's "King Zentar", Zentar
//     Reese's look), standing in the courtyard with KingHP hit points. Only the attackers may hurt him, and only
//     once the siege is live; he fights back at the attackers who strike him (a level-60 fighter's melee), notices
//     no one on sight, and heals from nothing but the defenders' spells (their beneficial spells reach him as a
//     pet's do; no regeneration of his own). His death wins the siege for the attackers.
//   - The gates: a copy of each gate's spawn-only NPC (the castle's own portcullis, 0xAA243482, let down into its
//     doorway: the gatehouse's outer door and its inner door to the courtyard), GateHP hit points each. A gate
//     attacks no one and holds no hate; only the attackers may hurt it. Its body is the doorway's collision (a
//     SimpleSprite with its own collision buffer is as solid as the zone's walls: vanilla's place_actors, the
//     invisible blockers' note), so no one passes a standing gate. A broken gate leaves the world at the next tick
//     (a corpse of a SimpleSprite would still be solid), and the way stays open until the siege ends.
//   - The end: the King's death (the attackers win), the time limit (the defenders win), a team empty 30 s (a
//     forfeit), a GM's #siege stop (no victor). Everyone is told, the server too, each player its own count (gates
//     broken, kills, deaths, the King's last blow), and EndHold later all go back beside the herald.
//
// Neither King nor gate gives experience, coins, loot or faction. Every rule that keeps a Metal Moogie his
// challenger's alone (metalmoogie.go's challengeBars) keeps these the attackers': anyone else's melee is not
// engaged, a harmful spell refused, an area spell passes them by, a pet will not go for them, and every client but
// an attacker's in a live siege sees them with the NPC bit (CLIENT will not swing at them). The words are ours.

// The arena layout's games (CTFLayout.Game).
const (
	ArenaCTF   = "ctf"
	ArenaSiege = "siege"
)

// The siege's teams: the defenders hold the castle, the attackers come at it.
const (
	SiegeDefenders = 0
	SiegeAttackers = 1
)

// DefaultSiegeSettings are the siege's numbers where a server sets none (config world.siege; the note's table):
// capture the flag's engine numbers, its world 15, and the King's and the gates' hit points.
func DefaultSiegeSettings() CTFSettings {
	s := DefaultCTFSettings()
	s.World = 15
	s.ScoreLimit = 0
	s.WoundAfter = 0
	s.KingHP = 100000
	s.GateHP = 30000
	return s
}

// SiegeNPCLayout is the King or a gate as the recipe has it: its name, the spawn-only NPC it is a copy of (its look,
// level and size), where it stands, and the body it is made with when no such NPC is in the world.
type SiegeNPCLayout struct {
	Name     string
	Original string
	At       CTFSpot
	Body     uint32
	Size     float32
	Level    int
}

// SiegeLayout is the siege's own places: the King and the gates, in the order they are met.
type SiegeLayout struct {
	King  SiegeNPCLayout
	Gates []SiegeNPCLayout
}

// siegeGate is a gate in a siege.
type siegeGate struct {
	lay  SiegeNPCLayout
	npc  EntityID
	down bool      // broken
	gone bool      // its NPC taken out of the world, its fall told
	by   string    // who struck its last blow
	at   time.Time // when
}

// siegeMatch is a siege's King and gates.
type siegeMatch struct {
	king     EntityID
	kingDown bool
	kingBy   string
	gates    []*siegeGate
}

// npcSiege marks an NPC a siege's: its match, and the King or a gate (gate its index).
type npcSiege struct {
	m    *ctfMatch
	king bool
	gate int
}

// memberOf reports id among ids.
func memberOf(ids []EntityID, id EntityID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// mayHurt reports id one who may strike it: an attacker of its match while the siege is live.
func (s *npcSiege) mayHurt(id EntityID) bool {
	return s != nil && s.m != nil && s.m.phase == ctfPlay && memberOf(s.m.teams[SiegeAttackers].members, id)
}

// defends reports id a defender of its match.
func (s *npcSiege) defends(id EntityID) bool {
	return s != nil && s.m != nil && s.m.phase != ctfOver && memberOf(s.m.teams[SiegeDefenders].members, id)
}

// SiegeSpawn reports a siege's King or gate: nothing to loot, no coins (handlers' loot and corpse coins).
func (n *NPC) SiegeSpawn() bool { return n != nil && n.siege != nil }

// The siege's lines (ours).
const (
	siegeLineTeamDefend = "You hold %s with %s: keep %s alive until the time runs out. %s must break %s to reach him."
	siegeLineTeamAttack = "You fight with %s: break %s, then slay %s before the time runs out."
	siegeLineGo         = "The siege is live! %s have %d minutes to break the gates and slay %s."
	siegeLineGateDown   = "%s has broken %s! The way is open."
	siegeLineGateDownN  = "%s has fallen! The way is open."
	siegeLineKingSlain  = "%s has slain %s: %s take %s!"
	siegeLineKingDies   = "%s has fallen: %s take %s!"
	siegeLineHeld       = "%s holds %s: %s lives!"
	siegeLineForfeit    = "%s has no one left: %s win the siege of %s."
	siegeLineNoVictor   = "The siege of %s ends with no victor."
	siegeLineYou        = "Your siege: %d %s broken, %d kills, %d deaths."
	siegeLineYouKing    = "You struck %s's last blow."
	siegeLineNotLive    = "The siege has not begun yet."
	siegeLineDefend     = "You defend %s: only the attackers strike at it."
	siegeLineNotYours   = "%s is the siege's: only its attackers may strike at it."
	siegeScoreFmt       = "%d of %d gates stand, %s at %d%%"
)

// siegeWords are the siege's lines where the engine has a game's own (ctf.go's ctfWordSet).
var siegeWords = ctfWordSet{
	event:   "the siege",
	closed:  "The siege is closed on this server.",
	inMatch: "You are already fighting in the siege.",
	joined:  "You are signed up for the siege. %s",
	prep:    "The siege begins in %d seconds.",
	signAll: "A siege begins in %d seconds at %s. Talk to the PvP Events herald beside Dabrish to join.",
	notLive: siegeLineNotLive,
}

// siegeGateWords is the gates in a sentence: "the Outer Gate and the Inner Gate".
func siegeGateWords(gates []SiegeNPCLayout) string {
	names := make([]string, 0, len(gates))
	for _, g := range gates {
		names = append(names, "the "+g.Name)
	}
	switch len(names) {
	case 0:
		return "the gates"
	case 1:
		return names[0]
	}
	out := names[0]
	for i := 1; i < len(names)-1; i++ {
		out += ", " + names[i]
	}
	return out + " and " + names[len(names)-1]
}

// siegeTeamLineLocked is what a player of team t is told at its sending in.
func (w *World) siegeTeamLineLocked(t int) string {
	lay := w.ctf.lay
	king, gates := lay.Siege.King.Name, siegeGateWords(lay.Siege.Gates)
	if t == SiegeDefenders {
		return fmt.Sprintf(siegeLineTeamDefend, lay.Name, lay.Teams[SiegeDefenders].Name, king,
			upFirstWord(lay.Teams[SiegeAttackers].Name), gates)
	}
	return fmt.Sprintf(siegeLineTeamAttack, lay.Teams[SiegeAttackers].Name, gates, king)
}

// siegeScoreLocked is how a siege stands as a line: "2 of 2 gates stand, King Zentar at 100%".
func (w *World) siegeScoreLocked(m *ctfMatch) string {
	sm := m.siege
	if sm == nil {
		return ""
	}
	stand := 0
	for _, g := range sm.gates {
		if !g.down {
			stand++
		}
	}
	pct := 0
	if k := w.NPCs[sm.king]; k != nil && !k.Dead && k.MaxHP > 0 {
		pct = int(math.Ceil(100 * float64(k.HP) / float64(k.MaxHP)))
	}
	return fmt.Sprintf(siegeScoreFmt, stand, len(sm.gates), w.ctf.lay.Siege.King.Name, pct)
}

// siegeNPCLocked makes one of the siege's NPCs from its layout: a copy of its original (spawn-only, its look,
// level and size) where the world has one, else of the layout's body; hp its hit points (0: the original's own).
func (w *World) siegeNPCLocked(m *ctfMatch, lay SiegeNPCLayout, hp int, king bool, gate int) *NPC {
	at, world := lay.At, w.ctf.set.World
	var npc *NPC
	if t := w.npcByNameLocked(lay.Original); lay.Original != "" && t != nil {
		npc = w.spawnCopyLocked(t, at.X, at.Y, at.Z, at.Facing, world)
	} else {
		if lay.Original != "" {
			log.Printf("Siege: no NPC %q to copy for %s: made of the recipe's body %08X", lay.Original, lay.Name, lay.Body)
		}
		id := w.ctfNPCLocked(lay.Name, lay.Body, lay.Size, at.X, at.Y, at.Z, at.Facing)
		npc = w.NPCs[id]
		npc.NPCType = 0 // a creature to fight, not the NPC bit's
		npc.Level = max(lay.Level, 1)
		npc.MaxHP = w.balance.NPCMaxHP(npc.Level)
	}
	npc.Name = lay.Name
	if hp > 0 {
		npc.MaxHP = hp
	}
	npc.HP = npc.MaxHP
	npc.siege = &npcSiege{m: m, king: king, gate: gate}
	return npc
}

// siegeStartLocked puts a siege's King and gates out, held until the siege is live.
func (w *World) siegeStartLocked(m *ctfMatch, now time.Time) {
	lay, set := w.ctf.lay.Siege, w.ctf.set
	sm := &siegeMatch{}
	m.siege = sm
	sm.king = w.siegeNPCLocked(m, lay.King, set.KingHP, true, -1).ServerID
	for i, g := range lay.Gates {
		npc := w.siegeNPCLocked(m, g, set.GateHP, false, i)
		sm.gates = append(sm.gates, &siegeGate{lay: g, npc: npc.ServerID})
	}
	k := w.NPCs[sm.king]
	log.Printf("Siege: %s stands with %d hit points (body %08X), %d gates of %d each", k.Name, k.MaxHP, uint32(k.ModelID),
		len(sm.gates), set.GateHP)
}

// siegeOpenLocked makes a siege live: told to its players.
func (w *World) siegeOpenLocked(m *ctfMatch) {
	lay := w.ctf.lay
	w.ctfTellMatchLocked(m, fmt.Sprintf(siegeLineGo, upFirstWord(lay.Teams[SiegeAttackers].Name),
		int(math.Round(w.ctf.set.TimeLimit.Minutes())), lay.Siege.King.Name), ctfGold)
}

// siegeNPCDownLocked is a siege's NPC dying (killNPC's, before its hate is let go): a gate broken or the King
// slain, the last blow its highest hater's among the players. The tick takes it from there (siegeTickLocked): the
// NPC is still in its killer's hands now. Must be called with the lock held.
func (w *World) siegeNPCDownLocked(npc *NPC) {
	s := npc.siege
	if s == nil || s.m == nil || s.m.siege == nil || s.m.phase == ctfOver {
		return
	}
	var top *Player
	var best int64 = -1
	for id, t := range npc.Threats {
		if p := w.Players[id]; p != nil && p.inMatch(s.m) && (t > best || t == best && top != nil && p.ServerID < top.ServerID) {
			top, best = p, t
		}
	}
	by := ""
	if top != nil {
		by = top.Name
	}
	sm := s.m.siege
	if s.king {
		sm.kingDown, sm.kingBy = true, by
		if top != nil {
			top.ctf.king++
		}
		log.Printf("Siege: %s is slain (the last blow %s's)", npc.Name, by)
		return
	}
	if s.gate >= 0 && s.gate < len(sm.gates) {
		g := sm.gates[s.gate]
		g.down, g.by, g.at = true, by, time.Now()
		if top != nil {
			top.ctf.gates++
		}
		log.Printf("Siege: %s is broken (the last blow %s's)", npc.Name, by)
	}
}

// siegeTickLocked is a live siege's tick: a broken gate out of the world and its fall told, the King's death the
// attackers' win, a King gone some other way (a GM's despawn) the end with no victor.
func (w *World) siegeTickLocked(m *ctfMatch, now time.Time) {
	sm := m.siege
	if sm == nil {
		return
	}
	for _, g := range sm.gates {
		if !g.down || g.gone {
			continue
		}
		g.gone = true
		w.siegeDropLocked(g.npc)
		line := fmt.Sprintf(siegeLineGateDownN, "The "+g.lay.Name)
		if g.by != "" {
			line = fmt.Sprintf(siegeLineGateDown, g.by, "the "+g.lay.Name)
		}
		w.ctfTellMatchLocked(m, line, ctfGold)
	}
	if sm.kingDown {
		w.ctfEndLocked(SiegeAttackers, "king", now)
		return
	}
	if k := w.NPCs[sm.king]; k == nil {
		w.ctfEndLocked(-1, "the King is gone", now)
	}
}

// siegeDropLocked takes a siege's NPC out of the world (a corpse too).
func (w *World) siegeDropLocked(id EntityID) {
	if npc := w.NPCs[id]; npc != nil && npc.siege != nil {
		w.removeNPCLocked(npc)
	}
}

// siegeEndLocked is a siege's end (ctfEndLocked's, past what both games do): the result told to everyone, each
// player its count, the gates and a living King taken in (a slain King lies until everyone goes back).
func (w *World) siegeEndLocked(m *ctfMatch, winner int, how string) {
	lay := w.ctf.lay
	sm := m.siege
	king := lay.Siege.King.Name
	att, def := lay.Teams[SiegeAttackers], lay.Teams[SiegeDefenders]
	var line string
	switch {
	case how == "forfeit" && winner >= 0:
		line = fmt.Sprintf(siegeLineForfeit, upFirstWord(lay.Teams[1-winner].Name), lay.Teams[winner].Name, lay.Name)
	case winner == SiegeAttackers && sm != nil && sm.kingBy != "":
		line = fmt.Sprintf(siegeLineKingSlain, sm.kingBy, king, att.Name, lay.Name)
	case winner == SiegeAttackers:
		line = fmt.Sprintf(siegeLineKingDies, king, att.Name, lay.Name)
	case winner == SiegeDefenders:
		line = fmt.Sprintf(siegeLineHeld, upFirstWord(def.Name), lay.Name, king)
	default:
		line = fmt.Sprintf(siegeLineNoVictor, lay.Name)
	}
	log.Printf("Siege: match %d ends (%s): %s", m.id, how, line)
	w.ctfAnnounceLocked(line, ctfGold, true)
	gates := "gates"
	for t := range m.teams {
		for _, id := range m.teams[t].members {
			p := w.Players[id]
			if p == nil {
				continue
			}
			if p.ctf.gates == 1 {
				gates = "gate"
			} else {
				gates = "gates"
			}
			w.ctfTellLocked(p, fmt.Sprintf(siegeLineYou, p.ctf.gates, gates, p.ctf.kills, p.ctf.deaths), ctfBlue)
			if p.ctf.king > 0 {
				w.ctfTellLocked(p, fmt.Sprintf(siegeLineYouKing, king), ctfBlue)
			}
		}
	}
	if sm == nil {
		return
	}
	for _, g := range sm.gates {
		g.gone = true
		w.siegeDropLocked(g.npc)
	}
	if k := w.NPCs[sm.king]; k != nil && !k.Dead {
		w.siegeDropLocked(sm.king)
	}
}

// siegeClearLocked takes in whatever is left of a siege's NPCs (the slain King's corpse) as everyone goes back.
func (w *World) siegeClearLocked(m *ctfMatch) {
	if m == nil || m.siege == nil {
		return
	}
	for _, g := range m.siege.gates {
		w.siegeDropLocked(g.npc)
	}
	w.siegeDropLocked(m.siege.king)
}

// siegeBars reports id barred from hurting npc, a siege's: anyone but an attacker of its live siege
// (challengeBars's).
func siegeBars(npc *NPC, id EntityID) bool {
	return npc != nil && npc.siege != nil && !npc.siege.mayHurt(id)
}

// siegeRefusal is the line for one barred from a siege's NPC, "" for none.
func siegeRefusal(npc *NPC, id EntityID) string {
	s := npc.siege
	switch {
	case s == nil || s.mayHurt(id):
		return ""
	case s.m != nil && s.m.phase == ctfPrep && memberOf(s.m.teams[SiegeAttackers].members, id):
		return siegeLineNotLive
	case s.defends(id):
		return fmt.Sprintf(siegeLineDefend, npc.Name)
	}
	return fmt.Sprintf(siegeLineNotYours, npc.Name)
}

// siegeHelps reports id one whose beneficial spells reach npc, a siege's King: a defender of its siege.
func siegeHelps(npc *NPC, id EntityID) bool {
	return npc != nil && npc.siege != nil && npc.siege.king && npc.siege.defends(id)
}

// siegeViewLocked shows a siege's NPC to sess: with the NPC bit (byte 146 0x80, byte 143 0, as an NPC no one may
// attack is sent: CLIENT swings at no such creature) to everyone but an attacker of its live siege. Must be called
// with the lock held.
func (w *World) siegeViewLocked(viewer *Player, npc *NPC, buf []byte) {
	if npc.siege == nil || npc.Dead || len(buf) <= 146 {
		return
	}
	if viewer != nil && npc.siege.mayHurt(viewer.ServerID) {
		return
	}
	buf[146] |= 0x80
	buf[143] = 0
}
