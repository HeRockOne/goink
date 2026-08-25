# 创作测试会话审计指南

> 本文档供 AI 模型在新会话中快速接手创作测试审计工作。按步骤执行即可完成全架构交叉验证。

---

## 一、数据库基础

数据库路径：`D:\Goink\novel-agent.db`（SQLite）。用 `sqlite3` 命令行查询。

### 核心表结构

| 表名 | 主键 | 关键列 | 说明 |
|------|------|--------|------|
| `sessions` | `session_id TEXT` | novel_id, model, created_at, current_phase, total_tokens, total_cost | 每次创作会话 |
| `messages` | `id INTEGER` | session_id, role(system/assistant/user/tool), content, extra_metadata(JSON), token_count, duration_ms, model | 全部对话消息 |
| `chapters` | `id INTEGER`(PK≠chapter_number) | novel_id, chapter_number, title, summary, key_events, content_length, characters_in | 章节正文元数据 |
| `review_records` | `id INTEGER` | novel_id, session_id, chapter_start, chapter_end, total_score, verdict(pass/revise/fail/unknown), fatal_count, dim_structure/character/pacing/prose/scene, instruction, report, created_at | 审稿落库 |
| `model_usage` | 复合 | session_id, model_id, hit_tokens, miss_tokens, completion_tokens | 模型级 token 累计 |
| `volumes` | `id INTEGER` | novel_id, name, start_chapter, end_chapter | 卷范围 |
| `outline_beats` | `id INTEGER` | novel_id, chapter, description | 大爽点规划 |
| `timeline_entries` | `id INTEGER` | novel_id, status(pending/resolved), target_chapter, resolved_chapter_id, description | 伏笔/悬念 |
| `story_arcs` / `arc_nodes` | id | novel_id, arc_id, chapter_number, status(pending/completed), actual_chapter | 弧线进度 |
| `characters` | `id INTEGER` | novel_id, name, status(alive/dead), personality, abilities | 角色设定 |
| `preferences` | `id INTEGER` | novel_id, is_global, category, content, status(active/inactive) | 创作偏好/禁忌 |
| `app_config` | `key TEXT` | value(JSON) | 全局配置（含字数范围等） |

### 重要：PK ≠ 逻辑编号

- `chapters.id` 是自增 PK，**不等于** `chapter_number`。例如 PK=31 可能对应 chapter_number=1。
- `resolved_chapter_id` 存的是 `chapters.id`（PK），不是 chapter_number。需 JOIN chapters 表转换。
- 查询某章正文用 `chapter_number`，查询伏笔回收用 `resolved_chapter_id`（PK）。

---

## 二、审计流程（按顺序执行）

### 第1步：定位目标会话

```sql
-- 查最新会话
SELECT session_id, model, created_at, current_phase, total_tokens, 
       ROUND(total_cost, 4) as cost
FROM sessions 
WHERE novel_id = {N} 
ORDER BY created_at DESC 
LIMIT 5;

-- 查会话消息统计
SELECT role, COUNT(*) as cnt 
FROM messages 
WHERE session_id = '{session_id}' 
GROUP BY role;
```

**预期消息分布**（单章创作）：
- assistant: 30-60（含纯 thinking 无 content 的）
- tool: 50-100
- user: 10-20
- system: 8-12（NS + 技能注入 + 提醒）

### 第2步：Token 消耗与缓存效率

```sql
SELECT model_id, hit_tokens, miss_tokens, completion_tokens,
       ROUND(100.0 * hit_tokens / (hit_tokens + miss_tokens), 2) as cache_hit_pct
FROM model_usage 
WHERE session_id = '{session_id}';
```

**基准**：
- 缓存命中率：≥97% 为正常（前缀缓存机制）
- 命中率 <95%：检查是否首次对话（冷启动）或 NS 被压缩破坏

#### 缓存 Miss 深度分析

当命中率 <97% 时，按以下步骤定位 miss 来源：

**Step 1：提取 per-call token 数据**

