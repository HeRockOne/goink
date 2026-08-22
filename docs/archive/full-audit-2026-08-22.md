# Goink 全量审计报告 (2026-08-22)

> 审计指令：「审计所有系统提示词，所有mcp工具，所有门禁系统，所有skill，读取真实代码文件。创作质量第一，省token第二。」
>
> 6 个子代理并行审计：系统提示词 / MCP工具 / 门禁系统 / skills 三批（核心+技术+类型）。

## 审计规模

| 范围 | 文件数 | 代码量 |
|------|--------|--------|
| 系统提示词 | identity.go(285行) + novel_state.go(65行) + skill_catalog.go(120行) + agent.go(~1200行) | ~1670行 |
| MCP工具 | 28个 .go 文件 | 3346行 |
| 门禁系统 | phase_gate.go(1032行) + safety.go(100行) + compress.go(448行) + tokens.go(261行) + default_phase_gate_config.go(160行) | ~2001行 |
| 内置Skill | 44个 .md 文件 | - |
| 用户级Skill | 6个 .md 文件 | - |
| **合计** | ~7000行 Go + 50个 skill md | |

---

## 一、系统提示词审计

### Bug

- **`internal/agentcfg/identity.go:231,242`** — reviewAgentSystem1 步骤编号重复，两个「5.」应为5和6

### 设计观察

- `novel_state.go:54` — `maxGoinkChars = 1000` 硬编码，缓存协议依赖固定字节长度，改动需同步修改缓存前缀计算
- 组装链路：AgentIdentity -> AlwaysSkills -> SkillCatalog -> Slash Inject -> User Message -> NovelState（动态尾部）
- 缓存协议设计精巧：稳定前缀 + 动态尾部，命中率 89-93%

### 安全

- Prompt注入 / 路径遍历 / SQL注入均低风险

---

## 二、MCP工具审计 (28个文件, 3346行)

### P0 -- 安全漏洞

**preference 删除越权**：`internal/mcp_tools/delete_tools.go:440`

`is_global = true` 的全局偏好可在任何 novel 上下文被删除，无 scope 保护。任何会话都能删全局配置。

修复建议：在删除全局偏好时校验调用上下文是否具备全局权限，或限制全局偏好只能在设置页操作。

### P1 -- RawArgs PATCH 模式 (6处)

`json.Unmarshal(tc.RawArgs, &entity)` 直接覆盖全部 JSON 标签字段，零值不可区分（无法区分「未传」和「传了零值」）。

涉及位置：
- `timeline_tools.go:253`
- `storyarc_tools.go:288, 429`
- `location_tools.go:368, 560`
- `reader_perspective_tools.go:307`
- `novel_tools.go:301`

正确范式：`volume_tools.go:150-167` 的逐字段检查模式。

### P1 -- 删除审批不一致

`delete_tools.go:85-92` -- scene / lore / item 删除跳过 approval flow，而 character / location / timeline 走 approval。删除破坏性操作的审批标准不统一。

### P2 -- 6处 N+1 查询

| 位置 | 问题 |
|------|------|
| `writing_context.go:146-149` | 每个 scene 查 location / arc |
| `writing_context.go:200-213` | 每个 character 查 location / item |
| `writing_context.go:233-235` | 每个 arc 两次 Count |
| `character_tools.go:67,77` | countItemsForChar |
| `appearance_tools.go:101-103` | 加载全部 scenes |
| `appearance_tools.go:462-492` | per-item 后续章节 |

### P2 -- delete_volume 无审批无级联

`volume_tools.go:195-202` -- 删卷后章节变孤儿，无级联清理，无审批。

### P3 -- 变量遮蔽

`appearance_tools.go:762` -- `db` range var 遮蔽外层 `db *gorm.DB`。

### 创作质量保护（确认到位）

- 死角角色守卫：character / snapshot / consistency 三层
- 终态守卫：item destroyed 不可恢复，character dead 不可复活
- 上下文膨胀防护：reader_perspective 限 60 条，timeline 10 章窗口
- 所有 28 个工具 description 含方法论指导，无一处被精简

---

## 三、门禁系统审计

### Bug 需修复

1. **`internal/agent/phase_gate.go:940`** -- `SaveState()` 中 `json.Marshal(data)` 错误被 `_` 吞掉，序列化失败静默丢失门禁状态。门禁状态可能不持久化，重启后丢失。

2. **`internal/agent/safety.go:12-23`** -- `readOnlyTools` 缺少 `get_scenes` / `get_item_occurrences` / `get_writing_snapshot` / `get_lore` / `get_items` / `get_preferences`，死循环检测可能漏检这些只读工具。

3. **`internal/agent/safety.go:93-96`** -- `toolPattern` 截断到 100 字符，含长中文的 edit 调用被截断后 pattern 匹配失真。

### 设计债务

- PhaseGate 所有方法无 `sync.Mutex`（当前 Run 局部变量安全，但无并发测试）
- `tokens.go:52+83` -- updateUsage 每次读 session 表 2 次，可合并为 1 次

### 文档不一致

- `docs/architecture/phase-gate.md:92` 写 maintain require 13 项，代码实际 14 项（多了 `check_story_consistency`）
- `docs/architecture/phase-gate.md:187` 写 auto_skill_injection 2 项，代码实际 3 项（多了 `main-tech-data-hygiene`）

### 创作质量风险

- auto_skill_injection 加载失败 -> 创作工具全锁死（不可逆死锁）
- write 阶段字数校验 false -> 无法推进

### 确认无问题

- Config 完整性
- edit_paths 覆盖 require
- batch write 有 loop:true
- compress.go 压缩逻辑正确 + phaseSkills 补回
- CheckToolAllowed 硬拦截
- SetPhase require 检查
- visited 重置防跨周期跳转

