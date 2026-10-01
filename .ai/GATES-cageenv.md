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

## S3a Task wiring (CLI task primary, DOD task context)
CHECK: go test ./cmd -run TestTaskWiring -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: first user message to model contains "TASK: <cli string>" and the DOD task as context; test fails before fix
RESULT S3a: PASS (failed first, passes after composeTask)

## S3b note tool in ralph registry; prompt tool list generated from registry
CHECK: go test ./internal/worker -run 'TestRalph(PromptMatchesRegistry|NoteTool)' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: ralph registry has note (writes only NOTES.md); every registry tool listed in prompt; prompt mentions note only when registered; {"note":"x"} is a successful note call, not unknown tool
RESULT S3b: PASS

## S3c Deterministic decodeAction
CHECK: go test ./internal/worker -run 'TestDecodeAction(Deterministic|Precedence)' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: precedence "tool"+"args" > "done" > tool-name-keyed (sorted keys); 2-key reply decoded 100x gives identical tool
RESULT S3c: PASS

## S3d max_attempts (default 5) -> handoff terminal state
CHECK: go test ./cmd ./internal/config -run 'TestMaxAttempts|TestConfigMaxAttempts' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: config default 5 and key accepted by strict decode; exhausted loop returns handoff error, writes .cage/HANDOFF.md, makes no commit
RESULT S3d: PASS (compile-fail first, then pass)

## S3e Turn-out verify
CHECK: go test ./cmd -run TestTurnOutVerifyPasses -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: worker exhausts turns without done after writing correct files; verify runs once and PASS commits via normal path; events log a turn-out status. NOTE recon said verify-after-attempt may be missing: code already verifies after worker error; this step makes it explicit (ErrTurnsExhausted) + observable.
RESULT S3e: PASS (verify-after-attempt already existed; added ErrTurnsExhausted + turn-out event)

## S3f auto_version_bump (default true)
CHECK: go test ./cmd ./internal/cage ./internal/config -run 'AutoVersionBump' -v 2>&1 | grep -E '^(--- |ok|FAIL)'
EXPECT: config key accepted, default true; core check does not fail missing bump when enabled (still fails when false); run that never touches VERSION passes and commit contains bumped .ai/VERSION; model-made bump not doubled
RESULT S3f: PASS

## S4 Replay fixtures before/after
CHECK: go test ./internal/worker -run TestReplayTable -v 2>&1 | grep -E '^(\s+replay_table|--- |ok|FAIL)'
EXPECT: table of fixture | outcome | turns | protocol errors | note-tool results for all 5 fixtures, on S2 commit (before) vs HEAD (after); no panics, all terminate
RESULT S4: PASS. Replay of the 5 fixtures through the real Agent loop (fake backend, 24-turn cap), before = commit 76981c0 (S2), after = HEAD.
Columns: fixture | outcome before -> after | turns before -> after | protocol errors before -> after | "unknown tool" results before -> after | note tool OK after

| fixture | outcome (before -> after) | turns | protocol errors | unknown-tool results (before -> after) | note OK (after) |
|---|---|---|---|---|---|
| parse-toolname | script exhausted -> script exhausted | 4 -> 4 | 0 -> 0 | 3 -> 0 | 3 |
| loop-read | loop abort (4 reads/6) -> same | 4 -> 4 | 0 -> 0 | 0 -> 0 | 0 |
| loop-version | all 24 turns, no done -> same (now ErrTurnsExhausted, run verifies once) | 24 -> 24 | 0 -> 0 | 10 -> 0 | 10 |
| loop-edit | all 24 turns, no done -> same | 24 -> 24 | 0 -> 0 | 1 -> 0 | 1 |
| dedup | 3 consecutive protocol errors abort -> same | 7 -> 7 | 5 -> 5 | 1 -> 0 | 1 |

Reading: fixtures hold only the model's replies, so worker-level turns/outcomes cannot change; the measurable win is 15 wasted "unknown tool" turns -> 0 (note now works). Protocol errors in dedup are genuinely invalid replies and remain. Run-level (attempt loop) gains are covered by unit tests: max_attempts handoff, turn-out verify, auto version bump.
