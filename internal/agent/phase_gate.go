package agent

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// PhaseGate 从 always-mode skill 内容中解析阶段门禁配置，跟踪阶段状态，执行门禁检查。
//
// 核心设计：
// 1. 支持 single（单章）和 batch（批量）两种模式
// 2. 初始阶段为配置中的第一个阶段
// 3. 每次工具调用后，记录调用成功次数
// 4. set_phase 切换阶段时检查 require，未满足则阻塞
// 5. 自动推进：回合收尾时 require 已满足则自动 set_phase（agent.go 循环后兜底），
//    阶段内仍支持 LLM 主动 set_phase 立即切换
type PhaseGate struct {
	phases          []PhaseConfig
	currentPhase    string
	calledTools     map[string]int // tool_name → 调用次数（含失败）
	successfulTools map[string]int // tool_name → 成功次数（require 只看这个）
	lastToolResults map[string]string // tool_name → 最近一次调用结果内容（结果门控用）
	consistencyErrors []string       // 未解决的一致性 [ERROR] 指纹（缩窄 check_types 复查无法绕过，全量复查才清空）
	mode            string         // "single" | "batch"
	active          bool           // 是否启用
	wordCountOK     *bool          // get_chapter_list 字数校验结果（nil=未检查）
	visited         []string       // 已访问过的阶段列表，用于回退校验；回到起点时重置
	readsByPhase    map[string]map[string]bool // 阶段 → 已成功读取的技能集合（auto_skill_injection 用，阶段切换后从零开始）
	roundCompleted  bool                       // 本轮完整流程走完（SetPhase 回到流程起点触发），batch 模式清除标记用
}

// PhaseConfig 是单个阶段的配置。
type PhaseConfig struct {
	Name      string   // 阶段名称
	Mode      string   // 所属模式："single" | "batch"（空=两种模式都适用）
	Tools     []string // 允许使用的显式工具名
	ToolCategories []string // 工具类别（get/create/update/delete/search/remove），自动展开为对应前缀
	Require   []string // 必须调用过的工具（完成条件）
	AutoSkillInjection []string // 必须读取过的技能名（完成条件，如 main-tech-show-dont-tell）
	Next      string   // 满足条件后可进入的下一阶段
	FailNext  string   // require 不满足时的回退阶段
	Loop      bool     // batch 模式下是否循环（write → outline）
	EditPaths string   // edit 工具的路径范围（如 "outlines/*, goink.md"，"*" = 不限制）
	Note      string   // 注意/提示文本（用于 checklist 动态生成）
}

// ParsePhaseGateConfig 从 markdown 内容中解析 <!-- phase-gate-config --> 块。
// mode 参数选择 "single" 或 "batch"，只加载对应模式的阶段配置。
// 返回 nil 表示未找到任何配置。
func ParsePhaseGateConfig(content string, mode string) *PhaseGate {
	re := regexp.MustCompile(`(?s)<!--\s*phase-gate-config\s*\n(.*?)-->`)
	matches := re.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	var phases []PhaseConfig
	for _, m := range matches {
		pc := parsePhaseBlock(m[1])
		if pc.Name == "" {
			continue
		}
		// 只加载匹配模式的阶段（Mode 为空表示两种模式都适用）
		if pc.Mode != "" && pc.Mode != mode {
			continue
		}
		phases = append(phases, pc)
	}
	if len(phases) == 0 {
		return nil
	}

	// 强制激活：直接进入第一个阶段，不允许空状态
	firstPhase := phases[0].Name
	return &PhaseGate{
		phases:          phases,
		currentPhase:    firstPhase,
		visited:         []string{firstPhase},
		calledTools:     make(map[string]int),
		successfulTools: make(map[string]int),
		lastToolResults: make(map[string]string),
		readsByPhase:    make(map[string]map[string]bool),
		mode:            mode,
		active:          true,
	}
}

// knownCategories 可展开为前缀的类别名，用于 tools: 精简配置。
var knownCategories = map[string]bool{
	"get": true, "create": true, "update": true,
	"delete": true, "search": true, "remove": true,
	"web": true, "check": true,
}

// categoryPrefixes 类别名到前缀的映射，用于 CheckToolAllowed 匹配。
var categoryPrefixes = map[string]string{
	"get":    "get_",
	"create": "create_",
	"update": "update_",
	"delete": "delete_",
	"search": "search_",
	"remove": "remove_",
	"web":    "web_",
	"check":  "check_",
}

// toolMatchesCategory 检查工具名是否匹配某类别。
func toolMatchesCategory(toolName, category string) bool {
	prefix, ok := categoryPrefixes[category]
	if !ok {
		return toolName == category
	}
	return strings.HasPrefix(toolName, prefix)
}

