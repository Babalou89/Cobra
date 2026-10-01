"""Rules engine: the one implementation of the directory contract (docs/0029v).

Public API: check(root, actor, op, path, content=None, dest=None, config=None, lock=None) -> Verdict
Node classification is documented in RULES.md. Everything here is stdlib only.
"""
import os
import re
from dataclasses import dataclass

OPS = ("create", "write", "edit", "move", "delete")

DIR_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
NUMBERED_DIR_RE = re.compile(r"^\d{4}v")
NUMBERED_RE = re.compile(r"^\d{4}[vV_]")
ANY_CHAIN_RE = re.compile(r"^(\d{4})v(.+)\.([A-Za-z0-9]+)$")
LEGACY_DIR_RE = re.compile(r"^legacy-\d{4}-\d{2}-\d{2}$")
EXT_RE = re.compile(r"\.([A-Za-z0-9]+)$")
STATUSES = ("open", "blocked", "solved")


@dataclass(frozen=True)
class Config:
    groups: tuple = ("infra", "models", "services", "agents", "projects", "archive")
    root_files: tuple = ("CLAUDE.md", "SPEC.md", "RULES.md", "MEMORY.md")
    group_files: tuple = ("SPEC.md", "RULES.md")
    unit_files: tuple = ("SPEC.md", "MEMORY.md", "RULES.md")
    allowlist: tuple = (".gitignore", "config.yaml")
    max_depth: int = 3
    ignore: tuple = (".git",)


DEFAULT = Config()


@dataclass(frozen=True)
class Verdict:
    allow: bool
    rule_id: str
    reason: str
    suggested_name: object = None


class _Stop(Exception):
    def __init__(self, rule, reason, suggested=None):
        self.verdict = Verdict(False, rule, reason, suggested)


def _deny(rule, reason, suggested=None):
    raise _Stop(rule, reason, suggested)


def _allow(reason="allowed"):
    return Verdict(True, "OK", reason, None)


# ---------------------------------------------------------------- primitives (shared with scan)

def chain_name(slug, version, ext):
    return "%04dv%s.%s" % (version, slug, ext)


def parse_chain(name, slug):
    """Return (version, ext) if name is a valid chain file name for slug, else None."""
    m = ANY_CHAIN_RE.match(name)
    if m and m.group(2) == slug:
        return int(m.group(1)), m.group(3)
    return None


def dir_name_problem(name):
    """None if ok, else a reason string (rule R07)."""
    if NUMBERED_DIR_RE.match(name) or re.match(r"^\d{4}[vV_]", name):
        return "directory %r is numbered; directories are never numbered" % name
    if not DIR_RE.match(name):
        return "directory %r must match ^[a-z0-9]+(-[a-z0-9]+)*$ (lowercase, hyphen, no underscore/space)" % name
    return None


def header_problem(text, slug, version):
    """None if the first 5 lines are a valid chain header (rule R09), else a reason."""
    lines = text.replace("\r\n", "\n").split("\n")
    if len(lines) < 5:
        return "header needs 5 lines (# slug vNNNN / Status / Supersedes / Summary / ---)"
    want = "# %s v%04d" % (slug, version)
    if lines[0] != want:
        return "line 1 must be %r, got %r" % (want, lines[0][:60])
    m = re.match(r"^Status: (\S+)$", lines[1])
    if not m or m.group(1) not in STATUSES:
        return "line 2 must be 'Status: open|blocked|solved', got %r" % lines[1][:60]
    sup = "none" if version == 1 else "%04dv" % (version - 1)
    if lines[2] != "Supersedes: " + sup:
        return "line 3 must be 'Supersedes: %s', got %r" % (sup, lines[2][:60])
    if not lines[3].startswith("Summary:") or not lines[3][len("Summary:"):].strip():
        return "line 4 must be 'Summary: <text>' (non-empty)"
    if len(lines[3]) > 300:
        return "line 4 Summary longer than 300 chars"
    if lines[4] != "---":
        return "line 5 must be '---', got %r" % lines[4][:60]
    return None