```sql
-- 从 assistant 消息的 extra_metadata.usage 中提取每次 LLM 调用的 token 分布
SELECT id,
  json_extract(extra_metadata, '$.usage.prompt_tokens') as prompt,
  json_extract(extra_metadata, '$.usage.cached_tokens') as cached,
  json_extract(extra_metadata, '$.usage.completion_tokens') as comp,
  json_extract(extra_metadata, '$.usage.prompt_tokens') - json_extract(extra_metadata, '$.usage.cached_tokens') as miss
FROM messages
WHERE session_id = '{session_id}'
  AND role = 'assistant'
  AND json_extract(extra_metadata, '$.usage.prompt_tokens') IS NOT NULL
ORDER BY id;
```

**Step 2：重建消息序列，标记注入点**

```sql
-- 查看所有 system 消息（含注入的技能/提醒），标注出现位置
SELECT id, role,
  CASE
    WHEN content LIKE '%--- main-tech-%' OR content LIKE '%--- main-core-%' THEN 'auto_skill'
    WHEN content LIKE '%当前阶段%' THEN 'phase_reminder'
    WHEN content LIKE '%方向锚%' THEN 'direction_anchor'
    WHEN content LIKE '%小说基础信息%' THEN 'NS'
    WHEN content LIKE '%核心原则%' THEN 'identity'
    WHEN content LIKE '%available_skills%' THEN 'skill_catalog'
    ELSE 'other'
  END as type,
  length(content) as bytes,
  substr(content, 1, 80) as preview
FROM messages
WHERE session_id = '{session_id}'
  AND role = 'system'
ORDER BY id;
```

**Step 3：关联注入点与 miss 峰值**

将 Step 1 的 per-call miss 与 Step 2 的注入位置交叉：
- miss 峰值出现在注入消息之后的第一个 LLM 调用 → 注入打破了前缀
- 常见注入点（按 token 贡献排序）：
  - `auto_skill` 注入：~3-9K tokens/次（write 阶段 5 个技能拼一条 ~9K）
  - `phase_reminder` + `direction_anchor`：~2-3K tokens/次
  - `post-write instruction`：~5-7K tokens/次

**Step 4：计算理论 savings**

```
理论 savings = Σ(注入消息 tokens) × (1 - 该注入后续调用的 miss 率)
```

实际 savings 取决于注入消息是否在 LLM 调用的 prefix 中。如果注入消息在 `appendMsg` 时追加到 `opts.Messages` 末尾（历史消息和新用户消息之间），它改变了后续调用的 prefix → miss。

**已知根因**：`agent.go:1097` 的 `appendMsg` 把注入消息追加到 `opts.Messages` 末尾。注入消息位于历史消息和新用户消息之间，导致后续 LLM 调用的 prefix 改变 → 缓存 miss。

### 第3步：审稿记录

```sql
SELECT id, chapter_start, chapter_end, total_score, verdict, fatal_count,
       dim_structure, dim_character, dim_pacing, dim_prose, dim_scene,
       created_at
FROM review_records 
WHERE novel_id = {N} 
ORDER BY id;
```

**解读**：
- 5维分 = -1.0 表示 ParseReport 正则未匹配到（格式问题，非评分）
- verdict: pass(≥9.0) / revise(7.0-8.9) / fail(<7.0) / unknown(解析失败)
- fatal_count > 0：必须查看 report 原文确认致命问题
- 多轮 review 同一章：检查是否有进展（分数递增 = 修复有效）

```sql
-- 查完整审稿报告
SELECT id, total_score, verdict, report FROM review_records WHERE id = {N};
```

### 第4步：错误清单

从 messages 中提取所有工具调用失败：

```sql
-- 工具错误
SELECT id, role, content 
FROM messages 
WHERE session_id = '{session_id}' 
  AND role = 'tool' 
  AND (content LIKE '%error%' OR content LIKE '%Error%' OR content LIKE '%失败%' OR content LIKE '%拒绝%')
ORDER BY id;
```

