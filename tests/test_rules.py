import os
import unittest

from librarian.rules import check, Verdict
from tests.helpers import TreeCase, hdr, mk, SPEC_OK

U = "projects/demo/"


class Base(TreeCase):
    def setUp(self):
        super().setUp()
        self.build()

    def ok(self, op, path, content=None, actor="agent", dest=None):
        v = check(self.root, actor, op, path, content, dest=dest)
        self.assertIsInstance(v, Verdict)
        self.assertTrue(v.allow, "expected allow, got %s %s" % (v.rule_id, v.reason))
        return v

    def no(self, rule, op, path, content=None, actor="agent", dest=None):
        v = check(self.root, actor, op, path, content, dest=dest)
        self.assertFalse(v.allow, "expected deny %s for %s" % (rule, path))
        self.assertEqual(v.rule_id, rule, v.reason)
        self.assertTrue(v.reason)
        return v


class TestR00(Base):
    def test_bad_op_denied(self):
        self.no("R00", "chmod", U + "SPEC.md")

    def test_outside_root_denied(self):
        self.no("R00", "create", "/etc/passwd")
        self.no("R00", "create", "../x/0001vx.md")

    def test_edit_missing_denied(self):
        self.no("R00", "edit", U + "access/adb/0009vadb.md")

    def test_move_without_dest_denied(self):
        self.no("R00", "move", U + "access/adb/0001vadb.md", actor="librarian")

    def test_absolute_path_inside_root_ok(self):
        self.ok("create", os.path.join(self.root, U, "access/adb/0002vadb.md"), hdr("adb", 2))

    def test_ignored_git_dir_ok(self):
        self.ok("create", ".git/config", "x")


class TestR01(Base):
    def test_valid_names_allowed(self):
        self.ok("create", U + "access/adb/0002vadb.md", hdr("adb", 2))
        self.ok("create", U + "scripts/diagnostic/0001vdiagnostic.py", "print(1)")
        self.ok("create", U + "scripts/adb-connect/0001vadb-connect.sh", "#!/bin/sh")

    def test_wrong_slug_denied(self):
        v = self.no("R01", "create", U + "access/adb/0002vserial.md", hdr("serial", 2))
        self.assertEqual(v.suggested_name, "0002vadb.md")

    def test_uppercase_v_denied(self):
        self.no("R01", "create", U + "access/adb/0002Vadb.md", hdr("adb", 2))

    def test_underscore_denied(self):
        self.no("R01", "create", U + "access/adb/0002_adb.md")

    def test_unnumbered_denied(self):
        v = self.no("R01", "create", U + "access/adb/notes.txt")
        self.assertEqual(v.suggested_name, "0002vadb.txt")


class TestR02(Base):
    def test_next_allowed(self):
        self.ok("create", U + "access/adb/0002vadb.md", hdr("adb", 2))
        self.ok("create", U + "hardware/0002vhardware.md", hdr("hardware", 2))

    def test_gap_denied(self):
        v = self.no("R02", "create", U + "access/adb/0004vadb.md", hdr("adb", 4))
        self.assertEqual(v.suggested_name, "0002vadb.md")

    def test_zero_denied(self):
        self.no("R02", "create", U + "access/serial/0000vserial.md", hdr("serial", 0))

    def test_first_must_be_one(self):
        self.no("R02", "create", U + "access/serial/0002vserial.md", hdr("serial", 2))
        self.ok("create", U + "access/serial/0001vserial.md", hdr("serial", 1))


class TestR03(Base):
    def test_librarian_move_to_legacy_allowed(self):
        self.ok("move", U + "access/adb/0001vadb.md", actor="librarian",
                dest=U + "raw/legacy-2026-09-30/0001vadb.md")
        self.ok("move", U + "access/adb/0001vadb.md", actor="librarian",
                dest=U + "raw/legacy-2026-09-30/sub/0001vadb.md")

    def test_edit_write_denied(self):
        v = self.no("R03", "edit", U + "access/adb/0001vadb.md")
        self.no("R03", "write", U + "access/adb/0001vadb.md", hdr("adb", 1))
        self.no("R03", "create", U + "access/adb/0001vadb.md", hdr("adb", 1))
        self.assertEqual(v.suggested_name, "0002vadb.md")

    def test_delete_denied(self):
        self.no("R03", "delete", U + "access/adb/0001vadb.md")
        self.no("R03", "delete", U + "access/adb")  # dir containing numbered files

    def test_move_by_other_actor_denied(self):
        self.no("R03", "move", U + "access/adb/0001vadb.md", dest=U + "raw/legacy-2026-09-30/0001vadb.md")

    def test_rename_and_wrong_dest_denied(self):
        self.no("R03", "move", U + "access/adb/0001vadb.md", actor="librarian",
                dest=U + "raw/legacy-2026-09-30/0002vadb.md")
        self.no("R03", "move", U + "access/adb/0001vadb.md", actor="librarian", dest=U + "raw/dumps/0001vadb.md")
        self.no("R03", "move", U + "access/adb/0001vadb.md", actor="librarian", dest=U + "access/serial/0001vserial.md")

    def test_legacy_uppercase_is_immutable(self):
        mk(self.root, {U + "old/0003Vfoo.txt": "x"})
        self.no("R03", "edit", U + "old/0003Vfoo.txt")
        self.ok("move", U + "old/0003Vfoo.txt", actor="librarian", dest=U + "raw/legacy-2026-09-30/0003Vfoo.txt")


