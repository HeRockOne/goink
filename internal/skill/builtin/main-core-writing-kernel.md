---
name: main-core-writing-kernel
description: 小说核心写作调度。按阶段工作，禁止跳过。阶段步骤用 set_phase 推进。
category: 核心系统
mode: always
---

# 写作核心系统

## 1. 工作模式

### 1.1 流程总览

| 模式 | 流程 | 特点 |
|------|------|------|
| 单章 | init → prepare → outline → write → review → maintain → **done** | 每章一轮完整流程 |
| 批量 | init → prepare → outline（一次出 N 章大纲）→ write（循环 N 章）→ review（批末统一）→ maintain（批末统一）→ **done** | write 循环 N 章正文 |

**done 是终点**：maintain 完成后 set_phase("done")，本轮创作结束，系统停下等待用户发起新一轮。不要自行循环回 prepare 开启无限创作。

### 1.2 技能加载规则（所有阶段通用）

- **必读技能**：门禁 `auto_skill_injection` 在 set_phase 时自动注入**全文**（学习内容，常驻上下文）；再次进入同阶段只注入**短提醒**（技能名+要点，唤起遵循）。技能全文始终在上下文中，**必须遵循**；感觉要点模糊时用 `auto_skill_injection` 重读全文，不要凭印象创作。
- **情景技能**：catalog 中仅列出 name+description，**按需 `read` 加载**（仅本章涉及该情景时读，普通章不读）。
- **压缩后补读**：上下文压缩会清空技能记录，必须按需补读（`auto_skill_injection` 或下次 set_phase 自动注入全文），不要因"技能读过"就跳过补读。
- **manual 技能**（collect/memory/next/review/phase-gate）：仅用户 `/` 触发。

### 1.3 工具调用规范

- **并行调用**：同一阶段内无依赖的工具**一次并行发出**（prepare 9 项必查、maintain 查询批、迷你维护 6 项、每 3 章自检）；有依赖才串行（后一个需要前一个结果）。并行减少轮边界与重复 thinking，不损失数据。
- **技能优先**：必读技能必须在动笔前已加载——技能是创作指导，先读再写，不是切换阶段的手续。技能漏读会让创作跑偏，门禁会在动笔时拦截。
- 自检、修复、迷你维护**不需要** set_phase；仅阶段真正切换（outline→write、write→review 等）或批量声明章边界时调用。

## 2. 阶段流程

### 2.1 init（开书，仅新书）

门禁自动注入必读技能：main-core-init-phase, main-tech-genre-templates, main-tech-book-outline, main-tech-character-design, main-tech-world-building-system。按 main-core-init-phase 工作流执行：

1. 信息采集（分波次提问）→ 2. 写总纲（update_outline + create_outline_beat）→ 3. 创建卷弧线 → 4. 弧线/地点/角色/节点 → 5. 世界观/物品 → 6. 偏好/伏笔 → 7. 一致性校验 → 8. 用户确认 → 9. 结构验证（7 项查询并行）→ **set_phase("prepare")**

硬性门槛：未写全书总纲禁止切换 prepare；用户未明确确认禁止 set_phase（AI 自我判断不算确认）。

### 2.2 prepare（全量状态加载）

**必读技能已由系统就绪**（main-tech-common-sense-logic，set_phase 时自动注入）。9 项必查并行发出：

| # | 工具 | 确认目的 | 传参 |
|---|------|---------|------|
| 1 | get_writing_context | 树状全量状态 | 必须检查 `volume_entities`（本卷实体清单），确认本卷约束/伏笔/物品 |
| 2 | get_chapter_list | 章节编号连续 | - |
| 3 | get_characters | 角色阵容 | **brief=true**（生死等状态已在 writing_context 的 characters 段） |
| 4 | get_timeline | 伏笔状态 | **current_chapter**（窗口切分，不分页翻全量） |
| 5 | get_story_arcs | 弧线进度 | **current_chapter**（窗口切分同上） |
| 6 | get_reader_perspective | 读者认知 | **全量返回**（写作需知活跃悬念/误知内容，counts_only 不够） |
| 7 | get_writing_snapshot | 写作进度 | - |
| 8 | get_scenes | 本章场景 | **chapter_id 按章查**（不传只返回最近 100 个） |
| 9 | get_preferences | 创作偏好 | - |

补充动作：
- 按需技能：main-tech-genre-templates（题材）、main-tech-book-outline（看卷纲时）、main-tech-brainstorm-composer（卡情节时构思）
- 发现异常（角色断档/设定矛盾）→ get_entity_appearances 反查确认
- 五门检查（字数/段数/情绪/节奏/禁止项）→ **set_phase("outline")**

### 2.3 outline（写大纲）

