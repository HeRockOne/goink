# 计费面板技术文档

> 日期：2026-07-29
> 状态：已实施（2026-08-07 更新：功能已落地于 `internal/agent/tokens.go` + `frontend/src/components/chat/ContextRing.tsx`；本文档按 archive 规则不再维护正文，仅保留正确口径供参考）
> 原则：**永远用 API 返回的 usage 对象，不自己计算 token 数**
> 注意：AGENTS.md 引用的缓存字段口径以此为准——**优先 `prompt_tokens_details.cached_tokens`（OpenAI 标准），fallback `prompt_cache_hit_tokens`**（与本文 3.1 表修正后一致）

---

## 一、背景

曾尝试实现计费面板（显示 token 消耗、成本估算、缓存命中率），但因口径问题导致面板数据与服务商账单不一致，最终移除。本文档记录失败原因和正确方案。

---

## 二、失败复盘

### 2.1 上一个计费面板的3个 Bug

| Bug | 根因 | 影响 |
|-----|------|------|
| 输入 token 只累计增量 | `prompt_tokens` 是 API 返回的当前请求完整值，旧代码用 delta 累加 | 面板显示远低于实际 |
| total_tokens 用当前请求值 | `total_tokens = promptTokens + completionTokens`（当前请求），不是累积值 | "已用"显示错误 |
| detail 比例失真 | `runningTokens` 是累积值，与当前请求 `apiTotal` 混算比例 | 系统上下文显示4.5M（应为12K） |

### 2.2 核心教训

> **API 返回的 `usage` 对象是唯一可信数据源。不要自己累加 token 数。**

DeepSeek 的 `usage` 对象：
```json
{
  "prompt_tokens": 12000,           // 当前请求的完整输入 token
  "prompt_cache_hit_tokens": 9000,  // 缓存命中
  "prompt_cache_miss_tokens": 3000, // 缓存未命中
  "completion_tokens": 950,         // 输出 token
  "total_tokens": 12950             // = prompt_tokens + completion_tokens
}
```

**验证公式**：
- `prompt_tokens = prompt_cache_hit_tokens + prompt_cache_miss_tokens`
- `total_tokens = prompt_tokens + completion_tokens`

---

## 三、正确方案

### 3.1 最终口径（2026-07-29 确认）

#### 数据源

| 数据 | 来源 | 精确度 | 说明 |
|------|------|--------|------|
| prompt_tokens | API `usage.prompt_tokens` | 精确 | 含消息内容 + 工具定义 + 格式开销 |
| completion_tokens | API `usage.completion_tokens` | 精确 | 当前请求的输出 token |
| prompt_cache_hit_tokens | API `usage.prompt_cache_hit_tokens` | 精确 | **fallback**（OpenAI 标准格式缺失时） |
| prompt_cache_miss_tokens | API `usage.prompt_cache_miss_tokens` | 精确 | fallback |
| 缓存（首选） | API `usage.prompt_tokens_details.cached_tokens` | 精确 | **优先**（OpenAI 标准格式，键存在即按 `miss = prompt - cached` 语义） |
| detail（分角色） | `runningTokens` 原值 | **估算** | 仅消息内容，不含工具定义和格式开销 |

#### 差值分析

```
API prompt_tokens        = 19,469（精确）
runningTokens 总和       =  5,892（消息内容估算）
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
差值                     = 13,577
├─ 工具定义 JSON Schema  ≈ 12,500（52个工具的完整 parameters，2026-07-29 快照，现为 59 个）
└─ 消息格式开销          ≈  1,077（role分隔符 <|im_start|>）
```

**差值属于系统上下文**，但 `runningTokens` 无法统计工具定义（不在 messages 里）和格式开销。这是 `CountMessageTokens` 的设计限制，所有 OpenAI 兼容实现的共同特征。

#### 成本计算

