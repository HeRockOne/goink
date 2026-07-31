# Goink 潜在优化综合评估报告

> 日期：2026-07-31
> 审计方式：两个探索子代理深度读码 + 联网交叉比对行业成熟方案
> 前置原则（产品定位最高指令）：**Goink 是 AI 创作小说软件，创作质量永远第一，省 token/省资源永远第二。** 任何优化若可能损害创作能力，一律不做。

---

## 〇、结论先行

本报告对 Goink 的上下文管理、压缩机制、缓存架构做了全面审计，并与 Anthropic/LangChain/文枢(WenShape)/Novel-creator-skill 等行业成熟方案交叉比对。

**核心判断**：Goink 的架构设计**已经非常先进**——"结构化外脑(DB) + 阶段门禁 + maintain 清单 + 跨对话快照(goink.md) + 稳定前缀缓存"这套组合与行业最佳实践高度一致，部分机制（阶段门禁硬拦截、代码级质量闭环）甚至优于多数竞品。**不建议做伤筋动骨的改动。**

但审计发现 **2 个不损害创作质量、纯收益的真实优化点**，以及 **3 个需要谨慎权衡的改进点**，详见下文。

---

## 一、现有架构质量红线段（必须保留）

子代理审计确认以下机制是创作质量的生命线，**任何优化都不得触碰**：

| 红线机制 | 位置 | 作用 |
|---------|------|------|
| 阶段门禁硬拦截 + require | phase_gate.go, agent.go:443 | 防跳步、强制 maintain |
| maintain 15 项清单 | writing-kernel.md:50-68 | 长篇一致性执行层 |
| "工具是唯一真相来源" | identity.go:129 | 防 LLM 幻觉污染设定库 |
| get_writing_context + overdue 预警 | writing_context_tools.go | 写前防遗忘雷达 |
| 伏笔/弧线/读者认知三套追踪 | timeline/storyarc/reader_perspective | 长篇叙事记忆基质 |
| 代码级闭环（回收带章号/误知带真相/换主记流转） | timeline_tools.go:236 / reader_perspective_tools.go:191 / item_tools.go:127 | 能埋能收 |
| review 子代理 + require=run_subagent | identity.go:199, 门禁配置 | 写后独立审读 |
| 工具 Description 里的创作方法论 | 各 *_tools.go | 世界观分类/伏笔节奏/悬念设计 |
| NovelState 动态注入 | novel_state.go, chat.go:207 | 跨对话一致性 |

**明确禁止的优化方向**（已被本项目"创作质量第一"原则否决）：
- ❌ 精简工具 description/schema 省 token（丢失创作方法论）
- ❌ 按阶段裁剪 tools 换缓存（破坏前缀，ADR-0001 已否决）
- ❌ 删减系统提示词里的创作规则

---

## 二、行业成熟方案调研（联网交叉比对）

### 2.1 Anthropic 官方（Claude Code）
- **Compaction**：上下文接近上限时 LLM 摘要 + 保留最近 5 个文件/轮次，目标是"最高保真度浓缩"
- **结构化笔记（agentic memory）**：把关键状态写到上下文之外的文件（如 NOTES.md），跨压缩周期持续存在
- **多代理架构**：子代理独立上下文，只返回浓缩摘要（1-2K tokens）
- 核心原则："找到最小的高信号 token 集合，最大化期望结果概率"

### 2.2 LangChain Deep Agents
- 工具结果 >20K tokens → 落盘，上下文只留文件路径 + 前 10 行预览
- 大工具输入（旧 write/edit 调用）→ 85% 阈值时替换为磁盘指针
- 摘要 + 原始消息全文落盘（可恢复）
- **关键**：触发压缩阈值建议 70-80%（留缓冲，防摘要调用本身溢出）

### 2.3 文枢 WenShape（同类：长篇小说上下文工程）
- **Token 预算分配**（128K）：系统规则 5% / 设定卡 15% / 事实表 10% / 历史摘要 20% / 当前草稿 30% / 输出预留 20%
- **动态事实表（Canon）**：每章结束提取新事实（角色变化/地点转移/物品获取）写入 JSONL，作为长篇一致性真值源
- **两级选择引擎**：确定性必选 + 检索式 Top-K
- **星级重要性**：设定卡重要程度影响注入优先级

### 2.4 Novel-creator-skill（百万字一致性）
- 五层机制：五步质量门禁 + RAG 两级检索 + 知识图谱 + 大纲锚点 + 跨 Agent 审核
- **低上下文策略**：写前默认只读 plan+state 两个文件，单章前置读取上限 4 个文件，RAG 只回 Top-K 不读整章

