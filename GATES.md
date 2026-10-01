# GATES

## L1 Rules engine
CHECK: python3 -m unittest tests.test_rules 2>&1 | tail -3 ; full suite python3 -m unittest discover -s tests 2>&1 | tail -3
EXPECT: OK. check(root, actor, op, path, content=None, dest=None) returns Verdict(allow, rule_id, reason, suggested_name) for rules R00-R11 with >=2 allow and >=2 deny cases each.
RESULT L1: PASS (58 tests OK after 1 fix: raw subdir names must skip R07).

## L2 Rule-family tests + fyt-7862 fixture
CHECK: python3 -m unittest tests.test_rule_families tests.test_fyt_fixture 2>&1 | tail -3 ; full suite
EXPECT: OK. Table has >=2 allow and >=2 deny rows for each of R00..R11 (meta-test enforces the counts); the fyt-7862 target tree from 0029v section 4 is built file by file and every check() returns allow.
RESULT L2: PASS (62 tests OK).
