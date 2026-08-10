# COBRA — the `cage` binary

One Go binary. A deterministic enforcement cage with the agent worker
folded in. The model plugs in from outside over HTTP; the cage reads a
DOD (definition of done), drives the model through a jailed workspace,
scrapes quality off every touched file, and decides done-or-not with
**zero LLM in the decision**.

The agent does not decide when work is done. The cage decides.

## Architecture — Cage-Ralph-Model (v1.7.0)

Three layers, each with one job:

```
Cage (director)    reads DOD, verifies code, decides done, directs Ralph
Ralph (guide)      translates cage signals into clear instructions for model
Model (executor)   writes code. Sees DOD criteria checklist. Does NOT decide when done.
```

**The cage** owns the DOD. It breaks the contract into tasks and tells
Ralph what to tell the model. When verification fails, the cage tells
Ralph exactly what's broken. The cage is the only thing that calls DONE.

**Ralph** is the ralph-mode loop. Each turn it reconstructs the model's
context from scratch: task + fix target + verify report. Context is
clamped to 200 lines max — oldest drops, newest stays. The model never
accumulates state.

**The model** executes. It sees 5-20 lines per turn. Turn 1: just the
task. Turn 2+: task + "fix these failures." No DOD, no NOTES.md, no
exploration. It writes code because that's all it can do.

```
Turn 1:  "Task: build a file dedup CLI tool"
Turn 2+: "Task: build a file dedup CLI tool
          Fix these failures:
          VERDICT: FAIL — 2 failure(s)
            X source files exist: dedup.py does not exist
            X tests pass: command exited 1"
```

## Install

Requires Go 1.21+.

```bash
git clone git@github.com:Babalou89/cobra.git
cd cobra
go build -o cage .
install -m 0755 cage ~/.local/bin/cage   # or anywhere on PATH
```

Single static binary. No Python tree, no venv, no scattered folders.

## Quick Start

```bash
cd your-project
git init                    # the cage measures change through git
cage init                   # scaffold .cage/ + .ai/
$EDITOR .cage/config.yaml   # pick a backend
$EDITOR .cage/dods/example.yaml
cage run "implement the thing the DOD describes"
```

## Usage

| Command | Behavior |
|---|---|
| `cage init` | scaffold `.cage/` (config, state, dods/, reports/) + `.ai/` (brain.md, VERSION). Refuses if already initialized. |
| `cage run "<task>"` | jail, load + validate the DOD (zero criteria = hard fail), run the worker loop, verify, commit on pass. Optional plugins via config: `planning_stage.enabled`, `critic.enabled`. |
| `cage plan "<task>"` | generate an audited step-plan from the DOD — coverage-checked against every criterion — written to `.cage/plan.json`. |
| `cage verify [--gate] [--dod FILE] [--worktree DIR]` | run core checks → DOD criteria → quality officer. Exit 0 pass, 1 fail. `--dod` evaluates one DOD file only. |
| `cage peek <file>` | the 8 quality checks on one file, boxed output, preview only, no strike. |
| `cage session start\|end` | jail setup / archive the report and reset strike bookkeeping. |
| `cage gate install\|uninstall <bare>` | write/remove the pre-receive hook in a bare repo. |
| `cage strikes [reset]` | show strike count and lock state; `reset` is human-only. |
| `cage evolve [--max N]` | continual harness pass: propose skills from trajectories; adopt only what passes the gate. |
| `cage skills` | list the installed, gate-verified skill library. |
| `cage watch [--port N]` | zero-dependency web dashboard: live model steps, cage actions, context metrics, strikes. |

## Backends

The model is external and swappable — configured in `.cage/config.yaml`,
reached over HTTP through the `Backend` interface, no rebuild to switch:

- `llama_cpp` — a local llama-server (OpenAI-compatible chat endpoint)
- `anthropic` — the Claude API (`ANTHROPIC_API_KEY` from the environment)
- `openai` — OpenAI-compatible / OpenRouter (`OPENAI_API_KEY` / `OPENROUTER_API_KEY`)

