import os
import re
import shutil
import tempfile
import unittest

from librarian import migration, treeparse
from librarian.scan import scan

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TREE = os.path.join(REPO, "fixtures", "babalou2-home-tree.txt")


class TestLb3(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.text = open(TREE, errors="replace").read()
        cls.dest = tempfile.mkdtemp(prefix="lb3-")
        cls.nd, cls.nf, cls.summary, cls.empty_dirs = treeparse.build_replica(cls.text, cls.dest)
        cls.vs = scan(cls.dest)

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.dest, True)

    def test_parse_matches_tree_summary(self):
        self.assertEqual(self.summary, (82, 272))
        self.assertEqual(self.nf, 272)
        self.assertEqual(self.nd + 1, 82)      # tree counts the root dir too

    def test_replica_files_are_empty(self):
        for dp, dn, fn in os.walk(self.dest):
            for f in fn:
                self.assertEqual(os.path.getsize(os.path.join(dp, f)), 0)

    def test_known_findings(self):
        by = {(v.path, v.rule) for v in self.vs}
        for top in ("architeture", "agent-pool", "operator-core", "recon", "watch"):
            self.assertIn((top, "R11"), by, top)
        for f in ("fyt.prop", "pull_test.sh", "test_reverse.sh", "headunit_screen.jpg", "0024vrecon-babalou2-storage.sh"):
            self.assertIn((f, "R05"), by, f)
        self.assertIn(("projects/fyt-7862-diagnostic", "R05"), by)       # missing SPEC/MEMORY
        self.assertTrue(any(p.startswith("projects/fyt-7862-diagnostic/diagnostic_2026") and r == "R07" for p, r in by))
        self.assertIn(("projects/0001Vagent-pool-build", "R07"), by)

    def test_every_violation_has_a_class(self):
        for v in self.vs:
            cid, label, action = migration.classify(v)
            self.assertTrue(cid and label and action, v)
            self.assertIn(cid, migration.CLASSES)

    def test_scan_did_not_modify_replica(self):
        n = sum(len(f) for _, _, f in os.walk(self.dest))
        self.assertEqual(n, 272)

    def test_report_written_and_consistent(self):
        out = os.path.join(tempfile.mkdtemp(), "r.txt")
        total = migration.write_report(self.dest, self.vs, out, self.summary, self.nd, self.nf)
        txt = open(out).read()
        self.assertEqual(total, len(self.vs))
        m = re.search(r"Total violations: (\d+)", txt)
        self.assertEqual(int(m.group(1)), len(self.vs))
        rule_counts = [int(x) for x in re.findall(r"^\[R\d\d\] (\d+) violation", txt, re.M)]
        self.assertEqual(sum(rule_counts), len(self.vs))
        class_counts = [int(x) for x in re.findall(r"^(?:A\d\d) \((\d+)\)", txt, re.M)]
        self.assertEqual(sum(class_counts), len(self.vs))
        self.assertIn("Top 5 migration actions", txt)
        self.assertIn("No auto-fix", txt)


if __name__ == "__main__":
    unittest.main()
