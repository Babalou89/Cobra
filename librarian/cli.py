"""CLI: scan | serve | resolve | incidents | map | watch.  `python3 -m librarian.cli <cmd> ...`"""
import argparse
import os
import sys

from . import mapgen
from .db import DB
from .scan import reconcile, scan
from .server import Librarian, make_server


def _db(a):
    return DB(a.db)


def cmd_scan(a):
    db = _db(a)
    vs, new = reconcile(a.root, db, open_incidents=a.open_incidents)
    for v in vs:
        extra = " [+%d files beneath]" % v.beneath if v.beneath else ""
        print("%s\t%s\t%s%s" % (v.rule, v.path, v.detail, extra))
    print("%d violation(s), %d new incident(s)" % (len(vs), len(new)))
    return 1 if vs else 0


def cmd_serve(a):
    db = _db(a)
    lib = Librarian(a.root, db)
    vs, new = lib.startup(open_incidents=False)
    servers = [make_server(lib, h, a.port) for h in (a.host or ["127.0.0.1"])]
    import threading
    for s in servers[1:]:
        threading.Thread(target=s.serve_forever, daemon=True).start()
    print("librarian serving %s on %s (startup: %d violations, %d incidents)" % (
        a.root, ", ".join("%s:%d" % s.server_address[:2] for s in servers), len(vs), len(new)), flush=True)
    try:
        servers[0].serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


def cmd_resolve(a):
    db = _db(a)
    try:
        print(db.resolve(a.id, a.resolution, a.root))
    except (KeyError, ValueError) as e:
        print("error: %s" % e, file=sys.stderr)
        return 1
    left = db.open_ids()
    print("open incidents left: %s" % (left or "none (lock released)"))
    return 0


def cmd_incidents(a):
    for r in _db(a).incidents("open" if a.open else None):
        print("%s\t%s\t%s\t%s\t%s\t%s" % (r["id"], r["state"], r["rule"], r["actor"], r["path"], r["detail"]))
    return 0


def cmd_map(a):
    if a.write:
        sys.stdout.write(mapgen.update_spec(a.root, a.unit))
    else:
        sys.stdout.write(mapgen.build_map(a.root, a.unit))
    return 0


def cmd_watch(a):
    from .notify import LogNotifier
    from .watcher import Watcher
    db = _db(a)
    lib = Librarian(a.root, db, notifier=LogNotifier(a.alarm_log))
    lib.startup()
    w = Watcher(lib, a.quarantine)
    print("watching %s" % a.root, flush=True)
    try:
        w.run_forever()
    except KeyboardInterrupt:
        w.stop()
    return 0


def main(argv=None):
    p = argparse.ArgumentParser(prog="librarian")
    p.add_argument("--db", default="state/librarian.db")
    sub = p.add_subparsers(dest="cmd", required=True)
    s = sub.add_parser("scan"); s.add_argument("root"); s.add_argument("--open-incidents", action="store_true"); s.set_defaults(f=cmd_scan)
    s = sub.add_parser("serve"); s.add_argument("root"); s.add_argument("--host", action="append"); s.add_argument("--port", type=int, default=8765); s.set_defaults(f=cmd_serve)
    s = sub.add_parser("resolve"); s.add_argument("id", type=int); s.add_argument("resolution", choices=["approve", "revert"]); s.add_argument("--root"); s.set_defaults(f=cmd_resolve)
    s = sub.add_parser("incidents"); s.add_argument("--open", action="store_true"); s.set_defaults(f=cmd_incidents)
    s = sub.add_parser("map"); s.add_argument("root"); s.add_argument("unit"); s.add_argument("--write", action="store_true"); s.set_defaults(f=cmd_map)
    s = sub.add_parser("watch"); s.add_argument("root"); s.add_argument("--quarantine", default="state/quarantine"); s.add_argument("--alarm-log", default="state/alarms.log"); s.set_defaults(f=cmd_watch)
    a = p.parse_args(argv)
    # allow --db after the subcommand too
    return a.f(a)


if __name__ == "__main__":
    # accept `--db` anywhere: argparse subparsers do not, so hoist it
    argv = sys.argv[1:]
    if "--db" in argv:
        i = argv.index("--db")
        argv = argv[i:i + 2] + argv[:i] + argv[i + 2:]
    sys.exit(main(argv))
