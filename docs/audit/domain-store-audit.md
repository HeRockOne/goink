# Goink 领域存储层源码审计摘要

审计范围：internal/session|novel|chapter|character|location|item|itemoccurrence|lore|scene|storyarc|timeline|reader|writing|style|pattern（store.go/types.go，测试跳过），附 shared internal/storage/pagination.go。

行号均以本次 read 结果（文件从头到尾）为准。文中"坑/要点"为事实与注释明示的语义，非改进建议，只报告事实。

## 0. 共享层 internal/storage

### 职责
- pagination.go：统一的泛型分页契约（PageParams / PageResult[T]），所有领域 Store 的 List / ListByNovel 都返回此结构。
- sqlite.go / operation_log.go / patch.go：DB 连接、操作日志、补丁（非本审计重点）。

### 关键类型
- PageParams（pagination.go L9-14）：{Page int; Size int}。
- Normalize()（L20-28）：Page<1 → 1；Size<1 或 >10000 → 20。链式返回自身。
- PageResult[T]（L3-7）：Items / Total / Page / Size / TotalPages。
- NewPageResult[T]（L31-45）：按 total/size 计算 TotalPages（向上取整）。

### 坑
- Normalize 返回新指针：各 Store 两种用法——`pp := opts.PageParams; pp.Normalize()`（novel/chapter 等）与 `pp := (&storage.PageParams{...}).Normalize()`（item/lore），均为新旧一致模式，无错误。
- Size 上限 10000；session 的 search 等不设分页的路径不经 Normalize。

## 1. internal/session —— 会话 + 消息 + 模型用量
### 职责
管理 Session 元数据、Message（append-only 消息）、ModelUsage（按模型 token 累计），支撑 agent loop 的 turn 管理、上下文压缩（active_version）、前后端可见性、阶段门禁状态、计费统计。
### 文件
- store.go（357 行）；types.go（117 行）。
### Session 模型（types.go L18-35，表 sessions）
| 字段 | 说明 |
|---|---|
| SessionID (PK) | 字符串主键 |
| NovelID | 索引 idx_sessions_novel（与 updated_at 复合） |
| Model | 默认 deepseek-v4-pro |
| ReasoningEffort | high/max/''（DeepSeek 推理深度） |
| Summary | 最新压缩摘要，压缩时全量替换 |
| ActiveVersion | 压缩代数，默认 1，配合 Message.version |
| LastTurnID | 最后一个 turn 编号，原子自增 |
| Usage | JSON，最近一次 LLM token 用量 |
| CurrentPhase/CalledTools/PhaseMode | 阶段门禁持久化字段 |
### Message 模型（types.go L50-75，表 messages）
- append-only、永不修改永不删除。
- ToAPI / ToFrontend 两个布尔独立控制可见性，由写入方独立决定，不由 role 推导；四种 role 可任意组合。
- Version = 消息所属压缩代数，查询时 = session.active_version。
- EventType：compression/interrupt/error/''。
- AgentType：main/review/memory（默认 main），区分主/子 agent。
- SubTaskID：run_subagent 的 tool call ID，前端路由用。
- TurnID：回退时直接 DELETE WHERE turn_id。
- ToAPI/ToFrontend 的 default:0 注释：ToAPI 必须默认 0（子 agent ToAPI=false 不能被默认值覆盖）；ToFrontend default:0 必须与 Go false 一致，否则 GORM 跳过零值时 DB 填 1 导致泄漏。
### 关键方法（store.go）
- NextTurn（L246-260）：原生 SQL 原子自增——UPDATE sessions SET last_turn_id=last_turn_id+1, updated_at=? WHERE session_id=? RETURNING last_turn_id。单步递增+持久化。
- GetMessagesForAPI（L266-282）：Where(session_id, to_api=true, version)，ORDER BY id DESC LIMIT 1000，再原地反转回 id 升序。注释自述历史坑：旧实现 ORDER BY id ASC LIMIT 1000 取最早 1000 条，超限时新 user 消息与 NS 快照（id 尾部）被截断，agent 第一轮无用户输入。反转保证历史顺序是请求前缀顺序。
- GetMessagesForFrontend（L287-295）：to_frontend=true，id ASC，无版本过滤。
- GetAllMessages（L301-309）：全量 id ASC，审计/回退用。
- BumpActiveVersion（L234-244）：加载→ActiveVersion++→Save，压缩时用，不删旧消息。
- UpdateSessionUsage（L99-110）：加载→替换 Usage 字段→Save。
- UpdateMessageUsage（L119-143）：按 (session_id, turn_id, role=assistant, agent_type) 取该 turn 该 agent 最后一条 assistant 消息（id DESC LIMIT 1），把 usage 写入其 ExtraMetadata.usage。agentType 区分主/子 agent，避免同 turn 互相覆盖。
- GetSessionCumulativeUsage（L150-187）：遍历全部 assistant 消息，从每条 ExtraMetadata.usage 提取 cached_tokens / completion_tokens / prompt_tokens（miss=prompt-cached），累加返回 prompt_cache_hit_tokens / prompt_cache_miss_tokens / acc_completion_tokens。仅统计 role='assistant'。
- UpsertModelUsage（L344-357）：按 (session_id, model_id) 存在则 Updates 累加增量，否则 Create。传增量值不是累计值。
- DeleteSession（L315-335）：事务内删 messages + 手工 DELETE FROM model_usage/turn_commits/operation_log WHERE session_id + 删 sessions。注释自述旧实现残留孤儿数据（实测 411 operation_log / 43 turn_commits / 10 model_usage）。
- SavePhaseGateState / SavePhaseGateMode（L337-350）：非事务 Updates。SavePhaseGateMode 注释：跨 turn 必须恢复 phase_mode，否则批量会话退化成 single。
### ToAPIFormat（types.go L84-117）
- 基础 payload = {role, content}。
- assistant：ThinkingContent 非空加 reasoning_content；解析 ExtraMetadata 的 tool_calls；有 tool_calls 且无 thinking 时补空 reasoning_content。
- tool：从 ExtraMetadata 取 tool_call_id / tool_name 拼入。
- 坑：ExtraMetadata 解析失败仅 slog.Warn 静默降级。
### 重点追问 1（session 消息存储协议）回答
- append-only：Message 永不改删（除 turn 级回退）。
- active_version：Session.ActiveVersion 与 Message.Version 配对，压缩不动旧消息只 bump 版本；API 查询按 version=active_version 过滤。
- to_api/to_frontend：两个独立布尔，写入方决定，不由 role 推导。
- GetMessagesForAPI 排序：DESC+Limit1000+反转，解决超限截断最新消息的历史 bug。
- NextTurn：Raw SQL 原子自增 last_turn_id。
- updateUsage：UpdateMessageUsage（消息 ExtraMetadata，按 turn+agentType 隔离）+ UpdateSessionUsage（session.Usage 快照）+ UpsertModelUsage（model_usage 按模型累加增量）+ GetSessionCumulativeUsage（读侧累加）。四路径并存。

