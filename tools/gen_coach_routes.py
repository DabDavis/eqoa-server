#!/usr/bin/env python3
"""EQOA-Data's coach network: writes internal/entity/coachroutes.go.

EQOA-Data (github.com/Jadiction/EQOA-Data, MIT) maps the live game's coaches
in Map/coach_map.json: each coach town and the towns its coach ran to. The GM
panel's map shows them beside each coachman (World.CoachMarkers); this
server's own coach menus offer every coach town of the world
(GetCoachDestinations), so the two are told apart there.

The towns are written as coachtowns.go names them (its coachLocationNames):
"Neriak, Nektulos" is Neriak, Castle Felstar Fort Seriak, Klick'Anon Klik'Anon,
the two Hazinaks "Hazinak (Good)" and "Hazinak (Evil)", the Stones of Morathal
likewise; a name's stray spaces go.

    python3 -E -s tools/gen_coach_routes.py --eqoa-data ../external/EQOA-Data \\
        [--out internal/entity/coachroutes.go]
"""

import argparse
import json
import os
import re
import shutil
import subprocess
import sys

RENAME = {
    "Neriak, Nektulos": "Neriak",
    "Castle Felstar": "Fort Seriak",
    "Klick'Anon": "Klik'Anon",
}


def town(name):
    name = name.strip()
    if name in RENAME:
        return RENAME[name]
    m = re.match(r"^(.*)-(Good|Evil)$", name)
    if m:
        return f"{m.group(1)} ({m.group(2)})"
    return name


def render(routes):
    lines = [
        "// Code written by tools/gen_coach_routes.py; DO NOT EDIT: change the tool",
        "// and run it again.",
        "",
        "package entity",
        "",
        "// liveCoachRoutes is the live game's coach network as EQOA-Data maps it",
        "// (Map/coach_map.json): each coach town and the towns its coach ran to, by",
        "// coachtowns.go's names.",
        "var liveCoachRoutes = map[string][]string{",
    ]
    for t in sorted(routes):
        dests = ", ".join(json.dumps(d, ensure_ascii=False) for d in routes[t])
        lines.append(f"\t{json.dumps(t, ensure_ascii=False)}: {{{dests}}},")
    lines.append("}")
    return "\n".join(lines) + "\n"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--eqoa-data", required=True)
    ap.add_argument("--out")
    args = ap.parse_args()
    raw = json.load(open(os.path.join(args.eqoa_data, "Map", "coach_map.json")))
    routes = {}
    for t, dests in raw.items():
        seen = []
        for d in dests:
            d = town(d)
            if d not in seen:
                seen.append(d)
        routes[town(t)] = seen
    text = render(routes)
    if args.out:
        with open(args.out, "w") as f:
            f.write(text)
        if shutil.which("gofmt"):
            subprocess.run(["gofmt", "-w", args.out], check=True)
        print(f"wrote {len(routes)} coach towns to {args.out}", file=sys.stderr)
    else:
        sys.stdout.write(text)


if __name__ == "__main__":
    main()
