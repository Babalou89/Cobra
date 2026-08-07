# CHANGELOG v1.8.0

## Production Stability Release — Cobra Defense Hardening

### Problem
v1.7.x cage-ralph architecture changes caused Qwen to get stuck in file_read loops.
7/7 E2E attempts failed on dedup-tool DOD. Root cause was NOT the model — it was
Cobra's infrastructure changes that broke the context system, tool surface, and
loop detection.

### Changes

#### 1. RalphRegistry — 4 tools instead of 11
- **Before:** file_read, file_write, file_edit, list_dir, shell, code_execute,
  web_fetch, web_search, system_info, note, skill_run (11 tools)
- **After (ralph mode):** file_read, file_write, note, code_execute (4 tools)
- Removed exploratory tools that caused Qwen to spend turns listing dirs,
  searching the web, and running system_info instead of writing code
- file_edit removed (Qwen can't match exact strings — 17/19 failures)
- shell removed (code_execute covers it)
- list_dir, web_*, system_info, skill_run all removed

#### 2. DOD criteria injected into model context
- Model now sees a checklist of all DOD criteria with PASS/FAIL/unknown status
- First attempt: all criteria show `[?]` (unknown status)
- Subsequent attempts: `[PASS]` or `[FAIL]` based on verify report
- Model knows exactly what to build and what's still broken

#### 3. Sliding window loop detector
- **Before:** Catches 3 consecutive identical actions (alternating reads fly through)
- **After:** Examines last 6 actions, aborts if file_read appears 4+ times
- Catches Qwen's exact failure pattern: alternating file_read(dedup.py) /
  file_read(test_dedup.py) that was invisible to the old detector

#### 4. file_edit blocked in ralph mode
- Blocked at dispatch level (not just prompt)
- Returns actionable error: "file_edit disabled — use file_write instead"
- Model can recover by switching to file_write

#### 5. file_read hard cap
- Caps total file_read calls at max_turns/3
- Prevents model from burning all turns reading files

#### 6. Ralph trim bumped from 2 to 8 steps
- **Before:** Only last 1 exchange pair (2 steps) — model amnesia every turn
- **After:** Last 4 exchange pairs (8 steps) — enough to see read patterns
- This was the primary cause of the file_read loop — model forgot what it read

#### 7. Planner summary injected into directive
- PLAN.md contents injected into FixTarget on fix attempts
- Model sees the planner's diagnosis alongside the failures

#### 8. Prompt aligned with tool surface
- "file_write is your ONLY tool for creating or modifying files"
- "NEVER use file_edit. It is disabled."
- "When you see file contents in a fix target, write the entire corrected file"

#### 9. --jinja on llama-server (infra, not code)
- Added to llama-main.service systemd unit
- Uses Qwen's embedded Jinja2 chat template for proper tool-call formatting

#### 10. max_gen_tokens bumped from 8192 to 12288
- Prevents mid-generation truncation

### Test Results
- `go test ./... -count=3`: ALL PASS (7 packages, 0 failures)
- 10 new defense tests in cobra_defense_test.go
- 3 updated directive tests for DOD injection
- All tests deterministic (fake backend, no LLM required)

### Breaking Changes
- Ralph mode now uses RalphRegistry (4 tools) instead of DefaultRegistry (11 tools)
- Conversational mode still uses DefaultRegistry (unchanged)
- DOD criteria now appear in FixTarget on all turns (not just failure turns)
