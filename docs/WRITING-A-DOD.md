# Writing a DOD — for humans

A **DOD** (Definition of Done) is a checklist a machine can verify. You write it.
The cage runs it. If every item passes, the cage says "done" and commits the work.
If not, it makes the model keep going. That's the whole deal.

## The one thing to understand

**The cage has zero judgment.** It does not know what you *meant*. It cannot tell
"good code" from "bad code." It only runs the checks you wrote and sees pass or fail.

So a DOD is you, translating *"what I actually want"* into *"checks a computer can
run without an opinion."* The quality of the result is the quality of your checks.
Garbage checks → garbage that technically passes. Sharp checks → the real thing.

## The golden rule

**Gate the outcome, not the artifact.**

Don't check "a file called `app.py` exists." A file existing proves nothing works.
Check "**running** `app.py` does the thing I wanted." Ask yourself:

> *How would I prove to a skeptic this is actually done — using only commands that
> either pass (exit 0) or fail?*

If a lazy or wrong solution could still pass your DOD, the DOD is too weak. Tighten it.

## Where the DOD goes

Put it in your project at `.cage/dods/<name>.yaml`. Then either:

- point your `.cage/config.yaml` at it: `dod: ".cage/dods/<name>.yaml"`, or
- run it directly: `cage run "your task" --dod .cage/dods/<name>.yaml`

Check a DOD without doing a run: `cage verify --dod .cage/dods/<name>.yaml`
(exit 0 = every check passes).

## The only 4 building blocks

Every criterion is a list of checks. These are the check `type`s:

| type | what it asks | example |
|---|---|---|
| `exists` | is there a file here? | `{type: exists, file: "README.md"}` |
| `lines` | does a file have at least / at most N lines? | `{type: lines, file: "README.md", min: 5}` |
| `grep` | does a file contain this text / pattern? | `{type: grep, file: "app.py", contains: "def main"}` |
| `command` | **run any shell command — pass if it exits 0** | `{type: command, run: "python app.py --selftest", expect_exit: 0}` |

`command` is the powerful one. It's how you *run the program*, *run the tests*, *lint
the code* — anything that ends in success-or-failure. Most of a good DOD is `command`.

## Bad DOD vs good DOD (same task)

Task: "make a greeting script."

**Weak (gates the artifact):**
```yaml
criteria:
  - name: "the script exists"
    verify:
      - type: exists
        file: "greet.py"
```
A file full of nonsense passes this. Useless.

**Strong (gates the outcome):**
```yaml
criteria:
  - name: "greet.py greets by name, proven by running it"
    verify:
      - type: exists
        file: "greet.py"
      - type: command
        run: "python3 -c \"import subprocess; o=subprocess.check_output(['python3','greet.py','Billy']).decode(); assert 'Hello, Billy' in o, o\""
        expect_exit: 0
```
The only way to pass is to actually build a script that actually greets. No bullshit.

## A template to copy

```yaml
task: "one line: what you're asking for"
name: "short-name"
description: "a sentence or two so future-you remembers the intent"

criteria:
  - name: "it exists and runs"
    verify:
      - type: exists
        file: "<the main file>"
      - type: command
        run: "<the command that runs your thing>"
        expect_exit: 0

  - name: "it does the right thing"
    verify:
      - type: command
        run: "<a command that checks the actual result — a test, an assertion>"
        expect_exit: 0

  - name: "it's clean"
    verify:
      - type: command
        run: "if command -v ruff >/dev/null; then ruff check .; else echo skip; fi"
        expect_exit: 0
```

A DOD with **zero** criteria is rejected — an empty contract gates nothing.

## Before you save it — the checklist

1. Does **every** criterion prove the thing *works*, or just that *code exists*? Kill
   the ones that only prove existence unless a run-check backs them up.
2. Could a wrong/lazy answer still pass? If yes, add a `command` that would catch it.
3. Can a machine run **every** check with **no** human judgment? If a check needs *you*
   to eyeball it, it doesn't belong in a DOD.
4. Is there at least one check that actually **runs** the thing (not just reads it)?

## The three mistakes everyone makes first

1. **Gating existence instead of behavior** — `exists app.py` instead of `run app.py`.
2. **Leaving placeholders in** — a `run: "true"` you meant to replace. It always passes
   and gates nothing.
3. **No "it runs" check** — checking the code looks right but never proving it executes.

That's it. Write what "done" means as pass/fail checks, gate behavior not files, and the
cage does the rest — honestly, every time.
