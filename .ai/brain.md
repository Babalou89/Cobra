# COBRA — AI Brain

## What This Project Is
COBRA is a single Go binary (`cage`): a deterministic enforcement cage with an
AI agent worker folded in. The model plugs in from outside over HTTP. The binary
reads a DOD, drives the model through a jailed workspace, scrapes quality off
every touched file, and decides done-or-not with **zero LLM in the decision**.
Runs headless on babalou (Ubuntu 24.04, 10.0.0.1). One artifact — no Python
tree, no venv, no folder sprawl.

> NOTE: this file is inspected by the cage (exists / length / today's entry).
> It is NEVER injected into the worker model's context. brain.md bloat was the
> original context-killer; it stays out of the prompt budget.

## Tech Stack
- Go 1.21+ (spf13/cobra, gopkg.in/yaml.v3) — single static binary `cage`
- Model: external, swappable — llama.cpp / Anthropic / OpenAI-compatible over HTTP
- No external services. State is flat files under `.cage/`. No mem9.

## Current State
- Version: 1.7.0 — planner (optional Claude Fable 5 diagnosis between attempts)
- Spec of record: `COBRA-BUILD.md` (brief) + `cobra-build.yaml` (DOD)
- Source lineage: clean kimiversonf Hermes Cage v1.0 + cage-monitor + verify.sh,
  minus mem9, minus folder sprawl
- Build: `go build -o cage .` clean, `go vet` clean
- Tests: 1 failure in internal/critic (TestBoundedRemediateFiresAndBounds) — active DOD
- Fine-tuned cage-agent-32B Q5_K_M model ready at /home/billy/models/cage-agent-32b-q5km.gguf

## Architecture (one line each — full contracts in COBRA-BUILD.md)
- `internal/backend` — Backend interface + llama_cpp/anthropic/openai; model-agnostic
- `internal/worker`  — agent loop, jailed tools, neutral prompt; NO commit power
- `internal/cage`    — verify, core, dod, quality, 8 checkers; the deterministic enforcer
- `internal/ctxdiet` — token budget + trajectory compression (always on)
- `internal/jail`    — the one contained workspace
- `internal/state`   — strikes (no-progress), cooldown, audit, session, report
- `internal/config`  — .cage/config.yaml with embedded defaults
- `internal/planner` — optional LLM planner: diagnoses verify failures, writes PLAN.md
- `internal/critic`  — optional mypy post-pass critic (bounded remediation loop)
- `internal/plan`    — optional planning stage: generates step-plan from DOD via backend
- `internal/memory`  — optional local JSONL + TF-IDF memory (off by default)
- `internal/skills`  — gated skill library for cage evolve
- `internal/evolve`  — trajectory-based skill proposer (the ONLY package importing backend)
- `internal/gitx`    — the binary's own git view: diffs, version history, commits
- `cmd/`             — init, run, verify, peek, session, gate, strikes, evolve, skills, watch, plan

## Decisions Already Made (do not relitigate — see COBRA-BUILD.md §1)
1. No LLM in verify, ever — the zero-hallucination property
2. Binary owns commit; agent never runs git — closes `--no-verify` by construction
3. Model external + swappable via the Backend interface
4. No mem9 / no external continual memory
5. Jailed workspace — nothing lands in `$HOME`
6. brain.md is a file-check, never model context
7. Versioning kept; old value computed from git history (agent can't fake a bump)
8. Strikes fire on no-progress (identical failure fingerprint), not every failure
9. Quality checks skip-on-unknown — never fabricate a failure
10. Evolver OUT of v1
11. Local memory OFF by default (TF-IDF available)

## What We're Working On RIGHT NOW
- v1.7.0 shipped: cage-ralph architecture (cage directs, ralph guides, model executes)
- Model context reduced to 5-20 lines per turn (was 200+)
- First E2E test: Qwen2.5-Coder-32B — 11/12 criteria on first real attempt
- Next: test with smaller models (14B, gpt-oss-20b)

## Session Log
### 2026-07-05 — Continual Harness v1 (Claude, babalou)
- `cage evolve` built: internal/skills (store + deterministic gate + 6 tests),
  internal/evolve (proposer — the ONLY new package importing backend),
  state/trace.go trajectory logging, worker skill_run tool, cmd/evolve + cmd/skills
- Design: proposer proposes skills as run_sh+test_sh; gate runs the test;
  pass = installed + committed by the binary, fail = discarded. Worker cannot
  write .cage/ or .git/ (protectedWrite guard). Verify path still model-free.
- Rationale from continual-agent autopsy: self-evolution without a deterministic
  fitness gate self-corrupted on generation 1 (prompt contaminated, empty
  duplicate skills, /tmp memorized). Gate fixes attribution.
- DOD: .cage/dods/dod-evolve.yaml — verified with ./cage verify --dod
- Prompt/memory evolution deferred: needs benchmark fitness function first
### 2026-07-04 — COBRA v1 built (Claude, babalou)
- Wrote the full tree per COBRA-BUILD.md §2: 4 backend files, 7 worker files,
  6 cage files, jail+test, 6 state files+test, ctxdiet (3), memory, config,
  gitx, 8 cmd files, go:embed assets, hooks, DOD fixtures, README (116 lines)
- `go build -o cage .` clean; `go vet` clean; jail + state test suites pass
- All acceptance criteria proven locally: banned strings absent, cage package
  imports neither backend nor worker, worker has no commit path, init
  scaffolds in a cold dir, peek reports hold with line number, verify blocks
  fail.dod.yaml and passes pass.dod.yaml, strike logic locks after 3
- Version 0.0.1 → 1.0.0
### 2026-06-28 — Spec locked
- Traced the design across `cobra_kia_cage.md`, `agentv2`, `kimiversonf` v1.0
- Read live sources on B: (`continual.git/cage/verify.sh`, `cage-monitor`) — intact, not bastardized
- Diagnosed the real problem: no containment — the agent worked in `$HOME` (197 items),
  which is a workspace problem, not a strike-counter problem
- Settled all 11 decisions; wrote `COBRA-BUILD.md` + `cobra-build.yaml` to `~/cobra/`
- Version 0.0.1

## Known Issues
- For real tasks set `jail.root: "."` in the target project's config — the
  default `.cage/jail` separates the agent's world from the dir verify
  checks; decide the v1.1 layout (verify-inside-jail vs jail=project)
- web_search uses DuckDuckGo HTML scraping — brittle if their markup shifts
- Two soft toggles defaulted, not chosen: retrieval TF-IDF vs local BGE-M3
  (memory is off in v1 regardless)

## Version History
| Version | Date       | Change                      | Files |
|---------|------------|-----------------------------|-------|
| 1.7.0   | 2026-08-06 | cage-ralph architecture: cage directs, ralph guides, model executes. 5-20 line context per turn. | internal/cage/directive.go, internal/worker/agent.go, cmd/run.go |
| 1.6.0   | 2026-08-06 | critic fix (exec.LookPath guard), 31 verification engine tests, clean test suite | internal/critic/mypy.go, internal/cage/cage_test.go |
| 1.5.0   | 2026-07-09 | planner: optional Claude Fable 5 diagnosis between attempts, writes PLAN.md | internal/planner/, cmd/run.go, README.md |
| 1.4.0   | 2026-07-09 | pluggable planning + mypy critic, burn-in hardening, strict config decode | internal/plan/, internal/critic/, cmd/run.go, cmd/plan.go |
| 1.3.0   | 2026-07-07 | reasoning-model readiness: reasoning_content, configurable temp, 900s timeout | internal/backend/, internal/worker/, cmd/run.go |
| 1.2.0   | 2026-07-05 | THE SPINE: dead-man's switch, in-attempt loop abort (3 identical = abort), JSON control-char repair, file_write append, ralph mode default (context reconstructed per turn from task+DOD+NOTES.md), note tool, cage watch dashboard, worker config section; 8 new spine tests | internal/worker/, internal/config/, internal/state/events.go, cmd/watch.go, cmd/run.go, assets/dashboard.html, README.md |
| 1.1.2   | 2026-07-05 | shell guard: block command position only — dir named sudo/ was uninspectable; guard tests | internal/worker/tools_shell.go, guard_test.go |
| 1.1.1   | 2026-07-05 | context diet hardened after live overflow (32792>32768 on home-audit run): conservative estimator (len/3), tool-result ceiling (prompt_ceiling/4), shed-until-fits loop, backend overflow recovery, budget auto-capped at 55% of backend MaxContext | internal/ctxdiet/budget.go, internal/worker/agent.go, cmd/run.go |
| 1.1.0   | 2026-07-05 | Continual Harness v1: cage evolve, gated skill adoption, trajectory log, skill_run tool, .cage write protection | internal/skills/, internal/evolve/, internal/state/trace.go, internal/worker/, cmd/evolve.go, cmd/skills.go, cmd/run.go, README.md |
| 1.0.1   | 2026-07-04 | live smoke test PASSED: cage run drove Qwen3.5-27B via llama_cpp, worker wrote+tested hello.py, verify passed, binary committed — 1 attempt, 13s | .ai/brain.md, .ai/VERSION |
| 1.0.0   | 2026-07-04 | COBRA v1 built + verified   | full Go tree: main.go, cmd/, internal/, assets/, hooks/, tests/, README.md |
| 0.0.1   | 2026-06-28 | Spec written, repo seeded   | COBRA-BUILD.md, cobra-build.yaml, .ai/brain.md, .ai/VERSION |
