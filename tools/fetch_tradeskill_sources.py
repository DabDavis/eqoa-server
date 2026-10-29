#!/usr/bin/env python3
"""Fetch what tools/seed_tradeskills.py reads into a folder of the operator's
own: eqoatrade.com ("EQoA Trade Center", 2003-04), as the Wayback Machine
archived it. Nothing of it is kept in this repository.

For a craft:
- its recipe grids: a page a piece (armor: six chain pieces, six plate pieces,
  three shields), each a table by trade level and modifier whose cells are the
  product's AC and stats, or "N/C" (no combine); jewelry's are Prima's
  Frontiers guide's gem tables instead (the gems changed in December 2003,
  after eqoatrade's grid), so for it the guide's PDF from archive.org, made
  into layout text with pdftotext (poppler-utils);
- its vendors by city: the trainer, and who sells the tools, the molds and
  patterns, and the metal;
- its tutorial: the trainer's starter kit.

    tools/fetch_tradeskill_sources.py --out <folder> [--craft armor] [--pause 5]

One request at a time, --pause seconds apart (the archive refuses bursts). A
file already fetched is kept. The pages are the archive's 2004 captures,
untouched (the `id_` form).
"""
import argparse
import os
import subprocess
import sys
import time

BASE = "https://web.archive.org/web/2004id_/http://www.eqoatrade.com:80/Website/"
PAGES = {
    "armor": [
        "Recipes/Armor/Original/Chain/Coif.htm",
        "Recipes/Armor/Original/Chain/Tunic.htm",
        "Recipes/Armor/Original/Chain/Bracers.htm",
        "Recipes/Armor/Original/Chain/Gloves.htm",
        "Recipes/Armor/Original/Chain/Legs.htm",
        "Recipes/Armor/Original/Chain/Boots.htm",
        "Recipes/Armor/Original/Plate/Helm.htm",
        "Recipes/Armor/Original/Plate/BP.htm",
        "Recipes/Armor/Original/Plate/Bracers.htm",
        "Recipes/Armor/Original/Plate/Gaunts.htm",
        "Recipes/Armor/Original/Plate/Greaves.htm",
        "Recipes/Armor/Original/Plate/Boots.htm",
        "Recipes/Armor/Original/Shield/Heater.htm",
        "Recipes/Armor/Original/Shield/Kite.htm",
        "Recipes/Armor/Original/Shield/Tower.htm",
        "Vendors/Armor.htm",
        "Tutorials/Armor.htm",
    ],
    "jewel": [
        "Vendors/Jewelry.htm",
        "Tutorials/Jewelry.htm",
    ],
}
PRIMA_PDF = ("https://archive.org/download/EverQuest_Online_Adventures_Frontiers_Prima_Official_eGuide/"
             "EverQuest_Online_Adventures_Frontiers_Prima_Official_eGuide.pdf")
NEEDS_PRIMA = {"jewel"}


def fetch(url, dest):
    tmp = dest + ".part"
    r = subprocess.run(["curl", "-s", "-L", "-g", "-m", "120", "-o", tmp, "-w", "%{http_code}", url],
                       capture_output=True, text=True)
    code = r.stdout.strip()
    if r.returncode != 0 or code != "200" or os.path.getsize(tmp) < 200:
        if os.path.exists(tmp):
            os.remove(tmp)
        return code or "error"
    os.replace(tmp, dest)
    return "200"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--out", required=True, help="the folder (made when absent)")
    ap.add_argument("--craft", default="armor", choices=sorted(PAGES))
    ap.add_argument("--pause", type=float, default=5.0)
    a = ap.parse_args()
    got = kept = failed = 0
    for rel in PAGES[a.craft]:
        dest = os.path.join(a.out, "eqoatrade", rel)
        if os.path.exists(dest):
            kept += 1
            continue
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        code = "?"
        for attempt in range(3):
            code = fetch(BASE + rel, dest)
            if code == "200":
                break
            time.sleep(a.pause * (attempt + 2))
        if code == "200":
            got += 1
            print(f"fetched {rel}")
        else:
            failed += 1
            print(f"failed {rel}: {code}", file=sys.stderr)
        time.sleep(a.pause)
    if a.craft in NEEDS_PRIMA:
        pdf = os.path.join(a.out, "prima", "prima-frontiers.pdf")
        txt = os.path.join(a.out, "prima", "prima-frontiers-layout.txt")
        os.makedirs(os.path.dirname(pdf), exist_ok=True)
        if not os.path.exists(pdf):
            code = fetch(PRIMA_PDF, pdf)
            if code != "200":
                failed += 1
                print(f"failed Prima's guide: {code}", file=sys.stderr)
            else:
                got += 1
                print("fetched Prima's guide")
        if os.path.exists(pdf) and not os.path.exists(txt):
            r = subprocess.run(["pdftotext", "-layout", pdf, txt], capture_output=True, text=True)
            if r.returncode != 0:
                failed += 1
                print(f"pdftotext: {r.stderr.strip()}", file=sys.stderr)
    print(f"{got} fetched, {kept} kept, {failed} failed, in {a.out}")
    if failed:
        sys.exit(1)


if __name__ == "__main__":
    main()
