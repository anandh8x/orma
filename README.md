<h1 align="center">Orma</h1>

<p align="center"><strong>Local operational memory for your terminal and coding agents.</strong></p>

*orma* means *memory* (Malayalam). Agents try things. Orma remembers what worked.

Orma runs entirely on your machine. It turns shell activity and agent tool runs into searchable workflows, project rituals, and error→fix links. When you need a past solution again, it rewrites paths for your current directory and drops the right command into your prompt.

> **Early stage.** This is a young project (v0.1). Expect rough edges, incomplete polish, and possible breaking changes as things settle. Feedback and issues are welcome.

---

## What it does

| You… | Orma… |
|------|--------|
| Run commands in zsh, bash, or fish | Captures them with cwd, exit code, and project root |
| Debug over many steps | Groups work into **sessions** and can **distill** them into workflows |
| Hit the same failure again | Links **fixes** from fail → success paths |
| Search in plain language | Uses **FTS + MiniLM ONNX** embeddings (local) |
| Reuse an old sequence | **Adapts** paths/aliases and step-through inserts (or `--run`) |
| Use Claude Code / Codex / OpenCode | Optional adapters + daemon file watch |

**Not** another smarter Ctrl-R only. History tools list lines. Orma stores *solutions*.

---

## Install

**Requirements:** Linux or macOS. Go is optional if you use a release binary.

### Install script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/anandh8x/orma/main/scripts/install.sh | bash
```

Installs to `~/.local/bin/orma`. Pin a version with `ORMA_VERSION=v0.1.0`.

### From source

```bash
git clone https://github.com/anandh8x/orma.git
cd orma
make install
```

Or:

```bash
go install github.com/anandh8x/orma/cmd/orma@latest
```
---

## Quick start

```bash
orma init
# creates config + SQLite DB
# downloads MiniLM ONNX + onnxruntime (checksum verified)
# optional shell-history import prompt

# add to ~/.zshrc  (or bash / fish)
eval "$(orma hook zsh)"
# eval "$(orma hook bash)"
# orma hook fish | source

orma doctor
```

Everyday flow:

```bash
# after you solve something
orma save boot
orma distill --last --name fix-db

# later
orma recall docker
orma recall --pick          # TUI picker (Ctrl-G in shell if hooked)
orma use boot               # preview + step 1
orma next                   # next step after you run the previous
orma use boot --run         # type "yes" to execute all steps

orma here                   # rituals for this git project
orma last                   # most recent session
orma context "reset postgres"   # markdown runbook for agents
```

---

## Coding agents

Orma stores proven shell sequences so agents (and you) can **reuse** what already worked instead of rediscovering it every session.

**Agent loop:**

```bash
# 1. Before exploring an ops/debug problem, load local memory
orma context "reset postgres"
orma recall docker

# 2. Prefer those steps (adapt paths to the current cwd). If empty, explore.

# 3. After you solve it, write it back so the next agent (or you) can reuse it
orma save fix-db-reset
orma distill --last --name fix-db-reset
orma note "staging: always migrate before restart"
```

**Wire agents into capture (optional):**

```bash
orma connect claude-code    # hook snippet + backfill ~/.claude
orma connect codex
orma connect opencode
orma daemon start           # watch agent logs + embed queue
```

**Agent skill** (when/how to call Orma):

```bash
mkdir -p ~/.agents/skills/orma
cp skills/orma/SKILL.md ~/.agents/skills/orma/SKILL.md
```

See [`skills/orma/SKILL.md`](skills/orma/SKILL.md).

---

## How it works

```text
shell / agents
    → orma ingest (events)
    → sessions
    → workflows / notes / pins / fixes
    → FTS + MiniLM vectors
    → recall → adapt → use
```

- **Store:** SQLite under XDG data dir (`~/.local/share/orma` on Linux; Application Support on macOS)
- **Config:** `~/.config/orma/config.toml` (or platform equivalent)
- **Embeddings:** `all-MiniLM-L6-v2` quantized ONNX from Hugging Face + Microsoft ONNX Runtime; hash embedder if download/runtime fails
- **Privacy:** local by default; no account; redaction optional (`redact = true` in config)

---

## CLI reference (main commands)

| Command | Purpose |
|---------|---------|
| `init` | Config, DB, model download, optional history import |
| `hook zsh\|bash\|fish` | Print shell integration (embeds absolute binary path) |
| `import history\|atuin` | Import shell histfile or Atuin DB |
| `save` / `note` / `pin` / `here` | Intentional memory and project rituals |
| `sessions` / `last` | Browse sessions |
| `distill` | Compress a session into a workflow |
| `recall` | Search workflows, notes, fixes |
| `use` / `next` | Preview, step-through, optional `--run` |
| `context` | Agent-facing markdown runbook |
| `fix list\|show` | Error→fix memory |
| `connect` | Claude Code / Codex / OpenCode |
| `daemon` | Background watch + embed worker |
| `embed ensure\|sync\|status\|update` | Model + embedding queue |
| `purge` | Delete local data categories |
| `doctor` | Health check |

---

## Configuration snippets

```toml
# ~/.config/orma/config.toml

redact = false
session_idle = "20m"
keybind = "ctrl-g"

# rewrite hosts/paths when using workflows
[aliases]
"192.168.1.10" = "10.0.0.5"
"/old/project" = "/home/you/project"
```

---

## Embeddings (ONNX)

On first `init` / `embed ensure`, Orma downloads:

1. **Model:** [Xenova/all-MiniLM-L6-v2](https://huggingface.co/Xenova/all-MiniLM-L6-v2) `onnx/model_quantized.onnx` (~23MB, sha256 verified)
2. **Vocab:** sentence-transformers MiniLM `vocab.txt`
3. **Runtime:** ONNX Runtime 1.27.1 shared library (Linux/mac, common arches)

Stored under `.../orma/models/`. After that, embedding is offline.

```bash
orma embed status
orma embed update   # re-fetch model
orma embed sync     # drain embed queue now
```

---

## Development

```bash
go test ./...
make build
make install
```

Version string in source is always **`dev`**. Stamp real versions only at release:

```bash
go build -ldflags "-X github.com/anandh8x/orma/internal/cli.version=v0.1.0" -o orma ./cmd/orma
```

Module: `github.com/anandh8x/orma`  
License: MIT
