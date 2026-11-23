"""Create consistent, owner-readable backups for SQLite maintenance scripts."""
from contextlib import closing
import os
from pathlib import Path
import sqlite3
import tempfile


def backup_database(source, destination):
    """Snapshot committed data, including WAL pages, and return the published path."""
    uri = Path(source).resolve().as_uri() + "?mode=ro"
    with closing(sqlite3.connect(uri, uri=True)) as src:
        fd, temp = tempfile.mkstemp(prefix=".sqlite-backup-", dir=Path(destination).parent)
        os.close(fd)
        try:
            with closing(sqlite3.connect(temp)) as dst:
                src.backup(dst)
            # Two runs within the same timestamp keep both snapshots. Publish
            # only a complete database, without replacing an existing file.
            name = str(destination)
            index = 1
            while True:
                try:
                    os.link(temp, name)
                    return name
                except FileExistsError:
                    name = f"{destination}.{index}"
                    index += 1
        finally:
            os.unlink(temp)
