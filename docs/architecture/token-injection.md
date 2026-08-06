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

> 注意：tokencount 只扫描 `skills/` 目录。内置 41 个 skill（含 8 新增）通过 `//go:embed` 加载，其中 37 auto 进入 catalog，4 manual 不出现。以下实测为 tokencount 直接输出，实际总注入另加内置 catalog 约 1,152 tokens。

---

## 二、实测构成（2026-08-01，`skills/` 目录 tokencount 实测）

```
首轮对话注入合计（仅 skills/ 目录）：~16,935 tokens
├─ 工具定义（57 个工具的完整 JSON Schema）  12,924 (76.3%)
├─ Identity（mainAgentSystem1）               1,340 (7.9%)
├─ Always skills（main-core-writing-kernel + main-core-ai-communication-standard 正文）  2,146 (12.7%)
└─ Skill catalog（8 auto skill 的 name+desc）       525 (3.1%)
```

| 组成部分 | Tokens | 占比 | 说明 |
|---------|--------|------|------|
| 工具定义 | 12,924 | 76.3% | 57 个工具 name + description + parameters schema（含 `$defs` 内联） |
| Identity | 1,340 | 7.9% | 系统提示词（人设/创作流程/阶段门禁/技能说明） |
| Always skills | 2,146 | 12.7% | main-core-writing-kernel 2,038 + main-core-ai-communication-standard 108 |
| Skill catalog | 525 | 3.1% | 8 auto skill（仅 `skills/` 目录新增，不含内置 29 auto） |
| **合计** | **16,935** | 100% | + 内置 catalog ~1,152 = 实际总注入 ~18,087 |

---

## 三、系统提示词层级划分

首次对话注入 4 段 system 消息（`app/chat.go` `writeSystemMessages`），按稳定前缀顺序：

```
L1  Identity        → 1,340 tokens   人设/流程/规范（agentcfg/identity.go）
	L2  Always skills   → 2,146 tokens   always 模式 skill 全量正文
	L3  Skill catalog   → 525+1,152 tokens   auto 模式 skill 的 name+description 目录（skills/ 8 auto + builtin 29 auto）
L4  NovelState      → 落库进消息历史    小说状态快照（紧跟 user 消息之后，永不清理，压缩兜底）
```

### 三种 Mode 的注入策略（`internal/skill/types.go`）

| Mode | 是否进目录 | 注入级别 | 触发方式 |
|------|-----------|---------|---------|
| `always` | ❌ | **全量正文**注入 system 消息（L2） | 常驻生效 |
| `auto` | ✅ | 仅 name + description（L3） | AI 按需 `read` 加载 |
| `manual` | ❌ | 不注入 | 仅用户 `/` 触发 |

### 关键设计

1. **L1+L2+L3 = 稳定前缀**（约 3.2K tokens）→ 写入 messages 表，保证 Prompt Caching 命中
2. **L4 NovelState 落库进历史（紧跟 user 之后，永不清理）** → 完整前缀匹配的关键：上一轮完整请求（含 NS）必须是本轮前缀；旧 NS 字节不变可命中，每轮只 miss 最新 NS。历史上试过的 K=3 清理（消息数组变化 → 删除位置起全 miss）和"请求尾临时拼"（新内容插到 NS 前 → 完整匹配失效，89%）均不可行
3. **skill 正文不占常驻 token** → 29 个 auto skill 只注入 ~1,150 tokens 的目录，正文按需加载，这是省 token 的核心策略

---

## 四、缓存命中与计费

- **首轮**：~17,500 tokens 全部按未命中（全额）计费——首轮无缓存可命中
- **后续轮次**：稳定前缀被复用命中缓存 → 按折扣价计费（DeepSeek 约 10%）
- **实测命中率**：主会话轮内 99%+；累计命中率受 turn 首轮（NS 更新）、子 agent 首轮（独立小上下文）影响。2026-08-06 起子 agent 复用主会话前缀（fork 模式），命中率统计全量计入（主+子），见 `design/cache-hit-fix-implementation.md`

**结论**：工具定义虽是 80% 的注入大头，但作为稳定前缀，每轮命中缓存享受折扣，成本侧已被 Prompt Caching 抵消大部分。

---

## 五、优化空间（按收益排序）

| 方向 | 可省 | 手段 | 现状 |
|------|------|------|------|
| 工具定义裁剪 | 数 K | 按阶段/意图只注入部分工具 | 已用 `allowed_tools` 运行时限制，但发送的 JSON 仍全量（刻意保缓存前缀稳定） |
| 工具 schema 精简 | ~2-3K | 合并重复 `$defs`、精简 description | 未实施 |
| Catalog 瘦身 | 少量 | 29 个 auto skill 目录压缩 | 未实施 |

> 完整方案见 `design/token-optimization-plan.md`。
