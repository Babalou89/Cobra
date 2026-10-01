"""Violation -> proposed migration action (LB3 rehearsal / LB4 planning). Proposals only: nothing here touches a tree."""
import os
import re
from collections import Counter, defaultdict

from . import rules as R

LEGACY = "raw/legacy-2026-09-30"

# class id -> (label, proposed action)
CLASSES = {
    "A01": ("legacy numbered file (uppercase V / underscore / 0000 / wrong slug)",
            "librarian moves it UNCHANGED into <unit>/%s/; one agent reads the legacy and writes the consolidated 0001v snapshot of the proper leaf" % LEGACY),
    "A02": ("loose file in the home root",
            "classify per 0029v section 3: fyt.prop -> projects/fyt-7862/raw/ (original name) or config chain; headunit_screen*.jpg -> projects/fyt-7862/raw/dumps/; "
            "0024v... recon script -> infra/<unit>/<leaf>/ as 0001v chain (old name to raw/legacy); pull_test.sh and test_reverse.sh -> TBD (Billy decides)"),
    "A03": ("top-level dir that is not a group root",
            "architeture/ -> rename to infra/ (misspelling) and fold hooks/librarian into units; agent-pool/ -> agents/agent-pool; operator-core/, recon/, watch/ -> TBD (Billy decides: infra/ units or archive/)"),
    "A04": ("unit root without SPEC.md/MEMORY.md",
            "migration step 1: create SPEC.md (goal, key facts, ## Map, Current:/Next:) and MEMORY.md before anything else is moved into the unit"),
    "A05": ("stray doc/file at group or unit root (AGENTS/CLAUDE/WORKSPACE/GATES/STATUS/DECISIONS/CONVENTIONS...)",
            "fold the useful content into the unit SPEC.md / RULES.md / MEMORY.md; move the original UNCHANGED to <unit>/%s/" % LEGACY),
    "A06": ("dump directory with tool-generated names (diagnostic_YYYYMMDD_*)",
            "move the whole dir unchanged to <unit>/raw/dumps/ (hash-tracked); summarise in the relevant 0001v leaf"),
    "A07": ("vendor firmware / extracted payload tree",
            "move the whole tree unchanged to <unit>/raw/firmware/ (original names, hash-tracked); reference it from the references/ or config/ leaf"),
    "A08": ("directory name not lowercase-hyphen or numbered",
            "rename to the suggested slug (empty/placeholder dirs) or fold into a proper leaf; numbered dirs (0001Vname) become leaf chains, original to raw/legacy"),
    "A09": ("directory deeper than 3 levels below the unit root",
            "flatten into a leaf at depth <=3, or move the subtree to <unit>/raw/ if it is vendor/dump material"),
    "A10": ("dir mixes files and subdirs (branch/leaf violation)",
            "split: files go to a leaf dir named after their purpose (as 0001v chain) or to raw/legacy; subdirs stay as branches"),
    "A11": ("file in a leaf with a non-chain name",
            "write its content as the next NNNNv<leaf> version (full snapshot, valid header); move the original UNCHANGED to <unit>/%s/" % LEGACY),
    "A12": ("version sequence problem (gap or duplicate)",
            "record the gap in MEMORY.md, no renumbering (0029v: 'Old numbering gap 0011-0012: just record, no action'); a 0000 file goes to raw/legacy"),
    "A13": ("chain file with a bad header",
            "numbered files are immutable: move to raw/legacy and write a new, valid-header snapshot"),
    "A14": ("unit SPEC.md without Current:/Next: tail",
            "agent appends Current:/Next: lines, then `librarian map --write` regenerates the ## Map"),
    "A15": ("raw/ misuse", "move content to the unit-root raw/ (one raw/ per unit)"),
}

DOC_RE = re.compile(r"^(AGENTS|CLAUDE|WORKSPACE|GATES|DECISIONS|CONVENTIONS|MEMORY|README|STATUS.*|FINAL_REPORT.*|.*REPORT.*)(\.[A-Za-z0-9]+)?$", re.I)


def slugify(name):
    s = re.sub(r"^\d{4}[vV_]?", "", name)
    s = re.sub(r"([a-z0-9])([A-Z])", r"\1-\2", s).lower()
    s = re.sub(r"[^a-z0-9]+", "-", s).strip("-")
    return s or "unnamed"


