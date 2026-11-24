import sqlite3
import tempfile
import unittest
from pathlib import Path

import work_handoff as view


class WorkHandoff(unittest.TestCase):
    def test_current_owner_wait_and_closed_version(self):
        row = dict(id=2030, version=9, status="in_progress", owner="jibrish", title="Marks", next="Check",
                   waiting_on="jibrish")
        rows = [dict(row, version=10, status="done"), row, dict(row, id=1, waiting_on="dab"),
                dict(row, id=2, waiting_on=""), dict(row, id=3), dict(row, id=4, waiting_on="logout")]
        result = view.handoff(rows)
        self.assertNotIn("2030", str(result))
        self.assertFalse(any(view.handoff([{k: v for k, v in rows[0].items() if k not in {"next", "waiting_on"}}]).values()))
        for key, ident in (("Needs Jibrish", 3), ("Waiting on Dab", 1), ("Ready for Codex", 2), ("Other waits", 4)):
            self.assertTrue(result[key][0].startswith(f"#{ident} v9"))

    def test_projected_read_only_input_and_pagination(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "work.db"
            db = sqlite3.connect(path)
            db.execute("CREATE TABLE workboard(id INTEGER, version INTEGER, status, owner, title, next, waiting_on, notes)")
            db.execute("INSERT INTO workboard VALUES(2,4,'planned','jibrish','Q','Run check','','large history')")
            db.commit()
            db.close()
            before = path.read_bytes()
            rows = view.database_rows(path, [2])
            self.assertEqual(rows[0]["version"], 4)
            self.assertNotIn("notes", rows[0])
            self.assertEqual(view.database_rows(path, [2030]), [])
            self.assertEqual(before, path.read_bytes())
        with self.assertRaisesRegex(ValueError, "incomplete"):
            view.entries_export({"entries": [], "matching": 2})
