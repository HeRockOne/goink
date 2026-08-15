---
name: main-core-writing-kernel
description: 小说核心写作调度。按阶段工作，禁止跳过。阶段步骤用 set_phase 推进。
category: 核心系统
mode: always
---

# 写作核心系统

## skill目录（加载顺序，避免浪费 read 调用）

- 内置 skill（数量以目录为准，通用 + 类型）位于 `/builtin/skills/<name>.md`（只读），auto 模式按需 read
- 同名 skill 优先级：小说级 > 用户级 > 内置（`/builtin/skills/`）
- always 模式 skill 已在会话开头注入，无需再 read

## 流程

单章：prepare → outline → write → review → maintain → prepare
批量：prepare → outline（一次出 N 章大纲）→ write（显式循环 N 章：每章动笔前 read 本章大纲，每章结束 set_phase("write") 声明章边界，每章写后迷你维护，每 3 章轻量自检）→ review（批末审稿覆盖全批）→ maintain → prepare

> 批量模式的 `outline ⇄ write（循环N章）` 含义：outline 阶段一次性产出全部 N 章大纲（连续 edit outlines/001.md ~ NNN.md），
> 然后 write 阶段循环写 N 章正文。**循环中每章 write 前必须 read outlines/NNN.md（本章大纲，门禁 require 强制）**，
> 把本章大纲锚定在上下文末尾再动笔，防止把别的章的大纲内容串进本章正文。write 阶段可回退 outline 修改大纲（loop）。

> **批量状态实时性（迷你维护）**：每章 write 完成后立即执行"迷你维护"——只写不查，把本章事实写入 DB
> （create_scene + update_character + create_timeline_entry + update_timeline_entry + create_item_occurrence + update_writing_snapshot），
> 不调用 get_*/search_* 查询。这样下一章的 get_writing_context 读到的是最新角色/伏笔/场景状态，
> 避免"整批末尾才 maintain 导致第 N 章读到第 1 章状态"。整批末尾仍保留完整 maintain（13 项清单 + goink.md 指纹）收尾核对。

> **批量质量节奏（每 3 章自检 + 批末全批审稿）**：
> - **每 3 章自检（三章一轮，不积攒，一致性 + 文笔双向）**：write 循环中每写 3 章停笔自检一次——
>   **一致性（重点）**：调 get_characters / get_timeline / get_writing_snapshot 读取角色状态、伏笔台账、
>   写作快照，**并调 check_story_consistency（current_chapter=当前章号）做程序化核对**（四类 SQL：
>   伏笔超期/角色断档/物品冲突/死者复出），对照最近 3 章正文检查设定矛盾（角色状态不符、时间线错乱、
>   伏笔状态矛盾、章节衔接断裂、重复内容）——AI 写作常见翻车是"前文死了的角色后文又出现、战力/地点
>   错乱"（注意力衰减 + 遗忘机制的产物），普通读者抓不住文笔，但设定矛盾一眼穿帮；
>   **文笔（次重点）**：read main-tech-revision-pass 与
>   sub-tech-anti-ai-grade 检查最近 3 章节奏、AI 味。发现问题立即 edit 修复，不攒到批末。
>   自检不调 run_subagent（那是批末的事），不 set_phase。
> - **批末审稿覆盖全批**：整批写完进入 review 阶段后，run_subagent 审读**本批全部 N 章**
>   （子代理 fork 完整主历史，正文已在上下文中，无需重复 read 注入）；审稿重点同样是
>   **一致性优先**（设定矛盾、章节衔接、伏笔状态），其次节奏与 AI 味；**子代理报告必须
>   列出实际审读的章节清单，主 agent 先核对覆盖范围再修复**——覆盖不全（漏章/只审开头）
>   先补审，然后根据审稿意见**逐章** read 自查 + edit 修复 + get_chapter_list 字数复查，
>   **不要只审第 1 章**。

