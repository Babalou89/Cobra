import os
import tempfile
import unittest

from librarian.db import DB
from librarian.rules import check
from tests.helpers import TreeCase, hdr

U = "projects/demo/"


class TestIncidents(TreeCase):
    def setUp(self):
        super().setUp()
        self.build()
        import shutil
        self.addCleanup(shutil.rmtree, self.root + "-state", True)
        self.db = DB(os.path.join(self.root + "-state", "lib.db"))
        self.addCleanup(self.db.close)

    def chk(self, op="create", path=U + "access/adb/0002vadb.md", content=None, **kw):
        return check(self.root, "agent", op, path, content or hdr("adb", 2), lock=self.db.open_ids, **kw)

    def test_open_incident_locks_everything(self):
        self.assertTrue(self.chk().allow)
        iid = self.db.open_incident("someone", "create", "x/y", "R01", "bad file")
        for v in (self.chk(), self.chk("edit", U + "SPEC.md", "a\nCurrent: x\nNext: none"),
                  self.chk("delete", U + "access/serial")):
            self.assertFalse(v.allow)
            self.assertEqual(v.rule_id, "LOCK")
            self.assertIn(str(iid), v.reason)
        lib = check(self.root, "librarian", "move", U + "access/adb/0001vadb.md",
                    dest=U + "raw/legacy-2026-09-30/0001vadb.md", lock=self.db.open_ids)
        self.assertEqual(lib.rule_id, "LOCK")   # even the librarian actor is frozen

    def test_resolve_unlocks(self):
        a = self.db.open_incident("x", "create", "p1", "R01", "d")
        b = self.db.open_incident("x", "create", "p2", "R02", "d")
        self.db.resolve(a, "revert")
        self.assertFalse(self.chk().allow)            # b still open
        self.db.resolve(b, "approve", self.root)
        self.assertTrue(self.chk().allow)
        rows = {r["id"]: r for r in self.db.incidents()}
        self.assertEqual((rows[a]["state"], rows[a]["resolution"]), ("resolved", "revert"))
        self.assertEqual((rows[b]["state"], rows[b]["resolution"]), ("resolved", "approve"))

    def test_resolve_errors(self):
        i = self.db.open_incident("x", "create", "p", "R01", "d")
        with self.assertRaises(ValueError):
            self.db.resolve(i, "maybe")
        self.db.resolve(i, "revert")
        with self.assertRaises(ValueError):
            self.db.resolve(i, "revert")
        with self.assertRaises(KeyError):
            self.db.resolve(999, "revert")

    def test_persists_across_connections(self):
        i = self.db.open_incident("x", "create", "p", "R01", "d")
        path = self.db.path
        db2 = DB(path)
        self.addCleanup(db2.close)
        self.assertEqual(db2.open_ids(), [i])

    def test_approve_restores_quarantined(self):
        q = os.path.join(self.root + "-state", "q", "1", "stray.txt")
        os.makedirs(os.path.dirname(q))
        with open(q, "w") as f:
            f.write("data")
        i = self.db.open_incident("x", "create", "stray.txt", "R05", "d", quarantined_to=q)
        self.db.resolve(i, "approve", self.root)
        self.assertTrue(os.path.isfile(os.path.join(self.root, "stray.txt")))
        self.assertIn("stray.txt", self.db.exceptions())

    def test_revert_keeps_quarantine(self):
        q = os.path.join(self.root + "-state", "q", "1", "stray.txt")
        os.makedirs(os.path.dirname(q))
        with open(q, "w") as f:
            f.write("data")
        i = self.db.open_incident("x", "create", "stray.txt", "R05", "d", quarantined_to=q)
        self.db.resolve(i, "revert", self.root)
        self.assertTrue(os.path.isfile(q))
        self.assertFalse(os.path.exists(os.path.join(self.root, "stray.txt")))


if __name__ == "__main__":
    unittest.main()