**错误分类**（见第六节详细分类）：

| 类型 | 标志 | 占比基准 |
|------|------|---------|
| 架构正确拦截 | verdict gate / timeline rejection / phase gate require / phase gate whitelist | ~10-15% |
| 架构可修复 | edit param confusion / ledger_integrity false positive / CreateLoreArgs validation | <5% |
| 模型行为 | wrong tool / blind retry / thinking 里写正文 / word count 挤牙膏 | ~80% |

### 第5步：阶段流转

```sql
-- 所有 set_phase 调用及结果
SELECT m.id, m.content, t.content as result
FROM messages m
JOIN messages t ON t.extra_metadata LIKE '%"tool_id":"%' || json_extract(m.extra_metadata, '$.tool_calls[0].id') || '%"'
WHERE m.session_id = '{session_id}'
  AND m.role = 'assistant'
  AND m.content LIKE '%set_phase%'
ORDER BY m.id;
```

**预期阶段流**：prepare → outline → write → review → maintain → done（单章）

### 第6步：关键注入验证

```sql
-- 方向锚注入（应出现在 write 阶段前）
SELECT id, role, substr(content, 1, 500)
FROM messages 
WHERE session_id = '{session_id}' 
  AND role = 'system' 
  AND content LIKE '%方向锚%'
ORDER BY id;

-- 技能自动注入
SELECT id, length(content), substr(content, 1, 100)
FROM messages 
WHERE session_id = '{session_id}' 
  AND role = 'system' 
  AND content LIKE '%--- main-tech-%'
ORDER BY id;

-- 门禁拦截记录
SELECT id, substr(content, 1, 300)
FROM messages 
WHERE session_id = '{session_id}' 
  AND (content LIKE '%存在硬错误%' OR content LIKE '%要求必须调用%' OR content LIKE '%禁止%')
ORDER BY id;
```

### 第7步：章节数据一致性

```sql
-- 最新章节
SELECT id, chapter_number, title, content_length, characters_in, key_events
FROM chapters 
WHERE novel_id = {N} 
ORDER BY chapter_number DESC 
LIMIT 3;

-- 伏笔状态（查异常）
SELECT id, status, target_chapter, resolved_chapter_id, description
FROM timeline_entries 
WHERE novel_id = {N} 
  AND status = 'resolved'
  AND resolved_chapter_id IS NOT NULL;

-- 弧线节点
SELECT an.id, sa.name as arc_name, an.chapter_number, an.status, an.actual_chapter
FROM arc_nodes an
JOIN story_arcs sa ON sa.id = an.story_arc_id
WHERE sa.novel_id = {N}
ORDER BY an.chapter_number;
```

**异常检测**：
- `resolved_chapter_id` > 最大 chapter_number → 数据错位（需 JOIN chapters 转换）
- arc_node status=completed 但 actual_chapter=0 → 语义失真

---

## 三、6 层架构闭环审计

每层都要从 DB 中找到证据。无证据 = 未生效。

### 第1层：检测层

| 功能 | DB 证据 | 查询 |
|------|---------|------|
| 方向锚注入 | system 消息含"方向锚" | `content LIKE '%方向锚%'` |
| 11 类一致性检查 | review 报告中引用 check_story_consistency 结果 | review_records.report |
| 审稿标准 #26/#27 | review 报告中提到"卷纲范围"或"类型方向" | review_records.report |
| 审稿记录落库 | review_records 表有记录 | `SELECT COUNT(*) FROM review_records WHERE novel_id={N}` |
| /ruling 偏好沉淀 | preferences 表有新禁忌 | `SELECT * FROM preferences WHERE category LIKE '%禁忌%' ORDER BY id DESC` |
| 门禁关闭提醒 | messages 中含"审稿缺席" | `content LIKE '%审稿%缺失%'` |

### 第2层：代码执行层

