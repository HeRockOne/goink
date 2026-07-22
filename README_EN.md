<p align="center">
  <img src="assets/logo-dark.svg#gh-dark-mode-only" alt="Goink" />
  <img src="assets/logo-light.svg#gh-light-mode-only" alt="Goink" />
</p>

<h1 align="center">Goink<br><sub>Desktop AI Novel-Writing System — Agent Decisions × Structured Memory × Self-Check</sub></h1>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go 1.25" />
  <img src="https://img.shields.io/badge/Wails-v2.12-DF0000?style=for-the-badge&logo=wails&logoColor=white" alt="Wails v2" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=for-the-badge&logo=react&logoColor=white" alt="React 19" />
  <img src="https://img.shields.io/badge/SQLite-3-003B57?style=for-the-badge&logo=sqlite&logoColor=white" alt="SQLite" />
  <br />
  <img src="https://img.shields.io/badge/TypeScript-6.0-3178C6?style=for-the-badge&logo=typescript&logoColor=white" alt="TypeScript 6" />
  <img src="https://img.shields.io/badge/Tailwind-4.3-06B6D4?style=for-the-badge&logo=tailwindcss&logoColor=white" alt="Tailwind 4" />
  <img src="https://img.shields.io/badge/ONNX_Runtime-1.26-005BED?style=for-the-badge&logo=onnx&logoColor=white" alt="ONNX Runtime" />
  <img src="https://img.shields.io/badge/license-AGPL_v3-blue?style=for-the-badge&logo=opensourceinitiative&logoColor=white" alt="AGPL v3" />
</p>

<p align="center"><strong><a href="README.md">中文</a> | English</strong></p>

---

