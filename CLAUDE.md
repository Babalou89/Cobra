# COBRA build session — token discipline

You are building the `cage` Go binary in THIS directory. Full spec: `COBRA-BUILD.md`. Acceptance: `cobra-build.yaml`.

## Read order (once, at session start — never re-read)
1. `COBRA-BUILD.md` — complete spec: layout, contracts, non-negotiables
2. `cobra-build.yaml` — the 14 acceptance criteria

Everything is specified. Zero exploration needed. Do NOT search the codebase, do NOT read `~/continual-agent/` internals, do NOT query mem9 (banned from this project entirely).

## Token rules
- Write each file ONCE, complete, from the spec. No skeleton-then-fill passes.
- Never read back a file you just wrote.
- Never print file contents to the terminal (no cat/head of your own output).
- Batch independent file writes; don't compile between every file.
- Cheap gates first, in order: `go build ./...` → `go vet ./...` → `go test ./...` → only then `cage verify`. The verify report is large; earn it.
- On verify failure: fix ALL listed failures in one pass, then re-verify. Never one-fix-one-verify.
- Keep chat output to one line per milestone. No summaries until done.

## The loop
```bash
# build here
go build -o cage .
# gate (run from continual-agent, --worktree flag is MANDATORY)
cd ~/continual-agent && cage verify --worktree /home/billy/cobra
```
Exit 0 = done. Nonzero = read report, fix everything listed, repeat. Verify's `cage` is the old Python cage on PATH — that is the gate. Your Go binary is `./cage` in this dir.

## Build order (dependency-sorted, minimizes rework)
1. `go.mod` (module cobra, go 1.21+; deps: github.com/spf13/cobra, gopkg.in/yaml.v3) + `main.go`
2. `internal/backend/` — backend.go (interface), registry.go, llamacpp.go, anthropic.go, openai.go — stdlib HTTP only, keys from env
3. `internal/config/`, `internal/gitx/`
4. `internal/jail/workspace.go` + `workspace_test.go`
5. `internal/state/` — strikes.go (fingerprint + no-progress logic) + strikes_test.go, cooldown.go, audit.go, session.go, report.go
6. `internal/cage/` — checkers.go (FileFacts + 8 checks: synt empt stub secr hold impo dupl stru), quality.go, dod.go, core.go, verify.go, pluggable.go — MUST NOT import backend or worker
7. `internal/ctxdiet/`, `internal/memory/`
8. `internal/worker/` — agent.go, tools*.go, prompt.go — no git calls anywhere in this package
9. `cmd/` — root, init, run, verify, peek, session, gate, strikes
10. `assets/` (go:embed), `hooks/`, `tests/fixtures/pass.dod.yaml` + `fail.dod.yaml`
11. `README.md` — full: ≥80 lines with `## Install`, `## Usage` (or Quick Start), `## Architecture` sections; must mention the cage commands, backends, and DOD. The gate greps for all of these.
12. `.ai/brain.md` (≥10 lines, mention COBRA) + bump `.ai/VERSION`

## After verify passes (mandatory final step)
```bash
git add -A && git commit -m "COBRA v1: cage binary, verified against dod-v5"
git push -u origin main
```
Remote `origin` = git@github.com:Babalou89/cobra.git (already configured, branch = main). If push fails with "Repository not found", STOP and tell the user to create the empty repo at github.com/new — do not retry or change the remote.

## Hard constraints (verify greps for these — violations = guaranteed fail)
- Strings `mem9`, `c1a5fed9`, `:9090` must appear NOWHERE in `internal/` or `cmd/`
- `internal/cage/` must not contain the strings `cobra/internal/backend` or `cobra/internal/worker`
- `internal/worker/` must not match regex `git.+commit`
- No hardcoded API keys in `internal/backend/`
- `cage verify --dod <file>` flag must exist (fixtures criterion uses it)
- `cage peek` output must contain `hold` + a line number when a TODO exists

## Environment
- Go 1.22.2 at `go`. This dir is a git repo. Gate DOD = dod-v5.yaml, already active.
- Do not modify `COBRA-BUILD.md`, `cobra-build.yaml`, or anything under `~/continual-agent/`.
