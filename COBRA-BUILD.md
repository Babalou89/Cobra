# COBRA — Build Brief

**For:** the writer on babalou (Claude or Kimi). You are building this from cold. Everything you need is in this file and `cobra-build.yaml`. Do not improvise beyond it. If a decision isn't here, it was deliberately left to you — make the simplest choice that satisfies the DOD.

**What you are building:** one Go binary called `cage`. It is a deterministic enforcement cage with the agent worker folded in. The model plugs in from outside over HTTP. The binary reads a DOD, drives the model through a jailed workspace, scrapes quality off every touched file, and decides done-or-not with zero LLM in the decision. One artifact. No Python tree, no venv, no scattered folders.

**Target:** Ubuntu 24.04 on babalou (10.0.0.1). Build with the Go toolchain on the node. Single static binary.

---

## 1. Non-negotiable decisions (already settled — do not relitigate)

1. **No LLM in the verification path. Ever.** `verify`, the quality scraper, and every checker are deterministic code — parse, string-compare, file-stat, exit-code. This is the property that gave zero hallucinations. A model anywhere in the decision loop is a build failure.
2. **The binary owns commit authority. The agent never runs git.** The agent works in a jailed workspace with no commit power. Only `cage` commits, and only after `verify` passes. This closes `--no-verify` by construction — there is no hook to skip.
3. **The model is external and swappable.** Reached over HTTP through the `Backend` interface. The binary never embeds a model and never assumes one. Llama.cpp, Anthropic, OpenAI-compatible — swap by config, no rebuild.
4. **No mem9. No external continual memory.** The string `mem9`, the host `10.0.0.1:9090`, and the key `c1a5fed9-...` must not appear anywhere in the source. Retrieval, if enabled, is local only.
5. **The workspace is jailed.** The agent gets one directory and that is its entire world. Nothing it writes lands in `$HOME`. This is the fix for the 197-file sprawl.
6. **brain.md is a file the cage inspects — never model context.** It is checked for existence/length/today's-entry. It is never injected into the model's prompt. It was the original context-killer; keep it out of the budget.
7. **Versioning stays.** `.ai/VERSION`, bumped per change. The *old* value is computed from git history so the agent cannot fake a bump.
8. **Strikes fire on no-progress, not on every failure.** Compare this run's failure fingerprint to the previous run's. Fewer/different failures = progress = no strike. Identical failures = spinning = strike. Three consecutive no-progress runs = lock, human reset only. This is the anti-oblivion-loop guard, not a hair-trigger.
9. **Quality checks skip-on-unknown.** A language we have no rules for returns pass, never a fabricated failure. The scraper never fails on ignorance.
10. **Evolver is OUT of v1.** No skill/subagent self-evolution. It is a second thing that shapes output and competes with the cage for attribution. Add later if ever.
11. **Local memory is OFF by default.** TF-IDF over a local `memories.jsonl`, available via config, disabled in v1 for clean attribution. The context diet (budget caps + compression) is always on regardless.

The oracle for any micro-decision: *does this sharpen the target, clean the workspace, or give an honest mirror? keep it. does it only police the agent? cut it.*

---

## 2. Repository layout (build exactly this)

