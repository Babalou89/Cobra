# librarian sandbox

Goal: reference implementation of the librarian from docs/0029v (directory contract + single enforcement point). Python 3 stdlib only, no network (NtfyNotifier is the only code that can reach the network and tests never call it).

This directory is the unit root for the sandbox. It is NOT a managed tree: code dirs (librarian/, tests/) and working files (GATES.md, REPORT.md) are deliberately outside the directory contract.

## How to run
- Tests: `python3 -m unittest discover -s tests 2>&1 | tail -3`
- Rules (RULES.md has the precise node classification): `from librarian.rules import check; check(root, actor, op, path, content=None, dest=None)`
- Scan a tree: `python3 -m librarian.cli scan ROOT [--db state/lib.db]`
- Serve: `python3 -m librarian.cli serve ROOT --db state/lib.db --port 8765`
  - POST /check {actor, op, path, content?, dest?}, GET /map?unit=group/unit, GET /health
- Client shim (exit 2 on deny/unreachable unless LIBRARIAN_BYPASS=1): `python3 -m librarian.client --url http://127.0.0.1:8765 ACTOR OP PATH`
- Resolve an incident (CLI only, direct DB): `python3 -m librarian.cli resolve ID approve|revert --db state/lib.db`
- Watcher: `python3 -m librarian.cli watch ROOT --db state/lib.db --quarantine state/quarantine`
- LB3 rehearsal: `python3 tools/lb3_rehearsal.py` (writes reports/lb3-rehearsal.txt)

## Files
- GATES.md: per-step gate (CHECK + EXPECT) written before each step and its result
- REPORT.md: decision log (one line each, with why)
- HANDOFF.md: only exists if a step was skipped

Current: GATES.md
Next: none
