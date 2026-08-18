package mcp_tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/config"
	"novel/internal/git"
	"novel/internal/item"
	"novel/internal/itemoccurrence"
	"novel/internal/location"
	"novel/internal/lore"
	"novel/internal/outline"
	"novel/internal/reader"
	"novel/internal/scene"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/timeline"
	"novel/internal/writing"
)

// ── get_writing_context ─────────────────────────────────
// 返回树状结构：以 current_chapter 为根，向下展开场景→角色→地点→物品、弧线→节点→关联

type GetWritingContextArgs struct {
	CurrentChapter int `json:"current_chapter" jsonschema:"required,description=要写的章节号（必填），用于定位本章场景、出场角色等"`
}

type GetWritingContextTool struct{}

func (t *GetWritingContextTool) Name() string { return "get_writing_context" }
func (t *GetWritingContextTool) Description() string {
	return "【省token专用·树状关联】一次性获取创作准备所需的全部上下文，替代多次 get_* 调用。" +
		"使用时机：prepare 阶段开头调用一次即可，不要反复调用。\n" +
		"返回结构说明：\n" +
		"chapter: 当前章节 {num=章节号, title=标题, word_count=字数}\n" +
		"recent_chapters[]: 最近5章 [{num, title, summary=本章摘要, key_events=关键事件JSON数组, word_cnt=字数, characters_in=本章出场角色ID数组JSON, arc_ids=本章涉及弧线ID数组JSON}]\n" +
		"scenes[]: 本章场景 [{title, summary, word_count, location={name=地点名, type=地点类型}, arc_node={title=节点标题, arc_name=所属弧线名}}]\n" +
		"characters[]: 出场角色 [{name, status=角色状态(alive/dead/missing/unknown，dead=已死亡不得出场), location={name=所在地点}, items=[{name, role=key_prop/supporting/minor}], item_count=持有物品总数}]\n" +
		"dead_characters[]: 已死亡角色名单（status=dead 的聚合，写作时严禁让其中任何角色再次出场/说话/被提及为在场）\n" +
		"active_arcs[]: 活跃弧线 [{name, type_zh=类型中文(主线/支线/角色弧/背景), nodes_done=已完成节点数, nodes_total=总节点数, related_lore=[关联设定ID], related_items=[关联物品ID]}]\n" +
		"global_lore[]: 全局设定索引（arc_id 为空、跨弧线根基设定，如修炼体系/势力格局/天道法则）[{id, name}]——写作用到这些设定时用 get_lore 取详情\n" +
		"timeline.pending[]: 待回收伏笔 [{title, category=foreshadowing/user_directive, target_chapter=目标回收章节, importance=重要度1-5}]\n" +
		"timeline.resolved[]: 已回收伏笔 [{title, resolved_chapter=实际回收章节}]\n" +
		"timeline.overdue[]: 超期未回收伏笔 [{title, target_chapter=原定回收章节, importance, overdue_by=超期了几章(越大越紧急)}]\n" +
		"reader: 读者认知计数 {known=已知信息数, suspense=活跃悬念数, misconception=读者误知数}\n" +
		"writing_snapshot: 写作快照 {last_chapter_num=最新已完成章节号, current_arc_id=当前弧线ID, current_location=当前地点, active_chars=活跃角色ID数组JSON}\n" +
		"stats: 统计 {total_chapters=总章数}\n" +
		"outline: 全书总纲摘要 {summary=book-outline.md 前400字, source=文件路径}——本章创作必须服务于总纲的核心矛盾与结局方向\n" +
		"volume: 当前卷信息 {name, description, detail_json=卷纲, start_chapter, end_chapter}——本章只展开本卷情节\n" +
		"progress: 进度锚点 {current_chapter=当前章号, volume_start=本卷起始章, volume_end=本卷结束章, rule=越界约束}——禁止提前展开后续卷情节"
}
func (t *GetWritingContextTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *GetWritingContextTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetWritingContextArgs{})
}
func (t *GetWritingContextTool) ExposeToLLM() bool { return true }
func (t *GetWritingContextTool) NewArgs() any      { return &GetWritingContextArgs{} }

