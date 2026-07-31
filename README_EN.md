<h1 align="center"><img src="assets/logo.svg" width="80" alt="Goink Logo"><br>Goink<br><sub>Desktop AI Writing System — Agent Real-time Decisions × Structured Memory × Post-Write Self-Check</sub></h1>

<p align="center"><strong>English | <a href="README.md">中文</a></strong></p>

---

> **Forked from [sigpanic/goink](https://github.com/sigpanic/goink) v1.1**
>
> All code modifications completed by open-source AI models, based on actual writing requirements.

---

## Table of Contents

- [Differences from Upstream v1.1](#differences-from-upstream-v11)
  - [1. New Creative Modules](#1-new-creative-modules)
  - [2. Data Pipeline Architecture](#2-data-pipeline-architecture)
  - [3. Phase Gate](#3-phase-gate)
  - [4. HTTP API](#4-http-api-23-endpoints)
  - [5. Model Config Enhancements](#5-model-config-enhancements)
  - [6. Mobile Web Frontend](#6-mobile-web-frontend)
  - [7. Chat UI Improvements](#7-chat-ui-improvements)
  - [8. Custom Themes](#8-custom-themes)
  - [9. Icon Replacement](#9-icon-replacement)
  - [10. WebDAV](#10-webdav)
  - [11. Other Features](#11-other-features)
  - [12. Field Extensions](#12-field-extensions)
  - [13. Database Tables](#13-database-tables)
  - [14. MCP Tools](#14-mcp-tools)
  - [15. Documentation](#15-documentation)
  - [16. Skill System](#16-skill-system)
  - [17. Security](#17-security)
- [Installation](#installation)
- [Project Structure](#project-structure)
- [Tech Stack](#tech-stack)
- [License](#license)

---

## Differences from Upstream v1.1

### 1. New Creative Modules (8)

| Module | Description |
|--------|-------------|
| Lore (World Settings) | 5 MCP tools + frontend UI + arc_id linking |
| Item (Artifacts) | 5 MCP tools + frontend UI + arc_id linking |
| ItemOccurrence | 2 MCP tools, track item appearances across chapters |
| Scene | 4 MCP tools + backend API, linked to arc nodes |
| ChapterMeta (update_chapter_meta) | summary / key_events / characters_in / arc_ids |
| Tree Context (get_writing_context) | One call gets all related data + overdue foreshadowing detection |
| Writing Snapshot | current_arc_id / active_chars / summary / detailed_state |
| Stats | word count / arc progress / foreshadowing rate / character count / location count |

### 2. Data Pipeline Architecture

```
prepare(get_writing_context) → outline(edit outlines/)
→ write(edit chapters/) → review(run_subagent)
→ maintain(update_*/create_* + update_chapter_meta + update_writing_snapshot
           + search_lore + search_items + set_phase)
→ back to prepare → reads latest data from maintain
```

- prepare requires `get_writing_context` for complete state in one call
- maintain requires `update_chapter_meta` + `update_writing_snapshot` + `search_lore` + `search_items` enforced data write-back
- **Dual-layer required**: phase gate require forces tool calls, jsonschema required forces complete fields

### 3. Phase Gate

- 5-phase validation: prepare → outline → write → review → maintain
- Each phase has tools whitelist + required call list
- maintain phase has 15-item checklist (see `writing-kernel.md`)
- Config stored in DB, zero token cost for AI

### 4. HTTP API (23 Endpoints)

No API in original. All read endpoints added:

```
GET  /api/novels              Novel list
GET  /api/novels/{id}/chapters  Chapter list
GET  /api/chapters/{id}        Chapter content
GET  /api/characters           Characters
GET  /api/character-relations  Character relations
GET  /api/locations            Locations
GET  /api/location-relations   Location relations
GET  /api/lore                 World settings
GET  /api/items                Items
GET  /api/item-occurrences     Item occurrences
GET  /api/scenes               Scenes
GET  /api/timeline             Timeline
GET  /api/arcs                 Story arcs
GET  /api/arc-nodes            Arc nodes
GET  /api/reader               Reader perspective
GET  /api/preferences          Preferences
GET  /api/stats                Statistics
GET  /api/writing-snapshot     Writing snapshot
GET  /api/phase-gate-config    Phase gate config
GET  /api/search-memory        Semantic search
GET  /api/writing-context      Writing context tree
GET  /api/read                 Read file
POST /api/chat                 AI chat (SSE)
```

Bearer Token authentication. See [mobile/API.md](mobile/API.md).

### 5. Model Config Enhancements

| Change | Description |
|--------|-------------|
| model.dev auto-fetch | Auto-fetch model list and parameters |
| Thinking mode | Deep reasoning toggle (high/max) |
| Custom model edit button | Click pencil icon to modify parameters after adding |
| Max turns per round | 50 → **100** |

### 6. Mobile Web Frontend

Access at `https://{LAN_IP}:8877/mobile/`.

| Module | Features |
|--------|----------|
| Bookshelf | Novel list, word counts |
| Novel Details | Chapters/Characters/Timeline/Arcs/Reader/Preferences/Locations/Lore/Items |
| Fullscreen Reader | Font/line spacing adjustment, page turning, chapter index, progress memory |
| AI Chat | SSE streaming, thinking process, conversation history, model switching, copy button |
| Settings | Light/dark mode, language (CN/EN), token management, model selection |

- **Offline cache**: idb-keyval + memory Map, instant read offline
- **Service Worker**: Pre-cache static assets for offline use
- **Real-time sync**: WebSocket full-duplex desktop-mobile sync
- **QR code connection**: Scan desktop QR code for quick connect
- **Auto HTTPS**: Auto-generate certificate on startup
- **Mobile theme**: Warm wood-study theme

### 7. Chat UI Improvements

| Change | Description |
|--------|-------------|
| Copy button on bubble edge | Outside bubble (AI right/user left), no content overlap |
| Scroll-to-bottom button | Quick jump to latest message |
| Message spacing | Increased spacing between AI and user messages |
| Phase gate block message | Moved below progress bar |

### 8. Custom Themes

Settings → Theme → Paste JSON → Click to apply, no confirm button needed.

**JSON format:**
```json
{
  "name": "Darkwood Study",
  "type": "dark",
  "colors": {
    "--background": "#0f1a14",
    "--foreground": "#d8e8d8",
    "--primary": "#5a9a6a",
    ...
  }
}
```

- `name` — Theme name
- `type` — `light` or `dark`, controls chart color scheme
- `colors` — All CSS variable key-value pairs

**Dedup key**: `name__type` (same name, different types can coexist). **Supports comments** (`//` and `/* */`).

**Color variables (67 total):**

| Variable | Area |
|----------|------|
| `--background` / `--foreground` | Page background / text |
| `--card` / `--card-foreground` | Card/panel/dialog |
| `--popover` / `--popover-foreground` | Popover/dialog overlays |
| `--primary🔑` / `--primary-foreground` | Buttons/links/selection/slider/switch |
| `--secondary` / `--secondary-foreground` | Secondary panels |
| `--muted` / `--muted-foreground` | Input fields/code blocks/helper text |
| `--accent` / `--accent-foreground` | Hover/highlight rows |
| `--destructive` / `--destructive-foreground` | Delete buttons/error messages |
| `--border` / `--input` / `--ring` | Borders/focus rings |
| `--chart-1` ~ `--chart-5` | Chart colors |
| `--sidebar-*` (6 vars) | Sidebar |
| `--tag-*` (6 colors × 2) | Tags/badges |
| `--reader-bg` / `--reader-paper` | Reading mode |
| `--bubble-user💬` / `--bubble-user-foreground` | User message bubble |
| `--success✅` / `--success-foreground` / `--success-border` | Success messages |
| `--danger-bg⚠️` / `--danger-border` | Error/warning messages |
| `--status-warning` / `--status-ok` | Status indicators |
| `--tool-*🔧` (4 colors × 2) | Tool call cards |
| `--contribution-0` ~ `--contribution-4📊` | Contribution graph |

**Notes:**
- All 67 variables required (missing any breaks UI)
- `type` only controls chart light/dark mode, not CSS mode
- JSON supports `//` and `/* */` comments
- Monaco editor theme follows CSS variables

### 9. Icon Replacement

| Location | Usage | Format |
|----------|-------|--------|
| `build/windows/icon.ico` | exe icon + window title bar icon | ICO (multi-size) |
| `appicon.png` | Wails app icon | PNG |
| `frontend/public/logo.svg` | Logo in title bar | SVG |
| `frontend/public/favicon.svg` | Browser tab icon | SVG |
| `assets/logo.svg` | Logo source file | SVG |

**Steps:**
1. Prepare new icons (SVG or HD PNG recommended)
2. Replace files:
   - **exe icon**: Convert PNG to ICO, replace `build/windows/icon.ico`
   - **App icon**: Place PNG at project root as `appicon.png`, copy to `build/appicon.png`
   - **Title bar logo**: Place SVG at `frontend/public/logo.svg`
   - **Favicon**: Place SVG at `frontend/public/favicon.svg`
3. Run `.\build.ps1` to rebuild
4. If exe icon not updating, clear Windows icon cache or restart

### 10. WebDAV

Built-in WebDAV server. Read novels directly from phone file manager.

### 11. Other Features

| Feature | Description |
|---------|-------------|
| Chapter word count range | Custom min/max words per chapter |
| Log toggle | Enable/disable file logging in settings |
| Backup & restore | One-click full data backup/restore |
| Custom software icon | Desktop/taskbar/title bar icons unified with theme (see Icon Replacement) |
| Help center | 52 tools described in Chinese & English with return structure docs |
| System prompt optimization | ~4700 → ~2400 tokens (49% savings) |
| Token injection stats | `tokencount` precisely counts system prompt + tool definitions (~16.1K tokens currently) |
| writing-kernel.md | 15-item maintain checklist |
| config.json removed | Data dir uses exe location directly |
| Billing panel | Per-model token accumulation, cache hit/miss split, configurable prices (CNY per million tokens) |
| Token usage trend chart | Monthly overview aggregated by date + model, SVG pie chart for cache ratio |
| Dynamic narrative panel | Canvas-style draggable/resizable cards aggregating 7 narrative info types |
| DOCX export | Pure standard-library implementation (archive/zip + XML), zero dependencies |
| Prompt caching optimization | Stable prefix (identity + always + catalog) + dynamic NovelState injection, prefix hash monitoring |
| Input guide cards | 4 guide cards shown on empty session |
| HTTPS toggle | Mobile API can switch to HTTP in settings (LAN debugging) |
| Resizable sidebar | SidePanel width draggable |

### 12. Field Extensions

| Table | Upstream Fields | Added Fields |
|-------|-----------------|--------------|
| `lore_entries` | reference_type, reference_id | arc_id, reveal_chapter_id, is_public |
| `items` | owner_id, location_id, status | arc_id, first_chapter_id, status_changed_chapter_id, narrative_role, previous_owner_id |
| `scenes` | chapter_id, character_ids, location_id | arc_id, arc_node_id |
| `chapters` | title, summary | key_events, characters_in, arc_ids |
| `writing_snapshots` | last_chapter_id, current_location | current_arc_id, active_chars, summary, detailed_state |

### 13. Database Tables

| Upstream | This Fork |
|----------|-----------|
| 22 tables | **25** tables (+item_occurrences, scenes, model_usage) |

### 14. MCP Tools

| Upstream | This Fork |
|----------|-----------|
| 33 tools | **52** tools (+19) |
| Some tools have Description | **All tools document return structure** |
| Some fields have required | **All dependency chain fields have jsonschema required** |

### 15. Documentation

| Document | Description |
|----------|-------------|
| `docs/README.md` | Project status overview (central index) |
| `docs/01-architecture.md` | Full architecture (must-read for new AI) |
| `docs/02-phase-gate.md` | Phase gate documentation |
| `docs/03-competitor-analysis.md` | Chinese million-word novel tool comparison |
| `docs/10-billing-panel.md` | Billing panel technical design |
| `docs/12-prompt-caching-optimization.md` | Prompt caching optimization |
| `docs/20-narrative-panel.md` | Dynamic narrative panel design |
| `docs/30-mcp-tools-audit.md` | Tool dependency chain audit |
| `docs/31-mcp-schema-audit.md` | MCP Schema Required audit |
| `mobile/API.md` | HTTP API documentation (27 sections) |

### 16. Skill System

Three layers (builtin/user/novel) × three modes (auto/manual/always) = 9 strategies.

17 built-in skills. Create a new `.md` file to add a Skill, zero-code.

### 17. Security

- Dual sandbox: regex whitelist + SafePath path traversal protection
- File edit re-reads before writing, prevents overwriting manual changes

---

## Installation

Download from [Releases](https://github.com/HeRockOne/goink/releases).

### Runtime Dependencies

| Dependency | Description |
|------------|-------------|
| WebView2 Runtime | Built-in on Windows 11; requires install on Windows 10 |
| LLM API Key | OpenAI-compatible (DeepSeek, OpenAI, Claude, NVIDIA, etc.) |

### Build from Source

```bash
git clone https://github.com/HeRockOne/goink.git
cd goink
sudo apt install libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc  # Linux
make deps && make build  # or make dev
```

Windows one-click build: `.\build.ps1` or `build.bat`

---

## Project Structure

```
goink/
├── app/                    # Wails binding + HTTP API (23 endpoints)
├── tokencount/            # Token counting tool (precise system prompt + tool JSON injection stats)
├── internal/
│   ├── agent/              # Agent loop (MaxTurns 100)
│   ├── agentcfg/           # System prompt (2400 tokens) + tool whitelist
│   ├── mcp_tools/          # 52 MCP tools
│   ├── llm/                # Multi-provider LLM
│   ├── session/            # Session + messages
│   ├── character/          # Characters + directed relation graph
│   ├── chapter/            # Chapter metadata
│   ├── timeline/           # Foreshadowing + chapter plan
│   ├── storyarc/           # Story arcs + nodes
│   ├── reader/             # Reader perspective
│   ├── location/           # Location graph
│   ├── lore/               # World settings
│   ├── item/               # Items/artifacts
│   ├── itemoccurrence/     # Item occurrence tracking
│   ├── scene/              # Scene management
│   ├── writing/            # Writing log + snapshot
│   ├── rag/                # Vector search (ONNX)
│   ├── search/             # Full-text search
│   ├── skill/              # Skill system (3 layers × 3 modes)
│   ├── cert/               # Auto HTTPS certificate
│   ├── webdav/             # WebDAV server
│   └── migrate/            # 25 tables auto-migration
├── mobile/                 # Mobile web frontend
├── frontend/               # Desktop React frontend
├── docs/                   # Architecture/audit/competitor docs
├── skills/                 # 17 built-in Skills
├── build.ps1               # Windows one-click build
└── build.bat               # Windows one-click build
```

---

## Tech Stack

| Layer | Choice |
|-------|--------|
| Agent Engine | ReAct loop (Go, SSE + 52 tools + sub-agent, MaxTurns 100) |
| Desktop Framework | Wails v2 (Go + WebView) |
| Frontend | React + TypeScript + Tailwind CSS + shadcn/ui |
| Mobile | HTTP API + vanilla JS web frontend + idb-keyval offline cache |
| Database | SQLite + GORM (25 tables + auto-migration) |
| Vector Search | sqlite-vec + ONNX Runtime (BGE Chinese model) |
| Version Control | Built-in Git (auto commit / Diff / Revert) |

---

## License

AGPL-3.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