> **批量 write 显式循环（每章一个阶段边界）**：批量循环中**每章写完后调 set_phase("write") 声明下一章开始**
> ——同阶段切换幂等成功，不重置任何校验状态，只产生显式阶段记录（UI/审计逐章可见、LLM 获得"本章完成
> 下一章开始"的明确信号，防止连写多章越写越敷衍）。
> **技能注入（全文一次 + 短提醒每轮）**：系统在首次进入阶段时注入该阶段必读技能**全文**
> （学习内容，常驻上下文）；**再次进入同阶段**（含同阶段 set_phase 声明章边界）时不再重复全文，
> 只注入**短提醒**（技能名 + 要点，唤起对该阶段技能的遵循）。**技能全文始终在上下文中，必须遵循；
> 若感觉技能要点模糊，用 auto_skill_injection 主动重读全文，不要凭印象创作**。
> 自检、修复、迷你维护**不需要** set_phase；只有阶段真正切换（outline→write、write→review）
> 或声明章边界时才调用。**上下文压缩后技能记录被清空**，需按需补读（auto_skill_injection 或
> 下次 set_phase 自动注入全文），不要因为"技能读过"就跳过补读。

## 卷结构（长篇必建）

开书/开新卷时，用 create_story_arc 创建卷（arc_type=volume），**必须填 start_chapter 和 end_chapter**（卷的章节范围）。
- 卷是章节的物理分卷，与叙事弧线（main/sub/character/background）不同——卷管"第几章到第几章"，弧线管"故事线"
- 每卷必填：name（如"第一卷·崛起"）、description（卷纲概述）、start_chapter、end_chapter
- 可选：detail_json（卷级规划：core_event / protagonist_change / ending_hook 等 JSON）
- 卷创建后，get_writing_context 会在 prepare 阶段返回本卷涉及的实体（角色/物品/设定/伏笔），供写作参考
- **每卷结束时必须写卷摘要**：用 update_story_arc 将本卷摘要写入 detail_json.volume_summary，格式为 JSON 对象：
  `{"volume_summary": "本卷核心事件、角色变化、关键转折的一句话概括（50-120字）"}`
  - 跨卷连续性依赖卷摘要——第 20 卷的 AI 通过 get_writing_context 的 volume.detail_json 看到前 19 卷摘要，但不注入全量（只返回当前卷的 detail_json，前卷摘要由 AI 按需调用 get_story_arcs 查看）

## 阶段指令

> **并行工具调用**：同一阶段内无依赖的工具调用**并行发出**（一次请求多个工具调用，模型原生支持并行 tool_calls）——如 prepare 9 项必查、maintain 查询批、迷你维护 6 项、每 3 章自检查询，一次并行发出而不是一次一个；有依赖才串行（后一个需要前一个的结果）。并行减少轮边界与重复 thinking，且不损失任何数据。

### prepare

**必读技能在动笔前已由系统就绪**（门禁 auto_skill_injection 阶段会在 set_phase 时自动注入）。然后执行：
1. **get_writing_context**（required）— 一次获取树状全量状态。**必须检查返回的 volume_entities（本卷涉及的实体清单）**，确认本卷设定约束、伏笔状态、物品流转
2. **get_chapter_list**（required）— 确认章节编号连续
3. **get_characters**（required）— 确认角色阵容。**传 brief=true**（角色概览只需 id/name/location/item_count；角色生死等状态已由 get_writing_context 的 characters 段提供——不要全量拉取 description/personality 大字段）
4. **get_timeline**（required）— 确认伏笔状态。**传 current_chapter**（窗口切分：近期历史+未来+异常，不要分页翻全量）
5. **get_story_arcs**（required）— 确认弧线进度。**传 current_chapter**（窗口切分同上）
6. **get_reader_perspective**（required）— 确认读者认知。**全量返回**（写作必须知道活跃悬念/误知的具体内容，counts_only 只给数量不够用）
7. **get_writing_snapshot**（required）— 确认写作进度
8. **get_scenes**（required）— 确认本章场景。**传 chapter_id 按章查**（不传只返回最近 100 个场景）
9. **get_preferences**（required）— 确认创作偏好
10. 按需技能（不强制，按场景读）：main-tech-genre-templates、main-tech-book-outline（看卷纲时）、main-tech-brainstorm-composer（卡情节时构思）
11. 发现有异常（如角色断档、设定前后矛盾）用 **get_entity_appearances** 反查确认
12. 五门检查（字数/段数/情绪/节奏/禁止项）→ **set_phase("outline")**