func (t *GetWritingContextTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetWritingContextArgs)
	nid := tc.NovelID
	db := tc.DB
	log := slog.Default()
	chapNum := a.CurrentChapter

	result := map[string]any{}

	// ── 1. 当前章节信息 ──
	var curCh chapter.Chapter
	chapterData := map[string]any{"num": chapNum, "title": "", "id": 0}
	if err := db.WithContext(ctx).Where("novel_id = ? AND chapter_number = ?", nid, chapNum).First(&curCh).Error; err == nil {
		chapterData["id"] = curCh.ID
		chapterData["title"] = curCh.Title
		chapterData["word_count"] = curCh.WordCount
	}
	result["chapter"] = chapterData

	// ── 1.5 最近 5 章列表（以当前章节为锚点） ──
	cs := chapter.NewStore(db, log)
	recentChs, _ := cs.GetRecentBefore(ctx, nid, chapNum, 5)
	recentList := make([]map[string]any, 0)
	for _, ch := range recentChs {
		recentList = append(recentList, map[string]any{
			"num":           ch.ChapterNumber,
			"title":         ch.Title,
			"summary":       ch.Summary,
			"key_events":    ch.KeyEvents,
			"word_cnt":      ch.WordCount,
			"characters_in": ch.CharactersIn,
			"arc_ids":       ch.ArcIDs,
		})
	}
	result["recent_chapters"] = recentList

	// ── 2. 本章场景 → 角色 → 地点 → 弧线节点 ──
	sceneStore := scene.NewStore(db, log)
	// 先查当前卷，用于筛选规划场景
	var plannedArcID int64
	var vol storyarc.StoryArc
	if err := db.WithContext(ctx).
		Where("novel_id = ? AND arc_type = 'volume' AND status = 'active'", nid).
		Order("importance DESC").
		First(&vol).Error; err == nil {
		plannedArcID = vol.ID
		result["volume"] = map[string]any{
			"name":          vol.Name,
			"description":   vol.Description,
			"detail_json":   vol.DetailJSON,
			"start_chapter": vol.StartChapter,
			"end_chapter":   vol.EndChapter,
		}
		// 卷级聚合：查卷范围内实体（ID 列表，省 token）
		if vol.StartChapter > 0 && vol.EndChapter >= vol.StartChapter {
			result["volume_entities"] = buildVolumeEntitiesData(ctx, db, nid, vol)
		}
	}
	var scenes []scene.Scene
	if plannedArcID > 0 {
		db.WithContext(ctx).Raw(
			"SELECT * FROM scenes WHERE novel_id = ? AND (chapter_id = ? OR (chapter_id IS NULL AND arc_id = ?)) ORDER BY scene_number ASC",
			nid, curCh.ID, plannedArcID,
		).Scan(&scenes)
	} else {
		scenes, _ = sceneStore.ListByChapter(ctx, nid, curCh.ID)
	}
	sceneList := make([]map[string]any, 0)
	for _, s := range scenes {
		// 地点
		locInfo := map[string]any{"id": 0, "name": ""}
		if s.LocationID != nil {
			var loc location.Location
			if err := db.WithContext(ctx).First(&loc, *s.LocationID).Error; err == nil {
				locInfo = map[string]any{"id": loc.ID, "name": loc.Name, "type": loc.LocationType}
			}
		}
		// 弧线节点
		nodeInfo := map[string]any(nil)
		if s.ArcNodeID != nil {
			var node storyarc.ArcNode
			if err := db.WithContext(ctx).First(&node, *s.ArcNodeID).Error; err == nil {
				var arcName string
				db.WithContext(ctx).Model(&storyarc.StoryArc{}).Select("name").Where("id = ?", node.StoryArcID).Scan(&arcName)
				nodeInfo = map[string]any{"id": node.ID, "title": node.Title, "arc_id": node.StoryArcID, "arc_name": arcName}
			}
		}
		sceneList = append(sceneList, map[string]any{
			"id":         s.ID,
			"title":      s.Title,
			"summary":    s.Summary,
			"word_count": s.WordCount,
			"location":   locInfo,
			"arc_node":   nodeInfo,
			"scene_num":  s.SceneNumber,
		})
	}
	result["scenes"] = sceneList

	// ── 3. 本章出场角色 → 地点名 → 物品 ──
	// 收集所有场景里的角色 ID
	charIDSet := map[int64]bool{}
	for _, s := range scenes {
		ids := parseJSONInt64Array(s.CharacterIDs)
		for _, id := range ids {
			charIDSet[id] = true
		}
	}
	// 如果有 snapshot 的活跃角色，也加上
	snap, _ := writing.NewSnapshotStore(db, log).Get(ctx, nid)
	if snap != nil {
		ids := parseJSONInt64Array(snap.ActiveChars)
		for _, id := range ids {
			charIDSet[id] = true
		}
	}

	charList := make([]map[string]any, 0)
	if len(charIDSet) > 0 {
		charIDs := make([]int64, 0, len(charIDSet))
		for id := range charIDSet {
			charIDs = append(charIDs, id)
		}
		var chars []character.Character
		db.WithContext(ctx).Where("id IN ? AND novel_id = ?", charIDs, nid).Find(&chars)
		itemStore := item.NewStore(db, log)
		for _, ch := range chars {
			locInfo := map[string]any{"id": 0, "name": ""}
			if ch.LocationID != nil {
				var loc location.Location
				if err := db.WithContext(ctx).First(&loc, *ch.LocationID).Error; err == nil {
					locInfo = map[string]any{"id": loc.ID, "name": loc.Name}
				}
			}
			// 该角色持有的物品（仅 key_prop 和 supporting）
			items := make([]map[string]any, 0)
			var itemList []item.Item
			db.WithContext(ctx).Where("novel_id = ? AND owner_id = ? AND narrative_role IN ('key_prop','supporting')", nid, ch.ID).Find(&itemList)
			for _, it := range itemList {
				items = append(items, map[string]any{"id": it.ID, "name": it.Name, "role": it.NarrativeRole})
			}
			charList = append(charList, map[string]any{
				"id":         ch.ID,
				"name":       ch.Name,
				"status":     ch.Status, // alive/dead/missing/unknown：dead=已死亡（不得让其出场/说话/行动）
				"location":   locInfo,
				"items":      items,
				"item_count": countItemsForChar(itemStore, ctx, nid, ch.ID),
			})
		}
	}
	result["characters"] = charList

	// ── 4. 活跃弧线 → 节点进度 + 关联设定/物品 ──
	var arcs []storyarc.StoryArc
	db.WithContext(ctx).Where("novel_id = ? AND status = 'active'", nid).Find(&arcs)
	arcList := make([]map[string]any, 0)
	for _, ar := range arcs {
		// 节点进度
		var totalNodes, doneNodes int64
		db.WithContext(ctx).Model(&storyarc.ArcNode{}).Where("story_arc_id = ?", ar.ID).Count(&totalNodes)
		db.WithContext(ctx).Model(&storyarc.ArcNode{}).Where("story_arc_id = ? AND status = 'completed'", ar.ID).Count(&doneNodes)
		// 关联设定（初始化为空数组，不返回 null）
		loreIDs := make([]int64, 0)
		db.WithContext(ctx).Model(&lore.LoreEntry{}).Select("id").Where("novel_id = ? AND arc_id = ?", nid, ar.ID).Scan(&loreIDs)
		// 关联物品
		itemIDs := make([]int64, 0)
		db.WithContext(ctx).Model(&item.Item{}).Select("id").Where("novel_id = ? AND arc_id = ?", nid, ar.ID).Scan(&itemIDs)

		arcList = append(arcList, map[string]any{
			"id":            ar.ID,
			"name":          ar.Name,
			"type_zh":       arcTypeZh(ar.ArcType),
			"status":        ar.Status,
			"nodes_total":   totalNodes,
			"nodes_done":    doneNodes,
			"related_lore":  loreIDs,
			"related_items": itemIDs,
		})
	}
	result["active_arcs"] = arcList

	// ── 4.5 全局设定索引（arc_id 为 NULL 的跨弧线根基设定，如修炼体系/势力格局/天道法则） ──
	// 这些设定不绑定任何弧线，仅靠 arc_id 关联的查询永远看不见它们，这里显式注入 ID 列表供 AI 按需 get_lore。
	var globalLore []lore.LoreEntry
	db.WithContext(ctx).Select("id", "title").Where("novel_id = ? AND arc_id IS NULL", nid).Find(&globalLore)
	globalLoreList := make([]map[string]any, 0, len(globalLore))
	for _, gl := range globalLore {
		globalLoreList = append(globalLoreList, map[string]any{"id": gl.ID, "name": gl.Title})
	}
	result["global_lore"] = globalLoreList

	// ── 5. 时间线 ──
	tlStore := timeline.NewStore(db, log)
	tlEntries, err := tlStore.ListByNovel(ctx, nid, timeline.ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 100},
	})
	if err == nil {
		pending := make([]map[string]any, 0)
		resolved := make([]map[string]any, 0)
		overdue := make([]map[string]any, 0)
		for _, e := range tlEntries.Items {
			entry := map[string]any{
				"id":               e.ID,
				"title":            e.Title,
				"category":         e.Category,
				"status":           e.Status,
				"target_chapter":   e.TargetChapter,
				"importance":       e.Importance,
				"resolved_chapter": e.ResolvedChapterID,
			}
			if e.Status == "resolved" {
				resolved = append(resolved, entry)
			} else {
				pending = append(pending, entry)
				// 超期检测：target_chapter < 当前要写的章节号
				if chapNum > 0 && e.TargetChapter > 0 && e.TargetChapter < chapNum {
					overdue = append(overdue, map[string]any{
						"id":             e.ID,
						"title":          e.Title,
						"target_chapter": e.TargetChapter,
						"importance":     e.Importance,
						"overdue_by":     chapNum - e.TargetChapter,
					})
				}
			}
		}
		result["timeline"] = map[string]any{
			"pending":  pending,
			"resolved": resolved,
			"overdue":  overdue,
		}
	} else {
		result["timeline"] = map[string]any{"pending": []any{}, "resolved": []any{}, "overdue": []any{}}
	}

	// ── 6. 读者认知计数（Count 查询，不受 ListActive 截断影响） ──
	var knownCount int64
	db.WithContext(ctx).Model(&reader.ReaderPerspective{}).Where("novel_id = ? AND type = ?", nid, "known").Count(&knownCount)
	var suspenseCount, misconceptionCount int64
	db.WithContext(ctx).Model(&reader.ReaderPerspective{}).
		Where("novel_id = ? AND type = ? AND revealed_chapter = 0", nid, "suspense").Count(&suspenseCount)
	db.WithContext(ctx).Model(&reader.ReaderPerspective{}).
		Where("novel_id = ? AND type = ? AND revealed_chapter = 0", nid, "misconception").Count(&misconceptionCount)
	result["reader"] = map[string]int64{"known": knownCount, "suspense": suspenseCount, "misconception": misconceptionCount}

	// ── 7. 写作快照 ──
	if snap != nil {
		result["writing_snapshot"] = map[string]any{
			"last_chapter_num": snap.LastChapterNum,
			"current_arc_id":   snap.CurrentArcID,
			"current_location": snap.CurrentLocation,
			"active_chars":     snap.ActiveChars,
		}
	}

	// ── 8. 统计 ──
	settings, _ := config.LoadSettings(db)
	stats := map[string]any{}
	var totalChapters int64
	db.WithContext(ctx).Model(&chapter.Chapter{}).Where("novel_id = ?", nid).Count(&totalChapters)
	stats["total_chapters"] = totalChapters
	if settings != nil {
		stats["min_words"] = settings.MinChapterWords
		stats["max_words"] = settings.MaxChapterWords
		stats["phase_gate_enabled"] = settings.PhaseGateEnabled != nil && *settings.PhaseGateEnabled
	}
	result["stats"] = stats

	// ── 9. 全书总纲摘要（方向层，防 AI 偏离主线） ──
	// 从 book-outline.md 读取前 N 字注入；约束单向：总纲→卷纲→章纲。
	outlineMap := map[string]any{}
	// 优先读取数据库中的总纲
	oStore := outline.NewStore(db)
	if o, err := oStore.GetByNovelID(ctx, nid); err == nil {
		outlineMap["core_conflict"] = o.CoreConflict
		outlineMap["growth_arc"] = o.GrowthArc
		outlineMap["ending_direction"] = o.EndingDirection
		outlineMap["word_count_plan"] = o.WordCountPlan
		outlineMap["source"] = "database"

		// 读取大爽点
		beats, _ := oStore.ListBeats(ctx, nid)
		beatList := make([]map[string]any, 0, len(beats))
		for _, b := range beats {
			beatList = append(beatList, map[string]any{
				"chapter":     b.Chapter,
				"description": b.Description,
				"beat_type":   b.BeatType,
				"importance":  b.Importance,
			})
		}
		outlineMap["beats"] = beatList
	} else if bo, err := git.ReadFile(nid, git.BookOutlinePath()); err == nil && bo != "" {
		// 向后兼容：读取 book-outline.md
		outlineMap["summary"] = truncateRunes(bo, 400)
		outlineMap["source"] = "book-outline.md"
	} else {
		outlineMap["summary"] = "（未写入全书总纲。创作前应先创建总纲，含核心矛盾/主角成长弧线/结局方向/篇幅规划。）"
		outlineMap["source"] = ""
	}
	result["outline"] = outlineMap

	// ── 10. 进度锚点（位置层，防越界写后续卷情节） ──
	progress := map[string]any{
		"current_chapter": chapNum,
		"volume_start":    0,
		"volume_end":      0,
		"rule":            "只展开当前卷（volume_start~volume_end）范围内的情节；后续卷设定不得提前使用；所有章节事件必须服务于 outline 的核心矛盾与结局方向。",
	}
	if vol.StartChapter > 0 {
		progress["volume_start"] = vol.StartChapter
		progress["volume_end"] = vol.EndChapter
	}
	result["progress"] = progress

	return &ToolResult{Success: true, Data: result}, nil
}

