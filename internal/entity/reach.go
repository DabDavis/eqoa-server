package entity

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// reach.go — how far a creature is reached from: CLIENT's allowance for a
// large target, by the size of the creature's own model.
//
// TargetInRange (0x00BDC4B8 [V]) measures the squared 3D distance from the
// player to its target, centre to centre, against 4.5 for bare hands or a
// melee weapon (the item's range for a bow, a thrown weapon or a wand), and
// adds 3 for a target whose npc flags carry 0x400 or else 5 for 0x800. No
// other code in CLIENT reads those two bits. CLIENT reads the flags from
// the entity channel's bytes 146-147 (VICreatureEntity::ReadState: net
// crate's read_update in the vanilla client), and the server never set the
// high byte, so every creature, a dragon too, was reached at 4.5 from its
// centre: inside its model for anything bigger than a man, and stood at by
// the chase as close (F3 pass 16, "attacking and attack range correct").
//
// Which creatures carried the bits on live is not recorded anywhere we hold:
// none of the 22,750 creatures of Return Home's data or ours has them, and
// every creature in every capture (Sandstorm's, eqoa.live's) has a zero high
// byte. So the server gives them by its own rule, the one CLIENT's three
// choices suggest: the allowance (0, 3 or 5) that leaves the player the gap
// to the creature's edge nearest the gap it leaves a man. The edge is the
// model's: half the wider of its CSprite box's two level sides in CHAR.ESF
// (already in the model's own scale), times the creature's size.
// cmd/modelbake reads them off the operator's disc into <worlds>/models.txt.

// humanRadius is the edge of a man: the human body's CSprite (0x70D898C6)
// box is 2.04 wide and 2.79 deep.
const humanRadius float32 = 1.395

// The npc flags' reach bits, as CLIENT reads them (a large target, a huge
// one).
const (
	ReachLarge uint16 = 0x0400
	ReachHuge  uint16 = 0x0800
)

// modelRadii is each body's edge by its DictID (npcs.modelid as it is
// sent), from <worlds>/models.txt; empty without one.
var modelRadii atomic.Pointer[map[uint32]float32]

// SetModelRadii gives the server its bodies' edges (LoadModelRadii's).
func SetModelRadii(m map[uint32]float32) {
	modelRadii.Store(&m)
}

// ModelRadius is a body's edge, and whether it is known.
func ModelRadius(model uint32) (float32, bool) {
	m := modelRadii.Load()
	if m == nil {
		return 0, false
	}
	r, ok := (*m)[model]
	return r, ok
}

// LoadModelRadii reads cmd/modelbake's file: a line a body, its DictID in
// hex and its edge; # starts a comment.
func LoadModelRadii(path string) (map[uint32]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[uint32]float32{}
	s := bufio.NewScanner(f)
	for n := 1; s.Scan(); n++ {
		line := strings.TrimSpace(s.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		w := strings.Fields(line)
		if len(w) < 2 {
			return nil, fmt.Errorf("%s:%d: %q", path, n, line)
		}
		id, err := strconv.ParseUint(w[0], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		r, err := strconv.ParseFloat(w[1], 32)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		out[uint32(id)] = float32(r)
	}
	return out, s.Err()
}

// ReachFlagFor is the reach bit a body of edge r at size gives: the
// allowance whose gap is nearest a man's, 3 from 1.5 past his edge and 5
// from 4 past it.
func ReachFlagFor(r, size float32) uint16 {
	if size <= 0 {
		size = 1
	}
	switch over := r*size - humanRadius; {
	case over >= 4:
		return ReachHuge
	case over >= 1.5:
		return ReachLarge
	}
	return 0
}

// reachFlag is the creature's reach bit: its body's, none for a body the
// server does not know.
func (n *NPC) reachFlag() uint16 {
	r, ok := ModelRadius(uint32(n.ModelID))
	if !ok {
		return 0
	}
	return ReachFlagFor(r, n.Size)
}

// npcReachExtra is TargetInRange's allowance for the creature as a target:
// +3 for a large one, else +5 for a huge one.
func npcReachExtra(npc *NPC) float32 {
	switch f := npc.reachFlag(); {
	case f&ReachLarge != 0:
		return 3
	case f&ReachHuge != 0:
		return 5
	}
	return 0
}
