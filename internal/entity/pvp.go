package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// pvp.go — the PvP flag, a server's own switch (the user, 2026-11-07: "for heirloom server
// lets add a /pvp command to allow players to be attackable. players types /pvp for 1min
// cant take off flag until no pvp attacks for 1min. players with the flag on and be attacked
// by other players and attack other players. for now in any area.", then "for pvp on players
// add an inidicator next to their name like the red face icon").
//
// world.pvp turns it on (SetPvP); off, /pvp says so and no flag can go up, so nothing below
// ever runs. /pvp (or /t pvp, as a PS2 keeps an unknown /command to itself: raidflag.go) puts
// the flag up, and takes it down once PvPHoldTime has passed since it went up and since the
// player's last PvP attack, given or taken. It ends at a logout (it is in no row) and comes
// through a takeover (the same Player) and a keep restart (keep.go), its time with it.
//
// Two flagged players in the same world and not in one group are foes (pvpFoe), everywhere,
// towns too. Foes fight by a duel's own code: a PvP fight is a Duel in no table (pvpFightLocked),
// made for the moment, so its melee, spells, procs, shields and pets are duelRoundLocked,
// duelLandLocked, duelprocs.go and duelPetRoundLocked, at PvPDamageShare. It differs from a duel in
// that it needs no asking, nobody forfeits, and a hit the victim cannot take is a death
// (pvpDeathLocked, the one place that says what a PvP death costs). It is nothing to a creature:
// no threat, no faction, no guard's notice.
//
// What a client sees, the same on a PS2 and on vanilla, with no client change:
//
//   - The face in the target window: 0x0010's disposition 0 (DrawTarget's icon 115, the red face
//     a hostile creature gets) for a flagged player, whoever looks; a player's blue otherwise
//     (PlayerFace). CLIENT reads it only at 0x0010, and Msg0010 asks only that its token match
//     (m_p2BB68, which nothing sets but to 0), so a flag's change sends it to everyone targeting
//     that player (TargetersOf; handlers/pvp.go) with no re-target.
//   - The name over the head: channel byte 157 (VICreatureEntity m_p299, bit 0: ApplyLook's colour
//     2, NameColors[2]) green, from SerializePlayerUpdate, for every viewer. A foe stays green
//     because of the next line.
//   - A flagged player falls where it dies, for everyone (pvpViewLocked), and is made anew on every
//     client at its return (pvpRiseLocked).
//   - A foe can be swung at: TargetInRange refuses a player whose target's m_p256 (channel byte 142)
//     is 2 or 3, so a foe's channel says pvpFoeByte to that viewer alone (pvpViewLocked). The duels'
//     0 would do for the swing, but ApplyLook draws anything under 2 in the NPCs' gold. To CLIENT
//     (decomp, all of m_p256's readers) and vanilla (play.rs, the same tests) 4 is 0 but for the name:
//     selectable by byte 26 (VIEntity::IsSelectable), Tab finds it, the swing and the 0x000F go, □
//     talks (0x0004, nothing to say) where a player's info window would open, and RequestTrade and
//     InviteEntity refuse it (word 1015), as a duel opponent's are. A foe can still be invited from
//     the Who list (InviteToGroup, by its record), or by /raid.
//
// A foe's pets are fought as the foe is (the owner, 2026-11-18, on Heirloom with a player: "[a player]
// and I are Pvp flagged but pets dont get flagged so i cant attack"): everything a player owns that
// fights for it (its pet, a charmed one, the animal beside it, its mercs, their pets: pvpFoePetLocked)
// is a foe's pet to that player while their owners are foes, and nothing to anyone else. Its melee,
// spells, procs and area splash land on it through the duel's pet paths (duelRoundLocked,
// duelLandPetLocked, duelHurtPetLocked) at PvPDamageShare, every hit holding both flags; a hit it cannot
// take kills it, a pet's death (pvpPetDeathLocked: no debt, no experience, no threat, faction or
// guard). To that foe alone its channel's byte 142 is pvpFoeByte and its npc flags' 0x80 (a charmed
// creature's: ToggleAutoAttack's refusal) cleared (pvpViewPetLocked), its target face the red one
// (targetFaceLocked); the channel is made anew each tick, so the flag down, a group or a logout puts
// it back at once (and handlers/pvp.go tells those targeting it its face at a flag's change).
//
// Numbers and choices, one place each: PvPHoldTime, PvPDamageShare, pvpDeathDebt, pvpFoeByte,
// pvpAreaAtTarget, pvpSayEvery, pvpPetAnswers.

