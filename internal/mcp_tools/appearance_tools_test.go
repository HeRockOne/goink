package mcp_tools

import (
	"context"
	"encoding/json"
	"testing"

	"novel/internal/chapter"
	"novel/internal/outline"
	"novel/internal/storyarc"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&chapter.Chapter{}, &storyarc.StoryArc{}, &storyarc.ArcNode{}, &outline.OutlineBeat{})
	return db
}

// ── pacing_gap 演示 ──────────────────────────────────────

func TestPacingGap_Demo(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// === 故事情节：主角在学院修炼 ===
	// 第1章：入学考核（有冲突）
	// 第2章：认识室友小明（日常）
	// 第3章：图书馆查阅秘籍（探索）
	// 第4章：食堂吃饭聊天（对话）
	// 第5章：宿舍休息（日常）
	// 第6章：课堂听课（日常）
	// 第7章：又在食堂（对话）
	// → 连续5章无冲突/战斗，节奏拖沓！

	chapters := []chapter.Chapter{
		{NovelID: novelID, ChapterNumber: 1, Title: "入学考核",
			KeyEvents: "[冲突]与守卫比武通过考核"},
		{NovelID: novelID, ChapterNumber: 2, Title: "认识室友",
			KeyEvents: "[日常]与室友小明聊天介绍学院"},
		{NovelID: novelID, ChapterNumber: 3, Title: "图书馆",
			KeyEvents: "[探索]在图书馆发现一本古老功法"},
		{NovelID: novelID, ChapterNumber: 4, Title: "食堂风波",
			KeyEvents: "[对话]与学长讨论修炼心得"},
		{NovelID: novelID, ChapterNumber: 5, Title: "夜修",
			KeyEvents: "[日常]在宿舍打坐修炼"},
		{NovelID: novelID, ChapterNumber: 6, Title: "早课",
			KeyEvents: "[日常]听长老讲授功法原理"},
		{NovelID: novelID, ChapterNumber: 7, Title: "再遇学长",
			KeyEvents: "[对话]食堂再遇学长继续讨论"},
	}
	for _, ch := range chapters {
		db.Create(&ch)
	}

	// === 测试 pacing_gap ===
	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 7,
		CheckTypes:     `["pacing_gap"]`,
		Lookback:       5,
		MinGap:         3,
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	content := result.Data["content"].(string)

	t.Logf("=== pacing_gap 检查结果 ===")
	t.Logf("输入：第1章(冲突) → 第2-7章(日常/探索/对话)")
	t.Logf("检查：最近5章(3-7)连续无[冲突][战斗]标签")
	t.Logf("结果：%s", content)

	// 验证：应该触发警告（连续5章无动作场景）
	if !contains(content, "[WARNING] 节奏拖沓") {
		t.Errorf("应该触发节奏拖沓警告，但没有")
	}
	t.Logf("✓ 正确触发了 pacing_gap 警告")
}

// ── promise_fulfillment 演示 ──────────────────────────────

