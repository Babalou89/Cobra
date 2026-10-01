# RULES: node classification and rule list (implements docs/0029v section 2)

## Tree and node classification (all paths relative to the managed root, e.g. /home/billy)
Depth is counted in path components from the root. Classification is by POSITION first, then (for scan) by content.
- Tree root (depth 0): files CLAUDE.md SPEC.md RULES.md MEMORY.md; dirs = group roots only.
- Group root (depth 1): name must be in config groups (default: infra models services agents projects archive). Files: SPEC.md RULES.md only. Dirs = unit roots.
- Unit root (depth 2, e.g. projects/fyt-7862): files SPEC.md MEMORY.md RULES.md only. MUST contain SPEC.md and MEMORY.md before any other file may be created in the unit. Dirs = subject dirs + at most one raw/.
- raw (unit/raw): free zone. Any names, any depth, any file type. Existing files are never edited, overwritten, deleted or moved out. Only exception: actor "librarian" may move files INTO raw/legacy-YYYY-MM-DD/.
- Below the unit root (outside raw), a directory is:
  - empty: allowed (placeholder, e.g. rooting/success/).
  - branch: contains subdirectories ONLY (no files at all, not even allowlisted names).
  - leaf (a chain): contains ONLY files named NNNNv<dirname>.<ext>, plus allowlisted tool names.
  - a dir holding both files and subdirs is a violation (R04).
- Allowlisted unnumbered names (config.allowlist, default .gitignore config.yaml): allowed at root/group/unit level and in leaf or empty dirs, never in branch dirs.
- Numbered file = basename matches ^\d{4}[vV_]. This includes grandfathered legacy names (uppercase V, underscore, version 0000). All numbered files are immutable.
- Ignored entirely: any path component named in config.ignore (default .git).

## Rules
- R00 request: unknown op, path outside root, root itself, missing target for edit/move/delete, move without dest.
- R01 leaf file name must match ^\d{4}v<leafdir>\.<ext>$ (lowercase v, slug == directory name, single alnum extension). Suggests the next valid name.
- R02 versions consecutive from 0001: a new version must be max+1 (0000 never valid, no gaps, no duplicates). Scan also reports gaps once per leaf.
- R03 numbered files immutable: no edit/overwrite/rename/delete. Only move: actor librarian, into <unit>/raw/legacy-YYYY-MM-DD/ with the name unchanged.
- R04 branch dirs hold only dirs; leaf dirs hold only chain files; no mixing.
- R05 root/group/unit file allowlist; unit must have SPEC.md + MEMORY.md; SPEC.md/MEMORY.md at a unit may not be deleted.
- R06 raw rules: no edit/overwrite/delete/move-out of existing raw content; raw only at unit root (one raw/ per unit).
- R07 directory names ^[a-z0-9]+(-[a-z0-9]+)*$ and never numbered (not starting with 4 digits + v).
- R08 depth: at most 3 directory levels below the unit root (raw excluded). Example: access/adb = 2, a/b/c = 3.
- R09 chain .md header, exactly the first 5 lines: `# <slug> vNNNN`, `Status: open|blocked|solved`, `Supersedes: <NNNNv previous>|none` (none only for 0001, otherwise exactly the previous version), `Summary: <one line, 1..300 chars>`, `---`. Header numbers must match the filename.
- R10 unit SPEC.md: last two non-blank lines are `Current: ...` then `Next: ...`.
- R11 layout: top-level dirs must be group roots from the config.
- LOCK: while any incident is open every op by every actor is denied until `librarian resolve <id> approve|revert`.

## Scan-only behaviours (reconciliation)
- Directories violating R07/R08 are reported once; their contents are counted but not examined individually.
- Unknown top-level dirs (R11) are reported once and then scanned provisionally as if they were groups, to give a useful inventory.
- Numbered/raw files whose sha256 changed or that vanished since the last scan are R03/R06 violations. Files approved by `resolve approve` are exceptions and are not re-reported.

## Spec ambiguities resolved (see REPORT.md for the one-line reasons)
- "Summary: <=3 lines" vs a fixed 5-line header: Summary is a single line.
- "Max depth 3 (e.g. access/adb)": counted in directory levels below the unit root.
- "Last two lines" of SPEC.md: last two non-blank lines.