### 2.5 行业共识（多来源一致）
1. **压缩触发 70-80%**（非 90%+），留摘要调用缓冲
2. **保留最近 2-4 轮原文**，更早的进结构化摘要
3. **摘要结构化**（goal/progress/state/open issues/next steps 分段），不写自由文本
4. **外部持久化是防丢失真值源**（git/文件），摘要只指向不重编码
5. **工具输出写时截断/落盘**，不在压缩时才处理（"filter at ingestion, not at compression"）
6. **上下文漂移监控**：目标措辞变化、重复已完成工作、参数错误是前兆

---

## 三、逐项交叉比对：Goink vs 行业最佳

| 维度 | 行业最佳 | Goink 现状 | 差距 |
|------|---------|-----------|------|
| 结构化外脑 | Canon/Truth Files/知识图谱 | DB 25 表 + 三套追踪 + snapshot | ✅ 已对齐甚至更优 |
| 压缩触发阈值 | 70-80% | 0.7（可配 0.3-0.95） | ✅ 一致 |
| 压缩保留窗口 | 最近 2-4 轮原文 | 最近 15 条 user 消息 | ⚠️ 见问题 P1 |
| 压缩摘要格式 | 结构化分段（goal/decisions/next） | 5 节：已完成/断点/偏好/关键决策/待办 | ✅ 对齐 |
| 摘要策略 | 锚定迭代（增量合并） | 全量重建 | ⚠️ 见问题 P2 |
| 工具输出写时处理 | >20K 落盘 / 写时截断 | 完整入历史，零截断 | ⚠️ 见问题 P1 |
| 外部持久化真值源 | git + 文件 | 每小说 git 仓库 + goink.md | ✅ 一致 |
| 跨压缩持久记忆 | NOTES.md / agentic memory | goink.md + NovelState 重建 | ✅ 一致 |
| 缓存前缀稳定 | 稳定 system+tools | 全量工具 + 稳定前缀 + 哈希监控 | ✅ 一致 |
| 子代理 | 独立上下文返摘要 | review/memory 子代理 | ✅ 一致 |
| Token 预算分配 | 显式分池（文枢） | 自然增长到阈值压缩 | ⚠️ 可选改进 |
| 上下文漂移监控 | 目标措辞/重复检测 | 前缀哈希 + 死循环检测 | ⚠️ 可选改进 |

---

## 四、审计发现的潜在优化点

### P1【不损害质量，纯收益】压缩保留窗口误判 + 工具结果全量入历史

**问题**：压缩保留的是"最近 15 条 user 消息"，但 `<system-reminder>` 注入、门禁拦截、审批反馈**全是 role=user**（agent.go:425,434,449,467）。一章创作产生 3-8 条注入，15 条 user 实际只覆盖 **2-5 个真实用户回合**。更早的创作决策完全依赖摘要提取质量。

**风险**：伏笔/设定若在对话中口头敲定而未及时落库，压缩后可能丢失 → 后续章节脱节。

**行业对照**：Anthropic 保留"最近 5 个文件"，LangChain 保留"最近轮次"，都按真实轮次而非消息条数。

**建议（质量安全）**：
1. `retainMessages` 改为**按真实用户回合**保留（或把 inject 消息从 user 计数中剔除，单独计数）
2. **保守做法**：保留窗口从"15 条 user"提升到"最近 N 个真实回合 + 其后的全部消息"

**预期收益**：压缩丢失的创作决策显著减少，长篇一致性更有保障。**这是质量收益，不是省 token。**

### P2【不损害质量】压缩摘要从"全量重建"改为"锚定迭代"

**问题**：`Compress`（compress.go:68-128）每次从零总结全部历史。行业（Factory 36K 会话评估）证明**锚定迭代**（只总结新丢弃的区间，合并进持久 anchor）比全量重建准确率高（4.04 vs 3.74）。

**建议**：压缩时保留上一次的摘要作为 anchor，只对"本次新增的、即将被丢弃的区间"做增量摘要，合并进 anchor。goink.md 已经天然是"锚定状态"（NovelState 从它重建），可以把摘要 anchor 也沉淀到 goink.md 或单独文件。

**风险**：实现复杂度中等。需保证 anchor 文件本身不被压掉（建议写入 DB 或 goink.md 旁）。

### P3【可选项】压缩后 NovelState 双份

