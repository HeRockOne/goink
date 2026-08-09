# 链路优化方案 v2 — 最终决策记录

> 目标：消除消息链路中不影响创作质量的"无意义字节"，降低 token 消耗与成本。
> 对比基准：5 轮门禁创作（init → prepare → outline → write → review → maintain × 5），
> 当前成本 ¥0.195/章，优化后 ¥0.130/章（省 33%）。

## 实施状态

| 方案 | 状态 | 每章成本 | 省幅 | 质量影响 |
|------|------|:--------:|:----:|:--------:|
| 现状(优化前) | — | ¥0.195 | — | — |
| auto-inject + 自动推进 | **已实施** | **¥0.130** | **33%** | 0 |
| 删系统提示词阶段表格 | **已实施** | — | 微 | 0 |
| 配置字段统一 + 工具重命名 | **已实施** | — | — | 0 |

## 已实施方案

### 1. auto-inject require_reads（核心收益）

**现状**：各阶段必读技能通过 `auto_skill_injection` 工具调用加载，模型发 tool_call → 返回 tool_result(技能内容)。事前强制拦截兜底。

**优化**：`set_phase` 成功时，代码自动从 skill store 读取该阶段 `auto_skill_injection` 配置的技能，以 system 消息注入到下一轮（NS 后固定位置）。同时调用 `OnSkillInjected` 标记已读。

**省掉**：`auto_skill_injection` 的 tool_call + tool_result 两跳，以及事前强制拦截时 model 被拦后的重试 system-reminder。

**改动文件**：`agent.go`(injectPhaseSkills)、`auto_skill_injection_tools.go`(共享 BuildSkillsContent)、`phase_gate.go`(字段/函数重命名)

### 2. set_phase 自动推进

**现状**：require 满足后，模型必须主动调 `set_phase` 切换阶段，产生 tool_call + tool_result + 成功提醒三轮消息。

**优化**：`CheckTransitionReady` 满足时代码自动执行 `set_phase`，不经过 LLM。

**省掉**：set_phase 的 tool_call + tool_result + reminder 三轮消息。

**改动文件**：`agent.go`(自动推进逻辑)

### 3. 删系统提示词阶段表格

**现状**：`mainAgentSystem1` 中有一份 6 行 markdown 阶段表格（~640 token），与常驻 kernel skill 90% 重复。

**优化**：删除表格，替换为一句引用。表格中 2 条唯一指令（`volume_entities` 检查和 `get_entity_appearances` 反查）已补入 kernel prepare 阶段。

**附加**：kernel 复制一份到 `internal/skill/builtin/` 作为内置备份（同名优先级 novel > user > builtin，防止误删）。

**改动文件**：`identity.go`、`main-core-writing-kernel.md`、`default_phase_gate_config.go`

### 4. 配置字段统一 + 工具重命名

**现状**：工具名 `read_required` 与门禁配置字段 `require_reads` 名称混淆。

**优化**：工具重命名为 `auto_skill_injection`，配置字段改为 `auto_skill_injection:`。Go 内部标识符同步重命名（`RequireReads` → `AutoSkillInjection`、`OnReadRequired` → `OnSkillInjected` 等）。

**改动文件**：`auto_skill_injection_tools.go`（原 `read_required_tools.go`）、`phase_gate.go`、`agent.go`、`identity.go`、默认配置、示例配置

## 已排除方案

### A. 精简 writing_context 工具描述

**风险大，不动。** 工具描述是 AI 学习返回结构字段的唯一来源，同时包含多项创作红线规则（`dead=不得出场`、`禁止提前展开后续卷情节` 等）。精简空间有限（约 15-20%），但删过头会导致 AI 不知道响应字段存在，或丢失创作红线规则。

### B. NS 缩短 800 字符

**数据不支持。** 实际每章指纹约 200-300 字，当前 1500 字符覆盖 4 章（刚好满足防重复检查需要的 3 章）。800 字符只覆盖 2 章，不够防重复检查。不能缩。

### C. 精简子代理完整历史 fork

**模拟验证否定。** 子代理 fork 完整主历史时，子代理请求能命中主会话缓存（99.24%命中率）。精简后子代理只命中固定前缀，命中率降到 97.92%，成本反而增加 1.4 倍。保持现状。

## 模拟验证结果

```
完整门禁 5 轮,子代理 fork 完整历史:
现状(now)        : hit=34718150 miss=279626 命中率=99.20% 成本=¥0.9740 (0.1948/章)
优化1+2(inject+自动推进) : hit=23453630 miss=180445 命中率=99.24% 成本=¥0.6495 (0.1299/章)
```

每章 ¥0.195 → **¥0.130**,省 **33%**,命中率 99.24%,质量零影响。

## 排除项记录

### 子代理历史精简 — 缓存影响验证

**结论：不能精简，精简反而更贵。**

子代理 fork 完整主历史时，子代理请求命中主会话缓存。精简后子代理不再命中，命中率从 99.24% 跌到 97.92%，miss 从 180K 涨到 466K，成本增加 39%。

### NS 缩短 — 实际数据验证

**结论：不能缩。**

goink.md 实际每章指纹约 200-300 字，当前 1500 字符覆盖 4 章（第 16-19 章）。800 字符只覆盖 2 章（第 18-19 章），而防重复检查需要最近 3 章。缩到 800 不够。

### writing_context 描述精简 — 安全评估

**结论：风险大于收益，不动。**

工具描述中的字段名是 AI 学习响应结构的唯一来源，嵌入规则是创作红线。精简只能省约 15-20%，但一旦删过头，AI 不知道响应字段存在或丢失创作规则，得不偿失。