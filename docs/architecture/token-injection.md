# Token 注入构成分析 + tokencount 使用说明

> 日期：2026-07-31
> 工具：`tokencount/`（Go）
> 口径：仅统计工作目录内的系统提示词注入，不含用户级 skill、小说级 NovelState

---

## 一、tokencount 使用说明

### 运行

```powershell
# 需要 CGO 编译（mcp_tools 依赖 sqlite-vec），把 include 指向本机 go-sqlite3 的 sqlite3.h
$env:CGO_CFLAGS = "-IC:\Users\Sophia\go\pkg\mod\github.com\mattn\go-sqlite3@v1.14.44"
go run ./tokencount
```

> `CGO_CFLAGS` 路径随 go-sqlite3 版本变化，用 `go list -m github.com/mattn/go-sqlite3` 查实际版本目录。

### 统计范围（仅工作目录内）

| 组成部分 | 来源 |
|---------|------|
| Identity | `internal/agentcfg/identity.go` → `mainAgentSystem1` |
| Always skills | 扫描 `skills/*.md` 中 `mode: always` 的正文（不含 YAML frontmatter） |
| Skill catalog | 扫描 `skills/*.md` 中 `mode: auto` 的 name + description，经 `BuildSkillCatalog` 格式化 |
| 工具定义 | `mcp_tools.RegisterAllTools` + `registry.OpenAI(nil)` 生成完整 function-calling JSON |

**不统计**：用户级 skill（`~/.goink/skills`）、小说级 skill、NovelState（`goink.md`）——这些在工作目录之外，属于运行时动态内容。

---

## 二、实测构成（2026-08-01，tokencount 实测，含 builtin 目录）

```
首轮对话注入合计：~18,138 tokens
├─ 工具定义（57 个工具的完整 JSON Schema）  12,924 (71.2%)
├─ Identity（mainAgentSystem1）               1,340 (7.4%)
├─ Always skills（core-core-main-writing-kernel + core-core-main-ai-communication-standard 正文）  2,102 (11.6%)
└─ Skill catalog（37 auto skill 的 name+desc）     1,772 (9.8%)
```

| 组成部分 | Tokens | 占比 | 说明 |
|---------|--------|------|------|
| 工具定义 | 12,924 | 71.2% | 57 个工具 name + description + parameters schema（含 `$defs` 内联） |
| Identity | 1,340 | 7.4% | 系统提示词（人设/创作流程/阶段门禁/技能说明） |
| Always skills | 2,102 | 11.6% | core-core-main-writing-kernel 1,994 + core-core-main-ai-communication-standard 108 |
| Skill catalog | 1,772 | 9.8% | 37 auto skill 仅注入 name + description |
| **合计** | **18,138** | 100% | |

---

## 三、系统提示词层级划分

首次对话注入 4 段 system 消息（`app/chat.go` `writeSystemMessages`），按稳定前缀顺序：

```
L1  Identity        → 1,340 tokens   人设/流程/规范（agentcfg/identity.go）
	L2  Always skills   → 2,146 tokens   always 模式 skill 全量正文
	L3  Skill catalog   → 525+1,152 tokens   auto 模式 skill 的 name+description 目录（skills/ 8 auto + builtin 29 auto）
L4  NovelState      → 动态注入       小说状态快照（放 user 消息之后，走缓存前缀外）
```

### 三种 Mode 的注入策略（`internal/skill/types.go`）

| Mode | 是否进目录 | 注入级别 | 触发方式 |
|------|-----------|---------|---------|
| `always` | ❌ | **全量正文**注入 system 消息（L2） | 常驻生效 |
| `auto` | ✅ | 仅 name + description（L3） | AI 按需 `read` 加载 |
| `manual` | ❌ | 不注入 | 仅用户 `/` 触发 |

### 关键设计

1. **L1+L2+L3 = 稳定前缀**（约 3.2K tokens）→ 写入 messages 表，保证 Prompt Caching 命中
2. **L4 NovelState 刻意排除在稳定前缀外** → 小说状态每轮变化，放后面避免破坏缓存前缀（`app/chat.go` 动态注入 + `internal/agent/agent.go` 的 `computePrefixHash`）
3. **skill 正文不占常驻 token** → 29 个 auto skill 只注入 ~1,150 tokens 的目录，正文按需加载，这是省 token 的核心策略

---

## 四、缓存命中与计费

- **首轮**：~17,500 tokens 全部按未命中（全额）计费——首轮无缓存可命中
- **后续轮次**：稳定前缀被复用命中缓存 → 按折扣价计费（DeepSeek 约 10%）
- **实测命中率**：89-93%（商汤 sensenova-6.7-flash-lite，见 `archive/billing-test-report.md`）

**结论**：工具定义虽是 80% 的注入大头，但作为稳定前缀，每轮命中缓存享受折扣，成本侧已被 Prompt Caching 抵消大部分。

---

## 五、优化空间（按收益排序）

| 方向 | 可省 | 手段 | 现状 |
|------|------|------|------|
| 工具定义裁剪 | 数 K | 按阶段/意图只注入部分工具 | 已用 `allowed_tools` 运行时限制，但发送的 JSON 仍全量（刻意保缓存前缀稳定） |
| 工具 schema 精简 | ~2-3K | 合并重复 `$defs`、精简 description | 未实施 |
| Catalog 瘦身 | 少量 | 29 个 auto skill 目录压缩 | 未实施 |

> 完整方案见 `design/token-optimization-plan.md`。
