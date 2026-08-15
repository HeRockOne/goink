package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/git"
	"novel/internal/item"
	"novel/internal/itemoccurrence"
	"novel/internal/location"
	"novel/internal/lore"
	"novel/internal/reader"
	"novel/internal/scene"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/timeline"
	"novel/internal/writing"
)

// WritingVolume 卷纲摘要，由 get_writing_context 返回。
type WritingVolume struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ArcType     string `json:"arc_type"`
	DetailJSON  string `json:"detail_json,omitempty"`
}

// WritingVolumeEntity 卷内实体的紧凑标识。
type WritingVolumeEntity struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// WritingVolumeEntities 卷级聚合：当前卷涉及的所有实体（ID 列表，省 token）。
type WritingVolumeEntities struct {
	Characters []WritingVolumeEntity `json:"characters,omitempty"`
	Items      []WritingVolumeEntity `json:"items,omitempty"`
	Lore       []WritingVolumeEntity `json:"lore,omitempty"`
	Foreshadow []WritingVolumeEntity `json:"foreshadow,omitempty"`
}

// WritingContext 聚合叙事上下文，前端一次调用拿全部。
type WritingContext struct {
	Chapter         WritingChapter          `json:"chapter"`
	RecentChapters  []WritingChapterBrief   `json:"recent_chapters"`
	Characters      []WritingCharacterBrief `json:"characters"`
	DeadCharacters  []string                `json:"dead_characters,omitempty"` // 已死亡角色名单（写时防死者复出，醒目聚合）
	ActiveArcs      []WritingArcBrief       `json:"active_arcs"`
	Timeline        WritingTimeline         `json:"timeline"`
	Reader          WritingReader           `json:"reader"`
	WritingSnapshot *WritingSnapshotBrief   `json:"writing_snapshot"`
	Scenes          []WritingSceneBrief     `json:"scenes"`
	Volume          *WritingVolume          `json:"volume,omitempty"`
	VolumeEntities  *WritingVolumeEntities  `json:"volume_entities,omitempty"`
	ItemOccurrences []WritingItemOccurrence `json:"item_occurrences"`
	Preview         *WritingPreview         `json:"preview,omitempty"` // 写前预览（请求章 >= 最新完成章时返回）
}

// WritingPreview 写前预览：当前卡"即将写入正文的设定"（待写章视角）。
// 状态层（到期伏笔/节点/上章延续，prepare 阶段即可查）+ 规划层（大纲存在性）。
type WritingPreview struct {
	ChapterNum        int                    `json:"chapter_num"` // last+1，待写章
	DueForeshadow     []WritingTimelineEntry `json:"due_foreshadow,omitempty"`
	OverdueForeshadow []WritingTimelineEntry `json:"overdue_foreshadow,omitempty"`
	DueArcNodes       []WritingArcNodeBrief  `json:"due_arc_nodes,omitempty"`
	PrevLocation      string                 `json:"prev_location,omitempty"`
	PrevChars         []int64                `json:"prev_chars,omitempty"`
	PrevItems         []string               `json:"prev_items,omitempty"`
	RecentSuspense    int                    `json:"recent_suspense"`
	HasOutline        bool                   `json:"has_outline"`
}

// WritingItemOccurrence 物品在章节中的流转记录（叙事面板"当前卡·物品"用）。
type WritingItemOccurrence struct {
	ItemID      int64  `json:"item_id"`
	ItemName    string `json:"item_name"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
	ChapterNum  int    `json:"chapter_num"`
}

type WritingChapter struct {
	ID        int64  `json:"id"`
	Num       int    `json:"num"`
	Title     string `json:"title"`
	WordCount int    `json:"word_count"`
}

type WritingChapterBrief struct {
	Num       int    `json:"num"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	KeyEvents string `json:"key_events"`
	WordCnt   int    `json:"word_cnt"`
	// 结构化实体字段（数据已存在，现在暴露给 AI）
	CharactersIn string `json:"characters_in"`
	ArcIDs       string `json:"arc_ids"`
}

type WritingCharacterBrief struct {
	ID        int64                 `json:"id"`
	Name      string                `json:"name"`
	Status    string                `json:"status"`
	Desc      string                `json:"desc"`
	Location  *WritingLocationBrief `json:"location,omitempty"`
	ItemCount int64                 `json:"item_count"`
	Items     []string              `json:"items"`
}