### outline

**必读技能在动笔前已由系统就绪**。然后：
2. **先消费总纲与卷纲**：outline 阶段开始前，确认 get_writing_context 返回的：
   - `outline`（全书总纲摘要：核心矛盾/主角成长弧线/结局方向）— 本章事件必须服务于它
   - `volume`（当前卷：本卷核心事件、主角状态变化、爽点位置、收尾钩子、需回收伏笔）
   - `progress`（当前章号 + 本卷 start~end 范围）— **本章纲不得超出本卷范围，后续卷情节禁止提前展开**
3. 加载技能（**必读技能必须在动笔前已加载**：技能是创作指导，先读再写，不是切换阶段的手续。若技能内容已被滚动压缩出上下文，必须重新 auto_skill_injection，不要为了省一次 read 赌记忆——技能漏读会让大纲跑偏，门禁会在你动笔时拦截）：
   - 必读：main-tech-chapter-hook-enhanced, main-tech-chapter-title-design（门禁强制）
   - 常备：main-tech-book-outline, main-tech-chapter-opening, main-tech-maliang-method, main-tech-dialogue-subtext（有情感/对话/爽点设计时用）
   - 新书首次 outline 加：main-tech-golden-three-chapters, main-tech-golden-finger-design
   - 对应类型专精 skill（main-type-*，每本小说只加载对应 1 个，不重复读）
4. **edit**(outlines/NNN.md)（required）— 写大纲。
   **格式要求：**
   - 章节标题用 `# 第N章 标题`（单井号，一行）
   - 各 section 用 `## 标题`（双井号），必须包含以下全部 section，可自由扩展更多：
     - `## 场景设计`：核心场景、环境氛围、时间、地点
     - `## 关键事件`：按叙事节拍编号（1. 2. 3.），标注字数区间和功能
     - `## 重点角色`：角色动机、叙事功能、本章状态
     - `## 伏笔操作`：埋设/回收/推进，标注伏笔 ID 和目标章节
     - `## 章末钩子`：类型、设计、预期读者效果
     - `## 写作要点`：情绪节奏、信息密度、禁忌项
     - `## 字数预估`：目标字数、段落数、每段字数
   - 禁止使用 `**加粗**` 代替 `##` 标题
5. **set_phase("write")** 自动进入下一阶段（无需等待用户确认——门禁自动推进与主动 set_phase 双通道，8/8 版本即此行为）

### write

**必读技能在动笔前已由系统就绪**（write 阶段 5 个必读技能在 set_phase("write") 时自动注入）。然后：
1. **read**（required）— 读本章大纲 outlines/NNN.md 与相关文件，门禁 require 强制
2. 加载技能（**必读技能必须在动笔前已加载**：技能是创作指导，先读再写。若技能内容已被滚动压缩出上下文，必须重新 auto_skill_injection，不要为了省一次 read 赌记忆——技能漏读会写崩，门禁会在你动笔时拦截）：
   - 必读：main-tech-show-dont-tell, main-tech-anti-ai-writing, main-tech-pov-purity, main-tech-info-density, main-tech-word-count-calibration（门禁强制）
   - 情景按需（**仅本章涉及该情景时读**，普通章不读）：
     - 战斗/高潮章 → main-tech-climax-scene
     - 爽点/打脸章 → main-tech-shuangdian-pacing
     - 本章有伏笔操作 → main-tech-foreshadow-cycle
     - 情感/情绪戏 → main-tech-emotion-injection
     - 节奏紧张章 → main-tech-pacing-control
     - 关键场景现场描写 ≥300 字 → main-tech-scene-beats
    - 字数校验用 get_chapter_list（代码校验，min/max 以设置为准）；**字数不足必须一次扩到位，禁止挤牙膏式多次小扩**：先算出缺口（目标下限 - 当前字数），按缺口 ×1.2 设定扩写目标（预留余量，避免第二次仍差一点），一次 read 定位可扩段落、一次 edit 补足并超出目标下限，然后立即 get_chapter_list 复查；复查仍不足则重复"一次到位"流程（每次都以当前缺口 ×1.2 为目标），不要一次只补一两百字等校验打回
