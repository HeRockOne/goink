# 缓存命中率修复实施记录（NS 动态尾部注入 + 子 agent fork + 全量统计口径）

> 日期：2026-08-06（实施完成）
> 依据：`docs/design/cache-hit-fix-implementation.md` 原方案（K=3 落库清理）+ 实施中发现的问题
> 状态：已实施（commit cf49ff6）
> 说明：本文是原方案（P1 NS 落库协议 + P2 压缩 NS 双份）的**实施修订版**——实际采用"NS 不入历史、请求尾部动态注入"，并扩展了子 agent / 统计口径 / 压缩对齐三块

---

## 一、目标协议（不变）

让"上一轮完整请求字节 = 本轮请求前缀"，仅尾部增量 miss。轮次 N+1 请求 = 轮次 N 请求字节 + 新增内容。

## 二、实际方案 vs 原方案

| 项 | 原方案（2026-08-04） | 实际实施（2026-08-06） | 原因 |
|---|---|---|---|
| NS 落库 | 落 messages 表，K=3 保留、旧快照置 to_api=false | **不落库**：存 `session.extra_metadata.novel_state`，请求时由 `agent.go` 追加到消息末尾 | K 清理导致消息数组变化（删除历史中的 NS 块），从删除位置起后续全部 miss——实测每个 turn 首轮 miss 4-5 万 |
| 清理逻辑 | keepNovelStateSnapshots=3 | **删除**（无历史副本可清理） | 同上 |
| 请求尾部 | NS 作为消息在历史中 | 每轮请求临时 append（**必须复制新 slice**，直接 append 会污染 opts.Messages 底层数组，NS 残留混入历史——2026-08-06 曾因此从 96% 跌到 89%） | Go slice append 别名坑（Rob Pike《Arrays, slices and strings》） |
| computePrefixHash | 哈希所有 system 消息 | **只哈希前导 system**（identity+always+catalog），尾部 NS 不参与，避免每轮误报 | NS 变化是正常现象（动态尾部），不该触发前缀告警；同时输出变化块短哈希定位 |
| 子 agent | 不受影响（原方案声明） | **复用主会话前缀**（前导 system 原文）+ 身份/NS/指令下沉尾部（Anthropic fork 模式） | 原实现子 agent 用自己的 identity 开头 → 前缀从第一个 system 就与主会话不同 → 每次全 miss（日志 `hit=0 miss=20482`） |
| 压缩 | 仅去掉 NS 写系统区 | 压缩请求**缓存对齐**：ChatStream 带全量工具，与主循环同一前缀 | 原 GenerateText 不带 tools → 压缩请求前缀与主会话不同 → 全 miss（一次 25 万） |
| usage 统计 | 未涉及 | 主/子 agent **全量计入**命中率（真实成本口径）；消息级审计按 agent_type 分写避免互相覆盖；usage_ratio 改本地估算单调不回跳；流式 usage 仅请求结束时处理一次 | 子 agent 请求与主会话同模型，不统计是掩耳盗铃（业界口径 sum(cached)/sum(input) 全量） |

## 三、消息排布（实施后）

```
轮次 N   请求: [固定前缀][userN][assistantN][toolN][NS@尾部]
轮次 N+1 请求: [固定前缀][userN][assistantN][toolN][userN+1][assistantN+1][toolN+1][NS@尾部]
                           ↑↑↑↑ 轮次 N 请求字节（含尾部 NS）→ 全部命中 ↑↑↑↑
```

- 固定前缀（identity+always+catalog）只在新 session 写入，全量工具恒定
- 消息历史**纯 append-only**：只追加，永不删除/编辑（压缩是唯一重建点）
- NS 每轮变化只 miss NS 本身（几千 token），不伤历史

## 四、改动清单（实施）

1. **`app/chat.go`**：NS 不再 Create 成消息；删 keepNovelStateSnapshots 清理逻辑；最新 NS 写 `session.extra_metadata.novel_state`
2. **`internal/agent/agent.go`**：
   - `Run` 开头读最新 NS（`loadNovelState`），每轮请求前 `make` 新 slice 复制 `opts.Messages` 再 append NS（防污染）
   - `RunSubAgent`：子 agent 请求 = 主会话前导 system 原文 + `[subSystem(身份+NS)][user 指令]`
   - `computePrefixHash` 只哈希前导 system；prefix-change 日志输出各块短哈希
3. **`internal/agent/compress.go`**：
   - `generateSummary` 改用 `ChatStream` 带全量工具（与主循环同一前缀），只在末尾追加压缩指令 + NS
   - `persistCompression` 删除 NS 落库；压缩时重建的 NS 写回 `session.extra_metadata`
4. **`internal/agent/tokens.go`**：
   - `updateUsage` 主/子统一累计（accHit/accMiss/perModel/cache_hit_ratio），子 agent 不推前端事件
   - `UpdateMessageUsage` 按 agent_type 分写各自消息
   - `usage_ratio = (runningTokens + fixedPrefix + toolTokens) / window`（本地估算，单调）
5. **`internal/session/store.go`**：`UpdateMessageUsage` 加 agentType 参数 + WHERE agent_type
6. **`internal/agent/phase_gate.go`**：进入 write 阶段重置 wordCountOK（字数校验每章独立，见 phase-gate.md）

## 五、验证结果（2026-08-06 实测，MiniMax M2.5）

```
修复前（NS 落库 K=3）:  turn 首轮 miss 4-5 万，累计 96%
append 污染事故:        每轮命中恒定 72-112K（前缀在残留 NS 处断裂），累计跌至 89%
修复后预期:             turn 首轮 miss 只余 NS 本身，主会话轮内 99%+，累计 98%+
```

判据（同原方案 4.2）：每调用 hit 增量随轮次递增（不再恒定）；长会话命中率随轮次单调上升。

## 六、回退方案

- 纯逻辑改动、无 schema 变更：revert 上述 commit 即可；`session.extra_metadata.novel_state` 冗余字段无副作用
- 历史消息里残留的旧 NS 快照（`kind=novel_state`）已被 to_api=false，不占请求序列，无需清理

## 相关文档

- `docs/architecture/cache-hit-mechanism.md` — 缓存机制与实测
- `docs/architecture/token-injection.md` — 注入构成与层级
- `docs/architecture/phase-gate.md` — 门禁字数校验
- `docs/adr/0001-prompt-caching.md` — 前缀稳定化决策（本文不推翻）
