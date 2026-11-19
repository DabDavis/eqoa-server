#!/usr/bin/env python3
"""The were infection's migration (work 1463, a GM's of 2026-11-14: a successful animal-were infection is
a seven-day debuff, and that debuff is the infection flag; database/wereinfection.go, handlers/wereinfect.go).

Before it a character's infection was its line's hidden fever marker, the draft quest Weres/<line>/infection
finished (quests 101422-101426 on both shards: the wolf's, bear's, rat's, lion's, gator's), with no end. Now it is a
charinfection row with an absolute end seven days of real time after its catching; the marker stays as its derived
form (there while the row is active). This makes the rows for the markers held, once:
  - a marker with a timestamp (charquests.updated, RFC 3339, written when it was finished) at or before now: an
    infection from that time, its end seven days after it ("from its timestamp");
  - one with none (or one in the future): seven days from now ("migration default", the ticket's disclosed default);
  - one whose seven days are over already: the row ended expired at its end, the marker taken ("expired");
  - one on a character who has turned (any line's third quest, Weres/<line>/p3, finished): the marker taken, a row
    ended turned and no infection ("turned"): a transformation is not an infection;
  - a second marker of one character (one infection at a time): the newest by its timestamp is the infection, the
    others taken ("second marker");
  - a character with a charinfection row already (the server made it): its markers are the server's to keep in step,
    left ("has a row").
Each marker migrated is a charinfection_migration row (what the marker was: done, step, count, slot, updated, picks;
the infection made of it; the outcome) and a gm_audit line, in one transaction. A marker in charinfection_migration is
never migrated again, so a second --apply (or a restart: the server keeps the same rule for a marker it finds with no
row, recording it the same way) renews nothing. --undo puts back each logged marker as it was, deletes the rows this
migration made (source 'migration', key migration/<serverid>/<questid>) and its log rows; an infection caught since
(another key) is left.

It also checks what the code reads off the records, read-only: Disease Cloud (spell 875), the infection's icon
0xCF3C577B and its crit's visual 0x052EF90F (the reference server's recorded cast, spell_visuals_sandstorm_log), and counts the
weres the code gives their crit (the five lines' names, by template).

Report by default (--dry says the same); --apply, --undo. Run --apply in the swap's hold (tools/swap_keep.sh's
before.sh, the server stopped): the quest engine reads charquests at a character's first use.

    python3 -E -s tools/were_infection_migrate_20261115.py --db eqoa.db [--dry | --apply | --undo] [--now <unix>]
"""
import argparse
import datetime
import json
import sqlite3
import sys
import time

LINES = ["wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"]
NAMES = {  # the words a were's name carries (entity.WereLine)
    "wolfwere": ("wolfwere", "werewolf"), "bearwere": ("bearwere", "werebear"), "ratwere": ("wererat", "ratwere"),
    "lionwere": ("lionwere", "werelion"), "gatorwere": ("gatorwere", "weregator"),
}
WEEK = 7 * 24 * 3600
DETAIL = "the were infection's migration (work 1463, Jibrish 2026-11-14); tools/were_infection_migrate_20261115.py"
WHO, VIA = "dab", "tool"
DISEASE_CLOUD, ICON, FX = 875, 0xCF3C577B, 0x052EF90F

