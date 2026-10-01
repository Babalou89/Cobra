"""Reconciliation scan: walk a managed tree, report every contract violation, keep the DB image current.

scan() is pure (reads disk + optional DB image/exceptions). reconcile() = scan + image sync + optional incidents.
"""
import hashlib
import os
from dataclasses import dataclass, field

from . import rules as R

MAX_HASH_BYTES = 512 * 1024 * 1024


@dataclass
class Violation:
    path: str
    rule: str
    detail: str
    kind: str = "file"          # file | dir | tamper
    suggested: object = None
    beneath: int = 0            # files under a dir that were counted but not examined


def sha256_file(p, max_bytes=MAX_HASH_BYTES):
    try:
        size = os.path.getsize(p)
        if size > max_bytes:
            return "big:%d" % size
        h = hashlib.sha256()
        with open(p, "rb") as f:
            for chunk in iter(lambda: f.read(1 << 20), b""):
                h.update(chunk)
        return h.hexdigest()
    except OSError:
        return ""


def _entries(p, cfg, root=None):
    try:
        names = sorted(os.listdir(p))
    except OSError:
        return [], []
    files, dirs = [], []
    for n in names:
        if n in cfg.ignore:
            continue
        full = os.path.join(p, n)
        if root is not None and cfg.ignore_rel:
            rel = os.path.relpath(full, root).replace(os.sep, "/")
            if any(rel == r or rel.startswith(r + "/") for r in cfg.ignore_rel):
                continue
        if os.path.isdir(full) and not os.path.islink(full):
            dirs.append(n)
        else:
            files.append(n)
    return files, dirs


def _count_files(p, cfg):
    total = 0
    for dirpath, dirnames, filenames in os.walk(p):
        dirnames[:] = [d for d in dirnames if d not in cfg.ignore]
        total += len(filenames)
    return total


def _read(p):
    try:
        with open(p, "r", errors="replace") as f:
            return f.read()
    except OSError:
        return ""


