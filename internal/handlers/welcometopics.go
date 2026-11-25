package handlers

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/entity"
)

// welcometopics.go — the welcome guide's Commands/features topics (a GM's
// work entry of 2026-11-17: "Add an initial dialogue option to that tutorial
// that shows the normal tutorial we have and a secondary one saying
// Commands/features ... show several categories Graphics, Game features, UI
// ... For live the same thing but only for live features").
//
// Every word of the topics is here, in one place, so a GM can change it: the
// topics' names, and each topic's lines, a sentence each, packed into the
// question boxes' 127 characters (welcomePack). Both shards run one binary, so
// a Game features line is shown only where its fact holds (welcomeFacts, read
// off the running server: a switch, an NPC standing, a record), and a line in
// the spelling the player's client can use (a PS2 types "/t <word>"). No '%'
// or '|' (CLIENT's text expander reads both as codes) and nothing past ASCII,
// at most 127 characters a line at the longest name and spelling
// (TestWelcomeTopicWords).

// The topics, in the order the Topics box offers them.
type welcomeTopicID int

const (
	topicGraphics welcomeTopicID = iota
	topicFeatures
	topicUI
)

// The Topics box's question and the topics' names (to a PS2 the client's own
// topics say whose they are).
const (
	welcomeTopicsQuestion = "Commands and features: pick a topic."

	welcomeTopicGraphics    = "Graphics"
	welcomeTopicGraphicsPS2 = "Graphics (PC and browser client)"
	welcomeTopicFeatures    = "Game features"
	welcomeTopicUI          = "UI"
	welcomeTopicUIPS2       = "UI (PC and browser client)"
)

// Graphics: the PC and browser client's own, the same on every shard.
var welcomeGraphicsLines = []string{
	"Dynamic Lighting: torches, lamps, spells and glowing weapons light the world. Turn it on in Options > Extended > Graphics.",
	"Sun and Light Shadows: the sun's shadows move with the time of day, and torches cast their own. Both need Dynamic Lighting.",
	"Also there: Ambient Occlusion, Render Scale for smoother edges, Fog for draw distance, and how many NPCs and players show.",
}

// UI: the client's own, the same on every shard. welcomeUIExtras speaks of
// the server's quality-of-life additions (item links, the DPS meter, power
// bars), said only to a session that has them (QoLOnFor).
var welcomeUILines = []string{
	"Keybindings: Options > Controls (F1) changes any key or controller button. Options > Macros holds 16 macros.",
	"Move HUD (or /hud) in Options > Extended > UI puts the HUD's pieces where you like. Press H to hide the UI.",
}

const (
	welcomeUIExtras = "Also in UI: chat tabs (/tab, /tabname), item links, a DPS meter and power bars under your group."
	welcomeUISave   = "Options are kept by Save Changes, or turn on Auto Save Options under Extended > Other > General."
)

// Game features, each said only where its fact holds (welcomeFeatureLines).
const (
	welcomeMercs           = "Mercenaries: free mercs of your level, hired at the Mercenary Recruitment boards by the Announcements. %s commands them."
	welcomeArchivistFBoard = "The Spell Archivist, by Freeport's Announcements board, has your class's spells and can clear your spellbook."
	welcomeArchivistF      = "The Spell Archivist has your class's spells and can clear your spellbook."
	welcomeTeleporterBoard = "The Teleporter, by Freeport's Announcements board, sends you to any raid's way in or any XP camp, for nothing."
	welcomeTeleporterAny   = "The Teleporter sends you to any raid's way in or any XP camp, for nothing."
	welcomeBoonLine        = "XP Boon (optional, for testing): any city's Announcements gives %s for %s of play."
	welcomeAscended        = "Ascended gear: raid bosses drop Ascended copies of their gear with a tenth more stats, and some quests reward them too."
	welcomeMapsOurs        = "Maps: M opens the world map and N the minimap, with XP camps, coaches and quest marks. Click the map to set a waypoint."
	welcomeMapsPS2         = "Maps: the PC and browser client has a world map (M) and a minimap (N), with XP camps, coaches and quest marks."
	welcomeConOurs         = "/con tells you your target's level, and the PC client shows it in the target box."
	welcomeConPS2          = "/t con tells you your target's level."
	welcomePrefarm         = "%s off: quest items drop on your kills only for a player on that step. A group or raid leader's holds for the group."
)

// welcomeTopic is one topic as a player is shown it: its name and its lines.
type welcomeTopic struct {
	id    welcomeTopicID
	name  string
	lines []string
}

// welcomeTopics is every topic with a line for a server that has f, to a
// client of ours (vanilla) or not (a PS2), whose session has the server's
// quality-of-life additions (qol: QoLOnFor) or not, in the Topics box's order.
func welcomeTopics(f welcomeFacts, ours, qol bool) []welcomeTopic {
	graphics, ui := welcomeTopicGraphics, welcomeTopicUI
	if !ours {
		graphics, ui = welcomeTopicGraphicsPS2, welcomeTopicUIPS2
	}
	uiLines := append([]string(nil), welcomeUILines...)
	if qol {
		uiLines = append(uiLines, welcomeUIExtras)
	}
	uiLines = append(uiLines, welcomeUISave)
	all := []welcomeTopic{
		{topicGraphics, graphics, welcomeGraphicsLines},
		{topicFeatures, welcomeTopicFeatures, welcomeFeatureLines(f, ours, qol)},
		{topicUI, ui, uiLines},
	}
	out := all[:0]
	for _, t := range all {
		if len(t.lines) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// welcomeTopicOf is the topic of that id among ts, and whether there is one.
func welcomeTopicOf(ts []welcomeTopic, id welcomeTopicID) (welcomeTopic, bool) {
	for _, t := range ts {
		if t.id == id {
			return t, true
		}
	}
	return welcomeTopic{}, false
}

// welcomeFeatureLines is the Game features topic's lines: each only where its
// fact holds, in order.
func welcomeFeatureLines(f welcomeFacts, ours, qol bool) []string {
	var l []string
	if f.mercs {
		merc := "/t merc"
		if ours {
			merc = "/merc"
		}
		l = append(l, fmt.Sprintf(welcomeMercs, merc))
	}
	switch {
	case f.archivist && f.archivistBoard:
		l = append(l, welcomeArchivistFBoard)
	case f.archivist:
		l = append(l, welcomeArchivistF)
	}
	switch {
	case f.teleporter && f.teleporterBoard:
		l = append(l, welcomeTeleporterBoard)
	case f.teleporter:
		l = append(l, welcomeTeleporterAny)
	}
	if f.boon.On() {
		// CLIENT reads '%' as a code: "250% experience" says "250 percent"
		what := strings.ReplaceAll(f.boon.What(), "%", " percent")
		l = append(l, fmt.Sprintf(welcomeBoonLine, what, entity.SpanWords(f.boon.Duration)))
	}
	if f.ascended {
		l = append(l, welcomeAscended)
	}
	if qol {
		if ours {
			l = append(l, welcomeMapsOurs)
		} else {
			l = append(l, welcomeMapsPS2)
		}
		if f.conLevel {
			if ours {
				l = append(l, welcomeConOurs)
			} else {
				l = append(l, welcomeConPS2)
			}
		}
	}
	prefarm := "/t prefarm" // prefarm.go, every shard's
	if ours {
		prefarm = "/prefarm"
	}
	return append(l, fmt.Sprintf(welcomePrefarm, prefarm))
}
