#!/usr/bin/env python3
"""tools/price_zero_items_20261115.py on a made-up database: each kind of record worth 0 is kept or priced by its
rule, a second --apply does nothing, and --undo gives back the database exactly (but its gm_audit lines). No game
data.

    python3 -E -s tools/price_zero_items_20261115_test.py
"""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import price_zero_items_20261115 as tool  # noqa: E402

SCROLL = tool.SCROLL_ART
COLS = ["patternid", "itemcost", "itemname", "equipslot", "trade", "rent", "levelreq", "maxstack", "itemicon",
        "weapondamage", "AC", "HPMAX", "POWMAX", "PoT", "HoT"] + list(tool.STAT_COLS)


def item(pid, name, cost=0, slot=-1, level=10, stack=1, icon=1, dmg=0, ac=0, st=0, rent=1, trade=0):
    r = {c: 0 for c in COLS}
    r.update(patternid=pid, itemcost=cost, itemname=name, equipslot=slot, trade=trade, rent=rent, levelreq=level,
             maxstack=stack, itemicon=icon, weapondamage=dmg, AC=ac, str=st)
    return r


def table_dump(db, t):
    return sorted(db.execute(f"SELECT * FROM {t}").fetchall())


class PriceZero(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (" + ", ".join(
            f"{c} {'TEXT' if c == 'itemname' else 'INTEGER'}" for c in COLS) + ")")
        for ddl in [
            "CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, "
            "via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, "
            "reverted_by INTEGER)",
            "CREATE TABLE accountinfo (accountid INTEGER, username TEXT)",
            "CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER)",
            "CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY, class INTEGER, role TEXT, epic INTEGER)",
            "CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY, npcid INTEGER, patternid INTEGER)",
            "CREATE TABLE startitems (id INTEGER PRIMARY KEY, patternid INTEGER)",
            "CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER)",
            "CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT, fxdict INTEGER, source TEXT, effects TEXT)",
            "CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT)",
            "CREATE TABLE quests (questid INTEGER PRIMARY KEY, repeatable INTEGER, events TEXT)",
            "CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, items TEXT, rewarditems TEXT, "
            "byclass TEXT, choices TEXT, events TEXT)",
            "CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, level INTEGER, rarity INTEGER, "
            "product INTEGER, yield INTEGER, ingredients TEXT)",
            "CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_level INTEGER)",
            "CREATE TABLE lootpool (id INTEGER PRIMARY KEY, category TEXT, npc_name TEXT, patternid INTEGER, "
            "minlevel INTEGER, maxlevel INTEGER)",
            "CREATE TABLE loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER, minlevel INTEGER, "
            "maxlevel INTEGER)",
            "CREATE TABLE npcloot (npc_name TEXT, tablename TEXT)",
        ]:
            db.execute(ddl)
        rows = [
            # priced references: helms of slot 1
            item(10, "Leather Cap", 100, 1, 10, ac=10), item(11, "Bronze Cap", 120, 1, 11, ac=11),
            item(12, "Iron Cap", 140, 1, 12, ac=12), item(13, "Steel Cap", 900, 1, 40, ac=40),
            item(14, "Mithril Cap", 1000, 1, 42, ac=42), item(15, "Adamant Cap", 1100, 1, 44, ac=44),
            item(16, "Hide Cap", 110, 1, 11, ac=11), item(17, "Chain Cap", 130, 1, 12, ac=12),
            item(18, "Gold Cap", 950, 1, 41, ac=41),
            # priced scrolls of level 10
            item(20, "Fire Bolt", 50, icon=SCROLL), item(21, "Ice Bolt", 60, icon=SCROLL), item(22, "Shock", 70, icon=SCROLL),
            # priced trash of the 20s
            item(30, "rat tail", 40, level=0, stack=99), item(31, "rat ear", 50, level=0, stack=99),
            item(32, "rat eye", 60, level=0, stack=99),
            # components
            item(40, "Iron Bar", 300, stack=50), item(41, "Ring Mold", 10, stack=20),
            # --- worth 0 ---
            item(100, "Ascended Cap", 0, 1, 12, ac=13),                 # copy
            item(101, "Test Helm 1", 0, 1, 12, ac=12),                  # placeholder
            item(102, "Heirloom Helm", 0, 1, 1),                        # heirloom
            item(103, "Conjured Bread", 0, stack=20, rent=0),           # NO RENT
            item(104, "Summoned Water", 0, stack=20),                   # a create_item's id
            item(105, "Lava Stone", 0),                                 # a Summon line's name
            item(106, "Lava Stone", 0, icon=SCROLL),                    # its scroll: priced as a scroll
            item(107, "Free Tunic", 0, 5, 1),                           # a merchant gives it away
            item(108, "Apprentice Sword", 0, 12, 1, dmg=10),            # a new character's kit
            item(109, "Daily Ring", 0, 8, 10, st=5),                    # a repeatable quest's reward
            item(110, "Old Key", 0, icon=SCROLL),                       # a quest asks for it
            item(111, "Letter", 0),                                     # a quest gives it, no gear
            item(112, "Fire Bolt II", 0, icon=SCROLL),                  # a quest gives a spell scroll: priced
            item(113, "Lost Helm", 0, 1, 12, ac=12),                    # nothing gives it: unobtainable
            item(114, "Mid Cap", 0, 1, 11, ac=11, trade=1),             # gear, no trade, dropped: priced
            item(115, "Golden Coif", 0, 1, 0, ac=42),                   # level 0 with AC: AC measures it
            item(116, "Iron Gem Ring", 0, 8, 3),                        # a crafted product
            item(117, "Bloodstone", 0, stack=50),                       # a jeweler's gem (kept on a failure)
            item(118, "rat whisker", 0, level=0, stack=99),             # trash of a level 25 rat
            item(119, "Shock", 0, icon=SCROLL),                         # a merchant sells a Shock at 70
            item(120, "Ring of Veeshan", 0, 8, 50),                     # a GM set it to 0
            item(121, "Odd Trinket", 0),                                # held, but no rule fits
        ]
        db.executemany(f"INSERT INTO itempattern ({', '.join(COLS)}) VALUES ({', '.join('?' * len(COLS))})",
                       [[r[c] for c in COLS] for r in rows])
        db.execute("INSERT INTO itemascended VALUES (100, 12)")
        db.execute("INSERT INTO heirlooms VALUES (102, 0, 'tank', 0)")
        db.execute("INSERT INTO spelleffect VALUES (1, 'Self, Summon Lava Stone x4', 0, 't', '')")
        db.execute("INSERT INTO spelleffect VALUES (2, 'Self, Summon Water', 0, 't', ?)", (json.dumps(
            {"effects": [{"kind": "create_item", "item": {"patternid": 104, "name": "Summoned Water", "count": 3}}]}),))
        db.executemany("INSERT INTO spellpattern VALUES (?, ?)",
                       [(1, "Lava Stone"), (2, "Fire Bolt"), (3, "Ice Bolt"), (4, "Shock"), (5, "Fire Bolt II")])
        db.executemany("INSERT INTO npcitem (npcid, patternid) VALUES (?, ?)", [(1, 107), (1, 22)])
        db.execute("INSERT INTO startitems (patternid) VALUES (108)")
        db.executemany("INSERT INTO quests VALUES (?, ?, '')", [(1, 1), (2, 0)])
        db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?, '', '', ?)", [
            (1, 1, "talk", "", "109", ""),
            (2, 1, "give", "110", "", json.dumps([{"on": "done", "do": "give", "items": "111"}])),
            (2, 2, "talk", "", "112*1", "")])
        db.execute("INSERT INTO craftrecipes VALUES (1, 3, 1, 0, 116, 1, ?)", (json.dumps(
            [{"item": 40, "count": 1, "keep": False}, {"item": 41, "count": 1, "keep": False},
             {"item": 117, "count": 1, "keep": True}]),))
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?)", [(1, "a rat", 25), (2, "a goblin", 12)])
        db.executemany("INSERT INTO lootpool (category, npc_name, patternid, minlevel, maxlevel) VALUES (?, ?, ?, 1, 100)",
                       [("trash", "a rat", 30), ("trash", "a rat", 31), ("trash", "a rat", 32), ("trash", "a rat", 118),
                        ("uncommon", "a goblin", 114), ("uncommon", "a goblin", 115), ("uncommon", "a goblin", 119),
                        ("uncommon", "a goblin", 120), ("uncommon", "a goblin", 106), ("gems", "a goblin", 117)])
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, 121)")
        db.execute("INSERT INTO gm_audit (tbl, rowkey, before, after) VALUES ('itempattern', ?, ?, ?)",
                   (json.dumps({"patternid": 120}), json.dumps({"itemcost": 4215}), json.dumps({"itemcost": 0})))
        db.commit()
        db.close()

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            rc = tool.main(["--db", self.path, *args])
        return rc, out.getvalue()

    def test_classify(self):
        db = sqlite3.connect(self.path)
        priced, kept = tool.classify(db)
        db.close()
        k = {p: r for p, _n, r, _h in kept}
        v = {p: (r, val) for p, _n, r, val, _h in priced}
        self.assertEqual(k, {100: "copy", 101: "placeholder", 102: "heirloom", 103: "conjured", 104: "conjured",
                             105: "conjured", 107: "merchant-free", 108: "start-kit", 109: "repeatable",
                             110: "quest", 111: "quest", 113: "unobtainable", 120: "gm-zeroed", 121: "no-rule"})
        self.assertEqual(v[106], ("scroll", 60))      # Lava Stone's scroll: the scrolls of its level
        self.assertEqual(v[112], ("scroll", 60))
        self.assertEqual(v[114], ("gear", 120))       # the nearest caps of level 10-12 and AC 10-12
        self.assertEqual(v[115][0], "gear")
        self.assertEqual(v[115][1], 950)              # AC 42: the four high caps and the next, its level 0 not measured
        self.assertEqual(v[116], ("crafted", 3 * 300 + 10 + tool.MOD_WORTH[0]))
        self.assertEqual(v[117], ("material", tool.GEM_PRICE[0]))
        self.assertEqual(v[118], ("drop", 50))        # the trash of levels 20-29
        self.assertEqual(v[119], ("scroll", 60))      # not held to 70: the merchant's price is above it
        self.assertEqual(set(v), {106, 112, 114, 115, 116, 117, 118, 119})

    def test_merchant_price_caps(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET itemcost = 45 WHERE patternid = 22")   # the stocked Shock at 45
        db.commit()
        priced, _kept = tool.classify(db)
        db.close()
        got = {p: val for p, _n, _r, val, _h in priced}
        self.assertEqual(got[119], 45)

    def test_apply_reapply_undo(self):
        db = sqlite3.connect(self.path)
        before = table_dump(db, "itempattern")
        db.close()
        rc, out = self.run_tool()
        self.assertEqual(rc, 0)
        self.assertIn("report only", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(table_dump(db, "itempattern"), before)
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("applied: 8 values written", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 116").fetchone()[0], 960)
        self.assertEqual(db.execute(f"SELECT count(*) FROM {tool.LOG}").fetchone()[0], 8)
        aud = db.execute("SELECT rowkey, before, after, username, via FROM gm_audit WHERE id > 1 ORDER BY id").fetchall()
        self.assertEqual(len(aud), 8)
        self.assertIn((json.dumps({"patternid": 116}, separators=(",", ":")), '{"itemcost":0}', '{"itemcost":960}',
                       "dab", "tool"), aud)
        applied = table_dump(db, "itempattern")
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertIn("nothing to do", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(table_dump(db, "itempattern"), applied)
        db.close()
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 0, out)
        self.assertIn("put back 8", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(table_dump(db, "itempattern"), before)
        self.assertFalse(tool.table(db, tool.LOG))
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail LIKE 'undo:%'").fetchone()[0], 8)
        db.close()

    def test_undo_leaves_a_changed_record(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET itemcost = 7 WHERE patternid = 118")
        db.commit()
        db.close()
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 1)
        self.assertIn("left: 118", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 118").fetchone()[0], 7)
        self.assertEqual(db.execute(f"SELECT patternid FROM {tool.LOG}").fetchall(), [(118,)])
        db.close()


if __name__ == "__main__":
    unittest.main()
