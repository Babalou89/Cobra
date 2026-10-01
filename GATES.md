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
