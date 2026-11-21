#!/usr/bin/env python3
"""The Lore Library: SOE's lore as the EQOA forum's "Lore Library" keeps it (eqoa.forumotion.com/f48-lore-library
and its subforums, read 2026-11-21; the owner, 2026-11-21: "yes add the lore"), retold in our own words for Tunaria in
the Age of Adventure. The words are tools/dialog/lore_library_20261121.json, each piece with the forum thread it is
from. Where a piece contradicts our facts (tools/dialog/LORE-FACTS.md) ours stand: the elves of Takish'Hiz fled to
Fayspires (not across the sea), Erud lives in Highbourne, Ykesha is dead and Grobb is the trolls'; PC EverQuest's
places are left out.

WHAT IT WRITES, by NAMES (never by row ids: the two shards' rows are read where they stand):
  A. town dialog (npcdialog, source 'lore-1121'): each block's lines for its town, one box each (190 characters, the
     box less a speaker's name: seed_town_dialog.py's LINE_MAX). A town no faction "Citizens of <town>" names here is
     skipped and said; a line the table holds already, word for word, is skipped.
  B. books: a sign post (the body every sign stands on, handlers/signs.go's SignBody), a copy of the "sign post" row
     by Freeport's Midroad, set down beside the NPC the book is near (a row's step apart, the NPC's floor and facing),
     named for its page, with a signtext row of kind 'place' holding the page (at most 1,000 characters, SignTextMax;
     a longer piece is several pages, several signs). The sign says its page in dialogue boxes when talked to.
  C. "Tales of ..." quests as Tales of the Snow (tools/snow_chain_20261118.py): the giver tells the tale in three talk
     steps (the second and third on the answer "Go on."), the last giving +250 with the giver's city.

Every text is held to what a PS2 shows: ASCII only (no curly quotes or long dashes), no '|' (a run of them kills a
PS2's TextBox), no '%' (CLIENT's text expander), nothing naming another server or a PC EverQuest place; a dialog line
a box, a page 1,000 characters, a question 127 and a journal line 127.

Refused whole, nothing written, if a named NPC is not one row, the sign post to copy is not the one read, a faction
or a table is missing, a new sign's or quest's name is taken, or a text breaks the rules above.

Report only by default; --apply in one transaction with a gm_audit line a row and the log lore_library_1121_log (every
row's key, before and after); a second --apply does nothing; --applied says what it wrote; --undo puts back exactly
what it wrote (a row changed since is left, and said) and drops the log.

WHEN IT TAKES EFFECT: the dialog lines at the next talk (npcdialog is read at most 30 s old); the signs' words within
30 s (signtext), the signs themselves once the world reads its NPCs again (the GM panel's reload_npcs, or a restart);
the quests once the quests are read again (a GM panel quest edit, e.g. quest_set_enabled on one of them, or a restart).

    python3 -E -s tools/lore_library_20261121.py --db eqoa.db [--apply | --applied | --undo]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(HERE, "dialog", "lore_library_20261121.json")
LOG = "lore_library_1121_log"
SRC = "lore-1121"
WHO, VIA = "dab", "tool"
FORUM = "eqoa.forumotion.com f48 Lore Library"
DETAIL = f"the Lore Library ({FORUM}, the owner 2026-11-21 'yes add the lore'): tools/lore_library_20261121.py"

SIGN_BODY = 0x6A82352C
SIGN_COPY = ("sign post", 0, 25335.0, 15700.0)  # Freeport by Midroad: name, world, x, z
SIGN_KIND = "place"
SIGN_STEP = 2.5  # the first page this far along x from the NPC it is near, each next one as far again

BOX, NAME = 217, 25
LINE_MAX = BOX - NAME - 2  # 190: seed_town_dialog.py's
PAGE_MAX = 1000  # database.SignTextMax
ASK_MAX = 127  # questAskChars
JOURNAL_MAX = 127  # questLineChars
SAYS_MAX = 2 * BOX - NAME  # two boxes
SIGN_NAME_MAX = 23  # the longest NPC name the client was seen showing whole
REWARD = 250
ACCEPT, DECLINE, DECLINED, GO_ON = "I will listen.", "Not now.", "Another time, then.", "Go on."

# names that must not reach a player: PC EverQuest's places (not on Tunaria in this age) and other servers
BANNED = ["Faydwer", "Faydark", "Kelethin", "Felwithe", "Kaladim", "Ak'Anon", "Ak`Anon", "Akanon", "Erudin", "Kunark",
          "Velious", "Luclin", "Butcherblock", "Steamfont", "Paineel", "Antonica", "Odus", "Trakanon", "Sandstorm",
          "eqoa.live", "Return Home", "EverQuest", "forumotion", "Combine"]


def problems(text, most, what):
    """What breaks the PS2's rules in text (empty: none)."""
    out = []
    if not text or not text.strip():
        out.append(f"{what}: empty")
        return out
    if any(ord(ch) > 126 or (ord(ch) < 32 and ch != "\n") for ch in text):
        bad = sorted({ch for ch in text if ord(ch) > 126 or (ord(ch) < 32 and ch != "\n")})
        out.append(f"{what}: not plain ASCII {bad!r}")
    if "|" in text:
        out.append(f"{what}: has '|'")
    if "%" in text:
        out.append(f"{what}: has '%'")
    for b in BANNED:
        if re.search(r"(?<![A-Za-z])" + re.escape(b.lower()) + r"(?![A-Za-z])", text.lower()):
            out.append(f"{what}: names {b!r}")
    if len(text) > most:
        out.append(f"{what}: {len(text)} characters, at most {most}")
    return out


