#!/usr/bin/env python3
"""The Lore Library in our NPCs' and quests' words (the owner, 2026-11-21, of the proposal that followed
tools/lore_library_20261121.py: "apply to live also and use them all"). Six parts, each a data file in
tools/dialog/lore_dialog_20261121/ and selectable with --only / --skip:

  f  Live's short quest text made Heirloom's: for a quest both shards give (by giver, name, classes, races and
     level), whose steps are the same (kind, target, count, figures) and whose text is shorter on Live, Heirloom's
     words (offer, accept, decline, declined, started; each step's journal, answer, says, waiting, notyet, reminder).
     Written where the row is as Live's was read; on Heirloom it is already so (said as such). Runs first, so the
     parts after it find Live's rows as Heirloom's.
  a  the proposal's 23 rewrites (five of them change meaning: Fryda's hint at her house's secret, the Dawn and Moon
     tribes as Wegadas's heirs at war, Djivan Mertshak as the were-hunters' author, Kings' Rest in the Dead Hills),
     five boards' welcome words (signtext kind board) and Hodstock's town dialog.
  b  Grobb's founder: Warlord Jurglash, the name of Grobb's own warlord (the NPC and his quests), in place of the
     forum's "Jurgash" in what tools/lore_library_20261121.py wrote (its dialog lines, sign pages and Tales of the
     Grozmok). Undo this tool's part b before that tool's --undo, which leaves a row changed since.
  c  the quests the lore speaks to (the lycanthropes, the were-hunters, the Plane of Disease and the Dead Hills, the
     Elddar and Takish'Hiz, Grobb, Qeynos, Halas): their words deepened.
  d  the bare "Bring me N" errands: a voice for the giver and the place.
  e  the short hand-in replies ("Well done."): a line in the speaker's voice.

A quest text entry names its quest by giver and name (with classes, races and level where several share them),
the row (step 0 the quest itself), the column, the words BEFORE and AFTER. It is written only where the row's
words are BEFORE (each matching quest: class copies alike); a row already AFTER is said as done; any other row is
skipped and said (another shard's or a GM's words). Mechanics are never written: no answer of a stage
(accept, decline), no step's answer outside part f, no item, figure, target, event or reward; the tool refuses an
entry that changes a number or drops a name the words had, an offer that changes its paragraphs or its question,
or words that break a PS2's rules (ASCII only, no '|', no '%', no other server or PC EverQuest place, one box a
line with its speaker's name, a journal line 127).

Report only by default; --apply in one transaction, a gm_audit line a row, every change in lore_dialog_1121_log
(part, row, column, old, new); a part already applied is skipped; --undo [--only ...] puts back exactly what each
part wrote (a row changed since is left, and said); --check FILE checks a data file's entries alone.

WHEN IT TAKES EFFECT: quests (parts a, c-f, and b's tale) once the quests are read again (a GM panel quest edit,
e.g. quest_set_enabled on one quest, or a restart); dialog lines and board words within 30 s.

    python3 -E -s tools/lore_dialog_20261121.py --db eqoa.db [--only a,b | --skip f] [--apply | --applied | --undo]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(HERE, "dialog", "lore_dialog_20261121")
PARTS = ["f", "a", "b", "c", "d", "e"]  # the order they are written in
LOG = "lore_dialog_1121_log"
SRC = "lore-dialog-1121"
WHO, VIA = "dab", "tool"
DETAIL = "the Lore Library in our NPCs' and quests' words (the owner 2026-11-21): tools/lore_dialog_20261121.py"

BOX = 217
ASK = 127
JOURNAL = 127
ANSWER = 63
SIGN_BODY = 0x6A82352C
QUEST_TEXT = ["offer", "accept", "decline", "declined", "started"]
STEP_TEXT = ["journal", "answer", "says", "waiting", "notyet", "reminder"]
WRITABLE = {"quests": ["offer", "declined", "started"],
            "queststeps": ["journal", "says", "waiting", "notyet", "reminder"]}
BANNED = ["Faydwer", "Faydark", "Kelethin", "Felwithe", "Kaladim", "Ak'Anon", "Akanon", "Erudin", "Kunark", "Velious",
          "Luclin", "Butcherblock", "Steamfont", "Paineel", "Antonica", "Trakanon", "Sandstorm", "eqoa.live",
          "Return Home", "EverQuest", "forumotion"]
# words a sentence may start with that are not names
NOT_NAMES = set("""A An And As At Be But By Do For From Go He Her Here His How I If In Into Is It Its Kill Let Me My No Not
Now Of On Once One Or Our She So Some Take That The Their Then There These They This Those Though To Too Two Three Four
Five Six Seven Eight Nine Ten Up We Well What When Where Which While Who Why Will With Yes You Your Bring Find Return
Speak Tell Talk Show Give Hunt Good Get Come Ask Keep See Look Walk Run Head Follow Make Help Use Wait Only Even Every
Each Any All Both More Most Much Many First Second Third Last Next Just Still Yet Also Ah Oh Hmm Hail Greetings Welcome
Thank Thanks Please Very Over After Before Again Ever Never Always Off Out Down South North East West Should Would Could
Can Must May Might Shall Did Does Has Have Had Was Were Are Am Been Being Do Done Such Same Other Another Mind Watch
Hold Stand Sit Put Carry Fetch Gather Seek Search Slay Defeat Destroy Beware Remember Know""".split())


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def js(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True) if v is not None else None


def lines_of(text):
    return [l.strip() for l in (text or "").replace("\r", "").split("\n") if l.strip()]


def paragraphs(text):
    out, cur = [], []
    for l in (text or "").replace("\r", "").split("\n"):
        if not l.strip():
            if cur:
                out.append(cur)
                cur = []
            continue
        cur.append(l.strip())
    if cur:
        out.append(cur)
    return out


def text_problems(text, what):
    out = []
    if any(ord(ch) > 126 or (ord(ch) < 32 and ch != "\n") for ch in text):
        out.append(f"{what}: not plain ASCII {sorted({ch for ch in text if ord(ch) > 126 or (ord(ch) < 32 and ch != chr(10))})!r}")
    if "|" in text:
        out.append(f"{what}: has '|'")
    if "%" in text:
        out.append(f"{what}: has '%'")
    for b in BANNED:
        if re.search(r"(?<![A-Za-z])" + re.escape(b.lower()) + r"(?![A-Za-z])", text.lower()):
            out.append(f"{what}: names {b!r}")
    return out


def names_in(text):
    """The capitalised words of text that are not at a sentence's start: names, as a reader sees them."""
    out = set()
    for m in re.finditer(r"[A-Z][A-Za-z'`]+", text or ""):
        w = m.group(0)
        before = (text[:m.start()]).rstrip(" \"'(")
        if not before or before[-1] in ".!?:;\n" or w in NOT_NAMES:
            continue
        out.add(w[:-2] if w.endswith("'s") else w.rstrip("'"))
    return out


