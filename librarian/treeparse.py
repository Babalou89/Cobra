"""Parse `tree` output (as pasted from babalou2) into a replica of empty files/dirs."""
import os
import re

LINE_RE = re.compile(r"^((?:│   |    )*)(?:├── |└── )(.*)$")
SUMMARY_RE = re.compile(r"^(\d+) directories?, (\d+) files?$")

# Childless entries that `tree` cannot distinguish from empty dirs. Resolved by the tree's own summary line
# ("82 directories, 272 files"): these names are files (executables / extensionless vendor scripts).
FILE_NAMES_NO_EXT = {"pre-commit", "lsec6315update", "lsec6316update", "lsec6521update", "lsec6523update"}


def parse(text):
    """Return (entries, summary) ; entries = list of dict(path, depth, children:int). summary=(dirs, files) or None."""
    lines = text.replace("\r", "").replace(" ", " ").split("\n")
    entries, stack, summary = [], [], None
    for ln in lines:
        m = SUMMARY_RE.match(ln.strip())
        if m:
            summary = (int(m.group(1)), int(m.group(2)))
            continue
        m = LINE_RE.match(ln)
        if not m:
            continue
        depth = len(m.group(1)) // 4
        name = m.group(2)
        del stack[depth:]
        path = "/".join(stack + [name])
        entries.append({"path": path, "depth": depth, "children": 0})
        if stack:
            for e in reversed(entries[:-1]):
                if e["path"] == "/".join(stack):
                    e["children"] += 1
                    break
        stack.append(name)
    return entries, summary


def classify(entries, extra_files=()):
    """-> (dirs, files) sets of paths. Childless entries are files if they have an extension or are known
    extensionless files; everything else childless is an (empty) directory."""
    dirs, files = set(), set()
    for e in entries:
        name = e["path"].rsplit("/", 1)[-1]
        if e["children"] > 0:
            dirs.add(e["path"])
        elif re.search(r"\.[A-Za-z0-9_]+$", name) and not name.startswith(".") or name in FILE_NAMES_NO_EXT or e["path"] in extra_files:
            files.add(e["path"])
        else:
            dirs.add(e["path"])
    return dirs, files


def build_replica(text, dest):
    """Create dirs and empty files under dest. Returns (n_dirs, n_files, summary, ambiguous_dirs)."""
    entries, summary = parse(text)
    dirs, files = classify(entries)
    os.makedirs(dest, exist_ok=True)
    for d in sorted(dirs):
        os.makedirs(os.path.join(dest, *d.split("/")), exist_ok=True)
    for f in sorted(files):
        p = os.path.join(dest, *f.split("/"))
        os.makedirs(os.path.dirname(p), exist_ok=True)
        open(p, "w").close()
    childless_dirs = sorted(e["path"] for e in entries if e["children"] == 0 and e["path"] in dirs)
    return len(dirs), len(files), summary, childless_dirs
