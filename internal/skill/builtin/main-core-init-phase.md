---
name: main-core-init-phase
description: 开书阶段工作流（goink 系统适配版）。首次对话必须先加载此 skill，按系统数据模型完成信息采集→设定入库→一致性校验→用户确认，全部通过后 set_phase("prepare")。后续对话无需再加载。
category: 核心系统
mode: auto
---

# 开书阶段（init，仅新书首次）— goink 适配版

## 本阶段在 goink 系统中的产出物

开书阶段只做两件事，产出物只有两类：

1. **数据库总纲**：`outlines` + `outline_beats` 表（核心矛盾/成长弧线/结局方向/篇幅规划/大爽点，通过 `update_outline` + `create_outline_beat` 写入）
2. **数据库实体**：7 类实体全部入库——故事弧线、地点、角色、世界观、物品、偏好、伏笔（含弧线节点）

> 本阶段不写 goink.md（它是写作阶段的章节指纹台账，开书不涉及）；不创建场景、读者认知、角色关系、地点连通（留到 prepare/outline 阶段按需创建）。

## init 阶段允许使用的工具

- 总纲：`update_outline`、`create_outline_beat`、`update_outline_beat`、`delete_outline_beat`、`get_outline`
- 编辑：`edit`（init 阶段不使用）
- 创建：`create_story_arc`、`create_arc_node`、`create_location`、`create_character`、`create_lore`、`create_item`、`create_timeline_entry`、`create_preference`
- 查询验证：`get_characters`、`get_locations`、`get_story_arcs`、`get_lore`、`get_items`、`get_timeline`、`get_preferences`（**这 7 项就是门禁 require，阶段结束前必须全部调用过**）
- 系统：`auto_skill_injection`、`set_phase`

必读技能（开书时按需加载）：开始采集前调用 `auto_skill_injection(skills="main-tech-genre-templates,main-tech-book-outline,main-tech-character-design,main-tech-world-building-system")` 一次性加载以下 4 个技能到上下文，写总纲前确认内容已就绪：
- `main-tech-genre-templates`（类型模板，用于题材确认和一致性校验）
- `main-tech-book-outline`（总纲模板，用于写总纲结构）
- `main-tech-character-design`（角色设计，用于角色创建）
- `main-tech-world-building-system`（世界观构建，用于世界观和底层铁则）

## 总体流程

```
信息采集（分波次提问）→ 写总纲（update_outline + create_outline_beat）→ 创建卷弧线 → 弧线/地点/角色/节点 → 世界观/物品 → 偏好/伏笔 → 一致性校验 → 用户确认 → 结构验证(7项查询) → set_phase("prepare")
```

**三条铁律**：
1. **先收集，再生成。** 信息不足禁止创建任何设定；未过「充分性闸门」禁止往后推进。
2. **冲突让用户裁决。** AI 禁止替用户拍板方向判断（节奏快慢、爽点位置、战线长短属于用户意图，不是可代拟的技术参数）。
3. **AI 自我判断不算确认。** 用户没有明确回复确认（或提出修改并被落实）之前，禁止 set_phase("prepare")。

---

## 一、信息采集（分波次提问）

每轮只问"当前缺失且会阻塞下一步"的信息；用户已明确的不重复问；卡住时给 2-4 个候选方向（见 genre-templates 12 种类型）。