## 2. internal/novel —— 小说索引 + 偏好
### 职责
Novel 索引（正文在 Git 仓库，路径由 config.NovelDirPath 实时计算）+ PreferenceItem 创作偏好（用户全局 vs 小说专属）。
### 文件
- store.go（96 行）；types.go（34 行）。
### 模型
- Novel（types.go L8-19，表 novels）：ID / Title(index) / Genre(index) / Description / createdAt / updatedAt。无正文内容字段（正文在 Git）。
- PreferenceItem（types.go L22-32，表 preference_items）：NovelID / IsGlobal / Category（自由文本，LLM 归类）/ Content / Status('active'|'superseded') / Version（更新次数）。
### 关键方法（store.go）
- List（L24-50）：分页 + genre 过滤 + title LIKE，updated_at DESC。
- ListPreferences（L52-61）：WHERE is_global=true OR novel_id=?，ORDER BY is_global DESC, created_at ASC——小说专属在前、全局在后。给 LLM 注入偏好的主入口。
- ListNovelPreferences（L62-68）：is_global=false AND novel_id=?，前端编辑用。
- ListGlobalPreferences（L69-77）：仅 is_global=true。
### 数据流
LLM/前端 → PreferenceItem → ListPreferences 混合返回 → 注入系统提示词/上下文。
### 坑
- Novel 表无任何 Create/Update/Delete 方法，store 只读；写靠公开的 Store.DB 由外部直接操作。
- superseded 标记 + Version 递增谁维护：store 无写方法，靠外部 mcp 层（未审计）。

## 3. internal/chapter —— 章节元数据
### 职责
章节目录元数据（标题/摘要/关键事件/出场角色/弧线），正文与大纲在 Git 仓库（git.ChapterPath）。
### 文件
- store.go（161 行）；types.go（31 行）。
### 模型
- Chapter（types.go L8-18，表 chapters）：NovelID+ChapterNumber 复合唯一索引 uk_novel_chapter；NovelID constraint:OnDelete:CASCADE；KeyEvents/CharactersIn/ArcIDs 均为 JSON 字符串列；FilePath 是 gorm:"-"（非 DB 列，查询后由 store 填充）。
- ChapterArc（types.go L20-27，表 chapter_arcs）：章节-弧线多对多；ChapterID+StoryArcID 复合唯一 uk_chapter_arc；三列均 CASCADE。
### 关键方法（store.go）
- ListByNovel（L25-53）：分页，chapter_number ASC/DESC（默认 ASC），查询后循环填充 FilePath=git.ChapterPath(ChapterNumber)。
- ListAllByNovel（L55-69）：全量升序。
- GetByNovelAndNumber（L70-82）：novel_id+chapter_number 单查。
- GetLatestNumber（L84-96）：COALESCE(MAX(chapter_number),0)，无章节返回 0。
- SearchByNovel（L97-110）：title OR summary LIKE。
- GetRecent（L111-122）：chapter_number DESC + limit。
- GetRecentBefore（L124-142）：chapter_number <= N DESC + limit——叙事面板 current/past 卡片锚点。
- UpdateTitle（L143-151）：novel_id+chapter_number 更新 title。
### 数据流
正文在 Git：查询章 → 拿 summary + FilePath → 前端/LLM 通过 FilePath 读 Git 文件。Chapter 自身不含正文内容列。
### 坑
- SearchByNovel/GetRecent/GetRecentBefore 接收 limit 且无 <=0 兜底：limit<=0 时 SQL LIMIT 0 返回空（区别于 item/lore 的 Search 默认 10）。
- UpdateTitle 是仅有的单字段更新；无 Create（写靠外部）。
- ChapterArc 表存在但 store 无对应方法（无按章查弧线/加弧线），关联维护依赖外部 Store.DB。