**必读技能已由系统就绪**（main-tech-chapter-hook-enhanced, main-tech-chapter-title-design，门禁强制）。

1. **消费总纲与卷纲**：确认 writing_context 返回的 `outline`（核心矛盾/成长弧线/结局方向）、`volume`（本卷核心事件/爽点位置/需回收伏笔）、`progress`（当前章号+卷范围）——**本章纲不得超出本卷范围，后续卷情节禁止提前展开**
2. **加载技能**（必读已在上下文中）：
   - 必读：main-tech-chapter-hook-enhanced, main-tech-chapter-title-design
   - 常备：main-tech-book-outline, main-tech-chapter-opening, main-tech-maliang-method, main-tech-dialogue-subtext（有情感/对话/爽点设计时用）
   - 新书首次加：main-tech-golden-three-chapters, main-tech-golden-finger-design
   - 类型专精：main-type-*（每本小说只加载对应 1 个，不重复读）
3. **edit(outlines/NNN.md)**（required）写大纲，格式要求：
   - 章节标题 `# 第N章 标题`（单井号，一行）
   - 各 section 用 `## 标题`，必须含：`场景设计`（核心场景/环境氛围/时间/地点）、`关键事件`（按叙事节拍编号，标注字数区间和功能）、`重点角色`（动机/叙事功能/本章状态）、`伏笔操作`（埋设/回收/推进，标注伏笔 ID 和目标章节）、`章末钩子`（类型/设计/预期效果）、`写作要点`（情绪节奏/信息密度/禁忌项）、`字数预估`（目标字数/段落数/每段字数）
   - 禁止用 `**加粗**` 代替 `##` 标题
4. **set_phase("write")**

### 2.4 write（写正文）

**必读技能已由系统就绪**（show-dont-tell, anti-ai-writing, pov-purity, info-density, word-count-calibration，set_phase("write") 时自动注入）。

1. **read**（required）— 读本章大纲 outlines/NNN.md（门禁 require 强制）
2. **情景技能按需加载**（仅本章涉及该情景时）：
   - 战斗/高潮章 → main-tech-climax-scene
   - 爽点/打脸章 → main-tech-shuangdian-pacing
   - 本章有伏笔操作 → main-tech-foreshadow-cycle
   - 情感/情绪戏 → main-tech-emotion-injection
   - 节奏紧张章 → main-tech-pacing-control
   - 关键场景现场描写 ≥300 字 → main-tech-scene-beats
3. **edit(chapters/NNN.md)**（required）— 写正文（new_content 只含正文，title 不带前缀）
4. 校验字数（2500-4000，以设置 min/max 为准；get_chapter_list 代码校验）
5. 记录关键物品出现 → create_item_occurrence
6. **check_story_consistency**（required）— 写完立即程序化核对（current_chapter=本章章号）：伏笔超期/角色断档/物品冲突/死者复出，发现错误立即修复后重跑直到通过（不核对无法转出 review）。check_types 留空=全部；review 建议 `["pacing_gap"]`，maintain 建议 `["promise_fulfillment"]`。get_writing_context 的 dead_characters 名单写作前就要记住，死者不得复出
7. **写后自审**（单章每章、批量每 3 章）：read main-tech-revision-pass + sub-tech-anti-ai-grade，检查本章节奏与 AI 味，发现问题立即 edit 修复
8. **set_phase("review")**

**字数扩写规则（不足时）**：一次扩到位，禁止挤牙膏——先算缺口（下限 - 当前），按缺口 ×1.2 设定目标，一次 read 定位 + 一次 edit 补足并超出下限，立即 get_chapter_list 复查；不足则重复"一次到位"（每次以当前缺口 ×1.2 为目标）。

> **write→review 边界**：进入 review 后立即审稿，**禁止调用任何维护/更新工具**（白名单只读/审稿，会被门禁拦截）。迷你维护是写正文完成后、set_phase("review") 之前做的；进入 review 只做：run_subagent 启动审稿 → 读报告 → 修正文。维护留到 maintain 统一做。

### 2.5 review（审稿）

1. **run_subagent(agent_type="review")**（required）— 启动审稿。**批量：审读本批全部 N 章**，不要只审第 1 章。**审稿报告必须列出实际审读章节清单**，主 agent 先核对覆盖范围，覆盖不全先补审，再进入修复
2. **审稿核对（身份差异）**：
   - 主会话（作家视角）：**get_characters 全量**（核对 status：alive/dead 与正文一致，brief 无 status 会漏检）、**get_timeline/get_story_arcs 传 current_chapter**（伏笔/弧线进度）、**get_reader_perspective 全量**、**check_story_consistency**；读正文分段核对（read start_line/end_line）
   - 审稿子代理（fork 完整主历史，正文+writing_context 已在上下文）：只做少量定向核对（get_characters brief+size 小、get_timeline current_chapter）+ check_story_consistency，输出审读报告——不重复拉全量