// parseJSONInt64Array 解析 "[1,5,12]" 格式的 JSON 数组
func parseJSONInt64Array(raw string) []int64 {
	if raw == "" {
		return nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// truncateRunes 按 rune 截断字符串（避免按字节截断中文产生乱码）。
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// arcTypeZh 将弧线类型英文映射为中文
func arcTypeZh(t string) string {
	switch t {
	case "main":
		return "主线"
	case "sub":
		return "支线"
	case "character":
		return "角色弧"
	case "background":
		return "背景"
	case "volume":
		return "卷纲"
	default:
		return t
	}
}

func RegisterWritingContextTool(r *Registry) {
	r.Register(&GetWritingContextTool{})
}

// buildVolumeEntitiesData 查询时聚合当前卷范围内涉及的所有实体（ID 列表，省 token）。
// 从已有实体表派生，不建缓存表，避免同步负担。
func buildVolumeEntitiesData(ctx context.Context, db *gorm.DB, nid int64, vol storyarc.StoryArc) map[string]any {
	ve := map[string]any{
		"characters": []any{},
		"items":      []any{},
		"lore":       []any{},
		"foreshadow": []any{},
	}

	var chIDs []int64
	if err := db.WithContext(ctx).Model(&chapter.Chapter{}).
		Where("novel_id = ? AND chapter_number BETWEEN ? AND ?", nid, vol.StartChapter, vol.EndChapter).
		Pluck("id", &chIDs).Error; err != nil || len(chIDs) == 0 {
		return ve
	}

	// 角色：从卷内章节的场景中提取 character_ids
	charIDSet := map[int64]bool{}
	var scenes []scene.Scene
	if err := db.WithContext(ctx).Where("novel_id = ? AND chapter_id IN ?", nid, chIDs).Find(&scenes).Error; err == nil {
		for _, sc := range scenes {
			for _, id := range parseJSONInt64Array(sc.CharacterIDs) {
				if id > 0 {
					charIDSet[id] = true
				}
			}
		}
	}
	if len(charIDSet) > 0 {
		ids := make([]int64, 0, len(charIDSet))
		for id := range charIDSet {
			ids = append(ids, id)
		}
		var chars []character.Character
		db.WithContext(ctx).Where("novel_id = ? AND id IN ?", nid, ids).Find(&chars)
		list := make([]any, 0, len(chars))
		for _, c := range chars {
			list = append(list, map[string]any{"id": c.ID, "name": c.Name})
		}
		ve["characters"] = list
	}

	// 物品：item_occurrence
	var occs []itemoccurrence.ItemOccurrence
	if err := db.WithContext(ctx).Where("novel_id = ? AND chapter_id IN ?", nid, chIDs).Find(&occs).Error; err == nil {
		itemIDSet := map[int64]bool{}
		for _, o := range occs {
			if o.ItemID > 0 {
				itemIDSet[o.ItemID] = true
			}
		}
		if len(itemIDSet) > 0 {
			ids := make([]int64, 0, len(itemIDSet))
			for id := range itemIDSet {
				ids = append(ids, id)
			}
			var items []item.Item
			db.WithContext(ctx).Where("novel_id = ? AND id IN ?", nid, ids).Find(&items)
			list := make([]any, 0, len(items))
			for _, it := range items {
				list = append(list, map[string]any{"id": it.ID, "name": it.Name})
			}
			ve["items"] = list
		}
	}

	// 设定：reveal_chapter_id 在卷范围内，或 arc_id 关联卷，或 arc_id 为空（全局根基设定，始终可见）
	var lores []lore.LoreEntry
	if err := db.WithContext(ctx).
		Where("novel_id = ? AND (reveal_chapter_id IN ? OR arc_id = ? OR arc_id IS NULL)", nid, chIDs, vol.ID).
		Find(&lores).Error; err == nil {
		list := make([]any, 0, len(lores))
		for _, l := range lores {
			list = append(list, map[string]any{"id": l.ID, "name": l.Title})
		}
		ve["lore"] = list
	}

	// 伏笔：target_chapter 在卷范围内
	var tls []timeline.TimelineEntry
	if err := db.WithContext(ctx).
		Where("novel_id = ? AND target_chapter BETWEEN ? AND ?", nid, vol.StartChapter, vol.EndChapter).
		Find(&tls).Error; err == nil {
		list := make([]any, 0, len(tls))
		for _, e := range tls {
			if e.ID > 0 {
				list = append(list, map[string]any{"id": e.ID, "name": e.Title})
			}
		}
		ve["foreshadow"] = list
	}

	return ve
}
