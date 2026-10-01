"""SQLite image of the managed tree (files) and the incident log (incidents)."""
import json
import os
import shutil
import sqlite3
import threading
import time

SCHEMA = """
CREATE TABLE IF NOT EXISTS files(
  path TEXT PRIMARY KEY, unit TEXT, leaf TEXT, version INTEGER, slug TEXT, ext TEXT,
  sha256 TEXT, size INTEGER, mtime REAL, status TEXT, header_status TEXT);
CREATE TABLE IF NOT EXISTS incidents(
  id INTEGER PRIMARY KEY AUTOINCREMENT, ts REAL, actor TEXT, op TEXT, path TEXT, rule TEXT, detail TEXT,
  state TEXT DEFAULT 'open', resolution TEXT, quarantined_to TEXT);
CREATE TABLE IF NOT EXISTS exceptions(path TEXT PRIMARY KEY, sha256 TEXT, incident_id INTEGER);
CREATE TABLE IF NOT EXISTS bypass(id INTEGER PRIMARY KEY AUTOINCREMENT, ts REAL, actor TEXT, detail TEXT);
"""

FILE_COLS = ("path", "unit", "leaf", "version", "slug", "ext", "sha256", "size", "mtime", "status", "header_status")


class DB:
    def __init__(self, path=":memory:"):
        if path != ":memory:":
            os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
        self.path = path
        self._lock = threading.RLock()
        self.conn = sqlite3.connect(path, check_same_thread=False)
        self.conn.row_factory = sqlite3.Row
        with self._lock:
            self.conn.executescript(SCHEMA)
            self.conn.commit()

    def close(self):
        with self._lock:
            self.conn.close()

    def _x(self, sql, args=()):
        with self._lock:
            cur = self.conn.execute(sql, args)
            self.conn.commit()
            return cur

    def _q(self, sql, args=()):
        with self._lock:
            return [dict(r) for r in self.conn.execute(sql, args).fetchall()]

    # ---- files image
    def upsert_file(self, rec):
        vals = [rec.get(c) for c in FILE_COLS]
        self._x("INSERT OR REPLACE INTO files(%s) VALUES (%s)" % (",".join(FILE_COLS), ",".join("?" * len(FILE_COLS))), vals)

    def replace_files(self, recs):
        with self._lock:
            self.conn.execute("DELETE FROM files")
            for rec in recs:
                self.conn.execute("INSERT OR REPLACE INTO files(%s) VALUES (%s)" % (",".join(FILE_COLS), ",".join("?" * len(FILE_COLS))),
                                  [rec.get(c) for c in FILE_COLS])
            self.conn.commit()

    def get_file(self, path):
        r = self._q("SELECT * FROM files WHERE path=?", (path,))
        return r[0] if r else None

    def delete_file(self, path):
        self._x("DELETE FROM files WHERE path=?", (path,))

    def files(self):
        return self._q("SELECT * FROM files ORDER BY path")

    # ---- incidents
    def open_incident(self, actor, op, path, rule, detail, quarantined_to=None):
        cur = self._x("INSERT INTO incidents(ts,actor,op,path,rule,detail,state,quarantined_to) VALUES (?,?,?,?,?,?,'open',?)",
                      (time.time(), actor, op, path, rule, detail, quarantined_to))
        return cur.lastrowid

    def incidents(self, state=None):
        if state:
            return self._q("SELECT * FROM incidents WHERE state=? ORDER BY id", (state,))
        return self._q("SELECT * FROM incidents ORDER BY id")

    def get_incident(self, iid):
        r = self._q("SELECT * FROM incidents WHERE id=?", (iid,))
        return r[0] if r else None

    def open_ids(self):
        return [r["id"] for r in self._q("SELECT id FROM incidents WHERE state='open' ORDER BY id")]

    def has_open(self, path, rule):
        return bool(self._q("SELECT 1 FROM incidents WHERE state='open' AND path=? AND rule=?", (path, rule)))

    def exceptions(self):
        return {r["path"]: r["sha256"] for r in self._q("SELECT * FROM exceptions")}

    def add_exception(self, path, sha256, incident_id):
        self._x("INSERT OR REPLACE INTO exceptions(path,sha256,incident_id) VALUES (?,?,?)", (path, sha256, incident_id))

    def resolve(self, iid, resolution, root=None):
        """approve: Billy accepts the state (quarantined file is restored, path becomes an exception).
        revert: the violating state stays rejected (quarantined file stays in quarantine; modified/deleted
        numbered or raw files must be restored from git/backup: the librarian keeps hashes, not content).
        Returns a short human string."""
        if resolution not in ("approve", "revert"):
            raise ValueError("resolution must be approve|revert")
        inc = self.get_incident(iid)
        if inc is None:
            raise KeyError("no incident %s" % iid)
        if inc["state"] != "open":
            raise ValueError("incident %s already resolved (%s)" % (iid, inc["resolution"]))
        msg = ""
        if resolution == "approve":
            from .scan import sha256_file
            dest = os.path.join(root, inc["path"]) if root else None
            q = inc["quarantined_to"]
            if q and os.path.exists(q) and dest:
                if os.path.exists(dest):
                    raise ValueError("cannot restore: %s exists" % inc["path"])
                os.makedirs(os.path.dirname(dest), exist_ok=True)
                shutil.move(q, dest)
                msg = "restored %s" % inc["path"]
            sha = ""
            if dest and os.path.isfile(dest):
                sha = sha256_file(dest)
            if inc["op"] == "delete":
                self.delete_file(inc["path"])
            self.add_exception(inc["path"], sha, iid)
        self._x("UPDATE incidents SET state='resolved', resolution=? WHERE id=?", (resolution, iid))
        return msg or ("%s %s" % (resolution, iid))

    # ---- bypass log
    def log_bypass(self, actor, detail):
        self._x("INSERT INTO bypass(ts,actor,detail) VALUES (?,?,?)", (time.time(), actor, detail))

    def bypasses(self):
        return self._q("SELECT * FROM bypass ORDER BY id")
