# 链路优化方案 v2

> 目标：消除消息链路中不影响创作质量的"无意义字节"，降低 token 消耗与成本。
> 对比基准：5 轮门禁创作（init → prepare → outline → write → review → maintain × 5），
> 当前成本 ¥0.195/章，优化目标 ¥0.12-0.13/章。

## 方案 1：auto-inject require_reads（核心收益 -33.6% miss）

**现状**：各阶段必读技能通过 `read_required` 工具调用加载，模型发 tool_call → 返回 tool_result(技能内容)。事前强制拦截兜底（未读技能时拦 edit 等创作动作）。

**优化**：`set_phase` 成功时，代码自动从 skill store 读取该阶段 `require_reads` 技能，以 system 消息注入到下一轮（NS 后固定位置）。同时调用 `OnReadRequired` 标记已读。

**省掉**：read_required 的 tool_call + tool_result 两跳，以及事前强制拦截时 model 被拦后的重试 system-reminder。

**改动**：agent.go `set_phase` 成功处理处 + `buildRequiredSkillsContent` 方法。

## 方案 2：set_phase 自动推进（增量 -2.8% miss）

**现状**：require 满足后，模型必须主动调 `set_phase` 切换阶段，产生 tool_call + tool_result + 成功提醒 system-reminder 三轮消息。

**优化**：`CheckTransitionReady` 满足时，代码自动执行 `set_phase`，不经过 LLM。

**省掉**：set_phase 的 tool_call + tool_result + reminder 三轮消息。

**改动**：`phase_gate.go` 加自动推进逻辑 + `agent.go` 去掉 set_phase 的 reminder 生成。

## 方案 3：去 catalog（增量 -2.4% 总输入,主要省上下文空间）

**现状**：`BuildSkillCatalog` 把 30+ 个 auto 模式 skill 的 name+description 注入固定前缀。

**优化**：auto-inject 后技能已由系统注入，auto 技能不再需要出现在 catalog 中。auto 改 manual 模式，不进 catalog。

**省掉**：catalog 中 30+ 条 name+description（约 680K 缓存命中字节，主要释放上下文窗口，非省钱）。

**改动**：`skill_catalog.go` 的 `ListMetaForCatalog` 过滤逻辑，或改 skill 的 mode。

## 模拟结果汇总

| 方案 | 总输入 | miss | 成本/5章 | 每章 | 降幅 |
|------|--------|------|---------|------|------|
| 现状(now) | 35.05M | 280K | ¥0.975 | ¥0.195 | — |
| +方案1(inject) | 24.36M | 186K | ¥0.669 | ¥0.134 | **-31.3%** |
| +方案2(自动推进) | 23.68M | 181K | ¥0.651 | ¥0.130 | **-33.3%** |
| +方案3(去catalog) | 23.00M | 179K | ¥0.635 | ¥0.127 | **-34.9%** |

## 实施优先级

P0：方案1（auto-inject）— 核心收益
P1：方案2（自动推进）— 顺手做
P2：方案3（去catalog）— 释放窗口，非省钱