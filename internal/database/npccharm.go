package database

import (
	"fmt"
	"strings"
)

// npccharm: how the creatures of a name take a charm (entity/charm.go), the
// GMs' word over the charms' own rules (their level caps). The wiki's
// Charmable Mobs page (eqoa.fandom.com, 2010, Kaleif's and KriztianBBB's lists,
// the user's charm-pets.md) and its Charming page name creatures the live game
// never let be charmed (Were-Hunter NPCs, raging rhinos, snapping tortoises, the
// named raid mobs), one whose charm "will not stick and he will nuke as if
// taunted" (Ambassador Vu'Malik) and some with a "low success rate" (Ebony
// Recluse, Hand of Thule), and what each makes as a pet.
//
//   - charmable 0: a charm cast at it is refused at its start, a red line and
//     nothing taken (entity/charm.go charmCannot);
//   - resist_pct: its chance in percent to resist a charm that lands, in place
//     of the charm's own (entity/charm.go charmResistBase); 0 that default, 100
//     always (it comes at its caster, as any resisted charm's creature does);
//   - template: the class template (classtemplates) it fights by while it is a
//     charmed pet (entity/charm.go, pettemplate.go), its abilities cast from its
//     master's side; empty, it fights as itself. As a player's enemy it keeps
//     whatever npctemplate gives it.
//
// The key is the name as npcclass keys it (NPCClassKey: lower case, trimmed,
// no trailing '$'), so a GM's #spawn copy shares it. source: gm, or seed for
// tools/seed_npc_charm.py's rows (its --undo deletes those).
const npcCharmSchema = `
	CREATE TABLE IF NOT EXISTS npccharm (
		name       TEXT PRIMARY KEY,
		charmable  INTEGER NOT NULL DEFAULT 1,
		resist_pct INTEGER NOT NULL DEFAULT 0,
		template   TEXT NOT NULL DEFAULT '',
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCCharm makes table npccharm (initSchema).
func (db *SQLiteDB) ensureNPCCharm() error {
	if _, err := db.Exec(npcCharmSchema); err != nil {
		return fmt.Errorf("npccharm: %w", err)
	}
	return nil
}

// CharmRule is a row of npccharm, read.
type CharmRule struct {
	Charmable bool
	ResistPct int    // 0: the charm's own chance; 1-100 the creature's
	Template  string // TemplateKey of a classtemplates row; empty none
}

// NPCCharmRules is every row of npccharm, by NPCClassKey of the name.
func (db *SQLiteDB) NPCCharmRules() (map[string]CharmRule, error) {
	rows, err := db.Query(`SELECT name, charmable, resist_pct, template FROM npccharm`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]CharmRule)
	for rows.Next() {
		var name, template string
		var charmable, resist int
		if err := rows.Scan(&name, &charmable, &resist, &template); err != nil {
			return nil, err
		}
		out[NPCClassKey(name)] = CharmRule{Charmable: charmable != 0, ResistPct: min(max(resist, 0), 100),
			Template: TemplateKey(strings.TrimSpace(template))}
	}
	return out, rows.Err()
}