## 4. internal/character —— 角色 + 角色关系
### 职责
Character（角色元数据）+ CharacterRelation（有向关系快照，append-only + is_current）。
### 文件
- store.go（160 行）；types.go（63 行）。
### 模型
- Character（types.go L8-19，表 characters）：NovelID CASCADE；Name 索引；Personality/Abilities 为 JSON 自由格式；LocationID(*int64，当前所在地)；Status('alive'|'dead'|'missing'|'unknown')；StatusChangedChapterID。
- CharacterRelation（types.go L33-50，表 character_relations）：SourceCharacterID / TargetCharacterID 各自索引 + NovelID CASCADE；RelationDescribe 自由文本（LLM 自然语言描述多面关系，替代 Python 的 18 枚举）；ChapterID（关系确立/变更章节）；IsCurrent 布尔。
### types.go 注释明示的设计原则（与 Python 关键差异）
1. 自由文本 relation_describe 替代枚举类型。
2. 追加不可变：关系变更不修改旧行，而是 INSERT 新行 + 旧行 is_current=false。(source,target) 全记录按 created_at 排序即完整演变历史。
3. 不需要 evolved_from_id 自引用指针链：用 (source,target) 配对 + 时间戳隐式排序替代，消除断链与并发分叉。Python 因枚举类型致一对角色多行并行记录才被迫引入自引用链。
4. 不需要 status 字段：is_current 布尔替代 active/dormant/resolved/severed 四态。
### 关键方法（store.go）
- Character：ListByNovel（分页+name 搜索）；ListAllByNovel（name ASC 全量，前端侧边栏/关系图）；GetByIDs（IN 批量解析角色名）。
- CharacterRelation：
  - ListCurrentByNovel（L60-66）：is_current=true，前端关系图。注释：数据量大不建议直接给 LLM。
  - ListByCharacter（L68-76）：(source=X OR target=X) AND is_current=true——单角色邻域。
  - ListByCharacters（L83-90）：源或目标 IN 集合且 is_current——LLM 查角色群关系网。
  - GetHistory（L95-103）：双向匹配（A→B 或 B→A），created_at ASC，含历史。
  - ListBetweenCharacters（L104-114）：两端都在集合内，is_current。
  - Deactivate（L118-129）：按 id Update is_current=false；RowsAffected==0 返回 gorm.ErrRecordNotFound。
### 数据流
update_character_relationship（MCP）：事务内旧行 SET is_current=false + INSERT 新行。get_character_network 查当前全图。get_character_history 按 created_at 排序（types 注释参考实现）。
### 坑
- store.go 只提供读取+Deactivate；INSERT 新关系、事务式失效+新建写入在 MCP 工具层（未审计），store 无 Add/UpdateRelation。
- ListCurrentByNovel 无 ORDER BY，关系图渲染顺序不稳定（依赖物理序 ID）。
- 关系查询多数未按 NovelID 过滤（ListByCharacter/ListByCharacters/ListBetweenCharacters/GetHistory），只靠 is_current + 角色 ID；跨小说重名 ID 理论可串，但角色 ID 全局自增降低风险。

