"""Exercise distribution selection using fixture assets and a stub Go build."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
import tarfile


class DistroTest(unittest.TestCase):
    def test_package_excludes_operator_data_and_replaces_only_after_checks(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            server = root / "server"
            pkg = root / "go-eqoa-esf-tools"

            def put(path, data="fixture"):
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(data)

            (server / "tools").mkdir(parents=True)
            script = server / "tools" / "make-distro.sh"
            shutil.copyfile(Path(__file__).with_name("make-distro.sh"), script)
            for path in (
                "server/cmd/gameserver/main.go", "server/internal/network/tcp/patchserver.go",
                "server/sql/npcs.sql", "server/LICENSE", "server/NOTICE", "server/CLAUDE.md",
                "server/go.mod", "server/go.sum", "server/config.yaml.example",
                "server/tools/players-README.txt", "wintest/README.md",
                "go-eqoa-esf-tools/pkg/buffer/buffer.go", "go-eqoa-esf-tools/pkg/esf/zone.go",
                "go-eqoa-esf-tools/LICENSE", "go-eqoa-esf-tools/NOTICE",
                "eqoa-boot/out/EQOABOOT.ELF", "eqoa-boot/dist/README.txt",
                "eqscene/dist/EQOAHDD.ELF", "eqscene/dist/README.txt", "eqscene/dist/LICENSE.ps2sdk",
                "eqoa-diag/dist/EQOADIAG.ELF", "eqoa-diag/dist/README.txt",
            ):
                put(root / path)
            for path in ("serve/SD", "serve/SS", "serve/S2", "SD-hdd-mc0", "SD-hddnpc",
                         "SD-worlds", "ss-s2boot.dat", "S2-race10-nohelm"):
                put(server / "patches" / path)
            for repo in (server, pkg):
                subprocess.run(["git", "init", "-q", str(repo)], check=True)
                subprocess.run(["git", "-C", str(repo), "add", "."], check=True)
            put(server / "eqoa.db")
            put(server / "internal" / "untracked private data.db")
            put(root / "bin" / "go", """#!/bin/sh
case "$1" in
build) touch gameserver.exe ;;
run) exit "${CHECK_STATUS:-0}" ;;
*) exit 1 ;;
esac
""")
            (root / "bin" / "go").chmod(0o755)
            staging = root / "stage"
            unrelated = staging / "go-eqoa-server" / "operator-file"
            put(unrelated, "keep")
            out = root / "operators.tar.gz"
            out.write_bytes(b"old archive")
            env = dict(os.environ, STAGE_ROOT=str(staging),
                       PATH=f"{root}/bin:{os.environ['PATH']}")
            cmd = ["bash", str(script), "--out", str(out)]
            result = subprocess.run(cmd, cwd=server, env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            with tarfile.open(out) as archive:
                self.assertFalse(any(name.endswith(".db") for name in archive.getnames()))
                for name in ("NOTICE", "LICENSE", "_deps/eqoa-pkg/pkg/esf/zone.go"):
                    self.assertIn("go-eqoa-server/" + name, archive.getnames())
            verified = out.read_bytes()
            failed = subprocess.run(cmd, cwd=server, env=dict(env, CHECK_STATUS="1"), capture_output=True)
            self.assertNotEqual(failed.returncode, 0)
            self.assertEqual(out.read_bytes(), verified)
            self.assertEqual(unrelated.read_text(), "keep")
            self.assertEqual(list(staging.iterdir()), [unrelated.parent])


if __name__ == "__main__":
    unittest.main()
