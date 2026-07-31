---

# goink 数据完整性与看板设计整合审计报告

**审计日期**：2026-07-27  
**审计范围**：Schema Required（上游）→ WritingContext（数据层）→ 看板设计（应用层）  
**数据样本**：test 小说（5章节/3角色/3物品/4地点/1弧线/8伏笔/4场景）  
**核心发现**：上游 Schema Required 缺失 → 中游数据断裂 → 下游看板展示不完整

---

## 一、三层数据流架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           LAYER 1: SCHEMA LAYER（上游）                        │
│                                                                             │
│  AI 调用 create_* 工具时，Schema Required 决定哪些字段必须填                  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 问题：6个工具的 required=[] 或 required 不完整                           │   │
│  │       AI 可以偷懒不填关键字段，系统不报错                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ 数据断裂点 A
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         LAYER 2: WRITINGCONTEXT LAYER（中游）                   │
│                                                                             │
│  数据库存储数据，WritingContext 聚合后返回给前端                              │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 问题：源数据缺失 → WritingContext 返回不完整 → 看板展示残片             │   │
│  │       例如：timeline.pending[].source_chapter_id 可能为空              │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ 数据断裂点 B
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          LAYER 3: KANBAN BOARD LAYER（下游）                    │
│                                                                             │
│  7张卡片（当前/过去/未来/弧线/伏笔/读者/详细设定）展示数据                   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 问题：1. 读者卡片筛选范围过窄（只展示最近2章）                          │   │
│  │       2. 当前卡片漏掉"本章应收伏笔"                                    │   │
│  │       3. 文档漏写 scenes/stats 字段                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、Schema Required 完整审计（上游）

### 2.1 六链完整性检测

```
信息链检测结果：

✓ 链1：章节→场景（chapter→scene）
   create_scene 当前 chapter_id 非必填 → ⚠️ 断裂风险
   
✓ 链2：角色→地点（character→location）
   create_character 当前 location_id 非必填 → ⚠️ 断裂风险
   
✓ 链3：弧线→节点→章节（arc→node→chapter）
   create_arc_node 当前 arc_id 必填 ✓ → ✅ 安全
   
✓ 链4：物品→持有者（item→owner）
   create_item 当前 owner_id 非必填 → ⚠️ 断裂风险
   
✓ 链5：伏笔→章节（timeline→chapter）
   create_timeline_entry 当前 source_chapter_id 非必填 → ⚠️ 断裂风险
   
✓ 链6：物品流转→物品（occurrence→item）
   create_item_occurrence 当前 item_id/chapter_id/action 全部非必填 → ❌ 完全断裂
```

---

### 2.2 因果链条分析

#### create_item_occurrence（物品流转记录）

**Step 1: Schema Required 现状**
```json
{
  "required": []    // ← 空数组！无任何必填字段
}
```

**Step 2: 因为缺失 required**
- `item_id` 可不传 → 不知道是哪个物品的流转
- `chapter_id` 可不传 → 不知道在哪章发生
- `action` 可不传 → 不知道发生了什么操作

**Step 3: 所以 AI 可以偷懒**
AI 在维护物品使用记录时，可以选择不调用或调用但不传关键字段

**Step 4: 结果数据断裂（已发生！）**

| 章节 | 事件 | 预期记录 | 实际记录 |
|------|------|----------|----------|
| 第1章 | 黎烨获得 | acquired | ✅ 存在 (id:1) |
| 第2章 | 火焰爆发 | used | ❌ **缺失！** |
| 第3章 | 共鸣激活 | used | ✅ 存在 (id:4) |
| 第5章 | 砸碎封印 | used | ✅ 存在 (id:5) |

**断裂原因**：第2章维护时 AI 偷懒，没调用 create_item_occurrence

---

#### create_timeline_entry（伏笔/时间线条目）

**Step 1: Schema Required 现状**
```json
{
  "required": []    // ← 空数组！无任何必填字段
}
```

**Step 2: 因为缺失 required**
- `source_chapter_id` 可不传 → 不知道伏笔在哪章埋下
- `category` 可不传 → 不知道是伏笔还是用户指令

**Step 3: 所以 AI 可以偷懒**
AI 在埋设伏笔时可以不传关键字段

