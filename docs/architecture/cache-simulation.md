# 缓存命中模拟（Cacheprobe）

> 对应代码：`internal/cacheprobe/` + 设置面板「缓存模拟」Tab
> 创建日期：2026-08-08

## 为什么需要这个模块

LLM 的 Prompt Caching 是"字节精确前缀匹配"——如果本次请求与上次请求的公共前缀越长，命中量越大。但真实模型调用链路的缓存行为是黑盒，无法在真机测试前预估。cacheprobe 用字节级模拟来**事前估算**不同优化方案的缓存收益，避免盲目开发。

## 实现原理

1. **消息序列化**：用 Go map + encoding/json 构造与真实请求完全一致的字节序列（键序确定）
2. **前缀匹配**：`longestCommonPrefix` 逐字节比较两次请求，公共前缀覆盖的消息 token 数为命中量
3. **Token 计数**：每条消息用 tiktoken 精确计数（`llm.CountMessageTokens`），命中 = 公共前缀覆盖的消息 token 和，miss = 其余
4. **工具定义**：真实工具列表（60 个）作为第一条消息参与计数（固定前缀，始终命中）

## 架构

```
设置面板「缓存模拟」Tab
    │
    ▼
StartCacheSimulation(gateRounds, shortQARounds, batchChapters)
    │
    ▼
cacheprobe.Run()
    │
    ▼
buildMixedSession("auto", cache, ...)  ← 当前实际行为（auto-inject）
buildMixedSession("now", cache, ...)    ← 旧 read_required 行为（对比基线）
buildMixedSession("clean", cache, ...)  ← 实验方案（clean 清理）
```

## 三种协议

| 模式 | 含义 | 当前用途 |
|------|------|---------|
| `auto` | auto-inject 自动注入技能 + NS 落库缓存 | 当前实际行为（设置面板「当前版」） |
| `now` | read_required 工具调用 + NS 落库缓存 | 旧行为基线（设置面板「旧版」） |
| `legacy` | NS 不落库，每轮重发全部历史 | 仅用于对比研究，面板不展示 |
| `clean` | 发送前清理已读 skill 全文 | 实验方案，已从运行时移除 |

## 场景

`buildMixedSession` 模拟一个真实对话窗口：
1. **init 开书** — 创建世界观、角色、总纲
2. **短对话穿插** — 查设定、微调（qaRounds 参数控制）
3. **单章创作轮** — prepare → outline → write → review → maintain（gateRounds 参数控制）
4. **批量创作** — 一次性出大纲 → 循环写正文 → 统一审稿维护（batchChapters 参数控制）

三种创作模式交替发生在同一条历史里，真实反映用户写作流程。

## 为什么选择这个实现

### 为什么不用真实 LLM 调用

真实 LLM 调用需要 API Key、产生实际费用、受限流影响。cacheprobe 纯本地字节级模拟，零成本、可重复、快速迭代。

### 为什么用字节级前缀匹配而不是 token 级

DeepSeek/商汤等提供的是"字节精确前缀匹配"磁盘缓存，不是 token 级缓存。字节级匹配更接近真实 provider 行为。

### 为什么用 Go map 序列化而不是真实 LLM 请求

Go map 的 encoding/json 序列化行为与真实请求的 JSON 序列化一致（相同键序、相同编码），因此字节序列可精确复现。

## 关键验证结论

| 优化项 | 验证结果 | 状态 |
|--------|---------|------|
| NS 轮末落库(now vs legacy) | 门禁创作 5 轮 now miss << legacy miss | 已实施 |
| auto-inject 技能 | 总输入省 30%，命中率 99.2% 不变 | 已实施 |
| set_phase 自动推进 | 增量省 2.8% | 已实施 |
| 子代理完整历史 fork | 精简后成本更高(1.4x)，保持现状 | 已排除 |
| NS 缩短到 800 字符 | 防重复检查不够(需 3 章，800 只够 2 章) | 已排除 |

## 价格估算

价格配置从设置面板读取，默认值（DeepSeek V4-Flash/mimo 同价）：
- 缓存命中：¥0.02/M token
- 未命中：¥1.0/M token
- 输出：¥2.0/M token