// parsePhaseBlock 解析单个阶段配置块的键值对。
func parsePhaseBlock(block string) PhaseConfig {
	pc := PhaseConfig{}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "phase":
			pc.Name = val
		case "mode":
			pc.Mode = val
		case "tools":
			for _, item := range strings.Split(val, ",") {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				if knownCategories[item] {
					pc.ToolCategories = append(pc.ToolCategories, item)
				} else {
					pc.Tools = append(pc.Tools, item)
				}
			}
		case "require":
			pc.Require = parseToolList(val)
		case "auto_skill_injection":
			pc.AutoSkillInjection = parseToolList(val)
		case "next":
			pc.Next = val
		case "fail_next":
			pc.FailNext = val
		case "loop":
			pc.Loop = val == "true"
		case "edit_paths":
			pc.EditPaths = val
		case "note":
			pc.Note = val
		}
	}
	return pc
}

// parseToolList 解析逗号分隔的工具列表，去除空白。
func parseToolList(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// Active 返回门禁是否启用。
func (g *PhaseGate) Active() bool {
	return g != nil && g.active
}

// CurrentPhase 返回当前阶段名。
func (g *PhaseGate) CurrentPhase() string {
	if g == nil {
		return ""
	}
	return g.currentPhase
}

// OnToolCall 记录工具调用。
// success=true 表示工具执行成功，require 只统计成功次数。
// resultContent 是工具返回结果的内容（用于结果门控检查）。
// argsJSON 可选，是工具原始参数 JSON（check_story_consistency 判断检查范围用）。
func (g *PhaseGate) OnToolCall(toolName string, success bool, resultContent string, argsJSON ...string) {
	if g == nil || !g.active {
		return
	}

	g.calledTools[toolName]++
	if success {
		g.successfulTools[toolName]++
		// 存储最近一次调用结果（结果门控用）
		if resultContent != "" {
			g.lastToolResults[toolName] = resultContent
		}
		if toolName == "check_story_consistency" {
			args := ""
			if len(argsJSON) > 0 {
				args = argsJSON[0]
			}
			g.recordConsistencyCheck(args, resultContent)
		}
	}
}

// fullConsistencyTypes 全量一致性检查的 11 个类型；args 覆盖全部才视为全量复查。
var fullConsistencyTypes = []string{
	"foreshadow_overdue", "character_vanished", "item_conflict", "dead_appeared",
	"pacing_gap", "promise_fulfillment", "init_consistency", "ledger_integrity",
	"beat_window", "scope_guard", "type_drift",
}

// recordConsistencyCheck 维护一致性错误指纹集合，堵住"换一组 check_types 重查绕过 ERROR"的漏洞：
// 缩窄范围的复查只增不清——已有 ERROR 不因本次未覆盖而消失；
// 全量检查（留空=默认全部，或覆盖全部类型）重置集合后按本次结果重建，
// 修复后做一次全量复查即可解除拦截。
func (g *PhaseGate) recordConsistencyCheck(argsJSON, resultContent string) {
	lower := strings.ToLower(argsJSON)
	full := !strings.Contains(lower, "check_types")
	if !full {
		all := true
		for _, t := range fullConsistencyTypes {
			if !strings.Contains(lower, t) {
				all = false
				break
			}
		}
		full = all
	}
	if full {
		g.consistencyErrors = nil
	}
	for _, line := range extractErrorLines(resultContent) {
		dup := false
		for _, e := range g.consistencyErrors {
			if e == line {
				dup = true
				break
			}
		}
		if !dup {
			g.consistencyErrors = append(g.consistencyErrors, line)
		}
	}
}

// OnSkillInjected 记录当前阶段成功读取的技能（auto_skill_injection 用）。
// 阶段切换后 readsByPhase 对新阶段从零开始——每阶段必读独立生效。
func (g *PhaseGate) OnSkillInjected(skillName string) {
	if g == nil || !g.active || skillName == "" {
		return
	}
	if g.readsByPhase == nil {
		g.readsByPhase = make(map[string]map[string]bool)
	}
	if g.readsByPhase[g.currentPhase] == nil {
		g.readsByPhase[g.currentPhase] = make(map[string]bool)
	}
	g.readsByPhase[g.currentPhase][skillName] = true
}

// miniMaintainTools 批量 write 每章必须完成的状态结算六件套（miniMaintain）。
var miniMaintainTools = []string{
	"create_scene", "update_character", "create_timeline_entry",
	"update_timeline_entry", "create_item_occurrence", "update_writing_snapshot",
}

// missingMiniMaintain 返回当前阶段尚未成功调用的 miniMaintain 工具列表。
func (g *PhaseGate) missingMiniMaintain() []string {
	var missing []string
	for _, t := range miniMaintainTools {
		if g.successfulTools[t] == 0 {
			missing = append(missing, t)
		}
	}
	return missing
}

// ResetPhaseCounts 批量章边界调用：清空本阶段工具调用统计与字数校验状态，
// 强制每章重新满足 require（miniMaintain 六件套按章结算，而非跨章累计）。
// readsByPhase 不重置——技能已注入，无需重复读取。
func (g *PhaseGate) ResetPhaseCounts() {
	if g == nil || !g.active {
		return
	}
	g.calledTools = make(map[string]int)
	g.successfulTools = make(map[string]int)
	g.wordCountOK = nil
}

// SetWordCountOK 设置字数校验结果。get_chapter_list 工具调用后由 agent 注入。
func (g *PhaseGate) SetWordCountOK(ok bool) {
	if g == nil || !g.active {
		return
	}
	g.wordCountOK = &ok
}

// WordCountOK 返回字数校验状态。nil 表示未检查。
func (g *PhaseGate) WordCountCheck() *bool {
	if g == nil {
		return nil
	}
	return g.wordCountOK
}

// checkRequireMet 检查阶段的 require 条件是否全部满足。
// 只统计成功执行的工具调用，失败的不算。
func (g *PhaseGate) checkRequireMet(pc *PhaseConfig) bool {
	for _, req := range pc.Require {
		if g.successfulTools[req] == 0 {
			return false
		}
	}
	return true
}

// reviewVerdictRe 匹配审稿报告结论行：总分：X.X/10（通过/需修改/不通过）
// 与 internal/review/review.go verdictRe 保持一致。
var reviewVerdictRe = regexp.MustCompile(`总分[:：]\s*[\d.]+\s*/\s*10\s*[（(]\s*(通过|需修改|不通过)\s*[）)]`)

// checkResultGateMet 检查结果门控：
// 1. 一致性检查存在未解决的 [ERROR] 禁止推进（consistencyErrors 集合，缩窄 check_types 无法绕过）
// 2. run_subagent 审稿结论为不通过时，禁止推进到下一阶段
func (g *PhaseGate) checkResultGateMet(pc *PhaseConfig) (bool, string) {
	if len(g.consistencyErrors) > 0 {
		summary := extractErrorSummary(strings.Join(g.consistencyErrors, "\n"), 2)
		return false, fmt.Sprintf("check_story_consistency 存在未解决硬错误（共%d条）——修复后需做一次全量检查解除，禁止切换阶段：\n%s",
			len(g.consistencyErrors), summary)
	}
	// 审稿结论门控：仅"不通过"（<7.0）阻止推进，"需修改"（7.0-8.9）放行
	// 需修改=小问题修了就行，LLM 看到报告会自行修复；不通过=大问题必须重审
	// 取最后一个匹配：报告正文可能引用历史结论行，末尾的规范结论行才是本次结果
	if result, ok := g.lastToolResults["run_subagent"]; ok {
		if matches := reviewVerdictRe.FindAllStringSubmatch(result, -1); len(matches) > 0 {
			if m := matches[len(matches)-1]; m[1] == "不通过" {
				return false, "审稿结论为「不通过」（总分<7.0），禁止推进。请修复后重新审稿"
			}
		}
	}
	return true, ""
}

var errorLineRe = regexp.MustCompile(`\[ERROR\][^\n]*`)

// extractErrorLines 提取全部 [ERROR] 行（每行截断至 80 字符），作为错误指纹。
func extractErrorLines(result string) []string {
	matches := errorLineRe.FindAllString(result, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		r := []rune(m)
		if len(r) > 80 {
			m = string(r[:80]) + "…"
		}
		out = append(out, m)
	}
	return out
}

// extractErrorSummary 从工具结果中提取前 N 条 [ERROR] 行，每行截断至 80 字符。
func extractErrorSummary(result string, maxLines int) string {
	matches := errorLineRe.FindAllString(result, maxLines)
	if len(matches) == 0 {
		return "[无详细错误信息]"
	}
	for i, m := range matches {
		r := []rune(m)
		if len(r) > 80 {
			matches[i] = string(r[:80]) + "…"
		}
	}
	return strings.Join(matches, "\n")
}

// missingInjections 返回当前阶段尚未加载的必读技能列表（auto_skill_injection）。
func (g *PhaseGate) missingInjections(pc *PhaseConfig) []string {
	if len(pc.AutoSkillInjection) == 0 {
		return nil
	}
	reads := g.readsByPhase[g.currentPhase]
	var missing []string
	for _, pattern := range pc.AutoSkillInjection {
		if strings.Contains(pattern, "*") {
			matched := false
			for read := range reads {
				if ok, _ := path.Match(pattern, read); ok {
					matched = true
					break
				}
			}
			if !matched {
				missing = append(missing, pattern)
			}
			continue
		}
		if !reads[pattern] {
			missing = append(missing, pattern)
		}
	}
	return missing
}

// checkInjectionsMet 检查阶段的 auto_skill_injection（必读技能）是否全部满足。
// 只统计当前阶段内成功读取的技能（阶段切换后从零开始）。
// 支持通配符：配置项含 * 时用 path.Match 匹配（如 "main-tech-*" 匹配所有已读的 main-tech 系技能）。
func (g *PhaseGate) checkInjectionsMet(pc *PhaseConfig) bool {
	return len(g.missingInjections(pc)) == 0
}

// isMutatingTool 判断工具是否是有副作用的创作/维护动作。
// 这类动作必须在必读技能加载后才能执行——技能是创作指导，不是切换阶段的手续。
func isMutatingTool(toolName string) bool {
	if toolName == "edit" || toolName == "run_subagent" {
		return true
	}
	return strings.HasPrefix(toolName, "create_") ||
		strings.HasPrefix(toolName, "update_") ||
		strings.HasPrefix(toolName, "delete_") ||
		strings.HasPrefix(toolName, "remove_")
}

// phaseChecklist 从门禁配置动态生成当前阶段紧凑清单（上下文末尾注入，比 kernel 更易被注意）。
// 必做/禁止/转出 从 PhaseConfig 自动派生，注意 从 Note 字段读取——改门禁配置时顺手维护。
func phaseChecklist(pc *PhaseConfig) string {
	if pc == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【当前阶段：%s】\n", pc.Name))

	// 必做
	sb.WriteString("必做：")
	if len(pc.Require) > 0 {
		sb.WriteString(strings.Join(pc.Require, " + "))
	} else {
		sb.WriteString("（无强制要求）")
	}

	// 禁止：计算白名单外 mutating 工具
	allowed := make(map[string]bool, len(pc.Tools))
	for _, t := range pc.Tools {
		allowed[t] = true
	}
	// 类别覆盖的工具也算白名单内
	hasCategory := func(cat string) bool {
		for _, c := range pc.ToolCategories {
			if c == cat {
				return true
			}
		}
		return false
	}
	var forbidden []string

	prefixes := []struct {
		prefix string
		label  string
		cat    string
	}{
		{"create_", "create_*", "create"},
		{"update_", "update_*", "update"},
		{"delete_", "delete_*", "delete"},
		{"remove_", "remove_*", "remove"},
	}
	for _, p := range prefixes {
		if hasCategory(p.cat) {
			continue // 类别覆盖，全部放行
		}
		var allowedOfPrefix []string
		for t := range allowed {
			if strings.HasPrefix(t, p.prefix) {
				allowedOfPrefix = append(allowedOfPrefix, t)
			}
		}
		if len(allowedOfPrefix) == 0 {
			forbidden = append(forbidden, p.label)
		} else {
			sort.Strings(allowedOfPrefix)
			forbidden = append(forbidden, fmt.Sprintf("%s（仅%s）", p.label, strings.Join(allowedOfPrefix, "/")))
		}
	}

	if !allowed["edit"] && !hasCategory("edit") {
		forbidden = append(forbidden, "edit")
	} else if allowed["edit"] && pc.EditPaths != "" && pc.EditPaths != "*" {
		forbidden = append(forbidden, fmt.Sprintf("edit 仅限 %s", pc.EditPaths))
	}

	if !allowed["run_subagent"] && !hasCategory("run_subagent") {
		forbidden = append(forbidden, "run_subagent")
	}

	sb.WriteString("\n禁止：")
	if len(forbidden) > 0 {
		sb.WriteString(strings.Join(forbidden, "，"))
	} else {
		sb.WriteString("（无限制）")
	}

	// 注意
	if pc.Note != "" {
		sb.WriteString(fmt.Sprintf("\n注意：%s", pc.Note))
	}

	// 转出
	if pc.Next != "" {
		sb.WriteString("\n转出：")
		if len(pc.Require) > 0 {
			sb.WriteString(strings.Join(pc.Require, " + "))
			sb.WriteString(" 完成")
		}
		sb.WriteString(fmt.Sprintf(" → set_phase(\"%s\")", pc.Next))
	} else {
		sb.WriteString("\n转出：终点阶段，本轮创作结束")
	}

	return sb.String()
}

// ValidationIssue 门禁配置校验结果单条（设置页"校验配置"按钮用）。
type ValidationIssue struct {
	Mode    string `json:"mode"`    // "single" | "batch"
	Phase   string `json:"phase"`   // 出问题的阶段名
	Level   string `json:"level"`   // "error"（必然卡死）| "warning"（隐患）
	Message string `json:"message"`
}

// ValidateGateConfig 校验门禁配置合法性（两种模式都查）。
// knownSkills 是现有技能名集合（auto_skill_injection 引用检查用，含通配符则跳过）。
func ValidateGateConfig(content string, knownSkills []string) []ValidationIssue {
	var issues []ValidationIssue
	skills := make(map[string]bool, len(knownSkills))
	for _, s := range knownSkills {
		skills[s] = true
	}

	for _, mode := range []string{"single", "batch"} {
		gate := ParsePhaseGateConfig(content, mode)
		if gate == nil {
			continue // 该模式没有配置块（可能只有 single 或只有 batch）
		}
		names := make(map[string]bool, len(gate.phases))
		for _, p := range gate.phases {
			names[p.Name] = true
		}
		for _, p := range gate.phases {
			// next 指向存在的阶段（空 next = 终点阶段合法，如 done：流程走完停在终点，
			// 不再自动推进；终点前的阶段 next 必须存在）
			if p.Next != "" && !names[p.Next] {
				issues = append(issues, ValidationIssue{mode, p.Name, "error",
					fmt.Sprintf("next 指向不存在的阶段 [%s]", p.Next)})
			}
			// require 的工具必须在 tools 白名单或类别覆盖中，否则 set_phase 永远被拦
			tools := make(map[string]bool, len(p.Tools))
			for _, t := range p.Tools {
				tools[t] = true
			}
			for _, req := range p.Require {
				inTools := tools[req]
				if !inTools {
					for _, cat := range p.ToolCategories {
						if toolMatchesCategory(req, cat) {
							inTools = true
							break
						}
					}
				}
				if !inTools {
					issues = append(issues, ValidationIssue{mode, p.Name, "error",
						fmt.Sprintf("require 引用了 tools 中没有的工具 [%s]，切换阶段将永远被拦截", req)})
				}
			}
			// auto_skill_injection 的技能必须存在（通配符跳过）
			for _, pattern := range p.AutoSkillInjection {
				if strings.Contains(pattern, "*") {
					continue
				}
				if !skills[pattern] {
					issues = append(issues, ValidationIssue{mode, p.Name, "warning",
						fmt.Sprintf("auto_skill_injection 引用的技能 [%s] 不存在，auto_skill_injection 将失败", pattern)})
				}
			}
			// 有 edit 工具但没限制路径
			hasEdit := tools["edit"]
			if !hasEdit {
				for _, cat := range p.ToolCategories {
					if cat == "edit" {
						hasEdit = true
						break
					}
				}
			}
			if hasEdit && p.EditPaths == "" {
				issues = append(issues, ValidationIssue{mode, p.Name, "warning",
					"tools 含 edit 但缺少 edit_paths（将允许编辑任意文件，建议限制路径）"})
			}
		}
	}
	return issues
}

// SetPhase 显式切换到目标阶段（LLM 主动调用 set_phase 时）。
// 同阶段切换（current == target）直接返回成功。
// require 未满足时阻塞，不允许跳转。
// 返回 (success, warningMessage)。
func (g *PhaseGate) SetPhase(targetPhase string) (bool, string) {
	if g == nil || !g.active {
		return true, ""
	}

	target := g.findPhase(targetPhase)
	if target == nil {
		return false, fmt.Sprintf("未知阶段: %s", targetPhase)
	}

	// 同阶段切换：直接成功，不检查 require。
	// 例外：批量 write 章边界（同阶段 set_phase("write") 声明下一章）——上一章的
	// miniMaintain 六件套必须完成才能继续，否则第 2-N 章的状态结算被跳过
	// （require 按阶段累计，第 1 章调过后面章不调也能转出，真机验证确认）。
	if g.currentPhase == targetPhase {
		if g.mode == "batch" && targetPhase == "write" {
			if missing := g.missingMiniMaintain(); len(missing) > 0 {
				return false, fmt.Sprintf("上一章迷你维护未完成，缺少: %v。请先完成本章状态结算（create_scene/update_character/create_timeline_entry/update_timeline_entry/create_item_occurrence/update_writing_snapshot）再声明下一章", missing)
			}
			// 写时把关：每章写完必须程序化一致性核对（kernel write 6.5）。
			// 只在转 review 时查会让中间章的设定硬伤（伏笔超期/死者复出/物品冲突）
			// 潜伏到批末才暴露，返工成本随批量线性放大。
			// 仅当当前阶段白名单实际放行该工具时才强制——用户自定义配置未包含时
			// 不得把章边界焊死（拦了也调不了）
			if g.toolAvailable("check_story_consistency") && g.successfulTools["check_story_consistency"] == 0 {
				return false, "上一章未执行 check_story_consistency 一致性核对。请先调用 check_story_consistency（current_chapter=本章章号）核对硬错误，发现 [ERROR] 定位修复后重跑通过，再声明下一章"
			}
		}
		return true, ""
	}

	// 进入 write 阶段时重置字数校验状态：上一章的字数检查结果（word_count_ok）
	// 不能带到本章，write 转出必须用本章 get_chapter_list 的结果
	if targetPhase == "write" {
		g.wordCountOK = nil
	}

	// 检查当前阶段的 require 是否满足（不满足则阻塞）
	current := g.findPhase(g.currentPhase)
	if current != nil && len(current.Require) > 0 {
		var missing []string
		for _, req := range current.Require {
			if g.successfulTools[req] == 0 {
				missing = append(missing, req)
			}
		}
		if len(missing) > 0 {
			return false, fmt.Sprintf("阶段 [%s] 要求必须调用以下工具后才能切换到 [%s]，当前未调用: %v",
				g.currentPhase, targetPhase, missing)
		}
	}

	// 结果门控：require 中的工具如果返回了 [ERROR]，禁止推进
	if current != nil {
		if ok, reason := g.checkResultGateMet(current); !ok {
			return false, reason
		}
	}

	// 检查当前阶段的 auto_skill_injection（必读技能）是否满足（不满足则阻塞）
	if current != nil && len(current.AutoSkillInjection) > 0 {
		if missingSkills := g.missingInjections(current); len(missingSkills) > 0 {
			return false, fmt.Sprintf("阶段 [%s] 要求必须用 auto_skill_injection 读取以下技能后才能切换到 [%s]，当前未读取: %v",
				g.currentPhase, targetPhase, missingSkills)
		}
	}

	// write 阶段转出时强制检查字数
	if g.currentPhase == "write" && targetPhase != "write" {
		if g.wordCountOK == nil {
			return false, fmt.Sprintf("阶段 [write] 转出前必须调用 get_chapter_list 校验字数，请先调用该工具")
		}
		if !*g.wordCountOK {
			return false, fmt.Sprintf("阶段 [write] 最新章节字数不达标，请扩写后再检查")
		}
	}

	// 校验 next 字段：允许推进到 next，也允许回退到已访问过的阶段。
	// visited 在"回到流程起点（第一个阶段）"时重置，代表完成了一轮完整流程，
	// 避免旧 bug：visited 永久累积导致第二轮创作可任意跳转。
	if current != nil && current.Next != "" && targetPhase != current.Next {
		allowed := false
		// Loop 标记：batch 模式允许循环回退到上一阶段（如 write ⇄ outline 连续多章）
		if g.mode == "batch" && current.Loop && targetPhase == g.prevPhaseName(current.Name) {
			allowed = true
		}
		for _, v := range g.visited {
			if v == targetPhase {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, fmt.Sprintf("阶段 [%s] 不允许直接切换到 [%s]，只允许推进到 [%s] 或回退到已访问过的阶段",
				g.currentPhase, targetPhase, current.Next)
		}
	}

	// 阶段切换成功后重置工具调用统计：require 的语义是"本阶段内必须调用"。
	// 跨阶段累计会让上一阶段调过的工具（如 write 的 edit）提前满足本阶段 require，
	// 轮末自动推进过早触发——batch 第 3 章 maintain 的 goink.md 指纹（edit）被 done
	// 白名单冻结，回退 maintain 后又立即被推进，形成死循环。
	// 与 readsByPhase（技能按阶段独立生效）保持一致。
	g.calledTools = make(map[string]int)
	g.successfulTools = make(map[string]int)
	g.lastToolResults = make(map[string]string)
	g.consistencyErrors = nil

	g.currentPhase = targetPhase
	// 判定一轮完整流程完成：目标阶段是"按 next 链推进回来的、且本轮已访问过"的阶段。
	// 例：single 的 maintain.next=prepare、batch 的 done.next=prepare，回到 prepare 说明走完一轮。
	// 此时重置 visited，避免旧 bug：visited 永久累积导致第二轮创作可任意跳转。
	if current != nil && current.Next == targetPhase && g.wasVisited(targetPhase) {
		g.visited = []string{targetPhase}
		g.roundCompleted = true
	} else {
		g.visited = append(g.visited, targetPhase)
	}
	return true, ""
}

// wasVisited 判断阶段是否已在本轮访问过。
func (g *PhaseGate) wasVisited(name string) bool {
	for _, v := range g.visited {
		if v == name {
			return true
		}
	}
	return false
}

// VisitedCount 返回本轮已访问的阶段数（判断完整流程是否走完）。
func (g *PhaseGate) VisitedCount() int {
	if g == nil {
		return 0
	}
	return len(g.visited)
}

// CheckEditPath 检查 edit 工具的目标路径是否在当前阶段允许的范围内。
// 返回 (allowed, warningMessage)。
func (g *PhaseGate) CheckEditPath(filePath string) (bool, string) {
	if g == nil || !g.active {
		return true, ""
	}

	current := g.findPhase(g.currentPhase)
	if current == nil || current.EditPaths == "" || current.EditPaths == "*" {
		return true, ""
	}

	// 解析允许的路径模式
	allowedPatterns := strings.Split(current.EditPaths, ",")
	for _, pattern := range allowedPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		// 精确匹配
		if pattern == filePath {
			return true, ""
		}
		// 目录前缀匹配（如 outlines/* 匹配 outlines/001.md），用 / 作为统一分隔符，
		// 避免 Windows 下 filepath.Match 的 \ 分隔符导致 * 跨目录匹配
		if strings.HasSuffix(pattern, "/*") {
			dir := strings.TrimSuffix(pattern, "/*")
			if filePath == dir || strings.HasPrefix(filePath, dir+"/") {
				return true, ""
			}
			continue
		}
		// 其它 glob 模式：用 path.Match（/ 固定分隔符，* 不跨目录）
		if strings.Contains(pattern, "*") {
			if matched, _ := path.Match(pattern, filePath); matched {
				return true, ""
			}
		}
	}

	warning := fmt.Sprintf("当前阶段 [%s] 不允许编辑文件 [%s]。允许的路径范围: %s",
		g.currentPhase, filePath, current.EditPaths)
	return false, warning
}

// CheckToolAllowed 检查工具在当前阶段是否允许使用。
//
// 规则：
// - 门禁未激活：所有工具允许
// - set_phase 始终允许
// - 当前阶段 tools 列表中的工具允许
// - 其他任何工具：阻止（必须先 set_phase 切换到目标阶段）
func (g *PhaseGate) CheckToolAllowed(toolName string) (bool, string) {
	if g == nil || !g.active {
		return true, ""
	}

	current := g.findPhase(g.currentPhase)
	if current == nil {
		return true, ""
	}

	// set_phase 始终允许
	if toolName == "set_phase" {
		return true, ""
	}

	// 门禁管理工具始终放行（查看/调整门禁配置，与 set_phase 同级）
	if toolName == "get_phase_gate_config" || toolName == "update_phase_gate_config" {
		return true, ""
	}

	// 当前阶段 tools 列表中的工具允许
	for _, t := range current.Tools {
		if t == toolName {
			// 事前技能强制：必读技能未加载前，禁止执行创作/维护动作。
			// 技能是创作指导，必须先读再动笔，不允许"干完活再补读解锁阶段"。
			if isMutatingTool(toolName) {
				if missing := g.missingInjections(current); len(missing) > 0 {
					warning := fmt.Sprintf("本阶段必读技能尚未加载: %v。请先用 auto_skill_injection 加载这些技能，再执行创作动作——技能是创作指导，必须先读再动笔。", missing)
					return false, warning
				}
			}
			return true, ""
		}
	}

	// 匹配类别前缀（get/create/update/delete/search/remove）
	for _, cat := range current.ToolCategories {
		if toolMatchesCategory(toolName, cat) {
			if isMutatingTool(toolName) {
				if missing := g.missingInjections(current); len(missing) > 0 {
					return false, fmt.Sprintf("本阶段必读技能尚未加载: %v。请先用 auto_skill_injection 加载这些技能，再执行创作动作——技能是创作指导，必须先读再动笔。", missing)
				}
			}
			return true, ""
		}
	}

	// 其他阶段的工具：阻止
	warning := fmt.Sprintf("当前阶段 [%s] 不允许使用工具 [%s]。需要的工具: %v。请调用 set_phase 切换到正确阶段。",
		g.currentPhase, toolName, current.Tools)
	return false, warning
}

// CheckTransitionReady 检查当前阶段是否满足切换到下一阶段的条件。
func (g *PhaseGate) CheckTransitionReady() (bool, string) {
	if g == nil || !g.active {
		return false, ""
	}

	current := g.findPhase(g.currentPhase)
	if current == nil || current.Next == "" {
		return false, ""
	}

	if g.checkRequireMet(current) {
		return true, current.Next
	}
	return false, ""
}

// ShouldAutoAdvance 判断当前阶段是否应该自动推进到下一阶段。
// 与 CheckTransitionReady 的区别：CheckTransitionReady 只检查 require 是否满足，
// ShouldAutoAdvance 额外检查是否处于 batch 循环模式（Loop=true）。
// batch 循环阶段（如 write）的 require 满足只代表本章完成，不代表整批完成，
// 由 LLM 通过 set_phase 手动控制章边界。
func (g *PhaseGate) ShouldAutoAdvance() (bool, string) {
	if g == nil || !g.active {
		return false, ""
	}
	// batch 模式下有 loop 标记的阶段（如 write），由 LLM 通过 set_phase 手动控制，
	// 不自动推进——require 满足只代表本章完成，不代表整批完成
	if g.mode == "batch" {
		if cur := g.findPhase(g.currentPhase); cur != nil && cur.Loop {
			return false, ""
		}
	}
	return g.CheckTransitionReady()
}

// toolAvailable 判断工具在当前阶段白名单（显式列表或类别前缀）中是否可用。
// 阶段不存在视为不限制（返回 true）。
func (g *PhaseGate) toolAvailable(toolName string) bool {
	cur := g.findPhase(g.currentPhase)
	if cur == nil {
		return true
	}
	for _, t := range cur.Tools {
		if t == toolName {
			return true
		}
	}
	for _, cat := range cur.ToolCategories {
		if toolMatchesCategory(toolName, cat) {
			return true
		}
	}
	return false
}

// findPhase 按名称查找阶段配置。
func (g *PhaseGate) findPhase(name string) *PhaseConfig {
	for i := range g.phases {
		if g.phases[i].Name == name {
			return &g.phases[i]
		}
	}
	return nil
}

// prevPhaseName 返回 phases 数组中指定阶段的前一个阶段名（用于 Loop 循环回退）。
func (g *PhaseGate) prevPhaseName(name string) string {
	for i := range g.phases {
		if g.phases[i].Name == name && i > 0 {
			return g.phases[i-1].Name
		}
	}
	return ""
}

// StatusString 返回当前状态的可读摘要。
func (g *PhaseGate) StatusString() string {
	if g == nil || !g.active {
		return "门禁未启用"
	}
	current := g.findPhase(g.currentPhase)
	if current == nil {
		return fmt.Sprintf("当前阶段: %s (未知)", g.currentPhase)
	}

	var called []string
	for tool, cnt := range g.successfulTools {
		if cnt > 0 {
			called = append(called, fmt.Sprintf("%s(x%d)", tool, cnt))
		}
	}

	var requireStatus string
	if len(current.Require) > 0 {
		var met, unmet []string
		for _, req := range current.Require {
			if g.successfulTools[req] > 0 {
				met = append(met, req)
			} else {
				unmet = append(unmet, req)
			}
		}
		requireStatus = fmt.Sprintf(" | require: ✅%v ❌%v", met, unmet)
	}

	wcStatus := "未检查"
	if g.wordCountOK != nil {
		if *g.wordCountOK {
			wcStatus = "✅达标"
		} else {
			wcStatus = "❌不达标"
		}
	}

	return fmt.Sprintf("阶段: %s | 已调用: %v%s | 字数: %s", g.currentPhase, called, requireStatus, wcStatus)
}

// PhaseStatus 事件数据，用于向前端报告阶段状态。
type PhaseStatus struct {
	Phase   string         `json:"phase"`
	Mode    string         `json:"mode,omitempty"`
	Called  map[string]int `json:"called"`
	Ready   bool           `json:"ready"`
	Next    string         `json:"next,omitempty"`
	Message string         `json:"message,omitempty"`
}

// Status 返回当前阶段状态的结构化数据。
func (g *PhaseGate) Status() PhaseStatus {
	ps := PhaseStatus{
		Called: make(map[string]int),
	}
	if g == nil || !g.active {
		return ps
	}
	ps.Phase = g.currentPhase
	ps.Mode = g.mode
	// 显示成功次数（require 只看成功）
	for k, v := range g.successfulTools {
		ps.Called[k] = v
	}
	ready, next := g.CheckTransitionReady()
	ps.Ready = ready
	ps.Next = next
	return ps
}

// SaveState 将门禁状态序列化为 JSON，用于持久化到 session。
// 序列化 successfulTools 与 visited（visited 丢失会导致断点续作后无法回退到已访问阶段）。
func (g *PhaseGate) SaveState() (currentPhase string, calledToolsJSON string) {
	if g == nil || !g.active {
		return "", ""
	}
	data := struct {
		Tools    map[string]int            `json:"tools"`
		Visited  []string                  `json:"visited"`
		Reads    map[string]map[string]bool `json:"reads,omitempty"`
	}{
		Tools:   g.successfulTools,
		Visited: g.visited,
		Reads:   g.readsByPhase,
	}
	b, err := json.Marshal(data)
	if err != nil {
		// 序列化失败时返回空状态，避免静默丢失门禁数据
		return g.currentPhase, "{}"
	}
	return g.currentPhase, string(b)
}

// SaveWordCount 返回字数校验状态的 JSON 片段。
func (g *PhaseGate) SaveWordCount() string {
	if g == nil || g.wordCountOK == nil {
		return ""
	}
	return fmt.Sprintf("%v", *g.wordCountOK)
}

// LoadState 从持久化数据恢复门禁状态。
// 兼容两种格式：新格式 {"tools":{...},"visited":[...]} 与旧格式 {tool: count}。
func (g *PhaseGate) LoadState(currentPhase string, calledToolsJSON string) {
	if g == nil || !g.active {
		return
	}
	if currentPhase != "" {
		g.currentPhase = currentPhase
	}
	if calledToolsJSON != "" {
		// 先尝试新格式
		var state struct {
			Tools   map[string]int             `json:"tools"`
			Visited []string                   `json:"visited"`
			Reads   map[string]map[string]bool `json:"reads"`
		}
		if json.Unmarshal([]byte(calledToolsJSON), &state) == nil && state.Tools != nil {
			g.successfulTools = state.Tools
			for k, v := range state.Tools {
				g.calledTools[k] = v
			}
			if len(state.Reads) > 0 {
				g.readsByPhase = state.Reads
			}
			if len(state.Visited) > 0 {
				g.visited = state.Visited
			} else {
				g.visited = []string{currentPhase}
			}
			return
		}
		// 旧格式：直接 tool→count map
		var tools map[string]int
		if json.Unmarshal([]byte(calledToolsJSON), &tools) == nil {
			g.successfulTools = tools
			for k, v := range tools {
				g.calledTools[k] = v
			}
			g.visited = []string{currentPhase}
		}
	}
	if len(g.visited) == 0 {
		g.visited = []string{g.currentPhase}
	}
}

// LoadWordCount 从持久化数据恢复字数校验状态。
func (g *PhaseGate) LoadWordCount(okStr string) {
	if g == nil || !g.active || okStr == "" {
		return
	}
	if okStr == "true" {
		v := true
		g.wordCountOK = &v
	} else if okStr == "false" {
		v := false
		g.wordCountOK = &v
	}
}

// ResetTo 重置门禁状态到指定阶段（清空所有调用记录、visited、reads）。
// 用于 done 阶段后新创作从 prepare 开始，防止旧状态残留导致自由回退。
func (g *PhaseGate) ResetTo(phase string) bool {
	if g == nil || !g.active {
		return false
	}
	for _, p := range g.phases {
		if p.Name == phase {
			g.currentPhase = phase
			g.visited = []string{phase}
			g.calledTools = make(map[string]int)
			g.successfulTools = make(map[string]int)
			g.lastToolResults = make(map[string]string)
			g.wordCountOK = nil
			g.readsByPhase = make(map[string]map[string]bool)
			g.roundCompleted = false
			g.consistencyErrors = nil
			return true
		}
	}
	return false
}
