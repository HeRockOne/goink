# TODO — 待落地清单

> 记录模拟验证结论对应的业务落地事项，防止上下文丢失。
> 规则：业务代码改动一律**待真机验证后**再实施；模拟器是业务代码的建模镜，不是反向约束。
> 每个待办含技术实现细节（文件/函数/行号/验证方法），供后续接手 AI 直接执行。
> 审视告警（2026-08-09 用户提出）：模拟器存在"为指标而指标"风险——质量分只算流程环节存在性，
> **未建模注意力衰减/遗忘/压缩触发/工具重试**，批量大章数的"省 40%"是乐观下界，真机验证前不可当结论。

## 背景

cacheprobe 模拟验证（2026-08-09，DeepSeek 价 0.02/1/2）：
- 批量模式每章成本 ¥0.09-0.10 vs 单章 ¥0.24-0.28（省 60-66%），真实日志方向一致
- 批量质量短板：第 2-N 章无审稿、无写后自检；补齐方案 = 三章一轮批内检查（白金方法论），成本仅 +6.1%
- 当前业务代码（auto-inject + 技能注入 + 批量循环）**无真机数据**，所有数字来自模拟器

## 待办清单

### P1. 真机验证 auto-inject 缓存表现（前置，所有业务落地的前提）

**目标**：拿到 auto-inject 时代的真实 usage 数据，校准模拟器；确认 99.3%（模拟）与真实命中率的差距来源。

**技术实现（只读操作，零改动）**：
1. 桌面端跑一轮真实创作（单章 1 轮 + 批量 5 章各一次，模型用 mimo-v2.5 或 DeepSeek）
2. 查真实 usage：`D:\Goink\goink.log` 中 grep `UPDATE model_usage`，提取每轮 `hit_tokens`/`miss_tokens` 增量
   ```powershell
   Get-Content "D:\Goink\goink.log" -Encoding UTF8 | Select-String -Pattern "UPDATE .model_usage." 
   ```
3. 对照点：
   - 新会话首轮全 miss 量（模拟单章 1 轮首轮 ~29.5K = tools 12.9K + 固定前缀+initInject 16.6K）
   - 大轮（一章完整门禁）miss 增量（模拟 ~73K/章）
   - 累计命中率 hit/(hit+miss)（模拟完整窗口 99.3%）
4. 有出入 → 修正模拟器建模（服务端缓存行为可能与字节前缀假设不同），**业务代码不动**
5. 关注两条告警日志：
   - `前缀变化，缓存可能失效`：internal/agent/agent.go:321-328（computePrefixHash + computeSystemBlockHashes 定位哪个 system 块变了）
   - `全量缓存未命中`：internal/agent/tokens.go:224（hit=0 且 miss>1 万，turn>1 出现才算异常）

**完成标准**：真实 vs 模拟的 miss 构成/命中率对照表，差异原因说明。

---

### P2. 批量模式：批内三章一轮检查（模拟已验证 checkKind=2）

**目标**：批量 write 循环每 3 章插一次批次检查（子代理审最近 3 章 + 修复），对齐白金"三章一轮"，成本 +6.1%。

**背景知识（重要）**：批量循环**不是 Go 代码循环**，是 LLM 按 kernel 技能驱动的流程（internal/skill/builtin/main-core-writing-kernel.md:19-25：`prepare → outline（一次出 N 章大纲）→ write（循环 N 章，每章 read 本章大纲 + 迷你维护）→ review → maintain`，write 阶段门禁配置 `loop: true`）。所以落地点是**技能文档 + 门禁配置**，不是 agent.go。

**技术实现（两步）**：
1. **main-core-writing-kernel.md 批量流程段**（第 19-25 行附近）加"批次自检"步骤：
   > 批量 write 循环每 3 章执行一次批次检查：`set_phase("review")` → run_subagent 审读最近 3 章（结构/逻辑/伏笔/AI 味）→ 修复问题 → get_chapter_list 字数复查 → `set_phase("write")` 继续循环
   （注意对齐 main-cmd-phase-gate.md:30 的批量流程描述，两处同步）
2. **门禁配置零改动**，已验证可行性：
   - run_subagent 白名单在 review 阶段（门禁配置示例.md batch review 段），write 阶段会拦截（internal/agent/phase_gate.go:466-505 CheckToolAllowed）→ 必须 set_phase("review") 后调用
   - write→review 是 next 推进（phase_gate.go:374），review→write 是回退到 visited 阶段（phase_gate.go:380-385）✓
   - batch review 段配置无 auto_skill_injection → 切 review 不注入技能；set_phase("write") 会重复注入 write 技能（成本已计入，无需处理）
   - set_phase("write") 会重置 wordCountOK（phase_gate.go:334-336）→ 批次检查里必须调 get_chapter_list 完成字数校验再回 write

