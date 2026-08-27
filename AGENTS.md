# AGENT.md — Goink 项目指南

Goink — 桌面 AI 创作小说软件，Wails (Go + React) 构建。

---

## 〇、产品定位（最高优先级，所有决策的出发点）

**Goink 是 AI 创作小说的软件，不是单纯的工具。所有代码和优化都必须以创作质量为第一出发点。**

这意味着：
- **创作质量永远第一，省 token/省资源永远第二。** 任何"优化"如果可能损害 AI 的创作能力，一律不做。
- **工具的 description/schema 是创作方法论的载体**，不是 token 冗余。它们教会 AI 世界观分类、伏笔回收节奏、悬念反转设计。**禁止精简它们**来省 token——那会丢失创作能力，导致设定遗漏、章节衔接不上、偏离小说主旨。
- 一致性、设定完整、长篇连贯 > 性能 > token 成本。任何改动前先问：**这会损害创作质量吗？**
- 系统提示词、MCP 工具、skill 里出现的创作规则，都可能是设计意图，不要轻易删减。

### Skill 结构约定

- **内置 skill（`internal/skill/builtin/`）**：瑞士军刀全量版（当前 44 个：37 auto + 5 manual + 2 always——main-core-ai-communication-standard 已内置兜底，与 kernel 同待遇；数量以目录为准，勿在文档写死）。可按需优化（补正反例/自查表/统一口径），但**创作规则零删减**——任何改动不得删掉原有的写作规则、判定标准或检查项，只能增加或改写表达
- **`skills/`（项目根目录，版本控制）= 用户级 skill 真源**：当前 6 个文件（main-core-writing-kernel、main-core-ai-communication-standard、main-core-init-phase、main-tech-chapter-title-design、main-tech-data-hygiene、sub-tech-review-standards），数量以目录为准。这些是**需要时可改**的调度/审稿技能（同名覆盖内置），同步到 `~/.goink/skills/` 后生效；改动后如属通用改进应回灌 `internal/skill/builtin/` 保持基线一致（2026-08-21 已双向同步：6 文件与 builtin 逐字节一致，其中 kernel/review-standards/init-phase 为演进版回灌——kernel 回灌时补回单章写后自审、write→review 边界、init 用户确认门槛三条规则）
- **新增 skill**：放 `internal/skill/builtin/<name>.md`，并在 main-core-writing-kernel 的阶段技能表登记（需重新编译，或放用户级即时生效）
- 同名优先级：小说级 > 用户级 > 内置（放用户级可覆盖内置默认行为）

---

## 一、新 AI 接手必读

**仓库布局**：`app/` = Wails 绑定层（Chat/设置/面板入口，chat.go 组装 LLM 链路）；`internal/` = 核心库（agent=循环+门禁+压缩、agentcfg=系统提示词/白名单、mcp_tools=工具、cacheprobe=成本模拟、session=消息存储、skill=技能库、llm=客户端）；`frontend/` = React 前端；`cmd/cacheprobe/` = 模拟 CLI；`skills/` = 用户级 skill 真源（版本控制，同名覆盖内置）。

阅读顺序：
1. `docs/README.md` — 文档索引（architecture/design/adr/archive 分层，archive 含历次审计）
2. `docs/architecture/architecture.md` — 系统架构
3. `docs/architecture/phase-gate.md` — 阶段门禁
4. `docs/adr/0001-prompt-caching.md` — 前缀缓存决策（改工具注入/消息顺序前必读）
5. `internal/agent/DESIGN.md` — Agent 循环设计（**有过时声明**，以代码为准）
6. `docs/archive/llm-chain-audit-2026-08-12.md` — 最近一次 LLM 链路全量审计（问题清单 + 修复记录）

---

## 二、构建

```powershell
.\build.ps1          # Windows 一键构建
```

Go 命令在项目根目录执行。前端构建在 `build.ps1` 中自动完成。

---

## 三、环境