func TestPromiseFulfillment_Demo(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// === 故事情节：修仙小说卷纲承诺 ===
	// 卷纲承诺：第5章主角碾压守卫（大爽点）
	// 实际：写到第10章还没兑现

	// 创建卷弧线（含承诺）
	detailJSON := map[string]any{
		"big_shuangdian": []map[string]any{
			{"chapter": 5, "desc": "碾压守卫展示实力"},
			{"chapter": 8, "desc": "击败天才学员"},
			{"chapter": 12, "desc": "获得长老认可"},
		},
	}
	detailBytes, _ := json.Marshal(detailJSON)
	vol := storyarc.StoryArc{
		NovelID:      novelID,
		Name:         "第一卷：学院崛起",
		ArcType:      "volume",
		Status:       "active",
		DetailJSON:   string(detailBytes),
		StartChapter: 1,
		EndChapter:   20,
	}
	db.Create(&vol)

	// 创建章节（第1-12章，但没有兑现承诺）
	for i := 1; i <= 12; i++ {
		ch := chapter.Chapter{
			NovelID:       novelID,
			ChapterNumber: i,
			Title:         "第" + string(rune('0'+i)) + "章",
			KeyEvents:     "[日常]日常修炼",
		}
		db.Create(&ch)
	}

	// 注意：没有创建任何 arc_node（承诺未兑现）

	// === 测试 promise_fulfillment ===
	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 12,
		CheckTypes:     `["promise_fulfillment"]`,
		Tolerance:      3,
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	data := result.Data
	content := data["content"].(string)

	t.Logf("=== promise_fulfillment 检查结果 ===")
	t.Logf("卷纲承诺：第5章碾压守卫、第8章击败天才、第12章获长老认可")
	t.Logf("实际：第1-12章全部是[日常]修炼，无兑现记录")
	t.Logf("当前：第12章，容差3章")
	t.Logf("结果：%s", content)

	// 验证：应该触发硬错误（承诺第5章，当前第12章，超期7章）
	if !contains(content, "承诺未兑现") {
		t.Errorf("应该触发承诺未兑现错误，但没有")
	}
	if !contains(content, "碾压守卫") {
		t.Errorf("应该提到具体承诺内容")
	}
	t.Logf("✓ 正确触发了 promise_fulfillment 硬错误")
}

// ── 综合演示：两个检查同时运行 ──────────────────────────────

