#!/usr/bin/env python3
"""The 5 m friendly spells reviewed (the owner, 2026-11-24, after Heirloom petition 2869: "yes review the 5 meter heals").

Petition 2869 found Divinity (900487) and Influence (900488) at 5 m while their Ascended copies (907242/907243) were 30 m;
both were set to 30 m on both shards (funyun, panel, audits 163811/163812). Every scope 1 spell at 5 m (16544) that a
player casts on another player was then read against its line and its sources. Most came from one place: the EQOAEmu
workbook import (2026-11-01, tools/import_eqoaemu_spells.py) read the workbook's Range 1, its word for self or touch, as
4.5 m (16528) on a target spell, over the record's 30 m; jibrish's edit of 2026-11-11 19:13-19:15 UTC moved every 4.5 to
5.0. The eqoa-data importer's 30 m was itself a scope convention, so a line's other ranks are the evidence here.

Changed here (range 16544 -> 16880, 5 m -> 30 m), each only where the line or a source shows it:
  900491 Radiance        the forum's Zaoran Plateau ability chart (eqoa.forumotion.com f52 t308, 2010, from the
                         Allakhazam records) gives "Range: 30"; the EQOAEmu workbook 30; the record was 30 until
                         jibrish's board entry 1768 (2026-11-21) set it to 5 "as Influence and Divinity", which petition
                         2869 has since put back to 30.
  701009 Lightning Coat  the druid's coat line: Static Coat, Charge Coat, Thunder Coat, Storm Coat all 30; the record
                         was 30 before the EQOAEmu import's 4.5 (workbook Range 1).
  700515 Thornshield     the druid's shield line: Thistleshield, Barbshield, Brambleshield, Spikeshield, Briarshield all
                         30; the record was 30 before the EQOAEmu import's 4.5 (workbook Range 1).
  901097 Thistlecoat +   the ranger's "+" coat line: Barbcoat +, Bramblecoat +, Spikecoat +, Bladecoat + all 30; made by
                         the EQOAEmu import at 4.5 (workbook Range 1).

A row not at 16544 (a GM's edit since) is refused. Live's rows are the same: mirrored (Live has no Ascended copies).
Report only by default; --apply in one transaction, a gm_audit line a row carrying its basis, log
petpass_both_1124ah_log; --undo exact. Spells are read at each cast; a spell book shows the range from the next login.

    python3 -E -s tools/petpass_both_1124ah.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124ah_log"
DETAIL = ("5 m friendly spells reviewed after Heirloom petition 2869 (owner 2026-11-24): range 5 -> 30 m "
          "(tools/petpass_both_1124ah.py)")
FIVE, THIRTY = 16544, 16880  # the float's high half: 5.0, 30.0

# (spellid, name, basis tag, why)
SPELLS = [
    (900491, "Radiance", "[basis:era forum f52 t308 Range 30] ",
     "Zaoran Plateau chart (t308) gives Radiance Range 30; set to 5 by board 1768 only to match Divinity/Influence"),
    (701009, "Lightning Coat", "[basis:shared petition 2869] ",
     "druid coat line (Static, Charge, Thunder, Storm Coat) all 30; 5 m was the EQOAEmu import's Range-1 default"),
    (700515, "Thornshield", "[basis:shared petition 2869] ",
     "druid shield line (Thistle-, Barb-, Bramble-, Spike-, Briarshield) all 30; 5 m was the EQOAEmu import's default"),
    (901097, "Thistlecoat +", "[basis:shared petition 2869] ",
     "ranger + coat line (Barbcoat +, Bramblecoat +, Spikecoat +, Bladecoat +) all 30; 5 m was the EQOAEmu default"),
]
DETAILS = {}


def changes(db):
    out, lines, refusals = [], [], []
    done = 0
    for sid, name, tag, why in SPELLS:
        key = {"spellid": sid}
        p = tr.get(db, "spellpattern", key)
        if p is None or p["spellname"] != name:
            refusals.append(f"spellpattern {sid} is not {name}: {p and p['spellname']}")
            continue
        if p["scope"] != 1:
            refusals.append(f"{sid} {name} scope {p['scope']}, not 1")
            continue
        DETAILS[json.dumps(key, sort_keys=True)] = tag + DETAIL + ": " + why
        if p["range"] == THIRTY:
            done += 1
            out.append(("spellpattern", key, dict(p)))
        elif p["range"] != FIVE:
            refusals.append(f"{sid} {name} range {p['range']}, not {FIVE} (a GM's edit since?)")
        else:
            out.append(("spellpattern", key, dict(p, range=THIRTY)))
            lines.append(f"spellpattern {sid} {name}: range {FIVE} (5 m) -> {THIRTY} (30 m)  {tag.strip()}")
    if refusals:
        out = []
    applied = bool(out) and done == len(out)
    lines.insert(0, f"{len(out) - done} rows to change")
    return out, lines, refusals, applied


def apply(db, spec, out):
    """tix4_rows.apply with each row's own gm_audit detail (its basis and why)."""
    db.execute(tr.log_schema(spec["log"]))
    n = 0
    for tbl, key, after in out:
        r = tr.write(db, tbl, key, after)
        if r is None:
            continue
        action, before, now = r
        a = tr.audit(db, action, tbl, key, before, now, DETAILS[json.dumps(key, sort_keys=True)])
        db.execute(f"INSERT INTO {spec['log']} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                    None if now is None else json.dumps(now, sort_keys=True), a))
        n += 1
    return n


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("spellpattern",), "changes": changes}

if __name__ == "__main__":
    tr.apply = apply
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
