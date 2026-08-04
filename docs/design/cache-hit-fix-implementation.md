# 缓存命中率技术修复方案（P1+P2 实施级）

> 日期：2026-08-04
> 依据：`docs/reports/cache-hit-rate-analysis.md`（根因与收益分析）+ 代码实读
> 状态：设计方案，待实施确认
> 范围：P1（NovelState 落库协议）+ P2（压缩 NS 双份 Bug），一并实施，二者必须同改

---

## 一、目标协议：让"上一轮完整请求字节 = 本轮请求前缀"

DeepSeek 硬盘缓存的匹配单位是"完整前缀单元"。要达到 Chat-deep 实测的 99.79% 追加式命中，必须做到：**轮次 N+1 请求的字节序列 = 轮次 N 请求的字节序列 + 新增内容**（仅尾部增量 miss）。

当前实现（`app/chat.go:206-212`）把 NovelState 每轮只追加到**内存**消息末尾（不落库），导致轮次 N 请求是 `[历史][userN][NS_N]`，轮次 N+1 请求是 `[历史][userN][assistantN]...` —— 分叉点钉死在每条 user 消息后，上轮正文/工具永不命中。

### 目标消息排布（修复后）

```
轮次 N   请求: [固定前缀][userN][NS_N][assistantN][toolN]
轮次 N+1 请求: [固定前缀][userN][NS_N][assistantN][toolN][userN+1][NS_{N+1}]
                           ↑↑↑↑ 轮次 N 请求的完整字节 → 全部命中 ↑↑↑↑
轮次 N+2 请求: [前缀][user1]...[NS_{N+1}][assistantN+1][toolN+1][userN+2][NS_{N+2}]
```

- NS 是 `role=system, to_api=true, to_frontend=false` 的普通消息，**按 ID/时序列入轮次序列**
- 每轮写入顺序（DB ID 决定序列位置）：`[该轮 userMsg]` 之后紧跟 `[NS]`，随后 agent.Run 的 `appendMsg`（assistant/tool/inject）自然落在其后
- 依赖 `GetMessagesForAPI` 顺序加载：修复为 `Order("id ASC")`（见 3.4）

### 时序规则（每轮 3 步）

1. **chatImpl 事务内**：写入 `userMsg` 后，紧接着 `Create(nsMsg)`（`role=system`, `to_api=true`, `to_frontend=false`, `ExtraMetadata={"kind":"novel_state"}`, `TurnID=当前轮`）
2. **清理过期快照**：同一事务内，把"早于最近 K 份"的 NS 快照置 `to_api=false`（只保留最近 K 份，见下）
3. **加载**：`loadAPIMessages` 从 DB 加载完整序列（含本轮新 NS）→ `agent.Run` 正常追加

### NS 快照保留策略：K=3（可配置常量）

- **K=∞（全保留）**：字节最连续、命中最高，但每份 1-3K token；50 轮 ≈ 50-150K，快压爆上下文。否决。
- **K=1（只留最近一份）**：上下文省，但旧 NS 消失位置成为分叉点，miss ≈ 一整轮（等价现状）。否决。
- **K=3（留最近 3 份）**：miss 有上界 = 最近 K 轮 + 本轮新增，随会话变长命中率单调逼近 95%+；上下文额外开销 ≈ 3-9K，可控。**采用**。

  K 的效果（会话 N 轮后）：
  - 现状：miss ≈ (N-1) 轮全部尾部，命中恒定 17.5-20K → 命中率**随会话下降**（实测 93%→89% 正是此趋势）
  - K=3：miss ≤ 3 轮 + 本轮，命中含全部更早历史 → 命中率**随会话上升**（长会话 95%+）

  压缩会重建消息并把快照清到只剩压缩时那份，天然与 K 策略互补。

---

## 二、改动清单

### 2.1 `app/chat.go` — 每轮落库 NS + 清理过期快照

替换 `chat.go:206-212` 的内存追加逻辑：

```go
// 8.5 动态注入 NovelState：改为落库到轮次末尾（P1 修复）
novelState, err := agentcfg.NovelState(a.session.DB, input.NovelID)
if err != nil { /* 原有日志 warn */ }
if novelState != "" {
    // a) 清理过期快照：只保留最近 K 份（含本轮写入后共 K 份）
    //    语义：先置 false 旧旧的，再写新的
    // 实现建议（同一事务内）：
    //   UPDATE messages SET to_api=false
    //   WHERE session_id=? AND (extra_metadata LIKE '%"kind":"novel_state"%')
    //     AND id NOT IN (SELECT id FROM messages WHERE session_id=? AND to_api=true
    //                     AND extra_metadata LIKE '%"kind":"novel_state"%'
    //                     ORDER BY id DESC LIMIT K-1)
    // b) 写本轮 NS（紧跟 userMsg 之后，保证 ID 序列 = [user][NS][assistant...]）
    //   Create(&session.Message{
    //     SessionID: sess.SessionID, TurnID: turnID, Role: "system",
    //     Content: novelState, Version: sess.ActiveVersion,
    //     ToAPI: true, ToFrontend: false, AgentType: "main",
    //     ExtraMetadata: `{"kind":"novel_state"}`,
    //   })
}
// 之后照常 loadAPIMessages → 请求 := 完整序列（已含本轮 NS）
```