**模拟器对照**：internal/cacheprobe/sim.go `batchGatePlaysWith(chapters, checkKind=2, checkEvery=3)` + `batchCheckPlays`（走阶段切换的实现），`go test ./internal/cacheprobe -run TestDiagBatchSelfReview`。

**预期**：批量 5 章 ¥0.0915 → ¥0.0971/章（+6.1%，仍比单章省 64.7%）。

**完成标准**：kernel 技能批量段含批次自检说明，真机跑批量 5 章验证子代理审稿实际触发 + 成本符合预期。

---

### P3. 批量大小建议与上限

**目标**：批量推荐 5-10 章/批（模拟：3 章 ¥0.119 最贵档、10 章 ¥0.084；批边界 2批×3章比单批6章贵 24%）；上限受上下文窗口/压缩点约束。

**智能决策规则（用户拍板，2026-08-09）**：
- 自检节奏 = **3 的倍数**（3/6/9 章），对齐白金"三章一轮"
- 批量 **<6 章**（3/4/5）→ **完整门禁流程**（每批 review+maintain，覆盖 100%）
- 批量 **≥6 章** → **批内检查**（第 3/6/9 章触发，覆盖 100% 且 maintain 只 1 次）

**重要修正（架构审视发现，模拟器未建模）**：批量上限由**模型窗口**决定，不是拍脑袋 10 章：
- 压缩阈值 = 0.7×ContextWindow（internal/agent/agent.go:307-314，compress.go 触发）
- 估算：固定前缀 ~34K + 每章增量 ~7K（正文 4K + thinking 0.5K + 工具结果 2K + NS 0.7K）
- mimo-v2.5 128K 窗口：安全批量 ≈6-8 章（0.7×128K=90K）；10 章 ≈104K **必触发压缩**
- deepseek-v4-pro 1M 窗口：10+ 章无压力
- 模拟器未建模压缩（README 明确"模拟未含压缩重置"）、注意力衰减（lost-in-middle）、thinking 波动、工具重试——**批量大章数的"省 40%"是乐观下界，P1 真机验证前不可当结论**

**技术实现**：
1. **文档侧**（已部分完成）：门禁配置示例.md 头部批量使用建议 + kernel 技能批量段（P2 时同步）
2. **压缩触发点模拟**（cacheprobe 目前无压缩模拟）：
   - internal/cacheprobe/sim.go 加压缩阈值：`sumRunningTokens` 超过模型 ContextWindow×0.7（对齐 internal/agent/compress.go 阈值逻辑）时插入压缩重建（TokenCache.Reset + summary 消息）
   - 按模型窗口参数化跑批量 6/8/10/12 章，找压缩触发点与每章成本拐点
3. **设置面板**：批量章数输入已有（frontend/src/components/settings/CacheSimTab.tsx batchChapters），业务侧批量章数由用户输入决定，文档给出建议即可

**完成标准**：cacheprobe 支持压缩模拟（按模型窗口），输出"批量章数 × 压缩触发"表，文档给出推荐上限（mimo 6-8 章 / deepseek 10+ 章）。

### P6. 模拟器 API 简化（用户批评过度设计，参数泥潭）

**现状**：`batchGatePlaysWith(chapters, checkKind, checkEvery)` 三个魔法参数（checkKind 0/1/2/3 + checkEvery），在一个函数里打补丁堆出 4 种方案，难读难维护。

**目标**：按场景拆命名函数，需要什么直接引用：
```go
// 现状（参数泥潭）
batchGatePlaysWith(5, 2, 3)   // 什么意思？要读文档

// 目标（命名即语义）
batchAsIs(5)                    // 现状：攒批统一审
batchLightSelfCheck(5, 3)       // 每 3 章轻量自检
batchInCheck(5, 3)              // 每 3 章批内检查（子代理审最近 N 章）
batchFullCycle(5, 3)            // 每 3 章完整门禁流程（review+maintain）
```
内部共享一个核心构造函数（plays 组装），对外暴露语义化入口；diag 测试同步改用新 API。
**原则**：模拟器是业务代码的建模镜，结构应跟业务概念走（方案/节奏），不是参数矩阵。

---

### P4. 技能注入去重（可选优化，待真机确认后评估）

**现状**：internal/agent/agent.go:215-239 `injectPhaseSkills` 每次 set_phase 无条件注入（每章重复注入相同技能全文 ~9.2K miss/章，占 miss 约 10%）。

**技术实现**：
1. internal/agent/phase_gate.go 已有技能状态：`OnSkillInjected(skillName)`（phase_gate.go:164）、`missingInjections`（phase_gate.go:202，按 AutoSkillInjection 配置查缺失）——先读这两个函数确认 injectedSkills 的存储与查询
2. agent.go injectPhaseSkills（215-239）注入前检查：
   ```go
   // 伪代码：已注入过则跳过（同阶段重复 set_phase 不重复注入）
   if pg.HasInjected(phase) { return }
   ```
   需要 PhaseGate 加 `HasInjected(phase string) bool`（检查该阶段 AutoSkillInjection 的技能是否都已注入，注意通配符 `*` 技能名，对齐 missingInjections 的匹配逻辑）
