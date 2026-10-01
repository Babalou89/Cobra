"""Table-driven coverage: every rule family has >=2 allow and >=2 deny cases (enforced by a meta-test)."""
import os
import unittest
from collections import defaultdict

from librarian.rules import check
from tests.helpers import TreeCase, hdr, mk, SPEC_OK

U = "projects/demo/"
H = hdr
# (family, expected rule id or "OK", op, path, content, actor, dest)
CASES = [
    # R00
    ("R00", "OK", "create", U + "access/adb/0002vadb.md", H("adb", 2), "a", None),
    ("R00", "OK", "create", ".git/hooks/x", "x", "a", None),
    ("R00", "R00", "chmod", U + "SPEC.md", None, "a", None),
    ("R00", "R00", "create", "/etc/hosts", "x", "a", None),
    # R01
    ("R01", "OK", "create", U + "scripts/t/0001vt.py", "x", "a", None),
    ("R01", "OK", "create", U + "access/adb/0002vadb.md", H("adb", 2), "a", None),
    ("R01", "R01", "create", U + "access/adb/0002vusb.md", H("usb", 2), "a", None),
    ("R01", "R01", "create", U + "access/adb/readme.md", "x", "a", None),
    # R02
    ("R02", "OK", "create", U + "hardware/0002vhardware.md", H("hardware", 2), "a", None),
    ("R02", "OK", "create", U + "access/serial/0001vserial.md", H("serial", 1), "a", None),
    ("R02", "R02", "create", U + "hardware/0003vhardware.md", H("hardware", 3), "a", None),
    ("R02", "R02", "create", U + "access/serial/0000vserial.md", H("serial", 0), "a", None),
    # R03
    ("R03", "OK", "move", U + "hardware/0001vhardware.md", None, "librarian", U + "raw/legacy-2026-09-30/0001vhardware.md"),
    ("R03", "OK", "create", U + "hardware/0002vhardware.md", H("hardware", 2), "a", None),
    ("R03", "R03", "edit", U + "hardware/0001vhardware.md", None, "a", None),
    ("R03", "R03", "delete", U + "hardware/0001vhardware.md", None, "a", None),
    # R04
    ("R04", "OK", "create", U + "access/usb/", None, "a", None),
    ("R04", "OK", "create", U + "access/usb/0001vusb.md", H("usb", 1), "a", None),
    ("R04", "R04", "create", U + "access/0001vaccess.md", H("access", 1), "a", None),
    ("R04", "R04", "create", U + "hardware/sub/", None, "a", None),
    # R05
    ("R05", "OK", "create", U + "RULES.md", "r", "a", None),
    ("R05", "OK", "create", "CLAUDE.md", "r", "a", None),
    ("R05", "R05", "create", U + "notes.md", "r", "a", None),
    ("R05", "R05", "delete", U + "SPEC.md", None, "a", None),
    # R06
    ("R06", "OK", "create", U + "raw/dumps/Some Vendor.bin", "x", "a", None),
    ("R06", "OK", "create", U + "raw/legacy-2026-09-30/0001Vold.txt", "x", "a", None),
    ("R06", "R06", "create", U + "hardware/raw/", None, "a", None),
    ("R06", "R06", "delete", U + "raw", None, "a", None),
    # R07
    ("R07", "OK", "create", U + "good-dir/", None, "a", None),
    ("R07", "OK", "create", "projects/other-unit/", None, "a", None),
    ("R07", "R07", "create", U + "Bad_Dir/", None, "a", None),
    ("R07", "R07", "create", U + "0001vdir/", None, "a", None),
    # R08
    ("R08", "OK", "create", U + "a/b/c/", None, "a", None),
    ("R08", "OK", "create", U + "raw/a/b/c/d/x.bin", "x", "a", None),
    ("R08", "R08", "create", U + "a/b/c/d/", None, "a", None),
    ("R08", "R08", "create", U + "a/b/c/d/0001vd.md", H("d", 1), "a", None),
    # R09
    ("R09", "OK", "create", U + "hardware/0002vhardware.md", H("hardware", 2, status="solved"), "a", None),
    ("R09", "OK", "create", U + "access/adb/0002vadb.md", H("adb", 2, status="blocked"), "a", None),
    ("R09", "R09", "create", U + "hardware/0002vhardware.md", "no header", "a", None),
    ("R09", "R09", "create", U + "hardware/0002vhardware.md", H("hardware", 2, sup="none"), "a", None),
    # R10
    ("R10", "OK", "write", U + "SPEC.md", "x\nCurrent: a\nNext: none\n", "a", None),
    ("R10", "OK", "edit", U + "SPEC.md", "x\nCurrent: a\nNext: do it", "a", None),
    ("R10", "R10", "write", U + "SPEC.md", "x\nNext: none\nCurrent: a\n", "a", None),
    ("R10", "R10", "write", U + "SPEC.md", "no tail", "a", None),
    # R11
    ("R11", "OK", "create", "infra/", None, "a", None),
    ("R11", "OK", "create", "archive/old-stuff/", None, "a", None),
    ("R11", "R11", "create", "architeture/", None, "a", None),
    ("R11", "R11", "create", "recon/", None, "a", None),
]


class TestFamilies(TreeCase):
    def setUp(self):
        super().setUp()
        self.build()

    def test_table(self):
        for fam, exp, op, path, content, actor, dest in CASES:
            with self.subTest(fam=fam, op=op, path=path):
                v = check(self.root, actor, op, path, content, dest=dest)
                if exp == "OK":
                    self.assertTrue(v.allow, "%s %s: %s %s" % (op, path, v.rule_id, v.reason))
                else:
                    self.assertFalse(v.allow, "%s %s should be denied" % (op, path))
                    self.assertEqual(v.rule_id, exp, v.reason)

    def test_coverage_at_least_two_each(self):
        count = defaultdict(lambda: [0, 0])
        for fam, exp, *_ in CASES:
            count[fam][0 if exp == "OK" else 1] += 1
        for r in ["R%02d" % i for i in range(12)]:
            self.assertGreaterEqual(count[r][0], 2, r + " allow cases")
            self.assertGreaterEqual(count[r][1], 2, r + " deny cases")


if __name__ == "__main__":
    unittest.main()
