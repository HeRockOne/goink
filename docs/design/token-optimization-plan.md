# Goink Token 终极混合优化方案

> 综合 2025-2026 行业全部最佳实践
> 原则：**宁可不乱改，也不要影响写作质量**
> 日期：2026-07-28
> 状态：✅ Phase 1 已实施，已于 07-30 实测验证缓存命中率（89-93%）

---

## 〇、实测基线

> 当前系统提示词注入构成已用 `tokencount` 精确统计，见 **`architecture/token-injection.md`**（~17,500 tokens / 59 工具 / 12,924 工具定义）。

> ⚠️ 注意：规划中的「分阶段裁剪」已在待办中标记完成，但实际采用**全量发送 + allowed_tools 限制**方案（见 `archive/prompt-caching-research.md` 与 ADR-0001），工具 JSON 未裁剪，这是刻意为保持缓存前缀稳定做的取舍。分阶段裁剪的 -6,000 收益未兑现。

---

## 一、全景分析：Token 去哪了

```
┌─────────────────────────────────────────────────────────┐
│  每轮 API 调用 Token 构成（当前 ~17,000 token）          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  工具 Schema（59 个）          10,500 (62%)      │  │
│  │  ├─ 名称                         200              │  │
│  │  ├─ 描述                       5,200              │  │
│  │  └─ 参数定义                   5,100              │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  系统提示词                    1,785 (10%)        │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  常驻技能（writing-kernel）    1,998 (12%)        │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  其他（技能目录+NovelState+门禁） 2,718 (16%)     │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**关键发现**：工具 Schema 占 62%，是最大优化目标。

---

## 二、实施状态

### 已完成 ✅

| 事项 | 状态 | 说明 |
|------|------|------|
| Prompt Caching（消息顺序优化） | ✅ 已实施 | novelState 移到用户消息之后 |
| 缓存命中显示 | ✅ 已实施 | ContextRing 显示缓存统计 |
| 价格可配置 | ✅ 已实施 | 计费面板可编辑价格 |
| 前缀哈希检测 | ✅ 已实施 | 检测前缀变化，日志警告 |
| 工具按名称排序 | ✅ 已有 | sort.Strings(keys) |
| 全量工具发送 | ✅ 已实施 | 不随阶段变化，优化缓存 |
| 复制按钮居下 | ✅ 已实施 | MessageBubble 改动 |

### 已测试 ✅

| 事项 | 状态 | 说明 |
|------|------|------|
| 缓存命中率验证 | ✅ 已测 07-30 | 命中率 89-93%，详见 `archive/billing-test-report.md` |
| 缓存命中显示正确性 | ✅ 已测 07-30 | 全局与 per_model 累计一致 |

### 待实施 ⏳

| 事项 | 优先级 | 说明 |
|------|--------|------|
| Token-Efficient Tool Use | 低 | 需评估模型兼容性 |
| Phase 2 优化 | 低 | 仅追加日志 |
| 易失状态分离 | 低 | 暂缓 |
| 工具调用修复 | 低 | 暂缓 |
| 成本控制 | 低 | 暂缓 |

---

## 三、优化策略矩阵（10 种策略）

### 2.1 输入侧优化（Schema + 提示词）

| 策略 | 攻击目标 | 效果 | 风险 | 来源 |
|------|---------|------|------|------|
| **A. 描述精简** | 工具描述 | -3,000 | 低 | OpenAI 官方 |
| **B. 参数精简** | 参数定义 | -2,000 | 低 | OpenAI 官方 |
| **C. 分阶段裁剪** | 工具列表 | -6,000 | 中 | Anthropic/OpenAI |
| **D. 工具搜索** | 全部 Schema | -9,000 | 中 | Anthropic/OpenAI |
| **E. Schema 压缩** | Schema 格式 | -5,000 | 低 | TSCG/Toolgz |
| **F. NovelState 裁剪** | goink.md | -1,500 | 低 | 本项目 |

### 2.2 输出侧优化（工具结果）

| 策略 | 攻击目标 | 效果 | 风险 | 来源 |
|------|---------|------|------|------|
| **G. 输出压缩** | 工具返回值 | -3,000 | 低 | Tokenless |
| **H. writing_context 增强** | 工具调用次数 | -6,000/章 | 低 | 本项目 |

### 2.3 成本侧优化

| 策略 | 攻击目标 | 效果 | 风险 | 来源 |
|------|---------|------|------|------|
| **I. Prompt Caching** | 成本 | -90% | 零 | Anthropic/OpenAI |
| **J. Token-Efficient Tool Use** | 输出成本 | -70% | 零 | Anthropic Claude 4+ |

---

## 三、混合方案设计（5 种组合）

### 方案 1：零风险速赢（工时 2h）

```
策略 A（描述精简）+ I（Prompt Caching）
↓
效果：-3,000 token/轮 + 成本 -90%
风险：零
```

### 方案 2：低风险高效（工时 5h）

```
策略 A + B + F + H + I
↓
效果：-12,500 token/轮 + 成本 -90%
风险：低
```

### 方案 3：中风险最优（工时 9h）

```
策略 A + B + C + F + H + I + J
↓
效果：-18,500 token/轮 + 成本 -95%
风险：中
```

### 方案 4：高收益激进（工时 12h）

```
策略 A + B + C + D + F + H + I + J
↓
效果：-24,500 token/轮 + 成本 -95%
风险：中高
```

### 方案 5：终极混合（工时 16h）

```
策略 A + B + C + D + E + F + G + H + I + J
↓
效果：-29,500 token/轮 + 成本 -97%
风险：中
```

---

## 四、各方案详细对比

| 方案 | 固定开销 | 单章节省 | 100章节省 | 工时 | 风险 |
|------|---------|---------|-----------|------|------|
| 当前 | 17,000 | - | - | - | - |
| **方案 1** | 14,000 | ~30K | ¥6 | 2h | 零 |
| **方案 2** | 4,500 | ~65K | ¥13 | 5h | 低 |
| **方案 3** | -1,500 | ~95K | ¥19 | 9h | 中 |
| **方案 4** | -7,500 | ~125K | ¥25 | 12h | 中高 |
| **方案 5** | -12,500 | ~150K | ¥30 | 16h | 中 |

**注意**：方案 3-5 的固定开销为负数，是因为 writing_context 增强后，prepare 阶段的工具调用结果 token 增加，但总开销仍然大幅下降。

---

## 五、推荐方案：方案 3（中风险最优）

### 5.1 为什么选方案 3

1. **收益/风险比最高**：-18,500 token/轮，风险可控
2. **工时合理**：9h 可以完成
3. **质量影响最小**：所有策略都有验证机制
4. **行业验证**：每个策略都有独立来源确认

### 5.2 实施步骤

#### Step 1：描述精简（1h，零风险）

```go
// 当前（平均 150 token/工具）
func (t *GetCharactersTool) Description() string {
    return "获取当前小说的角色列表或单个角色详情。返回格式：{characters: [{id, name, desc, personality, abilities, location={id, name}, items=[{id, name, role}], item_count}], total, page, size}。brief=true 只返回 id/name/location.name/item_count。"
}