def load(path=DATA):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def offer_lines(offer):
    """The offer as the engine asks it: its last sentence alone on the last line (the question, questAskChars)."""
    parts = re.split(r"(?<=[.!?])\s+", offer.strip())
    if len(parts) == 1:
        return parts[0]
    return " ".join(parts[:-1]) + "\n" + parts[-1]


def tale_rows(t):
    """A tale's quest and step columns (no ids)."""
    city = t["faction"].replace("Citizens of ", "")
    quest = {"name": t["name"], "enabled": 1, "giver": t["giver"], "minlevel": 1, "maxlevel": 60, "classes": "",
             "races": "", "requires": "", "repeatable": 0, "offer": offer_lines(t["offer"]), "accept": ACCEPT,
             "decline": DECLINE, "declined": DECLINED, "started": "",
             "source": f"{FORUM} {t['src']} ({SRC})",
             "notes": f"A tale told for {t['faction']} +{REWARD}, as Tales of the Snow is for the Plane of Sky: the Lore "
                      f"Library's {t['src']} ({FORUM}), retold in our words. tools/lore_library_20261121.py."}
    steps = []
    for i, says in enumerate(t["parts"], 1):
        s = {"step": i, "kind": "talk", "target": t["giver"], "answer": "" if i == 1 else GO_ON, "says": says,
             "journal": (f"Hear {t['giver']}'s tale in {city}." if i == 1 else f"Hear the rest of {t['giver']}'s tale."),
             "factions": f"{t['faction']}:+{REWARD}" if i == len(t["parts"]) else ""}
        steps.append(s)
    return quest, steps


