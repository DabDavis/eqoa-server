package handlers

import (
	"log"
	"math"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// handleUseSkill is 0x0032, Send0032 (viclient.cpp 15416): the ability
// stamp, the hotbar slot (VIAbility's m_slot, not its display index) and the
// target as the client has it, 0xFFFFFFFF for none. CLIENT checks nothing
// before sending one (no power, lockout, range or scope test anywhere), so
// the World does it all (entity/cast.go). A slot with no spell on it is let
// go unanswered.
func (h *SpellbookHandler) handleUseSkill(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil || h.store == nil {
		return nil
	}
	data := msg.Data()
	if len(data) < 9 {
		log.Printf("Cast: a 0x0032 of %d bytes", len(data))
		return nil
	}
	r := buffer.NewReader(data)
	_, _ = r.ReadUint32() // the stamp
	slot, _ := r.ReadByte()
	target, _ := r.ReadUint32()
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	book, err := h.store.GetCharacterSpells(p.CharServerID)
	if err != nil {
		log.Printf("Cast: %s's book: %v", p.Name, err)
		return nil
	}
	var e *database.CharSpellEntry
	for i := range book {
		if book[i].OnHotBar != 0 && book[i].WhereOnHotBar == int(slot) {
			e = &book[i]
			break
		}
	}
	if e == nil {
		log.Printf("Cast: %s has nothing on bar slot %d", p.Name, slot)
		return nil
	}
	line, fx, _, err := h.store.SpellEffect(e.Pattern.SpellID)
	if err != nil {
		log.Printf("Cast: %q's effect: %v", e.Pattern.SpellName, err)
	}
	h.gameWorld.StartCast(sess, castSpell(e, line, fx), target, time.Now())
	return nil
}

// castSpell is a book entry as the World casts it. The record gives cast
// time and recast in seconds (Sandstorm's: Minor Healing's 2 and 1 are a
// 3300 ms lockout), the front icon the effects channel shows, the range as
// writeAbilityRecord sends it.
func castSpell(e *database.CharSpellEntry, line string, fx uint32) entity.CastSpell {
	sp := &e.Pattern
	return entity.CastSpell{
		SpellID:      sp.SpellID,
		DisplayIndex: e.AddedOrder,
		Scope:        sp.Scope,
		Name:         sp.SpellName,
		Icon:         uint32(sp.Icon),
		CastTime:     time.Duration(sp.CastTime) * time.Second,
		Recast:       time.Duration(sp.Recast) * time.Second,
		PowerCost:    sp.Power,
		Range:        math.Float32frombits(abilityRangeBits(sp)),
		Def:          entity.ParseSpellText(line),
		FX:           fx,
	}
}

// gatePlayer is the World's gate hook (entity/cast.go): Return Home's
// landing. This server keeps no bind points, so home is where every login
// starts and every respawn goes (loginSpawnDest), reached by a teleport, as
// #goto moves a player (the player is alive: no dump).
func (h *CharacterHandler) gatePlayer(sess *session.Session, p *entity.Player) {
	gw := h.gameWorld
	if gw == nil || sess == nil || p == nil {
		return
	}
	dx, dy, dz, dw, via, found := ResolveTeleDest(gw, loginSpawnDest)
	if !found {
		log.Printf("Gate: no %q destination; %s stays", loginSpawnDest, p.Name)
		return
	}
	x, z := dx+loginSpawnOffset, dz+loginSpawnOffset
	world, err := ValidateTeleport(x, dy, z, dw)
	if err != nil {
		log.Printf("Gate: %s is not a placeable spawn (%v); %s stays", via, err, p.Name)
		return
	}
	snap, ok := gw.PlayerSnapshot(p.ServerID)
	if !ok {
		return
	}
	worldChanged := snap.World != world
	p.World = world
	gw.UpdatePosition(p.ServerID, x, dy, z, snap.Facing)
	sendTeleportGM(sess, x, dy, z, snap.Facing, world)
	beginWorldLoad(sess, worldChanged)
	log.Printf("Gate: %s home to %s (%.1f, %.1f, %.1f) world=%d", p.Name, via, x, dy, z, world)
}