```
cobra/                         # Go module · builds → `cage`
├── go.mod                     # module cobra; go 1.21+; deps: cobra, yaml.v3
├── go.sum
├── main.go                    # entrypoint → cmd.Execute()
├── COBRA-BUILD.md             # this file
├── cobra-build.yaml           # the DOD you must satisfy
│
├── cmd/
│   ├── root.go                # `cage` root command
│   ├── init.go                # cage init — scaffold .cage/ + .ai/ in a project
│   ├── run.go                 # cage run "<task>" — the worker loop
│   ├── verify.go              # cage verify [--gate] — the deterministic gate
│   ├── peek.go                # cage peek <file> — single-file quality preview
│   ├── session.go             # cage session start|end
│   ├── gate.go                # cage gate install|uninstall — pre-receive hook
│   └── strikes.go             # cage strikes [reset]
│
├── internal/
│   ├── backend/               # MODEL-AGNOSTIC layer
│   │   ├── backend.go         # Backend interface + Msg type
│   │   ├── registry.go        # type→constructor; New(cfg) (Backend, error)
│   │   ├── llamacpp.go        # local llama-server (OpenAI-compatible /v1/chat/completions)
│   │   ├── anthropic.go       # Claude API (/v1/messages, system split out)
│   │   └── openai.go          # OpenAI-compatible / OpenRouter (Bearer auth)
│   │
│   ├── worker/                # THE AGENT (loop, ported to Go)
│   │   ├── agent.go           # read DOD → act → attempt done → loop; no commit power
│   │   ├── tools.go           # registry + dispatch; ToolResult
│   │   ├── tools_fs.go        # file_read / file_write / file_edit — paths forced inside jail
│   │   ├── tools_shell.go     # shell / code_execute — blocked-command guard
│   │   ├── tools_web.go       # web_search / web_fetch
│   │   ├── tools_sys.go       # system_info
│   │   └── prompt.go          # NEUTRAL system prompt — no hardware/model/quality coaching
│   │
│   ├── ctxdiet/               # THE CONTEXT DIET (always on)
│   │   ├── budget.go          # hard token cap + prompt ceiling; refuse over-budget
│   │   ├── compress.go        # trajectory compression: last N steps, tool names only
│   │   └── retrieve.go        # LOCAL retrieval; no-op unless memory enabled
│   │
│   ├── memory/                # LOCAL memory (optional, off by default)
│   │   └── memory.go          # jsonl store + TF-IDF search
│   │
│   ├── cage/                  # THE ENFORCER
│   │   ├── verify.go          # orchestrate: core → DOD → quality → verdict → strike/commit
│   │   ├── core.go            # cage-lock, config valid, brain.md, version bump, syntax
│   │   ├── dod.go             # load + validate DOD; >=1 criterion or HARD FAIL
│   │   ├── quality.go         # Quality Officer: scrape facts, run 8 checks, build report
│   │   ├── checkers.go        # the 8 checkers + the FileFacts scraper
│   │   └── pluggable.go       # shell out to ruff/pytest/mypy/bandit when DOD names them
│   │
│   ├── jail/
│   │   └── workspace.go       # create / enter / teardown the one jailed work dir
│   │
│   ├── state/
│   │   ├── strikes.go         # no-progress strike logic + failure fingerprint + reset
│   │   ├── cooldown.go        # minimum interval between attempts
│   │   ├── audit.go           # append-only audit log
│   │   ├── session.go         # session lifecycle state
│   │   └── report.go          # quality report accumulate + archive
│   │
│   ├── config/
│   │   └── config.go          # load .cage/config.yaml + DOD pointer; embedded defaults
│   │
│   └── gitx/
│       └── git.go             # diff --numstat, changed files, version from history, worktree
│
├── hooks/
│   ├── pre-commit             # one line → cage verify  (convenience only)
│   └── pre-receive            # gate → cage verify --gate  (defense in depth)
│
└── assets/                    # go:embed defaults baked into the binary
    ├── config.default.yaml
    ├── dod.example.yaml
    └── brain.template.md
```

---

## 3. Core contracts (implement these signatures)

### Backend interface — `internal/backend/backend.go`

```go
type Msg struct {
    Role    string // "system" | "user" | "assistant" | "tool"
    Content string
}

type Backend interface {
    Chat(messages []Msg, temperature float64, maxTokens int) (string, error)
    Health() bool
    Name() string
    MaxContext() int
}
```

`registry.New(cfg)` reads `backend.type` (`llama_cpp` | `anthropic` | `openai`) and `backend.params`, returns the matching implementation. Unknown type = error listing valid types. All three implementations use only the Go standard library (`net/http`, `encoding/json`) — no SDK deps. API keys come from env vars (`ANTHROPIC_API_KEY`, etc.), never hardcoded.

### Quality scraper facts — `internal/cage/checkers.go`

