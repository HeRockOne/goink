package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"novel/internal/mcp_tools"
)

// toolDisplayNames 工具名 → 中文展示名称。
var toolDisplayNames = map[string]string{
	"get_chapter_list":                "查看章节目录",
	"search_story_memory":             "搜索故事记忆",
	"get_timeline":                    "查看故事时间线",
	"create_timeline_entry":           "记录追踪条目",
	"update_timeline_entry":           "更新追踪条目",
	"update_chapter_plan":             "更新章节计划",
	"get_locations":                   "查看地点信息",
	"create_location":                 "创建新地点",
	"update_location":                 "更新地点设定",
	"create_location_relation":        "创建地点关系",
	"update_location_relation":        "更新地点关系",
	"get_characters":                  "查看角色信息",
	"get_character_relations":         "查看人物关系",
	"create_character":                "创建新角色",
	"update_character":                "更新角色设定",
	"update_character_relationship":   "更新人物关系",
	"run_subagent":                    "调度AI子任务",
	"get_reader_perspective":          "查看读者视角",
	"create_reader_perspective_entry": "添加读者视角",
	"update_reader_perspective_entry": "更新读者视角",
	"get_story_arcs":                  "查看故事弧线",
	"create_story_arc":                "创建故事弧线",
	"update_story_arc":                "更新故事弧线",
	"create_arc_node":                 "创建弧线节点",
	"update_arc_node":                 "更新弧线节点",
	"get_preferences":                 "查看创作偏好",
	"create_preference":               "创建创作偏好",
	"update_preference":               "更新创作偏好",
	"delete_record":                   "删除记录",
	"edit":                            "编辑文件内容",
	"read":                            "读取文件内容",
	"web_search":                      "搜索网络信息",
	"web_fetch":                       "抓取网页内容",
	"get_entity_appearances":          "反查实体出场",
	"check_story_consistency":         "设定一致性检查",
	"update_chapter_meta":             "更新章节元数据",
	"get_item_occurrences":            "查看物品流转",
	"get_review_history":              "查询审稿记录",
	"submit_review":                   "提交审稿评分",
	"create_item_occurrence":          "记录物品出现",
	"get_items":                       "查看物品列表",
	"create_item":                     "创建新物品",
	"update_item":                     "更新物品设定",
	"search_items":                    "搜索物品",
	"get_lore":                        "查看世界观设定",
	"create_lore":                     "创建世界观设定",
	"update_lore":                     "更新世界观设定",
	"delete_lore":                     "删除世界观设定",
	"search_lore":                     "检索世界观设定",
	"get_phase_gate_config":           "查看门禁配置",
	"update_phase_gate_config":        "更新门禁配置",
	"get_scenes":                      "查看场景列表",
	"create_scene":                    "创建场景",
	"update_scene":                    "更新场景",
	"delete_scene":                    "删除场景",
	"get_writing_snapshot":            "查看写作快照",
	"update_writing_snapshot":         "更新写作快照",
	"get_stats":                       "查看统计",
	"get_writing_context":             "获取创作上下文",
}