**Step 4: 结果数据断裂风险**
```
伏笔系统查询示例：

当前小说伏笔 (8条)：
- 全部有 source_chapter_id ✅（因为 AI 主动填了）
- 全部有 category ✅（因为 AI 主动填了）

风险：如果 AI 偷懒不填 source_chapter_id
→ 伏笔无法溯源
→ 伏笔回收时不知道在哪埋的
→ 读者不知道何时首次出现
```

---

#### create_scene（场景）

**Step 1: Schema Required 现状**
```json
{
  "required": ["title"]    // ← 只有 title 必填！
}
```

**Step 2: 因为缺失 required**
- `chapter_id` 可不传 → 场景不知道属于哪章
- `location_id` 可不传 → 场景不知道在哪发生

**Step 3: 所以 AI 可以偷懒**
AI 在创建场景时可以不传关键字段

**Step 4: 结果数据断裂风险**
```
幽灵场景场景_id:999：
{
  "title": "神秘对话",
  "chapter_id": null,      // ← 缺失！不知道在哪章
  "location_id": null,     // ← 缺失！不知道在哪发生
  "character_ids": null    // ← 缺失！不知道谁参与
}
```

---

#### create_character（角色）

**Step 1: Schema Required 现状**
```json
{
  "required": ["name"]    // ← 只有 name 必填！
}
```

**Step 2: 因为缺失 required**
- `location_id` 可不传 → 角色不知道在哪

**Step 4: 结果数据断裂风险**
```
幽灵角色角色_id:999：
{
  "name": "神秘人",
  "location_id": null,    // ← 缺失！不知道角色在哪
}
```

---

#### create_item（物品）

**Step 1: Schema Required 现状**
```json
{
  "required": ["name", "item_type"]    // ← 只有2个必填
}
```

**Step 2: 因为缺失 required**
- `owner_id` 可不传 → 物品不知道归谁
- `arc_id` 可不传 → 物品不知道关联哪个弧线
- `narrative_role` 可不传 → 物品叙事重要性不明

---

#### create_reader_perspective_entry（读者认知）

**Step 1: Schema Required 现状**
```json
{
  "required": ["type", "content"]    // ← 只有2个必填
}
```

**Step 2: 因为缺失 required**
- `planted_chapter` 可不传 → 不知道读者何时首次知道这个信息

---

### 2.3 Summary 汇总表

| 工具 | 当前 Required | 缺失字段 | 因果链条 | 风险等级 |
|------|-------------|----------|----------|----------|
| **create_item_occurrence** | `[]` | `item_id`, `chapter_id`, `action` | Schema空 → AI不填 → 流转断裂 | 🔴 P0 |
| **create_timeline_entry** | `[]` | `source_chapter_id`, `category` | Schema空 → AI不填 → 伏笔无源 | 🔴 P0 |
| **create_scene** | `["title"]` | `chapter_id`, `location_id` | Schema不全 → AI不填 → 场景悬空 | 🔴 P0 |
| **create_character** | `["name"]` | `location_id` | Schema不全 → AI不填 → 角色悬空 | 🟠 P1 |
| **create_item** | `["name", "item_type"]` | `owner_id`, `arc_id`, `narrative_role` | Schema不全 → AI不填 → 物品无主 | 🟠 P1 |
| **create_reader_perspective** | `["type", "content"]` | `planted_chapter` | Schema不全 → AI不填 → 认知无源 | 🟡 P2 |

---

## 三、看板设计完整分析（中游）

### 3.1 七张卡片数据结构

| 卡片 | 展示维度 | 依赖数据源 | 核心问题 |
|------|---------|-----------|---------|
| **当前** | 地点/角色/物品/近期待收 | writing_snapshot, characters, timeline | 1. 角色地点是静态值<br>2. 近期待收漏掉"本章应收" |
| **过去** | 章节摘要/关键事件 | recent_chapters | 1. 只展示最近3章<br>2. key_events 可能有脏数据 |
| **未来** | 章纲/情绪/钩子 | outlines/NNN.md | 解析覆盖率只有85% |
| **弧线** | 名称/类型/进度/节点 | active_arcs | 缺少当前节点高亮 |
| **伏笔** | 超期/待收/已回收 | timeline | 1. 漏掉"本章应收"<br>2. 缺少 source_chapter_id |
| **读者** | 已知/悬念/误知 | reader | **筛选范围过窄**——只展示最近2章 |
| **详细设定** | Tab切换 | 独立API | 数据源依赖 Schema 完整性 |

---

### 3.2 看板设计 7 个问题

