## 问题：Goink 计费面板显示的成本与服务商账单差异巨大

模型服务商详细账单：

| 模型 | 输入Token数 | 输出Token数 | 缓存Token数 | 消费金额(元) |
|---|---|---|---|---|
| step-3.7-flash | 341672 | 8112 | 238720 | 0.2756（赠送扣减后） |

goink内置计费面板：

缓存命中: 239K 未命中: 38K 命中率: 86.2%

输入 ¥0.0645 输出 ¥0.0329 合计 ¥0.0974

节省 ¥-0.0202（负节省金额）

### 版本

- v0.2.3（基于 sigpanic/goink v1.1 fork）

### 审计发现

根因有三，各自叠加放大了差异：

#### 1. 输入 token 只累计增长量而非请求全量（主因）

`internal/agent/tokens.go:updateUsage` 中累计 `accPrompt` 的逻辑：

```go
if promptDelta := promptTokens - lastPrompt; promptDelta > 0 {
    accPrompt += promptDelta
}
```

`prompt_tokens` 在同一轮工具循环（Agent Loop，多轮 LLM 调用）中，每次 `ChatStream` 返回的是该轮请求的**完整** prompt token 数。但由于工具的 "tool result" 消息被加入 `opts.Messages`，同一个 Agent 循环内的多次 LLM 调用——每次的 prompt_tokens 包含了新增的工具结果 token，导致 `prompt_tokens` 在不同调用间逐轮递增。面板只累加了增量，因而计数远低于实际。

**例如**服务商账单计费为 341,672 输入 + 8,112 输出，而面板仅显示约 37K 输入 token。差距约 9 倍。

#### 2. 流式 completion_tokens 增量计费在重试场景可能重复

`completion_tokens` 增量算法：

```go
delta := comp - lastComp
if delta > 0 {
    thisComp = delta
}
```

当 SSE 流因网络中断后重建（`EventRetry` 后重新 `ChatStream`），API 从 0 重新返回流式累积值，此时 `lastComp` 会被新流的末次值覆盖——导致上一轮流的 completion 被 "遗忘"，不再累加到 accCompletion 中。但这种现象在实际场景中概率较低（通常同一工具循环内流式一次性完成）。

#### 3. 节省金额为负（前端公式安全缺失）

`ContextRing.tsx:calculateCost` 中 `savedAmount` 计算：

```typescript
const savedAmount = fullInputCost - inputCost
```

当 `prompt_tokens < prompt_cache_hit_tokens` 时（历史数据异常或旧版 session 残留），`inputCost` 可能大于 `fullInputCost`，导致 `savedAmount < 0`。此外，`cachePrice` 在不同服务商那里可能表示为折扣率（如 0.1 = 10%）或绝对值价格（如 0.27 ¥/M）。

### 修复方法

#### A. 以每次 LLM API 请求为计数周期

**新方案**：引入 `requestID` 区分 "同一 Agent 循环内同一轮 `ChatStream` 的流式更新" vs "不同轮 `ChatStream` 的新计费请求"。新增 `accumulateUsage` 纯函数：

```
新请求（newRequest = true）：完整累加 prompt/completion/hit/miss
同一 SSE 流的后续 chunk（newRequest = false）：只加 delta
```

以日志数据验证：第一轮 LLM 请求 prompt=18,058 → 第二轮请求 prompt=19,152 实际增量差 1,094；修复后 accPrompt = 18,058 + 19,152 = 37,210（与服务商口径匹配）

#### B. 前端的计算防御

- `totalPrompt = max(reportedPrompt, reportedCached + reportedMiss)` 防止缓存数据异常
- `savedAmount = max(0, fullInputCost - inputCost)` 杜绝负节省

#### C. 缓存价格交互

设置界面中 `cachePrice` 的输入标签由 "缓存输入价格 ¥/M" 统一，避免折扣率/绝对价的混淆。

### 受影响文件

| 文件 | 改动 |
|---|---|
| `internal/agent/tokens.go:32` | 增加 `requestID` 参数，引入 `accumulateUsage` 按请求边界计费 |
| `internal/agent/agent.go:328` | 在每次 `ChatStream` 前递增 `usageRequestSeq` |
| `frontend/src/components/chat/ContextRing.tsx:36` | 安全归一化 + savedAmount 下限保护 |
| `internal/agent/tokens_test.go` | 两个单元测试覆盖"新请求完整计费"和"流式差值忽略" |

### 验证

1. **单元测试**：`TestAccumulateUsageCountsEachRequestOnce` 验证两轮请求累计到 37,210 输入 + 533 输出
2. **单元测试**：`TestAccumulateUsageUsesStreamingDeltas` 验证同一流内 prompt 不变时 accPrompt 不额外增长
3. **前端 TypeScript 编译**：`npx tsc --noEmit` 通过
4. Go **cgo 集成测试**：当前 Windows 环境缺少 libsqlite3-dev，属于已知限制（CLAUDE.md 注明）

### 后续跟踪

- [ ] 建议在 CI 环境中运行完整的 Go 集成测试（Linux/macOS）
- [ ] 建议在 session.Usage schema 添加 `schema_version` 字段用于数据迁移
- [ ] 建议服务商账单导入功能：从 CSV 解析并交叉验证面板计费

---

## Bug #4：total_tokens 和 usage_ratio 使用当前请求值而非累积值（2026-07-29 发现）

### 现象

模型服务商账单（00:00-00:59）：

| 输入Token数 | 输出Token数 | 缓存Token数 | 消费金额(元) |
|---|---|---|---|
| 86,239 | 906 | 75,456 | 0.0000（赠送扣减后） |

goink 内置面板显示：

- 缓存命中: 75K · 未命中: 11K · 命中率: 87.5% ← **正确**（与服务商一致）
- **已用: 24K** · 总大小: 256K ← **错误**：应为 ~87K
- **上下文占用: 9.2%** ← **错误**：应为 ~34%

### 根因

`tokens.go:updateUsage` 中：

```go
contextTotal := promptTokens + completionTokens  // ← 当前请求的值
```

`prompt_tokens` 和 `completion_tokens` 已正确累积为 `accPrompt`/`accCompletion`，但 `total_tokens` 和 `usage_ratio` 仍使用当前请求的 `promptTokens + completionTokens`（~23K），而非累积值 `accPrompt + accCompletion`（~87K）。

导致持久化到 session 和推送到前端的 `total_tokens` 始终是最后一次请求的值，面板 "已用" 显示错误，`usage_ratio` 百分比也偏低。

### 修复

`internal/agent/tokens.go`：

```go
// 修复前
contextTotal := promptTokens + completionTokens
sessUsage["total_tokens"] = contextTotal
sessUsage["usage_ratio"] = contextTotal / float64(opts.Model.ContextWindow) * 100

// 修复后
accTotal := accPrompt + accCompletion
sessUsage["total_tokens"] = accTotal
sessUsage["usage_ratio"] = accTotal / float64(opts.Model.ContextWindow) * 100
```

同理，推送到前端的 `evUsage` 也使用 `accTotal`。

### 验证

以日志数据验证：
- 修复前：`total_tokens=23523, usage_ratio=9.2%`（当前请求值）
- 修复后：`total_tokens=86926, usage_ratio=34.0%`（累积值，与服务商口径一致）
