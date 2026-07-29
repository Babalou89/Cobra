# COBRA PLANNER + POWER MANAGEMENT PLAN

**Date:** 2026-07-29
**Author:** Billy + Hermes
**Status:** Awaiting review — DO NOT IMPLEMENT until approved

---

## Part 1: Planner (Agent-Level Speculative Decoding)

### Problem

Qwen2.5-Coder-32B (cage worker) is a strong coder but a weak planner.
When cage verify fails, it sees the failure list but can't diagnose WHY.
Result: loops rewriting the same files for 50 turns. 10 attempts on
babaface, all FAIL.

### Solution

Small model (Qwen 32B) writes code. Large model (Claude Fable 5)
diagnoses failures. Deterministic cage is the ground truth.

```
Worker (Qwen 32B :8080)       → writes code, 50 turns, free
Cage (deterministic, no LLM)   → verify against DOD + quality
Planner (Claude Fable 5 :3002) → diagnose failures, write PLAN.md
```

Cost: ~$0.007 per failed attempt. 10 attempts = $0.07.

### Architecture

```
cmd/run.go main loop:

  for attempt := 1; ; attempt++ {
      agent.Run(task, dodText, report)     // existing
      res := cage.Verify(...)              // existing
      if res.Passed { commit; return }     // existing

      // NEW: planner step (between verify and strike logic)
      plan := planner.Diagnose(cfg, res.Failures, dir)
      if plan != "" {
          os.WriteFile(jailRoot+"/PLAN.md", plan)
          agent.PlanPath = jailRoot+"/PLAN.md"
      }

      // existing: strikes, report, next attempt
      report = res.Report()
  }
```

### New Package: `internal/planner/`

**planner.go** — orchestration (~80 lines)
- Diagnose(cfg, failures, dir) → string
- Extracts file paths from failure strings
- Reads those files + DOD text
- Calls Claude Fable 5 via OneProvider
- Returns plan text (or "" on any error)

**client.go** — Anthropic Messages API client (~60 lines)
- POST to `{BaseURL}/v1/messages`
- Headers: x-api-key, anthropic-version: 2023-06-01
- Timeout: 60s
- Graceful degradation on error

**prompt.go** — prompt building (~50 lines)
- System: "You are a code diagnostician..."
- User: failures + DOD + relevant source code
- Output: numbered action plan with exact fixes

### Config Changes

```yaml
# .cage/config.yaml
planner:
  enabled: true
  base_url: "https://api.oneprovider.dev"
  api_key: "sk-..."    # OneProvider key
  model: "claude-fable-5"
  max_tokens: 2000
```

### Worker Context Changes

- Agent struct: add PlanPath string
- buildUser(): inject PLAN.md alongside NOTES.md (clamped 1500 tokens)
- Worker sees: task + DOD + verify report + NOTES.md + PLAN.md + last action

### Files to Create/Modify

| File | Action | Lines |
|------|--------|-------|
| `internal/planner/planner.go` | CREATE | ~80 |
| `internal/planner/client.go` | CREATE | ~60 |
| `internal/planner/prompt.go` | CREATE | ~50 |
| `internal/config/config.go` | MODIFY | ~15 |
| `internal/worker/agent.go` | MODIFY | ~3 |
| `internal/worker/prompt.go` | MODIFY | ~10 |
| `cmd/run.go` | MODIFY | ~15 |

Total: ~230 lines.

---

## Part 2: Babalou Power Management

### Problem

babalou runs 24/7 with 2 GPUs (RTX 3090 + RTX 3060). At idle, GPUs
draw ~46W but hold models in VRAM even when nobody's using them.
Power cost adds up when the machine is sitting idle overnight.

### Hardware

- Board: ASUS ROG Crosshair VIII Dark Hero (X570, AM4)
- GPU 0: RTX 3090 (390W cap, 38W idle with models loaded)
- GPU 1: RTX 3060 (170W cap, 8W idle with models loaded)
- NIC (active): be2net Emulex BladeEngine 3 (10GbE SFP+ OM4 to BhondusXI)
  — NO WOL support
- NIC (inactive): igb Intel GbE (enp6s0, no cable) — WOL capable
- NIC (inactive): r8125 Realtek 2.5GbE (enp5s0, no cable)

### Solution: 3-Layer Power Management

#### Layer 1: Idle Model Unload (saves ~30-40W)

Watchdog service monitors llama-server request activity. After 30min
idle → stop llama-server → GPU VRAM freed → GPU drops to ~10W each.

On demand → restart llama-server (30s to reload models).

Implementation:
- Systemd service: `babalou-idle-watchdog.service`
- Monitors llama-server /metrics endpoint or log activity
- 30min idle threshold (configurable)
- Stops: llama-server, babaface server
- On-demand restart: triggered by incoming request to a small
  reverse-proxy that checks if llama-server is up, starts it if not

#### Layer 2: GPU Power Cap (saves ~40-50W peak)