def numbers_in(text):
    return re.findall(r"\d[\d,]*", text or "")


def entry_problems(e, speaker=None):
    """What is wrong with a quest text entry, whatever the database."""
    what = f"{e.get('giver')!r} {e.get('name')!r} step {e.get('step', 0)} {e.get('field')}"
    tbl = "quests" if int(e.get("step", 0)) == 0 else "queststeps"
    field, before, after = e.get("field"), e.get("before"), e.get("after")
    out = []
    if not isinstance(after, str) or not isinstance(before, str):
        return [f"{what}: before and after must be words"]
    if e.get("part") != "f" and field not in WRITABLE[tbl]:
        return [f"{what}: {field} is not a column these parts write ({WRITABLE[tbl]})"]
    if e.get("part") == "f":
        return out + text_problems(after, what) if after else out
    if after == before:
        out.append(f"{what}: after is before")
    out += text_problems(after, what)
    speaker = speaker or (e.get("target") if tbl == "queststeps" else e.get("giver")) or e.get("giver") or ""
    most = BOX - len(speaker) - 2
    for l in lines_of(after):
        if field == "journal":
            if len(l) > JOURNAL:
                out.append(f"{what}: journal {len(l)} > {JOURNAL}")
        elif len(l) > most:
            out.append(f"{what}: a line of {len(l)} > {most} ({BOX} less {speaker!r})")
    if field == "journal" and len(lines_of(after)) > 1:
        out.append(f"{what}: a journal is one line")
    if field == "offer":
        pb, pa = paragraphs(before), paragraphs(after)
        if len(pb) != len(pa):
            out.append(f"{what}: {len(pa)} paragraphs, before had {len(pb)} (the offer's stages)")
        lb, la = lines_of(before), lines_of(after)
        if lb and la and (len(lb[-1]) <= ASK) != (len(la[-1]) <= ASK):
            out.append(f"{what}: the last line's question ({len(la[-1])} chars) is not as before's ({len(lb[-1])})")
    for n in numbers_in(before):
        if n not in numbers_in(after):
            out.append(f"{what}: drops the number {n}")
    drops = set(e.get("drops", []))  # names the entry means to change (a spelling made the lore's)
    for n in names_in(before):
        if n not in after and n.rstrip("'") not in drops:
            out.append(f"{what}: drops the name {n!r}")
    if before.strip() and not after.strip():
        out.append(f"{what}: empties words that were there")
    return out