// 优化后（约 50 token/工具）
func (t *GetCharactersTool) Description() string {
    return "获取角色列表。参数：brief(bool)只返回id/name。返回characters数组含id/name/desc/personality/abilities/location/items。"
}
```

**预计节省**：59 工具 × 100 token = **-5,200 token/轮**

#### Step 2：参数精简（1h，零风险）

删除 JSON Schema 中冗余的 `description` 字段，只保留类型和必要说明。

**预计节省**：**-2,000 token/轮**

#### Step 3：分阶段裁剪（3h，中风险）

在 `agent.go` 中根据当前阶段动态选择工具列表：

```go
// 伪代码
phase := a.getPG().CurrentPhase()
var allowedTools map[string]bool
switch phase {
case "prepare":
    allowedTools = toSet([]string{"get_writing_context", "get_chapter_list"})
case "outline":
    allowedTools = toSet([]string{"edit", "read"})
case "write":
    allowedTools = toSet([]string{"edit", "read", "create_item_occurrence"})
case "review":
    allowedTools = reviewAgentAllowlist
case "maintain":
    allowedTools = maintainAllowlist
}
tools := a.registry.OpenAI(allowedTools)
```

**预计节省**：**-6,000 token/轮**

#### Step 4：writing_context 增强 + prepare 合并（3h，低风险）

1. `writing_context.go` 增加缺失字段：
   - characters: + description/personality/abilities
   - timeline: + chapter_plan (next/near/far)
   - reader: + entries (content/planted_chapter)
   - + preferences

2. `writing-kernel.md` 更新 prepare 检查清单：
   - 从 9 个 required 工具减到 2 个

**预计节省**：**-6,000 token/章**

#### Step 5：Prompt Caching（0.5h，零风险）

确保每轮 API 调用的 system prompt + tools 参数前缀相同：

```go
// 在 agent.go 的 Run 方法中
// 1. system prompt 放在最前面（稳定）
// 2. tools 参数放在 system prompt 之后（稳定）
// 3. 动态内容（用户消息、工具结果）放在最后
```

**预计节省**：**成本 -90%**

#### Step 6：Token-Efficient Tool Use（0.5h，零风险）

如果使用 Claude 4+ 模型，启用内置的 token-efficient tool use：

```go
// 在 llm 包中添加 beta header
extra_headers["anthropic-beta"] = "token-efficient-tools-2025-02-19"
```

**预计节省**：**输出 token -70%**

### 5.3 总效果

```
当前每轮固定开销：~17,000 token
方案 3 后：~1,500 token（节省 91%）