## 5. internal/location —— 地点 + 空间关系
### 职责
Location（包含树节点）+ LocationRelation（无向空间边，UPSERT 覆盖式，非 append-only）。
### 文件
- store.go（149 行）；types.go（91 行）。
### 模型
- Location（types.go L47-59，表 locations）：NovelID CASCADE；Name 索引；LocationType 自由文本；DetailJSON（替代 Python geo_info，注释记载 Python 中 geo_info 定义了但 MCP 未暴露、AI 永远写不进，属设计了没接上线，Go 改名并接 MCP）；ParentLocationID(*int64，自引用树形包含)；Tags JSON 数组。
- types 注释列举砍掉字段及理由：related_characters/related_chapters/extra_metadata/first_appearance_chapter_id/geo_info。
- LocationRelation（types.go L70-84，表 location_relations）：LocationA/LocationB 联合唯一 uk_location_pair；无向边，工具层保证 location_a 总是较小 ID、location_b 较大；NovelID CASCADE；RelationType 自由文本。
### 关键方法（store.go）
- Location：ListByNovel（type/搜索过滤，name ASC）；ListAllByNovel（name ASC 全量，前端树/关系图）；GetChildren（parent_location_id 直接子）；GetByIDs（IN 批量）。
- LocationRelation：
  - ListRelationsByNovel（novel_id，relation_type ASC）。
  - ListRelationsByLocation（location_a=X OR location_b=X，无向）。
  - ListRelationsInvolving（a IN ? OR b IN ?）。
  - UpsertRelation（L130-141）：clause.OnConflict，Columns=[location_a, location_b]，DoUpdates=[relation_type, description, updated_at]，覆盖式写（非累加）。
### 坑
- 与 CharacterRelation 对比（types 注释明示）：地点关系静态，不需要 append-only+is_current，直接 UPSERT 覆盖。
- 可疑：LocationRelation 的 LocationA 字段 gorm 列是 location_a，但 json tag 写 location_a_id（types.go L76-77）；LocationB json tag 写 location_b_id，列是 location_b——序列化名与列名不一致。
- UpsertRelation 依赖调用方保证 location_a<location_b（工具层约定，store 不校验；反序会绕过唯一约束造成重复边）。

## 6. internal/item —— 物品/法宝
### 职责
核心道具条目，覆盖归属（owner）、叙事角色（narrative_role）、状态（destroyed 过滤）。
### 文件
- store.go（94 行）；types.go（29 行）。
### 模型
Item（types.go L8-28，表 items）：NovelID CASCADE；Name 索引；ItemType 索引；ArcID/FirstChapterID/StatusChangedChapterID(*int64 索引)；NarrativeRole('key_prop'|'supporting'|'minor'，默认 normal)；OwnerID/PreviousOwnerID(*int64 索引，持有者变迁)；LocationID；Status(默认 active)；Tags；Source('ai'|'user'|'import')。
### 关键方法（store.go）
- ListByNovel（L17-40）：分页；item_type 过滤；status 默认行为——若指定按值过滤，否则 WHERE status != 'destroyed'（默认排除已销毁）；搜索 name OR description；ORDER BY item_type, name。
- GetByID（L42-46）：id+novel_id 双条件。
- Create（L47-49）。
- Update（L51-61）：Updates map 全字段更新——用固定 map 而非 struct，跳过 GORM 零值跳过问题，owner_id/previous_owner_id 等指针零值(null)能正确写。
- Delete（L63-65）：id+novel_id。
- Search（L67-79）：name/description/ability/lore LIKE，updated_at DESC，limit<=0 默认 10。
### 坑
- ListByNovel 的 Status 默认排除 destroyed 是隐藏业务规则（不传 Status 时看不到已销毁物品）。

## 7. internal/itemoccurrence —— 物品出现记录
### 职责
物品在章节中的出现流水（acquired/used/lost/destroyed/mentioned）。
### 文件
- store.go（110 行）；types.go（16 行）。
### 模型
ItemOccurrence（types.go L8-16，表 item_occurrences）：NovelID/ItemID/ChapterID 均 CASCADE（三外键）；Action 自由文本；Description。
### 关键方法（store.go）
- ListByItem（L14-27）：chapter_id DESC + Limit 50——注释：核心道具每章出现记录随章节线性增长，全量返回撑爆上下文（MCP 路径），旧记录按需翻页。
- ListAllByItem（L29-39）：chapter_id ASC 无上限——仅前端/API 展示（非 LLM 上下文）。
- ListByItemRange（L41-62）：按章节号范围查最近 50 条。实现用 Table(item_occurrences) + JOIN chapters ON chapters.id=item_occurrences.chapter_id AND chapters.novel_id=item_occurrences.novel_id，换算章号范围；ORDER BY chapters.chapter_number DESC LIMIT 50。
- ListByChapter（novel_id+chapter_id 全量）；Create；Delete（id+novel_id）；ListByNovel（分页，chapter_id ASC）。
### 坑
- ListByItem 按 chapter_id（自增 ID）而非 chapter_number 排序；ListByItemRange 用章号排序。跨章插入场景两者可能不一致。
- ListByItemRange 依赖 chapters 表存在且 chapter_id 外键正确；章被删（CASCADE）则出现记录一并删。
- 无 Update（出现记录不可改，需删重建）。