| 波次 | 必收项 | 落到 goink 哪个实体 |
|------|--------|-------------------|
| 1 题材卖点 | 书名/题材(可复合)/目标规模(字数或章数至少一个)/一句话故事/核心冲突/目标读者平台 | outlines 表（core_conflict、word_count_plan） |
| 2 主角反派 | 主角姓名/核心欲望/核心缺陷(要付代价)/主角结构/感情线配置/反派分层(小中大)+镜像对抗一句话 | story_arc(主线) description、character.personality |
| 3 金手指 | 类型/名称/风格/可见度/不可逆代价(或明确无+理由)/成长节奏；条件项:系统流→系统性格+升级节奏,重生→时间点+记忆完整度,传承→辅助边界+出手限制 | item(金手指作为物品)、item.upgrade 相关写 lore 或总纲升级机制 |
| 4 世界观 | 世界规模/**三条底层铁则**(世界靠什么运转;核心稀缺是什么;金手指为何能存在)/力量体系等级划分(3-5大境界,每级质变)/势力格局/社会阶层 | lore(每条按 9 分类入库,绑 arc_id) |
| 5 约束节奏 | 反套路规则1-2条/硬约束2-3条(含禁忌)/节奏偏好(如"第N章前要有首次爽点")/爽点分布(每个大爽点标注章节号) | create_preference(节奏红线**原样**记入,禁止降级为建议)、volume detail_json.big_shuangdian |

---

## 二、设定入库（依赖链，顺序不可颠倒）

### 第 0 步：写全书总纲（update_outline + create_outline_beat，最先写）

用 update_outline 写入总纲数据，用 create_outline_beat 创建大爽点。必须包含：
- 核心矛盾一句话（core_conflict）
- 主角成长弧线（growth_arc）
- 结局方向（ending_direction）
- 篇幅规划（word_count_plan，单位：万字）
- 大爽点列表（每个大爽点调用 create_outline_beat，含 chapter + description）

**总纲写完先自查（一致性校验第 1-6 条），通过后把浓缩版展示给用户确认，用户确认前不创建任何数据库实体。**

### 第 1 步：创建卷弧线（create_story_arc，arc_type=volume）

goink 强制要求：**长篇必须建卷**。卷是章节的物理分卷，写法：
- **卷弧线必填 start_chapter 和 end_chapter**（覆盖连续章节范围，卷之间不重叠）
- name 用"第X卷·卷名"（如"第一卷·崛起"）
- description 写卷纲概述
- detail_json 写结构化卷纲，字段建议：`core_event`（本卷核心事件）/ `protagonist_change`（主角状态变化路径）/ `ending_hook`（卷末钩子）/ `big_shuangdian`（爽点数组，每项含 chapter+desc）/ `volume_pacing`（节奏分段）
- 卷的 end_chapter 与篇幅规划一致

### 第 2 步：创建叙事弧线（create_story_arc，arc_type=main/sub/character/background）

- 至少 1 条主线；支线/角色线/背景线按需（开局 ≤3 条弧线为宜）
- description 必须含：核心目标、主要冲突、关键转折点、预期结局
- importance：1=点缀、3=重要、5=核心主线
- 主线弧线的 description 与总纲核心矛盾一致，是总纲的"数据库投影"

### 第 3 步：创建核心地点（create_location，至少 3 个）

- 字段：name(具体有辨识度) / location_type(8选:城镇/村落/山脉/建筑/秘境/水域/战场/其他) / description(环境氛围+视觉特征+范围+功能,避免空洞形容词)
- parent_location_id 构建层级树（大陆→国家→城市→街区）；有空间关系的四处留到后续阶段建边
- 建议：核心舞台 2-3 个 + 外围场景 1-2 个

### 第 4 步：创建核心角色（create_character，至少 2 个）

- name 必填；description 自然语言 ≥200 字（外貌/身份/性格基调/核心动机）
- personality 为字符串形式 JSON，建议含 role/traits/background/motivation 四 key
- abilities 为**纯字符串数组**（如 `["剑术","隐身"]`，禁止对象数组）；有等级刻画时等级信息写进 description
- location_id 必填（关联第 3 步地点）——**先建地点后建角色**
- status 默认 alive；只建有戏份的角色，路人用一句话带过
- 核心角色 ≤5 个，次要角色后续按需添加

### 第 5 步：创建弧线节点（create_arc_node，至少 2 个，位于主线/卷弧线）

- 节点 = 弧线里程碑（关键转折点），不是每章一个；建议每条弧线 5-10 个
- title 概括核心事件；description 含事件内容+角色状态变化+对弧线的推进作用
- target_chapter 预计发生章节号；**先松后密**（前半段 5-8 章间距，后半段 2-3 章）

### 第 6 步：创建世界观设定（create_lore，至少 1 条，建议 3-5 条）

- category 9 选：力量体系/社会构成/历史事件/核心冲突/天道法则/文化习俗/种族/地理概述
- content 按分类写法（力量体系→等级/进阶条件/代价/克制；社会构成→权力结构/阶层/势力；历史事件→起因经过结果影响；核心冲突→对立双方/根源/态势；天道法则→底层规则；地理概述→地图/区域特色）
- **arc_id 必填**（没有弧线关联的设定是无效设定）
- reveal_chapter_id 控制信息投放：1-5 章只暗示、6-15 章小范围展示、16 章+系统性揭示
- is_public：true=读者已知公开设定，false=秘密（反转用）
- 三条底层铁则优先入库（世界运转机制/核心稀缺/金手指存在依据）

### 第 7 步：创建重要物品（create_item）

- 只创建对剧情有推动作用的物品（金手指、信物、关键道具）；narrative_role 四档：key_prop(核心道具)/supporting(辅助)/minor(小道具)/normal(普通)
- 字段：name / item_type(9选:法宝/丹药/灵药/功法/地图/信物/武器/防具/普通物品) / description(外观功能) / lore(来历历史)
- owner_id 关联持有角色；location_id 当前位置；arc_id 所属弧线（开局可暂不关联，>0 才写入）；first_chapter_id 首次出现章
- 金手指建议以 key_prop 入库并关联主角

### 第 8 步：创建创作偏好（create_preference，3-5 条）

- category 建议分类名：写作风格 / 字数规则 / 禁忌事项 / 角色命名 / 世界观规则
- content 写**具体可执行**的规则（"每章至少一个打斗场面不少于 500 字"），不要模糊期望（"文笔要好"）
- **用户明确说出的节奏红线（如"第N章要碾压""节奏要快"）必须原样记入，禁止 AI 擅自降级为建议**
- 创建后立即执行一致性校验第 7 条（偏好 vs 总纲对照）

### 第 9 步：创建伏笔（create_timeline_entry，至少 3 条，覆盖前 20 章）

- category：foreshadowing（伏笔）/ user_directive（用户创作指令）
- title 简短；content 含：埋下什么线索 + 预期如何回收 + 回收条件
- target_chapter 预计回收章（**不建议超过当前规划范围 + 20 章**，太长超出上下文易遗忘）
- importance 1-5：1=几章内回收，3=一卷内，5=贯穿全文核心伏笔
- 开局伏笔建议含：主角身世/金手指来历/第一个反派的背后势力 这类长线钩子

---

## 三、一致性校验（强制步骤，任一不过必须修正或请用户裁决）

开书阶段最常见的崩坏是"两套规则静默并存"——文件说一套、数据库排另一套、偏好说第三套，AI 只会随机听一个。以下校验全部在**用户确认之前**完成：

| # | 校验项 | 对照来源 | 不通过处理 |
|---|--------|---------|-----------|
| 1 | 类型节奏 vs 爽点排布 | genre-templates 类型节奏表 vs 总纲大爽点位置 | 按类型模板修正总纲（力量碾压类：首次大爽点 ≤5-8章,间隔 ≤10-15章,连续5章无武力展示即违规） |
| 2 | 节奏偏好 vs 爽点位置 | create_preference 节奏类 vs 总纲/卷弧线 detail_json.big_shuangdian | 列出冲突条目，逐条请用户裁决，按裁决修改总纲或偏好 |
| 3 | 人设承诺 vs 事件排布 | 总纲成长弧线性格描述 vs 大爽点间距与事件手段 | 调整其一：人设承诺了什么，事件排布必须能兑现 |
| 4 | 金手指 vs 世界规则 | 金手指设定 vs 三条底层铁则 | 铁则不推翻；金手指必须能在世界规则下成立，否则改金手指 |
| 5 | 禁忌 vs 情节设计 | 偏好禁忌 vs 总纲/卷纲情节 | 删冲突情节，或用户裁决（禁忌优先） |
| 6 | 手段 vs 力量等级 | 主角解决冲突的主要手段 vs 主角力量等级 | 有压倒性力量的主角必须正面碾压；若总纲大量依赖证据收集/心理博弈/潜行处理本可正面解决的对手，**默认判定类型错位**，须向用户确认是否为预期智斗流（力量流题材禁止） |
| 7 | 偏好 vs 总纲（入库后立即做） | get_preferences 全文 vs get_outline（总纲数据库） | 冲突立即列出并请用户裁决，禁止静默保留两套规则 |
| 8 | 数据库总纲 vs 卷纲同步 | get_outline（总纲） vs get_story_arcs（卷弧线 detail_json.big_shuangdian） | 总纲大爽点与卷弧线爽点位置一致；卷的 start/end_chapter 与篇幅规划一致 |
| 9 | 过渡章可填性 | 两大爽点之间的过渡章事件清单 | 过半靠文戏（证据链/谈判/潜行）填充=间距过大，压缩间距或加密小爽点 |

---

## 四、用户确认（强制交互，AI 自我判断不算确认）

1. **总纲确认**：总纲写入并自查通过后，把浓缩版逐条展示——核心矛盾 / 成长弧线（各阶段章节范围）/ 各卷大爽点位置 / 篇幅规划 / 主角解决冲突的主要手段 / 金手指与代价。用户明确确认或提出修改并被落实前，禁止创建任何数据库实体。
2. **初始化摘要确认**：全部入库 + 一致性校验通过后，生成"初始化摘要草案"（故事核/主角核/金手指核/世界核/创意约束核/爽点布局），展示给用户逐条确认。
   - 用户仅改局部：回到对应波次最小重采集，改完重跑涉及项校验
   - 用户否决方向：回对应波次重采集，已入库错误设定用 delete_record 清理
   - 用户未明确确认：禁止 set_phase("prepare")

---

## 五、充分性闸门（未过禁止推进）

1. 书名、题材（可复合）已确定
2. 目标规模可计算（字数或章数至少一个）
3. 主角姓名 + 欲望 + 缺陷完整
4. 世界规模 + 力量体系类型完整
5. 金手指类型已确定（允许"无金手指"）
6. 创意约束已确定：反套路 1 条 + 硬约束至少 2 条，或用户明确拒绝并记录原因
7. 一致性校验 9 项全部通过（或冲突已按用户裁决修正）
8. 用户确认完成（总纲确认 + 初始化摘要确认）

---

## 六、结构验证（阶段收尾，7 项查询并行发出）

**这 7 项查询同时就是门禁 require**——init 阶段结束前必须全部调用过，否则 set_phase("prepare") 会被拦截。一次并行发出：

| # | 验证项 | 工具 | 通过条件 |
|---|--------|------|---------|
| 1 | 角色已创建 | get_characters | characters.length > 0 |
| 2 | 地点已创建 | get_locations | locations.length >= 3 |
| 3 | 弧线已创建(含卷) | get_story_arcs | arcs.length >= 1 且 volume 弧线含 start/end_chapter |
| 4 | 世界观已创建 | get_lore | lore.length >= 1 |
| 5 | 物品已创建 | get_items | items.length >= 1 |
| 6 | 伏笔已创建 | get_timeline | timeline.pending.length >= 3 |
| 7 | 偏好已创建 | get_preferences | content 非空 |

7 项全部通过 → **set_phase("prepare")**（prepare 阶段要求 get_writing_context / get_chapter_list / get_reader_perspective / get_writing_snapshot / get_scenes 等 9 项必查，届时按 prepare 技能执行）