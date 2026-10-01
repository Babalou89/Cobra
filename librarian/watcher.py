"""Watcher: detects out-of-band changes (root/Billy/bash writes that bypass the clients), quarantines violating
new files/dirs, opens incidents (=> global LOCK) and alarms. inotify (ctypes) is only a wake-up source; the
decision logic is a snapshot diff, so poll mode (1s) and inotify mode behave identically."""
import ctypes
import ctypes.util
import os
import select
import shutil
import struct
import threading
import time

from . import rules as R
from .scan import file_record

IN_MODIFY, IN_ATTRIB, IN_CLOSE_WRITE = 0x2, 0x4, 0x8
IN_MOVED_FROM, IN_MOVED_TO, IN_CREATE, IN_DELETE = 0x40, 0x80, 0x100, 0x200
MASK = IN_MODIFY | IN_CLOSE_WRITE | IN_MOVED_FROM | IN_MOVED_TO | IN_CREATE | IN_DELETE
IN_NONBLOCK, IN_CLOEXEC = 0o4000, 0o2000000


class _Inotify:
    def __init__(self):
        libc = ctypes.CDLL(ctypes.util.find_library("c"), use_errno=True)
        self._add = libc.inotify_add_watch
        self._add.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_uint32]
        fd = libc.inotify_init1(IN_NONBLOCK | IN_CLOEXEC)
        if fd < 0:
            raise OSError(ctypes.get_errno(), "inotify_init1 failed")
        self.fd = fd
        self.watched = set()

    def watch(self, path):
        if path in self.watched:
            return
        if self._add(self.fd, os.fsencode(path), MASK) >= 0:
            self.watched.add(path)

    def wait(self, timeout):
        r, _, _ = select.select([self.fd], [], [], timeout)
        if r:
            try:
                while os.read(self.fd, 65536):
                    pass
            except BlockingIOError:
                pass
            return True
        return False

    def close(self):
        try:
            os.close(self.fd)
        except OSError:
            pass