All three are plain `net/http` — no SDKs, and API keys never live in
source or config.

**Reasoning models** (Qwen3, gemma "thinking", DeepSeek-style) are handled
natively: the backend reads `reasoning_content` as well as `content`, so a
reply truncated mid-thought is surfaced as an explicit error instead of an
empty reply that would burn a no-progress strike. Sampling temperature is
configurable (`worker.temperature`, default `0.6` — low temp makes thinking
models loop), and the per-request HTTP timeout is generous so a slow local
model (a split 32B at ~20 tok/s can spend minutes on one thinking turn) is
not severed mid-generation.

## Planner (optional)

The cage can call a second, stronger model between attempts to diagnose
verify failures and write an actionable PLAN.md. The worker reads it
next attempt. Inspired by DeepSeek speculative decoding at the agent
level: small model writes code, large model diagnoses failures.

The planner is never consulted during verification — the cage verdict
is 100% deterministic. The planner only reads failures and writes
advice. Cost: ~$0.007 per failed attempt via Claude Fable 5.

Enable in .cage/config.yaml:

  planner:
    enabled: true
    base_url: "https://api.oneprovider.dev"
    api_key: "sk-..."
    model: "claude-fable-5"

If the API call fails, the cage runs without it (graceful degradation).

## The DOD

A DOD is a YAML contract of criteria, each a list of deterministic
checks: `exists`, `command` (+ `expect_exit`), `grep` (`contains` /
`regex`), `lines` (`min` / `max`). Optional `quality:` thresholds
(e.g. `max_function_lines`, `no_todos`) ride on the scraped facts, and
optional `tools:` shell out to ruff/pytest/mypy/bandit. A DOD with zero
criteria is rejected outright — an empty contract gates nothing.

## Why Small Models Work

With the cage-ralph architecture, the model's context is 5-20 lines per
turn. It doesn't need to reason about the DOD or explore the codebase.
The cage directs, Ralph guides, the model executes.

This means speed matters more than intelligence. A 14B model at 50 tok/s
doing 5 attempts beats a 32B at 22 tok/s doing 2 attempts in the same
time. The cage catches mistakes — the model just needs to iterate fast.

Models tested with cage-ralph:
- Qwen2.5-Coder-32B-Instruct-Q5_K_M — 11/12 criteria on first real attempt
- (more to come)

## The 8 checks

`syntax` · `empty` · `stub` · `secrets` · `hold` · `imports` · `duplicate` · `structure`.

Run them on any single file with `cage peek <file>`.

## Changelog
**v1.9.0** — instruct harness (InstructRegistry + dod_format config)
- **InstructRegistry:** 2-tool registry (file_write + code_execute) for step-following models. No file_read, no note, no exploratory tools.
- **InstructPrompt:** aggressive system prompt — execute steps in order, write code immediately, no prose.
- **dod_format config field:** set `dod_format: instruct` in .cage/config.yaml to activate instruct mode.
- **RalphRegistry trimmed:** note tool removed (4 to 3 tools). Models wasted turns on notes instead of code.
- **Agent.Instruct:** skips brain.md and NOTES.md injection — task is self-contained.
- **Tests:** 33/33 worker tests pass including new TestInstructRegistryToolCount.