class _Scanner:
    def __init__(self, root, cfg, db):
        self.root = os.path.normpath(os.path.abspath(root))
        self.cfg = cfg
        self.v = []
        self.recs = []          # file image records
        self.exceptions = db.exceptions() if db is not None else {}
        self.prev = {f["path"]: f for f in db.files()} if db is not None else {}
        self.seen = set()

    def rel(self, *parts):
        return "/".join(parts)

    def add(self, path, rule, detail, kind="file", suggested=None, beneath=0):
        if kind != "tamper" and any(path == e or (self.exceptions[e] == "" and path.startswith(e + "/")) for e in self.exceptions):
            return
        self.v.append(Violation(path, rule, detail, kind, suggested, beneath))

    def record(self, relpath, absp, unit, leaf, version, slug, ext, status, header_status):
        st = os.stat(absp, follow_symlinks=False)
        old = self.prev.get(relpath)
        if old and old.get("size") == st.st_size and old.get("mtime") == st.st_mtime and old.get("sha256"):
            sha = old["sha256"]
        else:
            sha = sha256_file(absp)
        self.seen.add(relpath)
        # tamper detection against the previous image
        if old and old.get("sha256") and sha != old["sha256"] and old["status"] in ("numbered", "raw"):
            if self.exceptions.get(relpath) != sha:
                rule = "R06" if old["status"] == "raw" else "R03"
                self.add(relpath, rule, "modified since last scan (immutable file)", kind="tamper")
        self.recs.append(dict(path=relpath, unit=unit, leaf=leaf, version=version, slug=slug, ext=ext, sha256=sha,
                              size=st.st_size, mtime=st.st_mtime, status=status, header_status=header_status))

    # -------- walk
    def run(self):
        cfg = self.cfg
        files, dirs = _entries(self.root, cfg, self.root)
        for n in files:
            if n not in cfg.root_files and n not in cfg.allowlist:
                self.add(n, "R05", "file not allowed at tree root (allowed: %s)" % ", ".join(cfg.root_files + cfg.allowlist))
            self._plain(n, "", "", "special" if (n in cfg.root_files or n in cfg.allowlist) else "violation")
        for n in dirs:
            if n not in cfg.groups:
                self.add(n, "R11", "top-level dir %r is not a group root (%s); scanned provisionally as a group" % (n, ", ".join(cfg.groups)), kind="dir")
            self.group(n)
        # deleted tracked files
        for p, old in self.prev.items():
            if p not in self.seen and old["status"] in ("numbered", "raw"):
                self.add(p, "R06" if old["status"] == "raw" else "R03", "tracked immutable file is gone", kind="tamper")
        return self.v

    def _plain(self, relpath, unit, leaf, status):
        absp = os.path.join(self.root, relpath)
        self.record(relpath, absp, unit, leaf, None, None, os.path.splitext(relpath)[1].lstrip("."), status, "n/a")

    def group(self, g):
        cfg = self.cfg
        gp = os.path.join(self.root, g)
        files, dirs = _entries(gp, cfg, self.root)
        for n in files:
            ok = n in cfg.group_files or n in cfg.allowlist
            if not ok:
                self.add(self.rel(g, n), "R05", "file not allowed at group root (allowed: %s)" % ", ".join(cfg.group_files + cfg.allowlist))
            self._plain(self.rel(g, n), "", "", "special" if ok else "violation")
        for n in dirs:
            prob = R.dir_name_problem(n)
            if prob:
                self.add(self.rel(g, n), "R07", prob, kind="dir", beneath=_count_files(os.path.join(gp, n), cfg))
                continue
            self.unit(g, n)

    def unit(self, g, u):
        cfg = self.cfg
        up = os.path.join(self.root, g, u)
        unit = self.rel(g, u)
        files, dirs = _entries(up, cfg, self.root)
        for need in ("SPEC.md", "MEMORY.md"):
            if need not in files:
                self.add(unit, "R05", "unit root is missing %s" % need, kind="dir")
        for n in files:
            rp = self.rel(g, u, n)
            ok = n in cfg.unit_files or n in cfg.allowlist
            if not ok:
                self.add(rp, "R05", "file not allowed at unit root (allowed: %s)" % ", ".join(cfg.unit_files + cfg.allowlist))
            elif n == "SPEC.md":
                prob = R.spec_tail_problem(_read(os.path.join(up, n)))
                if prob:
                    self.add(rp, "R10", prob)
            self._plain(rp, unit, "", "special" if ok else "violation")
        for n in dirs:
            if n == "raw":
                self.raw(unit, os.path.join(up, n))
                continue
            prob = R.dir_name_problem(n)
            if prob:
                self.add(self.rel(g, u, n), "R07", prob, kind="dir", beneath=_count_files(os.path.join(up, n), cfg))
                continue
            self.sub(unit, [n])

    def raw(self, unit, rawp):
        for dirpath, dirnames, filenames in os.walk(rawp):
            dirnames[:] = sorted(d for d in dirnames if d not in self.cfg.ignore)
            for fn in sorted(filenames):
                ap = os.path.join(dirpath, fn)
                rp = os.path.relpath(ap, self.root).replace(os.sep, "/")
                self.record(rp, ap, unit, "raw", None, None, os.path.splitext(fn)[1].lstrip("."), "raw", "n/a")

    def sub(self, unit, dirs):
        cfg = self.cfg
        d = os.path.join(self.root, *unit.split("/"), *dirs)
        reld = self.rel(unit, *dirs)
        slug = dirs[-1]
        if len(dirs) > cfg.max_depth:
            self.add(reld, "R08", "depth %d exceeds max %d below the unit root" % (len(dirs), cfg.max_depth),
                     kind="dir", beneath=_count_files(d, cfg))
            return
        files, subdirs = _entries(d, cfg, self.root)
        if files and subdirs:
            self.add(reld, "R04", "mixes %d file(s) and %d subdir(s); branch dirs hold only dirs, leaf dirs only chain files" % (len(files), len(subdirs)), kind="dir")
        leaf = "/".join(dirs)
        versions = {}
        for n in files:
            rp = self.rel(reld, n)
            ap = os.path.join(d, n)
            if n in cfg.allowlist:
                if subdirs:
                    self.add(rp, "R04", "allowlisted file in a branch dir")
                self._plain(rp, unit, leaf, "special")
                continue
            pc = R.parse_chain(n, slug)
            if pc is None:
                sug = None
                if not subdirs:
                    sug = R._suggest(d, slug, n)
                self.add(rp, "R01", "must be named NNNNv%s.<ext> in %s/" % (slug, slug), suggested=sug)
                status = "numbered" if R.NUMBERED_RE.match(n) else "violation"
                self.record(rp, ap, unit, leaf, None, slug, os.path.splitext(n)[1].lstrip("."), status, "n/a")
                continue
            ver, ext = pc
            versions.setdefault(ver, []).append(n)
            hs = "n/a"
            if ext == "md":
                prob = R.header_problem(_read(ap), slug, ver)
                hs = "ok" if prob is None else "bad"
                if prob:
                    self.add(rp, "R09", prob)
            self.record(rp, ap, unit, leaf, ver, slug, ext, "numbered", hs)
        if versions:
            vs = sorted(versions)
            dup = [v for v in vs if len(versions[v]) > 1]
            if dup:
                self.add(reld, "R02", "duplicate version number(s) %s" % ", ".join("%04d" % v for v in dup), kind="dir")
            expect = list(range(1, vs[-1] + 1))
            miss = [v for v in expect if v not in versions]
            if miss or 0 in versions:
                msg = []
                if miss:
                    msg.append("missing %s" % ", ".join("%04d" % v for v in miss))
                if 0 in versions:
                    msg.append("version 0000 is never valid")
                self.add(reld, "R02", "versions not consecutive from 0001 (%s)" % "; ".join(msg), kind="dir")
        for n in subdirs:
            if n == "raw":
                self.add(self.rel(reld, n), "R06", "raw/ is only allowed at the unit root", kind="dir", beneath=_count_files(os.path.join(d, n), cfg))
                continue
            prob = R.dir_name_problem(n)
            if prob:
                self.add(self.rel(reld, n), "R07", prob, kind="dir", beneath=_count_files(os.path.join(d, n), cfg))
                continue
            self.sub(unit, dirs + [n])