def spec_tail_problem(text):
    lines = [l.strip() for l in text.replace("\r\n", "\n").split("\n") if l.strip()]
    if len(lines) < 2 or not lines[-2].startswith("Current:") or not lines[-1].startswith("Next:"):
        return "SPEC.md must end with 'Current: <path>' then 'Next: <line|none>' (last two non-blank lines)"
    return None


def _listdir(p):
    try:
        return sorted(os.listdir(p))
    except OSError:
        return []


def dir_contents(p):
    names = _listdir(p)
    files = [n for n in names if os.path.isfile(os.path.join(p, n)) or os.path.islink(os.path.join(p, n))]
    dirs = [n for n in names if os.path.isdir(os.path.join(p, n)) and not os.path.islink(os.path.join(p, n))]
    return files, dirs


def next_version(dirpath, slug):
    mx = 0
    for n in _listdir(dirpath):
        pc = parse_chain(n, slug)
        if pc:
            mx = max(mx, pc[0])
    return mx + 1


def _suggest(dirpath, slug, name):
    m = EXT_RE.search(name or "")
    ext = m.group(1) if m else "md"
    return chain_name(slug, next_version(dirpath, slug), ext)


# ---------------------------------------------------------------- path handling

def _rel_parts(root, path):
    root = os.path.normpath(os.path.abspath(root))
    p = path if os.path.isabs(path) else os.path.join(root, path)
    p = os.path.normpath(p)
    if p == root:
        _deny("R00", "path is the tree root itself")
    if os.path.commonpath([root, p]) != root:
        _deny("R00", "path %r is outside the managed root" % path)
    return tuple(p[len(root):].strip(os.sep).split(os.sep))


def _is_dir(root, parts, path):
    return path.endswith("/") or os.path.isdir(os.path.join(root, *parts))


def _unit_dir(root, parts):
    return os.path.join(root, parts[0], parts[1])


# ---------------------------------------------------------------- create validation

def _validate_create(cfg, root, parts, isdir, content):
    name = parts[-1]
    n = len(parts)
    if n == 1:
        if isdir:
            if name not in cfg.groups:
                _deny("R11", "top-level dir %r is not a group root (%s)" % (name, ", ".join(cfg.groups)))
            return
        if name not in cfg.root_files and name not in cfg.allowlist:
            _deny("R05", "tree root allows only %s" % ", ".join(cfg.root_files + cfg.allowlist))
        return
    if parts[0] not in cfg.groups:
        _deny("R11", "top-level dir %r is not a group root (%s)" % (parts[0], ", ".join(cfg.groups)))
    if n == 2:
        if isdir:
            prob = dir_name_problem(name)
            if prob:
                _deny("R07", prob)
            return
        if name not in cfg.group_files and name not in cfg.allowlist:
            _deny("R05", "group root allows only %s" % ", ".join(cfg.group_files + cfg.allowlist))
        return
    # n >= 3: inside a unit
    dir_parts = list(parts[1:] if isdir else parts[1:-1])
    if n >= 4 and parts[2] == "raw":
        dir_parts = dir_parts[:2]  # unit + raw; raw subdirs keep their original names
    for d in dir_parts:
        prob = dir_name_problem(d)
        if prob:
            _deny("R07", prob)
    bootstrap = (n == 3 and not isdir and name in ("SPEC.md", "MEMORY.md"))
    udir = _unit_dir(root, parts)
    if not bootstrap and not (os.path.isfile(os.path.join(udir, "SPEC.md")) and os.path.isfile(os.path.join(udir, "MEMORY.md"))):
        _deny("R05", "unit %s/%s needs SPEC.md and MEMORY.md at its root before anything else is created" % (parts[0], parts[1]))
    if n == 3:
        if isdir:
            return  # subject dir or raw/ at unit root, name already checked
        if name not in cfg.unit_files and name not in cfg.allowlist:
            _deny("R05", "unit root allows only %s (put content in a leaf chain or raw/)" % ", ".join(cfg.unit_files + cfg.allowlist))
        if name == "SPEC.md" and content is not None:
            prob = spec_tail_problem(content)
            if prob:
                _deny("R10", prob)
        return
    rel = parts[2:]
    if rel[0] == "raw":
        if not isdir and os.path.exists(os.path.join(root, *parts)):
            _deny("R06", "raw files are never overwritten")
        return
    if "raw" in rel[1:]:
        _deny("R06", "raw/ is only allowed at the unit root (one raw/ per unit)")
    dirs = list(rel) if isdir else list(rel[:-1])
    if len(dirs) > cfg.max_depth:
        _deny("R08", "depth %d exceeds max %d below the unit root" % (len(dirs), cfg.max_depth))
    # ancestors (everything above the immediate container) must not hold files
    base = udir
    for i, d in enumerate(dirs[:-1]):
        base = os.path.join(base, d)
        files, _ = dir_contents(base)
        if files:
            _deny("R04", "%s is a leaf dir (holds files %s); it cannot contain subdirectories" % (d, ", ".join(files[:3])))
    if isdir:
        return
    parent = os.path.join(udir, *dirs)
    _, subdirs = dir_contents(parent)
    if subdirs:
        _deny("R04", "%s is a branch dir (has subdirectories %s); branch dirs hold only dirs" % (dirs[-1], ", ".join(subdirs[:3])))
    slug = dirs[-1]
    if name in cfg.allowlist:
        return
    exists = os.path.exists(os.path.join(root, *parts))
    pc = parse_chain(name, slug)
    if pc is None:
        _deny("R01", "leaf file in %s/ must be named NNNNv%s.<ext> (4 digits, lowercase v, slug == dir name)" % (slug, slug),
              _suggest(parent, slug, name))
    if exists:
        _deny("R03", "%s exists; numbered files are immutable, write the next version" % name, _suggest(parent, slug, name))
    ver, ext = pc
    want = next_version(parent, slug)
    if ver != want:
        _deny("R02", "version %04d is not next; versions are consecutive from 0001 (next is %04d)" % (ver, want),
              chain_name(slug, want, ext))
    if ext == "md" and content is not None:
        prob = header_problem(content, slug, ver)
        if prob:
            _deny("R09", prob)


