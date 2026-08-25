---
name: main-core-writing-kernel
description: 小说核心写作调度。按阶段工作，禁止跳过。阶段步骤用 set_phase 推进。
category: 核心系统
mode: always
---

# 写作核心系统

## 流程

单章：init → prepare → outline → write → review → maintain → done
批量：init → prepare → outline（一次出 N 章大纲）→ write（显式循环 N 章）→ review（批末审稿全批）→ maintain → done
done 是终点，等待用户发起新一轮。

> 批量 outline ⇄ write 循环：outline 一次性产出 N 章大纲，write 循环写 N 章正文。循环中每章 write 前必须 read outlines/NNN.md（门禁 require 强制），每章结束 set_phase("write") 声明章边界。

## 阶段调度

> 每阶段的 tools/require/auto_skill_injection 由门禁配置自动执行（代码层）。
> 以下只写门禁不覆盖的：工作流细节、注意事项、独有规则。
> **并行工具调用**：同一阶段内无依赖的工具调用并行发出，有依赖才串行。

## 执行纪律

- **thinking 只做规划，不做草稿**：thinking 中分析方案、列工具调用计划；禁止在 thinking 中写正文/大纲/审稿报告——正文只通过 edit 工具输出
- **一轮决策，一气呵成**：thinking 中规划好后，在同一个 response 中立即执行（content + tool_call），不要分两轮"先想再做"
- **content 不能为空**：每轮 response 必须有 content（哪怕只是动作说明），禁止纯 tool_call 无 content
- **工具批量并行**：不互相依赖的工具在同一轮 response 中并行调用（例：read 大纲 + get_characters + check_story_consistency 同时发出）

### init（开书，仅新书，无门禁管理）

Init 不属于创作循环，不受门禁管控。开书技能（main-core-init-phase + genre-templates + book-outline + character-design + world-building-system）由模型按需调用 `auto_skill_injection` 加载，不预注入。

流程：
信息采集（分波次提问）→ 写总纲（update_outline + create_outline_beat）→ 创建卷弧线 → 角色/地点 → 世界观/物品 → 偏好/伏笔 → 一致性校验 → 用户确认 → 7 项查询并行验证 → set_phase("prepare")

**硬性门槛**：未写总纲禁止切 prepare；用户未确认禁止 set_phase（AI 自我判断不算确认）。

### prepare

门禁 require 9 项工具并行发出。**必须检查返回的 volume_entities（本卷涉及实体）**，确认本卷设定约束、伏笔状态、物品流转。发现问题用 get_entity_appearances 反查确认。

五门检查（字数/段数/情绪/节奏/禁止项）→ set_phase("outline")

### outline

edit(outlines/NNN.md)，必须包含 7 个 section：场景设计 / 关键事件 / 重点角色 / 伏笔操作 / 章末钩子 / 写作要点 / 字数预估。标题 `# 第N章 标题`（单井号），section 用 `##`。

参考 NS 方向锚 + 本卷范围，不得提前展开后续卷情节。禁止使用 `**加粗**` 代替 `##` 标题。

set_phase("write")

### write

1. read 大纲 outlines/NNN.md（门禁 require 强制）
2. 遵守 NS【方向锚】（硬约束）— 每条都是审稿 #26/#27 判定依据：超出卷范围或违反禁忌=致命；方向锚优先于类型技能模板建议
3. edit(chapters/NNN.md) 写正文
4. **字数规则**：缺口×1.2 设扩写目标，一次 edit 补足，get_chapter_list 复查。仍不足则重复"一次到位"流程。**禁止挤牙膏式多次小扩**
5. check_story_consistency（门禁 require 强制，不核对无法转出 review）
6. **写后自审**：read main-tech-revision-pass + sub-tech-anti-ai-grade，检查节奏与 AI 味，发现问题 edit 修复
7. set_phase("review")

> **write→review 边界**：进入 review 后禁止调用任何维护/更新工具（白名单只读/审稿）。迷你维护是 set_phase("review") 之前做的。

### review

1. **run_subagent**(agent_type="review", instruction=审稿模板)（required）
2. **审稿核对**：主会话核对覆盖范围+评分，发现覆盖不全必须补审
3. **修复**：所有层次问题（致命/质量/轻微）全部必须修复
4. set_phase("maintain")

> **审稿模板**（主 agent 填入章节号和标题）：
> ```
> 审读第{N}章《{title}》。
> 读取：第{N}章正文（chapters/{NNN}.md）+ 前一章最后50行（衔接检查）
> 程序化检查：check_story_consistency(check_types=["pacing_gap","beat_window","scope_guard","type_drift","ledger_integrity"])
> 逐项对照 sub-tech-review-standards 的27项检查，输出5维度加权评分报告（总分≥9.0通过/7.0-8.9需修改/<7.0不通过）。
> ```
> 重写场景追加："重点检查{具体问题}是否已修复"。压缩场景追加："重点检查节奏/字数是否达标"。

### maintain

