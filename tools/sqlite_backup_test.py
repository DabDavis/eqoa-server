from contextlib import closing
import os
from pathlib import Path
import sqlite3
import tempfile
import unittest

if __package__:
    from .sqlite_backup import backup_database
else:
    from sqlite_backup import backup_database


class SQLiteBackupTest(unittest.TestCase):
    def test_backups_include_committed_wal_and_keep_previous_copy(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "owner #1.db"
            dest = Path(folder) / "backup.db"
            with closing(sqlite3.connect(path)) as writer:
                writer.execute("PRAGMA journal_mode=WAL")
                writer.execute("PRAGMA wal_autocheckpoint=0")
                writer.execute("CREATE TABLE accounts(id INTEGER)")
                writer.commit()
                writer.execute("PRAGMA wal_checkpoint(TRUNCATE)")
                backups = []
                for value in (1, 2):
                    writer.execute("INSERT INTO accounts VALUES (?)", (value,))
                    writer.commit()
                    backups.append(backup_database(path, dest))
                self.assertNotEqual(*backups)
                for name, expected in zip(backups, [[(1,)], [(1,), (2,)]]):
                    with closing(sqlite3.connect(name)) as db:
                        self.assertEqual(db.execute("SELECT id FROM accounts ORDER BY id").fetchall(), expected)
                    self.assertEqual(os.stat(name).st_mode & 0o077, 0)

    def test_missing_or_invalid_source_leaves_no_backup(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "missing"
            dest = Path(folder) / "backup"
            with self.assertRaises(sqlite3.Error):
                backup_database(path, dest)
            self.assertEqual(list(Path(folder).iterdir()), [])
            path.write_text("invalid database")
            with self.assertRaises(sqlite3.Error):
                backup_database(path, dest)
            self.assertEqual(list(Path(folder).iterdir()), [path])


if __name__ == "__main__":
    unittest.main()