def _zone(parts):
    """raw | None for a path inside a unit."""
    if len(parts) >= 4 and parts[2] == "raw":
        return "raw"
    if len(parts) == 3 and parts[2] == "raw":
        return "rawdir"
    return None


def _walk_numbered_or_raw(p):
    """First problem found under dir p: ('R06', ...) or ('R03', ...) or None."""
    for dirpath, dirnames, filenames in os.walk(p):
        if os.path.basename(dirpath) == "raw" and os.path.dirname(dirpath) != p:
            pass
        for fn in filenames:
            if NUMBERED_RE.match(fn):
                return "R03", "contains numbered file %s (immutable)" % fn
    return None


def _check_delete(cfg, root, parts, path):
    ap = os.path.join(root, *parts)
    if not os.path.lexists(ap):
        _deny("R00", "delete target does not exist")
    name = parts[-1]
    isdir = os.path.isdir(ap)
    if _zone(parts) in ("raw", "rawdir"):
        _deny("R06", "raw/ content is never deleted")
    if len(parts) == 3 and not isdir and name in ("SPEC.md", "MEMORY.md"):
        _deny("R05", "%s is required at the unit root and cannot be deleted" % name)
    if not isdir:
        if NUMBERED_RE.match(name):
            _deny("R03", "numbered files are immutable and never deleted", None)
        return
    # directory: refuse if it would take raw or numbered files with it
    for dirpath, dirnames, filenames in os.walk(ap):
        rel = os.path.relpath(dirpath, root).split(os.sep)
        if len(rel) >= 3 and rel[2] == "raw":
            _deny("R06", "contains raw/ content (never deleted)")
        if len(rel) == 3 and "raw" in dirnames:
            _deny("R06", "contains raw/ content (never deleted)")
        for fn in filenames:
            if NUMBERED_RE.match(fn):
                _deny("R03", "contains numbered file %s (immutable)" % fn)
    if len(parts) == 2:
        # deleting a whole unit would remove SPEC/MEMORY: only an empty unit may go
        if os.listdir(ap):
            _deny("R05", "unit is not empty")