class TestR04(Base):
    def test_dirs_in_branch_allowed(self):
        self.ok("create", U + "access/usb/", None)
        self.ok("create", U + "scripts/")

    def test_leaf_dir_in_branch_with_file_ok(self):
        self.ok("create", U + "access/usb/0001vusb.md", hdr("usb", 1))

    def test_file_in_branch_denied(self):
        self.no("R04", "create", U + "access/0001vaccess.md", hdr("access", 1))

    def test_allowlisted_in_branch_denied(self):
        self.no("R04", "create", U + "access/config.yaml", "x")

    def test_subdir_in_leaf_denied(self):
        self.no("R04", "create", U + "access/adb/sub/")
        self.no("R04", "create", U + "access/adb/sub/0001vsub.md", hdr("sub", 1))


class TestR05(Base):
    def test_unit_files_allowed(self):
        self.ok("write", U + "SPEC.md", SPEC_OK)
        self.ok("create", U + "RULES.md", "rules")
        self.ok("create", U + ".gitignore", "x")
        self.ok("write", U + "MEMORY.md", "m")

    def test_leaf_allowlist_allowed(self):
        self.ok("create", U + "access/serial/config.yaml", "x")

    def test_stray_unit_file_denied(self):
        self.no("R05", "create", U + "fyt.prop", "x")
        self.no("R05", "create", U + "0001vdemo.md", hdr("demo", 1))

    def test_incomplete_unit_denied_but_bootstrap_ok(self):
        mk(self.root, {"projects/new/": None})
        self.no("R05", "create", "projects/new/hardware/0001vhardware.md", hdr("hardware", 1))
        self.ok("create", "projects/new/SPEC.md", SPEC_OK)
        self.ok("create", "projects/new/MEMORY.md", "m")

    def test_cannot_delete_unit_spec(self):
        self.no("R05", "delete", U + "SPEC.md")
        self.no("R05", "delete", U + "MEMORY.md")

    def test_group_and_root_files(self):
        self.ok("create", "CLAUDE.md", "x")
        self.ok("create", "projects/SPEC.md", "x")
        self.ok("create", "projects/RULES.md", "x")
        self.no("R05", "create", "fyt.prop", "x")
        self.no("R05", "create", "projects/0000vGATES.md", "x")
        self.no("R05", "create", "projects/MEMORY.md", "x")


class TestR06(Base):
    def test_raw_create_anything(self):
        self.ok("create", U + "raw/firmware/Vendor File.v1.bin", "x")
        self.ok("create", U + "raw/dumps/diagnostic_20260923/adb_devices.txt", "x")
        self.ok("create", U + "raw/legacy-2026-09-30/0000Vfoo.txt", "x")

    def test_raw_existing_immutable(self):
        mk(self.root, {U + "raw/dumps/a.bin": "x"})
        self.no("R06", "edit", U + "raw/dumps/a.bin")
        self.no("R06", "write", U + "raw/dumps/a.bin", "y")
        self.no("R06", "create", U + "raw/dumps/a.bin", "y")
        self.no("R06", "delete", U + "raw/dumps/a.bin")

    def test_raw_move_out_denied(self):
        mk(self.root, {U + "raw/dumps/a.bin": "x"})
        self.no("R06", "move", U + "raw/dumps/a.bin", dest=U + "raw/dumps/b.bin")
        self.no("R06", "move", U + "raw/dumps/a.bin", actor="librarian", dest=U + "hardware/a.bin")

    def test_raw_only_at_unit_root(self):
        self.no("R06", "create", U + "access/raw/")
        self.no("R06", "create", U + "access/raw/x.bin", "x")

    def test_move_unnumbered_into_raw_ok(self):
        mk(self.root, {U + "hardware/STATUS_X.txt": "x"})
        # not allowed here: hardware is a leaf whose name rules apply to existing? the move out of it is ok
        self.ok("move", U + "hardware/STATUS_X.txt", dest=U + "raw/legacy-2026-09-30/STATUS_X.txt")


