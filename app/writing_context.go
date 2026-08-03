package app

import (
	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/item"
	"novel/internal/location"
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

// WritingContext 聚合叙事上下文，前端一次调用拿全部。
type WritingContext struct {
	Chapter         WritingChapter          `json:"chapter"`
	RecentChapters  []WritingChapterBrief   `json:"recent_chapters"`
	Characters      []WritingCharacterBrief `json:"characters"`
	ActiveArcs      []WritingArcBrief       `json:"active_arcs"`
	Timeline        WritingTimeline         `json:"timeline"`
	Reader          WritingReader           `json:"reader"`
	WritingSnapshot *WritingSnapshotBrief   `json:"writing_snapshot"`
	Scenes          []WritingSceneBrief     `json:"scenes"`
	Volume          *WritingVolume          `json:"volume,omitempty"`
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
}

type WritingCharacterBrief struct {
	ID        int64                 `json:"id"`
	Name      string                `json:"name"`
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
			Num:       c.ChapterNumber,
			Title:     c.Title,
			Summary:   c.Summary,
			KeyEvents: c.KeyEvents,
			WordCnt:   c.WordCount,
		})
	}

	// 角色 + 所在地 + 持有物品数
	var chars []character.Character
	a.db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&chars)
	var charBriefs []WritingCharacterBrief
	for _, c := range chars {
		cb := WritingCharacterBrief{ID: c.ID, Name: c.Name, Desc: c.Description}
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
		a.db.WithContext(ctx).Where("story_arc_id = ?", ar.ID).Order("target_chapter ASC").Find(&nodes)
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

	// 卷纲查询：查找当前活跃的卷
	var volume *WritingVolume
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
	}

	// 伏笔分类
	ts := timeline.NewStore(a.db, a.logger)
	tlEntries, _ := ts.ListByNovel(ctx, novelID, timeline.ListByNovelOptions{PageParams: storage.PageParams{Page: 1, Size: 20}})
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

	// 当前章节的场景列表
	var sceneBriefs []WritingSceneBrief
	if ch.ID > 0 {
		var scenes []scene.Scene
		a.db.WithContext(ctx).Where("chapter_id = ?", ch.ID).Order("scene_number ASC").Find(&scenes)
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

	return &WritingContext{
		Chapter:        ch,
		RecentChapters: recent,
		Characters:     charBriefs,
		ActiveArcs:     arcBriefs,
		Timeline:       tl,
		Reader: WritingReader{
			Known: int(knownCount), Suspense: suspenseCount, Misconception: misconCount, Entries: readerEntries,
		},
		WritingSnapshot: snapBrief,
		Scenes:          sceneBriefs,
		Volume:          volume,
	}, nil
}
