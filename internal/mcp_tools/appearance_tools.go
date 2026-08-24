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
	"novel/internal/novel"
	"novel/internal/outline"
	"novel/internal/scene"
	"novel/internal/storyarc"
	"novel/internal/timeline"
	"novel/internal/volume"
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
	db.WithContext(ctx).Where("novel_id = ?", novelID).Order("id DESC").Limit(200).Find(&scenes)
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
	CheckTypes     string `json:"check_types" jsonschema:"description=JSON数组，要执行的检查项：[\"foreshadow_overdue\",\"character_vanished\",\"item_conflict\",\"dead_appeared\",\"pacing_gap\",\"promise_fulfillment\",\"init_consistency\",\"ledger_integrity\",\"beat_window\",\"scope_guard\",\"type_drift\"]。留空=全部"`
	Lookback       int    `json:"lookback" jsonschema:"description=pacing_gap 回溯窗口章数，默认5"`
	MinGap         int    `json:"min_gap" jsonschema:"description=pacing_gap 连续无动作场景触发阈值，默认3"`
	Tolerance      int    `json:"tolerance" jsonschema:"description=promise_fulfillment 承诺章+tolerance后仍未兑现才报警，默认3"`
	Genre          string `json:"genre" jsonschema:"description=pacing_gap 题材类型，影响检测标签：xuanhuan(冲突+战斗)/suspense(推理+反转)/romance(告白+误会)/urban(职场+商战)/default(冲突+战斗)"`
}

type CheckStoryConsistencyTool struct{}

