# 缓存命中率修复实施记录（NS 落库 + 子 agent fork + prompt_cache_key + 全量统计口径）

> 日期：2026-08-06（实施完成，含 08-06/08-07 多次修正）
> 依据：`docs/design/cache-hit-fix-implementation.md` 原方案（K=3 落库清理）+ 实施中发现的问题
> 状态：已实施（commit cf49ff6 / 46a0189 / 398b5f7 / c4104d2 / 7e6de62）
> 说明：最终协议 = **NS 落库永不清理 + 子 agent fork 完整主历史 + prompt_cache_key 路由粘性**。NS 经历三轮迭代（89% 教训），子 agent 经历两轮（重复 read miss），路由粘性为 websearch 对照 opencode 后补上

---

## 一、目标协议（不变）

让"上一轮完整请求字节 = 本轮请求前缀"，仅尾部增量 miss。轮次 N+1 请求 = 轮次 N 请求字节 + 新增内容。

**关键机制**：按"请求结束位置落盘 + 完整前缀单元匹配"命中——后续请求必须**完整匹配**某条落盘条目。上一轮请求末尾是什么，本轮请求对应位置必须还是什么；任何"本轮新增内容插到上轮内容之前"都会让上轮条目无法被完整匹配，命中率退化为公共前缀（实测 89% 恒定台阶）。

## 二、最终协议

```
轮次 N   请求: [固定前缀][userN][NS_N][assistantN][toolN]
轮次 N+1 请求: [固定前缀][userN][NS_N][assistantN][toolN][userN+1][NS_{N+1}]
                           ↑↑↑↑ 轮次 N 请求的完整字节 → 全部命中 ↑↑↑↑
```

- **NS**：`role=system` 消息，紧跟 user 之后**落库**（kind 标记），**永不清理**——旧 NS 字节不变可命中，每轮只 miss 最新 NS；膨胀由压缩兜底
- **prompt_cache_key**：所有请求携带 `prompt_cache_key = sessionID`（opencode 对 openai-compatible 的默认做法）——相同前缀路由到同一后端节点，消除负载均衡漂移导致的偶发全 miss（小米 MiMo 直连实测：不带 key 时偶发全 miss 15.5 万/次，23 秒间隔；走中转/openrouter 95% 命中）
- **子 agent**：请求 = **完整主历史原文** + 尾部 `[身份+NS][指令]`（fork 模式完整版）——首轮完整命中主会话缓存，正文/设定直接从历史读取，不再重复 read（旧实现每次 review 重复读相同内容，每轮 miss 4-10K）

## 三、迭代记录

| 版本 | 方案 | 结果 |
|---|---|---|
| 原方案（08-04） | NS 落库 + K=3 清理 | turn 首轮 miss 4-5 万（清理破坏前缀），累计 96% |
| 第一轮 | NS 不落库，请求尾临时拼 | 新内容插到上轮 NS 之前 → 完整匹配失效 → **89%**（hit 恒定 73-114K 台阶） |
| 第二轮 | NS 进 opts.Messages 内存 | 只修轮内，跨轮仍断 → 依旧 89% |
| 第三轮 | **NS 落库、永不清理** | 轮内+跨轮完整匹配成立 → 单调递增（实测轮内 99%+） |
| 第四轮 | **+ prompt_cache_key 路由粘性** | 消除偶发全 miss（厂商负载均衡漂移） |
| 第五轮 | **+ 子 agent fork 完整主历史** | 子 agent 从每轮 miss 4-10K（重复 read）→ 首轮几 K + 内部少量 |

失败教训：
1. **append 污染**：`reqMsgs := opts.Messages; append(...)` 原地写底层数组（Go slice 别名坑）——强制复制修复
2. **NS 位置**：完整前缀匹配下 NS 必须与上轮请求末尾位置一致——只有落库进历史才能保证
3. **子 agent 只复用前缀**：20K 前缀命中但主历史不在请求里 → 每次 review 重复 read 相同内容重复付费
4. **缺 prompt_cache_key**：openai-compatible 多节点负载均衡下路由漂移 → 偶发全 miss（websearch 对照 opencode PR #22569 发现）

## 四、改动清单（最终实施）

1. **`app/chat.go`**：NS 落库进 messages（user 后、kind 标记），永不清理
2. **`internal/agent/agent.go`**：
   - Run 不再手动 append NS（历史已含）
   - `RunSubAgent`：完整主历史 fork + 尾部 `[身份+NS][指令]`
   - `computePrefixHash` 只哈希前导 system + 前缀变化块哈希诊断
3. **`internal/agent/compress.go`**：generateSummary 缓存对齐（ChatStream 全量工具 + CacheKey）；persistCompression 恢复 NS 落库；retainMessages 跳过旧 NS
4. **`internal/llm/`**：`CallOptions.CacheKey` → `buildPayload` 发 `prompt_cache_key`
5. **`internal/agent/tokens.go`**：主/子统一累计命中率；UpdateMessageUsage 按 agent_type 分写；usage_ratio 本地估算；全 miss 告警日志（hit=0 且 miss>1 万）
6. **`internal/agent/phase_gate.go`**：进入 write 重置 wordCountOK
7. **`internal/session/store.go`**：UpdateMessageUsage 加 agentType
8. **`cmd/cacheprobe`**：完整创作流程模拟（init + 5 章 × ~86 工具调用 + 子 agent 内部序列 + legacy 真实 NS 协议）

## 五、验证结果

```
NS 落库+K=3（修复前）:      turn 首轮 miss 4-5 万，累计 96%
NS 尾部临时拼（第一轮）:    hit 恒定 73-114K 台阶，累计 89%
NS 落库永不清理:            轮内 99%+，hit 单调递增（实测 134K→167K）
子 agent fork 完整主历史:   子 agent 首轮命中主历史全部（15 万+）
prompt_cache_key:           消除偶发全 miss
cacheprobe 模拟（完整流程）: now 99.5% vs legacy 99.3%，miss 降 28.1%
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
