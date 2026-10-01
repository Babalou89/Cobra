import os
import shutil
import time
import unittest

from librarian.db import DB
from librarian.notify import LogNotifier
from librarian.rules import Config, check
from librarian.server import Librarian
from librarian.watcher import Watcher
from tests.helpers import TreeCase, hdr, mk

U = "projects/demo/"


def wait_for(cond, timeout=3.0, step=0.02):
    t0 = time.time()
    while time.time() - t0 < timeout:
        if cond():
            return time.time() - t0
        time.sleep(step)
    return None


class WatchBase(TreeCase):
    use_inotify = False

    def setUp(self):
        super().setUp()
        self.build()
        self.sdir = self.root + "-state"
        self.addCleanup(shutil.rmtree, self.sdir, True)
        self.db = DB(os.path.join(self.sdir, "lib.db"))
        self.addCleanup(self.db.close)
        self.notifier = LogNotifier(os.path.join(self.sdir, "alarm.log"))
        self.lib = Librarian(self.root, self.db, notifier=self.notifier)
        self.lib.startup()
        self.q = os.path.join(self.sdir, "quarantine")

    def watcher(self, **kw):
        w = Watcher(self.lib, self.q, settle=kw.pop("settle", 0.2), poll=1.0, use_inotify=self.use_inotify, **kw)
        self.addCleanup(w.stop)
        return w

    def path(self, rel):
        return os.path.join(self.root, rel)


class TestWatcherSteps(WatchBase):
    """Deterministic: drive step() with explicit clock."""

    def settle(self, w):
        w.step(now=1000.0)
        return w.step(now=1001.0)

    def test_bad_file_quarantined_incident_and_alarm(self):
        w = self.watcher()
        mk(self.root, {U + "fyt.prop": "ro.x=1"})
        ids = self.settle(w)
        self.assertEqual(len(ids), 1)
        self.assertFalse(os.path.exists(self.path(U + "fyt.prop")))
        self.assertTrue(os.path.isfile(os.path.join(self.q, str(ids[0]), U + "fyt.prop")))
        inc = self.db.get_incident(ids[0])
        self.assertEqual((inc["rule"], inc["state"], inc["actor"]), ("R05", "open", "out-of-band"))
        self.assertEqual(len(self.notifier.messages), 1)
        v = check(self.root, "agent", "create", U + "access/adb/0002vadb.md", hdr("adb", 2), lock=self.db.open_ids)
        self.assertEqual(v.rule_id, "LOCK")
        # quarantine is not re-reported
        self.assertEqual(self.settle(w), [])

    def test_valid_file_indexed_not_quarantined(self):
        w = self.watcher()
        mk(self.root, {U + "access/adb/0002vadb.md": hdr("adb", 2)})
        self.assertEqual(self.settle(w), [])
        self.assertTrue(os.path.exists(self.path(U + "access/adb/0002vadb.md")))
        self.assertEqual(self.db.get_file(U + "access/adb/0002vadb.md")["status"], "numbered")
        self.assertEqual(self.db.open_ids(), [])

    def test_gap_version_quarantined(self):
        w = self.watcher()
        mk(self.root, {U + "access/adb/0004vadb.md": hdr("adb", 4)})
        ids = self.settle(w)
        self.assertEqual(self.db.get_incident(ids[0])["rule"], "R02")

    def test_bad_header_quarantined(self):
        w = self.watcher()
        mk(self.root, {U + "access/adb/0002vadb.md": "no header"})
        ids = self.settle(w)
        self.assertEqual(self.db.get_incident(ids[0])["rule"], "R09")

    def test_bad_dir_quarantined_whole(self):
        w = self.watcher()
        mk(self.root, {U + "Bad_Dir/a.txt": "x", U + "Bad_Dir/sub/b.txt": "y"})
        ids = self.settle(w)
        self.assertEqual(len(ids), 1)
        self.assertEqual(self.db.get_incident(ids[0])["rule"], "R07")
        self.assertFalse(os.path.exists(self.path(U + "Bad_Dir")))
        self.assertTrue(os.path.isfile(os.path.join(self.q, str(ids[0]), U + "Bad_Dir/sub/b.txt")))

    def test_file_in_branch_dir_quarantined(self):
        w = self.watcher()
        mk(self.root, {U + "access/0001vaccess.md": hdr("access", 1)})
        ids = self.settle(w)
        self.assertEqual(self.db.get_incident(ids[0])["rule"], "R04")

    def test_partial_write_waits_for_settle(self):
        w = self.watcher()
        mk(self.root, {U + "access/adb/0002vadb.md": ""})
        w.step(now=1000.0)
        with open(self.path(U + "access/adb/0002vadb.md"), "w") as f:     # content arrives before settle
            f.write(hdr("adb", 2))
        w.step(now=1000.1)
        self.assertEqual(w.step(now=1000.2), [])                           # signature changed: timer restarted
        self.assertEqual(w.step(now=1000.5), [])
        self.assertTrue(os.path.exists(self.path(U + "access/adb/0002vadb.md")))

    def test_modified_numbered_file_incident_no_data_loss(self):
        w = self.watcher()
        p = self.path(U + "access/adb/0001vadb.md")
        with open(p, "a") as f:
            f.write("sneaky edit\n")
        ids = self.settle(w)
        self.assertEqual(len(ids), 1)
        inc = self.db.get_incident(ids[0])
        self.assertEqual((inc["rule"], inc["op"]), ("R03", "modify"))
        self.assertTrue(os.path.exists(p))
        self.assertTrue(os.path.exists(os.path.join(self.q, str(ids[0]), U + "access/adb/0001vadb.md")))

    def test_deleted_numbered_and_raw(self):
        mk(self.root, {U + "raw/dumps/a.bin": "x"})
        self.lib.startup()
        w = self.watcher()
        os.remove(self.path(U + "access/adb/0001vadb.md"))
        os.remove(self.path(U + "raw/dumps/a.bin"))
        ids = self.settle(w)
        self.assertEqual(sorted(self.db.get_incident(i)["rule"] for i in ids), ["R03", "R06"])

    def test_approve_restores_and_is_not_requarantined(self):
        w = self.watcher()
        mk(self.root, {U + "fyt.prop": "ro.x=1"})
        (iid,) = self.settle(w)
        self.db.resolve(iid, "approve", self.root)
        self.assertTrue(os.path.exists(self.path(U + "fyt.prop")))
        self.assertEqual(self.db.open_ids(), [])
        self.assertEqual(self.settle(w), [])
        self.assertTrue(os.path.exists(self.path(U + "fyt.prop")))

    def test_approve_dir_covers_children(self):
        w = self.watcher()
        mk(self.root, {U + "Bad_Dir/a.txt": "x"})
        (iid,) = self.settle(w)
        self.db.resolve(iid, "approve", self.root)
        self.assertEqual(self.settle(w), [])
        self.assertTrue(os.path.exists(self.path(U + "Bad_Dir/a.txt")))

    def test_revert_keeps_quarantined(self):
        w = self.watcher()
        mk(self.root, {U + "fyt.prop": "ro.x=1"})
        (iid,) = self.settle(w)
        self.db.resolve(iid, "revert", self.root)
        self.assertFalse(os.path.exists(self.path(U + "fyt.prop")))
        self.assertEqual(self.db.open_ids(), [])

    def test_raw_creates_are_fine_and_raw_edit_is_incident(self):
        w = self.watcher()
        mk(self.root, {U + "raw/Vendor Dump.bin": "x"})
        self.assertEqual(self.settle(w), [])
        w2 = w
        with open(self.path(U + "raw/Vendor Dump.bin"), "w") as f:
            f.write("changed")
        w2.step(now=2000.0)
        ids = w2.step(now=2001.0)
        self.assertEqual(self.db.get_incident(ids[0])["rule"], "R06")

    def test_quarantine_inside_tree_is_ignored(self):
        cfg = Config()
        qrel = "infra/librarian/quarantine"
        lib = Librarian(self.root, self.db, config=cfg, notifier=self.notifier)
        mk(self.root, {"infra/librarian/SPEC.md": "x\nCurrent: a\nNext: none\n", "infra/librarian/MEMORY.md": "m"})
        lib.startup()
        w = Watcher(lib, os.path.join(self.root, qrel), settle=0.2, use_inotify=False)
        self.addCleanup(w.stop)
        mk(self.root, {U + "fyt.prop": "x"})
        w.step(now=1.0)
        (iid,) = w.step(now=2.0)
        self.assertTrue(os.path.isfile(os.path.join(self.root, qrel, str(iid), U + "fyt.prop")))
        self.assertEqual(w.step(now=3.0), [])
        self.assertEqual(w.step(now=4.0), [])
        self.assertEqual(self.db.open_ids(), [iid])


