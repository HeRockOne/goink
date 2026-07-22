package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// PhaseGate 从 always-mode skill 内容中解析阶段门禁配置，跟踪阶段状态，执行门禁检查。
//
// 核心设计：
// 1. 支持 single（单章）和 batch（批量）两种模式
// 2. 初始阶段为配置中的第一个阶段
// 3. 每次工具调用后，记录调用成功次数
// 4. set_phase 切换阶段时检查 require，未满足则阻塞
// 5. 不自动推进——用户/LLM 必须主动调 set_phase 切换阶段
type PhaseGate struct {
	phases          []PhaseConfig
	currentPhase    string
	calledTools     map[string]int // tool_name → 调用次数（含失败）
	successfulTools map[string]int // tool_name → 成功次数（require 只看这个）
	mode            string         // "single" | "batch"
	active          bool           // 是否启用
	wordCountOK     *bool          // get_chapter_list 字数校验结果（nil=未检查）
}

// PhaseConfig 是单个阶段的配置。
type PhaseConfig struct {
	Name     string   // 阶段名称
	Mode     string   // 所属模式："single" | "batch"（空=两种模式都适用）
	Tools    []string // 允许使用的工具
	Require  []string // 必须调用过的工具（完成条件）
	Next     string   // 满足条件后可进入的下一阶段
	FailNext string   // require 不满足时的回退阶段
	Loop     bool     // batch 模式下是否循环（write → outline）
	EditPaths string   // edit 工具的路径范围（如 "outlines/*, goink.md"，"*" = 不限制）
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
		calledTools:     make(map[string]int),
		successfulTools: make(map[string]int),
		mode:            mode,
		active:          true,
	}
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
			pc.Tools = parseToolList(val)
		case "require":
			pc.Require = parseToolList(val)
		case "next":
			pc.Next = val
		case "fail_next":
			pc.FailNext = val
		case "loop":
			pc.Loop = val == "true"
		case "edit_paths":
			pc.EditPaths = val
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
// 不再自动推进阶段——用户必须手动调 set_phase 推进。
func (g *PhaseGate) OnToolCall(toolName string, success bool) {
	if g == nil || !g.active {
		return
	}

	g.calledTools[toolName]++
	if success {
		g.successfulTools[toolName]++
	}
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

	// 同阶段切换：直接成功，不检查 require
	if g.currentPhase == targetPhase {
		return true, ""
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

	// write 阶段转出时强制检查字数
	if g.currentPhase == "write" && targetPhase != "write" {
		if g.wordCountOK == nil {
			return false, fmt.Sprintf("阶段 [write] 转出前必须调用 get_chapter_list 校验字数，请先调用该工具")
		}
		if !*g.wordCountOK {
			return false, fmt.Sprintf("阶段 [write] 最新章节字数不达标，请扩写后再检查")
		}
	}

	g.currentPhase = targetPhase
	return true, ""
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
		// 支持 * 通配符匹配
		matched, _ := filepath.Match(pattern, filePath)
		if matched {
			return true, ""
		}
		// 支持目录前缀匹配（如 outlines/* 匹配 outlines/001.md）
		if strings.HasSuffix(pattern, "/*") {
			dir := strings.TrimSuffix(pattern, "/*")
			if strings.HasPrefix(filePath, dir+"/") || filePath == dir {
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

	// 当前阶段 tools 列表中的工具允许
	for _, t := range current.Tools {
		if t == toolName {
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

// findPhase 按名称查找阶段配置。
func (g *PhaseGate) findPhase(name string) *PhaseConfig {
	for i := range g.phases {
		if g.phases[i].Name == name {
			return &g.phases[i]
		}
	}
	return nil
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
func (g *PhaseGate) SaveState() (currentPhase string, calledToolsJSON string) {
	if g == nil || !g.active {
		return "", ""
	}
	// 保存成功次数（require 只看成功）
	b, _ := json.Marshal(g.successfulTools)
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
func (g *PhaseGate) LoadState(currentPhase string, calledToolsJSON string) {
	if g == nil || !g.active {
		return
	}
	if currentPhase != "" {
		g.currentPhase = currentPhase
	}
	if calledToolsJSON != "" {
		var tools map[string]int
		if json.Unmarshal([]byte(calledToolsJSON), &tools) == nil {
			g.successfulTools = tools
			// 恢复时也填充 calledTools（向后兼容）
			for k, v := range tools {
				g.calledTools[k] = v
			}
		}
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
