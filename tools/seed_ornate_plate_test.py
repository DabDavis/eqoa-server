import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_ornate_plate.py")

COLS = """patternid INTEGER PRIMARY KEY, itemcost INTEGER, unk1 INTEGER, itemicon INTEGER, unk2 INTEGER,
    equipslot INTEGER, unk3 INTEGER, trade INTEGER, rent INTEGER, unk4 INTEGER, attacktype INTEGER, weapondamage INTEGER,
    unk5 INTEGER, levelreq INTEGER, maxstack INTEGER, maxhp INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER,
    procanim INTEGER, lore INTEGER, unk6 INTEGER, craft INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER,
    color INTEGER, str INTEGER, sta INTEGER, AC INTEGER, HPMAX INTEGER"""


class SeedOrnatePlate(unittest.TestCase):
    """tools/seed_ornate_plate.py on a made-up database: five records of model 9 (set 9), colour 0, at the first id on,
    each the Valiant piece's figures (level, AC, stats, classes, value) under its own name and the level 48 Ornate
    piece's icon; a report writes nothing, a second apply writes nothing, an id or a name taken refuses the run, and an
    undo deletes what it wrote unless a character holds one. No game data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute(f"CREATE TABLE itempattern ({COLS})")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER)")
        valiant = [(5, "Valiant Chestplate"), (7, "Valiant Bracers"), (19, "Valiant Gloves"), (10, "Valiant Leggings"),
                   (11, "Valiant Boots")]
        ornate = [(5, "Ornate Tunic"), (7, "Ornate Bracers"), (19, "Ornate Gloves"), (10, "Ornate Leggings"),
                  (11, "Ornate Boots")]
        for i, (wear, name) in enumerate(valiant):
            db.execute("INSERT INTO itempattern (patternid, itemcost, itemicon, equipslot, trade, rent, levelreq, "
                       "classuse, raceuse, itemname, itemdesc, model, color, sta, AC, HPMAX) "
                       "VALUES (?, 6000, 111, ?, 0, 1, 60, 879, 1023, ?, 'plain', 4, -1, ?, ?, 150)",
                       (100 + i, wear, name, 10 + i, 400 + i))
        for i, (wear, name) in enumerate(ornate):
            db.execute("INSERT INTO itempattern (patternid, itemcost, itemicon, equipslot, levelreq, itemname, itemdesc, "
                       "model, color) VALUES (?, 1, ?, ?, 48, ?, '', 9, 0)", (200 + i, 900 + i, wear, name))
        db.commit()
        db.close()

    def tearDown(self):
        for f in os.listdir(os.path.dirname(self.path)):
            p = os.path.join(os.path.dirname(self.path), f)
            if f.startswith(os.path.basename(self.path)):
                os.remove(p)

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-I", TOOL, "--db", self.path, "--first-id", "5000", *args],
                           capture_output=True, text=True)
        if ok:
            self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        return r

    def rows(self):
        db = sqlite3.connect(self.path)
        out = db.execute("SELECT patternid, itemname, equipslot, model, color, levelreq, AC, sta, classuse, itemcost, "
                         "itemicon FROM itempattern WHERE patternid >= 5000 ORDER BY patternid").fetchall()
        db.close()
        return out

    def test_writes_five_set_nine_records(self):
        self.run_tool()
        self.assertEqual(self.rows(), [], "a report writes nothing")
        self.run_tool("--apply")
        got = self.rows()
        self.assertEqual([r[1] for r in got], ["Ornate Breastplate", "Ornate Vambraces", "Ornate Gauntlets",
                                               "Ornate Greaves", "Ornate Sabatons"])
        for i, r in enumerate(got):
            self.assertEqual(r[0], 5000 + i)
            self.assertEqual((r[3], r[4], r[5], r[8], r[9]), (9, 0, 60, 879, 6000), r)
            self.assertEqual((r[6], r[7]), (400 + i, 10 + i), "the Valiant piece's AC and stamina")
            self.assertEqual(r[10], 900 + i, "the level 48 Ornate piece's icon")
        self.assertEqual([r[2] for r in got], [5, 7, 19, 10, 11])
        self.run_tool("--apply")
        self.assertEqual(len(self.rows()), 5, "a second apply writes nothing more")
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, 5002)")
        db.commit()
        db.close()
        r = self.run_tool("--undo", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(len(self.rows()), 5, "a held record keeps the run")
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory")
        db.commit()
        db.close()
        self.run_tool("--undo")
        self.assertEqual(self.rows(), [])

    def test_refuses_a_taken_id_or_name(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO itempattern (patternid, itemname, model) VALUES (5003, 'Something Else', 1)")
        db.commit()
        db.close()
        r = self.run_tool("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("5003", r.stderr)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET patternid = 7000, itemname = 'Ornate Greaves' WHERE patternid = 5003")
        db.commit()
        db.close()
        r = self.run_tool("--apply", ok=False)
        self.assertIn("Ornate Greaves", r.stderr)
        self.assertEqual(len(self.rows()), 1, "nothing written")


if __name__ == "__main__":
    unittest.main()