def load(part, data=DATA):
    p = os.path.join(data, f"{part}.json")
    if not os.path.exists(p):
        return {"entries": []}
    with open(p, encoding="utf-8") as f:
        d = json.load(f)
    for e in d.get("entries", []):
        e["part"] = part
    return d


def quests_of(db, e):
    w, a = "enabled = 1 AND giver = ? AND name = ?", [e["giver"], e["name"]]
    for k in ("classes", "races", "minlevel"):
        if k in e:
            w += f" AND {k} = ?"
            a.append(e[k])
    return [r[0] for r in db.execute(f"SELECT questid FROM quests WHERE {w} ORDER BY questid", a)]


class Plan:
    def __init__(self, db, parts, data=DATA):
        self.bad, self.skipped, self.done_already = [], [], []
        self.writes = {p: [] for p in parts}  # (tbl, key, field, old, new) or ('insert', tbl, row)
        self.parts = parts
        for t in ("quests", "queststeps", "npcdialog", "signtext", "npcs", "npcfaction", "gm_audit"):
            if not has(db, t):
                self.bad.append(f"no table {t}")
        if self.bad:
            return
        for part in parts:
            d = load(part, data)
            for e in d.get("entries", []):
                self.quest_entry(db, part, e)
            if part == "a":
                self.part_a_extra(db, d)
            if part == "b":
                self.part_b(db, d)

    def quest_entry(self, db, part, e):
        step = int(e.get("step", 0))
        tbl = "quests" if step == 0 else "queststeps"
        speaker = e.get("target")
        qids = quests_of(db, e)
        if not qids:
            self.skipped.append((part, f"no enabled quest {e['name']!r} by {e['giver']!r}"))
            return
        if step and not speaker:
            r = db.execute("SELECT target FROM queststeps WHERE questid = ? AND step = ?", (qids[0], step)).fetchone()
            speaker = r[0] if r else e["giver"]
        if part == "f":
            p = []
            texts = [e["heirloom"][f] for f in QUEST_TEXT] + [st["heirloom"][f] for st in e["steps"] for f in STEP_TEXT]
            for t in texts:
                # Heirloom's own words, %p (the player's name, filled by the server) as they stand there
                p += [x for x in text_problems(re.sub(r"%[prctn]", "", t or ""), f"{e['giver']!r} {e['name']!r}")]
        else:
            p = entry_problems(e, speaker)
        if p:
            self.bad += [f"[{part}] {x}" for x in p]
            return
        fields = [e["field"]] if part != "f" else None
        for q in qids:
            if part == "f":
                self.part_f_quest(db, e, q)
                continue
            key = {"questid": q} if step == 0 else {"questid": q, "step": step}
            w = " AND ".join(f"{c} = ?" for c in key)
            r = db.execute(f'SELECT {e["field"]} FROM "{tbl}" WHERE {w}', tuple(key.values())).fetchone()
            if r is None:
                self.skipped.append((part, f"quest {q} {e['name']!r} has no step {step}"))
            elif r[0] == e["after"]:
                self.done_already.append((part, f"quest {q} {e['name']!r} {e['field']}"))
            elif r[0] != e["before"]:
                self.skipped.append((part, f"quest {q} {e['name']!r} step {step} {e['field']} is not as read: {str(r[0])[:60]!r}"))
            else:
                self.writes[part].append((tbl, key, e["field"], r[0], e["after"]))

    def part_f_quest(self, db, e, q):
        """Heirloom's words onto this quest, where every row is as Live's was read and the steps are as read."""
        steps = {int(s["step"]): s for s in e["steps"]}
        have = {r[0]: r[1:] for r in db.execute(
            "SELECT step, kind, target, count FROM queststeps WHERE questid = ? ORDER BY step", (q,))}
        if sorted(have) != sorted(steps) or any(tuple(have[s]) != (steps[s]["kind"], steps[s]["target"], steps[s]["count"]) for s in have):
            self.skipped.append(("f", f"quest {q} {e['name']!r}: its steps are not as read"))
            return
        writes = []
        row = dict(zip(QUEST_TEXT, db.execute(f"SELECT {','.join(QUEST_TEXT)} FROM quests WHERE questid = ?", (q,)).fetchone()))
        wants = [("quests", {"questid": q}, f, row[f], e["live"][f], e["heirloom"][f]) for f in QUEST_TEXT]
        for s, st in steps.items():
            r = dict(zip(STEP_TEXT, db.execute(f"SELECT {','.join(STEP_TEXT)} FROM queststeps WHERE questid = ? AND step = ?", (q, s)).fetchone()))
            wants += [("queststeps", {"questid": q, "step": s}, f, r[f], st["live"][f], st["heirloom"][f]) for f in STEP_TEXT]
        if all(now == after for _, _, _, now, _, after in wants):
            self.done_already.append(("f", f"quest {q} {e['name']!r}"))
            return
        if any(now != before for _, _, _, now, before, _ in wants):
            bad = [f"{t} {k} {f}" for t, k, f, now, before, _ in wants if now != before]
            self.skipped.append(("f", f"quest {q} {e['name']!r}: not as Live's was read ({bad[0]}{' ...' if len(bad) > 1 else ''})"))
            return
        for t, k, f, now, before, after in wants:
            if now != after:
                writes.append((t, k, f, now, after))
        self.writes["f"] += writes

    def part_a_extra(self, db, d):
        for b in d.get("boards", []):
            p = text_problems(b["text"], f"board {b['name']!r}")
            if len(b["text"]) > 1000:
                p.append(f"board {b['name']!r}: {len(b['text'])} > 1000")
            if p:
                self.bad += p
                continue
            rows = db.execute("SELECT id, modelid FROM npcs WHERE npc_name = ?", (b["name"],)).fetchall()
            if not rows:
                self.skipped.append(("a", f"no board {b['name']!r} here"))
            for nid, model in rows:
                if (model & 0xFFFFFFFF) != SIGN_BODY:
                    self.bad.append(f"[a] {b['name']!r} ({nid}) is not on the sign body")
                elif db.execute("SELECT 1 FROM signtext WHERE npcid = ?", (nid,)).fetchone():
                    self.skipped.append(("a", f"board {b['name']!r} ({nid}) has words already"))
                else:
                    self.writes["a"].append(("insert", "signtext", {"npcid": nid, "kind": "board", "text": b["text"],
                                                                   "source": SRC, "note": "a town's welcome: " + DETAIL}))
        for blk in d.get("lines", []):
            town = blk["town"]
            if not db.execute("SELECT 1 FROM npcfaction WHERE faction = ?", ("Citizens of " + town,)).fetchone():
                self.skipped.append(("a", f"no faction 'Citizens of {town}': its lines skipped"))
                continue
            for l in blk["lines"]:
                p = text_problems(l, f"{town} line")
                if len(l) > BOX - 25 - 2:
                    p.append(f"{town} line {len(l)} > {BOX - 27}: {l[:40]!r}")
                if p:
                    self.bad += p
                elif db.execute("SELECT 1 FROM npcdialog WHERE line = ?", (l,)).fetchone():
                    self.skipped.append(("a", f"{town}: already said: {l[:50]!r}"))
                else:
                    self.writes["a"].append(("insert", "npcdialog", {"town": town, "line": l, "source": SRC}))

    def part_b(self, db, d):
        old, new = d["from"], d["to"]
        if not db.execute("SELECT 1 FROM npcs WHERE npc_name = ?", (d["npc"],)).fetchone():
            self.bad.append(f"[b] no NPC {d['npc']!r}: nothing to name the founder after")
            return
        if not has(db, "lore_library_1121_log"):
            self.skipped.append(("b", "tools/lore_library_20261121.py is not applied here"))
            return
        pat = re.compile(r"\b" + re.escape(old) + r"\b")
        for rid, line in db.execute("SELECT id, line FROM npcdialog WHERE source = 'lore-1121'"):
            if pat.search(line):
                self.writes["b"].append(("npcdialog", {"id": rid}, "line", line, pat.sub(new, line)))
        for nid, text in db.execute("SELECT npcid, text FROM signtext WHERE source = 'lore-1121'"):
            if pat.search(text):
                self.writes["b"].append(("signtext", {"npcid": nid}, "text", text, pat.sub(new, text)))
        for q, s, says, journal in db.execute(
                "SELECT s.questid, s.step, s.says, s.journal FROM queststeps s JOIN quests q USING (questid) "
                "WHERE q.name = ? AND q.giver = ?", (d["tale"], d["teller"])):
            for f, v in (("says", says), ("journal", journal)):
                if pat.search(v or ""):
                    self.writes["b"].append(("queststeps", {"questid": q, "step": s}, f, v, pat.sub(new, v)))
        for tbl, key, f, a, b in self.writes["b"]:
            self.bad += text_problems(b, f"[b] {tbl} {key}")