当前单章总消耗：~140K token
方案 3 后：~85K token（节省 39%）

当前 100 章成本：~¥30
方案 3 后：~¥11（节省 63%）
```

---

## 六、进阶选项：方案 4（含工具搜索）

如果方案 3 验证成功，可以追加工具搜索：

### 6.1 工具搜索实现

```go
// 1. 创建 search_tools 元工具
type SearchToolsTool struct{}

func (t *SearchToolsTool) Name() string { return "search_tools" }
func (t *SearchToolsTool) Description() string {
    return "搜索可用工具。参数：query(string)自然语言描述需要的功能。返回匹配的工具名列表。"
}

// 2. 在 agent.go 中实现动态工具注入
func (a *Agent) handleSearchTools(query string) []Tool {
    // 1. 用 embedding 相似度搜索工具描述
    // 2. 返回 top-5 匹配工具的完整 schema
    // 3. 注入到后续消息中
}
```

### 6.2 预计效果

```
方案 3 + 工具搜索：
固定开销：~800 token/轮（节省 95%）
单章：~70K token（节省 50%）
100 章成本：~¥8（节省 73%）
```

---

## 七、风险控制矩阵

| 策略 | 风险 | 影响 | 缓解措施 | 验证方法 |
|------|------|------|---------|---------|
| 描述精简 | 低 | AI 不知道何时用工具 | 保留"适用场景"描述 | 测试 3 章 |
| 参数精简 | 低 | AI 传错参数 | 保留类型和必要说明 | 测试 3 章 |
| 分阶段裁剪 | 中 | AI 在错误阶段调用工具 | 门禁硬拦截+清晰提示 | 测试 3 章+监控拦截次数 |
| writing_context 增强 | 低 | 响应变慢 | 增加的查询很轻量 | 测试响应时间 |
| Prompt Caching | 零 | 无 | 无 | 监控缓存命中率 |
| Token-Efficient | 零 | 无 | 无 | 监控输出质量 |
| 工具搜索 | 中 | AI 搜索不到工具 | 优化搜索算法 | 测试搜索准确率 |

---

## 八、监控指标

实施后需要监控：

1. **每轮 token 消耗**（通过 `tokens.go`）
2. **门禁拦截次数**（分阶段裁剪后）
3. **工具调用成功率**（描述精简后）
4. **写作质量**（审稿 Agent 评分）
5. **Prompt Caching 命中率**（成本优化）
6. **用户反馈**（AI 是否"变笨了"）

---

## 九、最终建议

### 9.1 推荐路径

```
Phase 1（2h）：方案 1（零风险速赢）
    ↓ 验证无负面影响