class Watcher:
    def __init__(self, lib, quarantine_dir, settle=0.3, poll=1.0, use_inotify=None):
        self.lib = lib
        self.root = os.path.normpath(os.path.abspath(lib.root))
        self.q = os.path.abspath(quarantine_dir)
        self.cfg = lib.cfg
        self.settle = settle
        self.poll = poll
        self.pending = {}
        self.known = self.snapshot()
        self._stop = threading.Event()
        self._thread = None
        self.ino = None
        if use_inotify is not False:
            try:
                self.ino = _Inotify()
            except Exception:
                if use_inotify:
                    raise
                self.ino = None
        self.mode = "inotify" if self.ino else "poll"
        self._sync_watches()

    # ---------------------------------------------------------------- snapshot
    def snapshot(self):
        snap = {}
        for dirpath, dirnames, filenames in os.walk(self.root):
            keep = []
            for d in sorted(dirnames):
                rel = os.path.relpath(os.path.join(dirpath, d), self.root).replace(os.sep, "/")
                if d in self.cfg.ignore or R.is_ignored(self.cfg, rel.split("/")):
                    continue
                if os.path.islink(os.path.join(dirpath, d)):
                    continue
                keep.append(d)
                snap[rel] = ("d",)
            dirnames[:] = keep
            for fn in filenames:
                rel = os.path.relpath(os.path.join(dirpath, fn), self.root).replace(os.sep, "/")
                if R.is_ignored(self.cfg, rel.split("/")):
                    continue
                try:
                    st = os.lstat(os.path.join(dirpath, fn))
                except OSError:
                    continue
                snap[rel] = ("f", st.st_size, st.st_mtime_ns)
        return snap

    def _sync_watches(self):
        if not self.ino:
            return
        self.ino.watch(self.root)
        for rel, sig in self.known.items():
            if sig[0] == "d":
                self.ino.watch(os.path.join(self.root, rel))

    # ---------------------------------------------------------------- core
    def step(self, now=None):
        """One reconciliation pass. Returns list of incident ids opened."""
        now = time.time() if now is None else now
        cur = self.snapshot()
        for p in set(cur) | set(self.known):
            sc, sk = cur.get(p), self.known.get(p)
            if sc == sk:
                self.pending.pop(p, None)
            elif p not in self.pending or self.pending[p][0] != sc:
                self.pending[p] = (sc, now)
        ready = [p for p, (sig, since) in self.pending.items() if now - since >= self.settle]
        opened = []
        # dirs before files, shallow before deep, so a bad dir is quarantined whole
        for p in sorted(ready, key=lambda x: (x.count("/"), 0 if (cur.get(x) or ("f",))[0] == "d" else 1, x)):
            if p not in self.pending:
                continue  # swallowed by a quarantined ancestor
            sig = self.pending.pop(p)[0]
            ids = self._handle(p, sig)
            opened.extend(ids)
            if sig is None or not os.path.lexists(os.path.join(self.root, *p.split("/"))):
                self.known.pop(p, None)
            else:
                self.known[p] = sig
        self._sync_watches()
        return opened

    def _excepted(self, rel, sha):
        ex = self.lib.db.exceptions()
        for e, esha in ex.items():
            if rel == e and (esha == "" or esha == sha):
                return True
            if esha == "" and rel.startswith(e + "/"):
                return True
        return False

    def _incident(self, op, rel, rule, detail, quarantined=None):
        iid = self.lib.db.open_incident("out-of-band", op, rel, rule, detail)
        if quarantined:
            self.lib.db.set_quarantine(iid, quarantined)
        inc = self.lib.db.get_incident(iid)
        if self.lib.notifier:
            self.lib.notifier.notify(inc)
        return iid

    def _quarantine(self, rel, isdir, op, rule, detail):
        iid = self.lib.db.open_incident("out-of-band", op, rel, rule, detail)
        dest = os.path.join(self.q, str(iid), *rel.split("/"))
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        shutil.move(os.path.join(self.root, *rel.split("/")), dest)
        self.lib.db.set_quarantine(iid, dest)
        # forget the quarantined subtree
        for p in [p for p in list(self.known) + list(self.pending) if p == rel or p.startswith(rel + "/")]:
            self.known.pop(p, None)
            self.pending.pop(p, None)
        for f in self.lib.db.files():
            if f["path"] == rel or f["path"].startswith(rel + "/"):
                self.lib.db.delete_file(f["path"])
        if self.lib.notifier:
            self.lib.notifier.notify(self.lib.db.get_incident(iid))
        return iid

    def _evidence_copy(self, rel, iid):
        src = os.path.join(self.root, *rel.split("/"))
        dest = os.path.join(self.q, str(iid), *rel.split("/"))
        try:
            os.makedirs(os.path.dirname(dest), exist_ok=True)
            shutil.copy2(src, dest)
        except OSError:
            pass

    def _handle(self, rel, sig):
        parts = rel.split("/")
        db = self.lib.db
        if sig is None:                                    # deleted
            old = db.get_file(rel)
            if not old:
                return []
            rule = None
            if old["status"] == "numbered":
                rule = "R03"
            elif old["status"] == "raw":
                rule = "R06"
            elif len(parts) == 3 and parts[2] in ("SPEC.md", "MEMORY.md"):
                rule = "R05"
            if rule and not self._excepted(rel, ""):
                iid = self._incident("delete", rel, rule, "tracked file deleted out-of-band")
                return [iid]
            db.delete_file(rel)
            return []
        isdir = sig[0] == "d"
        absp = os.path.join(self.root, *parts)
        if isdir:
            if self._excepted(rel, ""):
                return []
            v = R.validate_existing(self.cfg, self.root, parts, True)
            if not v.allow:
                return [self._quarantine(rel, True, "create", v.rule_id, v.reason)]
            return []
        rec = file_record(self.root, rel, self.cfg, db.get_file(rel))
        old = db.get_file(rel)
        if old and old["status"] in ("numbered", "raw") and old["sha256"] and old["sha256"] != rec["sha256"]:
            if self._excepted(rel, rec["sha256"]):
                db.upsert_file(rec)
                return []
            iid = self._incident("modify", rel, "R06" if old["status"] == "raw" else "R03",
                                 "immutable file modified out-of-band (evidence copy in quarantine, file left in place)")
            self._evidence_copy(rel, iid)
            return [iid]
        if old and old["sha256"] == rec["sha256"]:
            return []
        if self._excepted(rel, rec["sha256"]):
            db.upsert_file(rec)
            return []
        v = R.validate_existing(self.cfg, self.root, parts, False)
        if v.allow:
            db.upsert_file(rec)
            return []
        if old:                                              # tracked file edited into an invalid state: keep it, alarm
            iid = self._incident("modify", rel, v.rule_id, v.reason + " (evidence copy in quarantine)")
            self._evidence_copy(rel, iid)
            return [iid]
        return [self._quarantine(rel, False, "create", v.rule_id, v.reason)]

    # ---------------------------------------------------------------- loop
    def run_forever(self):
        while not self._stop.is_set():
            self.step()
            timeout = min(self.poll, self.settle / 2.0) if self.pending else self.poll
            if self.ino:
                self.ino.wait(timeout)
            else:
                self._stop.wait(timeout)

    def start(self):
        self._thread = threading.Thread(target=self.run_forever, daemon=True)
        self._thread.start()
        return self

    def stop(self):
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=3)
        if self.ino:
            self.ino.close()