```go
type FileFacts struct {
    Path         string
    Exists       bool
    SizeBytes    int
    SyntaxOK     bool
    Lines        int     // total
    CodeLines    int     // non-blank, non-comment
    BlankLines   int
    CommentLines int
    CommentRatio float64 // comments / max(code,1)
    MaxLineLen   int
    Functions    int
    Classes      int
    LongestFunc  int     // lines in the largest function
    MaxNesting   int     // deepest indent / brace depth
    BranchCount  int     // if/for/while/case/&&/|| — complexity proxy
    EmptyBodies  int     // pass-only or {} bodies
    TodoCount    int
    UnusedImports []string
    MixedIndent  bool
    SecretHits   int
    SHA256       string
}

type ChangeFacts struct {
    FilesTouched, FilesCreated, FilesDeleted int
    LinesAdded, LinesRemoved                 int
    DuplicateFiles [][]string  // identical SHA-256 groups
    VersionOld, VersionNew     string
}

type Finding struct {
    Code   string // synt empt stub secr hold impo dupl stru
    Passed bool
    Line   int    // 0 = file-level
    Detail string
}
```

The scraper produces `FileFacts` per touched file (from the binary's own `git diff`, not agent self-report) and one `ChangeFacts` for the run. Two consumers read the same facts:

- **The 8 checks** — absolute fails, always on (toggle in config): `synt` syntax (native Go parser for go/json/yaml; deterministic shell-out `py_compile`/`bash -n`/`node --check` otherwise), `empt` empty/zero-byte, `stub` pass-only bodies/empty except/imports-only, `secr` hardcoded-secret regex, `hold` TODO/FIXME (code files only, toggleable), `impo` unused imports, `dupl` identical-hash files, `stru` missing docstring/heading (language-specific).
- **The DOD thresholds** — project-specific floors/ceilings on the facts: `min_code_lines`, `max_function_lines`, `max_nesting`, `min_comment_ratio`, `contains: [...]`, `no_empty_bodies`, `no_todos`, changeset `max_duplicate_files`, `min_lines_added`.

Unknown language → `stru`/`impo`/`stub` return pass. Never fabricate a failure.

### Verdict — `internal/cage/verify.go`

Order: core checks → DOD criteria → quality officer. Any error → no commit, run the no-progress strike logic, print the full report (every failure, every file, at once). All pass → the binary commits, bumps audit, writes cooldown. Never partial.

### Worker loop — `internal/worker/agent.go`

Read the active DOD. Build a neutral prompt (no hardware/model/quality coaching — that's the cage's job, not the prompt's). Drive the backend through tools inside the jail, enforcing the context budget every turn (compression + caps; never inject brain.md or full history). When the model signals done, the binary runs `verify`. Fail → hand the model the full report, loop. Pass → binary commits. The model has no git access.

---

## 4. Command surface

| Command | Behavior |
|---|---|
| `cage init` | scaffold `.cage/` (config, state, dods/, reports/) + `.ai/` (brain.md, VERSION). Refuse if already initialized. |
| `cage run "<task>"` | jail, load+validate DOD (>=1 criterion or hard fail), run worker loop, verify, commit on pass. |
| `cage verify [--gate]` | run core+DOD+quality against the diff (local) or pushed tree (`--gate`). Exit 0 pass, 1 fail. |
| `cage peek <file>` | the 8 checks on one file, boxed output, preview only, no strike. |
| `cage session start\|end` | jail setup / archive report + reset strikes. No mem9. |
| `cage gate install\|uninstall <bare>` | write/remove the pre-receive hook. |
| `cage strikes [reset]` | show count/lock state; `reset` is human-only. |

---

## 5. Done means done (acceptance — satisfy `cobra-build.yaml`)

You are not done until, on babalou:

1. `go build -o cage .` succeeds with no errors.
2. `grep -ri "mem9\|c1a5fed9\|9090" internal cmd` returns nothing.
3. The `internal/cage` package imports nothing from `internal/backend` or `internal/worker` (no model reachable from the verifier — enforce by structure).
4. `cage init` in an empty dir scaffolds `.cage/` and `.ai/`.
5. `cage peek` on a file with a `TODO` reports the `hold` failure with a line number.
6. `cage verify` blocks when a DOD criterion fails and passes when all hold.
7. A run that fails verify twice with the same failures records a strike; a run that reduces failures does not.
8. `cage run` writes only inside the jail — `$HOME` is untouched after a run.

Build it, run it, prove each line. Update `.ai/brain.md` with what you did and bump `.ai/VERSION`. The proof is that the binary gated something real, not that the files exist.