**问题**：`persistCompression`（compress.go:250-254）把 NovelState 烘进压缩新版本，但下一轮 `chatImpl` 又动态注入一份（chat.go:207-212）。压缩后每次对话带**两份 NovelState**（旧快照 + 新快照），浪费且可能给模型两份不一致状态。

**建议**：压缩时**不**把 NovelState 写入新版本（它反正会被动态注入最新的）。纯收益，省 1-5K token，且消除状态不一致风险。

**风险**：零。这是纯粹的冗余消除。

### P4【可选项】估算 token 不含 tools 前缀，小窗口模型可能误判

**问题**：`InitRunningTokens` 只估算消息 token（token_counter.go），**不含 40-50K 的 tools 固定前缀**。对 200K 窗口（GLM）或 128K 兜底模型，70% 阈值触发压缩时实际请求可能已超窗。

**建议**：压缩阈值判断时把 tools 前缀估算加入（`sumRunningTokens + toolsTokens`），或对小窗口模型调低默认阈值。

**风险**：低。只影响压缩触发时机，不影响质量。需注意 DeepSeek 1M 窗口下此问题不存在。

### P5【可选改进】显式 Token 预算分配（借鉴文枢）

**建议**：文枢的"系统5%/设定卡15%/事实表10%/历史摘要20%/草稿30%/输出预留20%"预算模型很成熟。Goink 现在是自然增长到阈值。可考虑在 `get_writing_context` 返回前按预算控制各块大小（写时裁剪），但这属于**优化窗口内的内容选择**，需谨慎——宁可多带也不要让 AI 缺上下文。

**决策**：**暂缓**。当前 prepare 阶段一次拉全貌 + 门禁 require 强制读取，已是"宁可多带"的保守策略，改预算分配有丢上下文风险，不符合质量第一。

### P6【可选改进】上下文漂移监控

**建议**：行业强调监控"目标措辞漂移/重复已完成工作/参数错误"作为退化前兆。Goink 已有前缀哈希 + 死循环检测 + 工具失败次数。可加一个轻量"重复已完成工作"检测（如检测到同章节重复 edit 时告警）。

**风险**：低。非核心。

---

## 五、综合建议（按优先级，以创作质量为准绳）

| 优先级 | 动作 | 类型 | 质量影响 | token 影响 | 风险 |
|--------|------|------|---------|-----------|------|
| **P1** | 压缩保留窗口改为按真实回合（剔除注入消息计数） | 质量修复 | ✅ 显著提升 | 无 | 低 |
| **P2** | 压缩摘要改锚定迭代（增量合并） | 质量修复 | ✅ 提升 | 略省 | 中 |
| **P3** | 压缩后去除冗余 NovelState 双份 | 冗余消除 | 无（消除不一致） | 省 1-5K | 零 |
| **P4** | 压缩阈值计入 tools 前缀 | 正确性修复 | 无 | 提前压缩略省 | 低 |
| **P5** | 显式 Token 预算分配（文枢式） | 架构优化 | ⚠️ 有丢上下文风险 | 省 | 中高 |
| **P6** | 上下文漂移监控（重复工作检测） | 可观测性 | ✅ 间接提升 | 无 | 低 |

**明确不做**：
- ❌ 精简工具 description/schema
- ❌ 按阶段裁剪 tools
- ❌ 删减创作规则

---

## 六、执行原则（写入 AGENT.md 的呼应）

1. **每个优化落地前先问：创作质量会受影响吗？** 会则不做
2. **P1/P2/P3 是"质量优先"方向**（减少信息丢失/不一致），优先做
3. **P4 是正确性修复**，顺带省 token
4. **P5 有质量风险，除非用户明确要求省成本，否则不做**
5. **每步落地后需实测验证**：写 3 章，对比压缩前后的伏笔回收率、设定一致性、审稿问题数

---

## 七、参考

- Anthropic Context Engineering: https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- LangChain Deep Agents Context Management: https://www.langchain.com/blog/context-management-for-deepagents
- 文枢 WenShape: https://github.com/hkxiaoyao/WenShape
- Novel-creator-skill: https://github.com/Sivyer9303/novel-creator-skill
- ACON (Microsoft): https://www.microsoft.com/en-us/research/publication/acon-optimizing-context-compression-for-long-horizon-llm-agents/
- 相关内部文档：`docs/adr/0001-prompt-caching.md`、`docs/architecture/token-injection.md`、`docs/archive/billing-test-report.md`
