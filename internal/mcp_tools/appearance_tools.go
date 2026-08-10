package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/item"
	"novel/internal/itemoccurrence"
	"novel/internal/lore"
	"novel/internal/scene"
	"novel/internal/timeline"
)

// ── get_entity_appearances ─────────────────────────────
// 反查：实体 X 出现在哪些章节。解决"角色X第几章出场""物品X何时流转"等历史回溯查询。

type GetEntityAppearancesArgs struct {
	EntityType string `json:"entity_type" jsonschema:"required,description=实体类型,enum=character,enum=item,enum=location,enum=lore,enum=foreshadow" validate:"required,oneof=character item location lore foreshadow"`
	EntityID   int64  `json:"entity_id" jsonschema:"required,description=实体ID" validate:"required,min=1"`
	Limit      int    `json:"limit" jsonschema:"description=最多返回条数,default=20,minimum=1,maximum=100"`
}

type GetEntityAppearancesTool struct{}

func (t *GetEntityAppearancesTool) Name() string { return "get_entity_appearances" }
func (t *GetEntityAppearancesTool) Description() string {
	return "反查指定实体出现在哪些章节（历史回溯，最多返回 limit 条，默认 20 上限 100）。返回按章节号升序的出场记录。" +
		"【使用时机】①确认角色最后一次出场（防写死角色复活的错误）；②物品流转史核对；③设定揭示章/伏笔埋收章反查。" +
		"【省token】limit 默认 20 足够定位最近出场，不要传 100 拉全量——历史出场用 search_story_memory 或按需扩大 limit。"
}
func (t *GetEntityAppearancesTool) Category() ToolCategory { return CategoryMemoryRetrieval }
func (t *GetEntityAppearancesTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetEntityAppearancesArgs{})
}
func (t *GetEntityAppearancesTool) ExposeToLLM() bool { return true }
func (t *GetEntityAppearancesTool) NewArgs() any      { return &GetEntityAppearancesArgs{} }

func (t *GetEntityAppearancesTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetEntityAppearancesArgs)
	limit := a.Limit
	if limit <= 0 {
		limit = 20
	}
	db := tc.DB.WithContext(ctx)

	var records []map[string]any
	switch a.EntityType {
	case "character":
		records = characterAppearances(ctx, db, tc.NovelID, a.EntityID, limit)
	case "item":
		records = itemAppearances(ctx, db, tc.NovelID, a.EntityID, limit)
	case "location":
		records = locationAppearances(ctx, db, tc.NovelID, a.EntityID, limit)
	case "lore":
		records = loreAppearances(ctx, db, tc.NovelID, a.EntityID, limit)
	case "foreshadow":
		records = foreshadowAppearances(ctx, db, tc.NovelID, a.EntityID)
	}

	if len(records) == 0 {
		return &ToolResult{Success: true, Data: map[string]any{"content": "该实体没有任何出场记录。"}}, nil
	}

	// 按章节号升序
	sort.Slice(records, func(i, j int) bool {
		return records[i]["chapter_number"].(int) < records[j]["chapter_number"].(int)
	})

	var lines []string
	for _, r := range records {
		lines = append(lines, fmt.Sprintf("第%d章 %s — %s", r["chapter_number"], r["chapter_title"], r["context"]))
	}
	return &ToolResult{Success: true, Data: map[string]any{"content": strings.Join(lines, "\n")}}, nil
}

// chapterByIDMap 批量查章节号/标题映射。
func chapterByIDMap(ctx context.Context, db *gorm.DB, novelID int64, ids []int64) map[int64]chapter.Chapter {
	m := map[int64]chapter.Chapter{}
	if len(ids) == 0 {
		return m
	}
	var chs []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ? AND id IN ?", novelID, ids).Find(&chs)
	for _, c := range chs {
		m[c.ID] = c
	}
	return m
}