type WritingLocationBrief struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type WritingArcBrief struct {
	ID         int64                 `json:"id"`
	Name       string                `json:"name"`
	TypeZh     string                `json:"type_zh"`
	NodesDone  int64                 `json:"nodes_done"`
	NodesTotal int64                 `json:"nodes_total"`
	Nodes      []WritingArcNodeBrief `json:"nodes"`
}

type WritingArcNodeBrief struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	TargetCh    int    `json:"target_chapter"`
	ActualCh    int    `json:"actual_chapter"`
}

type WritingTimeline struct {
	Pending  []WritingTimelineEntry `json:"pending"`
	Resolved []WritingTimelineEntry `json:"resolved"`
	Overdue  []WritingTimelineEntry `json:"overdue"`
}

type WritingTimelineEntry struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	TargetChapter   int    `json:"target_chapter"`
	Importance      int    `json:"importance"`
	ResolvedChapter int64  `json:"resolved_chapter"`
	OverdueBy       int    `json:"overdue_by,omitempty"`
}

type WritingReader struct {
	Known         int                  `json:"known"`
	Suspense      int                  `json:"suspense"`
	Misconception int                  `json:"misconception"`
	Entries       []WritingReaderEntry `json:"entries"`
}

type WritingReaderEntry struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	PlantedCh  int    `json:"planted_chapter"`
	RevealedCh int    `json:"revealed_chapter"`
}

type WritingSnapshotBrief struct {
	LastChapterNum  int    `json:"last_chapter_num"`
	CurrentArcID    *int64 `json:"current_arc_id"`
	CurrentLocation string `json:"current_location"`
	ActiveChars     string `json:"active_chars"`
}

