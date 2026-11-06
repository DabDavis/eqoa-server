#!/usr/bin/env python3
"""The guilds a restart ended, rebuilt from the server's log into the tables that keep them now (the user, 2026-11-06:
"did larrybirds guild not get saved"; until then guilds lived in the server's memory alone, entity/guild.go, so the
restart of 04:47 UTC that night ended "Bad Boys" and "Prophets of Zion").

The log says each change (handlers/guild.go): "Guild: <who> created "<name>" (<id>)", "... joined "<name>"",
"... left "<name>"", "... removed <whom> from "<name>"", "... set <whom> to rank <n>", "... disbanded "<name>"". It is
replayed from --since (the last start before the guilds were lost) to its end, ACROSS the restarts after it, as if
the guilds had been kept: a guild made again under its old name after a restart starts over with its new leader (the
players rejoin it), a join takes a character out of any other guild, a leader leaving hands the guild to its highest
member (the lowest character id among equals), the last one out ends it, as the World does. A charter is not in the
log and is left empty. Characters by their names in `characters`; one the log names that no longer exists is left out.

It writes only into empty tables (a server that keeps guilds has written its own: --force replaces them), in one
transaction, and prints what it wrote. Run it with the server paused or stopped (a keep restart's before.sh), so the
new image reads the guilds at its start.

    tools/restore_guilds_from_log.py --db eqoa.db --since "2026/11/06 02:28:35" gameserver.log.1.gz gameserver.log [--apply] [--force]
"""

import argparse
import gzip
import re
import sqlite3
import sys

LEADER = 3
LINE = re.compile(r'^(\d{4}/\d\d/\d\d \d\d:\d\d:\d\d) Guild: (.*)$')
EVENTS = [
    ("created", re.compile(r'^(.+?) created "(.+)" \(\d+\)$')),
    ("disbanded", re.compile(r'^(.+?) disbanded "(.+)"$')),
    ("left", re.compile(r'^(.+?) left "(.+)"$')),
    ("removed", re.compile(r'^(.+?) removed (.+?) from "(.+)"$')),
    ("rank", re.compile(r'^(.+?) set (.+?) to rank (\d+)$')),
    ("joined", re.compile(r'^(.+?) joined "(.+)"$')),
]


def lines_of(paths):
    for path in paths:
        opener = gzip.open if path.endswith(".gz") else open
        with opener(path, "rt", errors="replace") as f:
            for line in f:
                yield line.rstrip("\n")


class Guilds:
    def __init__(self):
        self.guilds = {}  # name (lower) -> {"name", "members": {who: rank}, "order": n}
        self.n = 0

    def of(self, who):
        for key, g in self.guilds.items():
            if who in g["members"]:
                return key
        return None

    def drop(self, key, who):
        g = self.guilds.get(key)
        if not g or who not in g["members"]:
            return
        was = g["members"].pop(who)
        if not g["members"]:
            del self.guilds[key]
            return
        if was == LEADER and LEADER not in g["members"].values():
            best = max(g["members"].items(), key=lambda kv: kv[1])  # its highest (the World's rule, by rank)
            g["members"][best[0]] = LEADER

    def apply(self, kind, m):
        if kind == "created":
            who, name = m.group(1), m.group(2)
            if (k := self.of(who)) is not None:
                self.drop(k, who)
            self.n += 1
            self.guilds[name.lower()] = {"name": name, "members": {who: LEADER}, "order": self.n}
        elif kind == "disbanded":
            self.guilds.pop(m.group(2).lower(), None)
        elif kind == "left":
            self.drop(m.group(2).lower(), m.group(1))
        elif kind == "removed":
            self.drop(m.group(3).lower(), m.group(2))
        elif kind == "rank":
            whom, rank = m.group(2), int(m.group(3))
            k = self.of(whom)
            if k is not None:
                self.guilds[k]["members"][whom] = rank
        elif kind == "joined":
            who, name = m.group(1), m.group(2)
            if (k := self.of(who)) is not None and k != name.lower():
                self.drop(k, who)
            g = self.guilds.get(name.lower())
            if g is None:  # a guild the replay did not see made: one of a member
                self.n += 1
                g = self.guilds[name.lower()] = {"name": name, "members": {}, "order": self.n}
            g["members"].setdefault(who, 0)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--since", required=True, help='the log time to replay from, "YYYY/MM/DD HH:MM:SS"')
    ap.add_argument("logs", nargs="+", help="the logs in order, oldest first (.gz read as gzip)")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--force", action="store_true", help="replace guilds the tables already hold")
    a = ap.parse_args()
    state = Guilds()
    seen = 0
    for line in lines_of(a.logs):
        m = LINE.match(line)
        if not m or m.group(1) < a.since:
            continue
        for kind, rx in EVENTS:
            e = rx.match(m.group(2))
            if e:
                state.apply(kind, e)
                seen += 1
                break
    db = sqlite3.connect(a.db, timeout=10, isolation_level=None)
    ids = {name: sid for sid, name in db.execute("SELECT serverid, charName FROM characters")}
    out, missing = [], set()
    for gid, g in enumerate(sorted(state.guilds.values(), key=lambda g: g["order"]), start=1):
        members = [(ids[w], r, w) for w, r in g["members"].items() if w in ids]
        missing |= {w for w in g["members"] if w not in ids}
        if not members:
            continue
        if not any(r == LEADER for _, r, _ in members):
            members.sort(key=lambda t: (-t[1], t[0]))
            members[0] = (members[0][0], LEADER, members[0][2])
        out.append((gid, g["name"], members))
    print(f"{seen} guild events since {a.since}: {len(out)} guilds")
    for gid, name, members in out:
        lead = next(w for _, r, w in members if r == LEADER)
        print(f"  {gid} {name!r}, led by {lead}: " + ", ".join(f"{w} {r}" for _, r, w in sorted(members, key=lambda t: (-t[1], t[2]))))
    if missing:
        print("  left out (no such character):", ", ".join(sorted(missing)))
    if not a.apply:
        return
    have = db.execute("SELECT COUNT(*) FROM guilds").fetchone()[0] + db.execute("SELECT COUNT(*) FROM guild_members").fetchone()[0]
    if have and not a.force:
        sys.exit(f"the tables hold guilds already ({have} rows): --force to replace them")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute("DELETE FROM guild_members")
        db.execute("DELETE FROM guilds")
        for gid, name, members in out:
            lead = next(sid for sid, r, _ in members if r == LEADER)
            db.execute("INSERT INTO guilds (guild_id, name, leader_id, motd) VALUES (?, ?, ?, '')", (gid, name, lead))
            for sid, r, _ in members:
                db.execute("INSERT INTO guild_members (guild_id, player_id, rank) VALUES (?, ?, ?)", (gid, sid, r))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {len(out)} guilds")


if __name__ == "__main__":
    main()
