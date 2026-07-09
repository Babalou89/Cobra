# COBRA on Windows — quick start (for a normal human)

Goal: install a binary, point it at a local model, tell it what "done" means,
and watch it build the thing. No compiling, no Python trees.

You install **three** things, all one-click, then you're running.

## 1. A model server — Ollama

Download and run the installer from **ollama.com** (Windows). It handles your
GPU automatically. Then open a terminal (PowerShell) and pull a coding model:

```powershell
ollama pull qwen2.5-coder:14b
```

(On a 16 GB card use `:14b`; on 8–12 GB use `:7b`. The bigger `:32b` needs ~20 GB.)

Ollama now serves a model at `http://localhost:11434`. Leave it running.

## 2. Git for Windows

Install from **git-scm.com**. COBRA measures progress through git, so it needs it.
Nothing to configure — just install.

## 3. cage.exe

Drop `cage.exe` somewhere on your PATH (e.g. `C:\Users\you\bin\` added to PATH,
or just run it from its folder). Check it:

```powershell
cage.exe --help
```

## Wire it up

In the folder you want to work in:

```powershell
git init
cage.exe init
```

Open `.cage\config.yaml` and point it at Ollama:

```yaml
backend:
  type: llama_cpp
  params:
    base_url: "http://localhost:11434"
    model: "qwen2.5-coder:14b"
    max_context: "32768"
```

## Write what "done" means

Open `.cage\dods\example.yaml` and replace it with your real checks. If you've
never written a DOD, read **WRITING-A-DOD.md** first — the whole skill is
*gate the outcome, not the artifact*. A tiny real one:

```yaml
task: "a python script that adds two numbers from the command line"
name: "adder"
criteria:
  - name: "it runs and adds correctly"
    verify:
      - type: exists
        file: "add.py"
      - type: command
        run: "python add.py 2 3"
        expect_exit: 0
      - type: command
        run: "python -c \"import subprocess;assert subprocess.check_output(['python','add.py','2','3']).decode().strip()=='5'\""
        expect_exit: 0
```

## Run it

```powershell
cage.exe run "write add.py: takes two numbers as command-line args and prints their sum"
```

It builds until your checks pass, then commits — or tells you it couldn't.
Watch it live in a browser with `cage.exe watch` (opens a dashboard on
`http://localhost:8060`).

## If something's off

- **"backend returned no choices" / connection refused** — Ollama isn't running,
  or the model name in config doesn't match `ollama list`.
- **A `command` check fails on a missing tool** (python, ruff) — install that tool,
  or drop that check. COBRA skips *its own* language checks when a toolchain is
  absent, but a `command` you wrote runs literally.
- **Slow** — smaller model (`:7b`) or a smaller `max_context`.

That's the whole thing: Ollama + Git + cage.exe, a config pointed at your model,
a DOD that says what done means. If this felt hard, that's the feedback we need —
tell us exactly where it snagged.
