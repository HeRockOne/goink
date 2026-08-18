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
6. **批量长循环技能衰减观察**（方案 B 验证点：技能只在 outline→write 注入一次）：批量 10 章后观察
   write 4 技能规则是否仍被遵守（质量是否衰减）。若衰减，补救 = 每 3 章自检顺带 read write 核心技能
   （非重复注入）；业界共识：规则性指导注入即内化，重复注入 = miss + 注意力浪费，状态新鲜由
   miniMaintain/查询保证，非技能注入职责

**完成标准**：真实 vs 模拟的 miss 构成/命中率对照表，差异原因说明。

---

### P2. 批量模式：批末全批审稿 + 每 3 章轻量自检（业界组合，2026-08-09 定案）

**背景**：原"批内三章一轮检查（batchInCheck，阶段切换子代理）"经 websearch 对照业界长篇框架（ainovel-cli / novel-creator / QMAI / webnovel-writer / xuanji-write）判定**过度复杂**：
- 业界共识 = **轻量高频 + 重型低频**：每章/每 3 章做轻量自检（状态对照/正则/技能自检，零阶段切换），每 10 章/段级才上 LLM 子代理评审
- 批内检查每 3 章子代理 + 7 步阶段切换（set_phase review→write）是"重型高频"，且 plays 存在"子代理审 3 章但只 read 1 章修 2 处"的不一致
- ainovel-cli 原话："越简单越稳定，拒绝复杂编排"

**定案方案（batchLightEndReview，模拟实测性价比 94.4 全场第一）**：
1. **每 3 章轻量自检**（对齐白金"三章一轮"）：selfReviewPlays（2 技能 + 1 修改），不跳阶段，+0.3%
2. **批末全批审稿**（对齐业界"段级评审"）：run_subagent 审全批（子代理 fork 完整主历史，正文天然在上下文中）+ 主会话**查 N 修 N**（每章 read + 修复 1 处）+ 字数复查，+4%
3. 每章 miniMaintain（状态实时）+ 字数校验 + 读大纲（轻量机制，现状已有）
4. 零阶段切换、零技能重复注入

**模拟数据（批量 5 章，now 协议，DeepSeek 价）**：
| 方案 | 成本¥/章 | 省vs单章 | 审稿覆盖 | 自检节奏 | 质量分 | 质量/成本 |
|---|---|---|---|---|---|---|
| 批量现状（只审第 1 章） | 0.0886 | 67.8% | 20% | 无 | 4.1 | 46.3 |
| 批量+批末全审（简单） | 0.0921 | 66.5% | 100% | 无 | 6.5 | 70.6 |
| **批量+轻量自检+批末全审（定案）** | **0.0954** | **65.3%** | **100%** | **每3章** | **9.0** | **94.4** |
| 批量+批内检查（旧方案，废弃） | 0.0971 | 64.7% | 60% | 每3章 | 7.8 | 80.3 |
| 批量+完整门禁流程 | 0.1110 | 59.6% | 100% | 每3章 | 9.0 | 81.1 |

**落地（✅ kernel 技能已改 2026-08-09；剩余真机验证）**：
1. ✅ skills/main-core-writing-kernel.md（用户级生效）+ internal/skill/builtin 备份 + ~/.goink/skills 同步：
   - 批量流程概述加"每 3 章轻量自检 + 批末审稿覆盖全批"
   - 新增"批量质量节奏"说明段（每 3 章 read revision-pass + anti-ai-grade 自检修复，不调 run_subagent 不 set_phase；批末 run_subagent 审全部 N 章，逐章 read 自查 + edit 修复 + 字数复查）
   - review 阶段指令加批量说明（审全批，不要只审第 1 章）
2. ✅ internal/skill/builtin/main-cmd-phase-gate.md 批量流程描述同步
3. ✅ 门禁配置示例.md 头部批量建议更新为定案方案
4. 门禁配置零改动（review 阶段白名单已有 run_subagent；write 阶段轻量自检用 read/edit 白名单已有）

**完成标准**：真机跑批量验证"每 3 章轻量自检 + 批末子代理审全批"实际触发 + 成本符合预期（¥0.0954/章）。

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

### P6. 模拟器 API 简化（✅ 已完成 2026-08-09）

**现状**：`batchGatePlaysWith(chapters, checkKind, checkEvery)` 三个魔法参数（checkKind 0/1/2/3 + checkEvery），在一个函数里打补丁堆出 4 种方案，难读难维护。

