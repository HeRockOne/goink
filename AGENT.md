# AGENT.md — Goink 项目指南

Goink — 桌面 AI 写作系统，Wails (Go + React) 构建。

---

## 一、新 AI 接手必读

阅读顺序：
1. `docs/README.md` — 项目状态总览，含踩坑记录
2. `docs/01-architecture.md` — 系统架构
3. `docs/02-phase-gate.md` — 阶段门禁

---

## 二、构建

```powershell
.\build.ps1          # Windows 一键构建
```

Go 命令在项目根目录执行。前端构建在 `build.ps1` 中自动完成。

---

## 三、环境

- **OS**: Windows 10, PowerShell 7
- **依赖**: 仅 WebView2 Runtime（系统内置）
- **数据目录**: `D:\Goink\`（exe 同级），含 `novel-agent.db`、`novels/`
- **Git**: 每本小说独立仓库在 `{DataDir}/novels/{id}/`，含 `chapters/NNN.md`、`outlines/NNN.md`

### 外部调试（HTTP API）

桌面端启动后自动监听 `https://localhost:{端口}`，端口在桌面端「设置 → API 认证令牌」中查看和配置（默认 8877）。令牌也在同一页面获取。

```powershell
$token = "从设置页复制的令牌"
$port = 8877  # 以实际设置为准
$base = "https://localhost:$port"

# 获取小说列表
Invoke-RestMethod -Uri "$base/api/novels" -SkipCertificateCheck -Headers @{Authorization="Bearer $token"}

# 获取会话列表
Invoke-RestMethod -Uri "$base/api/sessions?novel_id=11&page=1&size=5" -SkipCertificateCheck -Headers @{Authorization="Bearer $token"}

# 查会话 token 消耗（usage 字段含 per_model 数据）
Invoke-WebRequest -Uri "$base/api/sessions?novel_id=11&page=1&size=50" -SkipCertificateCheck -Headers @{Authorization="Bearer $token"}

# 发消息（SSE 流，查 goink.log 看 usage 推送）
$body = @{message="hi"; novel_id=11; provider="商汤"; model="sensenova-6.7-flash-lite"} | ConvertTo-Json
Invoke-WebRequest -Uri "$base/api/chat" -Method Post -Body $body -ContentType "application/json" -SkipCertificateCheck -Headers @{Authorization="Bearer $token"} -TimeoutSec 60
```

所有 API 端点详见 `mobile/API.md`。

---

## 四、开发指引

### 数据库
- 所有表通过 `migrate/migrate.go` 自动建表，新增表加在 `models` 数组中
- `model_usage` 表存模型级 token 累计。累计时传**增量值**，不是累计值
- 消息是 append-only，`to_api`/`to_frontend` 独立控制可见性
- 角色关系追加式，`is_current` 标记当前状态
- 时间线 `target_chapter` 仅用于 ORDER BY，不用于 WHERE
- 压缩时递增 `active_version`，不删旧消息

### 代码
- 计费有关的缓存字段：优先 `prompt_tokens_details.cached_tokens`，fallback `prompt_cache_hit_tokens`。详见 `docs/10-billing-panel.md`
- `updateUsage` 在 `tokens.go`，每次 EventUsage 触发。改这里要小心 `perModel` 和全局累计值的一致性
- CGO：ONNX/sqlite-vec 用 `//go:build cgo`，Windows 上 cgo 编译报错是预期行为
- 不改 `frontend/src/lib/wailsjs/go/models.ts`（Wails 自动生成）

---

## 五、规范

- **并行读取文件**，减少来回
- **每次修改写入审计**到 `docs/README.md`（更新状态总览）
- **每次疑问先 `websearch`** 联网比对
- **Commit**: 英文，具体描述，无 emoji
- **用户用中文** — 用中文回复
- **不改日志和注释**，除非明确要求
- **不用 sed/python 脚本改代码**，用 Edit/Write
- **不问 "commit?"、"开始写？"**

## Output
- Return code first. Explanation after, only if non-obvious.
- No inline prose. Use comments sparingly - only where logic is unclear.
- No boilerplate unless explicitly requested.

## Code Rules
- Simplest working solution. No over-engineering.
- No abstractions for single-use operations.
- No speculative features or "you might also want..."
- Read the file before modifying it. Never edit blind.
- No docstrings or type annotations on code not being changed.
- No error handling for scenarios that cannot happen.
- Three similar lines is better than a premature abstraction.

## Review Rules
- State the bug. Show the fix. Stop.
- No suggestions beyond the scope of the review.
- No compliments on the code before or after the review.

## Debugging Rules
- Never speculate about a bug without reading the relevant code first.
- State what you found, where, and the fix. One pass.
- If cause is unclear: say so. Do not guess.

## Simple Formatting
- No em dashes, smart quotes, or decorative Unicode symbols.
- Plain hyphens and straight quotes only.
- Natural language characters (accented letters, CJK, etc.) are fine when the content requires them.
- Code output must be copy-paste safe.