class TestR07(Base):
    def test_good_names(self):
        self.ok("create", U + "my-dir2/")
        self.ok("create", U + "a/")

    def test_bad_names_denied(self):
        for n in ("Upper", "under_score", "0001vfoo", "dbl--hyphen", "trail-", "sp ace"):
            self.no("R07", "create", U + n + "/")

    def test_bad_unit_name(self):
        self.no("R07", "create", "projects/Fyt_7862/")
        self.ok("create", "projects/fyt-7862/")

    def test_bad_name_in_file_path(self):
        self.no("R07", "create", U + "Bad_Dir/0001vbad_dir.md", hdr("bad_dir", 1))


class TestR08(Base):
    def test_depth_3_allowed(self):
        self.ok("create", U + "a/b/c/0001vc.md", hdr("c", 1))
        self.ok("create", U + "a/b/c/")

    def test_depth_4_denied(self):
        self.no("R08", "create", U + "a/b/c/d/")
        self.no("R08", "create", U + "a/b/c/d/0001vd.md", hdr("d", 1))

    def test_raw_excluded(self):
        self.ok("create", U + "raw/a/b/c/d/e/f.txt", "x")


class TestR09(Base):
    def p(self):
        return U + "access/adb/0002vadb.md"

    def test_valid(self):
        for st in ("open", "blocked", "solved"):
            self.ok("create", self.p(), hdr("adb", 2, status=st))
        self.ok("create", U + "access/serial/0001vserial.md", hdr("serial", 1))

    def test_missing_header(self):
        self.no("R09", "create", self.p(), "just text\n")

    def test_bad_status(self):
        self.no("R09", "create", self.p(), hdr("adb", 2, status="done"))

    def test_wrong_title(self):
        self.no("R09", "create", self.p(), hdr("serial", 2))
        self.no("R09", "create", self.p(), hdr("adb", 3))

    def test_bad_supersedes(self):
        self.no("R09", "create", self.p(), hdr("adb", 2, sup="none"))
        self.no("R09", "create", self.p(), hdr("adb", 2, sup="0005v"))
        self.no("R09", "create", U + "access/serial/0001vserial.md", hdr("serial", 1, sup="0000v"))

    def test_missing_summary_or_sep(self):
        self.no("R09", "create", self.p(), "# adb v0002\nStatus: open\nSupersedes: 0001v\nSummary:\n---\n")
        self.no("R09", "create", self.p(), "# adb v0002\nStatus: open\nSupersedes: 0001v\nSummary: x\nbody\n")

    def test_non_md_no_header_needed(self):
        self.ok("create", U + "scripts/tool/0001vtool.sh", "echo")

    def test_content_none_skips_header(self):
        self.ok("create", self.p(), None)


class TestR10(Base):
    def test_good_tail(self):
        self.ok("write", U + "SPEC.md", "# s\nCurrent: a\nNext: none\n\n")
        self.ok("edit", U + "SPEC.md", "# s\nCurrent: a/b/0001vb.md\nNext: do x")

    def test_bad_tail(self):
        self.no("R10", "write", U + "SPEC.md", "# s\nNext: none\nCurrent: a\n")
        self.no("R10", "write", U + "SPEC.md", "# s\njust text\n")
        self.no("R10", "edit", U + "SPEC.md", "")


class TestR11(Base):
    def test_group_dir_allowed(self):
        self.ok("create", "projects/", None)
        self.ok("create", "infra/")

    def test_unknown_top_dir_denied(self):
        self.no("R11", "create", "stuff/")
        self.no("R11", "create", "architeture/foo/0001vfoo.md", hdr("foo", 1))


class TestDelete(Base):
    def test_delete_ordinary_ok(self):
        mk(self.root, {U + "scripts/tool/config.yaml": "x"})
        self.ok("delete", U + "scripts/tool/config.yaml")
        self.ok("delete", U + "access/serial")

    def test_delete_raw_dir_denied(self):
        mk(self.root, {U + "raw/a.bin": "x"})
        self.no("R06", "delete", U + "raw")


if __name__ == "__main__":
    unittest.main()
