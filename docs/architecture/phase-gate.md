# 阶段门禁（Phase Gate）

## 设计动机（为什么存在门禁）

门禁不是凭空设计的，来自真实使用 AI 写小说时观察到的三个问题：

1. **AI 写完正文就不维护设定/伏笔**（偷懒跳过 maintain）→ 解决：maintain 15 项强制清单 + require 只统计成功调用
2. **AI 跳过流程直接写正文**（不听阶段指挥）→ 解决：阶段硬拦截，非白名单工具直接拒绝执行，不自动推进
3. **AI 设定前后矛盾**（凭记忆写，不查数据库）→ 解决：prepare 9 项必查 + get_writing_context 全量状态树 + 代码级闭环
**设计脉络**：本项目最初在 Chinese-novel-pipeline（用户自研 skill）里用 Python 状态机（`director.py`）验证了"按阶段推进创作"的理念，属于**建议性**状态机（脚本检测，AI 可能绕过）。Goink 将其升级为**强制性**硬拦截状态机：工具执行前检查（`phase_gate.go` + `registry.Execute` 之前拒绝）、require 成功计数防虚报、write 转出强制字数、edit 路径白名单。

**核心原则**：门禁让 AI 始终遵循 prepare → outline → write → review → maintain 稳定推进，防止它忘了更新设定、漏填设定、虚报过程、不读 skill 直接写、埋头不听指挥。宁可多调用工具，不可漏掉状态维护。

## 概述

阶段门禁是 Goink 的创作流程强制执行系统。它确保 LLM 按照 main-core-writing-kernel.md 定义的阶段顺序执行，不能跳步或跳过必要操作。

**核心特性：**
- 系统级强制：每次对话自动激活，不依赖 LLM 配合
- 硬拦截：门禁检查在工具执行之前，被拦截的工具不会执行
- 主动推进：require 满足后**必须主动调 set_phase 切换阶段**，系统不自动推进
- 跨 turn 持久化：工具调用记录保存在 session 中
- 两种模式：单章（single）和批量（batch）
- 单轮内可回退修正，走完一轮完整流程到 done 后停止，新一轮由用户重新发起，不能利用上一轮的访问历史任意跳转

## 设计哲学

**prepare 允许 edit**：一般编辑任务（改大纲、改角色设定）在 prepare 阶段自由使用，不受门禁拦截。

**require 触发收紧**：当 LLM 完成 prepare 的 9 项必查（get_writing_context、get_chapter_list、get_characters、get_timeline、get_story_arcs、get_reader_perspective、get_writing_snapshot、get_scenes、get_preferences）时，require 满足，但门禁**不会自动推进**——必须由 LLM 主动调 `set_phase("outline")` 切换，后续流程受控。

**硬拦截**：门禁检查在 `registry.Execute` 之前。被拦截的工具不会执行，LLM 收到错误结果。

**回退修正**：单轮创作内，LLM 可回退到本轮已访问过的阶段（如 write 阶段发现大纲问题，回 outline 修改）。

**循环重置**：done 是流程终点——maintain 完成 set_phase("done") 后系统停下，不再自动推进；新一轮创作由用户重新发起（新会话从首个阶段开始），不利用上一轮的访问历史任意跳转。

**字数校验（write 阶段转出）**：`set_phase("review")` 前强制检查：
- 必须调用过 `get_chapter_list`（其返回的 `word_count_ok` 写入门禁状态），未检查则阻塞
- `word_count_ok=false`（低于/高于用户设置的字数范围）则阻塞，AI 需扩写后重新检查
- **进入 write 阶段时重置字数状态**（2026-08-06 修复）：上一章检查通过的 `word_count_ok` 不会带到本章——每章必须用本章的 `get_chapter_list` 结果（旧实现用布尔值跨章，上一章达标会放行本章未达标）

## 工作流程

### 单章模式（mode: single）