func characterAppearances(ctx context.Context, db *gorm.DB, novelID, charID int64, limit int) []map[string]any {
	var scenes []scene.Scene
	db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&scenes)
	chIDs := map[int64]bool{}
	contexts := map[int64]string{}
	for _, sc := range scenes {
		if sc.ChapterID == nil {
			continue
		}
		cid := *sc.ChapterID
		for _, id := range parseJSONInt64Array(sc.CharacterIDs) {
			if id == charID {
				chIDs[cid] = true
				if sc.Title != "" {
					contexts[cid] = "场景「" + sc.Title + "」出场"
				} else {
					contexts[cid] = "场景出场"
				}
				break
			}
		}
	}
	if len(chIDs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(chIDs))
	for id := range chIDs {
		ids = append(ids, id)
	}
	chMap := chapterByIDMap(ctx, db, novelID, ids)
	var out []map[string]any
	for id := range chIDs {
		ch, ok := chMap[id]
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"chapter_number": ch.ChapterNumber,
			"chapter_title":  ch.Title,
			"context":        contexts[id],
		})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func itemAppearances(ctx context.Context, db *gorm.DB, novelID, itemID int64, limit int) []map[string]any {
	var occs []itemoccurrence.ItemOccurrence
	db.WithContext(ctx).Where("novel_id = ? AND item_id = ?", novelID, itemID).Order("id DESC").Limit(limit).Find(&occs)
	if len(occs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(occs))
	for _, o := range occs {
		ids = append(ids, o.ChapterID)
	}
	chMap := chapterByIDMap(ctx, db, novelID, ids)
	var out []map[string]any
	for _, o := range occs {
		ch, ok := chMap[o.ChapterID]
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"chapter_number": ch.ChapterNumber,
			"chapter_title":  ch.Title,
			"context":        fmt.Sprintf("物品动作: %s", o.Action),
		})
	}
	return out
}