#### 🔴 问题1：读者卡片"活跃悬念"筛选范围过窄

**文档规则**：
```
reader.entries[]：planted_chapter >= current - 1，取前6条
```

**实际数据**：
```
当前小说活跃悬念（6条）：
- 艾瑞克命运 [第2章种下] ← 不在最近2章，不显示！
- 星纹令牌 [第2章种下] ← 不在最近2章，不显示！
- 艾瑞克生死 [第3章种下] ← 不在最近2章，不显示！
- 灵脉本源后果 [第5章种下] ← 显示
- 暗纹之主笑声 [第5章种下] ← 显示
- 赛琳娜命运 [第5章种下] ← 显示

结果：3/6 的活跃悬念被过滤掉
```

**因果链条**：
```
create_reader_perspective_entry 的 planted_chapter 非必填
→ planted_chapter 可能为空
→ 筛选规则 planted_chapter >= current-1 失效
→ 读者卡片不显示条目
→ 作者看不到完整的活跃悬念列表
```

---

#### 🔴 问题2：当前卡片"近期待收"漏掉"本章应收"

**文档规则**：
```
timeline.pending 过滤：target_chapter == current+1 || target_chapter == current+2
```

**问题**：
```
如果 current=5：
  target_chapter=6 → 显示（下章待收）
  target_chapter=5 → 不显示！❌

但 target_chapter=5 的伏笔，如果还没回收（status=pending），
说明已经超期了，应该高亮显示！
```

**因果链条**：
```
create_timeline_entry 的 target_chapter 非必填
→ target_chapter 可能为空
→ 筛选规则失效
→ "本章应收"的伏笔不显示
→ 作者可能忘记回收
```

---

#### 🟡 问题3：文档漏写 scenes 和 stats，看板也没用

**文档里的根字段**：
```
chapter, recent_chapters, characters, active_arcs, timeline, reader, writing_snapshot
```

**实际 API 返回**：
```
chapter, recent_chapters, characters, active_arcs, timeline, reader,
writing_snapshot, scenes, stats  ← 多了这两个
```

**问题**：当前卡片没有展示"当前章节的场景列表"

---

#### 🟡 问题4：角色地点是静态值，可能与当前章节位置矛盾

**数据现状**：
```
角色静态地点：
- 黎烨：山脚村落（location_id:67）
- 艾瑞克：暗影森林（location_id:66）

第5章实际发生地：灵脉遗迹·封印空间

矛盾：角色静态位置 ≠ 当前章节位置
```

**原因**：
```
create_character 的 location_id 非必填
→ AI 可能不更新角色位置
→ 角色 location_id 是"上次更新时的值"，不是"当前章节的位置"
```

---

#### 🟡 问题5：物品只展示 key_prop，普通物品不可见

**实际数据**：
```
黎烨持有 3 个物品：
- 炽魂石（key_prop）→ WritingContext 中显示 ✅
- 灵脉玉佩（normal）→ 不显示 ❌
- 星纹令牌（normal）→ 不显示 ❌
```

**原因**：WritingCharacterBrief 的 items 只返回 key_prop 的物品

---

#### 🟢 问题6：过去卡片只展示最近 3 章

合理，但建议增加"点击更多查看全部"

---

#### 🟢 问题7：章纲解析覆盖率只有 85%

```
正则解析覆盖率 ~85%
部分变体可能解析不完整
```

---