def _check_move(cfg, root, actor, parts, dparts, path, dest):
    src = os.path.join(root, *parts)
    if not os.path.lexists(src):
        _deny("R00", "move source does not exist")
    sname = parts[-1]
    isdir = os.path.isdir(src)
    if _zone(parts) in ("raw", "rawdir"):
        _deny("R06", "raw/ content never moves (not out of raw, not within raw)")
    numbered = (not isdir and NUMBERED_RE.match(sname)) or (isdir and _walk_numbered_or_raw(src))
    if numbered:
        legacy_ok = (actor == "librarian" and not isdir and len(dparts) >= 5 and parts[:2] == dparts[:2]
                     and dparts[2] == "raw" and LEGACY_DIR_RE.match(dparts[3]) and dparts[-1] == sname)
        if not legacy_ok:
            _deny("R03", "numbered files are immutable; the only move is by actor 'librarian' into "
                         "<unit>/raw/legacy-YYYY-MM-DD/ with the same name")
        if os.path.exists(os.path.join(root, *dparts)):
            _deny("R06", "destination exists; raw files are never overwritten")
        return
    _check_delete(cfg, root, parts, path)
    content = None
    if not isdir and sname.endswith(".md"):
        try:
            with open(src, "r", errors="replace") as f:
                content = f.read()
        except OSError:
            content = None
    _validate_create(cfg, root, dparts, isdir, content)


def check(root, actor, op, path, content=None, dest=None, config=None, lock=None):
    cfg = config or DEFAULT
    if lock is not None:
        ids = lock()
        if ids:
            return Verdict(False, "LOCK", "librarian is locked: open incident(s) %s; Billy must run "
                           "`librarian resolve <id> approve|revert`" % ", ".join(str(i) for i in ids), None)
    try:
        return _check(cfg, root, actor, op, path, content, dest)
    except _Stop as s:
        return s.verdict


def _check(cfg, root, actor, op, path, content, dest):
    if op not in OPS:
        _deny("R00", "unknown op %r (create|write|edit|move|delete)" % (op,))
    parts = _rel_parts(root, path)
    if any(p in cfg.ignore for p in parts):
        return _allow("ignored path")
    ap = os.path.join(root, *parts)
    isdir = _is_dir(root, parts, path)
    if op == "delete":
        _check_delete(cfg, root, parts, path)
        return _allow("delete ok")
    if op == "move":
        if not dest:
            _deny("R00", "move needs dest")
        dparts = _rel_parts(root, dest)
        if any(p in cfg.ignore for p in dparts):
            return _allow("ignored path")
        _check_move(cfg, root, actor, parts, dparts, path, dest)
        return _allow("move ok")
    exists = os.path.exists(ap)
    if op == "edit" and not exists:
        _deny("R00", "edit target does not exist (use create)")
    if exists and not isdir:
        # edit/write/create over an existing file
        name = parts[-1]
        if _zone(parts) == "raw":
            _deny("R06", "raw files are never edited or overwritten")
        if NUMBERED_RE.match(name):
            sug = None
            if len(parts) >= 4:
                sug = _suggest(os.path.dirname(ap), parts[-2], name)
            _deny("R03", "numbered files are immutable (never edit/overwrite); write the next version", sug)
        special = False
        if len(parts) == 1:
            special = name in cfg.root_files or name in cfg.allowlist
        elif len(parts) == 2:
            special = name in cfg.group_files or name in cfg.allowlist
        elif len(parts) == 3:
            special = name in cfg.unit_files or name in cfg.allowlist
        elif name in cfg.allowlist:
            special = True
        if not special:
            _deny("R01", "%s is a stray non-chain file; move it to raw/ or write a chain version" % name)
        if len(parts) == 3 and name == "SPEC.md" and content is not None:
            prob = spec_tail_problem(content)
            if prob:
                _deny("R10", prob)
        return _allow("edit ok")
    _validate_create(cfg, root, parts, isdir, content)
    return _allow("create ok")