## 8. internal/lore —— 世界观设定
### 职责
LoreEntry 世界观条目（公开/秘密设定）。
### 文件
- store.go（85 行）；types.go（25 行）。
### 模型
LoreEntry（types.go L8-24，表 lore_entries）：NovelID CASCADE；Title/Category 索引（category not null）；Content 非空；ArcID/RevealChapterID(*int64)；IsPublic(默认 true)；ReferenceID/ReferenceType；Tags；Source；Version(默认 1，更新时 +1)。
### 关键方法（store.go）
- ListByNovel（category 过滤 + title/content 搜索，category,total 排序）；GetByID；Create；
- Update（L31-44）：Updates map 更新 + version: gorm.Expr("version + 1")（SQL 表达式原子自增，非读改写）。
- Delete；Search（title/content/summary LIKE，limit<=0 默认 10）。
### 坑
- Update 的 version 自增用 gorm.Expr 在 DB 层完成（原子，避免并发覆盖），是唯一用 Expr 的地方。
- Update 用 map 显式传 is_public，能正确写 false（无 GORM 零值跳过问题）。

## 9. internal/scene —— 场景
### 职责
章节内场景条目。
### 文件
- store.go（73 行）；types.go（22 行）。
### 模型
Scene（types.go L8-21，表 scenes）：NovelID CASCADE；ChapterID(*int64, OnDelete:SET NULL)——区别于其他表，章节删除时场景 ChapterID 置 NULL 而非删除；SceneNumber；LocationID/ArcID/ArcNodeID(*int64)；CharacterIDs(JSON 数组字符串列)；WordCount；Summary。
### 关键方法（store.go）
- ListByChapter（novel_id+chapter_id，scene_number ASC）。
- ListByNovel（L23-37）：chapter_id DESC, scene_number ASC + Limit 100——注释：~3 场景/章线性增长，全量撑爆上下文（MCP 路径），完整场景查询应传 chapter_id。
- ListAllByNovel（无上限，仅前端 UI）；GetByID；Create；Update；Delete。
### 坑
- ChapterID 是唯一的 SET NULL 外键（场景孤儿保留）；其余表删子项均 CASCADE。语义：章节删除后场景保留但失去归属。
- ListByNovel 用 chapter_id 倒序取最新，不做章内去重。

## 10. internal/storyarc —— 叙事弧线 + 弧线节点
### 职责
StoryArc 弧线容器（3-5 条战略层）+ ArcNode 有序弧线节点（承接 Python plot_node）。
### 文件
- store.go（208 行）；types.go（45 行）。
### 模型
- StoryArc（types.go L9-22，表 story_arcs）：NovelID CASCADE；Name；ArcType 索引('main'|'sub'|'character'|'background'|'volume')；Importance(1-5)；Status 索引(active|paused|completed|abandoned)；ReactivateAt（自然语言恢复条件）；DetailJSON（卷纲结构化数据）；StartChapter/EndChapter（volume 时必填）。
- ArcNode（types.go L27-43，表 arc_nodes）：NovelID+StoryArcID 均 CASCADE；Title；TargetChapter(预计章节 0 未定)；ActualChapter(实际章节 0 未发生)；Status('pending'|'completed'|'abandoned')。
### 设计要点（store.go 头部注释 + types 注释）
- 排序由 target_chapter ASC, id ASC 替代序列号——创建顺序天然打破同章平局，无需 sequence。
- target_chapter 是 LLM 对不确定未来的估算，只排不滤，不准确由 review agent 校准。
- 节点窗口策略跟 TimelineEntry 一致：ListNodesAfter（未来 >=N）、ListNodesBefore（<N 最近 N 条）、ListPendingNodesBefore（远古未完成兜底）。
- ArcNode 无 Upsert：create/update 拆开，create 直接 INSERT，不依赖序列号。
- paused 弧线恢复不在此层判断：MCP 工具把弧线名+断点+reactivate_at 格式化呈现给 LLM 自行判断。
### 关键方法（store.go）
- StoryArc：ListByNovel（type/status 过滤，ORDER BY importance DESC, created_at ASC）；ListNonArchived（status IN [active,paused]）；SearchByNovel（name/description LIKE，importance DESC）。
- ArcNode：
  - ListByArcs（arcIDs IN，ORDER BY story_arc_id, target_chapter ASC, id ASC）。
  - ListNodesByChapterRange（target_chapter 范围，0 不限）。
  - ListNodesBeforeByArc（L92-106）：对每条弧线分别取 target_chapter < N 的最近 limit 条，返回 map[arcID]nodes——保证每条弧线独占窗口。
  - ListPendingNodesBeforeByArc（L109-123）：target_chapter < N 且 pending，Asc，Limit 100 兜底。
  - ListNodesAfterByArc（L126-139）：target_chapter >= N，Asc，Limit 100。
  - GetBreakpoint（L157-184）：paused 断点——before=最近 2 个 completed/abandoned（DESC 取后翻回 ASC 保时间升序）；pending=断点+下一个（最多 2 个，pending[0] 为断点）。