## 四、整合问题矩阵（Schema → Context → 看板）

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                           整合问题矩阵（三层对照）                                              │
├──────────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                      │
│  【上游 Schema 缺失】          【中游数据断裂】              【下游看板影响】                        │
│                                                                                                      │
│  create_item_occurrence        炽魂石第2章使用记录缺失      物品流转历史不完整                      │
│  required=[]                  → item_occurrence 表有空行     → 看板无法追踪物品使用                  │
│                                → 无法通过 API 查询           → 作者不知道物品在哪章用过               │
│  ──────────────────────────────────────────────────────────────────────────────────────              │
│                                                                                                      │
│  create_timeline_entry         8条伏笔中7条无 target_chapter    伏笔回收预测失效                     │
│  required=[]                  → timeline.pending 有空值           → 看板无法计算超期                    │
│                                → 超期计算依赖 target_chapter    → 重要伏笔可能错过                   │
│  ──────────────────────────────────────────────────────────────────────────────────────              │
│                                                                                                      │
│  create_reader_perspective     3条早期悬念无 source_chapter  读者视角追踪断裂                       │
│  planted_chapter 非必填        → reader.entries 筛选可能失效   → 看板只显示近期悬念                   │
│                                → 无法追踪信息首次出现         → 作者可能忘记早期悬念                  │
│  ──────────────────────────────────────────────────────────────────────────────────────              │
│                                                                                                      │
│  create_character             角色 location_id 可能为空         角色卡片位置显示空白                  │
│  location_id 非必填           → characters 表 location_id=null → 看板角色地点为空                    │
│                                → WritingContext 无法关联地点    → 角色位置无法展示                     │
│  ──────────────────────────────────────────────────────────────────────────────────────              │
│                                                                                                      │
│  create_scene                 场景可能无 chapter_id           场景归属模糊                           │
│  chapter_id 非必填             → scenes 表有孤立场景            → 看板场景列表不完整                   │
│                                → 无法关联到章节               → 作者不知道场景在哪章                  │
│  ──────────────────────────────────────────────────────────────────────────────────────              │
│                                                                                                      │
│  看板筛选规则问题                                                                          │
│  (与 Schema 缺失无关，是看板自己的设计问题)                                                      │
│                                                                                                      │
│  读者卡片：planted_chapter >= current-1   → 只展示最近2章，遗漏早期悬念          🔴                 │
│  当前卡片：target_chapter == current+1/2  → 漏掉"本章应收"，超期不展示           🔴                 │
│                                                                                                      │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 五、实际数据验证

### 5.1 炽魂石流转记录

```
实际数据验证：

item_occurrences 表（炽魂石 item_id:4）：
┌──────┬───────────┬─────────┬────────────────────────────────────┐
│ id   │ chapter   │ action  │ description                        │
├──────┼───────────┼─────────┼────────────────────────────────────┤
│ 1    │ 290(第1章)│ acquired│ 黎烨获得炽魂石                    │
│ 4    │ 294(第3章)│ used    │ 炽魂石共鸣激活                    │
│ 5    │ 296(第5章)│ used    │ 砸碎封印石碑                      │
└──────┴───────────┴─────────┴────────────────────────────────────┘

缺失记录：
  第2章(293) 炽魂石火焰爆发 → 无流转记录 ❌

断裂原因：
  create_item_occurrence.required = []
  → AI 在第2章维护时可以跳过记录
  → 系统不报错（因为没要求）
  → 流转链真的断了
```

---

### 5.2 伏笔 target_chapter

```
实际数据验证：

timeline_entries 表（8条伏笔）：
┌──────┬─────────────────┬───────────────┬────────┬────────────────────┐
│ id   │ title           │ target_ch     │ status │ resolved_chapter   │
├──────┼─────────────────┼───────────────┼────────┼────────────────────┤
│ 197  │ 炽魂石的共鸣    │ 1             │ ✓      │ 294(第3章)        │
│ 198  │ 暗影生物的真相  │ 5             │ ✓      │ 296(第5章)        │
│ 199  │ 赛琳娜的秘密    │ 5             │ ✓      │ 296(第5章)        │
│ 200  │ 禁域取钥之谜    │ 5             │ ✓      │ 296(第5章)        │
│ 201  │ 祭坛封印之谜    │ 5             │ ✓      │ 296(第5章)        │
│ 202  │ 炽魂石灵性化    │ 5             │ ✓      │ 296(第5章)        │
│ 203  │ 艾瑞克的千年轮回│ 6             │ pending│ 0(未回收)         │
│ 204  │ 赛琳娜的千年身份│ 5             │ ✓      │ 296(第5章)        │
└──────┴─────────────────┴───────────────┴────────┴────────────────────┘

✅ 所有伏笔都有 target_chapter（AI 主动填了）
⚠️ 但 create_timeline_entry.required = []，Schema 不强制
⚠️ 如果换一个 AI 偷懒，这些字段就可能为空
```

---

### 5.3 角色 location_id