```
每次对话开始 → 自动进入 prepare 阶段
  ↓ prepare 允许 edit（一般编辑自由用）
  ↓ 调 9 项必查（get_writing_context 等）
  ↓ require 满足后，LLM 主动调 set_phase("outline")
outline → 写大纲（require: edit）→ set_phase("write")
write → 写正文（require: edit + get_chapter_list）→ 字数校验 → set_phase("review")
review → 审读（require: run_subagent）→ 结果门控（不通过则阻止推进）→ set_phase("maintain")
maintain → 状态维护（require: 14 项清单）→ set_phase("done")
  ↓ done 是终点：创作完成，系统停下。新一轮由用户重新发起
```

### 批量模式（mode: batch）

```
init → prepare → outline（一次出 N 章大纲）→ [write → 迷你维护] × N 章 → review → maintain → done
```

与单章差异：outline 一次产出全部 N 章大纲；write 循环 N 章正文，每章写后紧跟迷你维护（只写不查，6 个状态写入工具，状态实时结算）；review / maintain 整批统一一次；整批末尾 maintain 收尾后 set_phase("done") 结束本轮创作。

## 工具白名单

> 下表为简化示意。**精确白名单以数据库配置为准**（出厂时自动写入默认配置，也可在设置面板修改 phase_gate_config，或参考 `门禁配置示例.md`）。

