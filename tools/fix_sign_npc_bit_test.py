import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
BOARD = 0x6A82352C


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FixSignNPCBit(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_level INTEGER, "
                   "npc_type INTEGER, modelid INTEGER)")
        # the board's body is kept signed in the table, as the loader stores it
        signed = BOARD - (1 << 32) if BOARD >= 1 << 31 else BOARD
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
            (3104, "Fayspires Announcements", 65, 0, signed),
            (3087, "signpost", 1, 0, signed),
            (462, "Freeport Announcements", 65, 384, signed),  # has it already
            (5000, "Guard Gorm", 40, 0, 0x70D898C6),  # the control: another body, left alone
        ])
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fix_sign_npc_bit.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=True).stdout

    def test_boards_get_the_bit_and_undo_puts_back(self):
        self.assertIn("2 signs and boards without the NPC bit", self.tool())
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("gave 2 the NPC bit", self.tool("--apply"))
        db = sqlite3.connect(self.path)
        types = dict(db.execute("SELECT id, npc_type FROM npcs"))
        db.close()
        self.assertEqual(types, {3104: 0x80, 3087: 0x80, 462: 384, 5000: 0}, "the boards; Freeport's and the guard as they were")
        self.assertIn("0 signs and boards", self.tool(), "nothing left to do")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