4. **edit**(chapters/NNN.md)（required）— 写正文
5. 校验字数（2500-4000，以设置中 min/max 为准，get_chapter_list 代码校验；默认下限 2500）
6. 记录关键物品出现 → create_item_occurrence
6.5. **check_story_consistency**（required）— 写完立即程序化核对（current_chapter=本章章号）：四类硬错误
   （伏笔超期/角色断档/物品冲突/死者复出）由 SQL 实证输出，发现错误立即定位修复，修复后重跑核对直到
   通过——门禁 require 强制，不核对无法转出 review（写时把关，不等审稿阶段才发现设定硬伤；
   get_writing_context 的 dead_characters 名单写作前就要记住，死者不得复出）
7. **set_phase("review")**

### review

1. **run_subagent**(agent_type="review")（required）— 启动审稿。**批量模式：审读本批全部 N 章**（子代理 fork 完整主历史，正文已在上下文中），不要只审第 1 章。**审稿报告必须列出实际审读的章节清单**（如"已审：第 3-7 章"），主 agent 收到报告先核对覆盖范围，发现覆盖不全（漏章/只审了开头几章）必须再启动子代理补审未覆盖章节，全部覆盖后才进入修复
2. **审稿核对（身份差异：主会话核对全量，子代理定向）**：
   - 主会话（作家视角）按意见修复前，先核对状态：**get_characters 全量**（核对角色 status：alive/dead 与正文一致性——brief 无 status 会漏检）、**get_timeline/get_story_arcs 传 current_chapter**（核对伏笔/弧线进度）、**get_reader_perspective 全量**、**check_story_consistency**（自动 DB 核对，输出问题条目）；读本章正文分段核对（read start_line/end_line）
   - 审稿子代理（fork 完整主历史，正文+writing_context 已在上下文）：只做**少量定向核对**（get_characters brief+size 小、get_timeline current_chapter），加 check_story_consistency 自动核对，然后输出审读报告——不要重复拉全量
3. 根据意见修复（**批量模式：逐章 read 自查 + edit 修复 + 字数复查**，查 N 修 N）
4. **set_phase("maintain")**

### maintain（逐项检查清单，每章必做）

**必读技能在动笔前已由系统就绪**。然后逐项执行以下清单。

**每章必做的状态查询**（门禁 require 强制，宁可多调用不可漏维护）：

| # | 查询项 | 工具 | 确认目的 |
|---|--------|------|---------|
| A | 查角色状态 | get_characters | 是否有角色需要更新 |
| B | 查伏笔状态 | get_timeline | 是否有伏笔要回收/校准 |
| C | 查弧线进度 | get_story_arcs | 是否有节点要推进 |
| D | 查读者认知 | get_reader_perspective | 是否有悬念要维护 |
| E | 查场景状态 | get_scenes | 是否有场景要创建/更新 |
| F | 查物品流转 | get_item_occurrences | 是否有物品易主未记录 |
| G | 查角色关系 | get_character_relations | 是否有关系变化未记录 |

> 每章 maintain 必须完成以上 7 项状态查询 + 下方更新动作，门禁 require 才放行。查到有变化就执行对应更新工具（6-14 项）。