const (
	// PvPHoldTime is how long the flag stays on after it went on and after the player's last PvP
	// attack, given or taken (the user's "1min").
	PvPHoldTime = time.Minute
	// PvPDamageShare is what a PvP hit lands at: a duel's (the coordinator's default, 2026-11-07).
	PvPDamageShare = DuelDamageShare
	// pvpDeathDebt is the experience debt of a death at a flagged player's hand: none.
	pvpDeathDebt = 0
	// pvpFoeByte is channel byte 142 for a foe (the file's doc).
	pvpFoeByte byte = 4
	// pvpDyingState is channel byte 58 for a dead flagged player: CLIENT's m_state 14, which lays
	// a sprite down (UpdateGame's SetDeath), as an NPC's corpse is sent.
	pvpDyingState byte = 14
	// pvpSayEvery is how often a press on a player not to be fought says why: the press repeats
	// every tick while the button is held (pressUnsentLocked).
	pvpSayEvery = 5 * time.Second
	// pvpPetAnswers is how long after a foe's hit a defensive or aggressive pet still goes for it.
	pvpPetAnswers = 10 * time.Second
)

// pvpAreaAtTarget says whether an area spell may be aimed at a foe and splash its caster's foes:
// true (the user, 2026-11-08: "neo cant use aoe in pvp, allow aoes to only hit players in pvp mode
// when pvp"), its target's share lands on the foe and its splash on the caster's other foes in its
// reach (areaFoesLocked), each at the PvP share, never on a player who is no foe (unflagged, or in
// the caster's group); false (the coordinator's first default: "so an AoE can't drag bystanders
// in"), refused at a foe as at any player, and its splash spares every player.
var pvpAreaAtTarget = true

// The lines. A PS2 draws them from its own characters, and they wrap.
const (
	pvpOffServer = "PvP is not available on this server."
	pvpLineOn    = "You are now flagged for PvP. Other flagged players can attack you, and you them."
	pvpLineOnHow = "/pvp off (on a PS2, /t pvp off) takes it off, a minute after it went on and after your last fight; logging out takes it off too."
	pvpLineOff   = "You are no longer flagged for PvP."
	pvpLineStays = "Your PvP flag stays on for %d more %s."
	pvpLineNotOn = "You are not flagged for PvP."
	pvpLineIsOn  = "You are already flagged for PvP."
	pvpLineDead  = "You cannot do that now."
	pvpLineStill = "Your PvP flag is still on."
	pvpLineArea  = "Area spells do not reach players."
	pvpLineNot   = "You are not flagged for PvP, so you cannot attack %s. /pvp (on a PS2, /t pvp) flags you."
)

// pvpFlag is a player's flag; its zero value is off.
type pvpFlag struct {
	on       bool
	hold     time.Time // it cannot come off before this
	said     time.Time // the last press refused with a line (pvpSayEvery)
	struckBy EntityID  // the last foe that hit it, for its pet's stance
	struckAt time.Time
	hurt     []pvpHurt // who hurt it lately, for its death's assists (combatrecord.go)
}

// SetPvP is world.pvp: whether players may flag themselves at all.
func (w *World) SetPvP(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pvpAllowed = on
}

// PvPFlagged reports the player flagged.
func (p *Player) PvPFlagged() bool { return p.pvp.on }

// pvpFoe reports q a player p may fight now: both flagged, alive, in one world and in no group
// together. Nothing else; in any area. In the arena (ctf.go) the teams alone decide: two players of
// one match on different teams while its banners are live, whatever their flags and groups, and a
// match's player is no one else's foe, nor anyone else its.
func (p *Player) pvpFoe(q *Player) bool {
	if p == nil || q == nil || p == q || p.World != q.World {
		return false
	}
	if p.IsDead() || q.IsDead() || p.respawnPending() || q.respawnPending() {
		return false
	}
	if p.ctf.match != 0 || q.ctf.match != 0 {
		return p.ctf.match == q.ctf.match && p.ctf.team != q.ctf.team && p.ctf.live && q.ctf.live
	}
	if !p.pvp.on || !q.pvp.on {
		return false
	}
	return p.GroupID == 0 || p.GroupID != q.GroupID
}

