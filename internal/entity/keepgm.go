package entity

// keepgm.go — the GMs' modes carried through a restart that keeps the players
// (keep.go; the owner, 2026-11-16: "also keep gm safemode on across
// restarts"). Before it every one was lost at each such restart, the GM's
// client still believing them on: the TEST menu's Safe Mode (#aggro off),
// Toggle Invis, Change Sprite, Freeze, and #testmode, #tankprofile, #speed and
// #size. Each is the running world's alone and ends at the character's next
// login, as before; a restart that keeps the players is no login, so it keeps
// them. The Safe Mode is kept by the handlers besides (handlers/gmnoaggro.go),
// over logouts and every restart; carried here it is on at once, before the
// resumed client reports. A player's /testinvis comes back from its own table
// (handlers/safemode.go) as the character is built.

// KeptGMModes is what a restart carries of a player's GM modes; nil none.
type KeptGMModes struct {
	NoAggro     bool    `json:"no_aggro,omitempty"`     // Safe Mode, #aggro off (aggro.go)
	Hidden      bool    `json:"hidden,omitempty"`       // Toggle Invis (gmchannel.go)
	Form        uint32  `json:"form,omitempty"`         // Change Sprite's body, 0 its own
	FormSize    float32 `json:"form_size,omitempty"`    // and its scale
	Frozen      bool    `json:"frozen,omitempty"`       // a GM's Freeze on it
	TestMode    bool    `json:"test_mode,omitempty"`    // #testmode
	TankProfile int     `json:"tank_profile,omitempty"` // #tankprofile1-3, 0 none
	Speed       float32 `json:"speed,omitempty"`        // #speed's factor of its own, 0 none (gmscale.go)
	Size        float32 `json:"size,omitempty"`         // #size's factor of its own, 0 none
}

// keptGMModesLocked is what a restart carries of p's GM modes, nil for none.
// Must be called with the lock held.
func (w *World) keptGMModesLocked(p *Player) *KeptGMModes {
	k := KeptGMModes{NoAggro: w.aggro.ignored[p.CharServerID], Hidden: p.GMHidden, Form: p.GMForm, FormSize: p.GMFormSize,
		Frozen: p.Frozen, TestMode: p.GMTestMode, TankProfile: p.GMTankProfile}
	if p.gmSpeedBase > 0 {
		k.Speed = p.Speed / p.gmSpeedBase
	}
	if p.gmScaleBase > 0 {
		k.Size = p.Size / p.gmScaleBase
	}
	if k == (KeptGMModes{}) {
		return nil
	}
	return &k
}

// restoreGMModesLocked gives p, built anew from its row, the GM modes its last
// process kept of it. Must be called with the lock held.
func (w *World) restoreGMModesLocked(p *Player, k *KeptGMModes) {
	if k == nil {
		return
	}
	if k.NoAggro {
		if w.aggro.ignored == nil {
			w.aggro.ignored = make(map[uint32]bool)
		}
		w.aggro.ignored[p.CharServerID] = true
	}
	p.GMHidden, p.Frozen, p.GMTestMode = k.Hidden, k.Frozen, k.TestMode
	if k.Form != 0 {
		p.GMForm, p.GMFormSize = k.Form, k.FormSize
	}
	if k.TankProfile >= 1 && k.TankProfile <= 3 {
		p.GMTankProfile = k.TankProfile
		p.StatsDirty = true
	}
	if k.Speed > 0 {
		p.SetGMSpeed(k.Speed) // its dump tells its client (handlers/keep.go ResumeDump)
	}
	if k.Size > 0 && p.Size > 0 {
		p.gmScaleBase = p.Size
		p.Size *= k.Size
	}
}