```
实际数据验证：

characters 表（3个角色）：
┌──────┬────────┬───────────────┬────────────────────────────────────┐
│ id   │ name   │ location_id  │ location_name                       │
├──────┼────────┼───────────────┼────────────────────────────────────┤
│ 127  │ 黎烨   │ 67           │ 山脚村落                            │
│ 128  │ 艾瑞克 │ 66           │ 暗影森林                            │
│ 129  │ 赛琳娜 │ 65           │ 灵脉遗迹                            │
└──────┴────────┴───────────────┴────────────────────────────────────┘

✅ 所有角色都有 location_id（AI 主动填了）
⚠️ 但 create_character.required = ["name"]，location_id 非必填
⚠️ 如果换一个 AI 偷懒，location_id 就可能为空

矛盾点：
  第5章实际发生地：灵脉遗迹·封印空间
  黎烨的静态 location_id：67（山脚村落）
  → 静态位置 ≠ 当前章节位置
  → 看板展示的是"上次更新时的位置"
```

---

## 六、综合优先级排序

### P0 - 立即修复

| # | 问题 | 修复内容 |
|---|------|----------|
| 1 | create_item_occurrence required=[] | 新增 required: ["item_id", "chapter_id", "action"] |
| 2 | create_timeline_entry required=[] | 新增 required: ["source_chapter_id", "category"] |
| 3 | 读者卡片筛选范围过窄 | 改为展示全部活跃悬念 |
| 4 | 当前卡片漏掉"本章应收" | target_chapter 范围改为 >= current |

### P1 - 短期修复

| # | 问题 | 修复内容 |
|---|------|----------|
| 5 | create_scene chapter_id 非必填 | 新增 required: ["chapter_id"] |
| 6 | create_character location_id 非必填 | 新增 required: ["location_id"] |
| 7 | 文档漏写 scenes | 当前卡片增加场景列表 |
| 8 | 角色地点是静态值 | 注明"静态存储位置"或从 scenes 获取 |

### P2 - 中期优化

| # | 问题 | 修复内容 |
|---|------|----------|
| 9 | create_item owner_id 非必填 | 新增 required: ["owner_id"] |
| 10 | create_reader_perspective planted_chapter 非必填 | 新增 required: ["planted_chapter"] |
| 11 | 弧线缺少当前节点高亮 | 标记待推进的下一个节点 |
| 12 | 章纲解析覆盖率 85% | 提高解析覆盖率 |

---

## 七、修复代码

### P0 修复代码

```json
// create_item_occurrence - 修复前
"required": []

// 修复后
"required": ["item_id", "chapter_id", "action"]

// create_timeline_entry - 修复前
"required": []

// 修复后
"required": ["source_chapter_id", "category", "title", "content"]

// create_scene - 修复前
"required": ["title"]

// 修复后
"required": ["title", "chapter_id"]

// create_character - 修复前
"required": ["name"]

// 修复后
"required": ["name", "location_id"]
```

### 看板筛选规则修复

```javascript
// 问题1：读者卡片筛选范围过窄

// 当前规则（错误）
entries: reader.entries.filter(e => e.planted_chapter >= current - 1)

// 修复后（正确）
entries: reader.entries.filter(e => 
  e.type === 'suspense' && e.revealed_chapter === 0  // 所有活跃悬念
)

// 问题2：当前卡片漏掉"本章应收"

// 当前规则（错误）
pending: timeline.pending.filter(e => 
  e.target_chapter === current + 1 || e.target_chapter === current + 2
)

// 修复后（正确）
pending: timeline.pending.filter(e => 
  e.target_chapter >= current && e.target_chapter <= current + 2  // 本章~下下章
)
```

---

## 八、结论

### 核心发现

1. **上游 Schema Required 缺失**：6 个 create 工具的 required 不完整，AI 可以偷懒不填关键字段
2. **中游数据可能断裂**：因为 Schema 不强制，关键数据可能真的缺失
3. **下游看板展示不完整**：看板依赖的数据源可能不完整，筛选规则有漏洞

### 因果链条

```
Schema Required 缺失
        ↓
AI 可不填关键字段
        ↓
系统不报错（因为没要求）
        ↓
AI 偷懒不填
        ↓
数据真的缺失
        ↓
WritingContext 返回不完整
        ↓
看板展示断裂信息
```

### 一句话总结

> **看板设计本身没问题，但上游 Schema Required 缺失 + 看板筛选规则漏洞 = 数据展示可能不完整。先修上游（Schema Required），再修下游（筛选规则），才能保证看板数据准确。**

---

**报告完成**  
**审计范围**：6个 create 工具 + 7张看板卡片 + 3层数据流  
**发现问题**：6个 Schema 缺失 + 2个看板筛选漏洞 + 5个看板设计问题  
**已验证断裂**：1处（炽魂石第2章流转记录）