def apply(db, plan, parts):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, part TEXT NOT NULL, tbl TEXT NOT NULL, "
               "rowkey TEXT NOT NULL, field TEXT NOT NULL, old TEXT, new TEXT)")
    n = {}

    def audit(action, tbl, key, before, after):
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    for part in parts:
        n[part] = 0
        for w in plan.writes[part]:
            if w[0] == "insert":
                _, tbl, values = w
                have = {r[1] for r in db.execute(f'PRAGMA table_info("{tbl}")')}
                values = {c: v for c, v in values.items() if c in have}
                cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})',
                                 list(values.values()))
                key = {"npcid": values["npcid"]} if tbl == "signtext" else {"id": cur.lastrowid}
                wk = " AND ".join(f"{c} = ?" for c in key)
                c2 = db.execute(f'SELECT * FROM "{tbl}" WHERE {wk}', tuple(key.values()))
                row = dict(zip([x[0] for x in c2.description], c2.fetchone()))
                db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, field, old, new) VALUES (?, ?, ?, '*', NULL, ?)",
                           (part, tbl, js(key), js(row)))
                audit("insert", tbl, key, None, row)
            else:
                tbl, key, field, old, new = w
                wk = " AND ".join(f"{c} = ?" for c in key)
                db.execute(f'UPDATE "{tbl}" SET {field} = ? WHERE {wk}', [new] + list(key.values()))
                db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, field, old, new) VALUES (?, ?, ?, ?, ?, ?)",
                           (part, tbl, js(key), field, old, new))
                audit("update", tbl, key, {field: old}, {field: new})
            n[part] += 1
    return n