func TestCombined_Demo(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// === 故事情节：悬疑小说 ===
	// 卷纲承诺：第6章揭露真凶
	// 实际：第1-8章全是对话推理，无冲突，承诺也未兑现

	detailJSON := map[string]any{
		"big_shuangdian": []map[string]any{
			{"chapter": 6, "desc": "揭露真凶身份"},
		},
	}
	detailBytes, _ := json.Marshal(detailJSON)
	vol := storyarc.StoryArc{
		NovelID:      novelID,
		Name:         "第一卷：迷雾重重",
		ArcType:      "volume",
		Status:       "active",
		DetailJSON:   string(detailBytes),
		StartChapter: 1,
		EndChapter:   15,
	}
	db.Create(&vol)

	for i := 1; i <= 8; i++ {
		ch := chapter.Chapter{
			NovelID:       novelID,
			ChapterNumber: i,
			Title:         "第" + string(rune('0'+i)) + "章",
			KeyEvents:     "[对话]与嫌疑人博弈",
		}
		db.Create(&ch)
	}

	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 8,
		CheckTypes:     `["pacing_gap","promise_fulfillment"]`,
		Lookback:       5,
		MinGap:         3,
		Tolerance:      2,
		Genre:          "suspense",
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	data := result.Data
	content := data["content"].(string)

	t.Logf("=== 综合检查结果 ===")
	t.Logf("情节：悬疑小说，连续8章全是[对话]推理")
	t.Logf("承诺：第6章揭露真凶（已过期2章，容差2章）")
	t.Logf("结果：\n%s", content)

	// 应该同时触发两个问题
	if !contains(content, "[WARNING] 节奏拖沓") {
		t.Error("应该触发 pacing_gap")
	}
	if !contains(content, "[ERROR] 承诺未兑现") {
		t.Error("应该触发 promise_fulfillment")
	}
	t.Logf("✓ 同时触发了 pacing_gap 警告 + promise_fulfillment 硬错误")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ── 不同题材测试 ──────────────────────────────────────────

func TestPacingGap_RomanceGenre(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// 言情小说：连续4章没有[告白][误会][心动][冲突]
	chapters := []chapter.Chapter{
		{NovelID: novelID, ChapterNumber: 1, Title: "初遇", KeyEvents: "[心动]咖啡厅偶遇"},
		{NovelID: novelID, ChapterNumber: 2, Title: "日常", KeyEvents: "[日常]上班"},
		{NovelID: novelID, ChapterNumber: 3, Title: "日常", KeyEvents: "[日常]下班"},
		{NovelID: novelID, ChapterNumber: 4, Title: "日常", KeyEvents: "[日常]周末"},
		{NovelID: novelID, ChapterNumber: 5, Title: "日常", KeyEvents: "[日常]假期"},
	}
	for _, ch := range chapters {
		db.Create(&ch)
	}

	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 5,
		CheckTypes:     `["pacing_gap"]`,
		Lookback:       5,
		MinGap:         3,
		Genre:          "romance",
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	content := result.Data["content"].(string)
	t.Logf("=== 言情题材 pacing_gap ===")
	t.Logf("结果：%s", content)

	if !contains(content, "[WARNING] 节奏拖沓") {
		t.Error("应该触发节奏拖沓")
	}
	t.Logf("✓ 言情题材正确检测到连续无情感场景")
}

func TestPacingGap_SuspenseGenre(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// 悬疑小说：连续3章没有[推理][线索][反转][揭秘]
	chapters := []chapter.Chapter{
		{NovelID: novelID, ChapterNumber: 1, Title: "案件", KeyEvents: "[推理]分析现场"},
		{NovelID: novelID, ChapterNumber: 2, Title: "日常", KeyEvents: "[日常]吃饭"},
		{NovelID: novelID, ChapterNumber: 3, Title: "日常", KeyEvents: "[日常]睡觉"},
		{NovelID: novelID, ChapterNumber: 4, Title: "日常", KeyEvents: "[日常]散步"},
	}
	for _, ch := range chapters {
		db.Create(&ch)
	}

	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 4,
		CheckTypes:     `["pacing_gap"]`,
		Lookback:       5,
		MinGap:         3,
		Genre:          "suspense",
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	content := result.Data["content"].(string)
	t.Logf("=== 悬疑题材 pacing_gap ===")
	t.Logf("结果：%s", content)

	if !contains(content, "[WARNING] 节奏拖沓") {
		t.Error("应该触发节奏拖沓")
	}
	t.Logf("✓ 悬疑题材正确检测到连续无推理/线索场景")
}

func TestPacingGap_XuanhuanNoTrigger(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// 玄幻小说：连续有冲突场景，不触发
	chapters := []chapter.Chapter{
		{NovelID: novelID, ChapterNumber: 1, KeyEvents: "[冲突]入门考核"},
		{NovelID: novelID, ChapterNumber: 2, KeyEvents: "[战斗]比武招亲"},
		{NovelID: novelID, ChapterNumber: 3, KeyEvents: "[冲突]守护宝藏"},
	}
	for _, ch := range chapters {
		db.Create(&ch)
	}

	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 3,
		CheckTypes:     `["pacing_gap"]`,
		Lookback:       5,
		MinGap:         3,
		Genre:          "xuanhuan",
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	content := result.Data["content"].(string)
	t.Logf("=== 玄幻题材 pacing_gap（无触发） ===")
	t.Logf("结果：%s", content)

	if contains(content, "[WARNING] 节奏拖沓") {
		t.Error("不应该触发节奏拖沓")
	}
	t.Logf("✓ 玄幻题材连续有冲突/战斗场景，正确不触发")
}

// ── init_consistency 测试 ──────────────────────────────────

func TestInitConsistency_FileDBSync(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// 创建 outline_beats（数据库中的大爽点）
	db.Create(&outline.OutlineBeat{NovelID: novelID, Chapter: 5, Description: "碾压守卫", BeatType: "shuangdian"})
	db.Create(&outline.OutlineBeat{NovelID: novelID, Chapter: 10, Description: "击败天才", BeatType: "shuangdian"})

	// 创建卷弧线，detail_json.big_shuangdian 包含一个数据库中不存在的章号
	detailJSON := map[string]any{
		"big_shuangdian": []map[string]any{
			{"chapter": 5, "desc": "碾压守卫"},
			{"chapter": 10, "desc": "击败天才"},
			{"chapter": 15, "desc": "获得长老认可"}, // 这个在 outline_beats 中不存在
		},
	}
	detailBytes, _ := json.Marshal(detailJSON)
	db.Create(&storyarc.StoryArc{
		NovelID:    novelID,
		Name:       "第一卷",
		ArcType:    "volume",
		Status:     "active",
		DetailJSON: string(detailBytes),
	})

	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 1,
		CheckTypes:     `["init_consistency"]`,
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	content := result.Data["content"].(string)
	t.Logf("=== init_consistency file_db_sync 测试 ===")
	t.Logf("outline_beats: 第5章, 第10章")
	t.Logf("detail_json: 第5章, 第10章, 第15章")
	t.Logf("结果：%s", content)

	if !contains(content, "[ERROR] file_db_sync") {
		t.Error("应该触发 file_db_sync 错误")
	}
	if !contains(content, "第15章") {
		t.Error("应该提到第15章")
	}
	t.Logf("✓ 正确检测到 outline_beats 与 detail_json 不一致")
}

func TestInitConsistency_TypePacing(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// 创建 outline_beats，间距过大
	db.Create(&outline.OutlineBeat{NovelID: novelID, Chapter: 5, Description: "首个大爽点", BeatType: "shuangdian"})
	db.Create(&outline.OutlineBeat{NovelID: novelID, Chapter: 25, Description: "第二个大爽点", BeatType: "shuangdian"}) // 间距20章

	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 1,
		CheckTypes:     `["init_consistency"]`,
		Genre:          "xuanhuan",
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	content := result.Data["content"].(string)
	t.Logf("=== init_consistency type_pacing 测试 ===")
	t.Logf("大爽点：第5章, 第25章（间距20章）")
	t.Logf("结果：%s", content)

	if !contains(content, "[WARNING] type_pacing") {
		t.Error("应该触发 type_pacing 警告")
	}
	t.Logf("✓ 正确检测到大爽点间距过大")
}

func TestInitConsistency_Pass(t *testing.T) {
	db := setupTestDB(t)
	novelID := int64(1)
	ctx := context.Background()

	// 创建 outline_beats，间距合理
	db.Create(&outline.OutlineBeat{NovelID: novelID, Chapter: 5, Description: "首个大爽点", BeatType: "shuangdian"})
	db.Create(&outline.OutlineBeat{NovelID: novelID, Chapter: 12, Description: "第二个大爽点", BeatType: "shuangdian"})

	// 创建卷弧线，detail_json 与 outline_beats 一致
	detailJSON := map[string]any{
		"big_shuangdian": []map[string]any{
			{"chapter": 5, "desc": "首个大爽点"},
			{"chapter": 12, "desc": "第二个大爽点"},
		},
	}
	detailBytes, _ := json.Marshal(detailJSON)
	db.Create(&storyarc.StoryArc{
		NovelID:    novelID,
		Name:       "第一卷",
		ArcType:    "volume",
		Status:     "active",
		DetailJSON: string(detailBytes),
	})

	tool := &CheckStoryConsistencyTool{}
	args := &CheckStoryConsistencyArgs{
		CurrentChapter: 1,
		CheckTypes:     `["init_consistency"]`,
		Genre:          "xuanhuan",
	}

	result, err := tool.Execute(ctx, args, ToolContext{DB: db, NovelID: novelID})
	if err != nil {
		t.Fatal(err)
	}

	content := result.Data["content"].(string)
	t.Logf("=== init_consistency 通过测试 ===")
	t.Logf("结果：%s", content)

	if contains(content, "[ERROR]") || contains(content, "[WARNING]") {
		t.Error("不应该触发任何错误或警告")
	}
	t.Logf("✓ init_consistency 检查通过")
}
