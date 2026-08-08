# CLAUDE.md — Cobra Agent Guide

## Tech Stack & Commands

- **MCP Skills**: Codebase MCP enabled — prefer native MCP tools over raw grep for structural queries.
- **Go 1.21+** (`go.mod`) | Build: `go build -o cage .` | Test: `go test ./...`
- Gate: `cd ~/continual-agent && cage verify --worktree /home/billy/cobra`

## Token Rules & Constraints

- Write files ONCE, complete. No skeletons. Never read back just-written files or print file content to terminal.
- Batch writes. Cheap gates first: `go build ./...` → `go vet ./...` → `go test ./...` → `cage verify`.
- Fix ALL verify failures in one pass. Keep terminal output to one line per milestone.
- **Banned Strings in `internal/` or `cmd/`**: `mem9`, `c1a5fed9`, `:9090`.
- **Imports**: `internal/cage/` MUST NOT import `backend` or `worker`. `internal/worker/` MUST NOT call git commit.
- **Bash Logs**: Always pipe test/build commands to show only errors/failures (e.g., grep or head) to conserve context budget.

