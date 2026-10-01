import os
import unittest

from librarian.db import DB
from librarian.rules import check
from librarian.scan import scan, reconcile
from tests.fixture_fyt import FIXTURE
from tests.helpers import TreeCase, hdr, mk, unit_tree, SPEC_OK

U = "projects/demo/"


def rules(vs):
    return sorted({v.rule for v in vs})


class TestScan(TreeCase):
    def test_valid_unit_clean(self):
        self.build()
        self.assertEqual(scan(self.root), [])

    def test_fyt_fixture_tree_clean(self):
        for op, rel, content in FIXTURE:
            mk(self.root, {rel: content})
        self.assertEqual([(v.path, v.rule) for v in scan(self.root)], [])

    def test_seeded_violations(self):
        self.build()
        mk(self.root, {
            "stray.txt": "x",                                           # R05 root
            "projects/loose.md": "x",                                   # R05 group
            "recon/a.txt": "x",                                         # R11 (+ provisional scan)
            "projects/Bad_Unit/x": "x",                                 # R07 unit
            "projects/nospec/hardware/0001vhardware.md": hdr("hardware", 1),   # R05 missing SPEC/MEMORY
            U + "fyt.prop": "x",                                        # R05 unit
            U + "access/adb/0002Vadb.md": "x",                          # R01
            U + "hardware/0003vhardware.md": hdr("hardware", 3),        # R02 gap
            U + "access/usb/0001vusb.md": "no header",                  # R09
            U + "access/usb2/x": None,
            U + "mixed/0001vmixed.md": hdr("mixed", 1),                 # R04 (file + subdir)
            U + "mixed/sub/0001vsub.md": hdr("sub", 1),
            U + "Weird_Dir/x.txt": "x",                                 # R07
            U + "a/b/c/d/0001vd.md": hdr("d", 1),                       # R08
            U + "hardware/raw/x": "x",                                  # R06
            U + "SPEC.md": "no tail\n",                                 # R10
        })
        vs = scan(self.root)
        found = rules(vs)
        for r in ("R01", "R02", "R04", "R05", "R06", "R07", "R08", "R09", "R10", "R11"):
            self.assertIn(r, found, r)
        bypath = {(v.path, v.rule) for v in vs}
        self.assertIn(("stray.txt", "R05"), bypath)
        self.assertIn(("projects/nospec", "R05"), bypath)
        self.assertIn(("projects/demo/a/b/c/d", "R08"), bypath)
        d = [v for v in vs if v.path == "projects/demo/Weird_Dir"][0]
        self.assertEqual(d.beneath, 1)
        r01 = [v for v in vs if v.rule == "R01" and v.path.endswith("0002Vadb.md")][0]
        self.assertEqual(r01.suggested, "0002vadb.md")

    def test_duplicate_version_and_gap(self):
        self.build()
        mk(self.root, {U + "hardware/0001vhardware.txt": "x", U + "access/adb/0003vadb.md": hdr("adb", 3)})
        vs = scan(self.root)
        self.assertEqual({(v.path, v.rule) for v in vs}, {
            (U + "hardware", "R02"), (U + "access/adb", "R02")})

    def test_scan_agrees_with_check_on_valid_creates(self):
        # whatever check() allows as a create, scan must accept afterwards
        self.build()
        for rel, content in [(U + "access/adb/0002vadb.md", hdr("adb", 2)), (U + "scripts/x/0001vx.py", "p"),
                             (U + "raw/dumps/a b.bin", "x"), (U + "RULES.md", "r")]:
            v = check(self.root, "a", "create", rel, content)
            self.assertTrue(v.allow, v.reason)
            mk(self.root, {rel: content})
        self.assertEqual(scan(self.root), [])

    def test_ignore_git(self):
        self.build()
        mk(self.root, {".git/HEAD": "x", ".git/objects/ab/cd": "x"})
        self.assertEqual(scan(self.root), [])


class TestTamper(TreeCase):
    def setUp(self):
        super().setUp()
        self.build()
        mk(self.root, {U + "raw/dumps/a.bin": "orig"})
        self.db = DB(":memory:")

    def test_image_populated(self):
        reconcile(self.root, self.db)
        f = self.db.get_file(U + "access/adb/0001vadb.md")
        self.assertEqual((f["version"], f["slug"], f["ext"], f["status"], f["header_status"], f["leaf"]),
                         (1, "adb", "md", "numbered", "ok", "access/adb"))
        self.assertEqual(self.db.get_file(U + "raw/dumps/a.bin")["status"], "raw")
        self.assertEqual(len(self.db.get_file(U + "SPEC.md")["sha256"]), 64)

    def test_modified_numbered_detected_and_incident(self):
        reconcile(self.root, self.db)
        with open(os.path.join(self.root, U, "access/adb/0001vadb.md"), "a") as f:
            f.write("tampered\n")
        vs, new = reconcile(self.root, self.db)
        self.assertEqual([(v.path, v.rule, v.kind) for v in vs], [(U + "access/adb/0001vadb.md", "R03", "tamper")])
        self.assertEqual(len(new), 1)
        # second reconcile: image updated, no re-report
        vs, new = reconcile(self.root, self.db)
        self.assertEqual(vs, [])

    def test_deleted_and_raw_modified(self):
        reconcile(self.root, self.db)
        os.remove(os.path.join(self.root, U, "hardware/0001vhardware.md"))
        with open(os.path.join(self.root, U, "raw/dumps/a.bin"), "w") as f:
            f.write("changed!")
        vs, new = reconcile(self.root, self.db)
        self.assertEqual({(v.path, v.rule) for v in vs}, {(U + "hardware/0001vhardware.md", "R03"), (U + "raw/dumps/a.bin", "R06")})

    def test_other_violations_do_not_lock_unless_asked(self):
        mk(self.root, {"stray.txt": "x"})
        vs, new = reconcile(self.root, self.db)
        self.assertTrue(vs)
        self.assertEqual(new, [])
        self.assertEqual(self.db.open_ids(), [])
        vs, new = reconcile(self.root, self.db, open_incidents=True)
        self.assertEqual(len(new), 1)
        vs, new = reconcile(self.root, self.db, open_incidents=True)   # idempotent
        self.assertEqual(new, [])

    def test_exception_suppresses(self):
        mk(self.root, {"stray.txt": "x"})
        vs, new = reconcile(self.root, self.db, open_incidents=True)
        self.db.resolve(new[0], "approve", self.root)
        self.assertEqual(scan(self.root, self.db), [])


if __name__ == "__main__":
    unittest.main()