func locationAppearances(ctx context.Context, db *gorm.DB, novelID, locID int64, limit int) []map[string]any {
	var scenes []scene.Scene
	db.WithContext(ctx).Where("novel_id = ? AND location_id = ?", novelID, locID).Find(&scenes)
	if len(scenes) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(scenes))
	for _, sc := range scenes {
		if sc.ChapterID != nil {
			ids = append(ids, *sc.ChapterID)
		}
	}
	chMap := chapterByIDMap(ctx, db, novelID, ids)
	var out []map[string]any
	for _, sc := range scenes {
		if sc.ChapterID == nil {
			continue
		}
		ch, ok := chMap[*sc.ChapterID]
		if !ok {
			continue
		}
		ctx := "场景发生于此"
		if sc.Title != "" {
			ctx = "场景「" + sc.Title + "」"
		}
		out = append(out, map[string]any{
			"chapter_number": ch.ChapterNumber,
			"chapter_title":  ch.Title,
			"context":        ctx,
		})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func loreAppearances(ctx context.Context, db *gorm.DB, novelID, loreID int64, limit int) []map[string]any {
	var l lore.LoreEntry
	if err := db.WithContext(ctx).Where("novel_id = ? AND id = ?", novelID, loreID).First(&l).Error; err != nil || l.RevealChapterID == nil {
		return nil
	}
	var ch chapter.Chapter
	if err := db.WithContext(ctx).First(&ch, *l.RevealChapterID).Error; err != nil {
		return nil
	}
	return []map[string]any{{
		"chapter_number": ch.ChapterNumber,
		"chapter_title":  ch.Title,
		"context":        "设定首次揭示",
	}}
}

func foreshadowAppearances(ctx context.Context, db *gorm.DB, novelID, entryID int64) []map[string]any {
	var e timeline.TimelineEntry
	if err := db.WithContext(ctx).Where("novel_id = ? AND id = ?", novelID, entryID).First(&e).Error; err != nil {
		return nil
	}
	var out []map[string]any
	if e.SourceChapterID > 0 {
		var ch chapter.Chapter
		if db.WithContext(ctx).First(&ch, e.SourceChapterID).Error == nil {
			out = append(out, map[string]any{
				"chapter_number": ch.ChapterNumber, "chapter_title": ch.Title, "context": "伏笔埋设",
			})
		}
	}
	if e.TargetChapter > 0 {
		out = append(out, map[string]any{
			"chapter_number": e.TargetChapter, "chapter_title": "", "context": fmt.Sprintf("目标回收章（状态: %s）", e.Status),
		})
	}
	if e.ResolvedChapterID > 0 {
		var ch chapter.Chapter
		if db.WithContext(ctx).First(&ch, e.ResolvedChapterID).Error == nil {
			out = append(out, map[string]any{
				"chapter_number": ch.ChapterNumber, "chapter_title": ch.Title, "context": "伏笔回收",
			})
		}
	}
	return out
}

// ── check_story_consistency ────────────────────────────
// 程序化一致性检查：用 SQL 实证替代"感觉"，给 review agent 提供硬数据。

type CheckStoryConsistencyArgs struct {
	CurrentChapter int    `json:"current_chapter" jsonschema:"required,description=当前章节号" validate:"required,min=1"`
	CheckTypes     string `json:"check_types" jsonschema:"description=JSON数组，要执行的检查项：[\"foreshadow_overdue\",\"character_vanished\",\"item_conflict\",\"dead_appeared\"]。留空=全部"`
}

type CheckStoryConsistencyTool struct{}

func (t *CheckStoryConsistencyTool) Name() string { return "check_story_consistency" }
func (t *CheckStoryConsistencyTool) Description() string {
	return "程序化一致性检查，用 SQL 实证返回四类问题：\n" +
		"1. foreshadow_overdue：超过目标章仍未回收的伏笔（硬错误）\n" +
		"2. character_vanished：近30章未出场但有历史戏份的角色（出场断档，疑似被遗忘）\n" +
		"3. item_conflict：已销毁/丢失的物品在之后章节又出现（硬错误）\n" +
		"4. dead_appeared：已死亡（status=dead）的角色在死亡章节之后又被写入章节出场列表（硬错误，死者复出）\n" +
		"review 阶段调用，作为审稿的硬数据支撑。" +
		"【使用时机】审稿/每 3 章自检时调用（自动核对，输出问题条目）；发现问题后按条目定位修复。" +
		"【注意】检查是程序化 SQL 核对，不含文笔/节奏判断——文笔问题仍需人工审读。"
}
func (t *CheckStoryConsistencyTool) Category() ToolCategory { return CategoryConsistencyCheck }
func (t *CheckStoryConsistencyTool) JSONSchema() json.RawMessage {
	return SchemaOf(CheckStoryConsistencyArgs{})
}
func (t *CheckStoryConsistencyTool) ExposeToLLM() bool { return true }
func (t *CheckStoryConsistencyTool) NewArgs() any      { return &CheckStoryConsistencyArgs{} }

func (t *CheckStoryConsistencyTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CheckStoryConsistencyArgs)
	db := tc.DB.WithContext(ctx)

	checkTypes := map[string]bool{"foreshadow_overdue": true, "character_vanished": true, "item_conflict": true, "dead_appeared": true}
	if a.CheckTypes != "" {
		checkTypes = map[string]bool{}
		for _, c := range parseJSONStringArray(a.CheckTypes) {
			checkTypes[c] = true
		}
	}

	var findings []string
	var warnings []string

	if checkTypes["foreshadow_overdue"] {
		var overdue []timeline.TimelineEntry
		db.Where("novel_id = ? AND status = 'pending' AND target_chapter > 0 AND target_chapter < ?",
			tc.NovelID, a.CurrentChapter).Find(&overdue)
		if len(overdue) > 0 {
			for _, e := range overdue {
				findings = append(findings, fmt.Sprintf("🔴 伏笔超期未回收：%s（目标第%d章，当前第%d章）", e.Title, e.TargetChapter, a.CurrentChapter))
			}
		}
	}

	if checkTypes["character_vanished"] {
		vanished := findVanishedCharacters(ctx, db, tc.NovelID, a.CurrentChapter)
		for _, v := range vanished {
			warnings = append(warnings, fmt.Sprintf("🟡 角色出场断档：%s 近30章未出场（最近出场第%d章）", v.Name, v.LastChapter))
		}
	}

	if checkTypes["item_conflict"] {
		conflicts := findItemConflicts(ctx, db, tc.NovelID)
		for _, c := range conflicts {
			findings = append(findings, fmt.Sprintf("🔴 物品状态冲突：%s 状态为%s，但之后章节仍出现", c.Name, c.Status))
		}
	}

	if checkTypes["dead_appeared"] {
		dead := findDeadAppeared(ctx, db, tc.NovelID, a.CurrentChapter)
		for _, d := range dead {
			findings = append(findings, fmt.Sprintf("🔴 死者复出：%s 状态为 dead（第%d章死亡），但第%d章出场列表中仍包含该角色", d.Name, d.DeathChapter, d.AppearedChapter))
		}
	}

	var content string
	if len(findings) == 0 && len(warnings) == 0 {
		content = "✅ 一致性检查通过：未发现伏笔超期、角色断档、物品冲突、死者复出。"
	} else {
		parts := append([]string{}, findings...)
		parts = append(parts, warnings...)
		content = strings.Join(parts, "\n")
	}
	return &ToolResult{Success: true, Data: map[string]any{"content": content}}, nil
}

