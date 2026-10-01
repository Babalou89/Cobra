import os
import unittest

from librarian.rules import check
from tests.fixture_fyt import FIXTURE, U
from tests.helpers import TreeCase, mk


class TestFytFixture(TreeCase):
    def test_every_step_allowed_in_order(self):
        for op, rel, content in FIXTURE:
            v = check(self.root, "agent", op, rel, content)
            self.assertTrue(v.allow, "%s %s -> %s %s" % (op, rel, v.rule_id, v.reason))
            mk(self.root, {rel: content})

    def test_fixture_bad_variants_denied(self):
        for op, rel, content in FIXTURE:
            mk(self.root, {rel: content})
        # legacy-style names inside the chain leaves are refused
        self.assertFalse(check(self.root, "agent", "create", U + "scripts/diagnostic/0002Vdiagnostic.py", "x").allow)
        self.assertFalse(check(self.root, "agent", "create", U + "access/adb/0002_adb.md", "x").allow)
        self.assertFalse(check(self.root, "agent", "edit", U + "access/adb/0001vadb.md").allow)
        self.assertFalse(check(self.root, "agent", "create", U + "rooting/success/sub/deeper/deepest/x/").allow)


if __name__ == "__main__":
    unittest.main()
