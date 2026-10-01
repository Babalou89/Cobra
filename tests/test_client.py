import io
import os
import subprocess
import sys
import tempfile
import unittest

from librarian import client
from tests.helpers import hdr
from tests.test_server import ServerCase, U

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


class TestClient(ServerCase):
    def run_cli(self, *args, env=None, url=None):
        e = dict(os.environ)
        e.pop("LIBRARIAN_BYPASS", None)
        e["LIBRARIAN_BYPASS_LOG"] = os.path.join(self.sdir, "bypass.log")
        e.update(env or {})
        os.makedirs(self.sdir, exist_ok=True)
        p = subprocess.run([sys.executable, "-m", "librarian.client", "--url", url or self.url, "--timeout", "2", *args],
                           cwd=REPO, env=e, capture_output=True, text=True)
        return p.returncode, p.stderr

    def test_allow_exit_0(self):
        rc, err = self.run_cli("agent", "create", U + "access/adb/0002vadb.md")
        self.assertEqual(rc, 0, err)

    def test_deny_exit_2_with_reason(self):
        rc, err = self.run_cli("agent", "create", U + "access/adb/junk.md")
        self.assertEqual(rc, 2)
        self.assertIn("R01", err)
        self.assertIn("0002vadb.md", err)

    def test_unreachable_fails_closed(self):
        rc, err = self.run_cli("agent", "create", U + "access/adb/0002vadb.md", url="http://127.0.0.1:1")
        self.assertEqual(rc, 2)
        self.assertIn("failing closed", err)

    def test_bypass_unreachable_logs_to_spool_then_flushes(self):
        env = {"LIBRARIAN_BYPASS": "1"}
        rc, err = self.run_cli("billy", "create", "whatever", env=env, url="http://127.0.0.1:1")
        self.assertEqual(rc, 0)
        spool = os.path.join(self.sdir, "bypass.log")
        self.assertTrue(os.path.getsize(spool) > 0)
        self.assertEqual(self.db.bypasses(), [])
        # next normal contact with the server flushes the spool
        rc, err = self.run_cli("agent", "create", U + "access/adb/0002vadb.md")
        self.assertEqual(rc, 0, err)
        self.assertFalse(os.path.exists(spool))
        self.assertEqual(len(self.db.bypasses()), 1)
        self.assertEqual(self.db.bypasses()[0]["actor"], "billy")

    def test_bypass_reachable_overrides_deny_and_posts(self):
        rc, err = self.run_cli("billy", "create", U + "access/adb/junk.md", env={"LIBRARIAN_BYPASS": "1"})
        self.assertEqual(rc, 0)
        self.assertEqual(len(self.db.bypasses()), 1)

    def test_lock_denies_through_client(self):
        self.db.open_incident("x", "create", "p", "R01", "d")
        rc, err = self.run_cli("agent", "create", U + "access/adb/0002vadb.md")
        self.assertEqual(rc, 2)
        self.assertIn("LOCK", err)

    def test_content_file(self):
        cf = os.path.join(self.sdir, "c.md")
        os.makedirs(self.sdir, exist_ok=True)
        with open(cf, "w") as f:
            f.write("bad header")
        rc, err = self.run_cli("--content-file", cf, "agent", "create", U + "access/adb/0002vadb.md")
        self.assertEqual(rc, 2)
        self.assertIn("R09", err)


class TestCli(ServerCase):
    def cli(self, *args):
        p = subprocess.run([sys.executable, "-m", "librarian.cli", "--db", self.db.path, *args], cwd=REPO,
                           capture_output=True, text=True)
        return p.returncode, p.stdout, p.stderr

    def test_resolve_cli_unlocks(self):
        iid = self.db.open_incident("x", "create", "p", "R01", "d")
        rc, out, err = self.cli("resolve", str(iid), "revert")
        self.assertEqual(rc, 0, err)
        self.assertIn("lock released", out)
        self.assertEqual(self.db.open_ids(), [])
        rc, out, err = self.cli("resolve", str(iid), "revert")
        self.assertEqual(rc, 1)

    def test_scan_and_map_cli(self):
        rc, out, err = self.cli("scan", self.root)
        self.assertEqual(rc, 0, out + err)
        self.assertIn("0 violation(s)", out)
        rc, out, err = self.cli("map", self.root, "projects/demo")
        self.assertIn("- access/adb/ | v0001", out)


if __name__ == "__main__":
    unittest.main()
