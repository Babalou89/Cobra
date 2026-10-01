# GATES

## L1 Rules engine
CHECK: python3 -m unittest tests.test_rules 2>&1 | tail -3 ; full suite python3 -m unittest discover -s tests 2>&1 | tail -3
EXPECT: OK. check(root, actor, op, path, content=None, dest=None) returns Verdict(allow, rule_id, reason, suggested_name) for rules R00-R11 with >=2 allow and >=2 deny cases each.
RESULT L1: PASS (58 tests OK after 1 fix: raw subdir names must skip R07).

## L2 Rule-family tests + fyt-7862 fixture
CHECK: python3 -m unittest tests.test_rule_families tests.test_fyt_fixture 2>&1 | tail -3 ; full suite
EXPECT: OK. Table has >=2 allow and >=2 deny rows for each of R00..R11 (meta-test enforces the counts); the fyt-7862 target tree from 0029v section 4 is built file by file and every check() returns allow.
RESULT L2: PASS (62 tests OK).

## L3 SQLite image, scan reconciliation, incidents, LOCK
CHECK: python3 -m unittest tests.test_scan tests.test_db 2>&1 | tail -3 ; full suite
EXPECT: OK. scan(root) over the fyt-7862 fixture tree = 0 violations; seeded bad trees report each rule R01-R11 (R00 n/a); tamper (modified/deleted numbered or raw file between scans) reported as R03/R06; incidents open/resolve approve|revert persist in sqlite; with any open incident rules.check(lock=db.open_ids) denies every op with rule LOCK, and allows again after resolve.
RESULT L3: PASS (79 tests OK, first run).

## L4 HTTP service, map generator, CLI, fail-closed client shim
CHECK: python3 -m unittest tests.test_mapgen tests.test_server tests.test_client 2>&1 | tail -3 ; full suite
EXPECT: OK. POST /check returns {allow, rule_id, reason, suggested_name} (LOCK honoured); GET /map?unit= returns the generated block (leaf | highest version | status | summary first line); GET /health ok; POST /resolve is NOT served over HTTP (404, CLI only); server refuses hosts other than 127.0.0.1/10.0.0.1; client shim exit 0 allow, 2 deny, 2 unreachable (fail closed), 0 with LIBRARIAN_BYPASS=1 (logged to a spool and posted to /bypass when reachable).
RESULT L4: PASS (100 tests OK; fix: test path typo in mapgen test).

## L5 Watcher, quarantine, notifier
CHECK: python3 -m unittest tests.test_notify tests.test_watcher 2>&1 | tail -3 ; full suite
EXPECT: OK. Out-of-band bad file (both inotify-if-available and forced 1s poll modes) -> moved to quarantine/<incident-id>/<relpath>, incident opened, LogNotifier alarm recorded, global LOCK active, all within 3s of creation. Valid out-of-band files are indexed, not quarantined; half-written files are not judged until settled; modified/deleted immutable files open incidents (evidence copy, no data destroyed); approve restores a quarantined file and the watcher does not re-quarantine it; quarantine inside the tree (infra/librarian/quarantine) is ignored. NtfyNotifier is only constructed in tests, never sent.
RESULT L5: PASS (123 tests OK; watcher suite run 3x, stable; inotify mode available and used in AutoMode tests).

## L6 LB3 rehearsal on a replica of the babalou2 ~ tree
CHECK: python3 tools/lb3_rehearsal.py && python3 -m unittest tests.test_lb3 2>&1 | tail -3 ; full suite
EXPECT: OK. fixtures/babalou2-home-tree.txt parses to 272 files + 81 dirs (+ root = 82 per tree's own summary); replica of empty files is scanned (no changes, no auto-fix); reports/lb3-rehearsal.txt exists, is grouped by rule with counts that sum to the total, and gives one proposed migration action per violation class; every violation maps to a class.
RESULT L6: PASS (129 tests OK; report: 199 violations on replica).
