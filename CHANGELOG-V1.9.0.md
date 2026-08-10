# CHANGELOG — v1.9.0 (2026-08-10)

## What Changed
- **InstructRegistry** — 2-tool registry (file_write + code_execute) for step-following models like Qwen. No file_read, no note, no exploratory tools.
- **InstructPrompt** — aggressive system prompt: "execute steps in order, write code immediately, no prose, no notes."
- **dod_format config field** — set `dod_format: instruct` in .cage/config.yaml to activate instruct mode.
- **RalphRegistry trimmed** — note tool removed (4 to 3 tools: file_read, file_write, code_execute). Models were wasting turns on notes instead of writing code.
- **Agent.Instruct field** — skips brain.md and NOTES.md injection in instruct mode. Task is self-contained.
- **cmd/run.go switch** — instruct selects InstructRegistry, ralph selects RalphRegistry, conversational selects Default.

## What Was Broken
- Qwen spent 22 notes/attempt in ralph mode (old: 27 attempts on babaface test)
- Note tool was a crutch — model recorded findings instead of acting on them
- No way to configure a stripped-down toolset for instruct DODs

## What's Fixed
- Instruct harness tested: 6 attempts, 0 notes, PASS on babaface DOD
- 33/33 worker tests pass (including new TestInstructRegistryToolCount)
- All 7 packages build and pass: cage, critic, jail, plan, skills, state, worker

## Test Results Before vs After

| Metric | v1.8.2 (before) | v1.9.0 (after) |
|--------|-----------------|----------------|
| RalphRegistry tools | 4 | 3 |
| InstructRegistry tools | N/A | 2 |
| babaface DOD attempts | 6 (with notes) | 6 (0 notes) |
| Worker tests | 29 | 33 |

## Config

    dod_format: instruct    # activates InstructRegistry + InstructPrompt
    worker:
      mode: ralph           # ralph is still the default