type vanishedChar struct {
	Name        string
	LastChapter int
}

func findVanishedCharacters(ctx context.Context, db *gorm.DB, novelID int64, currentChapter int) []vanishedChar {
	// 近30章的章节 ID
	var recentChapters []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ? AND chapter_number >= ?", novelID, currentChapter-30).
		Find(&recentChapters)
	recentIDs := map[int64]bool{}
	for _, c := range recentChapters {
		recentIDs[c.ID] = true
	}

	// 近30章出现的角色
	recentChars := map[int64]bool{}
	var recentScenes []scene.Scene
	if len(recentIDs) > 0 {
		ids := make([]int64, 0, len(recentIDs))
		for id := range recentIDs {
			ids = append(ids, id)
		}
		db.WithContext(ctx).Where("novel_id = ? AND chapter_id IN ?", novelID, ids).Find(&recentScenes)
		for _, sc := range recentScenes {
			for _, id := range parseJSONInt64Array(sc.CharacterIDs) {
				recentChars[id] = true
			}
		}
	}

	// 角色最后一次出场章节
	lastSeen := map[int64]int{} // charID → chapter_number
	var allScenes []scene.Scene
	db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&allScenes)
	// 建立 chapter_id → chapter_number
	allChIDs := map[int64]int{}
	var chs []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&chs)
	for _, c := range chs {
		allChIDs[c.ID] = c.ChapterNumber
	}
	for _, sc := range allScenes {
		if sc.ChapterID == nil {
			continue
		}
		chNum := allChIDs[*sc.ChapterID]
		for _, id := range parseJSONInt64Array(sc.CharacterIDs) {
			if chNum > lastSeen[id] {
				lastSeen[id] = chNum
			}
		}
	}

	// 有历史戏份（出场过）但近30章没出现，且最近出场 < currentChapter-30
	var out []vanishedChar
	if len(lastSeen) == 0 {
		return out
	}
	ids := make([]int64, 0, len(lastSeen))
	for id := range lastSeen {
		ids = append(ids, id)
	}
	var chars []character.Character
	db.WithContext(ctx).Where("novel_id = ? AND id IN ?", novelID, ids).Find(&chars)
	charMap := map[int64]character.Character{}
	for _, c := range chars {
		charMap[c.ID] = c
	}
	for id, lastCh := range lastSeen {
		if lastCh > 0 && lastCh <= currentChapter-30 && !recentChars[id] {
			if c, ok := charMap[id]; ok {
				out = append(out, vanishedChar{Name: c.Name, LastChapter: lastCh})
			}
		}
	}
	return out
}

