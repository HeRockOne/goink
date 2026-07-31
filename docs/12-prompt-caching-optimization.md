# Prompt Caching 优化完整记录

> 基于 Reasonix、OpenCode、Claude Code 等优秀平台的调研
> 日期：2026-07-28
> 状态：✅ Phase 1 已实施，待测试验证

---

## 一、行业调研结果

### 1.1 Reasonix（DeepSeek 原生 Agent）

**核心架构：三区域分区**
```
┌─────────────────────────────────────────┐
│ IMMUTABLE PREFIX（不可变前缀）            │ ← 会话内固定
│   system + tool_specs + few_shots        │   缓存命中候选
├─────────────────────────────────────────┤
│ APPEND-ONLY LOG（仅追加日志）             │ ← 单调增长
│   [assistant₁][tool₁][assistant₂]...    │   保留前轮前缀
├─────────────────────────────────────────┤
│ VOLATILE SCRATCH（易失草稿）             │ ← 每轮重置
│   R1 thought, transient plan state      │   不发送到上游
└─────────────────────────────────────────┘
```

**实际效果**：99.82% 缓存命中率，435M token 只花 ~$12

### 1.2 OpenCode（编程 Agent）

**核心策略：系统提示词拆分**
```
S1（稳定层）：系统提示词 + 全局技能 + 工具定义
S2（动态层）：项目上下文 + 用户消息
```

**关键做法**：
- 系统提示词拆分为稳定/动态两部分
- 工具按名称排序（确定性顺序）
- 全局技能放 S1，项目技能放 S2

### 1.3 Claude Code（Anthropic）

**分层缓存**
```
Layer 1：系统提示词（永久缓存）
Layer 2：CLAUDE.md（项目级缓存）
Layer 3：会话上下文（会话级缓存）
Layer 4：对话消息（每轮增长）
```

**关键做法**：
- 缓存断点（cache_control）标记稳定层边界
- 工具搜索：延迟加载而不是删除
- 压缩时保留前缀（cache-safe forking）

---

## 二、通用优化 vs 特定优化

### 2.1 通用优化（所有 OpenAI 兼容格式）

| 优化 | 适用性 | 原因 |
|------|--------|------|
| 工具按名称排序 | ✅ 通用 | 确定性顺序，任何模型都需要 |
| 前缀哈希检测 | ✅ 通用 | 检测前缀变化，任何缓存机制都需要 |
| 仅追加日志 | ✅ 通用 | 保留前缀，任何缓存机制都需要 |
| 易失状态分离 | ✅ 通用 | 减少不必要的 token |

### 2.2 DeepSeek 特定优化

| 优化 | 适用性 | 原因 |
|------|--------|------|
| 工具调用修复 | ❌ DeepSeek | 处理 DeepSeek 特定的失败模式 |
| 思考模式处理 | ❌ DeepSeek | reasoning_content 是 DeepSeek 特有的 |
| 成本控制（flash-first） | ❌ DeepSeek | 基于 DeepSeek 定价模型 |

### 2.3 缓存命中指标差异

| 提供商 | 字段名 |
|--------|--------|
| DeepSeek | `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` |
| OpenAI | `prompt_tokens_details.cached_tokens` |
| Anthropic | `cache_read_input_tokens` / `cache_creation_input_tokens` |

---

## 三、Goink 适用性分析

### 3.1 Goink 支持的提供商

- DeepSeek（主要）
- OpenAI 兼容格式
- 其他（Doubao、Qwen、Moonshot 等）

### 3.2 建议实施的优化

| 优化 | 适用范围 | 风险 | 收益 | 建议 |
|------|---------|------|------|------|
| 工具按名称排序 | 通用 | 🟢 低 | 高 | ✅ 立即实施 |
| 前缀哈希检测 | 通用 | 🟢 低 | 中 | ✅ 立即实施 |
| 仅追加日志 | 通用 | 🟡 中 | 中 | ✅ 实施 |
| 易失状态分离 | 通用 | 🟡 中 | 中 | ⏸️ 暂缓 |
| 工具调用修复 | DeepSeek | 🔴 高 | 不确定 | ❌ 不做 |
| 成本控制 | DeepSeek | 🟡 中 | 中 | ⏸️ 暂缓 |

---

## 四、实施计划

### Phase 1（低风险，立即做）

#### 4.1 工具按名称排序

**目标**：确保工具顺序确定性

**实施位置**：`internal/mcp_tools/base.go` 的 `OpenAI()` 方法

**改动**：
```go
// 当前
for _, k := range keys {
    t := r.tools[k]
    // ...
}

// 优化后（已经按名称排序，无需改动）
for _, k := range keys {
    t := r.tools[k]
    // ...
}
```

**验证**：检查 keys 是否已经排序（当前代码 `sort.Strings(keys)` 已排序）

#### 4.2 前缀哈希检测

**目标**：检测前缀变化，监控缓存稳定性

**实施位置**：`internal/agent/agent.go` 的 `Run()` 方法

**改动**：
```go
// 在 Run 开始时计算前缀哈希
prefixHash := computePrefixHash(opts.Messages, tools)
// 与上一轮哈希对比
if lastPrefixHash != 0 && lastPrefixHash != prefixHash {
    a.logger.Warn("前缀变化，缓存可能失效", "last", lastPrefixHash, "current", prefixHash)
}
lastPrefixHash = prefixHash
```

### Phase 2（中风险，验证后做）

#### 4.3 仅追加日志

**目标**：确保对话历史不被重写

**验证**：检查压缩机制是否保持前缀稳定

### Phase 3（高风险，暂不做）

- 易失状态分离
- 工具调用修复
- 成本控制

---

## 五、风险评估

| 方案 | 风险 | 收益 | 建议 |
|------|------|------|------|
| 工具排序 | 🟢 低 | 高 | ✅ 立即实施 |
| 前缀哈希 | 🟢 低 | 中 | ✅ 立即实施 |
| 仅追加日志 | 🟡 中 | 中 | ✅ 实施 |
| 易失状态分离 | 🟡 中 | 中 | ⏸️ 暂缓 |
| 工具调用修复 | 🔴 高 | 不确定 | ❌ 不做 |
| 成本控制 | 🟡 中 | 中 | ⏸️ 暂缓 |

---

## 六、参考资源

- Reasonix: https://github.com/esengine/DeepSeek-Reasonix
- OpenCode: https://github.com/anomalyco/opencode
- Claude Code: https://code.claude.com/docs/en/prompt-caching
- DeepSeek 文档: https://api-docs.deepseek.com/guides/kv_cache/
- OpenAI 文档: https://developers.openai.com/api/docs/guides/prompt-caching
