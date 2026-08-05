# MCP Tools 依赖链审计

> 审计日期：2026-07-25
> 审计目标：确保所有写入工具的必填字段覆盖 `get_writing_context` 依赖链，防止 AI 偷懒导致数据缺失。

## 1. 数据管线

```
prepare(get_writing_context) → outline(edit outlines/)
→ write(edit chapters/) → review(run_subagent)
→ maintain(update_*/create_* + update_chapter_meta + update_writing_snapshot + search_lore + search_items)
→ 回到 prepare → 读到 maintain 回写的最新数据
```

每个阶段的输入/输出/闭环：

| 阶段 | 读什么 | 写什么 | 闭环 |
|------|--------|--------|------|
| prepare | get_writing_context (DB 最新状态) | 无 | → outline 知道当前状态 |
| outline | read(goink.md) 指纹 + read(skills/) | edit(outlines/NNN.md) | → write 有章纲 |
| write | read(outlines/NNN.md) | edit(chapters/NNN.md) | → review 有全文 |
| review | read(chapters/NNN.md) + get_* | run_subagent 报告 | → maintain 知道要修什么 |
| maintain | get_writing_context + get_* | update_*/create_* + edit(goink.md)(append) + update_chapter_plan + update_writing_snapshot + update_chapter_meta + set_phase | → 下轮 prepare 读到最新状态 |

无断层。maintain 写回 DB → 下轮 get_writing_context 读到最新数据 → 闭环。

## 2. get_writing_context 依赖链

`current_chapter` 为必填参数，作为根节点。返回结构：

```json
{
  "chapter":       {"num":75, "title":"夜入皇城", "word_count":3500},
  "recent_chapters": [{"num":74, "title":"暗流涌动", "summary":"...", "key_events":["..."], "word_cnt":3200}],
  "scenes":        [{"id":1, "title":"城门对峙", "location":{"name":"皇城"}, "arc_node":{"title":"潜入", "arc_name":"复仇之路"}}],
  "characters":    [{"id":1, "name":"主角", "location":{"name":"皇城"}, "items":[{"name":"灵脉玉佩", "role":"key_prop"}]}],
  "active_arcs":   [{"id":1, "name":"复仇之路", "type_zh":"主线", "nodes_total":5, "nodes_done":3, "related_lore":[3,7], "related_items":[4]}],
  "timeline":      {"pending":[...], "resolved":[...]},
  "reader":        {"known":12, "suspense":5, "misconception":2},
  "writing_snapshot": {"last_chapter_num":74, "current_arc_id":1, "current_location":"皇宫偏殿"},
  "stats":         {"total_chapters":75, "min_words":2500, "max_words":4000}
}
```

### 依赖映射表

| get_writing_context 字段 | 写入工具 | required 字段 | 依赖关系 |
|---|---|---|---|
| `recent_chapters[].summary` | `update_chapter_meta` | `summary` | maintain 写 → prepare 读 |
| `recent_chapters[].key_events` | `update_chapter_meta` | `key_events` | maintain 写 → prepare 读 |
| `scenes[].location` | `create_scene` / `update_scene` | `location_id` (推荐) | maintain 写 → prepare 读 |
| `scenes[].arc_node` | `create_scene` / `update_scene` | `arc_node_id` (推荐) | maintain 写 → prepare 读 |
| `characters[].location` | `create_character` / `update_character` | `location_id` (推荐) | maintain 写 → prepare 读 |
| `characters[].items` | `create_item` / `update_item` | `owner_id` | maintain 写 → prepare 读 |
| `active_arcs.related_items` | `create_item` / `update_item` | `arc_id` | maintain 写 → prepare 读 |
| `active_arcs.related_lore` | `create_lore` / `update_lore` | `arc_id` | maintain 写 → prepare 读 |
| `active_arcs.nodes_done` | `create_arc_node` / `update_arc_node` | `story_arc_id`, `status` | maintain 写 → prepare 读 |
| `timeline.pending` | `create_timeline_entry` / `update_timeline_entry` | `title`, `category`, `target_chapter` | maintain 写 → prepare 读 |
| `timeline.resolved` | `update_timeline_entry` | `resolved_chapter_id` | maintain 写 → prepare 读 |
| `reader` | `create_reader_perspective_entry` / `update_reader_perspective_entry` | `type`, `content`, `planted_chapter` | maintain 写 → prepare 读 |
| `writing_snapshot` | `update_writing_snapshot` | `summary` | maintain 写 → prepare 读 |

## 3. 全部 jsonschema:required 字段

### CREATE 工具