// pvpLive reports p fighting players: flagged for PvP, or playing in the arena's match (ctf.go).
func (p *Player) pvpLive() bool {
	return p.pvp.on || p.ctf.match != 0
}

// PvPWant is what a /pvp asks for: the flag turned over, or (/pvp on, /pvp off) one way only, so
// that a player making sure it is off does not put it up.
type PvPWant int

// What a /pvp asks for.
const (
	PvPToggle PvPWant = iota
	PvPWantOn
	PvPWantOff
)

// PvPCommand is /pvp from sess's player: the flag on, or off once its time allows, the player
// told either way. It returns the player when the flag changed, for the handlers to tell those
// targeting it (TargetersOf).
func (w *World) PvPCommand(sess *session.Session, now time.Time) *Player {
	return w.PvPAsk(sess, PvPToggle, now)
}

// PvPAsk is PvPCommand asking for one way, or the other way round (PvPToggle).
func (w *World) PvPAsk(sess *session.Session, want PvPWant, now time.Time) *Player {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil
	}
	if !w.pvpAllowed {
		w.sendChatToSession(sess, pvpOffServer)
		return nil
	}
	if p.IsDead() || p.respawnPending() {
		w.sendChatToSession(sess, pvpLineDead)
		return nil
	}
	if want == PvPWantOff && !p.pvp.on {
		w.sendChatToSession(sess, pvpLineNotOn)
		return nil
	}
	if want == PvPWantOn && p.pvp.on {
		w.sendChatToSession(sess, pvpLineIsOn)
		return nil
	}
	if !p.pvp.on {
		p.pvp = pvpFlag{on: true, hold: now.Add(PvPHoldTime)}
		log.Printf("PvP: %s puts up the flag", p.Name)
		w.sendChatToSession(sess, pvpLineOn)
		w.sendChatToSession(sess, pvpLineOnHow)
		return p
	}
	if left := p.pvp.hold.Sub(now); left > 0 {
		n, word := int(left.Seconds()+0.999), "seconds"
		if n == 1 {
			word = "second"
		}
		w.sendChatToSession(sess, fmt.Sprintf(pvpLineStays, n, word))
		return nil
	}
	w.pvpOffLocked(p)
	w.sendChatToSession(sess, pvpLineOff)
	return p
}

// pvpOffLocked takes p's flag down: what foes put on it goes with it, as a duel's spells go at its
// end, and its melee on a player is let go next tick (duelRoundLocked).
func (w *World) pvpOffLocked(p *Player) {
	p.pvp = pvpFlag{}
	log.Printf("PvP: %s takes down the flag", p.Name)
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.duelFoe != 0 {
			if d := w.duels.of[p.ServerID]; d == nil || d.other(p.ServerID) != e.duelFoe {
				w.removeEffectLocked(p, e.SpellID)
			}
		}
	}
}

// pvpLeavingLocked is p leaving the world: what it put on foes goes (a caster gone ticks nothing:
// duelTickLocked), as what a duelist put on its opponent does.
func (w *World) pvpLeavingLocked(p *Player) {
	if !p.pvp.on {
		return
	}
	for _, q := range w.Players {
		if q != p && q.pvp.on {
			w.clearDuelEffectsLocked(q, p.ServerID)
		}
	}
}

// pvpFightLocked is the fight between two foes, nil when they are not: a Duel in no table, made
// for the moment (the file's doc). Must be called with the lock held.
func (w *World) pvpFightLocked(a, b *Player) *Duel {
	if !a.pvpFoe(b) {
		return nil
	}
	return &Duel{A: a.ServerID, B: b.ServerID, who: [2]*Player{a, b}, Phase: duelOn, World: a.World, pvp: true,
		pets: make(map[EntityID]bool), petOut: make(map[EntityID]bool), petTarget: make(map[EntityID]EntityID),
		struck: make(map[EntityID]bool), petStruck: make(map[EntityID]bool)}
}

// fightBetweenLocked is the fight p's hit on t belongs to: their duel, else their PvP fight, else nil.
func (w *World) fightBetweenLocked(p, t *Player) *Duel {
	if d := w.duelOnLocked(p.ServerID); d != nil && d.other(p.ServerID) == t.ServerID {
		return d
	}
	return w.pvpFightLocked(p, t)
}

