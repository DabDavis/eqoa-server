#!/usr/bin/env python3
"""tools/swap_when_empty.sh in a sandbox: sudo and systemctl are stand-ins
(the connection table read from a file, the service a small TCP listener on a
free port), the server's directory and database made up. Nothing staged does
nothing; a UDP flow to the game port, or a packet in the log's last two
minutes, holds the swap; empty starts the clock and a later run swaps (the
binary moved in, copies of it and the database made, the staging done); a
binary whose sha256 file disagrees is refused; one that does not listen is
taken back out. A staged before.sh runs with the server stopped and before the
new binary starts; one that fails leaves the old binary running on the
database as it was, and so does a new binary that does not listen after one.
No game data.

    python3 tools/swap_when_empty_test.py
"""
import os
import socket
import sqlite3
import subprocess
import tempfile
import textwrap
import time
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "swap_when_empty.sh")

FAKE_SUDO = textwrap.dedent("""\
    #!/bin/bash
    # the stand-in for sudo: the connection table, and systemctl
    [ "$1" = "-n" ] && shift
    if [ "$1" = "cat" ] && [ "$2" = "/proc/net/nf_conntrack" ]; then cat "$FAKE_CONNTRACK"; exit; fi
    if [ "$1" = "systemctl" ]; then
        case "$2" in
        stop) [ -f "$FAKE_DIR/server.pid" ] && kill "$(cat "$FAKE_DIR/server.pid")" 2>/dev/null; rm -f "$FAKE_DIR/server.pid"; sleep 0.2;;
        start)
            # the "binary" says whether it starts: one holding "broken" does not listen
            grep -q broken "$EQOA_DIR/gameserver" && exit 0
            python3 -c "import socket, time; s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); s.bind(('127.0.0.1', $EQOA_AUTH_PORT)); s.listen(); time.sleep(600)" </dev/null >/dev/null 2>&1 9>&- &
            echo $! > "$FAKE_DIR/server.pid"; sleep 0.5;;
        esac
        exit 0
    fi
    exec "$@"
    """)