> **维护一轮完成，禁止分段遗留**：进入 maintain 后必须**一轮内完整执行**——7 项状态查询**并行发出**（一次请求多个查询，勿逐条）；更新动作按依赖链聚合（互不依赖的更新工具同批并行发出，如 create_scene/update_character/update_timeline_entry 可同批；先查后更、edit 前先 read 属必要依赖链才串行）。全部更新完成后**一次性输出维护清单**（每项 done / 遗留及原因），**不得**留待用户追问"还有 X 没做"再补——用户每追加一轮都产生新上下文轮次与缓存 miss，且分散的维护轮边界增加成本。补录门槛（关键实体判定标准）只决定"某项做不做"，不决定"分几轮做"。

| # | 检查项 | 条件 | 工具 |
|---|--------|------|------|
| 1 | **写章节元数据** | 每章必做 | update_chapter_meta（summary + key_events + characters_in + arc_ids 全部 required） |
| 2 | **更新写作快照** | 每章必做 | update_writing_snapshot（summary required） |
| 3 | **搜索设定防遗忘** | 每章必做 | search_lore |
| 4 | **搜索物品防断裂** | 每章必做 | search_items |
| 5 | **更新章节计划** | 每章必做 | update_chapter_plan（main-cmd-next 触发） |
| 6 | **创建场景条目** | 查询 E 发现有**关键**场景时（判定见下方标准） | create_scene（title + summary required） |
| 7 | **记录物品流转** | 查询 F 发现有**关键**物品易主时（判定见下方标准） | create_item_occurrence（item_id + chapter_id + action 全部 required） |
| 8 | **更新角色状态** | 查询 A 发现有**关键**角色变化时（判定见下方标准） | update_character |
| 9 | **更新角色关系** | 查询 G 发现有变化时 | update_character_relationship（relation_describe required） |
| 10 | **推进弧线节点** | 查询 C 发现有变化时 | update_arc_node |
| 11 | **新伏笔/悬念** | 有新**关键**伏笔时（判定见下方标准） | create_timeline_entry（title + category + target_chapter 全部 required） |
| 12 | **回收伏笔** | 查询 B 发现有要回收的时 | update_timeline_entry（resolved_chapter_id） |
| 13 | **更新读者认知** | 查询 D 发现有悬念变化时 | create_reader_perspective_entry / update_reader_perspective_entry |
| 14 | **记录章节指纹** | 每章必做 | edit(goink.md, change_type=append)（追加本章指纹，格式见 anti-repetition skill：### 第N章 标题 + 开篇/场景/情感/对白/钩子/感官 各一行，段落间空行。必须用 append 模式，禁止 full_replace；goink.md 不做其他用途，状态/悬念/设定一律写 DB） |
| 15 | **阶段切换** | 全部完成后 | set_phase("prepare") |

## 关键实体判定标准（补录门槛，防止设定库污染）

> 新增角色/物品/场景/伏笔前，先按下表判定。**设定库只收录关键实体**——路过的狗、野草里的石头、一次性环境布景不得入库。宁可漏录（后续用到时随时补），不可滥录（污染设定库、浪费每轮上下文、让 AI 每次读到无关实体）。

### 关键角色（应补录）

- **有名字**，且与主角/其他关键角色有互动（对话、冲突、合作、委托）
- 参与**剧情因果**：推动事件、提供关键信息、改变局势走向
- 后续章节会再次出现（大纲/弧线/伏笔中提及，或显然会复用）
- 承担叙事功能：反派、盟友、见证者、信息源、对手戏对象

**不补录**：无名布景（路过的狗、掌柜、路人甲——除非确认后续复用）、群演（战斗中杂兵、围观群众）、一次性登场即退场的过客。

### 关键物品（应补录）

- **有名称**，且被角色主动使用/持有/传递（信物、武器、情报文件、关键道具）
- 推动剧情：解开谜题、改变局势、引发冲突
- 在伏笔/弧线/大纲中涉及，或角色之间易主（需 create_item_occurrence 记录流转）
- 后续章节还会出现

