# 缓存命中机制详解（DeepSeek 前缀缓存）

> 日期：2026-07-31
> 目的：解答"门禁阶段重复读 skill 是否命中缓存""改 skill 是否破坏当前窗口缓存"等疑问
> 适用：DeepSeek / OpenAI 兼容格式的 Prompt Caching（自动启用，无需手动标记）

---

## 一、核心机制：缓存是"字节级精确前缀匹配"，不是相似度

**错误理解**：文本相似度查重，90% 相似 = 命中 90%。

**正确机制**（DeepSeek 官方 KV Cache）：
- 缓存对象是**请求开头部分**的计算结果
- 匹配条件是**逐字节完全相同**——任何一处不同，从那一点开始全部 miss
- 不存在"部分命中率"，只有"完全相同"或"不同"

```
请求1: [AAA][BBB][CCC]          → 全部写入缓存
请求2: [AAA][BBB][DDD]          → [AAA][BBB] 命中，[DDD] miss
请求3: [AAA][BBX][CCC]          → [AAA] 命中，[BBX][CCC] miss（BBB 变 BBX，从此断）
```

---

## 二、系统提示词层级（固定前缀 vs 动态）

首次对话注入 4 段 system 消息（`app/chat.go` `writeSystemMessages`）：

```
L1  Identity        → 1,340 tokens   人设/流程/规范（agentcfg/identity.go）
L2  Always skills   → ~2,088 tokens   always 模式 skill 全量正文
L3  Skill catalog   →  ~1,150 tokens   auto 模式 skill 的 name+description 目录
L4  NovelState      → 落库进消息历史    小说状态快照（紧跟 user 消息之后，永不清理）
```

**固定前缀 = 工具定义（全量 JSON）+ L1 + L2 + L3 ≈ 20,899 tokens**（2026-08-06 实测 `fixed_prefix_tokens`）。工具定义在 API payload 顶层 `tools` 字段,`marshalPayload`（`stream.go`）将其提到 JSON 最前,确保始终命中缓存。L4 NovelState 落库进消息历史（紧跟 user 消息之后）。
- `writeSystemMessages` **只在创建新 session（isNew）时执行**（`chat.go`）
- 固定前缀写入 messages 表后**不再重写**，保证缓存稳定命中
- `computePrefixHash` 只哈希前导 system 消息 + 工具名（历史中的 NS 不参与，避免误报）

**NS 落库协议（完整前缀匹配的关键）**：MiniMax/DeepSeek 按"请求结束位置落盘 + 完整前缀单元匹配"命中——上一轮完整请求（含其末尾的 NS_N）必须是本轮请求的前缀。因此 NS 必须作为消息**落库进历史**（紧跟 user 之后、永不清理）：旧 NS 字节不变可命中，每轮只 miss 最新 NS。任何"NS 不落库/请求尾临时拼"都会让本轮新内容插到上轮 NS 之前，上轮条目无法被完整匹配 → 命中率退化为公共前缀（实测 89%）。历史膨胀由压缩兜底。

---

## 三、命中范围：整段前缀，不止固定前缀

DeepSeek 缓存匹配的是"请求开头到最后一个与前一次相同的位置"，**通常覆盖所有历史消息**：

```
请求1: [固定前缀][历史][user: 写一章][tool: read main-core-writing-kernel][assistant][tool: edit][NS@尾部]
请求2: [固定前缀][历史][user: 写一章][tool: read ...][assistant][tool: edit][user: 继续][NS@尾部]
         ↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑↑
         与请求1逐字节相同 → 全部命中（包括 read 的 skill、写过的正文、设定）
         [user: 继续][NS] ← 新增 → miss
```

**命中的是"前缀位置"，不是"内容查重"**：
- 所有"位置靠前、内容未变"的历史消息都命中——读过的 skill、写过的设定、正文、对话
- miss 的只有"本轮新追加的那一小段"（新 user 输入、新工具结果、尾部动态 NS）

---

## 四、具体场景推演

### 4.1 门禁阶段重复读 skill → 命中

```
请求N:   [固定前缀][历史][tool: read main-core-writing-kernel #1]...     → #1 写入缓存
请求N+1: [固定前缀][历史][read #1][assistant][tool: read #2]... → #1 命中，#2 新追加 miss
请求N+2: [固定前缀][历史][read #1][...][read #2][user: 继续]...  → #1 #2 都命中，只有新增 miss
```

**结论**：重复读 skill 不浪费——第一次读的在缓存里，第二次读虽新追加（miss 一次），读完后它又成为"已缓存的历史"，下次请求命中。

### 4.2 当前窗口改 skill → 零影响

| 改动 skill 的方式 | 当前窗口缓存 | 影响范围 |
|-----------------|-------------|---------|
| 改/新增 skill（含改 mode 为 always） | **零影响**（前缀已固化在 messages 表） | 只影响**新窗口**（isNew 重写前缀） |
| 触发**压缩** | 那轮重建前缀（`compress.go:98` 热加载最新 skill） | **当前窗口**压缩后生效 |

**热加载的真相**：skill 是热加载的（`store.go:60` `ListMeta` 每次读磁盘），但热加载只影响"下一次读取路径"（前端列表 / read 工具 / 压缩 / 新窗口），**不影响已固化的当前窗口缓存前缀**。