TABLE_EMPTY = "ipv4     2 tcp      6 431999 ESTABLISHED src=10.0.0.2 dst=10.0.0.1 sport=50000 dport=22 [ASSURED]\n"
TABLE_BUSY = TABLE_EMPTY + ("ipv4     2 udp      17 119 src=203.0.113.9 dst=198.51.100.1 sport=61307 dport=10070 "
                            "src=198.51.100.1 dst=203.0.113.9 sport=10070 dport=61307 [ASSURED]\n")


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class SwapWhenEmpty(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        t = self.tmp.name
        self.dir, self.swap, self.bin = f"{t}/server", f"{t}/swap", f"{t}/bin"
        for d in (self.dir, self.swap, self.bin):
            os.makedirs(d)
        with open(f"{self.bin}/sudo", "w") as f:
            f.write(FAKE_SUDO)
        os.chmod(f"{self.bin}/sudo", 0o755)
        with open(f"{self.dir}/gameserver", "w") as f:
            f.write("old binary\n")
        db = sqlite3.connect(f"{self.dir}/eqoa.db")
        db.execute("CREATE TABLE t (x)")
        db.commit()
        db.close()
        self.table = f"{t}/conntrack"
        self.set_table(TABLE_EMPTY)
        with open(f"{self.dir}/gameserver.log", "w") as f:
            f.write("2020/01/01 00:00:00 UDP received 28 bytes from 203.0.113.9:61307\n")
        self.port = free_port()
        self.env = dict(os.environ, PATH=f"{self.bin}:{os.environ['PATH']}", EQOA_DIR=self.dir, EQOA_SWAP_DIR=self.swap,
                        EQOA_UNIT="fake", EQOA_AUTH_PORT=str(self.port), EQOA_QUIET_SECONDS="1",
                        FAKE_CONNTRACK=self.table, FAKE_DIR=t)
        self.run_sudo("systemctl", "start")

    def tearDown(self):
        self.run_sudo("systemctl", "stop")
        self.tmp.cleanup()

    def run_sudo(self, *args):
        subprocess.run([f"{self.bin}/sudo", *args], env=self.env, check=True)

    def set_table(self, text):
        with open(self.table, "w") as f:
            f.write(text)

    def stage(self, content, sha=None):
        p = f"{self.swap}/pending"
        os.makedirs(p, exist_ok=True)
        with open(f"{p}/gameserver", "w") as f:
            f.write(content)
        if sha is None:
            sha = subprocess.run(["sha256sum", f"{p}/gameserver"], capture_output=True, text=True).stdout[:64]
        with open(f"{p}/sha256", "w") as f:
            f.write(sha + "\n")

    def run_script(self):
        r = subprocess.run(["bash", SCRIPT], env=self.env, capture_output=True, text=True)
        return r.returncode, r.stdout + r.stderr

    def binary(self):
        with open(f"{self.dir}/gameserver") as f:
            return f.read()

    def listening(self):
        with socket.socket() as s:
            return s.connect_ex(("127.0.0.1", self.port)) == 0

    def test_nothing_staged(self):
        code, out = self.run_script()
        self.assertEqual((code, out), (0, ""))

    def test_someone_there_holds_it(self):
        self.stage("new binary\n")
        self.set_table(TABLE_BUSY)
        for _ in range(2):
            self.run_script()
            time.sleep(1.1)
        self.assertEqual(self.binary(), "old binary\n")
        self.assertFalse(os.path.exists(f"{self.swap}/empty-since"))
        # a packet in the log's last two minutes holds it too
        self.set_table(TABLE_EMPTY)
        self.env["EQOA_QUIET_SECONDS"] = "120"
        with open(f"{self.dir}/gameserver.log", "a") as f:
            f.write(time.strftime("%Y/%m/%d %H:%M:%S") + " UDP received 28 bytes from 203.0.113.9:61307\n")
        self.run_script()
        self.assertFalse(os.path.exists(f"{self.swap}/empty-since"))

    def test_empty_swaps(self):
        self.stage("new binary\n")
        code, out = self.run_script()
        self.assertIn("empty: the staged binary goes in", out)
        self.assertEqual(self.binary(), "old binary\n")
        time.sleep(1.1)
        code, out = self.run_script()
        self.assertEqual(code, 0, out)
        self.assertIn("swapped", out)
        self.assertEqual(self.binary(), "new binary\n")
        self.assertTrue(self.listening())
        names = os.listdir(self.dir)
        self.assertTrue(any(n.startswith("gameserver.bak-") and n.endswith("-pre-swap") for n in names), names)
        self.assertTrue(any(n.startswith("eqoa.db.bak-") and n.endswith("-pre-swap") for n in names), names)
        self.assertFalse(os.path.exists(f"{self.swap}/pending"))
        self.assertTrue(any(n.startswith("done-") for n in os.listdir(self.swap)))

    def test_a_wrong_sha_is_refused(self):
        self.stage("new binary\n", sha="0" * 64)
        self.run_script()
        time.sleep(1.1)
        code, out = self.run_script()
        self.assertIn("refused", out)
        self.assertEqual(self.binary(), "old binary\n")
        self.assertTrue(any(n.startswith("failed-") for n in os.listdir(self.swap)))

    def stage_before(self, fail=False):
        """a before.sh that writes a row (and says whether the server was up), failing after it if asked"""
        with open(f"{self.swap}/pending/before.sh", "w") as f:
            f.write("#!/bin/bash\n"
                    f"(exec 3<>/dev/tcp/127.0.0.1/{self.port}) 2>/dev/null && up=up || up=down\n"
                    f"sqlite3 {self.dir}/eqoa.db \"INSERT INTO t VALUES ('$up')\"\n"
                    + ("exit 1\n" if fail else ""))
        os.chmod(f"{self.swap}/pending/before.sh", 0o755)

    def rows(self):
        db = sqlite3.connect(f"{self.dir}/eqoa.db")
        try:
            return [x for (x,) in db.execute("SELECT x FROM t")]
        finally:
            db.close()

    def test_before_runs_with_the_server_stopped(self):
        self.stage("new binary\n")
        self.stage_before()
        self.run_script()
        time.sleep(1.1)
        code, out = self.run_script()
        self.assertEqual(code, 0, out)
        self.assertIn("before.sh done", out)
        self.assertEqual(self.binary(), "new binary\n")
        self.assertEqual(self.rows(), ["down"])  # written with the server stopped, kept
        self.assertTrue(self.listening())

    def test_a_failed_before_leaves_the_old_binary_and_database(self):
        self.stage("new binary\n")
        self.stage_before(fail=True)
        self.run_script()
        time.sleep(1.1)
        code, out = self.run_script()
        self.assertNotEqual(code, 0)
        self.assertIn("before.sh FAILED", out)
        self.assertIn("the database is back as it was before the stop", out)
        self.assertEqual(self.binary(), "old binary\n")
        self.assertEqual(self.rows(), [])
        self.assertTrue(self.listening())
        self.assertTrue(any(n.startswith("failed-") for n in os.listdir(self.swap)))

    def test_a_new_binary_that_does_not_listen_takes_before_back(self):
        self.stage("broken binary\n")
        self.stage_before()
        self.run_script()
        time.sleep(1.1)
        code, out = self.run_script()
        self.assertIn("the old one goes back", out)
        self.assertIn("the database is back as it was before the stop", out)
        self.assertEqual(self.binary(), "old binary\n")
        self.assertEqual(self.rows(), [])
        self.assertTrue(self.listening())

    def test_a_binary_that_does_not_listen_goes_back_out(self):
        self.stage("broken binary\n")
        self.run_script()
        time.sleep(1.1)
        code, out = self.run_script()
        self.assertIn("the old one goes back", out)
        self.assertIn("the old binary is back and listening", out)
        self.assertEqual(self.binary(), "old binary\n")
        self.assertTrue(self.listening())


if __name__ == "__main__":
    unittest.main()