---

## 四、Skills 审计 -- 核心技能

### 同步状态

4 对 builtin / user-level 文件全部字节一致（MD5 校验通过）。

### P1 问题

1. 阶段技能表 (kernel L239) 引用了 `main-tech-emotional-arc` 和 `main-tech-opening-chapter` 用于 outline 阶段，但阶段指令 (kernel L107-110) 未引用 -- 指令与技能表不同步
2. maintain 阶段指令 (kernel L163) 未显式引用其 3 个必读技能（`anti-repetition`, `foreshadow-cycle`, `data-hygiene`）
3. write 阶段步骤编号 2->4 跳跃 (kernel L138-139)

### P2 问题

- lore category "9选" 实际只列 8 个 (init-phase L108)
- 金手指 item_type 映射规则缺失
- review #23 执行主体不明确

### 硬约束风险（设计观察，非bug）

- 段落 35-55 x 60-80 字过刚性
- "一章一事" 限制群像章
- 伏笔 +20 章窗口对长篇偏窄

---

## 五、Skills 审计 -- 技术技能 (36个文件)

### Critical (4个)

1. **`main-tech-info-density.md:12`** -- "每300字必须有信息增量" 与 emotional-arc 留白、pacing-control 慢段落矛盾。信息密度铁律会压制刻意留白的叙事节奏。

2. **`main-tech-word-count-calibration.md:12`** -- "35-55段 x 60-80字" 乘积 2100-4400 vs 目标 2500-4000 不自洽。段数x段字数的下限低于目标下限，上限高于目标上限。

3. **`main-tech-anti-ai-writing.md:24`** -- 铁律9"翻案腔"禁用列表过宽，"与其说...不如说..." 在推理文中是合法叙事手法，一刀切禁用损害推理类型创作。

4. **`main-tech-pov-purity.md` vs `main-tech-book-outline.md`** -- pov-purity 禁止"他不知道的是..."，但 book-outline 定义了 E-多线章（需要切换视角），例外条款不够醒目，AI 容易在多线章误判 POV 违规。

### Moderate (8个)

| Skill | 问题 |
|-------|------|
| foreshadow | 冷却期与 shuangdian 每章钩子冲突 |
| emotion-injection | 独白上限 200 字对悬疑/言情偏低 |
| chapter-title-design | 平台规则可能过时 |
| golden-three-chapters | 第3章同时要求小闭环+爽点+大钩子过重 |
| brainstorm-composer | 缺 Goink 工具链集成 |
| dialogue-subtext | "50%潜台词密度"无量化标准 |
| climax-scene | 铺垫2倍规则与A型章3000-3500字冲突 |
| sub-tech-anti-ai-grade | T1"带着..."禁用过宽 |

### Low (8个)

- show-dont-tell 示例重复
- golden-finger 300字vs1000字偏差
- anti-repetition 指纹格式未说明 edit 行为
- revision-pass 两套检查体系重叠
- 4个cmd技能内容过简
- book-outline E-多线章触发条件不明
- pacing-control 3:1比缺适用边界
- genre-templates 缺纯爱/百合类型

---

## 六、Skills 审计 -- 类型技能 (5个 + 2个技术对比)

### 结论：整体质量 A（优秀），0 Critical，5 Moderate，0 Low

| 类型 | 问题 | 严重度 |
|------|------|--------|
| 末世生存 | 禁忌"进化体系不明"描述有歧义（应指不一致而非太死板） | Moderate |
| 末世生存 | 缺"社会逻辑自洽性"核心禁忌 | Moderate |
| 历史穿越 | 开篇300字偏紧 | Moderate |
| 历史穿越 | 禁忌示例（唐宋八大家）不典型 | Moderate |
| 玄幻仙侠 | "120章一个大阶段"参考值偏高（斗破实际60-80章） | Moderate |

同步状态：`chapter-title-design` 和 `data-hygiene` 用户级与内置版完全一致。

---

## 总览

| 严重度 | 类别 | 数量 | 影响 |
|--------|------|------|------|
| P0 | 安全漏洞 | 1 | 全局偏好可被越权删除 |
| P1 | 代码Bug | 2 | 门禁状态可能丢失；提示词步骤编号错误 |
| P1 | 代码模式 | 6处 | RawArgs覆盖+审批不一致 |
| P2 | 性能 | 6处 | N+1查询 |
| P2 | 级联 | 1 | 删卷变孤儿章节 |
| P3 | 变量遮蔽 | 1 | db range var遮蔽 |
| Critical | Skill矛盾 | 4 | 信息密度vs留白/字数不自洽/翻案腔过宽/POV多线冲突 |
| Moderate | Skill | 13 | 技术8+类型5 |
| Low | Skill | 8 | 示例重复/规则缺量化等 |

### 创作质量红线

全部守住：28个工具 description 零删减、44个 skill 创作规则零删减、所有方法论描述完整。死角角色守卫 / 终态守卫 / 上下文膨胀防护三层保护到位。

### 修复优先级建议

1. **立即修**（P0 安全）：`delete_tools.go:440` 加 scope 校验
2. **立即修**（P1 Bug）：`phase_gate.go:940` 错误处理；`identity.go:231,242` 编号；`safety.go` 补全 readOnlyTools + 截断长度
3. **尽快修**（P1 代码模式）：6处 RawArgs 改为逐字段检查（参照 volume_tools.go 范式）；统一删除审批流程
4. **择机修**（Critical Skill 矛盾）：info-density 加留白例外条款；word-count-calibration 修正乘积自洽；anti-ai-writing 翻案腔加推理类型例外；pov-purity 加醒目的多线章例外
5. **可选优化**（P2/P3）：N+1 查询优化；删卷级联；变量遮蔽重命名