| 阶段 | 允许的工具（简化） | 阻止的工具（简化） |
|------|-------------------|-------------------|
| init | auto_skill_injection, edit(book-outline.md, goink.md), create_*, get_*, set_phase | update_*, delete_*, run_subagent |
| prepare | get_*, read, auto_skill_injection, search_story_memory, web_search, web_fetch, set_phase | edit, update_*, create_*, delete_*, run_subagent |
| outline | read, auto_skill_injection, edit(outlines/*, goink.md, book-outline.md), get_*, set_phase | update_*, create_*, delete_*, run_subagent |
| write | read, auto_skill_injection, edit(chapters/*), create_item_occurrence, update_writing_snapshot, search_story_memory, get_*, set_phase | update_*, create_*（除迷你维护 6 个）, delete_*, run_subagent |
| review | read, auto_skill_injection, edit(chapters/*), run_subagent, get_*, set_phase | update_*, create_*, delete_* |
| maintain | read, auto_skill_injection, edit(goink.md, chapters/*, outlines/*), update_*, create_*, delete_*, get_*, set_phase | run_subagent |

> **注意**：get_lore、get_items、get_scenes、get_stats、get_writing_snapshot 属于 get_*，在全部阶段可用。
> create_lore、create_item、create_scene、update_lore、update_item、update_scene、delete_lore、delete_item、delete_scene、update_writing_snapshot 属于 create_*/update_*/delete_*，仅在 init 和 maintain 阶段可用（即新建/修改设定的操作集中在开书与维护阶段）。
> set_phase 在所有阶段始终可用（它是阶段切换的唯一入口）。

## require 完成条件

| 阶段 | require | 说明 |
|------|---------|------|
| prepare | get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences | 9 项必查（全量状态必须加载） |
| outline | edit | 大纲必须写入文件 |
| write | edit, get_chapter_list | 正文必须写入文件 + 字数校验前置检查 |
| review | run_subagent | Review agent 必须启动 |
| maintain | edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations | 13 项强制清单（设定/伏笔/关系全量维护） |
> require 只统计**成功调用**（`phase_gate.go` `successfulTools`）——失败不算，防止"调了但没做成"蒙混过关。

## 跨 Turn 持久化

门禁状态保存在 `sessions` 表：
- `current_phase`：当前阶段名
- `called_tools`：已调用工具的 JSON 计数（含 visited 访问记录，新格式 `{"tools":{...},"visited":[...]}`，兼容旧格式）

每次 `agent.Run()` 结束时自动保存，下次对话时自动恢复。visited 随状态持久化，保证断点续作后仍可回退到已访问过的阶段。

## 配置格式

```markdown
<!-- phase-gate-config
mode: single
phase: prepare
tools: get_chapter_list, read, edit, get_characters, get_timeline
require: get_chapter_list, get_characters, get_timeline
next: outline
-->
```

| 字段 | 必填 | 说明 |
|------|------|------|
| mode | 否 | "single" 或 "batch"，空=两种模式都适用 |
| phase | 是 | 阶段名称 |
| tools | 是 | 该阶段允许使用的工具列表（白名单，未列出的工具被硬拦截） |
| require | 是 | 必须调用过（且成功）的工具列表，全部满足后才能切换阶段 |
| auto_skill_injection | 否 | 该阶段必读技能名列表（如 `main-tech-show-dont-tell, main-tech-anti-ai-writing`）。**系统在 set_phase 进入该阶段时自动注入**这些技能为 system 消息，模型无需手动调 auto_skill_injection 工具。支持 `*` 通配符（如 `main-tech-*`） |
| next | 是 | require 满足后可进入的下一阶段 |
| fail_next | 否 | require 不满足时的回退阶段（当前出厂配置未使用，代码支持） |
| edit_paths | 否 | edit 工具的路径范围（如 "outlines/*, goink.md"，"*"=不限制；逗号分隔） |
| loop | 否 | "true" 表示 batch 模式下 write 可回退到上一阶段 outline（连续多章写作时改大纲） |

> 批量模式循环：默认配置中 batch 的 write 阶段带 `loop: true`，配合 visited 回退机制实现「write 写多章时可回 outline 修大纲」。

## 配置设计指南（怎么设计一套门禁）

### 第一步：定阶段链

阶段 = 创作流程的步骤。默认六阶段流程，一般不需要增删：

```
init（开书）→ prepare（全量状态）→ outline（大纲）→ write（正文）→ review（审稿）→ maintain（维护）→ done（终点）
```

每阶段一个配置块；**第一个阶段是流程起点**（新会话从它开始），最后一个阶段 maintain 的 next 指向 done（终点，流程走完停止，不再循环回起点）。

### 第二步：定每阶段的 tools（放什么工具）

按工具角色分组，分配原则：

| 工具角色 | 工具名 | 放哪些阶段 |
|---------|--------|-----------|
| 技能加载 | auto_skill_injection | 所有阶段（该阶段有必读技能才需要） |
| 文件读取 | read | 需要读正文/大纲/文件的阶段（outline/write/review/maintain） |
| 查询 | get_*、search_*、check_story_consistency、get_stats | 所有阶段（随时查状态，宁可多查不可漏） |
| 网络 | web_search、web_fetch | 需要查资料/考据的阶段（prepare） |
| 文件写入 | edit | 配合 edit_paths 限制路径：init 写总纲（book-outline.md, goink.md）、outline 写大纲（outlines/*）、write 写正文（chapters/*）、review 修正文（chapters/*）、maintain 写指纹（goink.md） |
| 创建 | create_* | 只在 init（开书建世界观/角色）和 maintain（补录缺失条目） |
| 更新 | update_* | 只在 maintain（状态维护）；batch write 额外放迷你维护 6 个（create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot） |
| 删除 | delete_* | 只在 maintain |
| 审稿 | run_subagent | 只在 review |

**工具清单以 `internal/mcp_tools/` 各文件注册名为准**（get_characters、create_item_occurrence 等 69 个）。

### 第三步：定每阶段的 require（必须完成的动作）

原则：require = "该阶段不完成就产生创作事故的动作"。

| 阶段 | require | 为什么 |
|------|---------|--------|
| init | 7 个查询（characters/locations/story_arcs/lore/items/timeline/preferences） | 开书前必须确认世界观现状 |
| prepare | 9 项必查（writing_context/chapter_list/characters/timeline/story_arcs/reader_perspective/writing_snapshot/scenes/preferences） | 全量状态必须加载才能动笔 |
| outline | edit | 大纲必须写入文件 |
| write | edit、get_chapter_list、read、auto_skill_injection | 正文必须写入 + 字数校验 + 读大纲 + 读技能 |
| review | run_subagent | 审稿必须启动 |
| maintain | 13 项（edit 指纹 + 3 更新 + 2 搜索 + 7 查询） | 设定/伏笔/关系全量维护 |

注意：write 阶段转出时**自动强制字数检查**（get_chapter_list 的 word_count_ok），无需配置。

> **require 按阶段计数**：工具调用统计（calledTools/successfulTools）在每次阶段切换时重置——require 的语义是"本阶段内必须调用"，上一阶段调过的工具（如 write 的 edit）不会预填下一阶段的 require（2026-08-14 修复：跨阶段累计曾导致 maintain 的 goink.md 指纹 edit 被轮末自动推进提前推到 done 白名单冻结，形成死循环）。同阶段 set_phase（批量章边界）不重置。

### 结果门控（Result Gate）

require 检查"工具是否调用过"，结果门控检查"工具返回了什么"。两者在 `SetPhase` 内串行执行，require 通过后才检查结果。

**已实现的结果门控：**

| 工具 | 检查条件 | 行为 | 来源 |
|------|---------|------|------|
| check_story_consistency | 返回内容含 `[ERROR]` | 阻止切换阶段 | 2026-08 初始实现 |
| run_subagent | 审稿报告结论为"不通过"（总分<7.0） | 阻止推进到下一阶段 | 2026-08-24 review verdict gate |

**设计原则：**
- check_story_consistency 的 `[ERROR]` = 硬错误（伏笔超期/死者复出/台账越界），必须修复
- run_subagent 的"不通过" = 章节有根本问题（总分<7.0），必须重审
- "需修改"（7.0-8.9）= 小问题，LLM 从报告中自行修复，**不阻止推进**——避免每次 revise 都触发昂贵的子代理全量 fork 重审
- memory 子代理报告无 verdict 模式，不误拦

**实现位置：** `internal/agent/phase_gate.go` `checkResultGateMet()`；`run_subagent` 的报告文本通过 `Data["content"]` 暴露（`internal/mcp_tools/subagent_tools.go`），由 `OnToolCall` 存入 `lastToolResults["run_subagent"]`。

### 第四步：定每阶段的 auto_skill_injection（必读技能）

原则：该阶段核心方法论，对照 kernel 阶段技能表（`skills/main-core-writing-kernel.md` 的"阶段技能表"）。**这些技能由系统在 set_phase 进入该阶段时自动注入为 system 消息**，模型无需手动调用 auto_skill_injection 工具。

| 阶段 | auto_skill_injection |
|------|---------------|
| init | main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system |
| prepare | main-tech-common-sense-logic |
| outline | main-tech-chapter-hook-enhanced, main-tech-chapter-title-design |
| write | main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density, main-tech-word-count-calibration |
| review | 空（sub-tech-review-standards 由系统自动注入子代理，主代理不用读） |
| maintain | main-tech-anti-repetition, main-tech-foreshadow-cycle, main-tech-data-hygiene |

按需技能（情景类）不进 auto_skill_injection，由 kernel 措辞引导模型按需 read。

### auto_skill_injection 注入策略（2026-08-11 更新）

系统在 set_phase 进入阶段时注入技能，采用**全文一次 + 短提醒每轮**策略（`internal/agent/agent.go` `injectPhaseSkills`）：

1. **首次进入阶段**：注入技能**全文**（`BuildSkillsContent`，学习内容，常驻历史始终可见）
2. **再次进入同阶段**（历史中已有相同全文，`visibleIn` 全文比对判定）：不再重复全文，
   注入**短提醒**（`BuildSkillsReminder`：技能名 + description 要点，~300 字符 vs 全文 ~8K，
   紧跟请求尾部注意力最强位置）——全文保证可见，短提醒保证被注意
3. **压缩重建后**：技能消息被清出历史，可见性判定失败 → 自动恢复注入全文

> 背景：全文重复注入 = 每阶段切换固定 miss 全文（单章模式占 miss 构成 26%，模拟验证 miss 降
> 13.8%）；但"全文在历史里"≠"模型注意到"（Lost in the Middle——单章 5 轮时技能全文在历史
> 24.6% 位置，上下文增长后注意力衰减）。短提醒是业界标准解法（Anthropic skills #591 index-page
> 模式 / hermes-agent system-reminder / autogen 300-token 修复），模拟器与真机同标准
> （`skillDedupSim` 开关 + `visibleIn` 全文比对）。

### 常见设计错误

| 错误 | 后果 | 修正 |
|------|------|------|
| require 引用了 tools 里没有的工具 | set_phase 永远被拦，流程卡死 | require 的工具必须同时在 tools 里 |
| next 指向不存在的阶段名 | 切换时提示"未知阶段" | next 必须与某块配置的 phase 一致 |
| 两个 mode 都空的同名阶段 | 解析时只生效第一个（findPhase 取首个） | 同名阶段必须写 mode: single / mode: batch 区分 |
| edit_paths 不含 require 需要的路径 | edit 被拦，require 永不满足 | outline 的 edit_paths 必须含 outlines/* |
| 首阶段不是 init/prepare | 新会话从错误起点开始 | 第一个配置块就是流程起点 |
| tools 忘放 set_phase | set_phase 永远放行，不写也没事 | 可不写，但建议写上可读性更好 |

## 故障排查

| 现象 | 原因 | 解决 |
|------|------|------|
| 工具被拦截 | 当前阶段不允许该工具 | 完成当前阶段 require 后，主动调 `set_phase` 切换到目标阶段 |
| 阶段不推进 | require 未满足，或未调 set_phase | 先调用 require 列表中的工具，再主动 `set_phase` 切换 |
| 切换被拒 | 目标阶段不在 next 链，也不在本轮 visited | 只能推进到 next 或回退到本轮已访问过的阶段 |
| 完成后不停想再创作（旧 bug） | visited 永久累积，可任意跳转 | 已修复：流程到 done 终点即停止，新一轮由用户重新发起；done 阶段仅白名单只读工具 |
| 批量模式不循环 | write 阶段没有 `loop: true` | 默认配置已带；自定义配置需在 batch 的 write 阶段加 `loop: true` |
| 门禁未激活 | phase_gate_config 为空 | 出厂首次启动自动 seed 默认配置（single + batch）；已配置过则不会被覆盖 |

## 设置开关

阶段门禁可在桌面端「设置」中开启/关闭，也可点窗口标题栏的盾牌图标（Shield/ShieldOff）快捷切换：

- 开启时：AI 严格按照阶段顺序执行，工具调用受白名单限制
- 关闭时：AI 可自由调用所有工具，无阶段限制

默认开启。开关状态存 `app_config.phase_gate_enabled`，标题栏按钮与设置页共用同一状态。

> 本文档内容已冗余到内置 skill `main-cmd-phase-gate`（manual 模式，`/` 触发）——docs 可能被清理，内置 skill 随 exe 分发，防止门禁说明丢失。

## API 访问

通过 HTTP API 进行对话时，阶段门禁同样生效：

- `POST /api/chat` 发送消息后，Agent 按当前 session 的阶段执行
- 门禁状态持久化在 `sessions` 表的 `current_phase` 字段
- 新会话自动从配置的第一个阶段开始（默认 init），之后由 LLM 主动 set_phase 推进；已有小说的会话模型会快速走过 init（只查不建），切到 prepare


## 示例门禁配置

完整配置见 [`门禁配置示例.md`](../../门禁配置示例.md)。**首次启动时系统自动将默认配置（与示例一致）写入数据库并启用，无需手动配置**；用户可在设置面板修改或清空（清空后下次启动恢复默认）。

> 配置字段名为 `next`（`internal/agent/phase_gate.go` 只解析 `next` / `fail_next`）。