**v1.8.1** — build-breaker fix + loop-guard regression revert
- **Fixed:** `cmd/run.go` called `ensureProjectMeta(dir)`, a function that didn't exist anywhere in the codebase — `go build` was broken on `main`. Implemented it (creates `.ai/brain.md` / `.ai/VERSION` from embedded templates if missing, same pattern as `cage init`, never clobbers existing files).
- **Reverted:** `maxRepeat` (in-attempt exact-repeat loop guard) had been doubled from 3 to 6 in an uncommitted edit, weakening the v1.8.0 defense hardening with no evidence in logs to justify it. Restored to 3.
- **Added, left in:** an XML `<function=...>` tool-call fallback parser in `parseAction()`, with tests. Verified against the live model's actual Jinja chat template — it only emits `<tool_call>{json}</tool_call>` when the request includes an OpenAI `tools` schema, which cage never sends. Harmless, not exercised by the current pipeline.
- **Verified:** `go build`/`vet`/`test ./...` all clean (98 tests, 17 packages). Live production-style run against `dedup-tool.yaml` (the DOD that locked the cage on 2026-08-06) on Qwen2.5-Coder-32B-Instruct via llama.cpp: failures dropped 16→3→2→2 across attempts, no repeat of the catastrophic file_read loop. Run was still iterating when the test harness's 20-minute cap ended it — no regression, no lockup.

**v1.7.1** — cage reads files, injects code into directive
- **Cage reads broken files:**  in directive.go reads files mentioned in verify failures and includes their contents in the fix target. Model gets actual code, not just failure text.
- **System prompt updated:** WRITE CODE IMMEDIATELY, do not explore. file_write is primary tool.
- **Tests updated** to match new FixTarget format.



**v1.7.0** — cage-ralph architecture
- **Cage directs Ralph:** `internal/cage/directive.go` — `DOD.Direct(attempt, verifyReport)` breaks the DOD into a task for Ralph. The cage is the only thing that knows the DOD and decides when done.
- **Minimal model context:** `buildUser()` simplified — injects only task + fixTarget + verifyReport, clamped to 200 lines. No more NOTES.md, PLAN.md, or full DOD text in model context. Model sees 5-20 lines per turn.
- **Rolling window:** `ClampContext(content, maxLines)` — oldest context drops, newest stays. Model never accumulates state.
- **Agent.Run() signature change:** `(task, fixTarget, verifyReport)` — was 4 params, now 3. Model no longer sees DOD.
- **9 new tests:** directive (5), clamping (3), integration (1). All pass. Full suite clean.
- **First E2E test:** Qwen2.5-Coder-32B-Instruct — 11/12 criteria on first real attempt, no exploring, 14 file_writes in 14 turns.

**v1.6.0** — critic fix + verification engine tests
- **C1 fixed:** `RunMypy()` now checks `exec.LookPath("mypy")` before running. No more silent passes when mypy is missing.
- **C2 fixed:** `TestBoundedRemediateFiresAndBounds` skips cleanly when mypy is not installed (was the only failing test).
- **C3 added:** 31 new tests for `internal/cage/` — the verification engine now has real coverage: DOD loading, evaluation, Scrape, RunChecks, language detection, syntax checking, pluggable tools.
- **Full test suite passes with zero failures** for the first time.

**v1.5.0** — planner (optional)
- **Planner** (opt-in, planner.enabled): between attempts, a second model reads verify failures + relevant source code and writes PLAN.md with actionable fixes. Inspired by DeepSeek speculative decoding at the agent level. Uses Anthropic Messages API (OneProvider). Default off. Graceful degradation.

**v1.4.0** — pluggable planning + critic, burn-in hardening
- **Planning stage** (opt-in): `cage plan` and an in-run stage generate a step-plan from the DOD and audit it for coverage.
- **mypy critic** (opt-in): deterministic post-pass stage that runs mypy on touched `.py` files, bounded by max-rounds.
- Burn-in fixes: quality officer no longer grades cage's own files; action parser repairs JSON-escape quirk; config keys strict-decoded.

**v1.3.0** — reasoning-model readiness
- Backend reads `reasoning_content` + `finish_reason`.
- `worker.temperature` configurable. Backend HTTP timeout 900s.
- Jail root defaults to project root.

**v1.2.0** — THE SPINE: dead-man's switch, in-attempt loop abort, ralph mode, `cage watch` dashboard.

## Versioning

`.ai/VERSION` must be bumped per change. The *old* value is read from
git history — the agent cannot fake a bump by editing the file it is
compared against.