| 功能 | DB 证据 | 查询 |
|------|---------|------|
| 审稿结论门控 | set_phase 返回"不通过"阻断 | messages 中 tool role 含"不通过" |
| 伏笔拒绝 | timeline 工具返回"拒绝" | messages 中 tool role 含"拒绝" |
| 门禁 require 拦截 | set_phase 返回"要求必须调用" | messages 中 tool role 含"要求必须调用" |
| 门禁 whitelist 拦截 | 工具调用在禁止阶段被拒 | messages 中 tool role 含"禁止" |

### 第3层：LLM 提示词层

| 功能 | DB 证据 | 查询 |
|------|---------|------|
| 方向锚执行 | write 阶段正文遵守卷范围 | 正文内容 vs volumes 范围 |
| 一致性解读 | review 报告引用 SQL 检查结果 | review_records.report |
| 审稿判定 | review 报告有评分和判定 | review_records.verdict != 'unknown' |
| 修复循环 | 多轮 review 分数递增 | review_records 总分趋势 |
| 写前方向锚 | write 阶段前系统消息含方向锚 | messages system role |
| 维护差量提示 | maintain 阶段前系统消息含"新实体" | messages system role |

### 第4层：缓存层

| 功能 | DB 证据 | 查询 |
|------|---------|------|
| 命中率 | model_usage.hit_tokens 占比 | `hit/(hit+miss)` |
| NS 缓存稳定 | NS 消息在 messages 中存在 | `content LIKE '%小说基础信息%'` |

### 第5层：技能注入层

| 功能 | DB 证据 | 查询 |
|------|---------|------|
| write 阶段自动注入 | system 消息含 5 个技能 | `content LIKE '%word-count-calibration%'` |
| prepare 阶段注入 | system 消息含 common-sense-logic | `content LIKE '%common-sense-logic%'` |
| review 子代理注入 | review 报告引用 27 项标准 | review_records.report |

### 第6层：工具自描述层

| 功能 | DB 证据 | 查询 |
|------|---------|------|
| check_story_consistency 11 类 | 工具返回结果含各类型 | messages tool role |
| get_review_history 可用 | 有记录可查 | `SELECT COUNT(*) FROM review_records` |

---

## 四、4 会话对比模板

用于跟踪跨会话趋势：

```
| 指标 | 会话1 | 会话2 | 会话3 | 会话4 |
|------|-------|-------|-------|-------|
| 模型 | | | | |
| 消息数 | | | | |
| token 总量 | | | | |
| 缓存命中率 | | | | |
| 错误数 | | | | |
| - 架构拦截 | | | | |
| - 架构可修 | | | | |
| - 模型行为 | | | | |
| Review 轮次 | | | | |
| Review 分数 | | | | |
| Review 致命数 | | | | |
| 耗时 | | | | |
```

---

## 五、常见陷阱

1. **chapter PK ≠ chapter_number**：所有 resolved_chapter_id 存的是 chapters.id，不是 chapter_number。查伏笔回收范围时必须 JOIN。
2. **review_records 的 dim_scores = -1.0**：ParseReport 正则未匹配到评分，看 report 原文即可。
3. **空 content 的 assistant 消息**：纯 thinking + tool call，thinking 在 extra_metadata 或单独字段中，content 为空是正常的。
4. **gate-off reminder 不入库**：门禁关闭提醒是 streaming system-reminder，不持久化到 messages 表。无法从 DB 审计。
5. **auto_skill_injection 看起来只有1个技能**：实际5个技能拼在一条 system 消息中（用 `LIKE` 逐个检查），不要用 `content LIKE '%main-tech%'` 只匹配第一个。
6. **set_phase 失败不等于架构问题**：require 拦截（缺工具调用）和 whitelist 拦截（禁止阶段调用）是正确的架构行为，模型应自修正。
7. **word count 挤牙膏**：模型多次扩写（2018→2286→2323→2364→2421），是模型未遵循"一次扩到位"规则的行为问题，非架构缺陷。

---

## 六、错误分类决策树