- **OS**: Windows 10, PowerShell 7（C:\Program Files\PowerShell\7\pwsh.exe）
- **依赖**: 仅 WebView2 Runtime（系统内置）
- **数据目录**: `D:\Goink\`（exe 同级），含 `novel-agent.db`、`novels/`
- **调试日志**: `D:\Goink\goink.log`（DEBUG 级，含 LLM usage/model_usage 更新、门禁拦截、工具调用、appendMsg 落库等，排查 token/门禁问题先看这里）
- **Git**: 每本小说独立仓库在 `{DataDir}/novels/{id}/`，含 `chapters/NNN.md`、`outlines/NNN.md`

### Shell 使用约定

- **主用 PowerShell 7**：本机默认 shell。构建链（build.ps1）、系统管理（进程/端口/注册表）、HTTP API 调试、DSH 交互全部是 PS 生态，除非有明确理由不要换
- **Git Bash 备用**：`C:\Program Files\Git\bin\bash.exe`（已装，内含 git 2.55.0 + mingw64 工具链：grep/sed/awk/tar 等）。**仅两种场景切换**：① 批量文本流处理（多行 grep/sed/awk 流水线，PS 写起来啰嗦）；② 跑现成的 Linux 风格脚本
- **调用方式**（在 PS 里）：`& "C:\Program Files\Git\bin\bash.exe" -c "命令"`，退出码看 `$LASTEXITCODE`
- **注意**：bash 处理中文输出可能有编码坑（GBK/UTF-8 混用），发现乱码就换回 PS 的 `Get-Content -Encoding UTF8`；不要为了"统一"把 PS 能干的事硬搬到 bash

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
- 计费有关的缓存字段：优先 `prompt_tokens_details.cached_tokens`，fallback `prompt_cache_hit_tokens`。详见 `docs/archive/billing-panel.md`
- `updateUsage` 在 `tokens.go`，每次 EventUsage 触发。改这里要小心 `perModel` 和全局累计值的一致性
- CGO：ONNX/sqlite-vec 用 `//go:build cgo`。Windows 编译必须带 CGO 环境：`build.ps1` 已设置（PATH 含 MSYS2 mingw64、`CGO_ENABLED=1`、`CGO_CFLAGS=-I$(go env GOMODCACHE)/github.com/mattn/go-sqlite3@版本`——sqlite-vec 的 C 代码 include "sqlite3.h"，头文件由 mattn 包自带）。直接用 `go build ./...` 不带这些环境变量报 `sqlite3.h: No such file or directory` 是缺 include 路径，不是"Windows 不能编译"
- **验证命令**（项目根，需先设置上述 CGO 环境）：
  - `go build ./...`
  - `go test ./internal/... ./app/...`（e2e 需真实 git/ONNX 环境，可跳过）
  - 例外：`internal/web` 测试**必然失败**，与本轮改动无关，勿纠结，原因见下：
    - `internal/web` 是 `web_fetch` MCP 工具的网页抓取库（`fetch.go`：抓网页转纯文本给 LLM，带反爬检测 + SSRF 防护）
    - `fetch_test.go` / `realworld_test.go` 是**真实外网测试**（抓 `api-docs.deepseek.com`、Wikipedia、CSDN 等），依赖外网可达和域名解析正常
    - 本机 DNS 会把 `api-docs.deepseek.com` 解析到 ULA 保留地址 `fdfe:dcba:9876::f`（系统代理/DNS 劫持/广告过滤注入），`validateHost` 的 SSRF 防护把 fc00::/7 判为内网拒绝 → 报 `禁止访问内网地址`
    - 处理：整体测试时跳过该包；纯逻辑测试（`fetch_internal_test.go` 的乱码/反爬判定）正常，可单独跑 `go test ./internal/web/... -run TestIsEncodingGarbled|TestIsAntiCrawl|TestCompressionRatios`
  - 前端：`cd frontend && npm run build`（tsc + vite，前端改动必须跑）
- **bindings 自动生成**：`frontend/src/lib/wailsjs/`（App.js/App.d.ts/models.ts）由 `wails generate module` 生成，**不手改**。新增/修改 app 方法或结构体后，先跑 `wails generate module`（需 CGO 环境）再构建前端，否则前端调不到新方法

---

## 五、规范

### 创作质量红线（违反 = 事故）
- **禁止精简工具 description/schema 省 token** — 它们是创作方法论（世界观分类/伏笔节奏/悬念设计），精简会导致 AI 分不清工具、参数漏填、设定遗漏、长篇脱节
- **禁止裁剪工具定义换取缓存收益** — 全量发送 + 稳定前缀是 ADR-0001 的刻意设计（命中率 89-93%），改动前必须读 `docs/adr/0001-prompt-caching.md`
- **禁止删减系统提示词/skill 里的创作规则** — 可能是设计意图，先理解再动
- 任何 token/性能优化，先问：**创作质量会受影响吗？** 会则不做

### 常规规范

- **并行读取文件**，减少来回
- **审计**：涉及行为变更/新功能/修复的修改，commit 前在 `docs/README.md` 追加一行审计（格式：`> YYYY-MM-DD：改动摘要（关键文件/决策，一两句）`）；纯文案/格式微调可不写。漏了就在下一轮补
- **每次疑问先 `websearch`** 联网比对
- **每次修改完业务代码，必须Commit本地**: 英文，具体描述，无 emoji
- **不主动 push**：除非用户明确要求，只 commit 本地（远端同步由用户决定）
- **并发代码警惕共享状态**：Agent 单例字段/包级变量在多会话并发时会串扰（教训：agent 门禁 phaseGate 存 Agent 字段导致桌面+移动端并发互相覆盖，已改为 Run 局部变量；cacheprobe 包级状态靠 simMu 串行化）。新写并发路径时优先局部变量/context 传递
- **用户用中文** — 用中文回复
- **不改日志和注释**，除非明确要求
- **不用 sed/python 脚本改代码**，用 Edit/Write
- **不问 "commit?"、"开始写？"**

## 输出
- 先给代码。解释放后面,且只在含义不明显时才给。
- 不要内联散文。注释要克制——只在逻辑不清楚的地方用。
- 除非明确要求,不要样板代码。

## 代码规则
- 最简单可用的方案,不要过度设计。
- 单次使用不要抽象化。
- 不要投机性功能或"你可能还会想要..."。
- 修改前先读文件,绝不盲目修改。
- 不改的代码不要加 docstring 或类型注解。
- 不要为不可能发生的场景写错误处理。
- 三行相似代码好过过早的抽象。

## 审查规则
- 指出 bug,给出修复,然后停。
- 不要超出审查范围的建议。
- 审查前后都不要夸代码。

## 调试规则
- 不先读相关代码,绝不臆测 bug。
- 说清发现了什么、在哪、怎么修。一遍过。
- 原因不明就说原因不明,不要猜。

## 简单格式
- 不用破折号、弯引号或装饰性 Unicode 符号。
- 只用普通连字符和直引号。
- 内容需要时,自然语言字符(带音标字母、CJK 等)可以保留。
- 代码输出必须可安全复制粘贴。