// pvpAttackedLocked is an attack of attacker's landing on t (a hit from it or its pet, a harmful
// spell, a damage tick): both flags held from now, and t's pet told who.
func pvpAttackedLocked(attacker, t *Player, now time.Time) {
	attacker.pvp.hold = now.Add(PvPHoldTime)
	t.pvp.hold = now.Add(PvPHoldTime)
	t.pvp.struckBy, t.pvp.struckAt = attacker.ServerID, now
	t.noteHurt(attacker, now) // combatrecord.go: an assist, should t die
}

// pvpEngageLocked is EngageAttack's PvP half: a 0x000F at a foe engages it as a creature is
// engaged, the attacker's flag held from the press. It reports whether it took the press; a press
// on a player who is no foe is told why (at most every pvpSayEvery).
func (w *World) pvpEngageLocked(p *Player, ent Entity, objectID uint32, swing Swing, now time.Time) bool {
	if !w.pvpAllowed && p.ctf.match == 0 {
		return false
	}
	if pet, isNPC := ent.(*NPC); isNPC {
		// a foe's pet, engaged as its owner is; anyone else's stays a pet (attack.go)
		if w.pvpFoePetLocked(p, pet) == nil {
			return false
		}
		w.pvpEngageOnLocked(p, pet.ServerID, pet.Name, objectID, swing, now)
		return true
	}
	t, ok := ent.(*Player)
	if !ok || t == p {
		return false
	}
	if !p.pvpFoe(t) {
		if why := pvpRefusal(p, t); why != "" && now.Sub(p.pvp.said) >= pvpSayEvery && w.duels.of[p.ServerID] == nil {
			p.pvp.said = now
			if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
				w.sendColoredLocked(sess, why, castRefusalRGB)
			}
		}
		return false
	}
	w.pvpEngageOnLocked(p, t.ServerID, t.Name, objectID, swing, now)
	return true
}

// pvpEngageOnLocked is a press taken on a foe or a foe's pet: p engages it, its flag held from the press.
func (w *World) pvpEngageOnLocked(p *Player, id EntityID, name string, objectID uint32, swing Swing, now time.Time) {
	p.pvp.hold = now.Add(PvPHoldTime)
	p.Swing, p.swingNew = swing, false
	if p.AttackTarget == id && p.AttackObjectID == objectID {
		p.pressed(now)
		return
	}
	p.AttackTarget, p.AttackObjectID = id, objectID
	p.pressed(now)
	w.breakInvisLocked(p, "an attack")
	w.breakBoardWolfLocked(p, "an attack")
	log.Printf("PvP: %s engages %s (swing %d, %v, reach %.1f)", p.Name, name, swing.Kind, swing.Delay, swing.Reach)
}

// pvpFoePetLocked is the owner of npc when npc is a foe's pet to p (the file's doc): an NPC alive in
// p's world that a player owns and that fights for it (its pet, charmed or summoned, the animal
// beside it, a merc, a merc's pet; not a familiar, nor a pet that takes no fight), whose owner is p's
// foe (pvpFoe); nil otherwise. Must be called with the lock held.
func (w *World) pvpFoePetLocked(p *Player, npc *NPC) *Player {
	if p == nil || npc == nil || npc.Owner == 0 || npc.Owner == p.ServerID || npc.Dead || npc.World != p.World ||
		npc.isFamiliar() || !npc.petFights() || w.NPCs[npc.ServerID] != npc {
		return nil
	}
	if !npc.isMerc() && w.petStateOfNPCLocked(npc) == nil {
		return nil // owned by nothing that fights for a player (no pet state of its own)
	}
	if o := w.Players[npc.Owner]; p.pvpFoe(o) {
		return o
	}
	return nil
}

// pvpFoeOfLocked is the foe of p's that id is or owns (a foe, or a foe's pet), else nil.
func (w *World) pvpFoeOfLocked(p *Player, id EntityID) *Player {
	if t := w.Players[id]; t != nil {
		if p.pvpFoe(t) {
			return t
		}
		return nil
	}
	return w.pvpFoePetLocked(p, w.NPCs[id])
}

