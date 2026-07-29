# COBRA — the `cage` binary

One Go binary. A deterministic enforcement cage with the agent worker
folded in. The model plugs in from outside over HTTP; the cage reads a
DOD (definition of done), drives the model through a jailed workspace,
scrapes quality off every touched file, and decides done-or-not with
**zero LLM in the decision**.

The agent does not decide when work is done. The cage decides.

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
is 100%% deterministic. The planner only reads failures and writes
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

## Architecture

```
cmd/               command surface: init run verify peek session gate strikes
internal/backend/  model-agnostic HTTP layer (llama_cpp | anthropic | openai)
internal/worker/   the agent loop + tools (fs/shell/web/sys), neutral prompt
internal/ctxdiet/  always-on context diet: budgets, compression, retrieval
internal/memory/   optional local JSONL + TF-IDF memory (off by default)
internal/cage/     THE ENFORCER: verify, core checks, DOD, quality, 8 checkers
internal/jail/     the one jailed workspace — agent writes never leave it
internal/state/    strikes + failure fingerprint, cooldown, audit, reports
internal/config/   .cage/config.yaml with embedded defaults
internal/planner/  optional LLM planner: diagnoses verify failures, writes PLAN.md
internal/gitx/     the binary's own git view: diffs, version history, commits
assets/            go:embed defaults for `cage init`
hooks/             pre-commit / pre-receive one-liners
```

Structural guarantees, enforced by construction:

1. **No LLM in the verification path.** `internal/cage` imports nothing
   from the backend or worker layers; every checker is parse,
   string-compare, file-stat, exit-code.
2. **The binary owns commit authority.** The worker package has no
   version-control access at all — only the cage commits, and only after
   verify passes. There is no hook to skip.
3. **The workspace is jailed.** Every tool path resolves through the
   jail; escapes are rejected before touching the filesystem. `$HOME`
   stays untouched.
4. **brain.md is inspected, never injected.** The cage checks it for
   existence and length; it never enters the model's context budget.
5. **Strikes fire on no-progress, not on every failure.** Each verify
   run's failures hash into a fingerprint; identical fingerprint = the
   agent is spinning = strike. Three consecutive = lock, human reset only.
6. **Quality checks skip on unknown languages** — a language without
   rules passes; the scraper never fabricates a failure.

## The Spine — running unattended

Four controls substitute for a human watching the session:

1. **Dead-man's switch** — every attempt has a wall-clock cap
   (`worker.attempt_seconds`) and a turn cap (`worker.max_turns`). Breach
   aborts the attempt; verify and the strike logic still run.
2. **In-attempt loop detection** — three identical consecutive actions or
   protocol errors abort the attempt immediately instead of burning the
   turn budget. Strikes police *between* attempts; this polices *within*.
3. **Large-payload protocol** — malformed JSON with raw newlines inside
   strings is repaired deterministically; `file_write` supports
   `append: true` so big files land in chunks; generation cap is
   configurable (`worker.max_gen_tokens`).
4. **Ralph mode** (default, `worker.mode: ralph`) — context is
   *reconstructed* every turn, never accumulated: task + DOD + last verify
   report + `NOTES.md` + the last action. The model records durable
   findings with the `note` tool; `NOTES.md` is its only memory. Context
   per turn is constant, so it cannot overflow, and nothing important can
   scroll away. Set `worker.mode: conversational` for the windowed
   history instead.

Watch it live: `cage watch` serves a self-contained dashboard on
`:8060` — the model's replies, every dispatched tool with its measured
result, verify verdicts, context tokens against budget, and strike state,
all read from the cage's own append-only records.

## Continual Harness — `cage evolve`

The harness learns; the cage decides. Modeled on the Continual Harness
idea (an LLM refines its own scaffolding from trajectory windows) with one
inversion: **the refiner proposes, a deterministic gate adopts.**

Every `cage run` logs each dispatched action — tool, measured outcome,
first line of output — to `.cage/trajectory.jsonl` (dispatch facts, never
model self-report). An offline `cage evolve` pass then:

1. Reads the trajectory tail, the last verify report, and the strike state
   — the ground-truth failure signal the cage already measures.
2. Asks the backend to propose up to `--max` skills, each as bash
   `run_sh` plus a self-contained offline `test_sh`.
3. Stages each candidate and runs its test (30s timeout). Test passes →
   installed to `.cage/skills/<name>/` and committed by the binary. Test
   fails → discarded and logged. The proposer's claims count for nothing.

Installed skills appear to the worker as one `skill_run` tool. The worker
can execute skills but cannot write into `.cage/` — the library only grows
through the gate, so what accrues there is verified capital, not landfill.
Because skills are plain code + test, they transfer across backends: swap
the model and the new one inherits everything its predecessors proved.

Prompt and memory evolution are deliberately out of v1 — prompt changes
need a benchmark fitness function first.

## The 8 checks

`synt` syntax · `empt` empty file · `stub` placeholder bodies ·
`secr` hardcoded secrets · `hold` unfinished-work markers ·
`impo` unused imports · `dupl` identical-hash files · `stru` structure.

Run them on any single file with `cage peek <file>`.

## Changelog

**v1.5.0** — planner (optional)
- **Planner** (opt-in, planner.enabled): between attempts, a second model
  reads verify failures + relevant source code and writes PLAN.md with
  actionable fixes. The worker reads it next attempt. Inspired by DeepSeek
  speculative decoding applied at the agent level. Uses Anthropic Messages
  API (OneProvider). Default off — zero behavior change unless enabled.
  Graceful degradation: if the API call fails, the cage runs without it.

**v1.4.0** — pluggable planning + critic, burn-in hardening
- **Planning stage** (opt-in, `planning_stage.enabled`): `cage plan` and an
  in-run stage generate a step-plan from the DOD and audit it for coverage —
  every criterion must be addressed. Gives a model structure to execute against
  instead of spiral.
- **mypy critic** (opt-in, `critic.enabled`): a deterministic post-pass stage
  that runs mypy on touched `.py` files, bounded by max-rounds and scoped by an
  allowlist. No LLM in the verdict.
- Both ship as toggleable plugins, **default off** — zero behavior change unless
  you enable them.
- Burn-in fixes (found by running the cage against itself): the quality officer
  no longer grades the cage's own files (`.cage_snippet.py`, `NOTES.md`,
  `.cage/`); the action parser repairs the `\'` JSON-escape quirk local models
  emit when embedding code; config keys are strict-decoded, so a misspelled
  toggle errors instead of silently doing nothing.
- New human-facing docs: `docs/WRITING-A-DOD.md`, `docs/WINDOWS-QUICKSTART.md`.

**v1.3.0** — reasoning-model readiness
- Backend reads `reasoning_content` + `finish_reason`; truncated thinking
  no longer becomes an empty reply that trips the strike logic.
- `worker.temperature` is configurable (default `0.6` for Qwen3/thinking).
- Backend HTTP timeout raised to 900s so slow local 32Bs are not cut off.
- The jail root defaults to the project root, so the worker's output lands
  where `verify` and `commit` look; `.cage`/`.git` stay write-protected
  relative to the jail.
- The default DOD ships a `ruff` lint criterion (skips gracefully if ruff
  is absent); `mypy` is a commented, opt-in criterion.

**v1.2.0** — THE SPINE: dead-man's switch, in-attempt loop abort, ralph
mode, `cage watch` dashboard.

## Versioning

`.ai/VERSION` must be bumped per change. The *old* value is read from
git history — the agent cannot fake a bump by editing the file it is
compared against.
