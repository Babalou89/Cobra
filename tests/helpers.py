import os
import shutil
import tempfile
import unittest


def hdr(slug, ver, status="open", sup=None, summary="s"):
    if sup is None:
        sup = "none" if ver == 1 else "%04dv" % (ver - 1)
    return "# %s v%04d\nStatus: %s\nSupersedes: %s\nSummary: %s\n---\nbody\n" % (slug, ver, status, sup, summary)


SPEC_OK = "# x\n\n## Map\n\nCurrent: access/adb/0001vadb.md\nNext: none\n"


def mk(root, tree):
    """tree: dict relpath -> content (str) ; a path ending '/' is an empty dir."""
    for rel, content in tree.items():
        p = os.path.join(root, rel)
        if rel.endswith("/"):
            os.makedirs(p, exist_ok=True)
        else:
            os.makedirs(os.path.dirname(p), exist_ok=True)
            with open(p, "w") as f:
                f.write(content if content is not None else "")


def unit_tree(unit="projects/demo"):
    """A small valid unit."""
    return {
        unit + "/SPEC.md": SPEC_OK,
        unit + "/MEMORY.md": "2026-09-30 start\n",
        unit + "/hardware/0001vhardware.md": hdr("hardware", 1),
        unit + "/access/adb/0001vadb.md": hdr("adb", 1),
        unit + "/access/serial/": None,
        unit + "/raw/": None,
    }


class TreeCase(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp(prefix="libtest-")
        self.addCleanup(shutil.rmtree, self.root, True)

    def build(self, tree=None):
        mk(self.root, unit_tree() if tree is None else tree)
        return self.root