### 坑
- ArcNode 无 ListByNovel 全量方法（只按弧线/章节范围/单弧线窗口取），前端管理页需走 ListByArcs+已知 arcIDs 或外部 DB。
- GetBreakpoint 只返回 before/pending，不含 future 节点；恢复判断依赖 MCP 拼装。

## 11. internal/timeline —— 章节计划 + 伏笔/用户指令
### 职责
ChapterPlan（章节规划，三槽位 next/near/far，存 Git）+ TimelineEntry（伏笔、编年史、用户创作意图追踪）。
### 文件
- store.go（162 行）；types.go（99 行）。
### 模型
- ChapterPlan（types.go L8-21）：非 GORM model！仅 {NovelID, Scope, Content}，无 TableName/表；存 Git 文件 plans/{scope}.md（git.PlanPath）。注释：每小说每 scope 一条，固定 3 行，直接覆盖不改历史。
- TimelineEntry（types.go L58-83，表 time_entries）：NovelID CASCADE；EntryType('foreshadowing'|'chronicle')；Category 约束枚举('foreshadowing'|'user_directive'，驱动不同查询/注入策略)；Status('pending'|'resolved'|'abandoned'|'occurred' 编年史)；TargetChapter not null 主排序键；Importance(1-5 默认 3)；SourceChapterID；Source；ResolvedChapterID；ChronologyDate（编年史时间坐标）。
- types 注释砍掉字段：extra_metadata/related_entry_ids/tags/arc_id/sequence/version/last_editor/original_ai_output/resolved_at(可由 resolved_chapter_id 推导)/time_horizon。
### 关键方法（store.go）
- ChapterPlan：GetPlans（L25-44）——遍历 next/near/far 从 git.ReadFile 读，文件不存在(os.ErrNotExist)时 Content 为空串不报错；其他错误才返回 err。SavePlan（L46-49）——git.WriteFile 全量替换。
- TimelineEntry：
  - ListByChapterRange（L58-72）：target_chapter 窗口，0 不限，ORDER BY target_chapter ASC, importance DESC。
  - ListByNovel（分页，category/status 过滤，同排序）。
  - ListBefore（L77-84）：target_chapter < N，DESC，limit，不论状态。
  - ListPendingBefore（L88-101）：target_chapter < N AND status=pending，DESC（最近的在前），Limit 100 兜底。
  - ListAfter（L103-114）：target_chapter >= N，Asc，Limit 100。
  - SearchByNovel（L134-143）：title/content LIKE，importance DESC。
### 重点追问 2/3 + 关键坑
- target_chapter 只用于 ORDER BY 不做 WHERE 过滤（AGENTS.md 及 types 注释反复强调只排不滤）。但窗口方法（ListBefore/After/Pending/ChapterRange）的实际 WHERE 确实用了 target_chapter 范围做粗锚。可解读为：窗口查询用章号粗锚+LIMIT 兜底，而索引注入策略（types 注释 [N-3,N+5] 窗口+全量 100 条）不靠它精确过滤。是否真不一致取决于 mcp 注入层（未审计）。
- store.go L118-126 有一大段游离中文注释（设计备忘，未落码）：构造上下文时前 10 条历史+未来 100 条+之前所有 pending；状态异常（未来已完成/未结束）需提醒 LLM 修正；review agent 专属异常查询工具规划。
- ChapterPlan 无持久化表（存 Git 文件覆盖式）；TimelineEntry 实际表名是 time_entries（不是 timeline_entries），store 注释对比 Python 时称 timeline_entries。
- EntryType 与 Category 并存且语义重叠；ChronologyDate 是编年史新增。

## 12. internal/reader —— 读者认知
### 职责
追踪读者知道什么、在等什么答案、误以为是什么三类认知条目。
### 文件
- store.go（90 行）；types.go（35 行）。
### 模型
ReaderPerspective（types.go L25-34，表 reader_perspectives）：NovelID CASCADE；Type 索引(known|suspense|misconception，常量 L8-10)；Content；RelatedTruth（作者全知视角，所有类型可记录真相）；PlantedChapter(种植章)；RevealedChapter(回收章，0=未回收)；CreatedAt（无 UpdatedAt）。
### 关键方法（store.go）
- ListByNovel（type 过滤，ORDER BY type, planted_chapter ASC）。
- ListActive（L35-48）：revealed_chapter=0，ORDER BY type, planted_chapter ASC，Limit 100。注释：悬念只种不收线性增长，全量撑爆上下文，最近优先保留（counts 用 Count 不受截断影响）。
- ListActiveFiltered（L50-63）：revealed_chapter=0 + search(LIKE content) + planted_chapter 范围，Limit 100——定向查询替代全量拉后自筛，省 token 不丢目标。
### 坑
- types 注释：known 类型全量返回不过滤 revealed_chapter；但 store 的 ListActive/ListActiveFiltered 统一按 revealed_chapter=0 过滤，未按 type 区分（known 条目 revealed>0 也会被排除）。过滤需在 MCP/调用层再做 known 特判。
- 只一个时间戳 CreatedAt。

