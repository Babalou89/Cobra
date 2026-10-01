# REPORT — cageenv/build (executor: Claude subagent, direct)
Baseline before S2: 107 test RUN lines (go test ./... -v | grep -c '^=== RUN').

## Decisions
- D1: .anchor/ left untracked and uncommitted (not part of the brief).
- D2: tests count = `=== RUN` lines incl. subtests (brief recon used same method).
- D3: cmd/run.go held an uncommitted half-done refactor (runAttempts undefined, build broken); finished it as cmd/loop.go (behavior-preserving, characterization test first) because S3a/d/e/f need the attempt loop testable with the fake backend. Commit dbbe5e1.
- D4: S3e: recon said verify after turn-out might be missing; code already verifies after every attempt incl. worker error. Added worker.ErrTurnsExhausted + "turn-out" event + test instead of a redundant second verify.
- D5: S3a: CLI task goes in as "TASK: <cli>", DOD task appended under "Context (from the definition of done)"; if identical, DOD text is dropped.
- D6: S3d: handoff = error return (non-zero exit) + .cage/HANDOFF.md with last verify report; not a strike lock. Lock (4 identical-failure attempts) can still fire before 5.
- D7: S3f: bump = last numeric segment +1, only when tree has meaningful changes and VERSION still equals HEAD (model-made bumps never doubled); runs right before CommitAll after a passing verify.
- D8: S3b: prompt mentions the note tool only when the registry offers it; "definition of done" wording replaced with "DOD criteria".
- D9: S3c: when both done:true and tool+args are present the tool call wins; tool with no args + done is a done.
- D10: PR not opened: `gh` is not installed in this sandbox (no other auth attempted). Branch is pushed; open manually: base main, head cageenv/build.
- D11: Existing tests untouched; replay_test.go only gained counters (UnknownTools, NoteOK).

## Steps (all PASS, each pushed)
S3-pre dbbe5e1 | S3a 229d009 | S3b 8908545 | S3c 26c6df4 | S3d 34be148 | S3e f2490c0 | S3f 490f994 | S4 c3d21b0 | S5 this commit. S2 was already at 76981c0.

## Tests
`=== RUN` count: 107 (before S2) -> 132. Top-level PASS lines now 122. No FAIL; build and vet clean.

## S4 table (fixture replay through real Agent loop, fake backend; before = 76981c0)
| fixture | outcome | turns | protocol errors | unknown-tool results before -> after |
|---|---|---|---|---|
| parse-toolname | script exhausted (same) | 4 | 0 | 3 -> 0 |
| loop-read | loop abort 4 reads/6 (same) | 4 | 0 | 0 -> 0 |
| loop-version | 24 turns no done (same; run now verifies once) | 24 | 0 | 10 -> 0 |
| loop-edit | 24 turns no done (same) | 24 | 0 | 1 -> 0 |
| dedup | 3 consecutive protocol errors (same) | 7 | 5 | 1 -> 0 |
Fixtures hold only model replies, so worker outcomes cannot change; the win is 15 wasted unknown-tool turns -> 0. Live finished-rate still needs a real model.

## Skipped
None.