```
hitCost   = accHit   × cachePrice  / 1_000_000
missCost  = accMiss  × inputPrice  / 1_000_000
outCost   = accCompletion × outputPrice / 1_000_000
totalCost = hitCost + missCost + outCost
```

#### 缓存优化与 allowed_tools

- `tools` 数组始终发送全量59个 → 缓存前缀稳定（工具定义是缓存前缀的一部分）
- `allowed_tools` 参数传递门禁白名单 → 模型不浪费 token 考虑禁止工具
- 不支持 `allowed_tools` 的提供商忽略该参数 → 无副作用
- 门禁执行时硬拦截 → 双重保险

### 3.2 Session 级累加（仅用于缓存统计和成本估算）

```go
// 每请求累加（只累加 cache hit/miss 和 completion）
accHit += hitTokens    // API 返回的当前请求缓存命中
accMiss += missTokens  // API 返回的当前请求缓存未命中
accCompletion += completionTokens  // API 返回的当前请求输出

// 派生指标
cacheHitRatio = accHit / (accHit + accMiss) * 100
estimatedCost = (accHit × hitPrice + accMiss × missPrice + accCompletion × outputPrice) / 1_000_000
```

**不累加 `prompt_tokens`** — 它是当前请求的完整值，累加没有意义。

### 3.3 前端显示

| 显示项 | 数据源 | 说明 |
|--------|--------|------|
| 上下文占用 % | `usage_ratio = accTotal / context_window * 100` | Session 级累积占上下文窗口的比例 |
| 已用 token | `total_tokens`（当前请求的输入+输出） | 单次请求的完整 token |
| 缓存命中率 | `accHit / (accHit + accMiss) * 100` | Session 级累积 |
| 分角色明细 | `runningTokens` 原值 + 标注"估算" | 仅消息内容，与 API 总数有差值是预期行为 |
| 成本估算 | 累积值 × 定价表 | Session 级累积成本 |
| token 明细 | 缓存读取/未命中/输出 + 各自金额 | 用当前请求值计算金额 |

### 3.4 后端推送结构

```go
usage := map[string]any{
    // 当前请求值（直接来自 API）
    "prompt_tokens":            apiUsage["prompt_tokens"],
    "completion_tokens":        apiUsage["completion_tokens"],
    "total_tokens":             apiUsage["total_tokens"],
    
    // Session 级累积值
    "prompt_cache_hit_tokens":  accHit,
    "prompt_cache_miss_tokens": accMiss,
    
    // 快照（当前请求的上下文组成）
    "detail":                   runningTokens,  // 直接用原始值
    
    // 派生指标
    "context_window":           opts.Model.ContextWindow,
    "usage_ratio":              float64(apiTotal) / float64(opts.Model.ContextWindow) * 100,
    "cache_hit_ratio":          accHit / (accHit + accMiss) * 100,
}
```

---

## 四、行业参考

| 平台 | 取数方式 | 关键点 |
|------|---------|--------|
| DeepSeek | 每请求读 `usage` 对象 | `prompt_tokens` 已包含缓存命中 |
| OpenCode 插件 | 每请求读 `usage`，session 级累加 | 显示最近一次请求明细 + 累计成本 |
| LiteLLM | 每请求读 `usage`，按模型定价算 cost | 区分 OpenAI/Anthropic 两种缓存格式 |
| Reasonix | 前缀哈希 + 每请求读 `usage` | 99.82% 缓存命中率 |
| Braintrust | 每请求读 `usage`，按 trace 聚合 | 估算 token 用 `~` 前缀标记 |

---

## 五、成本计算公式

### 5.1 DeepSeek 定价（以 step-3.7-flash 为例）

| 类型 | 价格（¥/M token） | 变量名 |
|------|-------------------|--------|
| 缓存命中输入 | 0.27 | `cachePrice` |
| 缓存未命中输入 | 1.35 | `inputPrice` |
| 输出 | 8.1 | `outputPrice` |