## 13. internal/writing —— 写作日志 + 写作进度快照
### 职责
WritingLog（每次保存字数增量）+ WritingSnapshot（每本书一条当前写作进度快照）。
### 文件
- store.go（144 行）；types.go（32 行，含 Snapshot 类型）；snapshot_store.go（30 行）。
### 模型
- WritingLog（types.go L8-17，表 writing_log）：ID；Date(字符串 2006-01-02，索引 idx_writing_date，size:10)；NovelID/ChapterID(索引)；WordDelta(正=新增，负=删除)；CreatedAt。
- WritingSnapshot（snapshot_types.go L8-19，表 writing_snapshots）：NovelID 是主键（非自增，直接以小说 ID 为 PK）；LastChapterID/LastChapterNum；CurrentArcID；CurrentLocation；ActiveChars/PendingThreads/DetailedState(JSON 字符串)；Summary；时间戳。
- WritingStats / DailyActivity（types.go L21-32）：聚合返回结构。
### 关键方法（store.go / snapshot_store.go）
- LogDelta（store L21-32）：delta==0 直接 return；否则 Create 一条 WritingLog。append-only 日志表。
- GetDailyActivity（months<=0 默认 12）：date >= cutoff AND word_delta > 0，GROUP BY date SUM(word_delta)，date ASC。
- GetWritingStats：正数 delta 总和(WHERE word_delta > 0)→TotalWords；COUNT(DISTINCT date)→活跃天数；+ computeStreaks。
- computeStreaks（L64-117）：取近 730 天 DISTINCT date（限扫描），字符串 parse，连续判定 diff>=24 && diff<48 视为连续，>=48 重置；当前连续从最后一天往回数，仅当最后一天=today 或 yesterday 才算。
- SnapshotStore.Get（L17-22）：First(&snap, novelID)（主键查，无 NotFound 包装，透传 gorm.ErrRecordNotFound）。
- SnapshotStore.Upsert（L24-30）：Save(snap)——主键存在全字段更新，不存在插入，实现每本书仅一条。
### 重点追问 3（写作快照约定）
snapshot 不是 append-only，是覆盖式：NovelID 主键 + Save 语义，每本书一条当前进度，不保存历史。
### 坑
- GetWritingStats 的 TotalWords/TotalDays 依赖 word_delta > 0（只算新增），负 delta 不冲抵累计。
- WritingLog.Delta=0 被丢弃（不记录 0 变化）。
- 快照写入用 Save struct 全量覆盖（含零值字段，因传 struct 指针非 map）。

## 14. internal/style —— 风格素材
### 职责
风格素材样本（Sample）+ StringSlice JSON 数组类型。
### 文件
- store.go（76 行）；types.go（84 行）。
### 模型
- StringSlice（types.go L8-26）：[]string 的 sql.Scanner/driver.Valuer 适配——DB 读 JSON TEXT→[]string，写转 JSON（nil→"[]"）。
- Sample（types.go L52-70，表 style_samples）：NovelID；IsGlobal；Name；Content(大文本)；Preview；Tags(StringSlice JSON)；WordCount。
- Stats（L73-84）：代码计算的确定性文本统计——非 DB 模型，LLM 无关。
- ExtractResult（L86-92）：风格提取返回值。
### 关键方法（store.go）
- List（L22-49）：只 Select 摘要列(id, novel_id, is_global, name, preview, tags, word_count, created_at, updated_at)不读大 content。NovelID>0 → is_global=true OR novel_id=?；==0 → 仅 is_global=true。search name LIKE。created_at ASC。
- GetByIDs（IN，含 content 全字段，用于 ComputeStats/ExtractStyle）。
### 坑
- List 刻意不查 content 省传输，GetByIDs 才带全字段——两种取数路径分离。
- Sample.NovelID 结构 not null，但全局素材(is_global=true)的 novel_id 语义无定义（可为 0 占位）。
- style 无 Update/Delete 方法（store 只读+GetByIDs），写靠外部 DB。

## 15. internal/pattern —— 模式提取（无 DB）
### 职责
叙事模式提取的 DTO/常量/LLM 输出结构。本目录只有 types.go（120 行），无 store.go，无数据模型。
### 关键类型
- ExtractPatternInput / ExtractPatternResult(含 Trace)：任务入参/出参。
- Pipeline 阶段常量：StageLoaded/Boundaries/Summaries/InitialChunks/CompressChunks/Finalizing/Done。
- LLMStatus：thinking/generating。
- Progress：任务进度推送（Stage/Message/LLMStatus/Round/BatchIndex/BatchTotal/Tokens/Boundaries/Summaries/Chunks）。
- Trace：任务全量追踪（ChapterCount/ContextWindow/BatchBudget/Boundaries/Summaries/ChunkRounds/FinalTokens）。
- ChunkRoundTrace / BatchTrace：分块轮次与批次追踪。
- BoundaryHint/BoundaryHintsOutput/ChapterSummaryItem/ChapterSummariesOutput/Chunk/ChunksOutput：LLM 结构化输入输出，带 jsonschema 约束注释。
- ChapterSource（L113-119）：从 DB 元数据和 git 正文拼出的章节数据传递对象（含 Content）。
### 坑
- 该模块仅是 DTO 定义，无持久化逻辑；真正 pipeline 在别处（mcp_tools 或 app 层，不在审计范围）。