// pvpPetStruckLocked is an attack of attacker's landing on a pet of owner's, a foe: both flags held
// from now, and owner's own pet told who (its stance), as at a hit on owner (pvpAttackedLocked) but
// for its death's assists, which are owner's own hurts alone.
func pvpPetStruckLocked(attacker, owner *Player, now time.Time) {
	attacker.pvp.hold = now.Add(PvPHoldTime)
	if owner == nil {
		return
	}
	owner.pvp.hold = now.Add(PvPHoldTime)
	owner.pvp.struckBy, owner.pvp.struckAt = attacker.ServerID, now
}

// pvpPetDeathLocked is a foe's pet killed by killer's hand (its hit, its pet's, a spell's, a proc's,
// a shield's answer): a pet's death, as a creature's kill of it is (npcMeleeOnPetLocked), its owner
// told by its pet's or its merc's own passing (petSystem, mercSystem); no debt, no experience, no
// threat, faction or guard. Must be called with the lock held.
func (w *World) pvpPetDeathLocked(killer *Player, pet *NPC, now time.Time) {
	pet.HP = 0
	pet.endRound()
	w.killNPC(pet)
	w.dropAggroLocked(pet.ServerID)
	log.Printf("PvP: %s kills %s's %q", killer.Name, w.nameOfLocked(pet.Owner), pet.Name)
	w.duelSayLocked(killer, fmt.Sprintf("You have slain %s.", pet.Name), duelOrange)
	if killer.AttackTarget == pet.ServerID {
		killer.disengage()
	}
}

// pvpViewPetLocked is a foe's pet's channel as sess sees it: to its owner's foe its byte 142 is
// pvpFoeByte (as the foe's own: TargetInRange lets the swing go; ApplyLook's name colour stays a
// pet's), and its npc flags' 0x80 (byte 146: a charmed creature's, which ToggleAutoAttack and
// vanilla's attack_press refuse) cleared. Must be called with the lock held.
func (w *World) pvpViewPetLocked(sess *session.Session, npc *NPC, buf []byte) {
	if npc.Owner == 0 || len(buf) <= 146 {
		return
	}
	if w.pvpFoePetLocked(w.sessionPlayerLocked(sess), npc) != nil {
		buf[142] = pvpFoeByte
		buf[146] &^= 0x80
	}
}

// pvpRefusal is why p may not attack t, for the line; "" when it may or when nothing need be said
// (the dead).
func pvpRefusal(p, t *Player) string {
	if p.ctf.match != 0 || t.ctf.match != 0 {
		return ctfRefusal(p, t) // ctf.go
	}
	switch {
	case p.IsDead() || t.IsDead():
		return ""
	case !p.pvp.on:
		return fmt.Sprintf(pvpLineNot, t.Name)
	case !t.pvp.on:
		return fmt.Sprintf("%s is not flagged for PvP.", t.Name)
	case p.GroupID != 0 && p.GroupID == t.GroupID:
		return fmt.Sprintf("%s is in your group.", t.Name)
	}
	return ""
}

// pvpSpellTargetLocked is castTargetsLocked's PvP half: a harmful spell at a foe is cast at it; on a
// server with the flag, a harmful spell at another player who is no foe is refused saying why. ok false
// when target is none of its business.
func (w *World) pvpSpellTargetLocked(p *Player, spell *CastSpell, target Entity) (ids []EntityID, refusal string, ok bool) {
	d := &spell.Def
	if pet, isNPC := target.(*NPC); isNPC && d.Harmful && (w.pvpAllowed || p.ctf.match != 0) {
		// a foe's pet, as a duel opponent's (duelSpellTargetLocked); anyone else's stays a pet (cast.go)
		if w.pvpFoePetLocked(p, pet) == nil {
			return nil, "", false
		}
		switch {
		case d.Charms():
			return nil, castFriendly, true
		case spell.Area.Centre != AreaNone && !pvpAreaAtTarget:
			return nil, pvpLineArea, true
		}
		if outOfRange(p, pet.X, pet.Y, pet.Z, spell.Range+npcReachExtra(pet)) {
			return nil, refuseOutOfRange(p, spell, pet.Name, pet.X, pet.Y, pet.Z, spell.Range+npcReachExtra(pet)), true
		}
		spell.duel = true
		return []EntityID{pet.ServerID}, "", true
	}
	t, isPlayer := target.(*Player)
	if (!w.pvpAllowed && p.ctf.match == 0) || !d.Harmful || !isPlayer || t == p {
		return nil, "", false
	}
	if !p.pvpFoe(t) {
		if why := pvpRefusal(p, t); why != "" {
			return nil, why, true
		}
		return nil, "", false
	}
	switch {
	case d.Charms():
		return nil, castFriendly, true
	case d.undeadOnly():
		return nil, castNotUndead, true
	case d.TargetKind == "animal":
		return nil, castNotAnimal, true
	case d.TargetKind == "summoned":
		return nil, castNotSummoned, true
	case spell.Area.Centre != AreaNone && !pvpAreaAtTarget:
		return nil, pvpLineArea, true
	}
	if outOfRange(p, t.X, t.Y, t.Z, spell.Range) {
		return nil, refuseOutOfRange(p, spell, t.Name, t.X, t.Y, t.Z, spell.Range), true
	}
	spell.duel = true
	return []EntityID{t.ServerID}, "", true
}

