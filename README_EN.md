<h1 align="center"><img src="assets/logo.svg" width="80" alt="Goink Logo"><br>Goink<br><sub>Desktop AI Writing System — Agent Real-time Decisions × Structured Memory × Post-Write Self-Check</sub></h1>

<p align="center"><strong>English | <a href="README.md">中文</a></strong></p>

---

> Forked from [sigpanic/goink](https://github.com/sigpanic/goink) v1.1, with significant expansion of creative modules, tool system, and engineering capabilities.

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Project Structure](#project-structure)
- [Tech Stack](#tech-stack)
- [Comparison with Upstream](#comparison-with-upstream)
- [License](#license)

---

## Features

### Industrial-Grade Creative Pipeline

Goink's core is a **five-stage phase-gated pipeline** that guides AI from preparation to maintenance:

```
prepare → outline → write → review → maintain → back to prepare
```

- **prepare**: `get_writing_context` fetches full context (characters/arcs/foreshadowing/reader perception/scenes/items/stats) in one call
- **outline**: Write outline to `outlines/NNN.md`
- **write**: Write chapter content to `chapters/NNN.md`, released only when word count meets threshold
- **review**: Launch review sub-agent to check character consistency, setting contradictions, foreshadowing recovery, arc progression
- **maintain**: Force-write all state updates (characters/timeline/arcs/reader perception), 15-item checklist

Each phase has a **tools whitelist** and **require list**. Gate config is stored in DB, not in AI context.

### 8 New Creative Modules

| Module | Capabilities |
|--------|-------------|
| Worldbuilding (Lore) | 5 MCP tools + frontend UI, arc_id linkage |
| Items/Artifacts (Item) | 5 MCP tools + frontend UI, arc_id linkage |
| Item Occurrences | Track item appearances and state changes across chapters |
| Scene Management (Scene) | 4 MCP tools, arc_node_id linkage |
| Chapter Metadata (ChapterMeta) | summary / key_events / characters_in / arc_ids |
| Tree Context (WritingContext) | 8 data sources in one call + overdue foreshadowing detection |
| Writing Snapshot (Snapshot) | current_arc_id / active_chars / summary / detailed_state |
| Creative Stats (Stats) | word count / arc progress / foreshadowing recovery rate / entity counts |

### HTTP API + Mobile Frontend

23 REST endpoints + SSE chat stream, full Goink functionality from mobile browser:

```
GET  /api/novels              novel list
GET  /api/novels/{id}/chapters  chapter list
GET  /api/chapters/{id}        chapter content
GET  /api/characters           characters
GET  /api/character-relations  character relations
GET  /api/locations            locations
GET  /api/location-relations   location relations
GET  /api/lore                 worldbuilding entries
GET  /api/items                items
GET  /api/item-occurrences     item occurrences
GET  /api/scenes               scenes
GET  /api/timeline             timeline
GET  /api/arcs                 story arcs
GET  /api/arc-nodes            arc nodes
GET  /api/reader               reader perspective
GET  /api/preferences          preferences
GET  /api/stats                statistics
GET  /api/writing-snapshot     writing snapshot
GET  /api/phase-gate-config    phase gate config
GET  /api/search-main-cmd-memory        semantic search
GET  /api/writing-context      tree context
GET  /api/read                 read file
POST /api/chat                 AI chat (SSE)
```

Bearer Token authentication. See [mobile/API.md](mobile/API.md) for details.

- **Offline cache**: idb-keyval + in-memory dual cache
- **Service Worker**: Pre-cached static assets, offline-ready
- **Real-time sync**: WebSocket full-duplex between desktop and mobile
- **QR code connect**: Scan QR from desktop settings to connect mobile
- **Auto HTTPS**: Self-signed cert generated on startup

### Dynamic Narrative Panel

Canvas-based draggable/resizable card panel, all writing context in one IPC call:

- 7 cards: Current/Past/Future/Arcs/Foreshadowing/Reader/Detail
- Drag, resize, snap, rename, show/hide
- Layout persisted in localStorage
- Auto-refresh on file changes and chat events, 300ms debounce

### 57 MCP Tools

AI manages all novel data through 57 Function Calling tools. Each tool has detailed descriptions teaching AI creative methodology (worldbuilding classification, foreshadowing timing, plot twist design).

New tool categories:

| Category | Count | Description |
|----------|-------|-------------|
| Worldbuilding (Lore) | 5 | CRUD + semantic search |
| Item | 5 | CRUD + semantic search |
| Item Occurrence | 2 | Track item appearances |
| Scene | 4 | CRUD |
| Stats | 1 | Creative data aggregation |
| Snapshot | 2 | Writing progress snapshot |
| Phase Gate | 2 | Gate config read/write |
| Writing Context | 1 | 8 data sources in one call |
| Chapter Meta | 1 | Update chapter summary/events/characters |
| Web Search/Fetch | 2 | Exa API search + web fetch |
| Sub Agent | 1 | Launch review/memory sub-agent |
| Delete | 1 | Generic record deletion |

### 43 Skills

Three-layer skill system (builtin/user/novel x auto/manual/always), zero-code extension:

| Category | Count | Description |
|----------|-------|-------------|
| Core System | 5 | Creative pipeline dispatch, phase init |
| Writing Technique | 20+ | Show-dont-tell, chapter hooks, dialogue subtext, pacing, foreshadowing cycles |
| Genre Specialization | 8 | Xianxia cultivation, urban martial arts, post-apocalyptic, suspense, historical time-travel |
| Sub Skills | 8 | Review standards, anti-AI detection scoring |

### Model Configuration

- Auto-discover model list (`DiscoverModels`)
- Reasoning mode support (high / max)
- Max turns per session: 100
- LLM auto-retry: exponential backoff for 429 and retryable errors, up to 60s
- Configurable compression threshold (default 0.7)
- Web search via Exa API

### Billing Panel

- Compatible with OpenAI standard + DeepSeek cache format
- Per-model consumption tracking
- Token trend chart (date + model aggregation, SVG pie chart)
- Cache hit rate: 89-93% measured

### Built-in WebDAV

- Configurable port/user/password
- Auto-export TXT after each chat session
- Direct reading from mobile file managers

### Custom Themes

- 67 CSS variables fully covered
- Light/dark modes
- JSON paste to apply
- Sample theme: "Green Study"
- `normalizeTheme()` auto-fills missing variables

### Security

- **Dual sandbox**: Regex whitelist + SafePath prevents path traversal
- **File editing**: Re-read before write to prevent overwriting manual changes
- **API authentication**: Bearer Token, auto-generated
- **Audit log**: All DB changes are logged

---

## Installation

### Runtime Dependencies

- **Windows 10+**: WebView2 Runtime only (built-in)
- **macOS 11+**: System WebView only
- **Linux**: WebKit2GTK 4.1

### Build from Source

```powershell
# Windows
.\build.ps1

# macOS / Linux
make build
```

Binary output in `build/bin/`, auto-deployed to `D:\Goink\` (Windows) or same directory.

---

## Project Structure

```
goink-fork/
├── main.go              # Entry point
├── app/                 # Wails binding layer (42 files)
│   ├── handler.go       #   App struct + lifecycle
│   ├── chat.go          #   Chat entry
│   ├── api_server.go    #   HTTP API server
│   ├── novel.go         #   Novel CRUD
│   └── ...              #   View APIs, settings, backup, content
├── internal/            # Core logic (~150 files)
│   ├── agent/           #   ReAct Agent engine + phase gate
│   ├── agentcfg/        #   System prompts + tool allowlists
│   ├── mcp_tools/       #   57 MCP tool registry
│   ├── llm/             #   Multi-provider LLM client
│   ├── skill/           #   Three-layer skill system (41 builtin)
│   ├── rag/             #   Vector search (ONNX + sqlite-vec)
│   ├── search/          #   Three-way merged search
│   ├── session/         #   Session storage
│   ├── storage/         #   SQLite connection pool
│   ├── git/             #   Built-in Git management
│   ├── migrate/         #   25 table auto-migration
│   ├── ws/              #   WebSocket sync
│   ├── cert/            #   Self-signed certificate
│   ├── webdav/          #   LAN file sharing
│   ├── export/          #   TXT/MD/EPUB/DOCX
│   ├── import/          #   TXT/EPUB/LLM import
│   └── 20+ domain stores # Character/location/arc/timeline/etc.
├── frontend/            # React desktop (70+ files)
│   └── src/
│       ├── components/  # 25+ component directories
│       │   ├── chat/    #   Chat panel
│       │   ├── content/ #   Content editor
│       │   ├── narrative/ # Narrative panel
│       │   ├── character/ # Character graph
│       │   ├── settings/  # Settings
│       │   └── ...
│       └── i18n/        # Chinese/English
├── mobile/              # Mobile web frontend
│   ├── app.js           #   App logic (77k lines)
│   ├── style.css        #   Styles (33k lines)
│   └── API.md           #   API documentation
├── skills/              # Persistent dispatch skills
├── docs/                # Documentation
│   ├── architecture/    #   System design
│   ├── adr/             #   Decision records
│   ├── design/          #   Proposals
│   └── archive/         #   Archives
└── tokencount/          # Token counting tool
```

---

## Tech Stack

| Layer | Choice |
|-------|--------|
| Desktop Framework | Wails v2 (Go + WebView2) |
| Frontend | React 18 + TypeScript + Tailwind CSS + shadcn/ui |
| Backend | Go 1.26, GORM + SQLite |
| Agent Engine | ReAct loop (SSE + 57 tools + sub-agents, MaxTurns 100) |
| Vector Search | ONNX Runtime (BGE Chinese) + sqlite-vec |
| Version Control | Built-in Git (per-novel repository) |
| Mobile | Vanilla JS + idb-keyval + Service Worker |
| i18n | react-i18next (Chinese/English) |

---

## Comparison with Upstream

This fork is based on [sigpanic/goink](https://github.com/sigpanic/goink) v1.1. Key differences:

### Creative Modules

| Module | Upstream v1.1 | This Fork |
|--------|---------------|-----------|
| Worldbuilding (Lore) | None | 5 MCP tools + frontend UI |
| Items/Artifacts | None | 5 MCP tools + frontend UI |
| Item Occurrences | None | 2 MCP tools |
| Scene Management | None | 4 MCP tools |
| Chapter Metadata | None | summary / key_events / characters_in |
| Tree Context | None | 8 data sources in one call |
| Writing Snapshot | None | Progress snapshot |
| Creative Stats | None | Word count / arc / foreshadowing stats |

### Tools & Skills

| Metric | Upstream v1.1 | This Fork |
|--------|---------------|-----------|
| MCP Tools | 33 | **57** |
| Builtin Skills | 12 | **41** |
| Database Tables | 17 | **25** |

### Engineering

| Feature | Upstream v1.1 | This Fork |
|---------|---------------|-----------|
| Phase Gate | None | 5-stage validation + whitelist + require |
| HTTP API | None | 23 endpoints + SSE chat |
| Mobile Frontend | None | Full web app + offline cache |
| WebDAV | None | Built-in server |
| Billing Panel | None | Token stats + trend chart |
| Narrative Panel | None | 7-card canvas layout |
| Custom Themes | None | 67 CSS variables |
| DOCX Export | None | Pure stdlib implementation |
| Wails Version | v2.12.0 | **v2.13.0** |
| Go Version | 1.25 | **1.26** |
| Max Turns | 50 | **100** |
| Web Search | DeepSeek | **Exa API** |

### Data Pipeline

```
Upstream:  get_* one by one → manual maintenance
This fork: prepare(get_writing_context) → outline → write → review → maintain(force write-back)
```

- prepare requires `get_writing_context` for full state
- maintain requires `update_chapter_meta` + `update_writing_snapshot` + `search_lore` + `search_items` forced write-back
- Dual-layer required: gate require forces tool calls, jsonschema required forces complete fields

### Field Extensions

| Table | New Fields |
|-------|-----------|
| chapters | avatar_url, summary, key_events, characters_in |
| characters | avatar_url, location_id, description |
| locations | avatar_url, location_type, description |
| sessions | current_phase, called_tools, reasoning_effort |
| messages | thinking_content, extra_metadata, agent_type, sub_task_id |

---

## License

AGPL-3.0. See [LICENSE](LICENSE).