**已实施**：按场景拆命名函数，需要什么直接引用：
```go
batchAsIs(5)               // 现状：攒批统一审
batchLightSelfCheck(5, 3)  // 每 3 章轻量自检（2 技能+1 修改，不跳阶段）
batchInCheck(5, 3)         // 每 3 章批内检查（阶段切换 review→子代理→write，统一 maintain）
batchFullCycle(5, 3)       // 每 3 章完整门禁流程（review+maintain 阶段链，无统一收尾）
```
内部共享 `batchCore(chapters, checkKind, checkEvery)` 一个构造器；调用点（api.go buildBatchWithRounds/buildMixedSession、diag 测试）已全部迁移；`batchGatePlays`/`batchGatePlaysWith` 已删除。
**原则**：模拟器是业务代码的建模镜，结构应跟业务概念走（方案/节奏），不是参数矩阵。

---

### P4. 技能注入去重 + 短提醒（✅ 已落地 2026-08-11，真机短提醒效果待观察）

**演进**：最初"全文去重跳过"（e15dc94）→ 用户质疑"可见 ≠ 被注意"（Lost in the Middle）→
业界验证（Anthropic skills #591 index-page / hermes-agent system-reminder / autogen 300-token 修复）
→ 改为**全文一次 + 短提醒每轮**（2a798b6 模拟器 + cdbdbc3 真机）：

1. **可见性判定**（aeed97c）：模拟器 `visibleIn(history+cur, 全文)` 全文比对，替代 injectedPhases
   记录——压缩清理历史后判定失败自动重新注入全文，杜绝"记录还在内容没了"的误判
2. **注入策略**（cdbdbc3）：首次进入阶段注入全文（学习内容常驻历史）；再次进入同阶段注入
   `BuildSkillsReminder` 短提醒（技能名 + description 要点，~300 字符 vs 全文 8K，
   紧跟请求尾部注意力最强位置）——全文保证可见，提醒保证被注意
3. 压缩联动（e15dc94 保留）：压缩成功后 `pg.ResetReads()`（compress.go）——注入的技能 system
   消息已被压缩掉，清空 readsByPhase 防误判跳过；下次 set_phase 重新注入全文
4. 批量 write 显式循环：batchCore 每章 `set_phase("write")` 显式边界（N-1 次，同阶段幂等成功）

**模拟数据**：单章 5 轮 miss 降 13.8%（506,703→436,595），命中率 97.4% 不变；
write 提醒在 5 轮历史出现 5 次（每轮一次，紧跟请求尾部）。

**真机观察点**（待验证）：(a) 每章 set_phase("write") 只注入提醒不重复全文（日志确认）；
(b) 批量中途压缩后 LLM 能补读技能（事前技能强制 + 下次 set_phase 自动注入全文）；
(c) 短提醒是否足以维持创作质量（write 技能要点被遵循，无白开水文）。

---

### P5. 门禁系统性重构（✅ 部分落地 2026-08-09，剩余待真机）

**已落地（行为开关显式化，缺省=legacy 零迁移）**：
1. PhaseConfig 显式行为开关：`inject` / `inject_dedup` / `same_phase` / `word_count_check`（*bool，nil=legacy 仅 write）/ `word_count_reset`（*bool）/ `mutating_guard`——替代 "write" 阶段名硬编码、进阶段无条件注入、同阶段幂等隐式语义
2. **门禁自动推进已删除**（agent.go 回合末尾 CheckTransitionReady 自动 set_phase 块）：与文档/评估（system-reminder-assessment"高危不做"）对齐；曾有的失败路径缺陷（SetPhase 返回值被忽略、假成功 reminder、技能记错阶段）一并消除；阶段切换必须 LLM 主动 set_phase
3. set_phase 失败不计成功（OnToolCall("set_phase", ok)）
4. 通配符技能展开（injectPhaseSkills 对 "main-tech-*" 用 ListMeta 展开为具体名再注入，auto_skill_injection 通配符半成品修复）
5. 删除死代码：FailNext 字段、SaveWordCount/LoadWordCount
6. default 配置 single/batch write 显式声明全部行为开关

**剩余（需真机验证后做）**：
- require 按阶段重置（successfulTools 加阶段维度）——批量每章 read 大纲真实强制
- wordCountOK 结构化 `{chapter, ok}`——批量多章转出正确性
- visited 重置条件改为"回到首阶段"——回退往返后不失效
- chat.go 新会话起始阶段对齐配置首阶段（init 可达）
- 配置损坏显式告警（解析失败日志 + 前端状态）
- 工具元数据化（isMutatingTool/readOnlyTools 从注册层推导）
- 门禁状态 per-session（RunOptions 值化或 map 缓存）——多会话并发串扰（当前 PhaseGate 挂 Agent 单实例）
- 配置写入前强制 ValidateGateConfig（update_phase_gate_config 无校验）
- 技能缺失从 warning 升 error（当前会导致阶段死锁）

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