type WritingSceneBrief struct {
	ID           int64  `json:"id"`
	SceneNumber  int    `json:"scene_number"`
	Title        string `json:"title"`
	LocationName string `json:"location_name,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

// GetWritingContext 返回当前写作上下文，供叙事面板使用。
func (a *App) GetWritingContext(novelID int64, chapterNum int) (*WritingContext, error) {
	ctx := a.ctx

	// 当前章节
	ch := WritingChapter{Num: chapterNum}
	var chap chapter.Chapter
	if err := a.db.WithContext(ctx).Where("novel_id = ? AND chapter_number = ?", novelID, chapterNum).First(&chap).Error; err == nil {
		ch.ID = chap.ID
		ch.Title = chap.Title
		ch.WordCount = chap.WordCount
	}

	// 近5章摘要（以当前章节为锚点）
	cs := chapter.NewStore(a.db, a.logger)
	chs, _ := cs.GetRecentBefore(ctx, novelID, chapterNum, 5)
	var recent []WritingChapterBrief
	for _, c := range chs {
		recent = append(recent, WritingChapterBrief{
			Num:          c.ChapterNumber,
			Title:        c.Title,
			Summary:      c.Summary,
			KeyEvents:    c.KeyEvents,
			WordCnt:      c.WordCount,
			CharactersIn: c.CharactersIn,
			ArcIDs:       c.ArcIDs,
		})
	}

	// 卷纲查询：查找当前活跃的卷（放在角色查询之前，用于按卷过滤角色）
	var volume *WritingVolume
	var volumeEntities *WritingVolumeEntities
	var vol storyarc.StoryArc
	if err := a.db.WithContext(ctx).
		Where("novel_id = ? AND arc_type = 'volume' AND status = 'active'", novelID).
		Order("importance DESC").
		First(&vol).Error; err == nil {
		volume = &WritingVolume{
			Name:        vol.Name,
			Description: vol.Description,
			ArcType:     vol.ArcType,
			DetailJSON:  vol.DetailJSON,
		}
		// 卷级聚合：查卷范围内（start_chapter ~ end_chapter）涉及的所有实体
		if vol.StartChapter > 0 && vol.EndChapter >= vol.StartChapter {
			volumeEntities = a.buildVolumeEntities(ctx, novelID, vol)
		}
	}

	// 角色 + 所在地 + 持有物品数（按卷过滤，省 token）
	var chars []character.Character
	if volumeEntities != nil && len(volumeEntities.Characters) > 0 {
		ids := make([]int64, 0, len(volumeEntities.Characters))
		for _, c := range volumeEntities.Characters {
			ids = append(ids, c.ID)
		}
		a.db.WithContext(ctx).Where("novel_id = ? AND id IN ?", novelID, ids).Find(&chars)
	} else {
		// 无卷时返回全部角色（兼容老书）
		a.db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&chars)
	}
	var charBriefs []WritingCharacterBrief
	var deadNames []string
	for _, c := range chars {
		cb := WritingCharacterBrief{ID: c.ID, Name: c.Name, Status: c.Status, Desc: c.Description}
		if c.Status == "dead" {
			deadNames = append(deadNames, c.Name)
		}
		if c.LocationID != nil {
			var l location.Location
			if a.db.WithContext(ctx).First(&l, *c.LocationID).Error == nil {
				cb.Location = &WritingLocationBrief{ID: l.ID, Name: l.Name}
			}
		}
		a.db.WithContext(ctx).Model(&item.Item{}).Where("owner_id = ? AND novel_id = ?", c.ID, novelID).Count(&cb.ItemCount)
		// 物品名
		var items []item.Item
		a.db.WithContext(ctx).Select("name").Where("owner_id = ? AND novel_id = ?", c.ID, novelID).Find(&items)
		for _, it := range items {
			cb.Items = append(cb.Items, it.Name)
		}
		charBriefs = append(charBriefs, cb)
	}

	// 活跃弧线 + 节点统计
	var arcs []storyarc.StoryArc
	a.db.WithContext(ctx).Where("novel_id = ? AND status = 'active'", novelID).Find(&arcs)
	var arcBriefs []WritingArcBrief
	for _, ar := range arcs {
		var total, done int64
		a.db.WithContext(ctx).Model(&storyarc.ArcNode{}).Where("story_arc_id = ?", ar.ID).Count(&total)
		a.db.WithContext(ctx).Model(&storyarc.ArcNode{}).Where("story_arc_id = ? AND status = 'completed'", ar.ID).Count(&done)
		// 弧线节点详情
		var nodes []storyarc.ArcNode
		a.db.WithContext(ctx).Where("story_arc_id = ?", ar.ID).Order("target_chapter ASC").Limit(200).Find(&nodes)
		var nodeBriefs []WritingArcNodeBrief
		for _, n := range nodes {
			nodeBriefs = append(nodeBriefs, WritingArcNodeBrief{
				ID: n.ID, Title: n.Title, Description: n.Description,
				Status: n.Status, TargetCh: n.TargetChapter, ActualCh: n.ActualChapter,
			})
		}
		arcBriefs = append(arcBriefs, WritingArcBrief{
			ID: ar.ID, Name: ar.Name,
			TypeZh:     arcTypeZh(ar.ArcType),
			NodesDone:  done,
			NodesTotal: total,
			Nodes:      nodeBriefs,
		})
	}

	// 伏笔分类
	ts := timeline.NewStore(a.db, a.logger)
	tlEntries, _ := ts.ListByNovel(ctx, novelID, timeline.ListByNovelOptions{PageParams: storage.PageParams{Page: 1, Size: 100}})
	tl := WritingTimeline{
		Pending:  make([]WritingTimelineEntry, 0),
		Resolved: make([]WritingTimelineEntry, 0),
		Overdue:  make([]WritingTimelineEntry, 0),
	}
	if tlEntries != nil {
		for _, e := range tlEntries.Items {
			entry := WritingTimelineEntry{
				ID: e.ID, Title: e.Title, Status: e.Status,
				TargetChapter: e.TargetChapter, Importance: e.Importance,
				ResolvedChapter: e.ResolvedChapterID,
			}
			if e.Status == "resolved" {
				tl.Resolved = append(tl.Resolved, entry)
			} else {
				tl.Pending = append(tl.Pending, entry)
				if chapterNum > 0 && e.TargetChapter > 0 && e.TargetChapter < chapterNum {
					tl.Overdue = append(tl.Overdue, WritingTimelineEntry{
						ID: e.ID, Title: e.Title, TargetChapter: e.TargetChapter,
						Importance: e.Importance, OverdueBy: chapterNum - e.TargetChapter,
					})
				}
			}
		}
	}

	// 读者视角（计数 + 近2章条目详情）
	var knownCount int64
	a.db.WithContext(ctx).Model(&reader.ReaderPerspective{}).Where("novel_id = ? AND type = 'known'", novelID).Count(&knownCount)
	var activeReaders []reader.ReaderPerspective
	a.db.WithContext(ctx).Where("novel_id = ? AND (revealed_chapter = 0 OR revealed_chapter IS NULL)", novelID).Find(&activeReaders)
	suspenseCount, misconCount := 0, 0
	for _, r := range activeReaders {
		if r.Type == "suspense" {
			suspenseCount++
		}
		if r.Type == "misconception" {
			misconCount++
		}
	}
	var recentEntries []reader.ReaderPerspective
	a.db.WithContext(ctx).Where("novel_id = ? AND (revealed_chapter = 0 OR revealed_chapter IS NULL)", novelID).
		Order("planted_chapter DESC, id DESC").Limit(20).Find(&recentEntries)
	readerEntries := make([]WritingReaderEntry, 0, len(recentEntries))
	for _, e := range recentEntries {
		readerEntries = append(readerEntries, WritingReaderEntry{
			ID: e.ID, Type: e.Type, Content: e.Content,
			PlantedCh: e.PlantedChapter, RevealedCh: e.RevealedChapter,
		})
	}

	// 写作快照
	snapStore := writing.NewSnapshotStore(a.db, a.logger)
	snap, _ := snapStore.Get(ctx, novelID)
	var snapBrief *WritingSnapshotBrief
	if snap != nil {
		snapBrief = &WritingSnapshotBrief{
			LastChapterNum:  snap.LastChapterNum,
			CurrentArcID:    snap.CurrentArcID,
			CurrentLocation: snap.CurrentLocation,
			ActiveChars:     snap.ActiveChars,
		}
	}

	// 当前章节的场景列表 + 规划场景
	var sceneBriefs []WritingSceneBrief
	if ch.ID > 0 {
		var scenes []scene.Scene
		if vol.ID > 0 {
			a.db.WithContext(ctx).Raw(
				"SELECT * FROM scenes WHERE novel_id = ? AND (chapter_id = ? OR (chapter_id IS NULL AND arc_id = ?)) ORDER BY scene_number ASC",
				novelID, ch.ID, vol.ID,
			).Scan(&scenes)
		} else {
			a.db.WithContext(ctx).Where("novel_id = ? AND chapter_id = ?", novelID, ch.ID).Order("scene_number ASC").Find(&scenes)
		}
		for _, s := range scenes {
			sb := WritingSceneBrief{ID: s.ID, SceneNumber: s.SceneNumber, Title: s.Title, Summary: s.Summary}
			if s.LocationID != nil {
				var l location.Location
				if a.db.WithContext(ctx).First(&l, *s.LocationID).Error == nil {
					sb.LocationName = l.Name
				}
			}
			sceneBriefs = append(sceneBriefs, sb)
		}
	}

	// 当前章节的物品流转记录（叙事面板"当前卡·物品"：本章实际出现/易主/使用）
	var itemOccBriefs []WritingItemOccurrence
	if ch.ID > 0 {
		var occs []itemoccurrence.ItemOccurrence
		if err := a.db.WithContext(ctx).
			Where("novel_id = ? AND chapter_id = ?", novelID, ch.ID).
			Order("created_at DESC").Find(&occs).Error; err == nil && len(occs) > 0 {
			itemIDSet := map[int64]bool{}
			for _, o := range occs {
				itemIDSet[o.ItemID] = true
			}
			itemNames := map[int64]string{}
			if len(itemIDSet) > 0 {
				ids := make([]int64, 0, len(itemIDSet))
				for id := range itemIDSet {
					ids = append(ids, id)
				}
				var items []item.Item
				if a.db.WithContext(ctx).Where("novel_id = ? AND id IN ?", novelID, ids).Find(&items).Error == nil {
					for _, it := range items {
						itemNames[it.ID] = it.Name
					}
				}
			}
			for _, o := range occs {
				itemOccBriefs = append(itemOccBriefs, WritingItemOccurrence{
					ItemID:      o.ItemID,
					ItemName:    itemNames[o.ItemID],
					Action:      o.Action,
					Description: o.Description,
					ChapterNum:  ch.Num,
				})
			}
		}
	}

	// 写前预览：请求章 >= 最新完成章（或 0）时，返回"下一章要写什么"的设定摘要。
	// 数据源在 prepare 阶段即就绪（到期伏笔/节点是 DB 常驻状态，上一章延续已维护，大纲是文件系统），
	// 不依赖本章维护。
	var preview *WritingPreview
	if snap != nil && snap.LastChapterNum > 0 && (chapterNum == 0 || chapterNum >= snap.LastChapterNum) {
		next := snap.LastChapterNum + 1
		pv := &WritingPreview{ChapterNum: next, PrevLocation: snap.CurrentLocation}
		// 到期 / 超期伏笔
		if tlEntries != nil {
			for _, e := range tlEntries.Items {
				if e.Status == "resolved" || e.TargetChapter <= 0 {
					continue
				}
				entry := WritingTimelineEntry{
					ID: e.ID, Title: e.Title, Status: e.Status,
					TargetChapter: e.TargetChapter, Importance: e.Importance,
					ResolvedChapter: e.ResolvedChapterID,
				}
				if e.TargetChapter == next {
					pv.DueForeshadow = append(pv.DueForeshadow, entry)
				} else if e.TargetChapter < next {
					entry.OverdueBy = next - e.TargetChapter
					pv.OverdueForeshadow = append(pv.OverdueForeshadow, entry)
				}
			}
		}
		// 到期弧线节点（复用已加载的 arcBriefs.Nodes）
		for _, ar := range arcBriefs {
			for _, n := range ar.Nodes {
				if n.Status == "pending" && n.TargetCh == next {
					pv.DueArcNodes = append(pv.DueArcNodes, n)
				}
			}
		}
		// 上一章（N-1）的出场角色与物品流转（状态延续）
		var prevChap chapter.Chapter
		if err := a.db.WithContext(ctx).Where("novel_id = ? AND chapter_number = ?", novelID, next-1).First(&prevChap).Error; err == nil {
			pv.PrevChars = appParseInt64Array(prevChap.CharactersIn)
			var prevOccs []itemoccurrence.ItemOccurrence
			if err := a.db.WithContext(ctx).Where("novel_id = ? AND chapter_id = ?", novelID, prevChap.ID).Find(&prevOccs).Error; err == nil && len(prevOccs) > 0 {
				itemIDSet := map[int64]bool{}
				for _, o := range prevOccs {
					itemIDSet[o.ItemID] = true
				}
				if len(itemIDSet) > 0 {
					ids := make([]int64, 0, len(itemIDSet))
					for id := range itemIDSet {
						ids = append(ids, id)
					}
					var items []item.Item
					if a.db.WithContext(ctx).Where("novel_id = ? AND id IN ?", novelID, ids).Find(&items).Error == nil {
						seen := map[string]bool{}
						for _, it := range items {
							if !seen[it.Name] {
								seen[it.Name] = true
								pv.PrevItems = append(pv.PrevItems, it.Name)
							}
						}
					}
				}
			}
		}
		// 最近悬念（上一章起新种、未揭示）
		var recentSuspense int64
		a.db.WithContext(ctx).Model(&reader.ReaderPerspective{}).
			Where("novel_id = ? AND type = 'suspense' AND (revealed_chapter = 0 OR revealed_chapter IS NULL) AND planted_chapter >= ?", novelID, next-1).
			Count(&recentSuspense)
		pv.RecentSuspense = int(recentSuspense)
		// 大纲存在性（规划层）
		if outlinePath, err := git.ResolvePath(fmt.Sprintf("outlines/%03d.md", next), novelID); err == nil {
			if _, err := os.Stat(outlinePath); err == nil {
				pv.HasOutline = true
			}
		}
		preview = pv
	}

	return &WritingContext{
		Chapter:         ch,
		RecentChapters:  recent,
		Characters:      charBriefs,
		DeadCharacters:  deadNames,
		ActiveArcs:      arcBriefs,
		Timeline:        tl,
		Reader: WritingReader{
			Known: int(knownCount), Suspense: suspenseCount, Misconception: misconCount, Entries: readerEntries,
		},
		WritingSnapshot: snapBrief,
		Scenes:          sceneBriefs,
		Volume:          volume,
		VolumeEntities:  volumeEntities,
		ItemOccurrences: itemOccBriefs,
		Preview:         preview,
	}, nil
}

// buildVolumeEntities 查询时聚合当前卷（start~end 章）涉及的所有实体。
// 从已有表派生，不建缓存表，避免同步负担。
func (a *App) buildVolumeEntities(ctx context.Context, novelID int64, vol storyarc.StoryArc) *WritingVolumeEntities {
	db := a.db.WithContext(ctx)
	ve := &WritingVolumeEntities{
		Characters: []WritingVolumeEntity{},
		Items:      []WritingVolumeEntity{},
		Lore:       []WritingVolumeEntity{},
		Foreshadow: []WritingVolumeEntity{},
	}

	// 卷范围内的章节 ID
	var chIDs []int64
	db.Model(&chapter.Chapter{}).
		Where("novel_id = ? AND chapter_number BETWEEN ? AND ?", novelID, vol.StartChapter, vol.EndChapter).
		Pluck("id", &chIDs)
	if len(chIDs) == 0 {
		return ve
	}

	// 角色：从卷内章节的场景字符 ID 提取
	charIDSet := map[int64]bool{}
	var scenes []scene.Scene
	if err := db.Where("novel_id = ? AND chapter_id IN ?", novelID, chIDs).Find(&scenes).Error; err == nil {
		for _, sc := range scenes {
			for _, id := range appParseInt64Array(sc.CharacterIDs) {
				if id > 0 {
					charIDSet[id] = true
				}
			}
		}
	}
	if len(charIDSet) > 0 {
		var chars []character.Character
		ids := make([]int64, 0, len(charIDSet))
		for id := range charIDSet {
			ids = append(ids, id)
		}
		db.Where("novel_id = ? AND id IN ?", novelID, ids).Find(&chars)
		for _, c := range chars {
			ve.Characters = append(ve.Characters, WritingVolumeEntity{ID: c.ID, Name: c.Name})
		}
	}

	// 物品：从 item_occurrence（章节范围内出现过）
	var occs []itemoccurrence.ItemOccurrence
	if err := db.Where("novel_id = ? AND chapter_id IN ?", novelID, chIDs).Find(&occs).Error; err == nil {
		itemIDSet := map[int64]bool{}
		for _, o := range occs {
			if o.ItemID > 0 {
				itemIDSet[o.ItemID] = true
			}
		}
		if len(itemIDSet) > 0 {
			var items []item.Item
			ids := make([]int64, 0, len(itemIDSet))
			for id := range itemIDSet {
				ids = append(ids, id)
			}
			db.Where("novel_id = ? AND id IN ?", novelID, ids).Find(&items)
			for _, it := range items {
				ve.Items = append(ve.Items, WritingVolumeEntity{ID: it.ID, Name: it.Name})
			}
		}
	}

	// 设定：reveal_chapter_id 在卷范围内，或 arc_id 关联卷，或 arc_id 为空（全局根基设定，始终可见）
	var lores []lore.LoreEntry
	if err := db.Where("novel_id = ? AND (reveal_chapter_id IN ? OR arc_id = ? OR arc_id IS NULL)", novelID, chIDs, vol.ID).
		Find(&lores).Error; err == nil {
		for _, l := range lores {
			ve.Lore = append(ve.Lore, WritingVolumeEntity{ID: l.ID, Name: l.Title})
		}
	}

	// 伏笔：target_chapter 在卷范围内
	var tls []timeline.TimelineEntry
	if err := db.Where("novel_id = ? AND target_chapter BETWEEN ? AND ?", novelID, vol.StartChapter, vol.EndChapter).
		Find(&tls).Error; err == nil {
		for _, e := range tls {
			if e.ID > 0 {
				ve.Foreshadow = append(ve.Foreshadow, WritingVolumeEntity{ID: e.ID, Name: e.Title})
			}
		}
	}

	return ve
}

// appParseInt64Array 解析 "[1,5,12]" 格式的 JSON 数组到 []int64。
func appParseInt64Array(raw string) []int64 {
	if raw == "" {
		return nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}