Phase 2（3h）：方案 2（低风险高效）
    ↓ 验证无负面影响
Phase 3（4h）：方案 3（中风险最优）
    ↓ 验证无负面影响
Phase 4（3h）：方案 4（含工具搜索）
    ↓ 验证无负面影响
Phase 5（4h）：方案 5（终极混合）
```

### 9.2 核心原则

> **宁可不乱改，也不要影响写作质量。**

每个 Phase 之间都要验证：
1. 写 3 章测试
2. 对比优化前后的工具调用成功率
3. 对比审稿 Agent 发现的问题数
4. 人工评估写作质量

如果任何一个 Phase 导致质量下降，立即回滚。

---

## 十、行业对标

| 指标 | Goink 当前 | 行业最佳 | 方案 3 后 | 方案 5 后 |
|------|-----------|---------|----------|----------|
| 工具数 | 59 | <20（建议） | 2-32（按阶段） | 2-32（按阶段） |
| Schema token/轮 | 10,500 | <1,000 | ~1,500 | ~800 |
| 固定开销/轮 | 17,000 | <3,000 | ~1,500 | ~500 |
| 单章 token | ~140K | - | ~85K | ~70K |
| 100 章成本 | ~¥30 | - | ~¥11 | ~¥8 |

---

## 十一、工具描述精简风险矩阵（Grilling 后）

### 11.1 混淆高风险工具对（必须保留关键词）

| 工具对 | 混淆风险 | 简化建议 |
|--------|---------|---------|
| `get_items` / `search_items` | 🔴 高 | 必须保留"分页列表" vs "全文搜索" |
| `get_lore` / `search_lore` | 🔴 高 | 必须保留"按分类浏览" vs "全文搜索" |

### 11.2 混淆中风险工具对（需保留区分关键词）

| 工具对 | 混淆风险 | 简化建议 |
|--------|---------|---------|
| `get_characters` / `get_character_relations` | 🟡 中 | 保留"子图"和"关系边"关键词 |
| `get_story_arcs` / `create_arc_node` | 🟡 中 | 区分"弧线"和"弧线节点" |

### 11.3 安全工具（可自由简化）

| 工具 | 原因 |
|------|------|
| `get_locations` | 无混淆对 |
| `get_timeline` / `update_timeline_entry` | 动词区分足够 |
| `get_scenes` | 唯一场景工具 |
| `get_stats` | 唯一统计工具 |
| `get_preferences` | 唯一偏好工具 |
| `get_reader_perspective` | 唯一读者认知工具 |

---

## 十二、write 阶段伏笔查询分析

### 12.1 问题

write 阶段是否需要主动调 `get_timeline` 查询伏笔状态？

### 12.2 分析

**结论**：write 阶段**不需要**主动调 `get_timeline`。原因：

1. **大纲已经包含伏笔操作**：outline 阶段写的大纲中，"伏笔操作"章节已经明确了本章要埋/推/收哪些伏笔
2. **write 阶段专注于执行**：write 阶段的任务是根据大纲写正文，而不是查询状态
3. **伏笔状态在 prepare 阶段已经获取**：prepare 阶段调 `get_writing_context` 时已经返回了伏笔列表

### 12.3 建议

- **write 阶段白名单**：`edit` + `read` + `create_item_occurrence`（记录物品流转）
- **不需要**：`get_timeline`（伏笔已在大纲中）
- **maintain 阶段**：负责更新伏笔状态（`update_timeline_entry`）

---

## 十三、Token-Efficient Tool Use 风险评估

### 13.1 功能说明

Anthropic Claude 4+ 内置的 token-efficient tool use 可以将工具调用结果的序列化格式压缩，减少输出 token 消耗（平均 14%，最高 70%）。

### 13.2 风险矩阵

| 风险 | 严重度 | 概率 | 影响 | 缓解措施 |
|------|--------|------|------|---------|
| 模型兼容性 | 中 | 高 | 其他模型忽略 header，无副作用 | 始终发送，自动兼容 |
| Prompt Caching 冲突 | 低 | 中 | 缓存可能失效 | header 很短，影响小 |
| 输出格式变化 | 低 | 低 | API 自动处理 | 无需手动处理 |
| 调试困难 | 低 | 低 | 日志可能不完整 | 不影响功能 |

### 13.3 结论

**建议始终启用**。风险极低，收益明确。

---

## 十四、最终优化方案（含 Grilling 结果）

### 14.1 推荐方案：方案 3（中风险最优）+ 安全措施

| 步骤 | 方案 | 预计效果 | 工时 | 风险 |
|------|------|---------|------|------|
| 1 | 描述精简（保守策略，高混淆工具对保留关键词） | -3,000 | 1h | 低 |
| 2 | 参数精简 | -2,000 | 1h | 低 |
| 3 | 分阶段裁剪（动态 tools，outline 含 get_characters） | -6,000 | 3h | 中 |
| 4 | writing_context 增强 + prepare 合并（一次调用获取全貌） | -6,000/章 | 3h | 低 |
| 5 | Prompt Caching | 成本 -90% | 0.5h | 零 |
| 6 | Token-Efficient Tool Use（始终启用） | 输出 -70% | 0.5h | 零 |
| **合计** | | **-17,000/轮** | **9h** | |

### 14.2 分阶段裁剪白名单（Grilling 后确认）

| 阶段 | 白名单 | 说明 |
|------|--------|------|
| prepare | get_writing_context, get_chapter_list | 一次调用获取全貌 |
| outline | edit, read, get_characters | 大纲需要参考角色信息 |
| write | edit, read, create_item_occurrence | 根据大纲写正文，记录物品流转 |
| review | 14 个 get_* + edit + read | 审稿需要全面比对 |
| maintain | 所有 create/update + search_* + set_phase + edit(goink.md) | 维护需要所有写入工具 |

### 14.3 预计总效果

```
当前每轮固定开销：~17,000 token
方案 3 后：~1,500 token（节省 91%）