def scan(root, db=None, config=None):
    """Return the list of Violations (sorted by path). Does not modify the DB."""
    s = _Scanner(root, config or R.DEFAULT, db)
    s.run()
    scan.last_records = s.recs
    return sorted(s.v, key=lambda x: (x.path, x.rule))


scan.last_records = []


def reconcile(root, db, config=None, open_incidents=False, actor="librarian-scan"):
    """scan + image sync. Tamper (immutable file changed/vanished since the previous image) always opens an
    incident; other violations only when open_incidents=True (legacy trees are inventoried, not locked,
    until LB7). Returns (violations, new_incident_ids)."""
    vs = scan(root, db, config)
    recs = list(scan.last_records)
    new = []
    for v in vs:
        if v.kind != "tamper" and not open_incidents:
            continue
        if db.has_open(v.path, v.rule):
            continue
        new.append(db.open_incident(actor, "scan", v.path, v.rule, v.detail))
    db.replace_files(recs)
    return vs, new


def file_record(root, relpath, cfg=None, prev=None):
    """Image record for one existing file (used by the watcher). Mirrors what scan() records."""
    cfg = cfg or R.DEFAULT
    parts = relpath.split("/")
    absp = os.path.join(root, *parts)
    st = os.stat(absp, follow_symlinks=False)
    name = parts[-1]
    unit = "/".join(parts[:2]) if len(parts) >= 3 else ""
    leaf, version, slug, status, hs = "", None, None, "violation", "n/a"
    ext = os.path.splitext(name)[1].lstrip(".")
    if len(parts) >= 4 and parts[2] == "raw":
        status, leaf = "raw", "raw"
    elif len(parts) >= 4:
        dirs = parts[2:-1]
        leaf, slug = "/".join(dirs), dirs[-1]
        pc = R.parse_chain(name, slug)
        if name in cfg.allowlist:
            status = "special"
        elif pc:
            version, ext = pc
            status = "numbered"
            if ext == "md":
                hs = "ok" if R.header_problem(_read(absp), slug, version) is None else "bad"
        elif R.NUMBERED_RE.match(name):
            status = "numbered"
    elif name in cfg.allowlist or (len(parts) == 1 and name in cfg.root_files) or \
            (len(parts) == 2 and name in cfg.group_files) or (len(parts) == 3 and name in cfg.unit_files):
        status = "special"
    if prev and prev.get("size") == st.st_size and prev.get("mtime") == st.st_mtime and prev.get("sha256"):
        sha = prev["sha256"]
    else:
        sha = sha256_file(absp)
    return dict(path=relpath, unit=unit, leaf=leaf, version=version, slug=slug, ext=ext, sha256=sha,
                size=st.st_size, mtime=st.st_mtime, status=status, header_status=hs)
