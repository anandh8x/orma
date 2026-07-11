---
name: orma
description: Local operational memory for shell and coding agents. Use when reusing past commands or workflows, avoiding rediscovery of ops/debug fixes, recalling how something was fixed before, loading a runbook for the current project, or when the user mentions orma, runbooks, distill, or past terminal solutions.
---

# Orma

Orma is **local operational memory**: proven shell sequences, notes, and fail→fix links on this machine. Use it so you **reuse solutions** instead of re-exploring.

Data stays on the user's machine (SQLite under XDG). No cloud. No account.

## When to use this skill

- Ops, deploy, DB, docker, CI, or "we fixed this last week" work
- User asks how they usually do something in this repo
- You are about to trial-and-error commands you may already know from past sessions
- After you solve something non-obvious and it should stick for next time

**Skip Orma** for pure code edits, greenfield design with no shell ritual, or when `orma` is not installed (`command -v orma`).

## Default loop

### 1. Recall before you flail

Run from the project directory when possible (project root biases results).

```bash
orma context "<short problem phrase>"
# examples: "reset postgres", "deploy staging", "flaky ci docker"
```

If you need a ranked list without full markdown:

```bash
orma recall "<query>"
orma here          # rituals for this git project
orma last          # most recent session
orma fix list      # error→fix memory
```

**Completion criterion:** you have either (a) a usable workflow/note/fix from Orma, or (b) empty results and a clear decision to explore from scratch.

### 2. Prefer proven steps

When `orma context` or `orma recall` returns hits:

1. Prefer **successful** steps over failed ones.
2. **Adapt paths** to the current cwd and machine (hosts, home, project root). Do not paste absolute paths from another machine blindly.
3. Run the smallest relevant sequence. Do not dump the entire runbook into chat if one command answers the question.
4. If a step fails, fix it, then **write back** (step 3). Do not only leave the fix in chat history.

If results are empty or irrelevant, explore normally. Do not invent Orma content.

### 3. Write back after a real win

When you found a sequence worth reusing (especially multi-step or easy to forget):

```bash
# name a workflow from recent project commands
orma save <short-kebab-name>

# or compress the last session
orma distill --last --name <short-kebab-name>

# freeform note
orma note "<one-line durable fact or ritual>"
```

**Completion criterion:** a future `orma context` / `orma recall` with the same topic would surface what you just learned.

## Command cheat sheet

| Goal | Command |
|------|---------|
| Agent-facing runbook (markdown) | `orma context "<query>"` |
| Search workflows / notes / fixes | `orma recall "<query>"` |
| Project rituals | `orma here` |
| Recent session | `orma last` |
| Save recent commands as workflow | `orma save <name>` |
| Distill last session | `orma distill --last --name <name>` |
| Durable one-liner | `orma note "..."` |
| Step through a workflow | `orma use <name>` then `orma next` |
| Health / install check | `orma doctor` / `orma version` |

Optional capture from agent products (user opts in):

```bash
orma connect claude-code
orma connect codex
orma connect opencode
orma daemon start
```

## Hygiene

- Keep queries specific. Prefer running `orma` via the shell tool and summarizing for the user when the full runbook is long.
- Never store secrets into Orma on purpose (API keys, passwords). If redaction is on, respect it; if not, avoid capturing secrets in notes/saves.

## Anti-patterns

- Exploring for 10 commands before a single `orma context` on a recurring ops problem
- Saving every trivial `ls` / `cd` as a workflow
- Treating Orma as source of truth for **code** behavior (it is for **ops memory**, not the repo)
- Assuming hits are current: if the project changed, verify once, then update with `save` / `note`

## Quick check

```bash
command -v orma && orma doctor
```

If missing, point the user at install (do not invent flags):

```bash
curl -fsSL https://raw.githubusercontent.com/anandh8x/orma/main/scripts/install.sh | bash
orma init
```