## 16. 综述：追加式表与当前态标志清单
| 表 | 追加式? | 当前态标志 | 写入约定 |
|---|---|---|---|
| messages | 是 | version(+session.active_version) | 永不改删；turn 级 DELETE WHERE turn_id 回退 |
| character_relations | 是 | is_current | 变更=旧行 is_current=false + 新 INSERT；无自引用链 |
| item_occurrences | 是 | 无(流水) | 只 Create/Delete；ListByItem 50 条上限 |
| writing_log | 是 | 无 | 每条保存一条 delta；delta=0 丢弃 |
| scenes | 否(覆盖) | 无 | Update 覆盖；ChapterID SET NULL(孤儿保留) |
| location_relations | 否(UPSERT) | 无 | (a,b) 唯一，ON CONFLICT DO UPDATE 覆盖 |
| chapter_plans(文件) | 否(覆盖) | 无 | plans/{scope}.md 全量替换，固定 3 行 |
| writing_snapshots | 否(覆盖) | 无 | NovelID 主键+Save，每本一条 |

## 17. 索引 / 外键 / 级联汇总
### 索引（非默认 PK 外）
- sessions：idx_sessions_novel（novel_id, updated_at 复合）。
- messages：session_id / turn_id / version / to_api / to_frontend / agent_type / sub_task_id / created_at 各自单列索引（无复合）。
- chapters：uk_novel_chapter 唯一(novel_id, chapter_number)。
- chapter_arcs：uk_chapter_arc 唯一(chapter_id, story_arc_id)。
- location_relations：uk_location_pair 唯一(location_a, location_b)。
- writing_log：idx_writing_date(date)。
- 其余实体均 novel_id 单列索引 + 外键列单列索引。
### 外键/级联
- 绝大多数子实体（character/location/item/itemoccurrence/lore/scene/storyarc/arc_node/reader/chapter/chapter_arc）novel_id 均 OnDelete:CASCADE（删小说级联清空）。
- itemoccurrence 的 ItemID/ChapterID、chapter_arc 的 ChapterID/StoryArcID、arc_node 的 StoryArcID 均 CASCADE。
- scene.ChapterID 是 SET NULL——唯一例外，章节删除后场景保留但脱钩。
- Character.LocationID、item 的 OwnerID 等指向其他实体的列无 FK 约束声明（仅普通索引 *int64），依赖应用层一致性。
- 显式 FK 声明均为 constraint:OnDelete 系列；GORM 默认不建物理外键，这些是 model 声明，物理 FK 取决于 migrate 实现。
### 可疑/值得注意点汇总（事实）
1. LocationRelation json tag 为 location_a_id/location_b_id，gorm 列为 location_a/location_b（types.go L76-77）——序列化名不一致。
2. reader 的 known 类型不过滤 revealed_chapter 的注释承诺，与 store 方法统一 revealed_chapter=0 过滤不符。
3. GetMessagesForAPI 已修 DESC+反转 bug（注释自述历史坑）。
4. DeleteSession 已修孤儿数据问题（注释自述 411/43/10 条实测）。
5. GetRecent/GetRecentBefore/SearchByNovel 的 limit<=0 无兜底（SQL LIMIT 0 返回空）。
6. timeline store.go 有游离中文设计备忘注释未落码。
7. scene.ListByNovel/reader.ListActive/storyarc 窗口方法/item 等普遍硬编码截断上限（50/100），是上下文安全约束非 bug。
8. target_chapter 窗口方法 WHERE 与只排不滤注释存在表述张力（见 timeline 节）。

## 18. 对外依赖汇总
- 所有 Store 依赖：gorm.io/gorm + internal/storage（PageParams/PageResult[T]）。
- session.ModelUsage/DeleteSession 依赖表 model_usage/turn_commits/operation_log（后两者在 storage 包）。
- chapter/timeline 依赖 internal/git（git.ChapterPath/git.PlanPath/git.ReadFile/git.WriteFile）——正文与大纲/计划存 Git 不存 DB。
- timeline.ChapterPlan 无 GORM 模型，靠 git 文件。
- writing.SnapshotStore 独立于 Store（两个 Store 实例）。
- session.ToAPIFormat 依赖 encoding/json + slog（仅 Warn）。
- style 依赖 database/sql/driver（实现 Valuer/Scanner）。