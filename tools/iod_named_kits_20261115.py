#!/usr/bin/env python3
"""The Isle of Dread's Festering Jack, Contagion, breath stealers and virulent blades given abilities a fight notices
(Heirloom petitions 2191 and 2192, Trulytwo, 2026-11-15: "in IoD festerjack and Contanion do not do the AoE ablitys,
they still need work, as atm you are able to jsut face tank them"; "Breath stealer and The Blade thing in IoD do Not
mana tap, the need ablitys reworked").

WHAT THEY HAD. Festering Jack ("iod festering jack", work board 430/644) cast Shadow Burst (905002, 1,500 arcane,
radius 30) at 33%: 495 every 20 s from 10 s in, before the player's AR (a L60 of AR 214 takes about half): next to
nothing against a L60's 6-10k. Contagion ("iod contagion", 430/649) has only its five-minute poison AoE (905023 at 200%,
4,000, first at 4:45): a raid that kills it in three minutes, as the 2004 one did, never sees it. Both casts fire (the
server's own fight on this database: Jack's Shadow Burst every 20 s); they are too small or too late to be felt. The
breath stealers and the virulent blades fight by "iod native", the island's natives' profile, which has no ability at
all: no mana tap was ever given them (board 618 has the breath stealer's AoE lifetap, still a code ask; the blades were
added on 2026-11-14 as plain natives).

WHAT THIS GIVES THEM (sizes and timers ours, the board's "ours" figures moved; the players' word taken as the record):
  Festering Jack: Swarming Mentality's AoE at 67% of Shadow Burst, 1,000 arcane, every 20 s from 8 s (board 644's
    hunt: the one hit seen beside his tank on a 2012 tape is a 945).
  Contagion: beside its five-minute 4,000, a poison AoE every 30 s from 10 s, Pestilent Breath at 50%: 1,000 poison,
    radius 40.
  breath stealer: "iod breath stealer", the natives' profile whole plus Stolen Breath (760309, new): a mana tap of 600
    power, resisted by AR (80% of it at the cap, as Pariah's Mana Tap), every 20 s from 6 s.
  virulent blade: "iod virulent blade", the same with Virulent Siphon (760310, new), 600 power, every 20 s from 9 s.
  The two spells are Pariah's Mana Tap's records (760301: a 1 s cast, Mana Burst's blue circle A87CDC4B) under their own
  names and figure; the two templates are copies of "iod native" as it stands on the shard (melee_pct 370, its crit),
  so a later fit of the natives' melee must be made on them too.

Report only by default; --apply in one transaction with gm_audit lines and the log iod_named_kits_1115_log (every row
before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed since is left
and said) and drops the log. Refused whole if a row is not as this tool expects (a template other than the board's, an
npctemplate row other than "iod native", a spell id taken).

WHEN IT TAKES EFFECT. Templates, npctemplate and the spells they cast: the GM panel's reload_npcs (it reads the class
templates and npctemplate first) or a restart.

    python3 -E -s tools/iod_named_kits_20261115.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "iod_named_kits_1115_log"
WHO, VIA = "dab", "tool"
SRC = "iodkits1115"
DETAIL = ("the Isle of Dread's Festering Jack and Contagion AoEs, breath stealers' and virulent blades' mana taps "
          "(petitions 2191, 2192): tools/iod_named_kits_20261115.py")

JACK_OLD = '[{"kind":"aoe","spell_id":905002,"damage_pct":33,"every":20,"first":10}]'
JACK_NEW = '[{"kind":"aoe","spell_id":905002,"damage_pct":67,"every":20,"first":8}]'
CONT_OLD = '[{"kind":"aoe","spell_id":905023,"damage_pct":200,"every":300,"first":285}]'
CONT_NEW = ('[{"kind":"aoe","spell_id":905023,"damage_pct":200,"every":300,"first":285},'
            '{"kind":"aoe","spell_id":905023,"damage_pct":50,"every":30,"first":10}]')
TEMPLATE_EDITS = {
    "iod festering jack": (JACK_OLD, JACK_NEW, " (2026-11-15, petition 2191: Swarming Mentality 495 -> 1,000 every 20 s from 8 s, "
                                               "the one 945 on the 2012 tape; tools/iod_named_kits_20261115.py)"),
    "iod contagion": (CONT_OLD, CONT_NEW, " (2026-11-15, petition 2191: beside the five-minute 4,000, a 1,000 poison AoE every "
                                          "30 s from 10 s, ours: a three-minute kill saw nothing; tools/iod_named_kits_20261115.py)"),
}
TAP_LIKE = 760301  # Pariah's Mana Tap
FX = 0xA87CDC4B    # Mana Burst's blue circle
SPELLS = {  # id: (name, power, desc)
    760309: ("Stolen Breath", 600, "Steals the target's power. Resisted by ARCANE."),
    760310: ("Virulent Siphon", 600, "Siphons the target's power. Resisted by ARCANE."),
}
NATIVE = "iod native"
NEW_TEMPLATES = {  # template: (spell, first, the names it takes)
    "iod breath stealer": (760309, 6, "breath stealer"),
    "iod virulent blade": (760310, 9, "virulent blade"),
}


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def check(db):
    """What is wrong with the database for this tool, [] for nothing."""
    bad = []
    for name, (old, _, _) in TEMPLATE_EDITS.items():
        t = row(db, "classtemplates", "name = ?", (name,))
        if t is None or json.loads(t["abilities"]) != json.loads(old):
            bad.append(f"classtemplates {name!r}: {t and t['abilities']!r}, want {old}")
    if row(db, "classtemplates", "name = ?", (NATIVE,)) is None:
        bad.append(f"no classtemplates {NATIVE!r}")
    for name, (_, _, npc) in NEW_TEMPLATES.items():
        if row(db, "classtemplates", "name = ?", (name,)):
            bad.append(f"classtemplates {name!r} is there already")
        nt = row(db, "npctemplate", "name = ?", (npc,))
        if nt is None or nt["template"] != NATIVE:
            bad.append(f"npctemplate {npc!r}: {nt}, want {NATIVE!r}")
    if row(db, "spellpattern", "spellid = ?", (TAP_LIKE,)) is None:
        bad.append(f"no spellpattern {TAP_LIKE} (Pariah's Mana Tap)")
    for sid in SPELLS:
        if row(db, "spellpattern", "spellid = ?", (sid,)) or row(db, "spelleffect", "spellid = ?", (sid,)):
            bad.append(f"spell {sid} is taken")
    return bad


def report(db):
    for name in list(TEMPLATE_EDITS) + [NATIVE]:
        t = row(db, "classtemplates", "name = ?", (name,))
        print(f"  {name}: {t and t['abilities']}")
    for _, (_, _, npc) in NEW_TEMPLATES.items():
        nt = row(db, "npctemplate", "name = ?", (npc,))
        n = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (npc,)).fetchone()[0]
        print(f"  {npc} ({n} rows): fights by {nt and nt['template']!r}")


def apply(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")

    def done(tbl, key, keyval, before):
        after = row(db, tbl, f"{key} = ?", (keyval,))
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js({key: keyval}), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, "update" if before else "insert", tbl, js({key: keyval}), js(before), js(after), DETAIL))

    like = row(db, "spellpattern", "spellid = ?", (TAP_LIKE,))
    for sid, (name, power, desc) in SPELLS.items():
        p = dict(like, spellid=sid, spellname=name, spelldesc=desc)
        db.execute(f"INSERT INTO spellpattern ({', '.join(p)}) VALUES ({', '.join('?' * len(p))})", list(p.values()))
        done("spellpattern", "spellid", sid, None)
        eff = {"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "stack": 0, "level": 60, "min_level": 1,
               "req_equipment": "Any", "sheet_type": "Dmg/Heal",
               "effects": [{"id": "7E01", "type": "Dmg/Heal", "kind": "power", "amount": -power, "resist": "ar"}]}
        db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?, ?, ?, ?, ?)",
                   (sid, f"Target, Decrease Power by {power}", FX - (1 << 32), f"petition 2192 (Trulytwo): {SRC}", js(eff)))
        done("spelleffect", "spellid", sid, None)

    native = row(db, "classtemplates", "name = ?", (NATIVE,))
    for name, (sid, first, npc) in NEW_TEMPLATES.items():
        t = dict(native, name=name, source=f"petition 2192 + {native['source']}",
                 abilities=js([{"kind": "nuke", "spell_id": sid, "every": 20, "first": first}]),
                 note=native["note"] + f" ({npc}: the natives' profile plus {SPELLS[sid][0]}, a 600 power mana tap (AR) every "
                                       f"20 s from {first} s; petition 2192, 2026-11-15, sizes ours: tools/iod_named_kits_20261115.py)")
        db.execute(f"INSERT INTO classtemplates ({', '.join(t)}) VALUES ({', '.join('?' * len(t))})", list(t.values()))
        done("classtemplates", "name", name, None)
        before = row(db, "npctemplate", "name = ?", (npc,))
        db.execute("UPDATE npctemplate SET template = ?, note = note || ? WHERE name = ?",
                   (name, f" (-> {name}, petition 2192: tools/iod_named_kits_20261115.py)", npc))
        done("npctemplate", "name", npc, before)

    for name, (_, new, words) in TEMPLATE_EDITS.items():
        before = row(db, "classtemplates", "name = ?", (name,))
        db.execute("UPDATE classtemplates SET abilities = ?, note = note || ? WHERE name = ?", (new, words, name))
        done("classtemplates", "name", name, before)


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n = 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        (key, val), = json.loads(rowkey).items()
        now = row(db, tbl, f"{key} = ?", (val,))
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {key} = ?', (val,))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {key} = ?', list(b.values()) + [val])
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        report(db)
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("takes effect at the GM panel's reload_npcs or a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        bad = check(db)
        if bad:
            print("refused, nothing done:\n  " + "\n  ".join(bad))
            return 1
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        apply(db)
        db.execute("COMMIT")
        print("applied; takes effect at the GM panel's reload_npcs or a restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
