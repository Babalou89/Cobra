"""Generated `## Map` block for a unit SPEC.md (format fixed here; 0029v left it TBD).

Block:
    ## Map
    <!-- librarian:map:begin -->
    - <leaf path>/ | v<highest> | <status> | <summary first line>
    <!-- librarian:map:end -->
Non-md leaves (scripts) show status '-' and summary '-'. Empty dirs show 'v0000 | empty | -'.
"""
import os
import re

from . import rules as R

BEGIN = "<!-- librarian:map:begin -->"
END = "<!-- librarian:map:end -->"


def _leaves(udir, cfg):
    """Yield (relpath, files, subdirs) for every non-raw dir below the unit that holds files or is empty."""
    out = []
    for dirpath, dirnames, filenames in os.walk(udir):
        rel = os.path.relpath(dirpath, udir)
        dirnames[:] = sorted(d for d in dirnames if d not in cfg.ignore and not (rel == "." and d == "raw"))
        if rel == ".":
            continue
        if dirnames and not filenames:
            continue  # branch
        out.append((rel.replace(os.sep, "/"), sorted(filenames)))
    return sorted(out)


def _header_fields(path):
    try:
        with open(path, "r", errors="replace") as f:
            lines = [f.readline().rstrip("\n") for _ in range(5)]
    except OSError:
        return "?", "(unreadable)"
    st = re.match(r"^Status: (\S+)$", lines[1]) if len(lines) > 1 else None
    sm = lines[3][len("Summary:"):].strip() if len(lines) > 3 and lines[3].startswith("Summary:") else None
    return (st.group(1) if st else "?"), (sm if sm else "(bad header)")


def build_map(root, unit, config=None):
    cfg = config or R.DEFAULT
    udir = os.path.join(root, *unit.split("/"))
    lines = []
    for rel, files in _leaves(udir, cfg):
        slug = rel.split("/")[-1]
        best = None
        for fn in files:
            pc = R.parse_chain(fn, slug)
            if pc and (best is None or pc[0] > best[0]):
                best = (pc[0], pc[1], fn)
        if best is None:
            if not files:
                lines.append("- %s/ | v0000 | empty | -" % rel)
            continue
        ver, ext, fn = best
        if ext == "md":
            status, summary = _header_fields(os.path.join(udir, rel, fn))
        else:
            status, summary = "-", "-"
        lines.append("- %s/ | v%04d | %s | %s" % (rel, ver, status, summary))
    return "\n".join(["## Map", BEGIN] + lines + [END]) + "\n"


def apply_map(spec_text, block):
    """Insert/replace the Map block in SPEC.md text, keeping the Current:/Next: tail as the last two lines."""
    t = spec_text.replace("\r\n", "\n")
    if BEGIN in t and END in t:
        pre = t[:t.index(BEGIN)]
        post = t[t.index(END) + len(END):]
        pre = re.sub(r"## Map[ \t]*\n$", "", pre)
        return pre + block.rstrip("\n") + post
    m = re.search(r"^## Map[ \t]*\n.*?(?=^## |^Current:|\Z)", t, re.S | re.M)
    if m:
        return t[:m.start()] + block + "\n" + t[m.end():]
    cm = re.search(r"^Current:", t, re.M)
    if cm:
        return t[:cm.start()] + block + "\n" + t[cm.start():]
    return t.rstrip("\n") + "\n\n" + block


def update_spec(root, unit, config=None):
    sp = os.path.join(root, *unit.split("/"), "SPEC.md")
    with open(sp, "r") as f:
        old = f.read()
    new = apply_map(old, build_map(root, unit, config))
    if new != old:
        tmp = sp + ".tmp"
        with open(tmp, "w") as f:
            f.write(new)
        os.replace(tmp, sp)
    return new