Reduce GPU power limits. 3090 at 200W (vs 390W) is still fast for
inference. 3060 at 120W (vs 170W).

Implementation:
- `/etc/systemd/system/gpu-power-cap.service`
- Runs `nvidia-smi -i 0 -pl 200 && nvidia-smi -i 1 -pl 120`
- On boot, after nvidia driver loads

#### Layer 3: Wake-on-LAN (full shutdown recovery)

babalou's active NIC (be2net SFP+) does NOT support WOL. The Intel
igb NIC (enp6s0) does.

**Required hardware change:** Run a copper Ethernet cable from
BhondusXI to babalou's igb NIC (enp6s0). This is a secondary
connection for WOL only — all data still goes over the OM4 fiber.

Implementation:
- BIOS: Enable WOL on igb NIC (Advanced → Network → Wake on LAN)
- OS: `ethtool -s enp6s0 wol g` (enable magic packet wake)
- Systemd: `/etc/systemd/system/wol-enable.service` to set WOL on boot
- BhondusXI: `wol <babalou-igb-mac-addr>` to wake babalou
- systemd sleep target: `systemctl suspend` for sleep mode

**Alternative (no cable needed):** Smart plug + BIOS "Restore on AC
Power Loss = Power On". Kill plug from phone → restore → babalou boots.

### Idle Shutdown Flow

```
30min no activity
  → watchdog stops llama-server (Layer 1)
  → GPUs drop to ~10W each
  → 2hrs still idle
  → watchdog runs systemctl suspend (sleep, not shutdown)
  → babalou draws ~5W (RAM self-refresh)
  → UP board (SBC) sends WOL magic packet to igb NIC
  → babalou wakes, services restart
### Files to Create

| File | Purpose |
|------|---------|
| `scripts/idle-watchdog.sh` | Monitor activity, stop/start services |
| `scripts/gpu-power-cap.sh` | Set GPU power limits on boot |
| `scripts/wol-enable.sh` | Enable WOL on igb NIC |
| `scripts/wake-babalou.sh` | Run on BhondusXI to send WOL packet |
| `babalou-idle-watchdog.service` | Systemd service for idle monitoring |
| `gpu-power-cap.service` | Systemd service for GPU power limits |
| `wol-enable.service` | Systemd service for WOL enable |

### DOD Addition

Add to the babaface cage DOD or create a separate infra DOD:

```yaml
- name: "power management configured"
  verify:
    - type: command
      run: "nvidia-smi -q -d POWER | grep -q 'Power Limit.*200'"
      expect_exit: 0
    - type: exists
      file: "/etc/systemd/system/gpu-power-cap.service"
    - type: exists
      file: "/etc/systemd/system/babalou-idle-watchdog.service"
```

---

## Part 3: Cobra README Updates

Add to README.md architecture section:

```
internal/planner/  optional LLM planner: diagnoses verify failures,
                   writes PLAN.md for the worker (default off)
```

Add to backends section:

```
## Planner (optional)

The cage can optionally call a second, stronger model between attempts
to diagnose verify failures and write an actionable plan. The worker
reads PLAN.md next attempt. Default off; enable in config:

  planner:
    enabled: true
    base_url: "https://api.oneprovider.dev"
    api_key: "sk-..."
    model: "claude-fable-5"

The planner is never consulted during verification — the cage's verdict
is still 100% deterministic. The planner only reads failures and writes
advice. Cost: ~$0.007 per failed attempt.
```

Add to changelog:

```
**v1.5.0** — planner (optional)
- **Planner** (opt-in, `planner.enabled`): between attempts, a second
  model reads verify failures + relevant source code and writes PLAN.md
  with actionable fixes. The worker reads it next attempt. Inspired by
  DeepSeek's speculative decoding applied at the agent level. Uses
  Anthropic Messages API (OneProvider). Default off — zero behavior
  change unless enabled. Graceful degradation: if the API call fails,
  the cage runs without it.
```

---

## Implementation Order

1. **Part 2 Layer 2** — GPU power cap (5 min, immediate savings)
2. **Part 1** — Planner (2-3 hours, main feature)
3. **Part 2 Layer 1** — Idle watchdog (1 hour)
4. **Part 2 Layer 3** — WOL setup (needs cable run)
5. **Part 3** — README/docs updates
6. Push to `github.com/Babalou89/Cobra` main

## Risk Assessment

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Planner gives wrong diagnosis | Low (Fable tested) | Cage verify catches bad plans |
| API down / key expires | Medium | Graceful degradation |
| Model unload causes service interruption | Low | Auto-restart on demand |
| WOL not supported by igb NIC | Low (igb usually supports) | Smart plug fallback |
| GPU power cap slows inference | Very low | 200W is plenty for 3090 inference |

---

**Next step:** Billy reviews. If approved, implement in order listed.
Push to `github.com/Babalou89/Cobra` main branch.