### 4.3 中途开关门禁 → 零影响

门禁开/关只是代码里的拦截开关（DB 配置），不注入、不修改任何消息。**缓存前缀不受影响**。

### 4.4 多章循环（回到 prepare 再写下一章）→ 连续命中

```
第1章...maintain→prepare
第2章: [固定前缀][第1章全部历史][user: 写一章][NS@请求尾部]
       → 第1章全部历史命中 ✓，只有新 user + NS miss
```

### 4.5 子 agent（review/memory）→ fork 完整主历史

子 agent 请求 = **完整主会话历史原文** + 尾部追加（Anthropic fork 模式完整版），按稳定前缀顺序拆消息：

```
[主历史] [身份（常量）] [sub-* 技能（review 专属，常量）] [NS（动态）] [指令（动态）]
```

- 首轮 = 上一轮主请求的完整字节 → **整个主会话缓存条目命中**（15 万+），miss 只余身份+技能+NS+指令（几 K）
- **sub-* 技能自动注入（2026-08-08）**：review 子代理启动时自动注入所有 `sub-` 前缀技能（如审稿标准、反 AI 词表），技能内容为常量字节且放在 NS 之前 → 第 2 次 review 起命中；替代旧的"子代理自己 read 技能"（read 结果在尾部动态区，每次 miss 3.4K）
- 正文/设定已在历史里 → 子 agent 直接从上下文读取，不再重复 read
- 子 agent 内部工具循环在尾部增长，轮间连续
- 子 agent 消息不落库（ToAPI=false），不影响主历史

### 4.6 prompt_cache_key：路由粘性（偶发全 miss 的解法）

OpenAI 兼容多节点负载均衡下，相同前缀的请求可能被路由到不同后端节点，各节点缓存不共享 → 偶发全 miss（实测：23 秒间隔、上轮 99.9% 命中、下轮 hit=0 miss=15.5 万）。opencode 对 openai-compatible 默认发送 `promptCacheKey`（= sessionID，PR #22569），把相同前缀路由到同一节点。Goink 已对齐：所有请求携带 `prompt_cache_key = sessionID`。不支持的端点忽略未知参数。

### 4.7 上下文快满：压缩 vs 新窗口

| 操作 | 缓存影响 |
|------|---------|
| **压缩** | 重建系统消息 + 摘要（热加载最新 skill）→ 压缩请求已做缓存对齐（与主循环相同 system+tools+历史，只在末尾追加压缩指令），**前缀命中、只 miss 压缩指令**；压缩后新版本首轮有一次性重建成本 |
| **新开窗口** | 系统前缀用最新 skill → **首轮 miss**，之后恢复命中 |

两者都会在"变更点"有一次缓存重建成本（几厘钱级），之后都正常。

### 4.8 工具定义（tools）在 payload 最前，始终命中缓存

API 请求体（`marshalPayload`，`stream.go:74`）结构：

```json
{"tools":[...],"model":"goink-sim","messages":[...],"stream":true,"stream_options":{"include_usage":true}}
```

工具定义在顶层 `tools` 字段，`marshalPayload` 强制将其提到 JSON 最前（`model` 和 `messages` 之前）。工具定义是常量（跨请求不变），因此始终是字节级公共前缀的一部分，**每次请求都命中缓存**。

固定前缀（缓存命中）：`{"tools":<定义>,"model":"...","messages":[{L1},{L2},{L3}`——跨越工具定义、L1 Identity、L2 Always skills、L3 Skill catalog，直到动态消息开始处。共约 20,899 tokens。

---

## 五、实测数据佐证

2026-08-06（MiniMax M2.5，`D:\Goink\goink.log`）：

```
主会话轮内: hit 增量 19-24 万/轮, miss 300-8,000/轮 → 当轮命中 99.3-99.9%
turn 首轮:  miss 4-5 万（旧实现 NS 清理导致；NS 动态注入后消失）
子 agent:   旧实现全 miss 2 万/次；fork 模式后命中主前缀
```

累计命中率 = `Σhit / (Σhit + Σmiss)` 全量口径（主+子 agent 请求都计入，与面板一致）。

---

## 六、关键结论

1. **缓存是字节级精确前缀匹配**，不是相似度查重
2. **命中范围覆盖所有历史消息**，不止固定前缀——创作中累积的设定/正文/skill 内容恰是命中主体
3. **门禁重复读 skill 命中**——第一次读的在缓存里，重复读只有"新追加那次" miss 一次
4. **当前窗口改 skill 零影响**——热加载服务"下一次读取"，不破坏已固化的缓存前缀
5. **消息历史必须纯 append-only**——任何删除/编辑（如旧 NS 快照清理）都会让前缀从删除位置起全部失效；动态状态（NS）走请求尾部注入
6. **子 agent 复用主会话前缀（fork 模式）**——首轮命中主前缀，不再全 miss
7. **命中率统计全量计入**（主+子 agent），消息级审计按 agent_type 分写，面板与状态栏口径一致

---

## 相关文档

- `docs/architecture/token-injection.md` — 系统提示词层级划分 + tokencount 统计
- `docs/archive/billing-test-report.md` — 缓存命中率实测数据
- `docs/adr/0001-prompt-caching.md` — Prompt Caching 前缀稳定化决策
