package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// plushie.go — the Pegasus Plushie (the owner, 2026-11-17: "add pegasus
// plushie too"; the winged horse is entity/plushie.go's). The plushie is an
// item record of this name (tools/christmas_toys_20261117.py: CLIENT's usable
// type, procanim 2, so the item menu offers Use; tradeable, bound to no one,
// worth nothing), worn nowhere, never spent. A Use brings its tiny winged
// horse out, under a cute name; the next sends it home.

// PegasusPlushieName is the plushie's record's name.
const PegasusPlushieName = "Pegasus Plushie"

const (
	plushieCameLine  = "%s shakes out its wings and trots after you."
	plushieHomeLine  = "%s flutters back into the plushie."
	plushieDeadLine  = "The plushie stays still: you are dead."
	plushieArenaLine = "The plushie will not come out here."
)

// IsPegasusPlushie is whether an item record is the plushie: worn nowhere and
// so named.
func IsPegasusPlushie(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), PegasusPlushieName)
}

// usePegasusPlushie is the plushie's Use (equip.go), told to the player.
func (h *EquipHandler) usePegasusPlushie(sess *session.Session, player *entity.Player) {
	if h.gameWorld == nil {
		return
	}
	if line := plushieLine(h.gameWorld.UsePlushie(sess, time.Now())); line != "" {
		SendClientMessage(sess, line)
	} else {
		log.Printf("Pegasus Plushie: %s's use: not in the world", player.Name)
	}
}

// plushieLine is what a use of the plushie tells its user ("" for none).
func plushieLine(r entity.PlushieResult) string {
	switch r.Use {
	case entity.PlushieCame:
		return fmt.Sprintf(plushieCameLine, r.Name)
	case entity.PlushieHome:
		return fmt.Sprintf(plushieHomeLine, r.Name)
	case entity.PlushieDead:
		return plushieDeadLine
	case entity.PlushieAway:
		return plushieArenaLine
	}
	return ""
}
