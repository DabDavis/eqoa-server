#!/usr/bin/env python3
"""Gold tinsel drawn as tinsel (the owner, 2026-11-17: "the tinsel should be rotated per piece").

Sets the "Gold tinsel" light template's cord_style to tinsel: a client with it (vanilla tinsel-twist) draws the cord as
short pieces along it, each two crossed quads at a twist of its own, bushy from any side, where one band turned to the
eye read as a flat strip. The ribbon ("Red ribbon") stays a band. Makes the column if the server (tinsel-style) has not.

A report by default; --apply writes it with a gm_audit line and keeps it in --applied; --undo puts it back. Live at a
GM panel touch of a Light templates row.

    christmas_tinsel_style_20261117.py --db eqoa.db --applied FILE [--apply | --undo]
"""
import argparse, json, os, sqlite3, sys, time

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', 'update', 'lighttemplates', ?, ?, ?, ?)",
               (now, json.dumps({"name": "Gold tinsel"}), json.dumps(before), json.dumps(after), "the owner: tinsel rotated per piece; tools/christmas_tinsel_style_20261117.py"))


if "cord_style" not in {r[1] for r in db.execute("PRAGMA table_info(lighttemplates)")}:
    if a.undo:
        sys.exit("no cord_style column: nothing to undo")
    db.execute("ALTER TABLE lighttemplates ADD COLUMN cord_style TEXT NOT NULL DEFAULT 'band'")
if a.undo:
    j = json.load(open(a.applied))
    db.execute("UPDATE lighttemplates SET cord_style = ? WHERE name = 'Gold tinsel'", (j["was"],))
    audit({"cord_style": "tinsel"}, {"cord_style": j["was"]})
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
r = db.execute("SELECT cord_style FROM lighttemplates WHERE name = 'Gold tinsel'").fetchone()
if r is None:
    sys.exit("no 'Gold tinsel' template: nothing done")
if r[0] == "tinsel":
    sys.exit("Gold tinsel is tinsel already: nothing done")
db.execute("UPDATE lighttemplates SET cord_style = 'tinsel' WHERE name = 'Gold tinsel'")
audit({"cord_style": r[0]}, {"cord_style": "tinsel"})
print(f"Gold tinsel: cord_style {r[0]} -> tinsel")
if a.apply:
    json.dump({"was": r[0]}, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
