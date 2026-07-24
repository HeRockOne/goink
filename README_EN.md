<h1 align="center">Goink<br><sub>Desktop AI Novel-Writing System — Agent Decisions × Structured Memory × Self-Check</sub></h1>

<p align="center"><strong><a href="README.md">中文</a> | English</strong></p>

---

> **Forked from [sigpanic/goink](https://github.com/sigpanic/goink) with additions: mobile web frontend, HTTP API, token auth, phase gate, data backup, vintage UI theme, QR code connection, offline storage, auto HTTPS, and more.**
>
> **About this project:** Fully developed using [MiMoCode](https://github.com/nicepkg/Aide) + [MiMo-V2.5](https://huggingface.co/XiaomiMiMo/MiMo-V2.5) (Xiaomi's free open-source LLM), with AI handling all code changes. I'm not a programmer, have never systematically learned any code, and this is my first time forking a repo for practice. I only provide feature requirements based on my actual writing needs. I have no code review ability and no time/energy to handle bug reports. If there are issues, please fork and fix them yourself.

---

## Table of Contents

- [New Features](#new-features)
- [Installation](#installation)
- [Core Capabilities](#core-capabilities)
- [Project Structure](#project-structure)
- [Theme Colors](#theme-colors)
- [Icon Replacement](#icon-replacement)
- [Build From Source](#build-from-source)
- [Tech Stack](#tech-stack)
- [License](#license)

---

## New Features

### Mobile Web Frontend

Access from phone browser at `https://{LAN_IP}:8877/mobile/`:

| Module | Features |
|--------|----------|
| Bookshelf | Novel list, word count, current book indicator |
| Novel Detail | Chapters, characters, timeline, arcs, reader, preferences, locations |
| Reader | Font/line-height/background settings, swipe navigation, chapter list, progress memory |
| AI Chat | Streaming SSE, thinking process, session history, model switching |
| Settings | Dark/light mode, i18n, token management, model selection |

> First connection supports QR code scan or manual token input. Offline data cached via IndexedDB.

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
- Mobile: QR code scan or manual input on first connection
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
| Data Backup | One-click full backup/restore (DB, novels, user skills) |
| Session Management | Delete history sessions |
| Log Toggle | Enable/disable file logging |
| Chat UI | Message bubbles, Markdown rendering, copy button |
| Vintage Theme | Parchment-style light/dark modes + Monaco Diff editor theme |
| Bidirectional Sync | Desktop and mobile WebSocket full-duplex chat sync |
| QR Code Connection | Desktop shows token QR, mobile scans to connect |
| Offline Storage | IndexedDB cache for reading novels offline |
| Auto HTTPS | Generates certificate on startup, mobile camera works |
| Provider Display | Model selection shows `provider / model-name` format |
| Port Conflict Handling | Auto-kill processes occupying port on startup |
| Skill Token Optimization | 17 skills compressed 84%, tool descriptions with token hints |
| Sub-agent Event Sync | Sub-agent (review/memory) events synced to mobile |
| Config.json Removed | Data dir uses exe location directly, no config.json needed |

---

## Installation

Download from [Releases](https://github.com/HeRockOne/goink/releases).

### Runtime Dependencies

| Dependency | Notes |
|------------|-------|
| WebView2 Runtime | Built into Windows 11; Windows 10 needs install, auto-downloaded on first run |
| LLM API Key | OpenAI-compatible (DeepSeek, OpenAI, Claude, etc.) |

Program bundles Git and ONNX Runtime (vector search), no extra install needed.

### Build From Source

#### Build Requirements

| Component | Version | Notes |
|-----------|---------|-------|
| Go | 1.21+ | Backend compilation |
| Node.js | 18+ | Frontend build |
| MSYS2 | - | Windows only, provides gcc + headers |
| Wails CLI | v2.13+ | Framework build tool |

#### Windows Build Environment (Required)

The project uses CGO (SQLite, sqlite-vec vector search). On Windows, MSYS2 is required for gcc and POSIX headers. **Do not use TDM-GCC** — it lacks `sqlite3.h` and other headers needed for compilation.

```powershell
# 1. Install MSYS2 (https://www.msys2.org), default path C:\msys64

# 2. Open MSYS2 terminal, install toolchain
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-pkgconf

# 3. Add MSYS2 to system PATH
# C:\msys64\mingw64\bin
```

#### Build Steps

```bash
# Clone
git clone https://github.com/HeRockOne/goink.git
cd goink

# Linux dependencies (Ubuntu/Debian)
sudo apt install libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc

# Build
make deps
make build   # production
make dev     # dev mode
```

#### Windows One-Click Build

```powershell
.\build.ps1    # PowerShell
build.bat      # CMD
```

---

## Core Capabilities

### Agent Decision Making

31 structured tools. LLM autonomously decides which to call. Not a pipeline — Agent checks characters, foreshadowing, reads/writes content, and updates state within the current conversation.

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

3 layers (Novel > User > Built-in) x 3 modes (auto/manual/always) = 9 strategies. `.md` file = new Skill. Zero-code extensibility.

### Style Distillation

Paste sample text, AI extracts style across 6 dimensions and generates an imitation Skill. Load with `/stylename`.

### Diff Approval

Every edit generates a Diff preview. Approve before writing. Full Git history for rollback.

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
│   ├── mcp_tools/          # 31 MCP tools (with token hints)
│   ├── llm/                # Multi-provider LLM transport
│   ├── session/            # Sessions + messages
│   ├── character/          # Characters + directed graph
│   ├── timeline/           # Foreshadowing + chapter plans
│   ├── storyarc/           # Story arcs
│   ├── reader/             # Reader perspectives
│   ├── location/           # Location graph
│   ├── rag/                # Vector search (ONNX)
│   ├── ws/                 # WebSocket sync (wspulse)
│   ├── cert/               # Auto HTTPS certificate generation
│   ├── webdav/             # WebDAV server
│   └── ...
├── mobile/                 # Mobile web frontend (pure JS + wspulse)
│   ├── index.html          #   Entry
│   ├── app.js              #   Main logic + offline storage
│   ├── style.css           #   Styles
│   ├── jsQR.js             #   QR code scanner
│   ├── wspulse.mjs         #   WebSocket client
│   └── API.md              #   API docs
├── frontend/               # Desktop (React + TypeScript)
├── docs/                   # Documentation
├── skills/                 # Built-in writing Skills (token-optimized)
├── build.ps1               # One-click build/deploy
└── build.bat               # One-click build/deploy
```

---

## Theme Colors

Vintage parchment style with light/dark modes. Colors defined in `frontend/src/index.css`.

### Color Variables → File Locations

| Variable | Purpose | File |
|----------|---------|------|
| `--background` | Overall page background | `index.css` |
| `--foreground` | Global text color | `index.css` |
| `--card` | Card/panel background | `index.css` |
| `--primary` | Primary accent (buttons, links) | `index.css` |
| `--sidebar` | Sidebar background | `index.css` |
| `--border` | Border color | `index.css` |
| `--muted` | Secondary background (inputs, code blocks) | `index.css` |
| `--reader-bg` | Reading area outer background | `index.css` + `ContentPanel.css` |
| `--reader-paper` | Reading content paper background | `index.css` + `ContentPanel.css` |
| `--bubble-user` | User message bubble | `index.css` |

### Area Mapping

| Area | Light Mode | Dark Mode |
|------|------------|-----------|
| Page background | `--background` `#f5edd6` | `--background` `#1a1210` |
| Sidebar | `--sidebar` `#ede5ce` | `--sidebar` `#1e1612` |
| Reading content | `--reader-paper` `#faf4e4` | `--reader-paper` `#2a1e16` |
| Cards/popups | `--card` `#faf4e4` | `--card` `#2a1e16` |
| Inputs/code blocks | `--muted` `#ebe3cc` | `--muted` `#2e221a` |

### How to Modify Colors

1. Edit `frontend/src/index.css` in `:root` (light) or `[data-theme="dark"]` (dark)
2. Reading area also controlled by `frontend/src/components/content/ContentPanel.css`
3. Run `npm run build` to verify no errors
4. Run `.\build.ps1` to rebuild and deploy

---

## Icon Replacement

Multiple icon locations in the project. Replace and rebuild to apply:

| Location | Purpose | Format |
|----------|---------|--------|
| `build/windows/icon.ico` | exe icon + window title bar icon | ICO (multi-size) |
| `appicon.png` | Wails build app icon | PNG |
| `frontend/public/logo.svg` | Title bar top-left Logo | SVG |
| `frontend/public/favicon.svg` | Browser tab icon | SVG |
| `assets/logo.svg` | Logo source file | SVG |

### Replacement Steps

1. Prepare new icon (SVG or high-res PNG recommended)
2. Replace corresponding files:
   - **exe icon**: Convert PNG to ICO using online tools (e.g. convertio.co/png-ico), replace `build/windows/icon.ico`
   - **App icon**: Place PNG in project root as `appicon.png`, also copy to `build/appicon.png`
   - **Title bar Logo**: Place SVG at `frontend/public/logo.svg`
   - **Favicon**: Place SVG at `frontend/public/favicon.svg`
3. Run `.\build.ps1` to rebuild
4. If exe icon doesn't update, clear Windows icon cache or restart PC

---

## Build From Source

### Build Requirements

| Component | Version | Notes |
|-----------|---------|-------|
| Go | 1.21+ | Backend compilation |
| Node.js | 18+ | Frontend build |
| MSYS2 | - | Windows only, provides gcc + headers |
| Wails CLI | v2.13+ | Framework build tool |

### Windows Build Environment (Required)

The project uses CGO (SQLite, sqlite-vec vector search). On Windows, MSYS2 is required for gcc and POSIX headers. **Do not use TDM-GCC** — it lacks `sqlite3.h` and other headers needed for compilation.

```powershell
# 1. Install MSYS2 (https://www.msys2.org), default path C:\msys64

# 2. Open MSYS2 terminal, install toolchain
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-pkgconf

# 3. Add MSYS2 to system PATH
# C:\msys64\mingw64\bin
```

### Build Steps

```bash
# Clone
git clone https://github.com/HeRockOne/goink.git
cd goink

# Linux dependencies (Ubuntu/Debian)
sudo apt install libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc

# Build
make deps
make build   # production
make dev     # dev mode
```

### Windows One-Click Build

```powershell
.\build.ps1    # PowerShell
build.bat      # CMD
```

---

## Theme Colors

Vintage parchment style with light/dark modes. Colors defined in `frontend/src/index.css`.

### Color Variables → File Locations

| Variable | Purpose | File |
|----------|---------|------|
| `--background` | Overall page background | `index.css` |
| `--foreground` | Global text color | `index.css` |
| `--card` | Card/panel background | `index.css` |
| `--primary` | Primary accent (buttons, links) | `index.css` |
| `--sidebar` | Sidebar background | `index.css` |
| `--border` | Border color | `index.css` |
| `--reader-paper` | Reading content paper background | `index.css` + `ContentPanel.css` |

### Monaco Diff Editor Theme

Custom themes registered in `ContentPanel.tsx`:
- `goink-light`: Light paper background `#f5edd6`, green insert, red delete
- `goink-dark`: Dark leather background `#2a1e16`, warm white text

### How to Modify Colors

1. Edit `frontend/src/index.css` in `:root` (light) or `[data-theme="dark"]` (dark)
2. Reading area also controlled by `ContentPanel.css`
3. Monaco Diff theme registered in `ContentPanel.tsx`
4. Run `npm run build` to verify
5. Run `.\build.ps1` to rebuild and deploy

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| Agent Engine | ReAct loop (Go, SSE + 31 tools + sub-agents) |
| Desktop | Wails v2 (Go + WebView) |
| Frontend | React 19 + TypeScript + Tailwind 4 + shadcn/ui |
| Mobile | HTTPS API + pure JS web frontend + wspulse WebSocket |
| Database | SQLite + GORM |
| Vector Search | sqlite-vec + ONNX Runtime |
| Version Control | Built-in Git |
| Security | Dual sandbox + approval flow + API Token auth |
| LAN | WebDAV server |

---

## License

AGPL-3.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
