package weather

import (
	"fmt"
	"time"
)

// A restart that keeps the players in (network/udp/keep.go) carries the GMs'
// weathers: before 2026-11-16 it dropped them (a heavy snow called on Freeport
// at 23:44:35 UTC was gone with the 23:46 restart, weather_status showing no
// override). Each goes over as it stands: its id, its zones, and their phases
// at their wall-clock times, so it runs on for the time it had left and ends
// when it would have. The automatic weather needs nothing: it is the clock's.

// KeptOverride is one GM weather as a restart carries it.
type KeptOverride struct {
	ID       int            `json:"id"`
	World    int            `json:"world"`
	Kind     Kind           `json:"kind"`
	Strength float64        `json:"strength"`
	From     time.Time      `json:"from"`
	FadeMS   int64          `json:"fade_ms"`
	By       string         `json:"by,omitempty"`
	Run      int            `json:"run,omitempty"`
	Gen      int            `json:"gen,omitempty"`
	Ended    bool           `json:"ended,omitempty"`
	WindX    float32        `json:"wind_x"`
	WindZ    float32        `json:"wind_z"`
	Zones    []KeptZoneOver `json:"zones"`
}

// KeptZoneOver is one zone of a kept GM weather: its end, whether it was
// ended there, and its phases.
type KeptZoneOver struct {
	Zone   Zone        `json:"zone"`
	Until  time.Time   `json:"until"`
	Ended  bool        `json:"ended,omitempty"`
	Phases []KeptPhase `json:"phases"`
}

// KeptPhase is a Phase as a restart carries it.
type KeptPhase struct {
	At       time.Time `json:"at"`
	Kind     Kind      `json:"kind"`
	Strength float64   `json:"strength"`
}

// Keep is every GM weather the engine holds at now (those over still bar the
// automatic weather a while: pruneLocked), oldest first.
func (e *Engine) Keep(now time.Time) []KeptOverride {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pruneLocked(now)
	out := make([]KeptOverride, 0, len(e.overrides))
	for _, o := range e.overrides {
		k := KeptOverride{ID: o.id, World: o.world, Kind: o.kind, Strength: o.strength, From: o.from, FadeMS: o.fade.Milliseconds(),
			By: o.by, Run: o.run, Gen: o.gen, Ended: o.ended, WindX: o.wx, WindZ: o.wz}
		for _, z := range o.zones {
			kz := KeptZoneOver{Zone: z, Until: o.until[z], Ended: o.endedIn[z]}
			for _, p := range o.phases[z] {
				kz.Phases = append(kz.Phases, KeptPhase{At: p.At, Kind: p.Kind, Strength: p.Strength})
			}
			k.Zones = append(k.Zones, kz)
		}
		out = append(out, k)
	}
	return out
}

// Restore puts kept GM weathers back under their own ids (the next called
// takes an id past them all), each as it was: a restart's (ResumeKept). One
// whose id the engine holds already, or that is malformed, is left out, as is
// one over long enough that it would have been pruned. How many were put back.
func (e *Engine) Restore(ks []KeptOverride, now time.Time) (int, []error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var bad []error
	have := map[int]bool{}
	for _, o := range e.overrides {
		have[o.id] = true
	}
	keep := e.keepLocked()
	n := 0
	for _, k := range ks {
		if k.ID > e.seq {
			e.seq = k.ID // never hand out a kept id again, even one left out
		}
		switch {
		case k.ID <= 0 || have[k.ID]:
			bad = append(bad, fmt.Errorf("GM weather #%d: id taken or none", k.ID))
			continue
		case k.Kind >= kinds || len(k.Zones) == 0:
			bad = append(bad, fmt.Errorf("GM weather #%d: kind %d, %d zones", k.ID, k.Kind, len(k.Zones)))
			continue
		}
		o := &override{id: k.ID, world: k.World, until: map[Zone]time.Time{}, endedIn: map[Zone]bool{}, phases: map[Zone][]Phase{},
			kind: k.Kind, strength: k.Strength, from: k.From, fade: time.Duration(k.FadeMS) * time.Millisecond, by: k.By, run: k.Run,
			gen: k.Gen, ended: k.Ended, wx: k.WindX, wz: k.WindZ}
		ok := true
		for _, kz := range k.Zones {
			if len(kz.Phases) == 0 {
				ok = false
				break
			}
			ph := make([]Phase, 0, len(kz.Phases))
			for _, p := range kz.Phases {
				if p.Kind >= kinds {
					ok = false
				}
				ph = append(ph, Phase{At: p.At, Kind: p.Kind, Strength: p.Strength})
			}
			o.zones = append(o.zones, kz.Zone)
			o.until[kz.Zone] = kz.Until
			if kz.Ended {
				o.endedIn[kz.Zone] = true
			}
			o.phases[kz.Zone] = ph
		}
		if !ok {
			bad = append(bad, fmt.Errorf("GM weather #%d: a zone with no phases or a bad kind", k.ID))
			continue
		}
		if !now.Before(o.last().Add(keep)) {
			continue // long over: nothing it holds off any more
		}
		if len(e.overrides) >= MaxOverrides {
			bad = append(bad, fmt.Errorf("GM weather #%d: %d held already", k.ID, MaxOverrides))
			continue
		}
		e.overrides = append(e.overrides, o)
		have[o.id] = true
		n++
	}
	return n, bad
}