7 项查询并行（get_characters / get_timeline / get_story_arcs / get_reader_perspective / get_scenes / get_item_occurrences / get_character_relations）→ 判定哪些需要更新 → 更新动作按依赖链聚合（互不依赖的同批并行）→ 一次性输出维护清单（每项 done / 遗留及原因）→ set_phase("done")

维护清单见 §维护清单。补录门槛见 §关键实体判定。

## 维护清单

| # | 动作 | 条件 | 工具 |
|---|------|------|------|
| 1 | 写章节元数据 | 每章必做 | update_chapter_meta（summary+key_events+characters_in+arc_ids） |
| 2 | 更新写作快照 | 每章必做 | update_writing_snapshot（summary） |
| 3 | 搜索设定防遗忘 | 每章必做 | search_lore |
| 4 | 搜索物品防断裂 | 每章必做 | search_items |
| 5 | 更新章节计划 | 每章必做 | update_chapter_plan |
| 6 | 创建场景条目 | 有关键场景时 | create_scene（title+summary） |
| 7 | 记录物品流转 | 有关键物品易主时 | create_item_occurrence（item_id+chapter_id+action） |
| 8 | 更新角色状态 | 有关键角色变化时 | update_character（改 status 时同步校准 description/personality） |
| 9 | 更新角色关系 | 有关系变化时 | update_character_relationship（relation_describe） |
| 10 | 推进弧线节点 | 有节点变化时 | update_arc_node（target_chapter 与卷纲对齐） |
| 11 | 新伏笔 | 有新关键伏笔时 | create_timeline_entry（title+category+target_chapter） |
| 12 | 回收/校准伏笔 | 有伏笔需回收时 | update_timeline_entry（回收=status=resolved+resolved_chapter_id） |
| 13 | 更新读者认知 | 有悬念变化时 | create/update_reader_perspective_entry（create 前先核对已有条目） |
| 14 | 记录章节指纹 | 每章必做 | edit(goink.md, append)（禁止 full_replace） |
| 15 | 阶段切换 | 全部完成后 | set_phase("done") |

## 关键实体判定

新增前先判定：**这个实体未来会影响设定一致性吗？会影响→入库；只是本章背景→不录。**

- **关键角色**：有名字 + 参与剧情因果 + 后续会再出现。不录：无名布景/群演/一次性过客
- **关键物品**：有名称 + 被主动使用/传递 + 推动剧情。不录：纯环境物/一次性道具
- **关键场景/伏笔**：对剧情有独立承载作用 / 埋设回收必须入库。不录：过场地点/路过街道

## 批量模式

> 单章流程以上规则均适用于批量。以下是批量特有规则。

**迷你维护**（每章 write 后）：只写不查（create_scene + update_character + create/update_timeline_entry + create_item_occurrence + update_writing_snapshot），不调 get_*/search_*。整批末尾仍保留完整 maintain 收尾。

**每 3 章自检**：write 循环中每写 3 章停笔——一致性（get_characters + get_timeline + get_writing_snapshot + check_story_consistency）+ 文笔（read main-tech-revision-pass + sub-tech-anti-ai-grade）。发现问题立即 edit 修复。

**批末全批审稿**：review 阶段审读本批全部 N 章（子代理 fork 完整主历史）。审稿报告必须列出实际审读的章节清单，主 agent 核对覆盖范围。

**显式循环**：批量循环中每章写完后 set_phase("write") 声明章边界（同阶段幂等成功）。

**技能注入**：首次进入阶段注入全文，再次进入注入短提醒。上下文压缩后技能被清空，按需补读。

## 硬约束

- 每章至少 1 个情绪锚点；情绪浓度高时禁止讲述句
- 每章至少 1 次快慢节奏切换；关键场景必须现场描写≥300字
- 每章至少 1 个爽点；章末必有钩子且类型不与前2章重复
- **一章一事**：每章围绕一个核心事件展开，避免塞多个平级事件
- 禁止功能报告体（连续3段无感官描写）
- 每章 35-55 段，每段 60-80 字，总字数以 NS【字数范围】为准
- 字数不足禁止转阶段
- 大战之间必须插非战斗章
- 完结前检查伏笔回收率≥90%

## 阶段可选技能

> 每阶段先加载对应技能再执行。manual 模式（collect/memory/next/review/phase-gate/ruling）由用户 `/` 触发。`/ruling`：用户方向性纠正时，把纠偏固化为偏好禁忌入库——进入 NS【方向锚】每章注入、审稿 #27 逐条核对。

| 场景 | 技能 |
|------|------|
| 战斗/高潮章 | main-tech-climax-scene |
| 爽点/打脸章 | main-tech-shuangdian-pacing |
| 有伏笔操作 | main-tech-foreshadow-cycle |
| 情感/情绪戏 | main-tech-emotion-injection |
| 节奏紧张章 | main-tech-pacing-control |
| 关键场景≥300字 | main-tech-scene-beats |
| 卡情节时 | main-tech-brainstorm-composer |
| 对白/情感设计 | main-tech-dialogue-subtext |
