package entity

// ownhelm.go — bodies that wear a helm of their own (the owner, 2026-11-16:
// "fellstar is a hill giant player race and picked fire giant skin, but fire
// giant skin is showing a plate helm and the equipped plate helm is bigger
// encompassing both looks silly"). A hill giant in plate is shown in the fire
// giant reaver's body (racearmorbody: armorbody.go), whose mesh has a great helm
// built in, and a client that hangs a player's helm on a giant's head (vanilla's
// slotless.rs HEAD_SEATS) drew the worn helm round it, larger: two helms. CHAR.ESF
// has no other giant in plate without one (every body on the giant's 44-node
// skeleton seen, claude-notes/giant-own-helm-20261116.md), so such a body keeps
// its own and is sent no worn helm: the helm piece is still worn, with its stats,
// its effects and its aura's slot, and only its look is left out, as a hidden
// helm's is (hidelook.go). Under an illusion or another form the body is another
// and the worn helm shows again, as it does on every body not listed.

// ownHelmBodies are the bodies (CHAR.ESF CSprite DictIDs) a player may be shown
// in whose head is a helm of their own, found by sight with `eqoa dress <body>
// <out.png> helm=4 race=1 frame=head` against the bare body: the worn helm drawn
// round the built-in one, both seen.
var ownHelmBodies = map[uint32]string{
	0x2D686972: "the fire giant reaver's: a black great helm (the hill giant's plate body; option Fire giant (reaver))",
	0x2ECBB95A: "the grey goblin overseer's: a horned iron helm, the worn one drawn inside it",
	0xF54C2142: "the green goblin warmaster's: a horned skull helm over the worn one",
	0x063C5D97: "the spiked froglok's: an iron cap with a spike through the worn helm",
	0x1663EC1C: "the spiked ghoul's: the spiked froglok's cap",
	0xBADC25C9: "the ratman's (its race's own body): a spiked iron helm and face guard inside the worn one",
}

// Left out, each judged on the same pictures (the note's list): the Jal'Raeth
// knight's helmeted head (491F6347), which its own seat stretches the worn helm
// round (the owner's fit, 2026-11-11, vanilla's slotless.rs); headwear that is
// no helm, a crown (the fire giant king's), a turban (the vest's), hoods (the
// sand giants', the ice goblin mystic's, the ratmen's), a straw hat (the
// kappa's), a skull mask (the grey goblin's): those keep the worn helm unless
// the owner says otherwise.

// BodyWearsHelm reports whether body model wears a helm of its own, so no worn
// helm is drawn on it (ownHelmBodies).
func BodyWearsHelm(model uint32) bool {
	_, ok := ownHelmBodies[model]
	return ok
}

// ShownHiddenParts is what of the player's worn look every client is sent as
// nothing worn: the parts it hides (HiddenParts), and the helm when the body it
// is shown in wears its own (BodyWearsHelm).
func (p *Player) ShownHiddenParts() uint16 {
	parts := p.HiddenParts
	if model, _ := p.Body(); BodyWearsHelm(model) {
		parts |= HideHelm
	}
	return parts
}