**不补录**：纯环境描写物（野草、石头、桌椅、路灯）、一次性道具（随手用完即弃且无后续）。

### 关键场景/地点/伏笔

- 场景：对剧情有独立承载作用（关键事件发生地、反复出现的场所）；不录过场地点
- 地点：角色会反复进出、对行动有约束（禁地、据点、家园）；不录路过街道
- 伏笔：埋设或回收必须入库（create_timeline_entry）；普通背景信息不算伏笔

### 判定一句话

**这个实体未来会影响设定一致性吗？会影响→入库；只是本章背景→不录。**

## 阶段技能表（内置 skill 全量调度）

> 每阶段先加载对应技能再执行。manual 模式（collect/memory/next/review/phase-gate）由用户 `/` 触发，不在此表。

| 阶段 | 技能 |
|------|------|
| **init（开书）** | main-core-init-phase（开书流程）, main-tech-genre-templates（12类型）, main-tech-book-outline（总纲）, main-tech-character-design（角色设计）, main-tech-world-building-system（世界观） |
| **prepare（准备）** | main-tech-common-sense-logic（一致性）, main-tech-genre-templates, main-tech-book-outline（卷纲）, main-tech-brainstorm-composer（卡情节时构思） |
| **outline（大纲）** | main-tech-book-outline（章节蓝图）, main-tech-chapter-opening（每章开头）, main-tech-chapter-hook-enhanced（章末钩子）, main-tech-chapter-title-design（章节标题设计）, main-tech-maliang-method（打脸/金手指节奏）, main-tech-dialogue-subtext（对白设计）, main-tech-emotional-arc（情感弧线）, main-tech-opening-chapter（第一章开篇） |
| **write（正文）** | main-tech-show-dont-tell（展示）, main-tech-info-density（信息密度）, main-tech-pov-purity（视角）, main-tech-anti-ai-writing（九条铁律）, main-tech-shuangdian-pacing（爽点节奏）, main-tech-climax-scene（战斗章）, main-tech-foreshadow-cycle（埋伏笔）, main-tech-pacing-control（节奏控制）, main-tech-scene-beats（场景节拍）, main-tech-emotion-injection（情绪注入）, main-tech-word-count-calibration（字数校准） |
| **write后（自审）** | main-tech-revision-pass（修改润色）, sub-tech-anti-ai-grade（用词级反AI） |
| **review（审稿）** | run_subagent(agent_type="review") → sub-tech-review-standards（22项判定） |
| **maintain（维护）** | main-tech-anti-repetition（去重）, main-tech-foreshadow-cycle（回收伏笔） |
| **完结** | main-tech-book-completion（完本清单） |

## 硬约束

- 开书（init）必须先写全书总纲到 book-outline.md（核心矛盾/主角成长弧线/结局方向/篇幅规划），未写总纲禁止切换 prepare
- 每章 at least 1 个情绪锚点；情绪浓度高时禁止讲述句
- 每章至少1次快慢节奏切换；关键场景必须现场描写≥300字
- 每章至少1个爽点（对照 main-tech-shuangdian-pacing）；章末必有钩子且类型不与前2章重复
- **一章一事**：每章围绕一个核心事件/目标展开（大纲「关键事件」多条时，主事件只取一条为主线，其余并入铺垫/推进），避免一章塞多个平级事件导致节奏失控
- 禁止功能报告体（连续3段无感官描写）；禁止散文体
- 每章35-55段，每段60-80字，总字数2500-4000（以设置为准，默认下限2500）
- 字数不足禁止转阶段
- 大战之间必须插非战斗章（对照 main-tech-climax-scene）
- 完结前检查伏笔回收率≥90%（对照 main-tech-foreshadow-cycle / main-tech-book-completion）

## 工具使用规则

- read：用start_line/end_line只读需要的行
- edit：优先search_replace
- **关联规则**：更新物品持有者时同步create_item_occurrence；创建角色时传location_id；写完章节后update_writing_snapshot