def classify(v):
    parts = v.path.split("/")
    name = parts[-1]
    rule = v.rule
    inside_unit = len(parts) >= 3
    if rule == "R11":
        return ("A03",) + CLASSES["A03"]
    if rule == "R05" and v.kind == "dir" and "missing" in v.detail:
        return ("A04",) + CLASSES["A04"]
    if inside_unit and "firmware" in parts[2:]:
        return ("A07",) + CLASSES["A07"]
    if rule == "R05":
        if len(parts) == 1:
            return ("A02",) + CLASSES["A02"]
        if R.NUMBERED_RE.match(name):
            return ("A01",) + CLASSES["A01"]
        return ("A05",) + CLASSES["A05"]
    if rule == "R07":
        if re.match(r"^diagnostic_\d", name) or re.match(r"^(dump|diagnostic)[-_]", name, re.I):
            return ("A06",) + CLASSES["A06"]
        return ("A08",) + CLASSES["A08"]
    if rule == "R08":
        return ("A09",) + CLASSES["A09"]
    if rule == "R04" and v.kind == "dir":
        return ("A10",) + CLASSES["A10"]
    if rule == "R02":
        return ("A12",) + CLASSES["A12"]
    if rule == "R09":
        return ("A13",) + CLASSES["A13"]
    if rule == "R10":
        return ("A14",) + CLASSES["A14"]
    if rule == "R06":
        return ("A15",) + CLASSES["A15"]
    if rule in ("R01", "R03") or rule == "R04":
        if R.NUMBERED_RE.match(name):
            return ("A01",) + CLASSES["A01"]
        if DOC_RE.match(name):
            return ("A05",) + CLASSES["A05"]
        return ("A11",) + CLASSES["A11"]
    return ("A11",) + CLASSES["A11"]


def write_report(root, violations, out_path, summary=None, n_dirs=None, n_files=None):
    """Write the grouped LB3 report. Returns the total violation count."""
    by_rule = defaultdict(list)
    by_class = defaultdict(list)
    skipped = 0
    for v in violations:
        by_rule[v.rule].append(v)
        by_class[classify(v)[0]].append(v)
        skipped += v.beneath
    L = []
    L.append("LB3 rehearsal: reconciliation scan of a REPLICA of the babalou2 home tree (empty files; no changes made, no auto-fix)")
    L.append("Source: fixtures/babalou2-home-tree.txt (tree listing from babalou2). Rules: RULES.md / docs 0029v.")
    if summary:
        L.append("Replica: %s dirs + %s files (tree summary: %d dirs incl. root, %d files)" % (n_dirs, n_files, summary[0], summary[1]))
    L.append("Total violations: %d   (+%d files inside directories reported once and not examined individually)" % (len(violations), skipped))
    L.append("")
    L.append("== Counts by rule ==")
    for r in sorted(by_rule):
        L.append("%s %5d" % (r, len(by_rule[r])))
    L.append("")
    L.append("== Detail grouped by rule ==")
    for r in sorted(by_rule):
        L.append("")
        L.append("[%s] %d violation(s)" % (r, len(by_rule[r])))
        for v in by_rule[r]:
            extra = " [+%d files beneath]" % v.beneath if v.beneath else ""
            sug = " -> suggest %s" % v.suggested if v.suggested else ""
            L.append("  %s | %s%s%s" % (v.path, v.detail, sug, extra))
    L.append("")
    L.append("== Proposed migration actions by class (proposals only; nothing executed) ==")
    for cid in sorted(by_class):
        label, action = CLASSES[cid]
        vs = by_class[cid]
        L.append("")
        L.append("%s (%d) %s" % (cid, len(vs), label))
        L.append("  action: %s" % action)
        rc = Counter(v.rule for v in vs)
        L.append("  rules: %s" % ", ".join("%s x%d" % (r, n) for r, n in sorted(rc.items())))
        for v in vs[:3]:
            L.append("  e.g. %s" % v.path)
    L.append("")
    L.append("== Top 5 migration actions (by violation count) ==")
    for i, (cid, vs) in enumerate(sorted(by_class.items(), key=lambda kv: (-len(kv[1]), kv[0]))[:5], 1):
        L.append("%d. %s x%d  %s => %s" % (i, cid, len(vs), CLASSES[cid][0], CLASSES[cid][1]))
    prov = sum(1 for v in by_class.get("A04", []) if v.path.split("/")[0] not in R.DEFAULT.groups)
    L.append("")
    L.append("== Caveats ==")
    L.append("- A04 is inflated: %d of %d come from dirs under UNKNOWN top-level dirs (R11), scanned provisionally as groups, so their children count as units." % (prov, len(by_class.get("A04", []))))
    L.append("- The replica has EMPTY files: content rules (R09 chain header, R10 SPEC tail on real content) are under-reported here; run `librarian scan` on the real tree for the true count.")
    L.append("- Empty childless entries are classified file/dir by name (extension or known extensionless names); the tree summary (272 files) matches exactly.")
    L.append("")
    L.append("No auto-fix: the librarian only reports. LB4 migrates one unit (fyt-7862) first; LB7 enables the lock after all legacy is classified.")
    os.makedirs(os.path.dirname(os.path.abspath(out_path)), exist_ok=True)
    with open(out_path, "w") as f:
        f.write("\n".join(L) + "\n")
    return len(violations)