3. 风险：门禁 require 依赖技能注入状态（missingInjections 用于 set_phase 阻塞 + 事前技能强制）——去重不能破坏"必读技能未加载前禁止创作动作"的语义（phase_gate.go:491-496），只跳过"已注入过的重复注入"，不跳过首次注入

**模拟器对照**：internal/cacheprobe/sim.go phaseInjectSkills 注入逻辑（batchGatePlaysWith set_phase 分支），加"已注入跳过"后对比 miss（预期每章省 ~9.2K）

**完成标准**：真机前后对照（P1 数据 vs 去重后数据），确认 miss 下降且创作质量不降（技能内容在上下文中仍存在）。

---

### P5. 模拟器已知差异（口径级，不影响结构结论）

| 差异 | 位置 | 收敛方式 |
|---|---|---|
| thinking 用阶段均值（真实逐次波动） | internal/cacheprobe/sim.go `phaseThinkChars`/`thinkingText` | 保持（均值口径已够用） |
| 正文用目标字数+正态波动（真实逐章波动） | internal/cacheprobe/sim.go `initBody`/`realWordStdDev` | 保持 |
| set_phase reminder 内容硬编码（真实 StatusString 动态） | internal/cacheprobe/api.go `phaseReminder` | 可对齐 StatusString 结构，几十 token 级噪声，低优先 |
| maintain 查询结果固定大小（真实随 DB 状态增长） | internal/cacheprobe/sim.go `maintainPlays` | 若 P3 压缩模拟做了，顺带按章数缩放 maintain 查询结果 |
| 无门禁拦截模拟（play 序列假设全部工具放行） | internal/cacheprobe/sim.go plays 执行 | 已在 P2 通过"阶段切换序列"规避（run_subagent 只在 review 阶段调用）；如未来加新工具调用，先查门禁白名单 |

## 结论速查（质量 × 成本 × token，批量 5 章，2026-08-09 实测）

质量分 = 写后自检(3) + 审稿覆盖×3 + 状态实时(2) + 写前对齐(1) + 章纲/防串章(1)，满分 10。

| 方案 | 输入hit | 输入miss | 输出out | 成本¥/章 | 省vs单章 | 审稿覆盖 | 自检节奏 | 质量分 | 质量/成本 |
|---|---|---|---|---|---|---|---|---|---|
| 单章 5 轮（基准） | 37,860,058 | 398,268 | 109,753 | 0.2750 | 0% | 100% | 每章 | 9.0 | 32.7 |
| 批量现状（攒批统一审） | 10,486,070 | 128,628 | 52,221 | 0.0886 | 67.8% | 20% | 无 | 4.1 | 46.3 |
| 批量 + 每章轻量自检 | 12,837,222 | 155,113 | 56,536 | 0.1050 | 61.8% | 0% | 每章 | 6.5 | 61.9 |
| 批量 + 三章一轮·轻量自检 | 10,937,647 | 133,925 | 53,084 | 0.0918 | 66.6% | 0% | 每3章 | 6.0 | 65.4 |
| 批量 + 三章一轮·批内检查 | 11,711,939 | 139,534 | 55,869 | 0.0971 | 64.7% | 60% | 每3章 | 7.8 | 80.3 |
| 批量 + 三章一轮·完整门禁流程 | 14,054,200 | 149,661 | 62,155 | 0.1110 | 59.6% | 100% | 每3章 | 9.0 | 81.1 |

> 注：批内检查的审稿覆盖 60% 是批量 5 章边界效应（第 3 章后触发 1 次）；批量 6 章时
> 第 3/6 章触发覆盖 100%，此时批内检查 ¥0.0991/章（性价比 90.8）优于完整门禁流程
> ¥0.1033（87.1）——见 TestDiagBatchCheckCoverage。

解读：
- **决策规则（批量大小 × 方案）**：
  - 批量 **≥6 章**（三章一轮完整触发：第 3/6/9 章）：**批内检查最优**（质量 9.0 与完整门禁流程相同，maintain 只做 1 次，成本低 4.1%，性价比 90.8）
  - 批量 **5 章**（检查只触发 1 次、覆盖 60%）：**完整门禁流程更好**（多 ¥0.014/章换 1.2 分质量，性价比 81.1 > 77.8）
- 三章一轮·批内检查是批量 6 章+ 的性价比最优；要满分质量且批量小（3-5 章）选完整门禁流程
- 单章性价比最低（32.7）：质量 9 分但贵 3.1 倍
- 批量现状性价比 46.3 但质量 4.1（第 2-N 章零审稿），不可取