### 5.2 三个区域的计算逻辑

**区域 A：token 明细（按 token 类型分）**

```
hitCost   = accHit   × cachePrice  / 1_000_000    // 缓存读取金额
missCost  = accMiss  × inputPrice  / 1_000_000    // 未命中金额
outCost   = accCompletion × outputPrice / 1_000_000  // 输出金额
totalCost = hitCost + missCost + outCost           // 合计（= 区域 A 求和）
```

**区域 B：分角色（仅 token 数，不分摊金额）**

> 2026-08-07 更新：实际实现（`ContextRing.tsx`）中**成本按缓存/未命中/输出计，不按角色分摊**；分角色区只显示各角色 token 数（`runningTokens` 估算值），不显示金额。以下原方案（按占比分配金额）未采用，仅保留作历史记录：

```
角色占比[role] = runningTokens[role] / sum(runningTokens)
角色金额[role] = totalCost × 角色占比[role]
```

求和验证：
```
sum(角色金额) = totalCost × sum(角色占比)
             = totalCost × 1.0
             = totalCost  ✓
```

**区域 C：标题行合计**

```
合计 = totalCost  // 直接引用区域 A 的结果，不重新计算
```

### 5.3 三个区域的关系

```
区域 A（token 明细）是成本的唯一计算源
         ↓
区域 B（分角色）按占比分配区域 A 的合计
         ↓
区域 C（标题行）直接显示区域 A 的合计
```

**核心原则**：区域 A 的 `totalCost` 是唯一真值，区域 B 和 C 都从它派生，不独立计算。

### 5.4 数值验证

以服务商账单验证（step-3.7-flash，01:00-01:59）：

```
输入 56,704 token，缓存命中 35,584，未命中 21,120，输出 1,091

区域 A：
  hitCost  = 35,584 × 0.27 / 1M = 0.0096
  missCost = 21,120 × 1.35 / 1M = 0.0285
  outCost  = 1,091 × 8.1 / 1M   = 0.0088
  totalCost = 0.0096 + 0.0285 + 0.0088 = 0.0469

区域 B（示例，假设系统上下文占 20%）：
  系统上下文金额 = 0.0469 × 0.20 = 0.0094

区域 C：
  合计 = 0.0469

验证：区域 A 求和 = 区域 C = 0.0469 ✓
```

### 5.5 浮点精度处理

金额显示保留4位小数（¥0.0001 精度），四舍五入。求和可能有 ±0.0001 误差，前端显示时用 `toFixed(4)` 统一精度。

---

## 六、面板布局

### 6.1 位置

复用现有 `ContextRing` 组件（SVG 圆环），hover 展开 popover。不新增独立面板。

### 6.2 布局结构

```
┌─────────────────────────────────────┐
│  上下文占用: 22.5%    缓存命中: 87% │  ← 标题行
├─────────────────────────────────────┤
│  ████████████░░░░░░░░░░░░░░░░░░░░░ │  ← 进度条（绿/橙/红）
├─────────────────────────────────────┤
│  已用: 58K · 总大小: 256K           │  ← token 统计
├─────────────────────────────────────┤
│  💰 成本估算         ¥0.042         │  ← 一行合计
├─────────────────────────────────────┤
│  缓存读取    36K     ¥0.010         │  ← token 明细 + 分角色金额
│  未命中      21K     ¥0.028         │
│  输出         1K     ¥0.006         │
├─────────────────────────────────────┤
│  系统上下文    12K     ¥0.016        │  ← 分角色金额
│  用户输入      8K     ¥0.011        │
│  AI 输出       1K     ¥0.008        │
│  工具结果     37K     ¥0.050        │
├─────────────────────────────────────┤
│  输入 ¥/M    输出 ¥/M   缓存 ¥/M   │  ← 价格配置（可折叠）
│  [1.35  ]   [8.1   ]   [0.27  ]   │
├─────────────────────────────────────┤
│  压缩阈值         85%               │  ← 压缩设置
│  ◀━━━━━━━━━━━━━━━━━▶               │
└─────────────────────────────────────┘
```