// areaFoesLocked is the flagged players a harmful area spell of p's reaches beside its creatures
// (pvpAreaAtTarget): p's foes (pvpFoe: both flagged, in one world, not in one group) within r of
// (cx, cy, cz) and in its reach (sight.go), the cast's own targets left out; none when p is not
// flagged, or for a spell no player takes (undead or animal alone, a charm). Must be called with
// the lock held.
func (w *World) areaFoesLocked(p *Player, cs *castState, cx, cy, cz, r float32) []areaNear {
	d := &cs.spell.Def
	if !pvpAreaAtTarget || (!(w.pvpAllowed && p.pvp.on) && p.ctf.match == 0) || d.undeadOnly() || d.TargetKind != "" || d.Charms() {
		return nil
	}
	place := w.areaPlaceLocked(p, cs)
	cone := w.playerConeLocked(p, cs) // area.go: a cone's foes in front of its caster
	var out []areaNear
	for id, t := range w.Players {
		if areaHas(cs.targets, id) || !p.pvpFoe(t) {
			continue
		}
		if in, dd := inArea(cone, cx, cy, cz, r, t.X, t.Y, t.Z, 0); in && w.reachesLocked(place, t.X, t.Y, t.Z) {
			out = append(out, areaNear{id, dd})
		}
	}
	// and their pets (pvpFoePetLocked), as the creatures are found (areaCreaturesLocked)
	for _, id := range w.SpatialIndex.QueryRadius(cx, cz, queryRadius(cone, r)) {
		pet := w.NPCs[id]
		if pet == nil || pet.Owner == 0 || areaHas(cs.targets, id) || w.pvpFoePetLocked(p, pet) == nil {
			continue
		}
		if in, dd := inArea(cone, cx, cy, cz, r, pet.X, pet.Y, pet.Z, npcReachExtra(pet)); in && w.reachesLocked(place, pet.X, pet.Y, pet.Z) {
			out = append(out, areaNear{id, dd})
		}
	}
	return out
}

// pvpCastBegunLocked is a cast begun: one aimed at a foe holds its caster's flag from now.
func (w *World) pvpCastBegunLocked(p *Player, spell *CastSpell, targets []EntityID, now time.Time) {
	if !spell.duel || !p.pvpLive() {
		return
	}
	for _, id := range targets {
		if w.pvpFoeOfLocked(p, id) != nil { // a foe, or a foe's pet
			p.pvp.hold = now.Add(PvPHoldTime)
		}
	}
}

// pvpDeathLocked is t's death at killer's hand, and everything a PvP death costs, here alone: it
// dies as of anything and comes back at its bind after the usual wait (respawn.go) with no
// experience debt (pvpDeathDebt), no item or tunar lost, no corpse (a player has none) and no
// experience for the killer. Must be called with the lock held.
func (w *World) pvpDeathLocked(killer, t *Player, now time.Time) {
	if t.GMTestMode { // a GM's #testmode (gmchannel.go): no death, no kill counted
		t.HP = 1
		t.StatsDirty = true
		return
	}
	if !t.respawnPending() {
		w.recordKillLocked(killer, t, now) // combatrecord.go: counted once a death
		w.ctfKillLocked(killer, t)         // ctf.go: the match's count
	}
	sess := w.findSessionByEntityIDLocked(t.ServerID)
	t.setDeathCause(CausePvP, killer.Name) // a Hardcore character's fall names its foe (hardcore.go)
	w.playerDownLocked(sess, t, now, pvpDeathDebt)
	if killer.AttackTarget == t.ServerID {
		killer.disengage()
	}
	log.Printf("PvP: %s kills %s", killer.Name, t.Name)
	if sess != nil {
		w.sendColoredLocked(sess, fmt.Sprintf("%s has slain you.", killer.Name), duelOrange)
	}
	w.duelSayLocked(killer, fmt.Sprintf("You have slain %s.", t.Name), duelOrange)
}

