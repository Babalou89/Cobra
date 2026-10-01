# GATES

## L1 Rules engine
CHECK: python3 -m unittest tests.test_rules 2>&1 | tail -3 ; full suite python3 -m unittest discover -s tests 2>&1 | tail -3
EXPECT: OK. check(root, actor, op, path, content=None, dest=None) returns Verdict(allow, rule_id, reason, suggested_name) for rules R00-R11 with >=2 allow and >=2 deny cases each.
RESULT L1: PASS (58 tests OK after 1 fix: raw subdir names must skip R07).