// toolActivityKinds 工具名 → 前端展示类别。
var toolActivityKinds = map[string]string{
	"get_chapter_list":                "browse",
	"search_story_memory":             "memory",
	"get_timeline":                    "view",
	"create_timeline_entry":           "write",
	"update_timeline_entry":           "edit",
	"update_chapter_plan":             "edit",
	"get_locations":                   "view",
	"create_location":                 "create",
	"update_location":                 "edit",
	"create_location_relation":        "create",
	"update_location_relation":        "edit",
	"get_characters":                  "view",
	"get_character_relations":         "view",
	"create_character":                "create",
	"update_character":                "edit",
	"update_character_relationship":   "edit",
	"run_subagent":                    "plan",
	"get_reader_perspective":          "view",
	"create_reader_perspective_entry": "write",
	"update_reader_perspective_entry": "edit",
	"get_story_arcs":                  "view",
	"create_story_arc":                "create",
	"update_story_arc":                "edit",
	"create_arc_node":                 "create",
	"update_arc_node":                 "edit",
	"get_preferences":                 "view",
	"create_preference":               "create",
	"update_preference":               "edit",
	"delete_record":                   "delete",
	"edit":                            "write",
	"read":                            "view",
	"web_search":                      "browse",
	"web_fetch":                       "view",
	"get_entity_appearances":          "view",
	"check_story_consistency":         "review",
	"update_chapter_meta":             "edit",
	"get_item_occurrences":            "view",
	"create_item_occurrence":          "write",
	"get_items":                       "view",
	"create_item":                     "create",
	"update_item":                     "edit",
	"search_items":                    "browse",
	"get_lore":                        "view",
	"create_lore":                     "create",
	"update_lore":                     "edit",
	"delete_lore":                     "delete",
	"search_lore":                     "browse",
	"get_phase_gate_config":           "view",
	"update_phase_gate_config":        "edit",
	"get_scenes":                      "view",
	"create_scene":                    "create",
	"update_scene":                    "edit",
	"delete_scene":                    "delete",
	"get_writing_snapshot":            "view",
	"update_writing_snapshot":         "edit",
	"get_stats":                       "view",
	"get_writing_context":             "memory",
}

// chapterTools 需要查章节标题的工具集。
var chapterTools = map[string]bool{
	"edit": true,
	"read": true,
}

// phaseDisplayNames 阶段名 → 中文展示名称。
var phaseDisplayNames = map[string]string{
	"init":     "初始化",
	"prepare":  "准备",
	"outline":  "大纲",
	"write":    "正文",
	"review":   "审读",
	"maintain": "维护",
	"done":     "完成",
}

// displayPhaseName 阶段名转展示名，未知阶段原样返回。
func displayPhaseName(name string) string {
	if v, ok := phaseDisplayNames[name]; ok {
		return v
	}
	return name
}

// buildDisplay 根据 tool_name + args + phase 生成前端展示文本。
// executing 阶段加 "正在" 前缀，completed/failed/cancelled 去掉。
// chapter 工具通过 novelID + chapter_number 查 DB 获取章节标题。
func (a *Agent) buildDisplay(name string, args map[string]any, phase mcp_tools.DisplayPhase, novelID int64, pg *PhaseGate) *mcp_tools.DisplayInfo {
	// set_phase 特殊处理：显示阶段流转（起点 → 终点），如 "准备 → 大纲"
	if name == "set_phase" {
		targetPhase := ""
		if args != nil {
			if p, ok := args["phase"].(string); ok {
				targetPhase = p
			}
		}
		text := displayPhaseName(targetPhase)
		if pg != nil {
			from := pg.CurrentPhase()
			if from != "" && from != targetPhase {
				text = displayPhaseName(from) + " → " + text
			}
		}
		return &mcp_tools.DisplayInfo{
			DisplayText:  text,
			ActivityKind: "phase",
		}
	}

	baseText := toolDisplayNames[name]
	if baseText == "" {
		baseText = name
	}
	activityKind := toolActivityKinds[name]
	if activityKind == "" {
		activityKind = "general"
	}

	var metadata map[string]any

	// run_subagent：根据 agent_type 定制展示文本
	if name == "run_subagent" {
		if at, ok := args["agent_type"].(string); ok {
			switch at {
			case "memory":
				baseText = "探索故事记忆"
			case "review":
				baseText = "审核章节内容"
			}
		}
		metadata = map[string]any{"agent_type": args["agent_type"]}
	}

	// chapter 工具：查 DB 取章节标题
	if chapterTools[name] {
		if cn, ok := chapterNumber(args); ok {
			label := a.lookupChapterBrief(novelID, cn)
			switch name {
			case "edit":
				baseText = "编辑 " + label
			case "read":
				baseText = "查看 " + label
			}
		}

		// rw 工具的 goink.md 路径特殊处理
		if path, ok := args["path"].(string); ok && path == "goink.md" {
			switch name {
			case "edit":
				baseText = "编辑 故事状态"
			case "read":
				baseText = "查看 故事状态"
			}
		}

		// rw 工具的 outlines/ 路径特殊处理
		if path, ok := args["path"].(string); ok && strings.HasPrefix(path, "outlines/") {
			var n int
			fmt.Sscanf(path, "outlines/%d.md", &n)
			label := fmt.Sprintf("第%d章大纲", n)
			switch name {
			case "edit":
				baseText = "编辑 " + label
			case "read":
				baseText = "查看 " + label
			}
		}
	}

	// executing 阶段加 "正在" 前缀
	isActive := phase == mcp_tools.PhaseExecuting || phase == mcp_tools.PhaseSelected
	if isActive {
		baseText = "正在" + baseText
	}

	return &mcp_tools.DisplayInfo{
		DisplayText:  baseText,
		ActivityKind: activityKind,
		Metadata:     metadata,
	}
}