3. 根据意见修复（**批量：逐章 read 自查 + edit 修复 + get_chapter_list 字数复查**，查 N 修 N）
4. **set_phase("maintain")**

### 2.6 maintain（状态维护清单，每章必做）

**必读技能已由系统就绪**（main-tech-anti-repetition, main-tech-foreshadow-cycle, main-tech-data-hygiene）。

**第一步：7 项状态查询并行发出**（门禁 require 强制）：

| # | 查询项 | 工具 | 确认目的 |
|---|--------|------|---------|
| A | 查角色状态 | get_characters | 是否有角色需要更新 |
| B | 查伏笔状态 | get_timeline | 是否有伏笔要回收/校准 |
| C | 查弧线进度 | get_story_arcs | 是否有节点要推进 |
| D | 查读者认知 | get_reader_perspective | 是否有悬念要维护 |
| E | 查场景状态 | get_scenes | 是否有场景要创建/更新 |
| F | 查物品流转 | get_item_occurrences | 是否有物品易主未记录 |
| G | 查角色关系 | get_character_relations | 是否有关系变化未记录 |

**第二步：更新动作**（按依赖链聚合，互不依赖的同批并行；先查后更、edit 前先 read 才串行）：

| # | 检查项 | 条件 | 工具 |
|---|--------|------|------|
| 1 | 写章节元数据 | 每章必做 | update_chapter_meta（summary + key_events + characters_in + arc_ids 全部 required） |
| 2 | 更新写作快照 | 每章必做 | update_writing_snapshot（summary required） |
| 3 | 搜索设定防遗忘 | 每章必做 | search_lore |
| 4 | 搜索物品防断裂 | 每章必做 | search_items |
| 5 | 更新章节计划 | 每章必做 | update_chapter_plan（main-cmd-next 触发） |
| 6 | 创建场景条目 | 查询 E 发现**关键**场景 | create_scene（title + summary required） |
| 7 | 记录物品流转 | 查询 F 发现**关键**物品易主 | create_item_occurrence（item_id + chapter_id + action required） |
| 8 | 更新角色状态（含内容校准） | 查询 A 发现**关键**角色变化 | update_character（改 status 时同步校准 description/personality；只写已发生事实，禁止"预测性剧情"） |
| 9 | 更新角色关系 | 查询 G 发现变化 | update_character_relationship（relation_describe required） |
| 10 | 推进弧线节点（含规划对齐） | 查询 C 发现变化 | update_arc_node（target_chapter 与卷纲 detail_json 对齐，偏差>3章以 volume 为准） |
| 11 | 新伏笔/悬念 | 有新**关键**伏笔 | create_timeline_entry（title + category + target_chapter required） |
| 12 | 回收/校准伏笔 | 查询 B 发现要回收/校准 | update_timeline_entry：回收（resolved + resolved_chapter_id）；校准（过时即修正，禁僵尸数据） |
| 13 | 更新读者认知（含去重） | 查询 D 发现悬念变化 | create_reader_perspective_entry / update_reader_perspective_entry（create 前先核对已有条目，同一事实不同角度优先 update 不新建，杜绝冗余条目） |
| 14 | 记录章节指纹 | 每章必做 | edit(goink.md, append)——格式见 anti-repetition：### 第N章 标题 + 开篇/场景/情感/对白/钩子/感官 各一行，段落间空行；必须 append，禁止 full_replace |
| 15 | 阶段切换 | 全部完成后 | **set_phase("done")** |

**维护一轮完成，禁止分段遗留**：7 项查询并行发出；更新按依赖链聚合；全部完成后一次性输出维护清单（每项 done/遗留及原因）。**不得留待用户追问再补**——每追加一轮都产生新上下文与缓存 miss。补录门槛只决定"某项做不做"，不决定"分几轮做"。

## 3. 批量模式细则

### 3.1 批量循环

- outline 一次产出全部 N 章大纲；write 循环写 N 章正文
- **每章 write 前必须 read outlines/NNN.md**（门禁 require 强制），把本章大纲锚定在上下文末尾再动笔，防串章
- write 阶段可回退 outline 修改大纲（loop）
- **每章写完后调 set_phase("write") 声明下一章开始**（同阶段幂等成功，只产生显式阶段记录，防连写敷衍）

### 3.2 迷你维护（每章实时结算）

每章 write 完成后立即执行——**只写不查**，把本章事实写入 DB：create_scene + update_character + create_timeline_entry + update_timeline_entry + create_item_occurrence + update_writing_snapshot。不调用 get_*/search_* 查询。这样下一章 writing_context 读到最新状态，避免批末才维护导致第 N 章读到第 1 章状态。整批末尾仍保留完整 maintain 收尾核对。