```
错误发生
├── 工具返回"不通过" → verdict gate 正确拦截 → 架构正确
├── 工具返回"拒绝" → timeline rejection 正确 → 架构正确
├── set_phase 返回"要求必须调用" → require 拦截 → 模型漏调工具 → 模型行为
├── set_phase 返回"禁止" → whitelist 拦截 → 模型调错工具 → 模型行为
├── 工具返回参数错误
│   ├── edit old_content → 已修复 alias → 已解决
│   ├── create_lore JSON 格式 → 已修复 batch → 已解决
│   └── 其他参数 → 模型行为
├── check_story_consistency ERROR → 查具体类型
│   ├── ledger_integrity → 已修复 JOIN → 已解决
│   ├── foreshadow_overdue → 伏笔真实超期 → 架构正确
│   └── 其他 → 真实设定问题
└── word count 不达标 → 模型挤牙膏 → 模型行为
```

---

## 七、字数分布参考

用户设置的字数范围通过 NS 注入（`internal/agentcfg/novel_state.go`），单一来源 `app_config`。各平台参考：

| 平台 | 推荐字数 | 备注 |
|------|---------|------|
| 番茄小说 | 2000-2500/章 | 90% 爆款在此区间 |
| 起点中文网 | 2000-3000/章 | 玄幻/仙侠 3800-4500 |
| 七猫 | 1000-1500 首章 | |
| 通用完读率最高 | 3000-5000 | 完读率 79% |
| webnovel-writer 参考 | 每 1000 字推进 1 情节点 | |

Goink 当前默认：2500-4000 字/章（可由用户在设置中自定义）。

---

## 八、完整架构能力清单

### 检测能力（11 类 SQL 检查 + 27 项审稿标准）

| 检查 | 来源 | 类型 |
|------|------|------|
| foreshadow_overdue | check_story_consistency | SQL:伏笔超期 |
| character_vanished | check_story_consistency | SQL:角色消失 |
| item_conflict | check_story_consistency | SQL:物品冲突 |
| dead_appeared | check_story_consistency | SQL:死者出场 |
| pacing_gap | check_story_consistency | SQL:节奏拖沓 |
| promise_fulfillment | check_story_consistency | SQL:承诺兑现 |
| init_consistency | check_story_consistency | SQL:开书一致性 |
| ledger_integrity | check_story_consistency | SQL:台账防腐(已修复JOIN) |
| beat_window | check_story_consistency | SQL:爽点窗口 |
| scope_guard | check_story_consistency | SQL:卷范围守卫 |
| type_drift | check_story_consistency | SQL:类型漂移 |
| 审稿标准 #1-#27 | review subagent | 人工审读(含 #26 卷纲越界/#27 类型契合) |

### 执行能力（代码级强制）

| 执行点 | 代码位置 | 行为 |
|--------|---------|------|
| verdict gate | phase_gate.go:checkResultGateMet | 不通过 → 阻断 set_phase |
| timeline rejection | timeline_tools.go:Execute | resolved_chapter > maxCh → 拒绝 |
| phase gate require | phase_gate.go:checkRequireMet | 缺工具调用 → 阻断 |
| phase gate whitelist | phase_gate.go:checkWhitelist | 禁止阶段调用 → 阻断 |
| /ruling → preference | main-cmd-ruling skill | 用户纠偏 → create_preference |
| edit old_content | rw_tools.go:applyChange | SearchText fallback |

### 约束能力（提示词级）

| 约束 | 来源 | 覆盖范围 |
|------|------|---------|
| 方向锚 | NS 每轮注入 | 本卷红线 + 类型承诺 + 未兑现爽点 + 活跃禁忌 |
| kernel 硬约束 | always 注入 | 字数范围 + 段数 + 节奏 + 伏笔回收率 |
| 写前方向锚 | write 阶段注入 | 方向锚全文 + 写前默读 |
| 维护差量提示 | maintain 阶段注入 | 新实体检查指令 |
| 审稿标准 | review 子代理注入 | 27 项逐条检查 |
| 门禁关闭提醒 | 阈值触发 | 方向锚 + 审稿提醒 |

