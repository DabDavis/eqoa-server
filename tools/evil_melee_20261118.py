#!/usr/bin/env python3
"""Epic-sode 3 of the level 60 Evil Melee epic as the owner's source has it: Ronson's walkthrough on the EQOA forum
(eqoa.forumotion.com/t155-evil-melee, the owner 2026-11-18: "make the Evil Melee epic, episode 3, work as that post
describes"), and its twin for the Good Caster (t152-good-caster, the same Boldunder and the same summoning), with the
two bosses as a GM's board entries have them (636 Boldunder, 615 Ahriaana, 692 "spawn from quest", 676's
hp_pct). By NAMES and places, never by ids: the two shards' rows are read where they stand.

WHAT DIFFERED, and what this writes:

  Boldunder (quests "Boldunder" by Rashiroz the Rogue and by Magi'Kot):
  - He stood in the world always (row by the depression, a roamer, 90 s respawn), and the quest's one reach step
    gave one popup. The post: "At the base of a hill, near a large depression you should get a popup telling you you
    feel the ground rumbling. Walking away from the hill toward the depression, you should get two more popups, then
    Boldunder spawns and tears you a new one." Now three reach steps: the hill's west foot (the ground files: the hill
    tops out at 108 at 11340, 5310; its foot at 64), half way down (40), the bowl's middle (35, the depression's
    floor, 11100, 5370); a popup each; the third's done raises him there, hostile, a GROUP's spawn (one for a group or
    raid; the other quest's group spawn of him stands for a mixed raid too: this branch's code), 120 minutes. His row
    is moved to the bowl's middle (the quest marks point there, and a quester who lost him gets him again there:
    respawnLost) and is spawn-only (npcspawnonly, board 692: "Spawn from quest").
  - His fight (board 636; 676's hp_pct): his own template "boldunder" (was "roamer"): hp_pct 5367 (~900,000), melee
    1,500 a hit on a warrior (melee_pct 2058: entry 1404's calibration, 3430 for 2,500, in proportion: OURS), a
    2,000 Critical Hit (30.77 a level) every 80 s from 6 s, all six resists at 75% ("He resists all nukes": 636's
    proposal; 100 the literal reading, its open question 3), uncharmable, a leash of 1500 (636's code ask 1: the era
    kited him across the coast; ours) .
  - Loot left as it is: his 10 souls a kill (GM ticket 2029, No Trade + Lore, shared one to a character) and each
    quester's own soul from the loot step. The post's "4 souls" is covered: every quester of the kill gets one.

  Ahriaana the Ultimate (quests "Ahriaana the Ultimate" by Rashiroz the Rogue and by Magi'Kot):
  - Step 1 at Magi'Kot sent the quester alone to the Rain isle (by Trixie Longwings, under the guardian gargoyles)
    and called her there at once. The post: "The Evil Melee doing the quest will hand out gems that will port you to
    PoS (the usual first island, where the Isle of Wonder port is). Once everyone's ready, talk to Magi`Kot again, who
    can also be found in PoS now, and he'll spawn Ahriaana the Ultimate." Now:
      step 1, Magi'Kot in the Elemental Towers (pinned to his row): 20 of a new TRADEABLE item "Magi'Kot's summoning
        gem" (use: to the main isle, where the Sky Portal lands; one use each; topped up to 20, not stacked past it)
        and the quester taken to the main isle;
      step 2, Magi'Kot on the main isle (a NEW npcs row, his body, by the Isle of Wonder portal, pinned): his talk
        fires the summoning (trigger "Ahriaana the Ultimate", one at a time as before);
      steps 3 and 4 the old 2 and 3 (her page; Rashiroz, or Magi'Kot's ghost for the good casters).
    The summoning (the trigger, its spawn and her row) moves to open ground on the main isle, 53 south of the landing;
    the ghost (the good casters' turn-in) moves north-west of the Isle of Wonder portal (t152: "head NW of the IoW
    port to talk to Magi`Kot's ghost").
  - Her port of the one she fights (board 615 code ask 1; this branch's code, kind "portfoe"): every 30 s from 30 s
    at 50%, a PLAYER she fights is sent to its bind point and forgotten (a pet never: the era fought her with pets),
    named "Ultimate Banishment". Her hour's full heal ("Ultimate Renewal") and her numbers are kept as they are.
  - Her pages: each quester's own from the loot step, as before (the post's "8 for 8 questers": none short).

  Characters on those quests are moved with their steps (Boldunder 2 -> 4, 3 -> 5; Ahriaana 2 -> 3, 3 -> 4).

Report only by default; --apply in one transaction with gm_audit lines and the log evil_melee_1118_log (every row
before and after); a second --apply does nothing; --applied says whether it is applied and what it wrote; --undo puts
back exactly what it wrote (a row changed since is left and said) and drops the log. Refused whole if a row is not as
this tool expects.

WHEN IT TAKES EFFECT: with the restart that brings branch evil-melee's code (portfoe; group spawns across quests;
it carries branch siliskor's placed and group spawns): an older server reads Ahriaana's template as unreadable. The
quests, NPC rows, templates, the trigger and spawn-only at that start; the gem's record at each holder's login; its
use as it is read (per use).

    python3 -E -s tools/evil_melee_20261118.py --db eqoa.db [--apply | --applied | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "evil_melee_1118_log"
WHO, VIA = "dab", "tool"
SRC = "evilmelee1118"
POST = "Ronson's walkthroughs (eqoa.forumotion.com/t155-evil-melee, t152-good-caster; the owner's source 2026-11-18)"
DETAIL = f"the Evil Melee epic-sode 3 as {POST}: tools/evil_melee_20261118.py"

ET_GIVER, ROGUE_GIVER = "Magi'Kot", "Rashiroz the Rogue"
GIVERS = (ROGUE_GIVER, ET_GIVER)
BOLD_Q, AHRI_Q = "Boldunder", "Ahriaana the Ultimate"
BOLD, AHRI, GHOST, MAGI = "boldunder", "ahriaana the ultimate", "ghost of magi'kot", "magi'kot"
TRIGGER = "Ahriaana the Ultimate"

# North Barren Coast (world 2): the hill's west foot, half way down, the bowl's middle (the ground files' floors)
HILL_FOOT = (11245.0, 63.9, 5355.0)
HALF_WAY = (11175.0, 40.3, 5365.0)
BOWL = (11100.0, 35.3, 5370.0)
BOLD_LIFE = 120  # minutes his spawn lives (Siliskor's; the engine's 10 would take a ~900k boss away mid-fight)

# The Plane of Sky's main isle (world 4): the Sky Portal's landing (spellgates 'sky portal'), Magi'Kot by it, the
# summoning's open ground 53 south of it, the ghost north-west of the Isle of Wonder portal (all floors 250.06, the
# ground files: reached on foot from the landing, out of water)
LANDING = (4834.8, 250.1, 5524.7)
SKY_MAGI = (4815.0, 250.06, 5520.0, 55)   # facing the landing (row byte)
SUMMON = (4812.0, 250.06, 5575.0, 110)    # facing the landing
GHOST_AT = (4745.0, 250.06, 5440.0, 39)   # facing the Isle of Wonder portal

GEM_ID, GEM_NAME, GEM_BASE = 716990, "Magi'Kot's summoning gem", (960196, "Gem of the Sky")
GEM_COUNT = 20
GEM_DESC = ("Magi'Kot's gift for the ultimate summoning: use it to be taken to the main isle of the Plane of Sky, "
            "where he waits. One use.")
GEM_LINE = "Self, Teleport to Sky Portal"
GATE = "sky portal"

OLD_BOLD_KINDS = {1: "reach", 2: "loot", 3: "give"}
OLD_AHRI_KINDS = {1: "talk", 2: "loot", 3: "give"}
BOLD_MOVE = {2: 4, 3: 5}
AHRI_MOVE = {2: 3, 3: 4}
OLD_TRIGGER_DOS = ["lock", "popup", "spawn", "clock", "awaitdead", "wait", "end", "despawn"]
OLD_AHRI_ABILITIES = [{"kind": "heal", "name": "Ultimate Renewal", "heal_pct": 100, "below": 100, "first": 3600,
                       "every": 3600}]
PORTFOE = {"kind": "portfoe", "name": "Ultimate Banishment", "every": 30, "first": 30, "chance": 50}
BOLD_TEMPLATE = {"class": 0, "hp_pct": 5367, "melee_pct": 2058, "special": "Critical Hit", "special_damage": 30.77,
                 "special_every": 80.0, "special_first": 6.0, "resist_kind": "FR,CR,LR,AR,PR,DR", "resist_pct": 75}
BOLD_LEASH = 1500.0


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def rows(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def named(db, name):
    return rows(db, "npcs", "lower(trim(npc_name)) = ? ORDER BY id", (name,))


class Plan:
    """What this database holds that the tool reads, found by names and places."""

    def __init__(self, db):
        self.bad = []
        for t in ("quests", "queststeps", "charquests", "npcs", "npcspawnonly", "classtemplates", "npctemplate",
                  "npcaggro", "npccharm", "gmtriggers", "itempattern", "itemeffect", "spellgates", "gm_audit"):
            if not has(db, t):
                self.bad.append(f"no {t} table here")
        if self.bad:
            return
        self.quests = {}  # (name, giver) -> questid
        for qn, kinds in ((BOLD_Q, OLD_BOLD_KINDS), (AHRI_Q, OLD_AHRI_KINDS)):
            for g in GIVERS:
                q = rows(db, "quests", "name = ? AND giver = ?", (qn, g))
                if len(q) != 1:
                    self.bad.append(f"{len(q)} quests named {qn!r} by {g!r}, want one")
                    continue
                qid = q[0]["questid"]
                steps = {r["step"]: r for r in rows(db, "queststeps", "questid = ?", (qid,))}
                got = {s: r["kind"] for s, r in steps.items()}
                if got != kinds:
                    self.bad.append(f"quest {qid} {qn!r} by {g!r} has steps {got}, want {kinds}")
                    continue
                self.quests[(qn, g)] = (qid, steps)
        if self.bad:
            return
        for g in GIVERS:
            bs = self.quests[(BOLD_Q, g)][1]
            if bs[2]["target"].lower() != BOLD or bs[3]["target"] != g:
                self.bad.append(f"{BOLD_Q!r} by {g!r}: step 2 {bs[2]['target']!r}, step 3 {bs[3]['target']!r}")
            a = self.quests[(AHRI_Q, g)][1]
            want3 = g if g == ROGUE_GIVER else "ghost of Magi'Kot"
            if a[1]["target"] != ET_GIVER or a[2]["target"].lower() != AHRI or a[3]["target"] != want3:
                self.bad.append(f"{AHRI_Q!r} by {g!r}: targets {[a[s]['target'] for s in (1, 2, 3)]}")
            if a[1]["npcid"] or a[3]["npcid"] or bs[1]["world"] != 2:
                self.bad.append(f"{g!r}'s steps are pinned or placed otherwise than this tool expects")
        if self.bad:
            return

        def one(name, world):
            r = [x for x in named(db, name) if x["world"] == world]
            if len(r) != 1:
                self.bad.append(f"{len(r)} rows named {name!r} in world {world}, want one")
                return None
            return r[0]
        self.bold, self.ahri, self.ghost = one(BOLD, 2), one(AHRI, 4), one(GHOST, 4)
        self.magi = one(MAGI, 0)
        if [x for x in named(db, MAGI) if x["world"] == 4]:
            self.bad.append("a Magi'Kot stands in world 4 already")
        if self.bad:
            return
        for r, near in ((self.bold, (11158, 5424)), (self.ahri, (4136, 5192)), (self.ghost, (4148, 5228))):
            if abs(r["x_coord"] - near[0]) > 30 or abs(r["z_coord"] - near[1]) > 30:
                self.bad.append(f"npcs {r['id']} {r['npc_name']!r} stands at ({r['x_coord']:.0f}, {r['z_coord']:.0f}), "
                                f"not by ({near[0]}, {near[1]}) as this tool expects")
        if row(db, "npcspawnonly", "lower(trim(name)) = ?", (BOLD,)):
            self.bad.append("Boldunder is spawn-only already")
        if row(db, "npcspawnonly", "lower(trim(name)) = ?", (AHRI,)) is None:
            self.bad.append("Ahriaana the Ultimate is not spawn-only")
        nt = row(db, "npctemplate", "name = ?", (BOLD,))
        if nt is None or nt["template"] != "roamer":
            self.bad.append(f"npctemplate boldunder is {nt and nt['template']!r}, want 'roamer'")
        if row(db, "classtemplates", "name = ?", (BOLD,)):
            self.bad.append("a classtemplates boldunder is there already")
        self.roamer = row(db, "classtemplates", "name = ?", ("roamer",))
        if self.roamer is None:
            self.bad.append("no classtemplates roamer")
        at = row(db, "classtemplates", "name = ?", (AHRI,))
        if at is None or json.loads(at["abilities"] or "[]") != OLD_AHRI_ABILITIES:
            self.bad.append(f"classtemplates {AHRI!r} abilities are {at and at['abilities']}, want {js(OLD_AHRI_ABILITIES)}")
        self.ahri_tmpl = at
        if row(db, "npcaggro", "npc_name = ?", (BOLD,)) or row(db, "npccharm", "name = ?", (BOLD,)):
            self.bad.append("npcaggro or npccharm has a boldunder row already")
        t = rows(db, "gmtriggers", "name = ?", (TRIGGER,))
        if len(t) != 1:
            self.bad.append(f"{len(t)} triggers named {TRIGGER!r}, want one")
        else:
            self.trig = t[0]
            steps = json.loads(self.trig["steps"])
            if [s.get("do") for s in steps] != OLD_TRIGGER_DOS or steps[2].get("npc") != f"#{self.ahri['id']}" or \
                    self.trig["world"] != 4 or steps[2].get("world") != 4:
                self.bad.append(f"trigger {self.trig['slot']} {TRIGGER!r} is not the summoning this tool expects: "
                                f"{self.trig['steps']}")
        g = row(db, "spellgates", "spell = ?", (GATE,))
        if g is None or g["world"] != 4 or abs(g["x"] - LANDING[0]) > 1 or abs(g["z"] - LANDING[2]) > 1:
            self.bad.append(f"spellgates {GATE!r} is not the main isle's landing: {g}")
        base = row(db, "itempattern", "patternid = ?", (GEM_BASE[0],))
        if base is None or base["itemname"] != GEM_BASE[1]:
            self.bad.append(f"itempattern {GEM_BASE[0]} is {base and base['itemname']!r}, not {GEM_BASE[1]!r}")
        self.gem_base = base
        if row(db, "itempattern", "patternid = ?", (GEM_ID,)) or rows(db, "itempattern", "lower(itemname) = lower(?)",
                                                                        (GEM_NAME,)):
            self.bad.append(f"itempattern {GEM_ID} or an item named {GEM_NAME!r} is there already")
        if row(db, "itemeffect", "patternid = ?", (GEM_ID,)):
            self.bad.append(f"itemeffect {GEM_ID} is there already")

    def qid(self, qn, g):
        return self.quests[(qn, g)][0]

    def old(self, qn, g):
        return self.quests[(qn, g)][1]


def popup(on, text):
    return {"on": on, "do": "popup", "text": text}


def reach_step(base, x, y, z, radius, journal, events):
    r = dict(base)
    r.update({"kind": "reach", "target": "", "npcid": 0, "items": "", "count": 1, "world": 2, "x": x, "y": y, "z": z,
              "radius": radius, "height": 0.0, "journal": journal, "says": "", "chance": 0.0, "events": js(events)})
    return r


def new_steps(p, sky_magi_id):
    """{questid: {step: row}} as the quests will be."""
    out = {}
    for g in GIVERS:
        # Boldunder: three reach steps, the soul, the hand-in
        old = p.old(BOLD_Q, g)
        b1 = reach_step(old[1], *HILL_FOOT, 25.0,
                        "Go to the North Barren Coast, in the deserts of Odus, to the foot of the hill beside the great "
                        "depression in its middle.",
                        [popup("done", "You feel the ground rumbling under your feet.")])
        b2 = reach_step(old[1], *HALF_WAY, 20.0,
                        "The ground rumbles. Walk away from the hill, down toward the depression.",
                        [popup("done", "The rumbling grows stronger. Something stirs beneath the depression!")])
        bx, by, bz = BOWL
        spawn = {"on": "done", "do": "spawn", "npc": "Boldunder", "hostile": True, "group": True, "minutes": BOLD_LIFE,
                 "world": 2, "x": bx, "y": by, "z": bz}
        b3 = reach_step(old[1], *BOWL, 20.0, "Go down into the middle of the depression.",
                        [popup("done", "The ground splits open, and Boldunder rises from the depression!"), spawn])
        b4 = dict(old[2], journal="Kill Boldunder, risen from the depression in the North Barren Coast, and take his "
                                  "soul. He resists nukes and hits hard: bring a good-sized force.")
        b5 = dict(old[3])
        out[p.qid(BOLD_Q, g)] = {1: b1, 2: b2, 3: b3, 4: b4, 5: b5}

        # Ahriaana: the gems and the port, the summoning at Magi'Kot on the main isle, the page, the hand-in
        old = p.old(AHRI_Q, g)
        lx, ly, lz = LANDING
        a1 = dict(old[1], npcid=p.magi["id"],
                  journal="Speak with Magi'Kot in the west of the Elemental Towers: he gives you gems for your raid "
                          "and takes you to the Plane of Sky.",
                  says="Gather twenty or more. Take these gems and hand them out: each carries one to the main isle "
                       "of the Plane of Sky. I go ahead and wait for you there.",
                  events=js([{"on": "done", "do": "give", "items": f"{GEM_ID}*{GEM_COUNT}", "unless_held": True},
                             {"on": "done", "do": "teleport", "world": 4, "x": lx, "y": ly, "z": lz}]))
        a2 = dict(old[1], npcid=sky_magi_id,
                  journal="When your raid is ready, speak with Magi'Kot on the Plane of Sky's main isle, by the Isle "
                          "of Wonder portal: he calls Ahriaana the Ultimate.",
                  says="Then witness the ultimate summoning!",
                  events=js([{"on": "done", "do": "trigger", "trigger": TRIGGER,
                              "text": "Ahriaana the Ultimate walks the island already. Join the fight, or wait until "
                                      "she falls."}]))
        a3 = dict(old[2], journal="Kill Ahriaana the Ultimate. She sends whoever holds her anger away, and is whole "
                                  "again if she lives an hour: pets are the way.")
        a4 = dict(old[3])
        if g == ET_GIVER:
            a4["journal"] = ("Bring the torn journal page to Magi'Kot's ghost, north-west of the Isle of Wonder portal "
                             "on the Plane of Sky's main isle.")
        out[p.qid(AHRI_Q, g)] = {1: a1, 2: a2, 3: a3, 4: a4}
    for qid, steps in out.items():
        for s, r in steps.items():
            r["questid"], r["step"] = qid, s
    return out


QUEST_NOTE = {
    BOLD_Q: (" Steps 1-3 (2026-11-18, " + POST + "): the hill's foot, half way down, the depression's middle, a popup "
             "each; the third raises Boldunder there, one for a group (board 692: spawn from quest). "
             "tools/evil_melee_20261118.py."),
    AHRI_Q: (" Steps 1-2 (2026-11-18, " + POST + "): Magi'Kot gives 20 summoning gems (tradeable, to the main isle) "
             "and takes the quester to the main isle; Magi'Kot there calls Ahriaana by the Isle of Wonder portal; she "
             "sends her tank to its bind point (board 615). tools/evil_melee_20261118.py."),
}


def report(db):
    for qn in (BOLD_Q, AHRI_Q):
        for g in GIVERS:
            for q in rows(db, "quests", "name = ? AND giver = ?", (qn, g)):
                print(f"  quest {q['questid']} {qn!r} by {g!r}")
                for r in db.execute("SELECT step, kind, target, npcid, round(x), round(y), round(z), radius, events "
                                    "FROM queststeps WHERE questid = ? ORDER BY step", (q["questid"],)):
                    print(f"    step {r[0]} {r[1]} {r[2]!r} pin {r[3]} at ({r[4]}, {r[5]}, {r[6]}) r{r[7]} events {r[8]!r}")
                n = db.execute("SELECT step, count(*) FROM charquests WHERE questid = ? GROUP BY step",
                               (q["questid"],)).fetchall()
                print(f"    characters by step: {dict(n)}")
    for n in (BOLD, AHRI, GHOST, MAGI):
        print(f"  {n}: {[(r['id'], r['world'], round(r['x_coord']), round(r['y_coord']), round(r['z_coord'])) for r in named(db, n)]}")
    nt = row(db, "npctemplate", "name = ?", (BOLD,))
    print(f"  boldunder's template: {nt and nt['template']}; spawn-only: "
          f"{[r[0] for r in db.execute('SELECT name FROM npcspawnonly WHERE lower(name) IN (?, ?)', (BOLD, AHRI))]}")


def apply(db, p):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")
    n = [0]

    def done(tbl, key, before):
        where = " AND ".join(f"{c} = ?" for c in key)
        after = row(db, tbl, where, tuple(key.values()))
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js(key), js(before), js(after)))
        if tbl != "sqlite_sequence":
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                       (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))
        n[0] += 1

    def insert(tbl, values, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})', list(values.values()))
        done(tbl, key, None)

    def update(tbl, key, values):
        where = " AND ".join(f"{c} = ?" for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {where}',
                   list(values.values()) + list(key.values()))
        done(tbl, key, before)

    # Magi'Kot on the main isle: his body, a row of its own (the autoincrement's mark logged, so --undo is exact)
    seq = row(db, "sqlite_sequence", "name = ?", ("npcs",))
    new_id = max(seq["seq"] if seq else 0, db.execute("SELECT COALESCE(MAX(id), 0) FROM npcs").fetchone()[0]) + 1
    x, y, z, facing = SKY_MAGI
    zone = db.execute("SELECT zone FROM npcs WHERE world = 4 ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * "
                      "(z_coord - ?) LIMIT 1", (x, x, z, z)).fetchone()[0]  # the main isle's (the zone column is unread)
    magi = dict(p.magi, id=new_id, world=4, x_coord=x, y_coord=y, z_coord=z, facing=facing, npc_id=0, zone=zone)
    insert("npcs", magi, {"id": new_id})
    if seq:
        after = row(db, "sqlite_sequence", "name = ?", ("npcs",))
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)",
                   ("sqlite_sequence", js({"name": "npcs"}), js(seq), js(after)))

    # the quests' steps (new rows for the steps past the old three), their characters moved with them
    steps = new_steps(p, new_id)
    for qid, ss in steps.items():
        for s, r in sorted(ss.items()):
            if row(db, "queststeps", "questid = ? AND step = ?", (qid, s)):
                update("queststeps", {"questid": qid, "step": s}, {k: v for k, v in r.items() if k not in ("questid", "step")})
            else:
                insert("queststeps", r, {"questid": qid, "step": s})
    for qn, move in ((BOLD_Q, BOLD_MOVE), (AHRI_Q, AHRI_MOVE)):
        for g in GIVERS:
            qid = p.qid(qn, g)
            q = row(db, "quests", "questid = ?", (qid,))
            update("quests", {"questid": qid}, {"notes": q["notes"] + QUEST_NOTE[qn]})
            for c in rows(db, "charquests", "questid = ? AND step IN (?, ?) ORDER BY serverid", (qid, *move)):
                update("charquests", {"serverid": c["serverid"], "questid": qid}, {"step": move[c["step"]]})

    # Boldunder: spawn-only, at the bowl's middle, his own template, leash, uncharmable
    insert("npcspawnonly", {"name": "Boldunder", "note": f"the epic-sode 3 quests raise him in the depression ({POST}; "
                                                        f"board 692): {SRC}", "source": SRC}, {"name": "Boldunder"})
    bx, by, bz = BOWL
    update("npcs", {"id": p.bold["id"]}, {"x_coord": bx, "y_coord": by, "z_coord": bz})
    insert("classtemplates", dict(p.roamer, name=BOLD, **BOLD_TEMPLATE, source=SRC,
                                  note="Boldunder (board 636; 676's hp_pct 5367 ~900k): 1,500 a hit on a warrior "
                                       "(melee_pct 2058: entry 1404's 3430 for 2,500, in proportion: ours), a 2,000 "
                                       "Critical Hit (30.77 a level) every 80 s from 6 s, all six resists 75% ('He "
                                       f"resists all nukes'; 636's open question 3: 75 or 100); {SRC}"), {"name": BOLD})
    update("npctemplate", {"name": BOLD}, {"template": BOLD, "note": f"board 636: his own; {SRC}", "source": SRC})
    insert("npcaggro", {"npc_name": BOLD, "aggressive": -1, "leash": BOLD_LEASH,
                        "note": f"board 636 code ask 1: the era kited him across the coast (1500 ours); {SRC}",
                        "source": SRC}, {"npc_name": BOLD})
    insert("npccharm", {"name": BOLD, "charmable": 0, "note": f"board 636: uncharmable; {SRC}", "source": SRC},
           {"name": BOLD})

    # Ahriaana: the summoning on the main isle, her row there, her port of the one she fights; the ghost moved
    sx, sy, sz, sf = SUMMON
    update("npcs", {"id": p.ahri["id"]}, {"x_coord": sx, "y_coord": sy, "z_coord": sz, "facing": sf})
    gx, gy, gz, gf = GHOST_AT
    update("npcs", {"id": p.ghost["id"]}, {"x_coord": gx, "y_coord": gy, "z_coord": gz, "facing": gf})
    tsteps = json.loads(p.trig["steps"])
    tsteps[2].update({"x": sx, "y": sy, "z": sz})
    update("gmtriggers", {"slot": p.trig["slot"]},
           {"x": sx, "y": sy, "z": sz, "steps": js(tsteps),
            "notes": p.trig["notes"] + f"; moved to the main isle by the Isle of Wonder portal ({POST}): {SRC}"})
    update("classtemplates", {"name": AHRI}, {"abilities": js(OLD_AHRI_ABILITIES + [PORTFOE]),
                                             "note": p.ahri_tmpl["note"] + " 2026-11-18: her port of the one she "
                                             "fights (board 615 code ask 1: a player to its bind point every 30 s at "
                                             f"50%, pets never: kind portfoe); {SRC}"})

    # the gem: the Gem of the Sky's record, tradeable, a stack of 20, spent a use
    gem = dict(p.gem_base, patternid=GEM_ID, itemname=GEM_NAME, itemdesc=GEM_DESC, trade=0, lore=0, maxstack=GEM_COUNT,
               procanim=7, classuse=32767, raceuse=32767, itemcost=0)
    insert("itempattern", gem, {"patternid": GEM_ID})
    insert("itemeffect", {"patternid": GEM_ID, "line": GEM_LINE, "name": GEM_NAME, "source": SRC}, {"patternid": GEM_ID})
    return n[0]


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n = 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        where = " AND ".join(f"{c} = ?" for c in key)
        now = row(db, tbl, where, tuple(key.values()))
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(b)}) VALUES ({", ".join("?" * len(b))})', list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back")
    return n


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--applied", action="store_true", help="say whether it is applied, and what it wrote")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.applied:
            if not has(db, LOG):
                print("not applied")
                return 1
            print("applied:")
            for tbl, key, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq"):
                print(f"  {tbl} {key}: {'insert' if before is None else 'update' if after else 'delete'}")
            return 0
        report(db)
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone; takes effect at a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        p = Plan(db)
        if p.bad:
            print("refused, nothing done:\n  " + "\n  ".join(p.bad))
            return 1
        print(f"  would write: quests {sorted(q for q, _ in p.quests.values())}; Boldunder {p.bold['id']}, Ahriaana "
              f"{p.ahri['id']}, the ghost {p.ghost['id']}, Magi'Kot {p.magi['id']} (copied to world 4), trigger "
              f"{p.trig['slot']}, the gem {GEM_ID} (from {GEM_BASE[0]})")
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied, {n} rows; takes effect at the restart that brings branch evil-melee's code")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
