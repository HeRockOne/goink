# 缓存命中率修复实施记录（NS 落库协议 + 子 agent fork + 全量统计口径）

> 日期：2026-08-06（实施完成，含 2026-08-06 二次修正）
> 依据：`docs/design/cache-hit-fix-implementation.md` 原方案（K=3 落库清理）+ 实施中发现的问题
> 状态：已实施（commit cf49ff6 / 46a0189 / 后续修正）
> 说明：本文是原方案（P1 NS 落库协议 + P2 压缩 NS 双份）的实施记录。**最终采用"NS 落库、永不清理"**——经过三轮迭代才收敛：第一轮（NS 不落库+请求尾临时拼）破坏完整前缀匹配导致 89%，第二轮（NS 进内存）只修轮内，第三轮恢复落库才同时满足轮内+跨轮完整匹配

---

## 一、目标协议（不变）

让"上一轮完整请求字节 = 本轮请求前缀"，仅尾部增量 miss。轮次 N+1 请求 = 轮次 N 请求字节 + 新增内容。

**关键机制**：MiniMax/DeepSeek 按"请求结束位置落盘 + 完整前缀单元匹配"命中——后续请求必须**完整匹配**某条落盘条目。上一轮请求末尾是什么，本轮请求对应位置必须还是什么；任何"本轮新增内容插到上轮内容之前"（含动态尾部 NS 被新内容越过）都会让上轮条目无法被完整匹配，命中率退化为公共前缀。

## 二、最终协议（NS 落库、永不清理）

```
轮次 N   请求: [固定前缀][userN][NS_N][assistantN][toolN]
轮次 N+1 请求: [固定前缀][userN][NS_N][assistantN][toolN][userN+1][NS_{N+1}]
                           ↑↑↑↑ 轮次 N 请求的完整字节 → 全部命中 ↑↑↑↑
```

- NS 是 `role=system, to_api=true, to_frontend=false` 的普通消息（ExtraMetadata 带 kind 标记），**紧跟 user 消息之后落库**（ID 序 = [user][NS][assistant...]）
- **永不清理**（不做 to_api=false）：旧 NS 在历史中字节不变 → 可命中；每轮只 miss 最新 NS 本身。历史上尝试过的 K=3 清理会让消息数组变化（删除位置起全部 miss，每 turn 首轮 4-5 万）
- 上下文膨胀由**压缩兜底**：压缩重建时旧 NS 随摘要清除（retainMessages 跳过 NS），只保留最新一份 NS 落库到新版本末尾
- 压缩后第一轮请求与压缩前前缀不同属**一次性重建成本**（文档认可），之后恢复完整匹配

## 三、迭代记录（为什么改了三次）

| 版本 | 方案 | 结果 |
|---|---|---|
| 原方案（08-04） | NS 落库 + K=3 清理 | turn 首轮 miss 4-5 万（清理破坏前缀），累计 96% |
| 第一轮（08-06） | NS 不落库，请求尾临时拼 | 本轮新内容插到 NS 之前 → 上轮条目无法完整匹配 → 退化为公共前缀，**命中率 89%**（hit 恒定 73-114K 台阶） |
| 第二轮 | NS 进 opts.Messages 内存（不落库） | 只修轮内（NS 位置对），跨轮仍断（DB 历史无 NS）→ 依旧 89% |
| **第三轮（最终）** | **NS 落库、永不清理** | 轮内+跨轮完整匹配同时成立 → 恢复单调递增命中 |

失败教训：
1. **append 污染**（第一轮实现）：`reqMsgs := opts.Messages; append(...)` 原地写底层数组，NS 残留混入历史（Go slice 别名坑）——已修复（强制复制），但并非 89% 主因
2. **NS 位置**（真正根因）：完整前缀匹配下，NS 必须与"上轮请求末尾"位置一致——只有落库进历史才能保证

## 四、改动清单（最终实施）

1. **`app/chat.go`**：NS 落库进 messages（user 后、kind 标记），**永不清理**；不再写 session.extra_metadata
2. **`internal/agent/agent.go`**：Run 不再手动 append NS（历史已含）；删除 loadNovelState；`computePrefixHash` 只哈希前导 system + 前缀变化块哈希诊断
3. **`internal/agent/compress.go`**：
   - `generateSummary` 用 ChatStream 带全量工具（压缩请求 = 主请求 + 压缩指令，前缀命中）
   - `persistCompression` 恢复 NS 落库（新版本末尾）
   - `retainMessages` 跳过旧 NS（压缩后只留最新一份）
4. **`internal/agent/tokens.go`**：主/子统一累计命中率；UpdateMessageUsage 按 agent_type 分写；usage_ratio 本地估算
5. **`internal/agent/phase_gate.go`**：进入 write 重置 wordCountOK
6. **`internal/session/store.go`**：UpdateMessageUsage 加 agentType

## 五、验证结果

```
NS 落库+K=3（修复前）:  turn 首轮 miss 4-5 万，累计 96%
NS 尾部临时拼（第一轮）:  hit 恒定 73-114K 台阶，累计 89%（完整匹配失效）
NS 落库永不清理（最终）:  轮内 99%+，跨轮完整匹配，累计应回 98%+
```

判据：每调用 hit 增量随轮次递增（不再恒定）；长会话命中率随轮次单调上升。

## 六、回退方案

- 纯逻辑改动、无 schema 变更：revert 上述 commit 即可；历史 NS 快照消息保留在消息表（to_api=true，占序列）——回退后不影响正确性
- 若 NS 膨胀观测不佳：优先调压缩阈值，勿恢复清理（清理必断前缀）

## 相关文档

- `docs/architecture/cache-hit-mechanism.md` — 缓存机制与实测
- `docs/architecture/token-injection.md` — 注入构成与层级
- `docs/architecture/phase-gate.md` — 门禁字数校验
- `docs/adr/0001-prompt-caching.md` — 前缀稳定化决策（本文不推翻）
