---
name: main-tech-data-hygiene
description: 数据卫生规则。每次维护阶段必须执行的校准动作，防止设定字段过时、规划系统脱节、读者认知条目冗余。
category: 写作规范
mode: auto
---

# 数据卫生规则

> 与 maintain 阶段 15 项清单配合。内核清单管"状态变更"，本 skill 管"内容校准"——两者互补。

## 规则一：伏笔内容校准

每次 get_timeline 后，遍历所有 **pending** 条目：

1. 对照当前章节剧情，检查每条伏笔的 title 和 content 是否还准确描述当前状态
2. 如果剧情已推进导致 title/content 过时（如"条件进度1/5"已跳到2/5），必须 update_timeline_entry 修正
3. 如果伏笔的 target_chapter 已过但剧情尚未回收，评估是否需要调整 target_chapter
4. 禁止让过时伏笔静默存在——僵尸数据会在后续章节误导 AI

**触发条件**：每章 maintain 阶段 get_timeline 后必做

## 规则二：字段校准

每次 update_character / update_item 修改 status 时：

1. 同步检查 description / lore / personality 字段是否还匹配实际剧情
2. 如果字段描述的是"未来计划"（如"第20章从公司翻出工作牌"）而实际剧情已偏离，必须修正
3. 禁止 description/lore 写"预测性剧情"——只写已发生的事实

**触发条件**：每次 status 变更时必做

## 规则三：读者认知去重

每次 create_reader_perspective_entry 前：

1. 必须先 get_reader_perspective 查已有条目
2. 如果新条目内容与已有条目重叠（同一事实的不同角度），优先 update 已有条目，不新建
3. 只有全新信息才 create 新条目
4. 合并后归档被替代的旧条目（update_reader_perspective_entry 标记 revealed_chapter——归档保留历史，勿物理删除；确需删除才用 delete_record）

**触发条件**：每次新增读者认知前必做

## 规则四：规划系统对齐

每次 get_story_arcs 后：

1. 对比 volume detail_json 中的章节规划与 arc node 的 target_chapter
2. 偏差超过 3 章的节点，以 volume detail_json 为准校准 arc node
3. volume 是精确规划（每卷结束前确定），arc node 是粗略估计——冲突时以 volume 为准

**触发条件**：每章 maintain 阶段 get_story_arcs 后必做

## 自检清单

1. 所有 pending 伏笔的 title/content 是否匹配当前剧情？不匹配 → 修正
2. 刚改过 status 的角色/物品，description/lore 是否同步校准？过时 → 修正
3. 新增读者认知前是否查了已有条目？未查 → 先查
4. 弧线节点 target_chapter 与卷纲规划是否一致？偏差>3章 → 校准
5. check_story_consistency 是否通过（maintain 阶段门禁 require 强制调用）？未通过 → 修复后重跑