class TimedMixin:
    def test_bad_file_quarantined_and_alarmed_within_3s(self):
        w = self.watcher(settle=0.3).start()
        time.sleep(0.2)
        t0 = time.time()
        mk(self.root, {U + "fyt.prop": "ro.x=1"})
        took = wait_for(lambda: self.notifier.messages and not os.path.exists(self.path(U + "fyt.prop")), 3.0)
        self.assertIsNotNone(took, "no quarantine+alarm within 3s (mode %s)" % w.mode)
        self.assertLess(time.time() - t0, 3.0)
        (iid,) = self.db.open_ids()
        self.assertTrue(os.path.isfile(os.path.join(self.q, str(iid), U + "fyt.prop")))
        self.assertIn("librarian resolve %d approve|revert" % iid, self.notifier.messages[0][1])

    def test_valid_write_not_touched_in_thread(self):
        w = self.watcher(settle=0.3).start()
        time.sleep(0.2)
        mk(self.root, {U + "access/adb/0002vadb.md": hdr("adb", 2)})
        time.sleep(1.5)
        self.assertTrue(os.path.exists(self.path(U + "access/adb/0002vadb.md")))
        self.assertEqual(self.db.open_ids(), [])


class TestPollMode(TimedMixin, WatchBase):
    use_inotify = False


class TestAutoMode(TimedMixin, WatchBase):
    use_inotify = None

    def test_mode_reported(self):
        w = self.watcher()
        self.assertIn(w.mode, ("inotify", "poll"))


if __name__ == "__main__":
    unittest.main()