> **Forked from [sigpanic/goink](https://github.com/sigpanic/goink) with major additions: mobile web frontend, HTTP API, token auth, phase gate, data backup, and more.**

---

## Table of Contents

- [New Features](#new-features)
- [Project Structure](#project-structure)
- [Core Capabilities](#core-capabilities)
- [Installation](#installation)
- [Tech Stack](#tech-stack)
- [License](#license)

---

## New Features

### Mobile Web Frontend

Access from phone browser at `http://{LAN_IP}:8877/mobile/`:

| Module | Features |
|--------|----------|
| Bookshelf | Novel list, word count, current book indicator |
| Novel Detail | Chapters, characters, timeline, arcs, reader, preferences, locations |
| Reader | Font/line-height/background settings, swipe navigation, chapter list, progress memory |
| AI Chat | Streaming SSE, thinking process, session history, model switching |
| Settings | Dark/light mode, i18n, token management, model selection |

### HTTP API

Full RESTful API. See [mobile/API.md](mobile/API.md):

```
GET  /api/novels              Novel list
GET  /api/novels/{id}/chapters  Chapter list
POST /api/chat                AI chat (SSE)
GET  /api/settings/model      Model settings
...  17 endpoints total
```

### API Token Authentication

All endpoints (except health check) require Bearer Token:

```
Authorization: Bearer <token>
```

- View/reset in desktop Settings → API Auth Token
- Mobile: prompt on first connection
- WebSocket: pass via `?token=<token>`

### Phase Gate

Enforced stage-based writing workflow. See [docs/phase-gate.md](docs/phase-gate.md):

```
prepare → outline → write → review → maintain → prepare
```

- Tool whitelist + completion conditions per stage
- Toggle in settings
- Works for API conversations too

### Other Additions

| Feature | Description |
|---------|-------------|
| Word Count Config | Custom min/max chapter word limits |
| WebDAV | Built-in server for phone file manager access |
| Model Management | Auto-fetch from model.dev, thinking mode support |
| Data Backup | One-click full backup/restore |
| Session Management | Delete history sessions |
| Log Toggle | Enable/disable file logging |
| Chat UI | Message bubbles, Markdown rendering, copy button |
| i18n | Chinese/English interface switching |

---

## Project Structure

```
goink/
├── app/                    # Wails binding layer
│   ├── api_server.go       #   HTTP API + Token auth
│   ├── chat.go             #   Chat entry
│   ├── settings.go         #   Settings + API Token
│   ├── backup.go           #   Data backup/restore
│   └── ...
├── internal/
│   ├── agent/              # LLM conversation loop, sub-agents
│   ├── mcp_tools/          # 30+ MCP tools
│   ├── llm/                # Multi-provider LLM transport
│   ├── session/            # Sessions + messages
│   ├── character/          # Characters + directed graph
│   ├── timeline/           # Foreshadowing + chapter plans
│   ├── storyarc/           # Story arcs
│   ├── reader/             # Reader perspectives
│   ├── location/           # Location graph
│   ├── rag/                # Vector search (ONNX)
│   ├── ws/                 # WebSocket sync
│   ├── webdav/             # WebDAV server
│   └── ...
├── mobile/                 # Mobile web frontend (pure JS)
│   ├── index.html          #   Entry
│   ├── app.js              #   Main logic
│   ├── style.css           #   Styles
│   └── API.md              #   API docs
├── frontend/               # Desktop (React + TypeScript)
├── docs/                   # Documentation
├── skills/                 # Built-in writing Skills
├── build.ps1               # One-click build/deploy
└── build.bat               # One-click build/deploy
```

---

## Core Capabilities

### Agent Decision Making

31 structured tools. LLM autonomously decides which to call. Not a pipeline—Agent checks characters, foreshadowing, reads/writes content, and updates state within the current conversation.

Auto-injects maintenance checks after writing: character changes, foreshadowing resolution, arc progression, reader knowledge. Optional Review Sub-Agent for independent audit.

### Local Semantic Search

BGE Chinese model via ONNX + sqlite-vec vector index. Find passages that hint at "the pendant" without mentioning the word. No network required.

### Structured Creative State

| Module | Capability |
|--------|------------|
| Characters | Directed relationship graph with history |
| Foreshadowing | Target chapter + importance + anomaly alerts |
| Arcs | Node chains, auto-advance on chapter completion |
| Locations | Hierarchy + spatial connectivity graph |
| Reader | Known/suspense/misconception tracking |
| Preferences | Global + per-novel two-tier management |

### Skill System

3 layers (Novel > User > Built-in) × 3 modes (auto/manual/always) = 9 strategies. `.md` file = new Skill. Zero-code extensibility.

### Style Distillation

Paste sample text, AI extracts style across 6 dimensions and generates an imitation Skill. Load with `/stylename`.

### Diff Approval

Every edit generates a Diff preview. Approve before writing. Full Git history for rollback.

---

## Installation

Download from [Releases](https://github.com/sigpanic/goink/releases):

- **Windows** — Run installer
- **macOS** — Open DMG, drag to Applications
- **Linux** — Run AppImage

Requires LLM API Key (OpenAI-compatible). Installer < 60MB. No Python/Node.js/GPU needed.

### Build From Source

```bash
# Linux dependencies
sudo apt install libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc

git clone https://github.com/sigpanic/goink
cd goink
make deps
make build   # production
make dev     # dev mode
```

### One-Click Build (Windows)

```powershell
.\build.ps1    # PowerShell
build.bat      # CMD
```

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| Agent Engine | ReAct loop (Go, SSE + 31 tools + sub-agents) |
| Desktop | Wails v2 (Go + WebView) |
| Frontend | React 19 + TypeScript + Tailwind 4 + shadcn/ui |
| Mobile | HTTP API + pure JS web frontend |
| Database | SQLite + GORM |
| Vector Search | sqlite-vec + ONNX Runtime |
| Version Control | Built-in Git |
| Security | Dual sandbox + approval flow + API Token auth |
| LAN | WebDAV server |

---

## License

AGPL-3.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
