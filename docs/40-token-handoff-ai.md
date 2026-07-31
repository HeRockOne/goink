# AI 交接文档：Token 优化项目

> **给下一个 AI 的快速交接**
> 日期：2026-07-28
> 状态：✅ Phase 1 已实施，已于 07-30 实测验证缓存命中率（89-93%）

---

## 一、当前状态（一句话）

**Phase 1 已实施并实测验证（Prompt Caching + 缓存统计 + 价格配置），缓存命中率 89-93%（见 `11-billing-test-report.md`）。Token 注入量已用 `tokencount` 精确统计（16,122 tokens，见 `42-token-injection.md`）。**

---

## 二、关键文件位置

| 文件 | 路径 | 用途 |
|------|------|------|
| **项目记录** | `docs/41-token-project-record.md` | 完整讨论记录、决策原因、实施方案 |
| **优化方案** | `docs/13-token-optimization-plan.md` | 行业调研、方案对比、风险评估 |
| **门禁配置** | `docs/02-phase-gate.md` | 5阶段工具白名单 |
| **架构文档** | `docs/01-architecture.md` | 项目整体架构 |
| **系统提示词** | `internal/agentcfg/identity.go` | mainAgentSystem1 + reviewAgentSystem1 |
| **Agent 循环** | `internal/agent/agent.go` | Run 方法、RunSubAgent |
| **Token 计数** | `internal/agent/tokens.go` | InitRunningTokens、updateUsage |
| **LLM 客户端** | `internal/llm/` | Anthropic/OpenAI 调用 |

---

## 三、基线数据（已测量）

```
单章总消耗：52K token
系统上下文：5K（10%）← 不是之前估算的 17K
工具结果：25K（48%）
AI 输出：22K（42%）← 含4200字正文
用户输入：1K（2%）
```

**关键发现**：门禁配置已经在做分阶段裁剪，writing_context 已经在做数据聚合，52K/章 已经很高效。

---

## 四、要实施的方案（2 个）

### 方案 1：Prompt Caching（✅ 已实施）

**目标**：成本降低 64%

**原理**：
- LLM 提供商缓存稳定的请求前缀（system prompt + tools）
- 后续请求命中缓存时，前缀按 10% 计费

**实施位置**：
1. `app/chat.go` — writeSystemMessages 函数
2. `app/chat.go` — chatImpl 函数

**关键改动**：
```go
// writeSystemMessages: 只写入稳定前缀，不写 novelState
for _, c := range []string{identity, always, catalog} {
    // novelState 不在此写入
}

// chatImpl: 动态注入 novelState（在系统消息之后、用户消息之前）
novelState, err := agentcfg.NovelState(a.session.DB, input.NovelID)
if novelState != "" {
    // 找到最后一个 system 消息的位置，在其后插入
    lastSystemIdx := -1
    for i, m := range messages {
        if m["role"] == "system" {
            lastSystemIdx = i
        }
    }
    // 插入 novelState
}
```

**效果**：
```
优化前消息顺序：
1. identity（稳定）
2. always（稳定）
3. catalog（稳定）
4. novelState（动态！破坏缓存）
5. user message
6. assistant response

优化后消息顺序：
1. identity（稳定）
2. always（稳定）
3. catalog（稳定）
─── 缓存边界 ───
4. user message（动态）
5. novelState（动态）
6. assistant response
```

### 方案 2：Token-Efficient Tool Use（待实施）

**目标**：输出 token 减少 32%

**原理**：Anthropic Claude 4+ 内置功能，压缩工具调用结果的序列化格式

**实施位置**：`internal/llm/anthropic.go`（如果使用 Anthropic 模型）

**注意**：当前 Goink 主要使用 DeepSeek/OpenAI 兼容格式，此方案可能不适用。

---

## 五、不做的方案（原因）

| 方案 | 原因 |
|------|------|
| writing_context 按阶段裁剪 | 违背"一次获取全貌"设计原则，outline 阶段缺角色信息会导致大纲与角色矛盾 |
| 描述精简 | 收益太小（~1K），存在工具混淆风险（get_items/search_items） |
| 工具搜索动态加载 | 52K 已经很高效，动态加载会增加复杂度 |
| Schema 压缩 | 工具定义实际只有 ~1K，不是 10K，压缩空间有限 |

---

## 六、验证步骤

1. **实施 Prompt Caching**
2. **测试 3 章**
3. **验证**：
   - 缓存命中率 >80%
   - 写作质量不下降
   - 工具调用成功率 100%
4. **实施 Token-Efficient Tool Use**
5. **测试 3 章**
6. **验证**：
   - 输出 token 减少
   - 功能正常

---

## 七、回滚措施（40 分钟）

**触发条件**：写作质量下降、工具调用失败率 >5%

**回滚步骤**：
1. 恢复 `app/chat.go` 中 `writeSystemMessages` 函数（恢复 novelState 写入）
2. 删除 `app/chat.go` 中动态注入 novelState 的代码
3. 重新构建（`.\build.ps1`）
4. 测试 3 章确认恢复

**具体代码**：
```go
// 回滚 writeSystemMessages
for _, c := range []string{identity, always, catalog, novelState} {
    // 恢复 novelState 写入
}

// 回滚 chatImpl
// 删除动态注入 novelState 的代码
```

**验证**：
- 测试 3 章
- 确认写作质量恢复
- 确认工具调用正常

---

## 八、核心原则

> **宁可不乱改，也不要影响写作质量。**

- 52K/章 已经很高效
- 优化空间有限
- 唯一值得做的是 Prompt Caching（零风险）
- 其他优化不值得冒质量风险

---

## 九、下一步

1. 阅读 `docs/41-token-project-record.md` 了解完整讨论过程
2. 阅读 `internal/agent/agent.go` 了解当前消息结构
3. 阅读 `internal/llm/anthropic.go` 了解当前 LLM 调用
4. ✅ 实施 Prompt Caching（已完成）
5. ✅ 测试验证缓存命中率（89-93%）
6. 实施 Token-Efficient Tool Use
7. 测试验证

---

## 十、联系方式

用户是非程序员，沟通用中文，AI 负责所有技术决策。
