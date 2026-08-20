package agent

import "fmt"

// phaseChecklist 返回当前阶段的紧凑清单（位于上下文末尾，比 kernel 更易被注意）。
func phaseChecklist(phase string) string {
	switch phase {
	case "init":
		return `【当前阶段：init（开书）】
必做：信息采集 → 写总纲(update_outline + create_outline_beat) → 创建卷弧线 → 实体入库 → 一致性校验 → 7项查询验证
允许：create_*, get_*（只读查询），edit(goink.md)，update_outline/create_outline_beat/delete_outline_beat
禁止：update_*（非outline的更新），delete_*，run_subagent
转出：7项查询全部通过(get_characters + get_locations + get_story_arcs + get_lore + get_items + get_timeline + get_preferences) → set_phase("prepare")`

	case "prepare":
		return `【当前阶段：prepare（全量状态加载）】
必做：9项查询并行发出(get_writing_context + get_chapter_list + get_characters + get_timeline + get_story_arcs + get_reader_perspective + get_writing_snapshot + get_scenes + get_preferences)
禁止：edit（不可编辑任何文件），create_*，update_*，delete_*，run_subagent
注意：还处于只读阶段，不要尝试写大纲或正文——edit 和所有写入工具都会被拦截
转出：9项必查全部完成 → set_phase("outline")`

	case "outline":
		return `【当前阶段：outline（写大纲）】
必做：read 总纲/卷纲 → edit(outlines/NNN.md) 写大纲
禁止：update_*，create_*（非outline），delete_*，run_subagent
注意：大纲写进 outlines/018.md 文件，不是数据库。不要调 get_outline（门禁已移除，信息在 get_writing_context 的 outline 段里）
转出：edit(outlines/NNN.md) 完成 → set_phase("write")`

	case "write":
		return `【当前阶段：write（写正文）】
必做：read 本章大纲 → edit(chapters/NNN.md) 写正文 → get_chapter_list 字数校验 → check_story_consistency 一致性核对
禁止：update_*，create_*（非miniMaintain），delete_*，run_subagent，edit(goink.md)
注意：正文只写当前章节内容，禁止复制前章标题或摘要。维护操作在 maintain 阶段做，write 阶段调维护工具会被拦截
转出：edit + get_chapter_list + check_story_consistency 完成 → set_phase("review")`

	case "review":
		return `【当前阶段：review（审稿）】
必做：run_subagent(agent_type="review") 启动审稿
禁止：update_*，create_*，delete_*（维护工具都在 maintain 阶段做）
转出：run_subagent + check_story_consistency 完成 → set_phase("maintain")`

	case "maintain":
		return `【当前阶段：maintain（状态维护）】
必做：7项查询并行(get_characters + get_timeline + get_story_arcs + get_reader_perspective + get_scenes + get_item_occurrences + get_character_relations) → 更新动作(update_chapter_meta + update_writing_snapshot + search_lore + search_items + update_chapter_plan + 按需 create/update) → edit(goink.md, append) 记录指纹
允许：所有 create_* / update_* / delete_* / search_* / edit / read
禁止：run_subagent
注意：一轮内完成，不留待办
转出：全部14项完成 → set_phase("done")`

	case "done":
		return `【当前阶段：done（完成）】
本轮创作已结束。系统停在 done 阶段，不做任何操作。等待用户发起新一轮创作（新会话从 init/prepare 开始）。`

	default:
		return fmt.Sprintf("【当前阶段：%s】\n按 kernel 阶段指令执行，完成后调 set_phase 切换到下一阶段。", phase)
	}
}