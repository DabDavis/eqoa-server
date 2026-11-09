package entity

// numbertags.go — what made each figure (0x00DB), for a client that colours
// its numbers by it (a player's ask, 2026-11-09: "Would it be possible to have
// ability to see/change colors of damage/procs/healing for player/pets similar
// to the way SS does? Really helpful in testing to see if things are working
// correctly"; the user: "we can add the number colors to extended menu").
//
// CLIENT colours a figure by its sign and by whether the player made it (white
// a hit made, yellow one taken, green a heal), and a pet's hit reaches its
// owner as the owner's own (petLandHitLocked), so a proc, a spell, a damage
// shield and a swing all look alike. The server knows which, so it says: once
// a client answers the offer (protocol.OpcodeNumberTagsAnswer,
// handlers/numbertags.go, kept on its session), each 0x00DB it is sent of a
// figure its player or its player's pet made carries one byte after the retail
// body (sendDamageIndication), a numberKind. Every other figure (one another
// made on the player: a creature's hit, a creature's or another player's heal,
// the sun, drowning, a price paid) and every figure to a session that never
// answered (a PS2, an older build) goes as it went, the retail body alone.
//
// What is what, by the path that makes it (each call site passes its kind):
//
//   - numberMelee: a swing's hit, the player's (playerHitLocked, a bow's shot
//     too) and in a duel or a PvP fight (duelRoundLocked), its pet's
//     (petHitLocked, a charmed creature's, a duel's).
//   - numberSpell: a spell's or an ability's figure that hurts, on its target
//     or an area's others (landOnNPCLocked, landOnPetLocked's harm on a pet,
//     duelLandLocked), a thrown potion's (throwLocked), a pet's spell, area
//     splash, taunt, special and tap (petLandLocked, petSplashLocked,
//     petTauntLocked, petTemplateSpecialLocked, pettap.go, petrotation.go).
//   - numberDoT: a damage over time's tick (tickNPCEffectLocked,
//     duelTickLocked, a pet's petTickLocked).
//   - numberShield: a damage shield's answer to a melee hit on the player or
//     its pet (playerShieldsLocked, petShieldsLocked, duelAnswerLocked).
//   - numberProc: whatever a proc does: an effect's proc and its life steal's
//     heal (playerProcsLocked, procTapLocked), a proc heal (procHealsLocked),
//     a weapon's proc, its tap's heal, its restore and its damage over time's
//     ticks (weaponproc.go), a spell proc's hit, heal and ticks
//     (workbookmore.go), a defensive proc (shieldAnswers' proc), a worn
//     item's answer and heal at a hit (wornshield.go), a mark's heal at a
//     weapon's hit (markheal.go), a pet's template proc and its effects' procs
//     (petTemplateProcLocked, petProcsLocked), and the same in a duel
//     (duelprocs.go).
//   - numberHeal: a spell's heal, direct or area, on the player itself,
//     another player or a pet (landOnPlayerLocked, landOnPetLocked), a spell's
//     own part for its caster (landOnCasterLocked), a drain's or a life tap's
//     take (drainLocked), a drunk potion's; a pet's heal on itself or its
//     owner, and its tap's take (tellPetHealLocked, petLandLocked, pettap.go).
//   - numberHoT: a heal over time's tick (runTickLocked), and a drain over
//     time's take at each tick (tickNPCEffectLocked).
//
// numberByPet is set on any of them the player's pet made. A spell the pet
// casts for its owner lands as the owner's own cast (petLandLocked's buff), its
// figures the pet's (figuresPet). A gift landed on the player as if it cast it
// (an NPC buffer's spells, a town board's, Mr. Moogie's favor: figuresGift) is
// none of the player's, and its figures go untagged; so does a heal over time's
// tick whose caster is gone (the figure falls to its bearer, runTickLocked).
type numberKind uint8

const (
	numberMelee  numberKind = 1
	numberSpell  numberKind = 2
	numberDoT    numberKind = 3
	numberShield numberKind = 4
	numberProc   numberKind = 5
	numberHeal   numberKind = 6
	numberHoT    numberKind = 7
	// numberByPet marks a kind its maker's pet made (bit 7).
	numberByPet numberKind = 0x80
)

// tagged is whether a figure of kind k is told with its byte: any kind but
// none (0, an untagged figure; the pet's bit alone is none too).
func (k numberKind) tagged() bool { return k&^numberByPet != 0 }

// byPet is k as its maker's pet made it; none stays none.
func (k numberKind) byPet() numberKind {
	if !k.tagged() {
		return 0
	}
	return k | numberByPet
}

// castFigures is whose a cast's figures are, as numberKind tells them: the
// caster's spell (the zero value), a proc's, the caster's pet's, or a gift's
// its caster did not make.
type castFigures uint8

const (
	figuresSpell castFigures = iota
	figuresProc
	figuresPet
	figuresGift
)

// kind is what a figure of whose kind f is told as: one that heals (heal) or
// hurts, in an instant or at a tick of what lasts of it (tick).
func (f castFigures) kind(heal, tick bool) numberKind {
	var k numberKind
	switch {
	case f == figuresGift:
		return 0
	case f == figuresProc:
		return numberProc
	case heal && tick:
		k = numberHoT
	case heal:
		k = numberHeal
	case tick:
		k = numberDoT
	default:
		k = numberSpell
	}
	if f == figuresPet {
		k = k.byPet()
	}
	return k
}

// hitKind is what a figure of the cast's that hurts is told as.
func (s *CastSpell) hitKind() numberKind { return s.figures.kind(false, false) }

// healKind is what a figure of the cast's that heals is told as.
func (s *CastSpell) healKind() numberKind { return s.figures.kind(true, false) }

// tickKind is what a tick of the effect is told as: one that heals (heal),
// else hurts.
func (ae *ActiveEffect) tickKind(heal bool) numberKind { return ae.figures.kind(heal, true) }