func (t *CheckStoryConsistencyTool) Name() string { return "check_story_consistency" }
func (t *CheckStoryConsistencyTool) Description() string {
	return "程序化一致性检查，用 SQL 实证返回十一类问题：\n" +
		"1. foreshadow_overdue：超过目标章仍未回收的伏笔（硬错误）\n" +
		"2. character_vanished：近30章未出场但有历史戏份的角色（出场断档，疑似被遗忘）\n" +
		"3. item_conflict：已销毁/丢失的物品在之后章节又出现（硬错误）\n" +
		"4. dead_appeared：已死亡（status=dead）的角色在死亡章节之后又被写入章节出场列表（硬错误，死者复出）\n" +
		"5. pacing_gap：连续多章无高密度场景，节奏拖沓（警告）\n" +
		"6. promise_fulfillment：卷纲承诺的大爽点到期未兑现（硬错误）\n" +
		"7. init_consistency：开书一致性校验（总纲/偏好/卷纲三方冲突，硬错误）\n" +
		"8. ledger_integrity：台账自检——伏笔 resolved_chapter 指向未来章节、弧线节点 completed 但 actual_chapter=0 等数据失真（硬错误）\n" +
		"9. beat_window：未来3章内到期的大爽点提醒（写前对齐，防止爽点被替换或顺延）\n" +
		"10. scope_guard：本章是否落在当前卷范围内、弧线节点相对规划的提前消耗/滞后（警告）\n" +
		"11. type_drift：回溯窗口内动作/冲突场景占比过低，类型方向漂移嫌疑（警告）\n" +
		"review/maintain/init 阶段调用，作为审稿的硬数据支撑。" +
		"【使用时机】审稿/每 3 章自检时调用（自动核对，输出问题条目）；发现问题后按条目定位修复。" +
		"【注意】检查是程序化 SQL 核对，不含文笔判断——文笔问题仍需人工审读。"
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

	checkTypes := map[string]bool{"foreshadow_overdue": true, "character_vanished": true, "item_conflict": true, "dead_appeared": true, "pacing_gap": true, "promise_fulfillment": true, "init_consistency": true, "ledger_integrity": true, "beat_window": true, "scope_guard": true, "type_drift": true}
	if a.CheckTypes != "" {
		checkTypes = map[string]bool{}
		for _, c := range parseJSONStringArray(a.CheckTypes) {
			checkTypes[c] = true
		}
	}
	lookback := a.Lookback
	if lookback <= 0 {
		lookback = 5
	}
	minGap := a.MinGap
	if minGap <= 0 {
		minGap = 3
	}
	tolerance := a.Tolerance
	if tolerance <= 0 {
		tolerance = 3
	}

	var findings []string
	var warnings []string

	if checkTypes["foreshadow_overdue"] {
		var overdue []timeline.TimelineEntry
		db.Where("novel_id = ? AND status = 'pending' AND target_chapter > 0 AND target_chapter < ?",
			tc.NovelID, a.CurrentChapter).Find(&overdue)
		if len(overdue) > 0 {
			for _, e := range overdue {
				findings = append(findings, fmt.Sprintf("[ERROR] 伏笔超期未回收：%s（目标第%d章，当前第%d章）", e.Title, e.TargetChapter, a.CurrentChapter))
			}
		}
	}

	if checkTypes["character_vanished"] {
		vanished := findVanishedCharacters(ctx, db, tc.NovelID, a.CurrentChapter)
		for _, v := range vanished {
			warnings = append(warnings, fmt.Sprintf("[WARNING] 角色出场断档：%s 近30章未出场（最近出场第%d章）", v.Name, v.LastChapter))
		}
	}

	if checkTypes["item_conflict"] {
		conflicts := findItemConflicts(ctx, db, tc.NovelID)
		for _, c := range conflicts {
			findings = append(findings, fmt.Sprintf("[ERROR] 物品状态冲突：%s 状态为%s，但之后章节仍出现", c.Name, c.Status))
		}
	}

	if checkTypes["dead_appeared"] {
		dead := findDeadAppeared(ctx, db, tc.NovelID, a.CurrentChapter)
		for _, d := range dead {
			findings = append(findings, fmt.Sprintf("[ERROR] 死者复出：%s 状态为 dead（第%d章死亡），但第%d章出场列表中仍包含该角色", d.Name, d.DeathChapter, d.AppearedChapter))
		}
	}

	if checkTypes["pacing_gap"] {
		pacingWarn := findPacingGap(ctx, db, tc.NovelID, a.CurrentChapter, lookback, minGap, a.Genre)
		if pacingWarn != "" {
			warnings = append(warnings, pacingWarn)
		}
	}

	if checkTypes["promise_fulfillment"] {
		promiseFinds := findPromiseUnfulfilled(ctx, db, tc.NovelID, a.CurrentChapter, tolerance)
		for _, f := range promiseFinds {
			findings = append(findings, f)
		}
	}

	if checkTypes["init_consistency"] {
		initFinds := findInitConsistency(ctx, db, tc.NovelID, a.Genre)
		for _, f := range initFinds {
			findings = append(findings, f)
		}
	}

	if checkTypes["ledger_integrity"] {
		ledgerFinds := findLedgerIntegrity(ctx, db, tc.NovelID, a.CurrentChapter)
		findings = append(findings, ledgerFinds...)
	}

	if checkTypes["beat_window"] {
		beatWarns := findBeatWindow(ctx, db, tc.NovelID, a.CurrentChapter)
		warnings = append(warnings, beatWarns...)
	}

	if checkTypes["scope_guard"] {
		scopeWarns := findScopeGuard(ctx, db, tc.NovelID, a.CurrentChapter)
		warnings = append(warnings, scopeWarns...)
	}

	if checkTypes["type_drift"] {
		driftWarn := findTypeDrift(ctx, db, tc.NovelID, a.CurrentChapter, a.Genre)
		if driftWarn != "" {
			warnings = append(warnings, driftWarn)
		}
	}

	var content string
	if len(findings) == 0 && len(warnings) == 0 {
		content = "✅ 一致性检查通过：未发现伏笔超期、角色断档、物品冲突、死者复出、节奏拖沓、承诺未兑现、开书冲突、台账失真、爽点临近未对齐、卷范围越界、类型漂移。"
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

	// 角色最后一次出场章节：单条 JOIN 查询只取三列（旧实现全量加载 scenes+chapters
	// 两个完整结构体切片，长篇几千章时每次调用内存与耗时线性膨胀）
	type sceneChRow struct {
		CharacterIDs string
		ChapterNum   int
	}
	var rows []sceneChRow
	db.WithContext(ctx).Raw(
		"SELECT s.character_ids AS character_ids, c.chapter_number AS chapter_num"+
			" FROM scenes s JOIN chapters c ON c.id = s.chapter_id"+
			" WHERE s.novel_id = ? AND s.chapter_id IS NOT NULL", novelID).Scan(&rows)

	lastSeen := map[int64]int{} // charID → chapter_number
	for _, r := range rows {
		for _, id := range parseJSONInt64Array(r.CharacterIDs) {
			if r.ChapterNum > lastSeen[id] {
				lastSeen[id] = r.ChapterNum
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
	var destroyed []item.Item
	db.WithContext(ctx).Where("novel_id = ? AND status IN ?", novelID, []string{"destroyed", "lost"}).Find(&destroyed)
	if len(destroyed) == 0 {
		return nil
	}

	// 收集所有状态变化章号
	chapterIDs := make([]int64, 0, len(destroyed))
	for _, it := range destroyed {
		if it.StatusChangedChapterID != nil {
			chapterIDs = append(chapterIDs, *it.StatusChangedChapterID)
		}
	}
	if len(chapterIDs) == 0 {
		return nil
	}

	// 批量查状态变化章的 chapter_number
	var chs []chapter.Chapter
	db.WithContext(ctx).Where("id IN ?", chapterIDs).Find(&chs)
	chNumMap := make(map[int64]int, len(chs))
	for _, ch := range chs {
		chNumMap[ch.ID] = ch.ChapterNumber
	}

	// 批量查后续章节 ID
	var laterChapters []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ? AND chapter_number > (SELECT MAX(chapter_number) FROM chapters WHERE id IN ?)", novelID, chapterIDs).Find(&laterChapters)
	if len(laterChapters) == 0 {
		return nil
	}
	laterIDs := make([]int64, 0, len(laterChapters))
	for _, lc := range laterChapters {
		laterIDs = append(laterIDs, lc.ID)
	}

	// 批量查 occurrence（哪些 destroyed items 在后续章节出现过）
	type occResult struct {
		ItemID int64
	}
	var occItems []occResult
	db.WithContext(ctx).Model(&itemoccurrence.ItemOccurrence{}).
		Select("DISTINCT item_id").
		Where("novel_id = ? AND item_id IN (SELECT id FROM items WHERE novel_id = ? AND status IN ('destroyed','lost')) AND chapter_id IN ?", novelID, novelID, laterIDs).
		Scan(&occItems)
	occSet := make(map[int64]bool, len(occItems))
	for _, o := range occItems {
		occSet[o.ItemID] = true
	}

	var conflicted []item.Item
	for _, it := range destroyed {
		if occSet[it.ID] {
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
// 单次遍历后续章节判定（旧实现按死亡角色逐个反查全部后续章节，O(死者数×章数)）；
// 每个角色只报最早一次复出章节，避免同一角色刷屏多条。
func findDeadAppeared(ctx context.Context, db *gorm.DB, novelID int64, currentChapter int) []deadAppeared {
	var deadChars []character.Character
	db.WithContext(ctx).Where("novel_id = ? AND status = 'dead' AND status_changed_chapter_id IS NOT NULL", novelID).Find(&deadChars)
	if len(deadChars) == 0 {
		return nil
	}

	// 章节 ID → 章节号 映射（用于 characters_in 反查章号）
	var chs []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ?", novelID).Select("id", "chapter_number").Find(&chs)
	chNumByID := map[int64]int{}
	for _, c := range chs {
		chNumByID[c.ID] = c.ChapterNumber
	}

	// 死亡角色：角色ID → 死亡章号 + 姓名
	deathChByID := map[int64]int{}
	nameByID := map[int64]string{}
	minDeathCh := currentChapter
	for _, ch := range deadChars {
		if ch.StatusChangedChapterID == nil {
			continue
		}
		deathChNum := chNumByID[*ch.StatusChangedChapterID]
		if deathChNum == 0 {
			continue
		}
		deathChByID[ch.ID] = deathChNum
		nameByID[ch.ID] = ch.Name
		if deathChNum < minDeathCh {
			minDeathCh = deathChNum
		}
	}
	if len(deathChByID) == 0 {
		return nil
	}

	// 单次遍历死亡章之后的章节，characters_in 命中即记录
	var out []deadAppeared
	seen := map[int64]bool{}
	var later []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ? AND chapter_number > ? AND chapter_number <= ?",
		novelID, minDeathCh-1, currentChapter).
		Select("id", "chapter_number", "characters_in").Order("chapter_number").Find(&later)
	for _, lc := range later {
		for _, id := range parseJSONInt64Array(lc.CharactersIn) {
			if seen[id] {
				continue
			}
			if deathCh, ok := deathChByID[id]; ok && lc.ChapterNumber > deathCh {
				seen[id] = true
				out = append(out, deadAppeared{Name: nameByID[id], DeathChapter: deathCh, AppearedChapter: lc.ChapterNumber})
			}
		}
	}
	return out
}

// findPacingGap 检测连续多章无高密度场景（节奏拖沓）。
// 通过 key_events 中的标签识别场景类型，根据题材动态调整检测标签。
func findPacingGap(ctx context.Context, db *gorm.DB, novelID int64, currentChapter, lookback, minGap int, genre string) string {
	// 根据题材选择检测标签
	actionTags := getActionTags(genre)

	startCh := currentChapter - lookback + 1
	if startCh < 1 {
		startCh = 1
	}
	var chapters []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ? AND chapter_number >= ? AND chapter_number <= ?",
		novelID, startCh, currentChapter).Order("chapter_number").Find(&chapters)

	if len(chapters) == 0 {
		return ""
	}

	// 从当前章往前统计连续无动作场景章数
	consecutiveNoAction := 0
	for i := len(chapters) - 1; i >= 0; i-- {
		ch := chapters[i]
		keyEvents := ch.KeyEvents
		if keyEvents == "" {
			consecutiveNoAction++
			continue
		}
		// 检查是否包含动作标签
		hasAction := false
		for _, tag := range actionTags {
			if strings.Contains(keyEvents, tag) {
				hasAction = true
				break
			}
		}
		if hasAction {
			break
		}
		consecutiveNoAction++
	}

	if consecutiveNoAction >= minGap {
		startIdx := currentChapter - consecutiveNoAction + 1
		genreHint := ""
		if genre != "" {
			genreHint = "（题材：" + genre + "）"
		}
		return fmt.Sprintf("[WARNING] 节奏拖沓：第%d-%d章连续%d章无高密度场景%s（回溯窗口%d章，阈值%d章）", startIdx, currentChapter, consecutiveNoAction, genreHint, lookback, minGap)
	}
	return ""
}

// getActionTags 根据题材返回检测标签
func getActionTags(genre string) []string {
	switch strings.ToLower(genre) {
	case "xuanhuan", "xianxia", "wuxia", "fantasy":
		// 玄幻/仙侠/武侠：冲突、战斗、碾压
		return []string{"[冲突]", "[战斗]", "[碾压]"}
	case "suspense", "mystery", "detective":
		// 悬疑/推理/侦探：推理、线索、反转、揭秘
		return []string{"[推理]", "[线索]", "[反转]", "[揭秘]"}
	case "romance", "love":
		// 言情：告白、误会、心动、冲突
		return []string{"[告白]", "[误会]", "[心动]", "[冲突]"}
	case "urban", "modern":
		// 都市：职场、商战、冲突
		return []string{"[职场]", "[商战]", "[冲突]"}
	case "horror":
		// 恐怖：惊吓、悬疑、冲突
		return []string{"[惊吓]", "[悬疑]", "[冲突]"}
	default:
		// 默认：冲突、战斗（适合动作类）
		return []string{"[冲突]", "[战斗]"}
	}
}

// findPromiseUnfulfilled 检查卷纲承诺的大爽点是否已兑现。
// 读取当前 volume 弧线的 detail_json.big_shuangdian，检查承诺章+tolerance后是否已发生。
func findPromiseUnfulfilled(ctx context.Context, db *gorm.DB, novelID int64, currentChapter, tolerance int) []string {
	var results []string

	// 查询当前活跃的卷（从 volumes 表）
	volStore := volume.NewStore(db)
	vols, err := volStore.ListByNovel(ctx, novelID)
	if err != nil || len(vols) == 0 {
		return nil
	}
	var currentVol *volume.Volume
	for i := range vols {
		if vols[i].StartChapter <= currentChapter && vols[i].EndChapter >= currentChapter {
			currentVol = &vols[i]
			break
		}
	}
	if currentVol == nil {
		return nil
	}

	// 解析 detail_json
	if currentVol.DetailJSON == "" {
		return nil
	}
	var detail struct {
		BigShuangdian []struct {
			Chapter int    `json:"chapter"`
			Desc    string `json:"desc"`
		} `json:"big_shuangdian"`
	}
	if err := json.Unmarshal([]byte(currentVol.DetailJSON), &detail); err != nil {
		return nil
	}

	for _, p := range detail.BigShuangdian {
		if p.Chapter <= 0 || p.Desc == "" {
			continue
		}
		// 承诺章 + tolerance < 当前章 才检查
		if p.Chapter+tolerance > currentChapter {
			continue
		}

		// 检查是否有对应的 completed arc_node（查 story_arcs 中同名卷的 arc_id）
		var arc storyarc.StoryArc
		var node storyarc.ArcNode
		db.WithContext(ctx).Where("novel_id = ? AND arc_type = 'volume' AND name = ?", novelID, currentVol.Name).First(&arc)
		err := db.WithContext(ctx).Where("story_arc_id = ? AND status = 'completed' AND actual_chapter >= ? AND actual_chapter <= ?",
			arc.ID, p.Chapter, currentChapter).First(&node).Error
		if err == nil {
			continue // 已兑现
		}

		// 检查最近3章 key_events 是否含承诺关键词
		var recent []chapter.Chapter
		db.WithContext(ctx).Where("novel_id = ? AND chapter_number >= ? AND chapter_number <= ?",
			novelID, currentChapter-3, currentChapter).Find(&recent)
		found := false
		for _, ch := range recent {
			if strings.Contains(ch.KeyEvents, p.Desc) || strings.Contains(ch.Summary, p.Desc) {
				found = true
				break
			}
		}
		if found {
			continue // 剧情已写但节点漏标
		}

		overdueBy := currentChapter - p.Chapter
		results = append(results, fmt.Sprintf("[ERROR] 承诺未兑现：第%d章承诺「%s」已过期%d章（当前第%d章，容差%d章）", p.Chapter, p.Desc, overdueBy, currentChapter, tolerance))
	}
	return results
}

// findInitConsistency 开书一致性校验：检查 outline_beats / preferences / story_arcs 三方冲突。
func findInitConsistency(ctx context.Context, db *gorm.DB, novelID int64, genre string) []string {
	var results []string

	// 获取 outline_beats
	var beats []outline.OutlineBeat
	db.WithContext(ctx).Where("novel_id = ? AND beat_type = 'shuangdian'", novelID).Order("chapter").Find(&beats)

	if len(beats) == 0 {
		return nil // 没有大爽点，跳过检查
	}

	// 获取卷的 detail_json.big_shuangdian（从 volumes 表）
	volStore := volume.NewStore(db)
	vols, err := volStore.ListByNovel(ctx, novelID)
	if err == nil && len(vols) > 0 {
		for _, v := range vols {
			if v.DetailJSON == "" {
				continue
			}
			var detail struct {
				BigShuangdian []struct {
					Chapter int    `json:"chapter"`
					Desc    string `json:"desc"`
				} `json:"big_shuangdian"`
			}
			if err := json.Unmarshal([]byte(v.DetailJSON), &detail); err == nil {
				// volume_beat_sync: outline_beats vs detail_json.big_shuangdian
				if len(detail.BigShuangdian) > 0 {
					dbChapters := make(map[int]bool)
					for _, b := range beats {
						dbChapters[b.Chapter] = true
					}
					for _, bd := range detail.BigShuangdian {
						if !dbChapters[bd.Chapter] {
							results = append(results, fmt.Sprintf("[ERROR] volume_beat_sync：卷纲第%d章「%s」在 outline_beats 中不存在", bd.Chapter, bd.Desc))
						}
					}
				}
			}
		}
	}

	// pref_conflict 检查已移除：原实现内层循环体为空（只 Contains 匹配无任何动作），
	// 节奏类偏好与 outline_beats 的复杂冲突需人工确认，程序化误报价值低

	// type_pacing: 大爽点间距检查（根据题材）
	if genre != "" {
		if len(beats) > 1 {
			for i := 1; i < len(beats); i++ {
				gap := beats[i].Chapter - beats[i-1].Chapter
				maxGap := 15 // 默认最大间距
				if genre == "suspense" || genre == "mystery" {
					maxGap = 10 // 悬疑节奏更快
				}
				if gap > maxGap {
					results = append(results, fmt.Sprintf("[WARNING] type_pacing：第%d章到第%d章间距%d章（题材：%s，建议最大%d章）",
						beats[i-1].Chapter, beats[i].Chapter, gap, genre, maxGap))
				}
			}
			// 首个大爽点位置检查
			if beats[0].Chapter > 8 {
				results = append(results, fmt.Sprintf("[WARNING] type_pacing：首个大爽点在第%d章（题材：%s，建议前8章内）",
					beats[0].Chapter, genre))
			}
		}
	}

	// golden_rule: 金手指 vs 世界观铁则
	var lores []lore.LoreEntry
	db.WithContext(ctx).Where("novel_id = ? AND category = '天道法则'", novelID).Find(&lores)
	if len(lores) > 0 {
		// 有世界观铁则，检查金手指是否存在
		var items []item.Item
		db.WithContext(ctx).Where("novel_id = ? AND item_type = '法宝'", novelID).Find(&items)
		if len(items) == 0 {
			results = append(results, "[WARNING] golden_rule：存在世界观铁则但无金手指物品（法宝），请确认金手指设定")
		}
	}

	// taboo_violation: 禁忌 vs 大爽点描述
	var prefs []novel.PreferenceItem
	db.WithContext(ctx).Where("novel_id = ?", novelID).Find(&prefs)
	for _, p := range prefs {
		if p.Category != "禁忌" && p.Category != "禁忌事项" {
			continue
		}
		for _, b := range beats {
			if strings.Contains(b.Description, p.Content) || strings.Contains(p.Content, b.Description) {
				results = append(results, fmt.Sprintf("[WARNING] taboo_violation：禁忌「%s」与大爽点「第%d章 %s」可能冲突",
					p.Content, b.Chapter, b.Description))
			}
		}
	}

	// means_power: 主角力量等级 vs 手段类型（需要人工确认，标记为 WARNING）
	if len(beats) > 0 {
		// 检查主角是否存在（优先读 personality.role，次选 description 关键词）
		var chars []character.Character
		db.WithContext(ctx).Where("novel_id = ? AND status = 'alive'", novelID).Find(&chars)
		protagonistFound := false
		for _, c := range chars {
			// 优先：personality JSON 中 role="主角"
			if c.Personality != "" {
				var p map[string]any
				if err := json.Unmarshal([]byte(c.Personality), &p); err == nil {
					if role, ok := p["role"].(string); ok && (strings.Contains(role, "主角") || strings.Contains(role, "protagonist")) {
						protagonistFound = true
						break
					}
				}
			}
			// 次选：description 关键词
			if strings.Contains(c.Description, "主角") || strings.Contains(c.Description, "protagonist") {
				protagonistFound = true
				break
			}
		}
		if !protagonistFound && len(chars) > 0 {
			results = append(results, "[INFO] means_power：未找到明确的主角标记（personality.role 或 description 含'主角'），请确认主角设定")
		}
	}

	// suspected_dead：角色 description 含死亡措辞但 status 非 dead（疑似死亡未标记）
	deathKeywords := []string{"焚毁", "灰烬", "毙命", "尸体", "死亡", "死去", "阵亡", "陨落", "消亡", "身死", "魂飞魄散", "形神俱灭"}
	var aliveChars []character.Character
	db.WithContext(ctx).Where("novel_id = ? AND status != 'dead'", novelID).Find(&aliveChars)
	for _, c := range aliveChars {
		for _, kw := range deathKeywords {
			if strings.Contains(c.Description, kw) {
				results = append(results, fmt.Sprintf("[WARNING] suspected_dead：角色「%s」description 含死亡措辞「%s」但 status=%s，建议确认是否标记为 dead", c.Name, kw, c.Status))
				break
			}
		}
	}

	return results
}

// ── 注册 ──────────────────────────────────────────────

func RegisterAppearanceTools(r *Registry) {
	r.Register(&GetEntityAppearancesTool{})
	r.Register(&CheckStoryConsistencyTool{})
}

// ── 防漂移检查（ledger_integrity / beat_window / scope_guard / type_drift）──
// 背景见 docs/archive/novel2-drift-audit-2026-08-23.md：渐进式设定漂移需要程序化
// 事实校验拦截，不能依赖 LLM 自觉。

// maxChapterNumber 返回当前已写最大章节号。
func maxChapterNumber(ctx context.Context, db *gorm.DB, novelID int64) int {
	var maxCh int64
	db.WithContext(ctx).Table("chapters").Where("novel_id = ?", novelID).
		Select("COALESCE(MAX(chapter_number), 0)").Scan(&maxCh)
	return int(maxCh)
}

// findLedgerIntegrity 台账自检：数据自身的一致性，不涉及剧情判断。
// 1) 伏笔 resolved_chapter_id 指向未写出的未来章节（事故案例：填了 34-51 而只写到 21 章）
// 2) 弧线节点 status=completed 但 actual_chapter=0（完成标记无证据）
func findLedgerIntegrity(ctx context.Context, db *gorm.DB, novelID int64, currentChapter int) []string {
	var results []string
	maxCh := maxChapterNumber(ctx, db, novelID)

	// JOIN chapters 将 resolved_chapter_id 解析为 chapter_number 再比较，
	// 避免 chapter_id(PK) 与 chapter_number 混淆导致假阳性。
	// 同时检测断裂引用（chapter_id 指向不存在的章节）。
	type badRow struct {
		Title          string
		ResolvedChapID int64
		ResolvedChapNum int64
	}
	var badFuture []badRow
	db.WithContext(ctx).
		Table("time_entries te").
		Select("te.title, te.resolved_chapter_id, c.chapter_number AS resolved_chap_num").
		Joins("JOIN chapters c ON c.id = te.resolved_chapter_id").
		Where("te.novel_id = ? AND te.status = 'resolved' AND te.resolved_chapter_id > 0 AND c.chapter_number > ?", novelID, maxCh).
		Order("te.id").Limit(20).Find(&badFuture)
	for _, r := range badFuture {
		results = append(results, fmt.Sprintf(
			"[ERROR] 台账失真：伏笔「%s」resolved_chapter_id=%d（第%d章）超过当前最大章节 %d——回收章号必须是实际已写的章节",
			r.Title, r.ResolvedChapID, r.ResolvedChapNum, maxCh))
	}

	var badBroken []struct {
		Title          string
		ResolvedChapID int64
	}
	db.WithContext(ctx).
		Table("time_entries te").
		Select("te.title, te.resolved_chapter_id").
		Joins("LEFT JOIN chapters c ON c.id = te.resolved_chapter_id AND c.novel_id = te.novel_id").
		Where("te.novel_id = ? AND te.status = 'resolved' AND te.resolved_chapter_id > 0 AND c.id IS NULL", novelID).
		Order("te.id").Limit(20).Find(&badBroken)
	for _, r := range badBroken {
		results = append(results, fmt.Sprintf(
			"[ERROR] 台账失真：伏笔「%s」resolved_chapter_id=%d 指向不存在的章节——请核对后修正或重置为 pending",
			r.Title, r.ResolvedChapID))
	}

	var badNodes []storyarc.ArcNode
	db.WithContext(ctx).
		Where("novel_id = ? AND status = 'completed' AND (actual_chapter = 0 OR actual_chapter > ?)",
			novelID, maxCh).Order("id").Limit(20).Find(&badNodes)
	for _, n := range badNodes {
		results = append(results, fmt.Sprintf(
			"[ERROR] 台账失真：弧线节点「%s」标记 completed 但 actual_chapter=%d 无效（当前最大章节 %d）——请核对正文后回填实际章节号",
			n.Title, n.ActualChapter, maxCh))
	}
	_ = currentChapter
	return results
}

// findBeatWindow 未来 window 章内到期的大爽点提醒：写前对齐，防止爽点被替换或顺延。
func findBeatWindow(ctx context.Context, db *gorm.DB, novelID int64, currentChapter int) []string {
	const window = 3
	var beats []outline.OutlineBeat
	if err := db.WithContext(ctx).
		Where("novel_id = ? AND chapter > ? AND chapter <= ?", novelID, currentChapter, currentChapter+window).
		Order("chapter").Find(&beats).Error; err != nil || len(beats) == 0 {
		return nil
	}
	parts := make([]string, 0, len(beats))
	for _, bt := range beats {
		desc := bt.Description
		if r := []rune(desc); len(r) > 40 {
			desc = string(r[:40]) + "…"
		}
		parts = append(parts, fmt.Sprintf("Ch%d「%s」", bt.Chapter, desc))
	}
	return []string{fmt.Sprintf("[WARNING] 爽点临近：接下来%d章内有大爽点到期——%s。本章及后续规划必须向其收敛，禁止替换核心承诺或顺延章号", window, strings.Join(parts, "、"))}
}

// findScopeGuard 卷范围与弧线推进对账。
// 1) 当前章不在任何卷范围内 → 卷数据缺失或越界，AI 失去范围约束（事故案例：volume_start=0 盲写）
// 2) pending 弧线节点 target 已过 3 章以上 → 规划滞后
// 3) 节点 actual 比 target 提前 5 章以上 → 后续卷冲突线被提前消耗
func findScopeGuard(ctx context.Context, db *gorm.DB, novelID int64, currentChapter int) []string {
	var results []string

	var vol volume.Volume
	err := db.WithContext(ctx).
		Where("novel_id = ? AND start_chapter <= ? AND end_chapter >= ?", novelID, currentChapter, currentChapter).
		Order("start_chapter").First(&vol).Error
	if err != nil {
		results = append(results, fmt.Sprintf(
			"[WARNING] 卷范围缺失：第%d章不属于任何卷（volumes 表缺少覆盖该章的记录），AI 失去本卷红线约束。请在卷纲中补齐当前卷的 start/end_chapter", currentChapter))
	}

	var nodes []storyarc.ArcNode
	db.WithContext(ctx).
		Where("novel_id = ? AND status = 'pending' AND target_chapter > 0 AND target_chapter < ?", novelID, currentChapter-3).
		Order("target_chapter").Limit(10).Find(&nodes)
	for _, n := range nodes {
		results = append(results, fmt.Sprintf(
			"[WARNING] 规划滞后：弧线节点「%s」目标第%d章但已到第%d章仍未发生——要么尽快兑现，要么明确改规划并更新 target_chapter",
			n.Title, n.TargetChapter, currentChapter))
	}

	var early []storyarc.ArcNode
	db.WithContext(ctx).
		Where("novel_id = ? AND actual_chapter > 0 AND target_chapter - actual_chapter > 5", novelID).
		Order("id").Limit(10).Find(&early)
	for _, n := range early {
		results = append(results, fmt.Sprintf(
			"[WARNING] 提前消耗：「%s」规划在第%d章、实际已在第%d章发生（提前%d章）——后续冲突线的节奏被压缩，确认是否为预期调整",
			n.Title, n.TargetChapter, n.ActualChapter, n.TargetChapter-n.ActualChapter))
	}
	return results
}

// findTypeDrift 类型方向漂移检测：回溯窗口内含动作/冲突场景的章节占比过低即报警。
// 这是 init-phase 一致性校验#1/#6（力量流题材连续多章无武力展示=违规）的运行期版本，
// 针对《祖国人》式"每章单独看都合理、累计后类型偷换"的渐进漂移。复用 pacing_gap 的标签体系。
func findTypeDrift(ctx context.Context, db *gorm.DB, novelID int64, currentChapter int, genre string) string {
	const lookback = 8
	const minActionChapters = 2 // 8 章窗口内至少 2 章含动作/冲突场景

	startCh := currentChapter - lookback + 1
	if startCh < 1 {
		startCh = 1
	}
	window := currentChapter - startCh + 1
	if window < 5 { // 前期样本不足不判
		return ""
	}

	actionTags := getActionTags(genre)
	var chapters []chapter.Chapter
	db.WithContext(ctx).Where("novel_id = ? AND chapter_number >= ? AND chapter_number <= ?",
		novelID, startCh, currentChapter).Find(&chapters)

	actionChapters := make([]int, 0, len(chapters))
	for _, ch := range chapters {
		for _, tag := range actionTags {
			if ch.KeyEvents != "" && strings.Contains(ch.KeyEvents, tag) {
				actionChapters = append(actionChapters, ch.ChapterNumber)
				break
			}
		}
	}

	if len(actionChapters) >= minActionChapters {
		return ""
	}
	hint := ""
	if len(actionChapters) > 0 {
		hint = fmt.Sprintf("仅第%s章含动作场景", intsToString(actionChapters))
	} else {
		hint = "全部章节零动作场景"
	}
	return fmt.Sprintf("[WARNING] 类型漂移嫌疑：最近%d章（第%d-%d章）%s，低于力量型叙事的下限（%d章）。"+
		"对照总纲的核心矛盾与主角手段承诺自查：是否正在用取证/心理战/社交博弈替代类型承诺的直接对抗？若是预期转向须先经用户确认并修订总纲",
		window, startCh, currentChapter, hint, minActionChapters)
}

// intsToString 把章节号列表格式化为逗号分隔字符串。
func intsToString(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, "/")
}