// pvpViewLocked is a flagged player's channel as sess sees it: to a foe its byte 142 is pvpFoeByte
// (the file's doc), and dead its byte 58 is CLIENT's dying state: its own client lays only its own
// body down (Msg005E), and a dead client standing still reports nothing, so everyone else would see
// the fallen stand there with no health until it went home. Must be called with the lock held.
func (w *World) pvpViewLocked(sess *session.Session, p *Player, buf []byte) {
	if !p.pvpLive() || len(buf) <= 142 {
		return
	}
	if p.respawnPending() {
		buf[58] = pvpDyingState
		return
	}
	if viewer := w.sessionPlayerLocked(sess); p.pvpFoe(viewer) {
		buf[142] = pvpFoeByte
	}
}

// pvpRiseLocked is a flagged player back from a death: every client that has it on a slot makes it
// anew under a new id at the next visibility pass. CLIENT never stands a sprite it laid down
// (VICSprite::m_dead clears only in Clear, a new sprite) and vanilla keeps a body by its entity's id,
// so one that watched it fall, and whose view it comes back into before its slot goes, would draw it
// lying there for as long as it stayed in view. Must be called with the lock held.
func (w *World) pvpRiseLocked(p *Player) {
	if !p.pvpLive() || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		vs, ok := s.VisibilityState().(*VisibilityState)
		conn := s.Connection()
		if !ok || vs == nil || conn == nil {
			return true
		}
		for i, id := range vs.ChannelEntity {
			if id != p.ServerID {
				continue
			}
			if state := conn.GetServerObject(i + 1); state != nil {
				state.MarkRemoved()
			}
			vs.ChannelEntity[i], vs.ChannelObjectID[i] = 0, 0
		}
		return true
	})
}

// PlayerFace is the face a target window shows for player t, whoever looks: the red face flagged, a
// player's blue otherwise.
func (w *World) PlayerFace(t *Player) byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if t.pvp.on {
		return FaceHostile
	}
	return FaceAlly
}

// PlayerFaceFor is the face viewer's target window shows for player t: in the arena (ctf.go) the red face
// for the other team's and a player's blue for its own team's, else PlayerFace's.
func (w *World) PlayerFaceFor(viewer, t *Player) byte {
	w.mu.RLock()
	m := w.ctfMatchOfLocked(viewer) // either game's (read only: no swap)
	if viewer.inMatch(m) && t.inMatch(m) {
		w.mu.RUnlock()
		if viewer.ctf.team != t.ctf.team {
			return FaceHostile
		}
		return FaceAlly
	}
	w.mu.RUnlock()
	return w.PlayerFace(t)
}

// Targeter is a session whose client has a player as its target.
type Targeter struct {
	Sess     *session.Session
	Viewer   *Player
	ObjectID uint32 // the id it knows the player by
	Token    uint32 // its last 0x0010's token
}

// TargetersOf is everyone whose client has t targeted, t itself among them. A target's face is read
// only from 0x0010, so these are who a flag's change must tell.
func (w *World) TargetersOf(t *Player) []Targeter {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []Targeter
	if w.SessionManager == nil {
		return nil
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() {
			return true
		}
		v := w.sessionPlayerLocked(s)
		if v == nil || v.TargetObjectID == 0 || v.TargetObjectID == noTargetObject {
			return true
		}
		if id := w.objectIDSeenByLocked(s, t.ServerID); id != 0 && id == v.TargetObjectID {
			out = append(out, Targeter{Sess: s, Viewer: v, ObjectID: id, Token: v.targetToken})
		}
		return true
	})
	return out
}

// PetTargeter is a session whose client has one of a player's pets as its target.
type PetTargeter struct {
	Targeter
	Pet *NPC
}