> 注：**定案方案 = 批量 + 轻量自检（每3章）+ 批末全批审稿（batchLightEndReview）**：
> 成本 ¥0.0954/章、质量 9.0、性价比 94.4 全场第一、零阶段切换、查 N 修 N 一致。
> 旧方案 batchInCheck（批内阶段切换子代理）经业界对照（ainovel-cli/novel-creator/QMAI/
> webnovel-writer/xuanji-write）判定过度复杂，已废弃（见 P2）。

解读：
- **定案：批量 + 每 3 章轻量自检 + 批末全批审稿**（业界"轻量高频 + 重型低频"组合），任何批量大小适用，性价比 94.4 全场最高
- 三章一轮·批内检查（旧方案）：5 章覆盖 60%（触发 1 次），6 章覆盖 100%——已被业界组合取代
- 单章性价比最低（32.7）：质量 9 分但贵 3.1 倍
- 批量现状性价比 46.3 但质量 4.1（第 2-N 章零审稿），不可取

---

## 节奏型带病检测（2026-08-18 新增）

### P1. 门禁结果门控

**目标**：check_story_consistency 返回 [ERROR] 时，禁止 set_phase 推进。

**技术实现**：
1. 修改 `internal/agent/phase_gate.go` 的 `SetPhase` 方法
2. 在 require 检查之后，增加结果检查：
   ```go
   // 检查上次 check_story_consistency 调用结果
   if lastResult, ok := g.lastToolResults["check_story_consistency"]; ok {
       if strings.Contains(lastResult, "[ERROR]") {
           return false, "check_story_consistency 存在硬错误，禁止切换阶段"
       }
   }
   ```
3. 需要在 `OnToolCall` 中缓存工具调用结果（当前只记录调用次数）
4. 返回格式已从 emoji 改为标准 [ERROR]/[WARNING]（已完成）

**验证方法**：
- 单元测试：模拟 check_story_consistency 返回 [ERROR]，验证 set_phase 被拒绝
- 真机测试：故意制造死者复出场景，验证门禁阻断

### P2. 总纲数据库化

**目标**：将 book-outline.md 迁移到数据库表，支持 init_consistency 检查。

**数据库设计**：
```sql
-- 总纲表
CREATE TABLE outlines (
    id INTEGER PRIMARY KEY,
    novel_id INTEGER NOT NULL UNIQUE,
    core_conflict TEXT,      -- 核心矛盾
    growth_arc TEXT,         -- 成长弧线
    ending_direction TEXT,   -- 结局方向
    word_count_plan INTEGER, -- 篇幅规划（万字）
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- 大爽点表
CREATE TABLE outline_beats (
    id INTEGER PRIMARY KEY,
    novel_id INTEGER NOT NULL,
    chapter INTEGER NOT NULL,        -- 承诺章号
    description TEXT NOT NULL,       -- 描述
    beat_type TEXT DEFAULT 'shuangdian', -- shuangdian/turning/climax
    importance INTEGER DEFAULT 3,    -- 1-5
    created_at TIMESTAMP
);
```

**实施步骤**：
1. 新建 outlines + outline_beats 表（migrate.go）
2. 新建 MCP 工具 update_outline / get_outline
3. 修改 init 阶段：AI 写数据库而不是 book-outline.md
4. 修改 get_writing_context：返回 outline 数据
5. 前端：outline 编辑界面（Wails 绑定）
6. 旧小说迁移脚本（从 book-outline.md 解析导入）

### P3. init_consistency 检查

**目标**：init 阶段收尾时检查总纲/偏好/卷纲三方冲突。

**依赖**：P2（总纲数据库化）完成后实施。

**7项子检查**：
1. type_pacing：查 outline_beats 计算间距
2. pref_conflict：查 preferences + outline_beats 关键词比对
3. promise_consistency：查 outlines.growth_arc + outline_beats
4. golden_rule：查 lore(category=天道法则)
5. taboo_violation：查 preferences(category=禁忌) + outline_beats
6. means_power：查 characters.abilities + outline_beats
7. file_db_sync：查 outline_beats + story_arcs.detail_json

**实施位置**：appearance_tools.go，check_types 新增 init_consistency