# as database/wereinfection.go makes them (CREATE ... IF NOT EXISTS: the same either way round)
SCHEMA = """
CREATE TABLE IF NOT EXISTS charinfection (
    serverid   INTEGER PRIMARY KEY,
    line       TEXT NOT NULL,
    applied_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    source     TEXT NOT NULL DEFAULT '',
    grant_id   TEXT NOT NULL DEFAULT '',
    ended_at   INTEGER NOT NULL DEFAULT 0,
    ended_how  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_charinfection_open ON charinfection(ended_at, expires_at);
CREATE TABLE IF NOT EXISTS charinfection_migration (
    serverid    INTEGER NOT NULL,
    questid     INTEGER NOT NULL,
    line        TEXT NOT NULL,
    marker_done INTEGER NOT NULL,
    marker_step INTEGER NOT NULL DEFAULT 0,
    marker_count INTEGER NOT NULL DEFAULT 0,
    marker_slot INTEGER NOT NULL DEFAULT 0,
    marker_updated TEXT NOT NULL DEFAULT '',
    marker_picks TEXT NOT NULL DEFAULT '',
    applied_at  INTEGER NOT NULL DEFAULT 0,
    expires_at  INTEGER NOT NULL DEFAULT 0,
    outcome     TEXT NOT NULL,
    migrated_at TEXT NOT NULL,
    by          TEXT NOT NULL,
    PRIMARY KEY (serverid, questid)
);
"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, sid, qid, before, after):
    row = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'charinfection', ?, ?, ?, ?)",
               (at, row[0] if row else 0, WHO, VIA, action, go_json({"serverid": sid, "questid": qid}),
                go_json(before), go_json(after), DETAIL))


def import_key(source):
    # database.QuestImportKey: "EQOA-Data <key> (tools/import_quests.py)"
    s = (source or "").strip()
    if not s.startswith("EQOA-Data "):
        return ""
    s = s[len("EQOA-Data "):]
    return s.split(" (", 1)[0].strip()


def were_quests(db):
    fever, turned = {}, {}
    for qid, src in db.execute("SELECT questid, source FROM quests"):
        parts = import_key(src).split("/")
        if len(parts) != 3 or parts[0] != "Weres":
            continue
        if parts[2] == "infection":
            fever[parts[1]] = qid
        elif parts[2] == "p3":
            turned[parts[1]] = qid
    return fever, turned


def parse_ts(s):
    try:
        return int(datetime.datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=datetime.timezone.utc).timestamp())
    except (TypeError, ValueError):
        return None


def plan(db, now):
    """[(marker row, line, outcome, applied, expires, ended_at, ended_how, write_row)], notes."""
    fever, turned = were_quests(db)
    notes = []
    missing = [l for l in LINES if l not in fever]
    if missing:
        notes.append(f"no marker quest for {', '.join(missing)}")
    notes.append("markers: " + ", ".join(f"{l} {fever.get(l)}" for l in LINES))
    line_of = {q: l for l, q in fever.items()}
    if not line_of:
        return [], notes
    marks = ",".join(str(q) for q in line_of)
    rows = db.execute(f"SELECT serverid, questid, step, count, done, slot, updated, picks FROM charquests "
                      f"WHERE questid IN ({marks}) AND done > 0 ORDER BY serverid, questid").fetchall()
    logged = {(s, q) for s, q in db.execute("SELECT serverid, questid FROM charinfection_migration")} if table(db, "charinfection_migration") else set()
    has_row = {s for (s,) in db.execute("SELECT serverid FROM charinfection")} if table(db, "charinfection") else set()
    tq = ",".join(str(q) for q in turned.values()) or "0"
    turned_chars = {s for (s,) in db.execute(f"SELECT DISTINCT serverid FROM charquests WHERE questid IN ({tq}) AND done > 0")}
    by_char = {}
    for r in rows:
        by_char.setdefault(r[0], []).append(r)
    out = []
    for sid, marks_held in by_char.items():
        todo = [r for r in marks_held if (sid, r[1]) not in logged]
        if not todo:
            continue
        if sid in has_row:
            for r in todo:
                notes.append(f"character {sid}: marker {r[1]} left, it has a charinfection row (the server keeps it in step)")
            continue
        # the newest by its timestamp is the infection; the others are second markers
        def ts(r):
            t = parse_ts(r[6])
            return t if t is not None and t <= now else -1
        ordered = sorted(todo, key=ts, reverse=True)
        for i, r in enumerate(ordered):
            line = line_of[r[1]]
            t = parse_ts(r[6])
            applied, outcome = (t, "from its timestamp") if t is not None and t <= now else (now, "migration default")
            expires = applied + WEEK
            ended_at, how, write = 0, "", True
            if sid in turned_chars:
                outcome, ended_at, how = "turned: marker taken", now, "turned"
            elif i > 0:
                outcome, write = "second marker: taken", False
            elif expires <= now:
                outcome, ended_at, how = "expired: marker taken", expires, "expired"
            out.append((r, line, outcome, applied, expires, ended_at, how, write and i == 0))
    return out, notes


def check_records(db):
    lines = []
    r = db.execute("SELECT spellname, icon & 4294967295 FROM spellpattern WHERE spellid = ?", (DISEASE_CLOUD,)).fetchone()
    e = db.execute("SELECT fxdict & 4294967295 FROM spelleffect WHERE spellid = ?", (DISEASE_CLOUD,)).fetchone()
    s = db.execute("SELECT why FROM spell_visuals_sandstorm_log WHERE spellid = ?", (DISEASE_CLOUD,)).fetchone() \
        if table(db, "spell_visuals_sandstorm_log") else None
    ok = r is not None and r[1] == ICON and e is not None and e[0] == FX
    lines.append(f"record check: spell {DISEASE_CLOUD} {r[0] if r else '(none)'}: icon 0x{(r[1] if r else 0):08X} (code 0x{ICON:08X}), "
                 f"visual 0x{(e[0] if e else 0):08X} (code 0x{FX:08X}); Sandstorm: {s[0] if s else '(no row)'}: {'OK' if ok else 'MISMATCH'}")
    return ok, lines


def inventory(db):
    lines = []
    tmpl = {n: t for n, t in db.execute("SELECT name, template FROM npctemplate")} if table(db, "npctemplate") else {}
    counts = {}
    for name, level, ty in db.execute("SELECT npc_name, npc_level, npc_type FROM npcs"):
        n = (name or "").lower()
        fam = next((l for l in LINES if any(w in n for w in NAMES[l])), None)
        if not fam:
            continue
        key = (fam, tmpl.get(n.strip(), "(none)"))
        c = counts.setdefault(key, [0, 999, 0, 0])
        c[0] += 1
        c[1], c[2] = min(c[1], level), max(c[2], level)
        c[3] += 1 if (ty or 0) & 0x80 else 0
    total = sum(c[0] for c in counts.values())
    lines.append(f"weres given their crit (the five lines' names): {total} spawn rows")
    for (fam, t), c in sorted(counts.items()):
        lines.append(f"  {fam:10s} template {t:12s} {c[0]:4d} rows, levels {c[1]}-{c[2]}" + (f", {c[3]} with the NPC bit" if c[3] else ""))
    return lines


def apply(db, steps, now):
    for r, line, outcome, applied, expires, ended_at, how, write in steps:
        sid, qid, step, count, done, slot, updated, picks = r
        key = f"migration/{sid}/{qid}"
        if write or ended_at:
            db.execute("INSERT INTO charinfection (serverid, line, applied_at, expires_at, source, grant_id, ended_at, ended_how) "
                       "VALUES (?, ?, ?, ?, 'migration', ?, ?, ?) ON CONFLICT(serverid) DO NOTHING",
                       (sid, line, applied, expires, key, ended_at, how))
        if ended_at or not write:
            db.execute("DELETE FROM charquests WHERE serverid = ? AND questid = ?", (sid, qid))
        db.execute("INSERT INTO charinfection_migration (serverid, questid, line, marker_done, marker_step, marker_count, marker_slot, "
                   "marker_updated, marker_picks, applied_at, expires_at, outcome, migrated_at, by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'tool')",
                   (sid, qid, line, done, step, count, slot, updated, picks, applied, expires, outcome,
                    datetime.datetime.fromtimestamp(now, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")))
        audit(db, "migrate", sid, qid, {"marker": {"done": done, "updated": updated}},
              {"line": line, "applied_at": applied, "expires_at": expires, "ended_how": how, "outcome": outcome})


def undo(db):
    if not table(db, "charinfection_migration"):
        return 0, 0
    back = removed = 0
    for sid, qid, line, done, step, count, slot, updated, picks, outcome in db.execute(
            "SELECT serverid, questid, line, marker_done, marker_step, marker_count, marker_slot, marker_updated, marker_picks, outcome "
            "FROM charinfection_migration").fetchall():
        db.execute("INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks) VALUES (?, ?, ?, ?, ?, ?, ?, ?) "
                   "ON CONFLICT(serverid, questid) DO UPDATE SET step = excluded.step, count = excluded.count, done = excluded.done, "
                   "slot = excluded.slot, updated = excluded.updated, picks = excluded.picks",
                   (sid, qid, step, count, done, slot, updated, picks))
        back += 1
        cur = db.execute("DELETE FROM charinfection WHERE serverid = ? AND source = 'migration' AND grant_id = ?", (sid, f"migration/{sid}/{qid}"))
        removed += cur.rowcount
        audit(db, "undo migrate", sid, qid, {"outcome": outcome}, {"marker": {"done": done, "updated": updated}})
    db.execute("DELETE FROM charinfection_migration")
    return back, removed


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--dry", action="store_true", help="report only (the default)")
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--now", type=int, default=None, help="unix seconds to migrate at (a test's)")
    a = ap.parse_args()
    now = a.now if a.now is not None else int(time.time())
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    ok, lines = check_records(db)
    for l in lines + inventory(db):
        print(l)
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        if not table(db, "charinfection"):
            db.execute("ROLLBACK")
            print("undone: nothing (no charinfection table)")
            return 0
        back, removed = undo(db)
        db.execute("COMMIT")
        print(f"undone: {back} markers put back, {removed} migration rows of charinfection deleted")
        return 0
    if a.apply and not ok:
        print("the records do not match the code's icon and visual: not applied", file=sys.stderr)
        return 2
    if a.apply:
        db.execute("BEGIN IMMEDIATE")
        for stmt in SCHEMA.split(";"):
            if stmt.strip():
                db.execute(stmt)
    steps, notes = plan(db, now)
    for n in notes:
        print(n)
    by = {}
    for s in steps:
        by[s[2].split(":")[0]] = by.get(s[2].split(":")[0], 0) + 1
        r = s[0]
        print(f"  character {r[0]} marker {r[1]} ({s[1]}, updated {r[6] or '(none)'}): {s[2]}; "
              f"{'infection until ' + datetime.datetime.fromtimestamp(s[4], datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ') if s[7] and not s[5] else 'no infection'}")
    print(f"markers to migrate: {len(steps)}" + ("" if not by else " (" + ", ".join(f"{k} {v}" for k, v in sorted(by.items())) + ")"))
    if a.apply:
        apply(db, steps, now)
        db.execute("COMMIT")
        print(f"applied: {len(steps)} markers migrated, in charinfection, charinfection_migration and gm_audit")
    else:
        print("dry run: nothing written (--apply to write)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