### 6.3 区块说明

| 区块 | 内容 | 数据源 | 折叠 |
|------|------|--------|------|
| 标题行 | 上下文占用 % + 缓存命中率 | `usage_ratio` + `cache_hit_ratio` | 否 |
| 进度条 | 彩色条（绿<80% 橙80-90% 红>90%） | `usage_ratio` | 否 |
| token 统计 | 已用 token + 上下文窗口大小 | `apiTotal` + `context_window` | 否 |
| 成本估算 | 合计金额（一行） | 累积值 × 定价表 | 否 |
| token 明细 | 缓存读取/未命中/输出 + 各自金额 | API usage + 定价表 | 否 |
| 分角色金额 | system/user/assistant/tool + 各自金额 | `runningTokens` × 角色单价 | 是 |
| 价格配置 | 输入/输出/缓存单价输入框 | 用户配置 | 是 |
| 压缩设置 | 阈值滑块（50-95%） | `compression_threshold` | 否 |

### 6.4 金额计算

**token 明细区**：
```
缓存读取金额 = accHit × cachePrice / 1_000_000
未命中金额 = accMiss × inputPrice / 1_000_000
输出金额 = accCompletion × outputPrice / 1_000_000
合计 = 缓存读取金额 + 未命中金额 + 输出金额
```

**分角色金额**（按角色在上下文中的占比分配成本）：
```
角色占比 = runningTokens[role] / sum(runningTokens)
角色金额 = 合计 × 角色占比
```

示例（合计 ¥0.042）：
```
系统上下文: 12K / 58K × ¥0.042 = ¥0.009
用户输入:    8K / 58K × ¥0.042 = ¥0.006
AI 输出:     1K / 58K × ¥0.042 = ¥0.001
工具结果:   37K / 58K × ¥0.042 = ¥0.027
```

### 6.5 交互规则

- **hover 展开**：鼠标移入圆环显示 popover，移出 150ms 后关闭
- **价格配置默认折叠**：点击展开，价格存 `app_config` 表
- **分角色金额默认折叠**：点击展开
- **压缩阈值实时生效**：拖动滑块立即保存

### 6.6 与现有 ContextRing 的差异

| 项目 | 现有 | 计费面板 |
|------|------|---------|
| 缓存统计 | 显示 hit/miss/命中率 | 不变 |
| 成本估算 | 无 | 新增（一行合计） |
| token 明细 | 无 | 新增（缓存读取/未命中/输出 + 金额） |
| 分角色金额 | 无 | 新增（按占比分配成本） |
| 价格配置 | 无 | 新增（输入/输出/缓存单价） |
| 节省金额 | 无 | 不做（易误导） |

---

## 七、实施清单

> 2026-08-07 更新：以下 8 项均已实施（`internal/agent/tokens.go` updateUsage + `frontend/src/components/chat/ContextRing.tsx` + `internal/session/store.go` UpsertModelUsage）。

- [x] 后端 `tokens.go`：每请求直接用 API usage，不累加 prompt_tokens
- [x] 后端 `tokens.go`：累加 accHit/accMiss/accCompletion 用于成本估算
- [x] 后端 `tokens.go`：detail 直接用 runningTokens 原始值
- [x] 前端 `ContextRing.tsx`：显示当前请求的 token 明细 + session 级缓存命中率
- [x] 前端 `ContextRing.tsx`：成本估算用累积值 × 定价表
- [x] 前端 `ContextRing.tsx`：价格配置（输入/输出/缓存命中 单价）
- [x] 单元测试：验证 `prompt_tokens = hit + miss`
- [x] 单元测试：验证 `total_tokens = prompt + completion`
- [x] 对账：用服务商账单交叉验证面板数据