func findItemConflicts(ctx context.Context, db *gorm.DB, novelID int64) []item.Item {
	var conflicted []item.Item
	var destroyed []item.Item
	db.WithContext(ctx).Where("novel_id = ? AND status IN ?", novelID, []string{"destroyed", "lost"}).Find(&destroyed)
	for _, it := range destroyed {
		if it.StatusChangedChapterID == nil {
			continue
		}
		var ch chapter.Chapter
		if err := db.WithContext(ctx).First(&ch, *it.StatusChangedChapterID).Error; err != nil {
			continue
		}
		// 状态变化章之后的章节
		var laterChapters []chapter.Chapter
		db.WithContext(ctx).Where("novel_id = ? AND chapter_number > ?", novelID, ch.ChapterNumber).Find(&laterChapters)
		if len(laterChapters) == 0 {
			continue
		}
		chIDs := make([]int64, 0, len(laterChapters))
		for _, lc := range laterChapters {
			chIDs = append(chIDs, lc.ID)
		}
		var cnt int64
		db.WithContext(ctx).Model(&itemoccurrence.ItemOccurrence{}).
			Where("novel_id = ? AND item_id = ? AND chapter_id IN ?", novelID, it.ID, chIDs).
			Count(&cnt)
		if cnt > 0 {
			conflicted = append(conflicted, it)
		}
	}
	return conflicted
}

// parseJSONStringArray 解析 JSON 字符串数组。
func parseJSONStringArray(raw string) []string {
	if raw == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil
	}
	return arr
}

// deadAppeared 记录死者复出证据。
type deadAppeared struct {
	Name            string
	DeathChapter    int
	AppearedChapter int
}

// findDeadAppeared 查找已死亡角色在死亡章节之后仍被写入章节出场列表（characters_in）的情况。
// characters_in 是 maintain 阶段每章回写的 JSON 数组（如 [127,128,129]），
// status=dead 且 status_changed_chapter_id 明确的角色出现在更晚章节的 characters_in 中即为死者复出。
func findDeadAppeared(ctx context.Context, db *gorm.DB, novelID int64, currentChapter int) []deadAppeared {
	var deadChars []character.Character
	db.WithContext(ctx).Where("novel_id = ? AND status = 'dead' AND status_changed_chapter_id IS NOT NULL", novelID).Find(&deadChars)
	if len(deadChars) == 0 {
		return nil
	}

	// 章节 ID → 章节号 映射（用于 characters_in 反查章号）
	var chs []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&chs)
	chNumByID := map[int64]int{}
	for _, c := range chs {
		chNumByID[c.ID] = c.ChapterNumber
	}

	var out []deadAppeared
	for _, ch := range deadChars {
		if ch.StatusChangedChapterID == nil {
			continue
		}
		deathChNum := chNumByID[*ch.StatusChangedChapterID]
		if deathChNum == 0 {
			continue
		}
		// 死亡章之后的章节，characters_in 中含该角色 ID → 死者复出
		var later []chapter.Chapter
		db.WithContext(ctx).Where("novel_id = ? AND chapter_number > ?", novelID, deathChNum).
			Select("id", "chapter_number", "characters_in").Find(&later)
		for _, lc := range later {
			for _, id := range parseJSONInt64Array(lc.CharactersIn) {
				if id == ch.ID {
					out = append(out, deadAppeared{Name: ch.Name, DeathChapter: deathChNum, AppearedChapter: lc.ChapterNumber})
					break
				}
			}
		}
	}
	return out
}

// ── 注册 ──────────────────────────────────────────────

func RegisterAppearanceTools(r *Registry) {
	r.Register(&GetEntityAppearancesTool{})
	r.Register(&CheckStoryConsistencyTool{})
}