def undo(db, parts):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    back, left = {}, 0
    marks = ",".join("?" * len(parts))
    for seq, part, tbl, rowkey, field, old, new in db.execute(
            f"SELECT seq, part, tbl, rowkey, field, old, new FROM {LOG} WHERE part IN ({marks}) ORDER BY seq DESC",
            parts).fetchall():
        key = json.loads(rowkey)
        wk = " AND ".join(f"{c} = ?" for c in key)
        if field == "*":
            c2 = db.execute(f'SELECT * FROM "{tbl}" WHERE {wk}', tuple(key.values()))
            r = c2.fetchone()
            now = dict(zip([x[0] for x in c2.description], r)) if r else None
            if js(now) != new:
                print(f"  [{part}] {tbl} {rowkey}: changed since, left as it is")
                left += 1
                continue
            db.execute(f'DELETE FROM "{tbl}" WHERE {wk}', tuple(key.values()))
            before, after = now, None
        else:
            r = db.execute(f'SELECT {field} FROM "{tbl}" WHERE {wk}', tuple(key.values())).fetchone()
            if r is None or r[0] != new:
                print(f"  [{part}] {tbl} {rowkey} {field}: changed since, left as it is")
                left += 1
                continue
            db.execute(f'UPDATE "{tbl}" SET {field} = ? WHERE {wk}', [old] + list(key.values()))
            before, after = {field: new}, {field: old}
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, "delete" if field == "*" else "update", tbl, rowkey,
                    js(before), js(after), "undone: " + DETAIL))
        back[part] = back.get(part, 0) + 1
    if not db.execute(f"SELECT 1 FROM {LOG} LIMIT 1").fetchone():
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def check_file(path):
    part = os.path.basename(path).split(".")[0]
    with open(path, encoding="utf-8") as f:
        d = json.load(f)
    bad = []
    for e in d.get("entries", []):
        e["part"] = part
        if part != "f":
            bad += entry_problems(e)
    return bad


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db")
    ap.add_argument("--data", default=DATA)
    ap.add_argument("--only", default="")
    ap.add_argument("--skip", default="")
    ap.add_argument("--check", metavar="FILE")
    ap.add_argument("-v", "--verbose", action="store_true", help="list every row skipped")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--applied", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    if a.check:
        bad = check_file(a.check)
        print("\n".join(bad) if bad else "ok")
        return 1 if bad else 0
    if not a.db:
        ap.error("--db is needed")
    only = [p for p in a.only.split(",") if p] or PARTS
    skip = [p for p in a.skip.split(",") if p]
    if set(only + skip) - set(PARTS):
        ap.error(f"parts are {PARTS}")
    parts = [p for p in PARTS if p in only and p not in skip]
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.applied or a.undo:
            if not has(db, LOG):
                print("not applied")
                return 0 if a.undo else 1
            if a.applied:
                for part, tbl, c in db.execute(f"SELECT part, tbl, count(*) FROM {LOG} GROUP BY part, tbl ORDER BY part"):
                    print(f"  part {part}: {tbl} {c}")
                return 0
            db.execute("BEGIN IMMEDIATE")
            back, left = undo(db, parts)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back, {left} left")
            return 0
        done = set()
        if has(db, LOG):
            done = {r[0] for r in db.execute(f"SELECT DISTINCT part FROM {LOG}")}
        todo = [p for p in parts if p not in done]
        for p in parts:
            if p in done:
                print(f"  part {p}: applied already (--undo --only {p} to take it back)")
        if not todo:
            return 0
        plan = Plan(db, todo, a.data)
        from collections import Counter
        sk = Counter(p for p, _ in plan.skipped)
        al = Counter(p for p, _ in plan.done_already)
        for p in todo:
            print(f"  part {p}: {len(plan.writes[p])} rows to write, {sk.get(p, 0)} skipped, {al.get(p, 0)} as wanted already")
        for p, why in plan.skipped if a.verbose else plan.skipped[:40]:
            print(f"    skipped [{p}] {why}")
        if not a.verbose and len(plan.skipped) > 40:
            print(f"    ... {len(plan.skipped) - 40} more (-v)")
        if plan.bad:
            print("refused, nothing done:\n  " + "\n  ".join(plan.bad[:80]))
            return 1
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, plan, todo)
        db.execute("COMMIT")
        print(f"applied: {n}; quests at the next quest reload (quest_set_enabled on one quest) or restart, "
              f"dialog and boards within 30 s")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
