package session

import (
	"log"
	"os"
	"sync"
)

// The three behaviours added on 2026-08-10 for the world-change resync — hold,
// clear, resend — each write to the client at the one moment its scene is being
// torn down and rebuilt, so each of them can only be judged by removing it.
// Setting any of these env vars to "0" removes that one layer; setting all
// three restores the server to how it behaved before the resync work, which is
// the control the last known-good world-9 load (2026-08-10 02:45) ran against.
//
//	EQOA_WORLDLOAD_HOLD=0     do not defer state channel output over the load
//	EQOA_WORLDLOAD_CLEAR=0    do not zero visibility channels on a world change
//	EQOA_WORLDLOAD_RESEND=0   do not re-arm channels on 0x0049 or on arrival
//
// They live here rather than beside their callers because the resend layer has
// two call sites in two packages — the 0x0049 handler and the first 0x40 after
// a teleport — and a switch that covers only one of them removes half a layer.
//
// They default ON. The values are logged once, because a switch nobody can see
// in the journal is a switch that silently did nothing.
var (
	worldLoadFlagsOnce sync.Once
	worldLoadHold      bool
	worldLoadClear     bool
	worldLoadResend    bool
)

// WorldLoadLayers reports which of the three world-load layers are enabled.
func WorldLoadLayers() (hold, clear, resend bool) {
	worldLoadFlagsOnce.Do(func() {
		worldLoadHold = os.Getenv("EQOA_WORLDLOAD_HOLD") != "0"
		worldLoadClear = os.Getenv("EQOA_WORLDLOAD_CLEAR") != "0"
		worldLoadResend = os.Getenv("EQOA_WORLDLOAD_RESEND") != "0"
		log.Printf("World load layers: hold=%v clear=%v resend=%v",
			worldLoadHold, worldLoadClear, worldLoadResend)
	})
	return worldLoadHold, worldLoadClear, worldLoadResend
}
