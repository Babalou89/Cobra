import os
import unittest

from librarian import mapgen
from librarian.rules import check
from tests.helpers import TreeCase, hdr, mk

U = "projects/demo"


class TestMap(TreeCase):
    def setUp(self):
        super().setUp()
        self.build()
        mk(self.root, {
            U + "/access/adb/0002vadb.md": hdr("adb", 2, status="blocked", summary="adb over wifi blocked by permission"),
            U + "/scripts/diag/0001vdiag.py": "x",
            U + "/scripts/diag/0002vdiag.py": "x",
        })

    def test_build_map_lines(self):
        m = mapgen.build_map(self.root, U)
        lines = m.splitlines()
        self.assertEqual(lines[0], "## Map")
        self.assertEqual(lines[1], mapgen.BEGIN)
        self.assertEqual(lines[-1], mapgen.END)
        self.assertIn("- access/adb/ | v0002 | blocked | adb over wifi blocked by permission", lines)
        self.assertIn("- hardware/ | v0001 | open | s", lines)
        self.assertIn("- scripts/diag/ | v0002 | - | -", lines)
        self.assertIn("- access/serial/ | v0000 | empty | -", lines)
        self.assertFalse([l for l in lines if "raw" in l.split("|")[0]])
        self.assertFalse([l for l in lines if l.startswith("- access/ ")])   # branch dir is not a leaf

    def test_update_spec_keeps_tail_and_is_idempotent(self):
        new = mapgen.update_spec(self.root, U)
        again = mapgen.update_spec(self.root, U)
        self.assertEqual(new, again)
        tail = [l for l in new.splitlines() if l.strip()][-2:]
        self.assertTrue(tail[0].startswith("Current:") and tail[1].startswith("Next:"))
        self.assertEqual(new.count(mapgen.BEGIN), 1)
        self.assertIn("v0002 | blocked", new)
        v = check(self.root, "a", "write", U + "/SPEC.md", new)
        self.assertTrue(v.allow, v.reason)

    def test_update_replaces_stale_block(self):
        mapgen.update_spec(self.root, U)
        mk(self.root, {U + "/hardware/0002vhardware.md": hdr("hardware", 2, status="solved", summary="done")})
        new = mapgen.update_spec(self.root, U)
        self.assertIn("- hardware/ | v0002 | solved | done", new)
        self.assertNotIn("v0001 | open | s\n", new)

    def test_apply_map_without_existing_section(self):
        out = mapgen.apply_map("# t\ngoal\n\nCurrent: a\nNext: none\n", "## Map\nB\n")
        self.assertTrue(out.index("## Map") < out.index("Current:"))


if __name__ == "__main__":
    unittest.main()