---

## 九、快速审计脚本（复制即用）

将以下脚本保存为 `.sql` 文件，用 `sqlite3 D:\Goink\novel-agent.db < audit.sql` 执行。替换 `{SESSION_ID}` 和 `{NOVEL_ID}`。

```sql
-- === 基础信息 ===
SELECT '会话信息' as section;
SELECT session_id, model, created_at, current_phase, total_tokens
FROM sessions WHERE session_id = '{SESSION_ID}';

SELECT '消息分布' as section;
SELECT role, COUNT(*) FROM messages WHERE session_id = '{SESSION_ID}' GROUP BY role;

SELECT 'Token 消耗' as section;
SELECT model_id, hit_tokens, miss_tokens, completion_tokens,
       ROUND(100.0*hit_tokens/(hit_tokens+miss_tokens),2) as cache_pct
FROM model_usage WHERE session_id = '{SESSION_ID}';

-- === 审稿记录 ===
SELECT '审稿记录' as section;
SELECT id, chapter_start, chapter_end, total_score, verdict, fatal_count,
       dim_structure, dim_character, dim_pacing, dim_prose, dim_scene
FROM review_records WHERE novel_id = {NOVEL_ID} ORDER BY id;

-- === 错误统计 ===
SELECT '工具错误' as section;
SELECT COUNT(*) as error_count FROM messages
WHERE session_id = '{SESSION_ID}' AND role = 'tool'
  AND (content LIKE '%error%' OR content LIKE '%Error%' OR content LIKE '%失败%' OR content LIKE '%拒绝%');

-- === 架构验证 ===
SELECT '方向锚注入' as section;
SELECT COUNT(*) FROM messages WHERE session_id = '{SESSION_ID}' AND role='system' AND content LIKE '%方向锚%';

SELECT '技能注入' as section;
SELECT COUNT(*) FROM messages WHERE session_id = '{SESSION_ID}' AND role='system' AND content LIKE '%--- main-tech-%';

SELECT '门禁拦截' as section;
SELECT COUNT(*) FROM messages WHERE session_id = '{SESSION_ID}' AND role='tool'
  AND (content LIKE '%要求必须调用%' OR content LIKE '%存在硬错误%' OR content LIKE '%不通过%');

-- === 缓存 Miss 分析 ===
SELECT 'Per-call token 数据' as section;
SELECT id,
  json_extract(extra_metadata, '$.usage.prompt_tokens') as prompt,
  json_extract(extra_metadata, '$.usage.cached_tokens') as cached,
  json_extract(extra_metadata, '$.usage.completion_tokens') as comp,
  json_extract(extra_metadata, '$.usage.prompt_tokens') - json_extract(extra_metadata, '$.usage.cached_tokens') as miss
FROM messages
WHERE session_id = '{SESSION_ID}'
  AND role = 'assistant'
  AND json_extract(extra_metadata, '$.usage.prompt_tokens') IS NOT NULL
ORDER BY id;

SELECT '注入消息分布' as section;
SELECT id,
  CASE
    WHEN content LIKE '%--- main-tech-%' OR content LIKE '%--- main-core-%' THEN 'auto_skill'
    WHEN content LIKE '%当前阶段%' THEN 'phase_reminder'
    WHEN content LIKE '%方向锚%' THEN 'direction_anchor'
    WHEN content LIKE '%小说基础信息%' THEN 'NS'
    ELSE 'other'
  END as type,
  length(content) as bytes
FROM messages
WHERE session_id = '{SESSION_ID}' AND role = 'system' ORDER BY id;

-- === 最新章节 ===
SELECT '最新章节' as section;
SELECT id, chapter_number, title, content_length FROM chapters
WHERE novel_id = {NOVEL_ID} ORDER BY chapter_number DESC LIMIT 3;
```

---

*最后更新：2026-08-26。基于7轮创作测试（dots3/mimo-v2.5×3/MiniMax-M2.7×2）的审计经验编写。新增缓存 miss 深度分析流程。*