def check_texts(data):
    """Every text against the PS2's rules and the boxes it goes in."""
    bad = []
    seen = set()
    for b in data["lines"]:
        for l in b["lines"]:
            bad += problems(l, LINE_MAX, f"{b['town']} line {l[:40]!r}")
            if l in seen:
                bad.append(f"line twice: {l[:60]!r}")
            seen.add(l)
    names = set()
    for bk in data["books"]:
        for title, text in bk["pages"]:
            bad += problems(title, SIGN_NAME_MAX, f"sign name {title!r}")
            bad += problems(text, PAGE_MAX, f"page {title!r}")
            if title in names:
                bad.append(f"sign name twice: {title!r}")
            names.add(title)
    for t in data["tales"]:
        q, steps = tale_rows(t)
        lines = q["offer"].split("\n")
        bad += problems(q["offer"], 2 * BOX, f"{t['name']} offer")
        bad += problems(lines[-1], ASK_MAX, f"{t['name']} offer's question")
        bad += problems(q["name"], JOURNAL_MAX, f"{t['name']} name")
        if len(t["parts"]) != 3:
            bad.append(f"{t['name']}: {len(t['parts'])} parts, want 3")
        for s in steps:
            bad += problems(s["says"], SAYS_MAX, f"{t['name']} step {s['step']} says")
            bad += problems(s["journal"], JOURNAL_MAX, f"{t['name']} step {s['step']} journal")
    return bad


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def rows(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def row(db, tbl, where, args):
    r = rows(db, tbl, where, args)
    return r[0] if r else None


def js(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True) if v is not None else None


def cols(db, tbl):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{tbl}")')]


class Plan:
    """What this database holds that the tool reads, by names; what it would write; what it skips and why."""

    def __init__(self, db, data):
        self.bad, self.skipped = [], []
        bad = self.bad.append
        self.bad += check_texts(data)
        for t in ("npcdialog", "npcs", "signtext", "quests", "queststeps", "factions", "npcfaction", "gm_audit"):
            if not has(db, t):
                bad(f"no table {t}")
        if self.bad:
            return
        # A: the lines, each town a faction's
        self.lines = []
        for b in data["lines"]:
            town = b["town"]
            if not db.execute("SELECT 1 FROM npcfaction WHERE faction = ?", ("Citizens of " + town,)).fetchone():
                self.skipped.append(f"{town}: no faction 'Citizens of {town}' here, its {len(b['lines'])} lines skipped")
                continue
            for l in b["lines"]:
                if db.execute("SELECT 1 FROM npcdialog WHERE line = ?", (l,)).fetchone():
                    self.skipped.append(f"{town}: already said: {l[:60]!r}")
                    continue
                self.lines.append((town, l))
        # B: the sign post to copy, the NPCs the books are near, the signs' names free
        name, world, x, z = SIGN_COPY
        cand = [r for r in rows(db, "npcs", "npc_name = ? AND world = ?", (name, world))
                if abs(r["x_coord"] - x) < 1 and abs(r["z_coord"] - z) < 1]
        if len(cand) != 1 or (cand[0]["modelid"] & 0xFFFFFFFF) != SIGN_BODY:
            bad(f"{len(cand)} {name!r} rows at ({x}, {z}) in world {world} on the sign body, want one")
            return
        self.sign = cand[0]
        self.books = []
        at = {}
        for bk in data["books"]:
            near = rows(db, "npcs", "npc_name = ?", (bk["near"],))
            if len(near) != 1:
                bad(f"{len(near)} rows of {bk['near']!r}, want one (a book is set beside it)")
                continue
            n = near[0]
            for title, text in bk["pages"]:
                if rows(db, "npcs", "lower(trim(npc_name)) = lower(?)", (title,)):
                    bad(f"an NPC named {title!r} is there already")
                k = at.get(n["id"], 0) + 1
                at[n["id"]] = k
                self.books.append({"near": n, "title": title, "text": text, "src": bk["src"],
                                   "x": n["x_coord"] + SIGN_STEP * k, "y": n["y_coord"], "z": n["z_coord"]})
        # C: the tales' givers and factions
        self.tales = []
        for t in data["tales"]:
            g = rows(db, "npcs", "npc_name = ?", (t["giver"],))
            if len(g) != 1:
                bad(f"{len(g)} rows of {t['giver']!r}, want one (the giver of {t['name']!r})")
            if not row(db, "factions", "name = ?", (t["faction"],)):
                bad(f"no faction {t['faction']!r}")
            if rows(db, "quests", "name = ?", (t["name"],)):
                bad(f"a quest {t['name']!r} is there already")
            self.tales.append(t)
        top = db.execute("SELECT max(questid) FROM quests").fetchone()[0] or 100000
        self.first_quest = top + 1

    def summary(self):
        towns = {}
        for town, _ in self.lines:
            towns[town] = towns.get(town, 0) + 1
        out = [f"  A. {len(self.lines)} dialog lines: " + ", ".join(f"{t} {n}" for t, n in sorted(towns.items()))]
        near = {}
        for b in self.books:
            near.setdefault(b["near"]["npc_name"], []).append(b["title"])
        out.append(f"  B. {len(self.books)} book signs (copies of {self.sign['npc_name']!r} {self.sign['id']}): " +
                   "; ".join(f"by {k}: {', '.join(v)}" for k, v in near.items()))
        out.append(f"  C. {len(self.tales)} tales from quest {self.first_quest}: " +
                   "; ".join(f"{t['name']!r} by {t['giver']} (+{REWARD} {t['faction']})" for t in self.tales))
        return out


def apply(db, p):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, "
               "after TEXT)")
    n = [0]

    def logged(tbl, key, before, after):
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)",
                   (tbl, js(key), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))
        n[0] += 1

    def insert(tbl, values, key=None):
        have = set(cols(db, tbl))
        values = {c: v for c, v in values.items() if c in have}
        cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})',
                         list(values.values()))
        if key is None:
            key = {"id": cur.lastrowid}
        w = " AND ".join(f"{c} = ?" for c in key)
        logged(tbl, key, None, row(db, tbl, w, tuple(key.values())))
        return cur.lastrowid

    # A
    for town, line in p.lines:
        insert("npcdialog", {"town": town, "role": "", "name": "", "mood": "", "race": -1, "sex": "", "line": line,
                             "weight": 1, "source": SRC, "speaker_race": -1, "away": 0, "class": -1, "not_race": -1})
    # B
    scols = [c for c in cols(db, "npcs") if c != "id"]
    for b in p.books:
        v = {c: p.sign[c] for c in scols}
        v.update(npc_name=b["title"], world=b["near"]["world"], x_coord=b["x"], y_coord=b["y"], z_coord=b["z"],
                 facing=b["near"]["facing"])
        nid = insert("npcs", v)
        insert("signtext", {"npcid": nid, "kind": SIGN_KIND, "text": b["text"], "source": SRC,
                            "note": f"a book beside {b['near']['npc_name']}: the Lore Library's {b['src']} ({FORUM}), "
                                    f"retold; tools/lore_library_20261121.py"}, {"npcid": nid})
    # C
    qid = p.first_quest
    for t in p.tales:
        q, steps = tale_rows(t)
        q["questid"] = qid
        insert("quests", q, {"questid": qid})
        for s in steps:
            s = dict(s, questid=qid)
            insert("queststeps", s, {"questid": qid, "step": s["step"]})
        qid += 1
    return n[0]


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n, left = 0, 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        w = " AND ".join(f"{c} = ?" for c in key)
        now = row(db, tbl, w, tuple(key.values()))
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            left += 1
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {w}', tuple(key.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {w}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back, {left} left")
    return n, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--data", default=DATA)
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
            counts = db.execute(f"SELECT tbl, count(*) FROM {LOG} GROUP BY tbl ORDER BY tbl").fetchall()
            print("applied: " + ", ".join(f"{t} {c}" for t, c in counts))
            for (qid,) in db.execute(f"SELECT json_extract(rowkey, '$.questid') FROM {LOG} WHERE tbl = 'quests'"):
                print(f"  quest {qid} {db.execute('SELECT name, giver FROM quests WHERE questid = ?', (qid,)).fetchone()}")
            for (nid,) in db.execute(f"SELECT json_extract(rowkey, '$.id') FROM {LOG} WHERE tbl = 'npcs'"):
                print(f"  sign {nid} {db.execute('SELECT npc_name, world, x_coord, y_coord, z_coord FROM npcs WHERE id = ?', (nid,)).fetchone()}")
            return 0
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone: the lines within 30 s; the signs and quests at the next reload_npcs / quest reload")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        p = Plan(db, load(a.data))
        for s in p.skipped:
            print(f"  skipped: {s}")
        if p.bad:
            print("refused, nothing done:\n  " + "\n  ".join(p.bad))
            return 1
        print("\n".join(p.summary()))
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied, {n} rows: the lines at the next talk; the signs after reload_npcs; the quests after a quest "
              f"reload (quest_set_enabled on one of them) or a restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
