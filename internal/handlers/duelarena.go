package handlers

import (
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

func SetupDuelArena(w *entity.World) {
	b, ok := ServerWorld(entity.DuelArenaWorld)
	if !ok || !worlds.IsNeriakArena(b.Recipe) || !w.ArenaLavaReady() {
		return
	}
	w.SetDuelArena(func(s *session.Session, to entity.GateSpot, facing float32) {
		sendTeleportGM(s, to.X, to.Y, to.Z, facing, to.World)
		beginWorldLoad(s, true)
	})
}