| 工具 | required 字段 | 消费者 |
|------|--------------|--------|
| `create_character` | `name` | get_characters |
| `create_location` | `name` | get_locations |
| `create_location_relation` | `location_a_id`, `location_b_id`, `relation_type` | get_locations (network) |
| `create_scene` | `chapter_id`, `scene_number`, `title`, `summary` | get_writing_context scenes |
| `create_item` | `name`, `arc_id`, `owner_id`, `narrative_role` | get_writing_context arcs/characters |
| `create_lore` | `title`, `category`, `content`, `arc_id`, `reveal_chapter_id` | get_writing_context arcs |
| `create_item_occurrence` | `item_id`, `chapter_id`, `action` | get_item_occurrences |
| `create_timeline_entry` | `title`, `category`, `target_chapter` | get_timeline |
| `create_story_arc` | `name`, `arc_type` | get_story_arcs |
| `create_arc_node` | `story_arc_id`, `title`, `target_chapter` | get_story_arcs (node progress) |
| `create_reader_perspective_entry` | `type`, `content`, `planted_chapter` | get_reader_perspective |
| `create_preference` | `category`, `content` | get_preferences |

### UPDATE 工具

| 工具 | required 字段 | 消费者 |
|------|--------------|--------|
| `update_chapter_meta` | `chapter_id`, `summary`, `key_events`, `characters_in`, `arc_ids` | get_writing_context recent_chapters |
| `update_writing_snapshot` | `summary` | get_writing_snapshot |
| `update_character_relationship` | `relation_describe` | get_character_relations |
| 其他 update_* | 仅 ID 字段 required | PATCH 语义，其余字段可选 |

### DELETE 工具

| 工具 | required 字段 |
|------|--------------|
| `delete_record` | `table`, `record_id` |

## 4. 阶段门禁配置

### single 模式

| 阶段 | require | tools 要点 | next |
|------|---------|-----------|------|
| prepare | get_writing_context, get_chapter_list, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_writing_snapshot, get_scenes, get_preferences | 全部只读 + set_phase | outline |
| outline | edit | 写大纲文件 + set_phase | write |
| write | edit, get_chapter_list | 写正文文件 + set_phase | review |
| review | run_subagent | 审稿 + set_phase | maintain |
| maintain | edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective | 全部读写工具 + set_phase | prepare |

### batch 模式

同 single，但 write → outline（循环），最后 prepare → done。

## 5. HTTP API 端点

| 端点 | 对应工具 | 用途 |
|------|---------|------|
| `GET /api/novels` | - | 小说列表 |
| `GET /api/novels/{id}/chapters` | get_chapter_list | 章节列表 |
| `GET /api/chapters/{id}` | read | 章节 DB 元数据 |
| `GET /api/characters` | get_characters | 角色列表 |
| `GET /api/character-relations` | get_character_relations | 角色关系 |
| `GET /api/locations` | get_locations | 地点列表 |
| `GET /api/location-relations` | get_locations (network) | 地点连通关系 |
| `GET /api/lore` | get_lore / search_lore | 设定列表 |
| `GET /api/items` | get_items / search_items | 物品列表 |
| `GET /api/item-occurrences` | get_item_occurrences | 物品出现记录 |
| `GET /api/scenes` | get_scenes | 场景列表 |
| `GET /api/timeline` | get_timeline | 时间线 |
| `GET /api/arcs` | get_story_arcs | 弧线列表 |
| `GET /api/arc-nodes` | get_story_arcs (nodes) | 弧线节点 |
| `GET /api/reader` | get_reader_perspective | 读者认知 |
| `GET /api/preferences` | get_preferences | 偏好 |
| `GET /api/stats` | get_stats | 统计 |
| `GET /api/writing-snapshot` | get_writing_snapshot | 写作快照 |
| `GET /api/phase-gate-config` | get_phase_gate_config | 门禁配置 |
| `GET /api/search-memory` | search_story_memory | 语义搜索 |
| `GET /api/writing-context` | get_writing_context | 书写上下文树 |
| `GET /api/read` | read | 读取文件内容 |
| `POST /api/chat` | - | AI 对话 (SSE) |

共 23 个端点。

## 6. 已修 Bug 记录

| Bug | 位置 | 修复 |
|-----|------|------|
| `create_character` 的 `location_id` 参数被静默忽略 | character_tools.go Execute | 构造 Character 时加 `LocationID: item.LocationID` |
| `update_lore` 无法更新 `arc_id`/`reveal_chapter_id`/`is_public` | lore_tools.go Args + Execute | 新增字段 + 更新逻辑 |
| `get_writing_context` 的 `related_lore` 返回 null 而非 [] | writing_context_tools.go | `var loreIDs []int64` → `make([]int64, 0)` |
| `get_writing_context` 缺少 `recent_chapters` 摘要和关键事件 | writing_context_tools.go | recent_chapters 加 summary + key_events 字段 |
| `type` 字段返回英文 "main" 而非中文 "主线" | writing_context_tools.go | 加 `arcTypeZh()` 映射函数 |

## 7. system prompt 优化记录

优化日期：2026-07-25
改前：~4700 token → 改后：~2400 token

| 删除内容 | 原因 | 省 token |
|---------|------|---------|
| 新增创作工具说明（10 个工具 Description） | 工具自身的 Description 已有 | ~900 |
| 六大模块管理指南（角色/时间线/弧线/地点/读者/偏好） | 工具 Description 已有 | ~700 |
| 操作准则 + 输出规范 + 系统架构 | 多处重复 | ~300 |
| 跨领域协同 | 空泛建议 | ~80 |

