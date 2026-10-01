#!/usr/bin/env python3
"""LB3 rehearsal: build a replica of the babalou2 ~ tree (empty files) from fixtures/babalou2-home-tree.txt,
scan it, write reports/lb3-rehearsal.txt. The replica lives in a temp dir and is removed afterwards."""
import os
import shutil
import sys
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, REPO)

from librarian import migration, treeparse  # noqa: E402
from librarian.scan import scan  # noqa: E402


def main():
    text = open(os.path.join(REPO, "fixtures", "babalou2-home-tree.txt"), errors="replace").read()
    dest = tempfile.mkdtemp(prefix="lb3-replica-")
    try:
        nd, nf, summary, _ = treeparse.build_replica(text, dest)
        vs = scan(dest)
        out = os.path.join(REPO, "reports", "lb3-rehearsal.txt")
        total = migration.write_report(dest, vs, out, summary, nd, nf)
    finally:
        shutil.rmtree(dest, True)
    print("replica %d dirs + %d files; %d violations; report %s" % (nd, nf, total, out))


if __name__ == "__main__":
    main()