- **放置时序**：NS 必须在事务中 **Create 在 `userMsg` 之后**（二者 ID 递增），随后 `agent.Run` 的 append 在其后 → 自然得到 `[user][NS][assistant][tool]` 排布。
- 现有 `chat.go:171-194` 的事务块与写入可在同一事务内完成，保证原子性（NS 数量级：1-3K 字符串传输，无性能顾虑）。
- **删除**：`chat.go:206-212` 的 `messages = append(messages, {role:system, content:novelState})` 整个删除。

### 2.2 `internal/agent/compress.go` — P2 修复：NS 不再写系统区

- `persistCompression`（`compress.go:249-254`）：**删除** `msg("system", novelState, ...)` 那一段 —— 压缩后新版本不再把 NS 写进系统区。
- `retainMessages`（`compress.go:337-393`）：过滤掉 `ExtraMetadata.kind == "novel_state"` 的消息（不在 retained 中复制旧 NS 快照），避免压缩后残留过期快照。
- 压缩后的首轮请求将不含 NS（chatImpl 下一轮会写新 NS），因应 P1 协议一致：新 NS 由 chatImpl 在 turn 开头按 2.1 方案补写。
- 保留 `novelState` 变量（`compress.go:92`）生成逻辑，改为只用于打印/校验，不写入 DB。

### 2.3 `internal/agent/agent.go` — 无需改动

- `appendMsg` 与 Run 循环不动：NS 由 chatImpl 落库后，`loadAPIMessages` 自然包含，`opts.Messages` 顺序即 DB 序列。
- `computePrefixHash`（agent.go:808）**不变**：仍只哈希 role=system 消息 + 工具名。本方案新增的 NS 属 system 消息，哈希会自动覆盖到 NS 变化 → 前缀变化告警仍然有效（这正是期望行为：NS 每轮变化，哈希产生 warn，但不再造成整段历史失效）。

### 2.4 `internal/session/store.go` — 排序列改为 `id ASC`

`GetMessagesForAPI`（`store.go:251`）当前 `Order("created_at ASC")`：同一微秒创建的多条消息排序未定义（SQLite 相等值按 rowid 兜底，但依赖它不安全）。改为：

```go
Order("id ASC")
```

保证 `[user][NS][assistant][tool]` 的 ID 顺序即请求字节顺序。`GetMessagesForFrontend` 同理可改（不影响 API，改一致性），`GetAllMessages` 一并统一。

### 2.5 常量与标记

- 新增常量 `novelStateKind = "novel_state"`（ExtraMetadata kind），供 2.1/2.2 识别；不引入新表/新列，复用 `extra_metadata`。
- `keepNSEntry = 3`（P1 的保留份数），放 package 级常量便于调优。

## 三、兼容性核对

| 机制 | 影响 | 处理 |
|------|------|------|
| 子 agent（`run_subagent`） | 独立上下文/独立缓存，不经过 chatImpl | 不受影响 |
| `generateTitle` | 独立请求、独立 system prompt | 不受影响 |
| 前端 ContextRing | NS `to_frontend=false`，前端不渲染 | 无前端改动 |
| 移动端 API | ChatWithCallback 复用 chatImpl | 自动生效 |
| 回滚（rollback） | 只回滚 git + turn_commits，**不删 messages** | NS 快照随序列保留，字节连续不受影响；实施时另核对：回滚后 `last_turn_id` 回退且 `NextTurn` 续传时，NS 快照的归属轮次仅用于展示无关，无影响 |
| 版本回退（active_version 压缩） | NS 快照 `version=当前值`，加载按 version 过滤 | 压缩后旧版本 NS 保留旧 version，不影响新版本 |
| DeepSeek/商汤 TTL | 不涉及请求结构 | 无 |
| 「不删创作规则」红线 | 每种改动只是消息**写入位置/可见性**，不改任何 system/skill 内容 | 合规 |

## 四、验证方案

### 4.1 单元测试

- `session/store_test.go`（新增）：`GetMessagesForAPI` 按 `id ASC` 顺序（插入不同 createdAt 时）。
- `agent/compress_test.go`（新增）：`retainMessages` 不包含 `kind=novel_state` 消息；`persistCompression` 不产系统区 novelState 行。

### 4.2 实测（复用 `archive/billing-test-report.md` 流程）

1. 新会话 + 4 轮问答，读每调用 hit/miss。
2. 判据：**每调用 hit 增量随轮次递增**（不再是恒定 20480），即历史正文/工具结果进入前缀。
3. 长会话（≥15 轮、含若干轮写作正文）后：命中率应 ≥ 93%，且随轮次**单调上升**（对照修复前单调下降）。
4. 触发一次手动压缩，验证：压缩摘要请求 hit 增量大（前缀 fork 命中）；压缩后首轮 miss（预期）；后续轮次恢复递增。
5. 用不同 `goink.md` 内容验证 NS 变化只在末尾分叉、不影响正文区命中。

## 五、回退方案

- 纯协议/SQL 级改动、无 schema 变更：回退 = revert 2.1-2.4 的改动即可，历史 NS 快照消息可留着（`to_api=false` 后不占序列）。
- 若 K 值观测不佳：临时调 `keepN`（1 恢复旧行为、5 更激进）即可，无需改代码结构。

## 相关文档

- `docs/reports/cache-hit-rate-analysis.md` — 根因与行业交叉验证
- `docs/architecture/token-injection.md` — 注入构成与分层
- `docs/archive/billing-test-report.md` — 现有实测流程（复用验证）
- `docs/adr/0001-prompt-caching.md` — 前缀稳定化决策（本文不推翻，仅落实"末尾落库形成完整上一请求单元"）