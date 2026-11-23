#!/usr/bin/env python3
"""Heirloom petitions of 2026-11-23 (the fourth batch of the day, 2771-2795), as data.

Every row is found by its name (and what else names it), never by an id alone, and must read as it did when this
was made; a row already as this writes it is left ("already"); anything else refuses the whole run. Every new place
was asked of the server's own ground files (cmd/campground: the floor a spawn is set down on, not in the rock, and no
wall between it and the creature it copies).

  2771  the monk's Punch line (the owner's yes): every Punch rank's recast 30 s (Freezing and Fiery Punch were 90),
        every Focused Strike version's 15 s (Ember and Lightning Strike were 30, Freezing and Fiery Strike 45).
  2773  the city level 17 quests that begin a chain (none requires another) are flagged for the quest marks
        (questmarks), as each race's and class's first quest is: their givers show a "!" (Scribe Cathassa's
        The Scribe's Inks among them). Highpass's part 2 follows part 1 by itself (markedWithFollowUps).
  2776  Captain Scilond's Scouts: the Klick'Anon stage's deadly corrosions (level 15) come at the player's level and
        the Highpass stage's pickclaw warriors (19) one above it (the event's con), so the stages climb as the
        giants' before them did (40-62).
  2777  the bearwere's Mirin: a reach step first, at his spawn (radius 60 and the kill's journal line, work board
        800's), that hides him and raises a hostile copy three over the player for fifteen minutes, as the gatorwere's
        Julian and the ratwere's Trissen do; then the kill, then Dorandas. Characters on a step past the first move
        with it. (800's companion row and spawn-only Mirin are not built: no new spawns here.)
  2778  Mark of Anguish (cleric 46, its scroll 716271) in every Spell Archivist's shop, as petition 1521 put the
        shadowknight's Shroud of Death there: his "Grant basic spells" writes it, and Inquisitor's Brand 3 (which
        the Exorcist mastery requires) can be bought.
  2781  The Test of Greed's golem of greed stood behind Dark Solace's wall, out of reach: its spawn moved beside the
        Stone Golem where the player stood, on the floor, 11 from Windle Geraprofit (work board 1071's own spot,
        (12590, 90, 21200), is inside the ground).
  2796  Goblin Attackers: Honjour (group 6 evil, level 40; 2797 names it): talking to Watcher Gabnu (step 2) fires
        a trigger at the foot of his tower's ramp: four waves of two froglok elites, each after the last falls, then
        The Captain, whose kill gives the shard (step 3). The two standing froglok elites and The Captain are
        spawn-only (by row) so they come only in the waves; the goblin elites stand as before. Lines and the
        30-minute clock are ours.
  (no petition) the Eye of the Sphynx takes CLIENT's gem type, as on Live (Live petition 5547).
  2785  Tobul: his spawn where the player stood among the zintec zombies, and his wander from 250 to the 40 his
        kind's rule gives (npcmove "tobul", work board 936).
  2788  the three mindwhippers inside Klick'Anon's gate moved out to the hive (Hivewhipper's mound), spread round it.
  2795  the lionwere's Lionell Redrick stood half in the Salt Mines room's west wall: his spawn moved to the room's
        middle, and the first ambush's three Siloth knights (step 2) come at three fixed places in the room's open
        floor, not round the player at the wall.

Every row written is a gm_audit line (via "tool") and a row of petpass_heirloom_1123d_log; --undo puts each back
as it was (a row changed since is left and said) and drops the log. A report by default; --apply writes once (a
second finds the log and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123d)
unless --no-backup.

WHEN IT TAKES EFFECT: the Eye of the Sphynx at each holder's next login; the recasts at each player's next login (the book's timers) and next cast (the server's);
the archivist's shop at its next opening and his grant within 10 minutes (archiveTTL); npcs, npcwander, queststeps,
questmarks, npcspawnonly and the trigger at the server's next start.

    python3 -E -s tools/petpass_heirloom_1123d.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123d.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123d.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123d_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petitions 2771-2795 (2026-11-23): tools/petpass_heirloom_1123d.py"
SOURCE = "petpass-1123d"

# 2771: name -> (spellid, old recast, new recast)
RECASTS = {
    "Freezing Punch": (700498, 90, 30), "Fiery Punch": (700711, 90, 30),
    "Ember Strike": (700841, 30, 15), "Lightning Strike": (900798, 30, 15),
    "Freezing Strike": (900839, 45, 15), "Fiery Strike": (701076, 45, 15),
}
MONK = 4

# 2773: the city level 17 chain heads (import key 17/<city>/...)
MARK_NOTE = "petition 2773: a city level 17 quest that begins its chain (the first of 17/%s)"

# 2776: Captain Scilond's Scouts' stages (step, creature, con)
SCILOND = "Captain Scilond's Scouts"
SCILOND_CON = [(22, "deadly corrosion", "player"), (26, "pickclaw warrior", "player+1")]

# 2777: Mirin
MIRIN_Q, MIRIN_GIVER = "Mirin", "Dorandas"
MIRIN_SPAWN = (6350.887, 77.7857, 7556.969)
MIRIN_EVENTS = [
    {"on": "done", "do": "hide", "npc": "Mirin", "minutes": 15},
    {"on": "done", "do": "spawn", "npc": "Mirin", "hostile": True, "con": "player+3", "group": True, "minutes": 15},
    {"on": "done", "do": "say", "npc": "Mirin",
     "text": "Dorandas sends a cub to take what is mine? Then let the bear decide it."},
]
MIRIN_RADIUS = 60.0  # work board 800's

# 2778: Mark of Anguish's scroll in the archivists' shops, beside Shroud of Death's (petition 1521)
ANGUISH, ANGUISH_SPELL, SHROUD = "Mark of Anguish", 700541, "Shroud of Death"
ARCHIVIST = "Spell Archivist"

# Eye of the Sphynx (no petition: Live petition 5547's change, the coordinator's ask): CLIENT's gem type, usable
# from the pack, as Magi'Kot's summoning gem (itempattern.procanim 0 -> 7; tools/petpass_live_1123d.py)
EYE, EYE_NAME, PLAIN_TYPE, GEM_TYPE = 701463, "Eye of the Sphynx", 0, 7

# 2781: the golem of greed
GREED = ("golem of greed", (12677.615, 108.898, 21144.392), (12574.2, 89.55, 21195.9))
GREED_Q = "The Test of Greed"

# 2785: Tobul
TOBUL = ("Tobul", (13569.7, 55.2578, 23606.3), (13562.026, 55.26, 23605.48))
TOBUL_WANDER = (250.0, 40.0)

# 2788: mindwhippers: (old, new)
WHIPPERS = [
    ((22344.202, 54.156, 6415.742), (20990.0, 54.52, 7180.0)),
    ((22357.579, 54.156, 6407.972), (20860.0, 53.64, 7180.0)),
    ((22358.606, 54.156, 6430.506), (20960.0, 52.55, 7140.0)),
]
HIVE = ("Hivewhipper", (20903.0, 7165.0))

# 2795: Lionell Redrick and the first ambush
LIONELL = ("Lionell Redrick", (14516.217, 30.5625, 19033.084), (14520.7, 30.5625, 19031.6))
LION_Q = "Lionell Redrick"
AMBUSH = [(14519.2, 30.5625, 19029.6), (14523.7, 30.5625, 19033.1), (14522.2, 30.5625, 19030.6)]

# 2796/2797: Goblin Attackers: Honjour (group 6 evil, level 40): the attack on Watcher Gabnu's tower in waves
HONJOUR_Q, HONJOUR_GIVER = "Goblin Attackers: Honjour", "Count Luvincius"
FROGLOKS = [(122636, "froglok elite", (9944.3, 15275.6)), (122637, "froglok elite", (9925.0, 15255.6))]
CAPTAIN = (122638, "The Captain", (9933.4, 15257.4))
RAMP = (9931.5, 57.66, 15297.6)  # the foot of the tower's ramp, on the floor (cmd/campground)
WAVES, TRIGGER_FROM = 4, 300
TRIGGER = "Honjour Attackers"
TRIGGER_BUSY = "The attack on the tower has begun already: hold the foot of the ramp until it is beaten."
HONJOUR_JOURNAL = ("Hold the foot of Watcher Gabnu's ramp: the froglok elites come two at a time, then their "
                   "captain. Kill the captain for a glowing silver shard.")


def honjour_steps():
    w, (x, y, z) = 0, RAMP
    at = {"world": w, "x": x, "y": y, "z": z}
    out = [{"do": "lock", "lock": "honjour attackers"}, {"do": "clock", "seconds": 1800, "empty": 300}]
    for i in range(1, WAVES + 1):
        out.append({"do": "popup", "text": f"Two froglok elites charge the foot of the tower! (wave {i} of {WAVES})"})
        out.append(dict({"do": "spawn", "npc": f"#{FROGLOKS[(i - 1) % 2][0]}", "count": 2, "spread": 3, "minutes": 30,
                         "hostile": True}, **at))
        out.append({"do": "awaitdead"})
    out.append({"do": "popup", "text": "The Captain who leads them comes for the tower himself!"})
    out.append(dict({"do": "spawn", "npc": f"#{CAPTAIN[0]}", "count": 1, "minutes": 30, "hostile": True}, **at))
    out.append({"do": "awaitdead"})
    out.append({"do": "end"})
    return json.dumps(out, separators=(",", ":"))


KEYS = {"itempattern": ("patternid",), "spellpattern": ("spellid",), "questmarks": ("questid",), "queststeps": ("questid", "step"),
        "npcs": ("id",), "npcwander": ("npcid",), "npcitem": ("itemid",), "charquests": ("serverid", "questid"),
        "npcspawnonly": ("name",), "gmtriggers": ("slot",)}
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""
AUDIT_DDL = """CREATE TABLE IF NOT EXISTS gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
    username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)"""


class Refused(Exception):
    pass


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, what, sql, args=()):
    got = rows(db, sql, args)
    if len(got) != 1:
        raise Refused(f"{what}: {len(got)} rows, want one")
    return got[0]


def near(a, b):
    return abs(float(a) - float(b)) < 0.05


def same(a, b):
    if isinstance(b, float) and a is not None and not isinstance(a, str):
        return near(a, b)
    return a == b


def events(*evs):
    return json.dumps(list(evs), separators=(",", ":"))


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row or None, values)
        self.lines = []

    def set(self, tbl, row, values, olds, what):
        """values over row, when each column of olds reads its old; already as values: left."""
        if all(same(row[c], v) for c, v in values.items()):
            self.lines.append(f"already  {what}")
            return
        for c, old in olds.items():
            if not same(row[c], old):
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {old!r} as read when this was made")
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(
            f"{c} {str(row[c])[:60]!r} -> {str(v)[:60]!r}" for c, v in values.items()))

    def insert(self, tbl, values, what, present):
        """a new row, unless present (a row already there as wanted: left; another: refused)."""
        if present is not None:
            if all(same(present[c], v) for c, v in values.items()):
                self.lines.append(f"already  {what}")
                return
            raise Refused(f"{what}: a row is there already: {present}")
        self.changes.append((tbl, None, None, dict(values)))
        self.lines.append(f"insert   {what}")

    def move(self, pet, world, npc_id, name, old, new, what):
        r = one(self.db, f"{pet} {what}", "SELECT * FROM npcs WHERE id = ? AND lower(npc_name) = ? AND world = ?",
                (npc_id, name.lower(), world))
        self.set("npcs", r, {"x_coord": new[0], "y_coord": new[1], "z_coord": new[2]},
                 {"x_coord": old[0], "y_coord": old[1], "z_coord": old[2]}, f"{pet} {what} ({npc_id})")

    def named_at(self, pet, name, old, world=0):
        return one(self.db, f"{pet} {name} at ({old[0]}, {old[2]})", """SELECT * FROM npcs WHERE npc_name = ? AND
            world = ? AND abs(x_coord - ?) < 0.05 AND abs(z_coord - ?) < 0.05""", (name, world, old[0], old[2]))

    def build(self):
        db = self.db
        self.recasts()
        self.marks()
        self.scilond()
        self.mirin()
        self.anguish()
        eye = one(db, EYE_NAME, "SELECT * FROM itempattern WHERE patternid = ? AND itemname = ?", (EYE, EYE_NAME))
        self.set("itempattern", eye, {"procanim": GEM_TYPE}, {"procanim": PLAIN_TYPE},
                 f"{EYE_NAME} ({EYE}): CLIENT's gem type, usable from the pack (Live petition 5547's change)")
        # 2781
        one(db, GREED_Q, "SELECT questid FROM quests WHERE name = ?", (GREED_Q,))
        name, old, new = GREED
        r = self.found("2781", name, old, new)
        self.move("2781", 0, r["id"], name, old, new, "the golem of greed's spawn, out from behind the wall")
        # 2785
        name, old, new = TOBUL
        r = self.found("2785", name, old, new)
        self.move("2785", 0, r["id"], name, old, new, "Tobul's spawn where the player stood")
        w = one(db, "Tobul's npcwander", "SELECT * FROM npcwander WHERE npcid = ?", (r["id"],))
        self.set("npcwander", w, {"radius": TOBUL_WANDER[1]}, {"radius": TOBUL_WANDER[0]},
                 f"2785 Tobul's wander {TOBUL_WANDER[0]:g} -> {TOBUL_WANDER[1]:g}")
        # 2788
        hn, (hx, hz) = HIVE
        one(db, "the Hivewhipper", "SELECT id FROM npcs WHERE npc_name = ? AND world = 0 AND abs(x_coord - ?) < 1 AND "
            "abs(z_coord - ?) < 1", (hn, hx, hz))
        for old, new in WHIPPERS:
            r = self.found("2788", "mindwhipper", old, new)
            self.move("2788", 0, r["id"], "mindwhipper", old, new, "a mindwhipper from Klick'Anon's gate to the hive")
        self.lionell()
        self.honjour()

    def honjour(self):
        db = self.db
        q = one(db, HONJOUR_Q, "SELECT questid FROM quests WHERE name = ? AND giver = ?", (HONJOUR_Q, HONJOUR_GIVER))
        for rid, name, (x, z) in FROGLOKS + [CAPTAIN]:
            one(db, f"2796 {name} {rid}", "SELECT id FROM npcs WHERE id = ? AND npc_name = ? AND world = 0 AND "
                "abs(x_coord - ?) < 0.1 AND abs(z_coord - ?) < 0.1", (rid, name, x, z))
            have = rows(db, "SELECT * FROM npcspawnonly WHERE name = ?", (f"#{rid}",))
            self.insert("npcspawnonly", {"name": f"#{rid}", "note": f"petition 2796: {name} comes only in the "
                        f"Honjour tower's waves ({TRIGGER})", "source": SOURCE},
                        f"2796 npcspawnonly #{rid} ({name})", have[0] if have else None)
        steps = honjour_steps()
        have = rows(db, "SELECT * FROM gmtriggers WHERE name = ?", (TRIGGER,))
        slot = have[0]["slot"] if have else max([TRIGGER_FROM - 1] + [r["slot"] for r in rows(
            db, "SELECT slot FROM gmtriggers WHERE slot >= ? AND slot < ?", (TRIGGER_FROM, TRIGGER_FROM + 100))]) + 1
        self.insert("gmtriggers", {"slot": slot, "name": TRIGGER, "enabled": 1, "scope": "near", "radius": 80.0,
                                   "world": 0, "x": RAMP[0], "y": RAMP[1], "z": RAMP[2], "steps": steps,
                                   "notes": "petition 2796: Goblin Attackers: Honjour's attack on Watcher Gabnu's "
                                            "tower, fired by quest step 2's done (tools/petpass_heirloom_1123d.py)"},
                    f"2796 trigger {slot} {TRIGGER}: {WAVES} waves of two froglok elites, then The Captain",
                    have[0] if have else None)
        s2 = one(db, "2796 step 2", "SELECT * FROM queststeps WHERE questid = ? AND step = 2 AND kind = 'talk' AND "
                 "target = 'Watcher Gabnu'", (q["questid"],))
        ev = events({"on": "done", "do": "trigger", "trigger": TRIGGER, "text": TRIGGER_BUSY})
        self.set("queststeps", s2, {"events": ev}, {"events": ""}, "2796 step 2's done fires the attack")
        s3 = one(db, "2796 step 3", "SELECT * FROM queststeps WHERE questid = ? AND step = 3 AND kind = 'loot' AND "
                 "target = 'The Captain'", (q["questid"],))
        self.set("queststeps", s3, {"journal": HONJOUR_JOURNAL},
                 {"journal": "Kill the goblin and froglok elites and their captain until you have a glowing silver shard."},
                 "2796 step 3's journal")

    def found(self, pet, name, old, new):
        """the row of the name at old, or at new when this moved it already."""
        return self.named_at(pet, name, new if self.already_at(name, new) else old)

    def already_at(self, name, at):
        return bool(rows(self.db, """SELECT id FROM npcs WHERE npc_name = ? AND world = 0 AND abs(x_coord - ?) < 0.05
            AND abs(z_coord - ?) < 0.05""", (name, at[0], at[2])))

    def recasts(self):
        for name, (sid, old, new) in RECASTS.items():
            r = one(self.db, f"2771 {name}", "SELECT * FROM spellpattern WHERE spellid = ? AND spellname = ?", (sid, name))
            one(self.db, f"2771 {name} a monk's", "SELECT 1 FROM classspells WHERE class = ? AND spellid = ?", (MONK, sid))
            self.set("spellpattern", r, {"recast": new}, {"recast": old}, f"2771 {name} ({sid}) recast")

    def marks(self):
        heads = rows(self.db, """SELECT k.key, q.questid, q.name, q.requires FROM quest_import_quests k
            JOIN quests q USING(questid) WHERE k.key LIKE '17/%' AND q.enabled = 1 ORDER BY k.key""")
        heads = [h for h in heads if not str(h["requires"] or "").strip()]
        if len(heads) < 11:
            raise Refused(f"2773: {len(heads)} level 17 chain heads, want the 11 cities' (or more)")
        for h in heads:
            have = rows(self.db, "SELECT * FROM questmarks WHERE questid = ?", (h["questid"],))
            city = h["key"].split("/")[1]
            self.insert("questmarks", {"questid": h["questid"], "note": MARK_NOTE % city},
                        f"2773 questmarks {h['questid']} {h['name']} ({h['key']})",
                        have[0] if have else None)

    def scilond(self):
        q = one(self.db, SCILOND, "SELECT questid FROM quests WHERE name = ? AND giver = 'Captain Scilond'", (SCILOND,))
        for step, npc, con in SCILOND_CON:
            st = one(self.db, f"2776 step {step}", "SELECT * FROM queststeps WHERE questid = ? AND step = ? AND "
                     "kind = 'loot' AND target = ?", (q["questid"], step, npc))
            old = events({"on": "start", "do": "spawn", "npc": npc, "hostile": True, "count": 5})
            new = events({"on": "start", "do": "spawn", "npc": npc, "hostile": True, "count": 5, "con": con})
            self.set("queststeps", st, {"events": new}, {"events": old},
                     f"2776 {SCILOND} step {step}: the {npc}s at con {con}")

    def mirin(self):
        db = self.db
        q = one(db, MIRIN_Q, "SELECT * FROM quests WHERE name = ? AND giver = ?", (MIRIN_Q, MIRIN_GIVER))
        qid = q["questid"]
        steps = rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (qid,))
        shape = [(s["step"], s["kind"], s["target"]) for s in steps]
        reach = {"step": 1, "kind": "reach", "target": "", "world": 0, "x": MIRIN_SPAWN[0], "y": MIRIN_SPAWN[1],
                 "z": MIRIN_SPAWN[2], "radius": MIRIN_RADIUS, "journal": steps[0]["journal"], "events": events(*MIRIN_EVENTS)}
        if shape == [(1, "reach", ""), (2, "kill", "Mirin"), (3, "talk", "Dorandas")]:
            self.lines.append("already  2777 Mirin's reach step")
            return
        if shape != [(1, "kill", "Mirin"), (2, "talk", "Dorandas")]:
            raise Refused(f"2777: Mirin's steps read {shape}")
        # characters past step 1 move with their step (done ones have step 0)
        for c in rows(db, "SELECT * FROM charquests WHERE questid = ? AND step >= 2 AND done = 0", (qid,)):
            self.set("charquests", c, {"step": c["step"] + 1}, {"step": c["step"]},
                     f"2777 character {c['serverid']} on Mirin's step {c['step']} -> {c['step'] + 1}")
        s2, s1 = steps[1], steps[0]
        self.set("queststeps", s2, {"step": 3}, {"step": 2}, "2777 Mirin step 2 (Dorandas) -> 3")
        self.set("queststeps", s1, {"step": 2}, {"step": 1}, "2777 Mirin step 1 (the kill) -> 2")
        row = {c: v for c, v in s1.items()}
        row.update(reach)
        row["questid"] = qid
        for c in ("target", "items", "answer", "says", "waiting", "notyet", "reminder", "rewarditems", "factions",
                  "spells", "byclass", "choices"):
            if c in row and c not in reach:
                row[c] = ""
        for c in ("npcid", "tunar", "xp", "rewardtunar", "anyorder", "exactbase"):
            if c in row:
                row[c] = 0
        for c in ("chance", "height"):
            if c in row:
                row[c] = 0.0
        if "count" in row:
            row["count"] = 1
        self.insert("queststeps", row, "2777 Mirin step 1: a reach at his spawn raises his hostile copy", None)

    def anguish(self):
        db = self.db
        sc = one(db, f"2778 {ANGUISH}'s scroll", """SELECT * FROM itempattern WHERE itemname = ? AND classuse = 512 AND
            levelreq = 46 AND equipslot = -1""", (ANGUISH,))
        one(db, f"2778 {ANGUISH}", "SELECT 1 FROM spellpattern s JOIN classspells c USING(spellid) WHERE "
            "s.spellid = ? AND s.spellname = ? AND c.class = 9", (ANGUISH_SPELL, ANGUISH))
        shroud = one(db, f"2778 {SHROUD}'s scroll", "SELECT patternid FROM itempattern WHERE itemname = ? AND "
                     "levelreq = 46 AND equipslot = -1", (SHROUD,))
        archs = [r["npc_id"] for r in rows(db, "SELECT DISTINCT npc_id FROM npcs WHERE npc_name = ? AND npc_id != 0 "
                                               "AND (npc_type & 1) = 1 ORDER BY npc_id", (ARCHIVIST,))]
        if not archs:
            raise Refused("2778: no Spell Archivist")
        for a in archs:
            tmpl = rows(db, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (a, shroud["patternid"]))
            if len(tmpl) != 1:
                raise Refused(f"2778: archivist {a} holds {len(tmpl)} {SHROUD} rows, want one")
            have = rows(db, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (a, sc["patternid"]))
            vals = {c: v for c, v in tmpl[0].items() if c != "itemid"}
            vals["patternid"] = sc["patternid"]
            self.insert("npcitem", vals, f"2778 {ANGUISH} ({sc['patternid']}) in archivist {a}'s shop",
                        have[0] if have else None)

    def lionell(self):
        db = self.db
        name, old, new = LIONELL
        r = self.found("2795", name, old, new)
        self.move("2795", 0, r["id"], name, old, new, "Lionell Redrick's spawn, out of the wall")
        q = one(db, LION_Q, "SELECT questid FROM quests WHERE name = ?", (LION_Q,))
        st = one(db, "2795 the first ambush", "SELECT * FROM queststeps WHERE questid = ? AND step = 2 AND kind = 'kill'"
                 " AND target = 'Siloth knight'", (q["questid"],))
        knight = {"on": "start", "do": "spawn", "npc": "Siloth knight", "hostile": True}
        old_ev = events(knight, knight, knight)
        new_ev = events(*[dict(knight, world=0, x=x, y=y, z=z) for x, y, z in AMBUSH])
        self.set("queststeps", st, {"events": new_ev}, {"events": old_ev},
                 "2795 Lionell's first ambush at three places in the room")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def keyof(tbl, row):
    return {k: row[k] for k in KEYS[tbl]}


def apply(db, plan):
    db.execute(LOG_DDL)
    for tbl, key, before, values in plan.changes:
        if before is None:
            cols = list(values)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [values[c] for c in cols])
            if all(k in values for k in KEYS[tbl]):
                key = keyof(tbl, values)
            else:  # a new row's id is the one made
                key = {KEYS[tbl][0]: cur.lastrowid}
            after = rows(db, f"SELECT * FROM {tbl} WHERE {where(key)[0]}", where(key)[1])[0]
            a = audit(db, "insert", tbl, key, None, after)
        else:
            w, args = where(key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            nkey = {k: values.get(k, v) for k, v in key.items()}  # a step renumbered keys anew
            after = rows(db, f"SELECT * FROM {tbl} WHERE {where(nkey)[0]}", where(nkey)[1])[0]
            a = audit(db, "update", tbl, nkey, before, after)
            key = nkey
        db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None, json.dumps(after), a, now()))


def undo(db):
    out = []
    for r in rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        after = json.loads(r["after"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        if r["before"] is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, after, None)
        else:
            before = json.loads(r["before"])
            cols = list(before)  # every column, the key's too (a step renumbered goes back to its number)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, keyof(tbl, before), after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {LOG}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123d"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone()
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
            if not a.no_backup:
                print("backup", backup(a.db))
            for line in undo(db):
                print(line)
            db.execute("COMMIT")
            return 0
        if logged and a.apply:
            print(f"applied already ({LOG} exists): nothing written")
            db.execute("ROLLBACK")
            return 0
        plan = Plan(db)
        plan.build()
        for line in plan.lines:
            print(line)
        if not a.apply:
            print(f"dry run: {len(plan.changes)} rows would change")
            db.execute("ROLLBACK")
            return 0
        if not a.no_backup:
            print("backup", backup(a.db))
        apply(db, plan)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