### 3.3 每 3 章自检（三章一轮，不积攒）

- **一致性（重点）**：调 get_characters / get_timeline / get_writing_snapshot + check_story_consistency（current_chapter=当前章号），对照最近 3 章正文查设定矛盾（角色状态不符/时间线错乱/伏笔矛盾/衔接断裂/重复内容，尤其死者复出、战力地点错乱）
- **文笔（次重点）**：read main-tech-revision-pass + sub-tech-anti-ai-grade 查最近 3 章节奏与 AI 味
- 发现问题立即 edit 修复，不攒到批末；自检不调 run_subagent、不 set_phase

### 3.4 批末审稿覆盖全批

进入 review 后 run_subagent 审读**本批全部 N 章**（子代理 fork 完整主历史，正文已在上下文）；审稿重点**一致性优先**（设定矛盾/衔接/伏笔），其次节奏与 AI 味；**子代理报告必须列出实际审读章节清单**，主 agent 先核对覆盖范围再修复——覆盖不全先补审，再逐章 read 自查 + edit 修复 + get_chapter_list 字数复查，**不要只审第 1 章**。

## 4. 卷结构（长篇必建）

开书/开新卷时，用 create_story_arc 创建卷（arc_type=volume），**必须填 start_chapter 和 end_chapter**：

- 卷是章节的物理分卷，与叙事弧线（main/sub/character/background）不同——卷管"第几章到第几章"，弧线管"故事线"
- 必填：name（如"第一卷·崛起"）、description（卷纲概述）、start_chapter、end_chapter
- 可选：detail_json（core_event / protagonist_change / ending_hook 等 JSON）
- 卷创建后，get_writing_context 在 prepare 阶段返回本卷涉及的实体（角色/物品/设定/伏笔），供写作参考
- **每卷结束时必须写卷摘要**：update_story_arc 将摘要写入 detail_json.volume_summary：`{"volume_summary": "本卷核心事件、角色变化、关键转折的一句话概括（50-120字）"}`
  - 跨卷连续性依赖此摘要；前卷摘要由 AI 按需 get_story_arcs 查看，不注入全量

## 5. 关键实体判定标准（补录门槛，防止设定库污染）

> **原则**：设定库只收录关键实体——路过的狗、野草里的石头、一次性环境布景不得入库。宁可漏录（后续用到时随时补），不可滥录（污染设定库、浪费每轮上下文、让 AI 每次读到无关实体）。

| 实体 | 应补录 | 不补录 |
|------|--------|--------|
| 角色 | 有名字 + 与主角/关键角色互动；参与剧情因果；后续会再出现；承担叙事功能（反派/盟友/见证者/信息源/对手） | 无名布景（路过的狗/掌柜/路人甲）、群演、一次性过客 |
| 物品 | 有名称 + 被主动使用/持有/传递；推动剧情；伏笔/弧线/大纲涉及或易主；后续会出现 | 纯环境描写物（野草/石头/桌椅/路灯）、一次性道具 |
| 场景/地点 | 场景：独立承载剧情（关键事件发生地/反复场所）；地点：角色反复进出、约束行动；伏笔：埋设或回收必入库 | 过场地点、路过街道、普通背景信息 |

**判定一句话**：这个实体未来会影响设定一致性吗？会→入库；只是本章背景→不录。

## 6. 硬约束

- 开书（init）必须先写全书总纲到数据库（outlines + outline_beats 表），未写总纲禁止切换 prepare
- 每章至少 1 个情绪锚点；情绪浓度高时禁止讲述句
- 每章至少 1 次快慢节奏切换；关键场景必须现场描写 ≥300 字
- 每章至少 1 个爽点（对照 shuangdian-pacing）；章末必有钩子且类型不与前 2 章重复
- **一章一事**：每章围绕一个核心事件/目标展开（大纲「关键事件」多条时，主事件只取一条为主线，其余并入铺垫/推进）
- 禁止功能报告体（连续 3 段无感官描写）；禁止散文体
- 每章 35-55 段，每段 60-80 字，总字数 2500-4000（以设置为准，默认下限 2500）
- 字数不足禁止转阶段
- 大战之间必须插非战斗章（对照 climax-scene）
- 完结前检查伏笔回收率 ≥90%（对照 foreshadow-cycle / book-completion）

## 7. 工具使用规则

- read：用 start_line/end_line 只读需要的行
- edit：优先 search_replace
- **关联规则**：更新物品持有者时同步 create_item_occurrence；创建角色时传 location_id；写完章节后 update_writing_snapshot