// PetTargetersOf is everyone whose client has one of t's pets (anything t owns: pvpFoePetLocked's
// kinds) targeted: a flag's change turns that pet's face for its owner's foes, read only from 0x0010.
func (w *World) PetTargetersOf(t *Player) []PetTargeter {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.SessionManager == nil {
		return nil
	}
	var pets []*NPC
	for _, n := range w.NPCs {
		if n.Owner == t.ServerID && !n.Dead {
			pets = append(pets, n)
		}
	}
	if len(pets) == 0 {
		return nil
	}
	var out []PetTargeter
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() {
			return true
		}
		v := w.sessionPlayerLocked(s)
		if v == nil || v.TargetObjectID == 0 || v.TargetObjectID == noTargetObject {
			return true
		}
		for _, n := range pets {
			if id := w.objectIDSeenByLocked(s, n.ServerID); id != 0 && id == v.TargetObjectID {
				out = append(out, PetTargeter{Targeter: Targeter{Sess: s, Viewer: v, ObjectID: id, Token: v.targetToken}, Pet: n})
			}
		}
		return true
	})
	return out
}

// SetTargetToken keeps the token of a player's last 0x0010.
func (w *World) SetTargetToken(id EntityID, token uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.Players[id]; p != nil {
		p.targetToken = token
	}
}

// pvpPetLocked is petSystem's PvP half: a flagged owner's pet goes for a foe or a foe's pet told to
// (0x0041 8: pvpPetCommandLocked) or by its stance as in a duel: aggressive what its owner swings at
// or what struck its owner (or its pet), defensive what struck its owner; every hit is its owner's
// PvP hit. To its owner's foes it is a foe's pet (pvpFoePetLocked), fought as they are. It reports
// whether the pet fought this tick (its other fights and its following wait).
func (w *World) pvpPetLocked(ps *petState, dt float32, now time.Time) bool {
	owner, pet := ps.owner, ps.npc
	if !owner.pvpLive() || pet.Charm != nil || !pet.petFights() {
		if ps.pvpTarget != 0 {
			ps.pvpTarget = 0
			pet.endRound()
			pet.stopMoving()
		}
		return false
	}
	foe := w.pvpFoeOfLocked(owner, ps.pvpTarget)
	if ps.pvpTarget != 0 && foe == nil {
		ps.pvpTarget = 0
		pet.endRound()
		pet.stopMoving()
	}
	if ps.pvpTarget == 0 {
		if now.Before(ps.heldUntil) {
			return false
		}
		if ps.stance == petAggressive && owner.Attacking() {
			if q := w.pvpFoeOfLocked(owner, owner.AttackTarget); q != nil {
				ps.pvpTarget, foe = owner.AttackTarget, q
			}
		}
		if foe == nil && (ps.stance == petAggressive || ps.stance == petDefensive) && now.Sub(owner.pvp.struckAt) < pvpPetAnswers {
			if q := w.Players[owner.pvp.struckBy]; owner.pvpFoe(q) {
				ps.pvpTarget, foe = q.ServerID, q
			}
		}
		if foe == nil {
			return false
		}
		log.Printf("PvP: %s's pet %q goes for %s", owner.Name, pet.Name, w.nameOfEntityLocked(ps.pvpTarget))
	}
	if !w.duelPetRoundLocked(ps, w.pvpFightLocked(owner, foe), ps.pvpTarget, dt, now) {
		ps.pvpTarget = 0 // past its leash: it follows its owner
		return false
	}
	return true
}

// nameOfEntityLocked is a player's or an NPC's name, for the log.
func (w *World) nameOfEntityLocked(id EntityID) string {
	if p := w.Players[id]; p != nil {
		return p.Name
	}
	if n := w.NPCs[id]; n != nil {
		return n.Name
	}
	return "?"
}

// pvpPetCommandLocked is PetCommand's attack (0x0041 8) at a foe of the owner's or a foe's pet: the
// pet goes for it. It reports whether the target was one.
func (w *World) pvpPetCommandLocked(ps *petState, target Entity) bool {
	if target == nil || ps.npc.Charm != nil || !ps.npc.petFights() || w.pvpFoeOfLocked(ps.owner, target.GetEntityID()) == nil {
		return false
	}
	ps.pvpTarget, ps.target = target.GetEntityID(), 0
	ps.heldUntil = time.Time{}
	return true
}