当前单章总消耗：~140K token
方案 3 后：~85K token（节省 39%）

当前 100 章成本：~¥30
方案 3 后：~¥11（节省 63%）
```

---

## 十五、Grilling Session 总结

### 15.1 关键决策

| 决策 | 结论 |
|------|------|
| outline 阶段是否需要 get_characters | ✅ 需要，加入白名单 |
| writing_context 增强范围 | ✅ 一次调用获取全貌，增加必要字段 |
| 描述精简策略 | ✅ 保守策略，高混淆工具对保留关键词 |
| 分阶段裁剪实现 | ✅ 动态 tools 参数 |
| 工具搜索算法 | ✅ Embedding 相似度（Phase 4） |
| maintain 检查清单 | ✅ 保留 writing-kernel.md |
| write 阶段伏笔查询 | ✅ 不需要，靠大纲 |
| Token-Efficient Tool Use | ✅ 始终启用，风险极低 |

### 15.2 待办事项

- [x] 分析所有工具的返回数据，确认哪些字段可以精简
- [x] 设计 outline 阶段白名单（含 get_characters）
- [x] 设计 maintain 阶段白名单（含所有 create/update）
- [x] 实施描述精简（保守策略）
- [x] 实施分阶段裁剪（动态 tools）→ 实际改为全量发送 + allowed_tools 限制（为保缓存前缀稳定）
- [x] 实施 writing_context 增强
- [x] 实施 Prompt Caching（消息顺序优化）
- [x] 测试验证缓存命中率（89-93%，见 archive/billing-test-report.md）
- [ ] 实施 Token-Efficient Tool Use（待评估，Claude 4+ 专属）
