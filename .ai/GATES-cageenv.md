# GATES — cageenv/build (executor: Claude subagent, direct)

## S0 Build fix: startWatchServer
CHECK: go build ./... && go vet ./... && go test ./... 2>&1 | grep -E 'FAIL|ok' | tail -20
EXPECT: no FAIL; 7+ packages ok (cmd now has a test); baseline 99 top-level tests pass
RESULT S0: PASS — build/vet clean, 8 pkgs ok (cmd added), RUN count 99 -> 101 (+2 watch tests)

## S1 Fixtures
CHECK: ls internal/worker/testdata/fixture-*.jsonl | wc -l && go test ./internal/worker -run TestFixturesLoad -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: 5 files; TestFixturesLoad PASS (5 subtests), ok
RESULT S1: PASS

## S2 Fake backend
CHECK: go test ./internal/backend/fake ./internal/worker -run 'TestFake|TestReplay' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: fake scripted backend + integration test driving the real Agent loop with fixture replies; PASS, current behavior asserted
RESULT S2: PASS — 107 -> 116 RUN lines

## S3-pre Extract attempt loop (cmd/loop.go runAttempts) — enables S3a/d/e/f tests
CHECK: go build ./... && go vet ./... && go test ./cmd -run TestRunAttemptsPassCommits -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: behavior-preserving extraction; characterization test: fake backend writes correct file, verify passes, run commits
RESULT S3-pre: PASS