func chapterNumber(args map[string]any) (int, bool) {
	if args == nil {
		return 0, false
	}
	for _, key := range []string{"chapter_number", "chapter_id"} {
		if v, ok := args[key]; ok {
			switch n := v.(type) {
			case float64:
				return int(n), true
			case int:
				return n, true
			}
		}
	}
	// edit 工具使用 path 参数，如 "chapters/001.md"
	if path, ok := args["path"].(string); ok {
		var n int
		if _, err := fmt.Sscanf(path, "chapters/%d.md", &n); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

type chapterTitleRow struct {
	Title string `gorm:"column:title"`
}

func (a *Agent) lookupChapterBrief(novelID int64, chapterNumber int) string {
	var row chapterTitleRow
	err := a.db.Table("chapters").
		Where("novel_id = ? AND chapter_number = ?", novelID, chapterNumber).
		Select("title").
		Scan(&row).Error
	if err != nil || row.Title == "" {
		return fmt.Sprintf("第%d章", chapterNumber)
	}
	// 标题常自带"第N章"前缀（DB 里存的就是完整标题），去掉避免与格式串重复声明
	title := row.Title
	prefix := fmt.Sprintf("第%d章", chapterNumber)
	title = strings.TrimPrefix(title, prefix)
	title = strings.TrimSpace(strings.TrimPrefix(title, "　"))
	if title == "" {
		return fmt.Sprintf("第%d章", chapterNumber)
	}
	return fmt.Sprintf("第%d章 %s", chapterNumber, title)
}

func buildToolDisplay(toolOutputs []toolOutput) []map[string]any {
	toolDisplays := make([]map[string]any, 0, len(toolOutputs))
	for _, to := range toolOutputs {
		phase := "completed"
		if !to.result.Success {
			phase = "failed"
		}
		entry := map[string]any{
			"tool_id":       to.id,
			"tool_name":     to.name,
			"display_text":  to.displayText,
			"activity_kind": to.activityKind,
			"phase":         phase,
		}
		if to.result != nil && to.result.Data != nil {
			if to.name == "web_search" || to.name == "web_fetch" {
				// 富文本卡片按结构渲染（WebSearchCard/WebFetchCard），保留原始 shape
				if to.result.Success {
					entry["result"] = to.result.Data
				}
			} else {
				// 其余工具结果序列化为截断 JSON 字符串，供前端历史详情展开查看
				entry["result"] = truncateResultJSON(to.result.Data)
			}
		}
		toolDisplays = append(toolDisplays, entry)
	}
	return toolDisplays
}

// truncateResultJSON 把工具结果序列化为 JSON 字符串（与前端展示截断一致，4000 字符），
// 防止 extra_metadata 无限膨胀
func truncateResultJSON(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	runes := []rune(string(b))
	if len(runes) > 4000 {
		return string(runes[:4000]) + "…（截断）"
	}
	return string(runes)
}
