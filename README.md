# Orma

**Local operational memory for your terminal and coding agents.**

*orma* means *memory* (Malayalam). Agents try things. Orma remembers what worked.

Orma runs entirely on your machine. It turns shell activity and agent tool runs into searchable workflows, project rituals, and error→fix links. When you need a past solution again, it rewrites paths for your current directory and drops the right command into your prompt.

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

**Requirements:** Go 1.22+, Linux or macOS.

```bash
git clone https://github.com/anandh8x/orma.git
cd orma
make install
# installs to ~/.local/bin/orma  (ensure that is on PATH)
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
orma context "reset postgres"   # markdown for pasting into an agent
```

Agents (optional):

```bash
orma connect claude-code    # hook snippet + backfill ~/.claude
orma connect codex
orma connect opencode
orma daemon start           # watch agent logs + embed queue
```

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
