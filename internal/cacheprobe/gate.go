// 门禁配置驱动：模拟器从真实门禁配置（门禁配置示例.md / GOINK_PHASE_CONFIG）解析
// 阶段序列、工具白名单、必读技能清单、行为开关，驱动 plays 生成——配置改动后
// 模拟器自动跟随，消除硬编码技能清单与门禁配置的漂移。
// 配置不可用时回退 legacy 硬编码表（保持旧行为）。
package cacheprobe

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// gatePhase 单个阶段配置（与 internal/agent PhaseConfig 字段对应，轻量解析避免依赖 agent 包）。
type gatePhase struct {
	Name        string
	Mode        string
	Tools       []string
	Require     []string
	Skills      []string // auto_skill_injection
	Next        string
	Loop        bool
	Inject      bool
	InjectDedup bool
	SamePhase   bool
	WordCountCheck *bool
	WordCountReset *bool
	MutatingGuard  bool
}

type gateConfig struct {
	mode   string
	Phases []gatePhase
}

var (
	gateOnce sync.Once
	simGates = map[string]*gateConfig{}
)

// loadGateConfig 加载门禁配置（mode: "single" | "batch"）。
// 来源优先级：GOINK_PHASE_CONFIG 环境变量 > 项目根目录 门禁配置示例.md（向上三级查找）> 无（返回 nil）。
func loadGateConfig(mode string) *gateConfig {
	gateOnce.Do(func() {
		content := ""
		if p := phaseConfigPath(); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				content = string(b)
			}
		}
		if content == "" {
			return
		}
		for _, m := range []string{"single", "batch"} {
			if gc := parseGateConfig(content, m); gc != nil {
				simGates[m] = gc
			}
		}
		// 门禁配置可用时用配置的技能清单重建注入表（消除硬编码漂移——
		// 配置里加/换技能后模拟器自动跟随）
		if gc := simGates["single"]; gc != nil {
			m := map[string]string{}
			for _, ph := range gc.Phases {
				if len(ph.Skills) > 0 {
					m[ph.Name] = readFilesText(ph.Skills)
				}
			}
			if len(m) > 0 {
				phaseInjectSkills = m
			}
		}
	})
	return simGates[mode]
}

func phaseConfigPath() string {
	if p := os.Getenv("GOINK_PHASE_CONFIG"); p != "" {
		return p
	}
	for _, dir := range []string{".", "..", "../..", "../../.."} {
		p := filepath.Join(dir, "门禁配置示例.md")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

var gateBlockRe = regexp.MustCompile(`(?s)<!--\s*phase-gate-config\s*\n(.*?)-->`)

func parseGateConfig(content, mode string) *gateConfig {
	gc := &gateConfig{mode: mode}
	for _, m := range gateBlockRe.FindAllStringSubmatch(content, -1) {
		ph := parseGatePhase(m[1])
		if ph.Name == "" {
			continue
		}
		if ph.Mode != "" && ph.Mode != mode {
			continue
		}
		gc.Phases = append(gc.Phases, ph)
	}
	if len(gc.Phases) == 0 {
		return nil
	}
	return gc
}

// parseGatePhase 解析单个阶段配置块。行为开关缺省开启（与 agent.parsePhaseBlock 语义一致）。
func parseGatePhase(block string) gatePhase {
	ph := gatePhase{Inject: true, InjectDedup: true, SamePhase: true, MutatingGuard: true}
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
			ph.Name = val
		case "mode":
			ph.Mode = val
		case "tools":
			ph.Tools = parseCSV(val)
		case "require":
			ph.Require = parseCSV(val)
		case "auto_skill_injection":
			ph.Skills = parseCSV(val)
		case "next":
			ph.Next = val
		case "loop":
			ph.Loop = val == "true"
		case "inject":
			ph.Inject = val == "true"
		case "inject_dedup":
			ph.InjectDedup = val == "true"
		case "same_phase":
			ph.SamePhase = val == "true"
		case "word_count_check":
			v := val == "true"
			ph.WordCountCheck = &v
		case "word_count_reset":
			v := val == "true"
			ph.WordCountReset = &v
		case "mutating_guard":
			ph.MutatingGuard = val == "true"
		}
	}
	return ph
}

func parseCSV(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// phase 按阶段名查配置。
func (g *gateConfig) phase(name string) *gatePhase {
	for i := range g.Phases {
		if g.Phases[i].Name == name {
			return &g.Phases[i]
		}
	}
	return nil
}

// skillsFor 返回阶段必读技能清单（门禁配置驱动；配置不可用时回退 legacy 硬编码表）。
func skillsFor(mode, phase string) []string {
	if gc := loadGateConfig(mode); gc != nil {
		if ph := gc.phase(phase); ph != nil && len(ph.Skills) > 0 {
			return ph.Skills
		}
	}
	return legacyPhaseSkills[phase]
}

// toolsFor 返回阶段工具白名单（门禁配置驱动；配置不可用时返回 nil=不校验）。
func toolsFor(mode, phase string) []string {
	if gc := loadGateConfig(mode); gc != nil {
		if ph := gc.phase(phase); ph != nil {
			return ph.Tools
		}
	}
	return nil
}

// gatePhaseSequence 返回阶段链（按配置顺序）。
func gatePhaseSequence(mode string) []string {
	if gc := loadGateConfig(mode); gc != nil {
		var seq []string
		for _, ph := range gc.Phases {
			seq = append(seq, ph.Name)
		}
		return seq
	}
	return nil
}

// legacyPhaseSkills 硬编码回退表（门禁配置不可用时保持旧行为，与 default_phase_gate_config 同步）。
// init 仅保留在 batch 配置中（single 已移除 init 门禁），legacy 回退保留 init 供 batch 使用。
var legacyPhaseSkills = map[string][]string{
	"init":     {"main-core-init-phase", "main-tech-genre-templates", "main-tech-book-outline", "main-tech-character-design", "main-tech-world-building-system"},
	"prepare":  {"main-tech-common-sense-logic"},
	"outline":  {"main-tech-chapter-hook-enhanced", "main-tech-chapter-title-design"},
	"write":    {"main-tech-show-dont-tell", "main-tech-anti-ai-writing", "main-tech-pov-purity", "main-tech-info-density", "main-tech-word-count-calibration"},
	"maintain": {"main-tech-anti-repetition", "main-tech-foreshadow-cycle", "main-tech-data-hygiene"},
}

// validatePlaysAgainstGate 校验 plays 序列与门禁配置的一致性：
// 每个工具调用必须在该时刻所处阶段的 tools 白名单内（set_phase 永远放行）。
// 场景开头未进入任何阶段（会话从配置首个阶段开始，但 plays 可能从 prepare 段
// 直接开始，如批量场景省去 initScript）——遇到第一个 set_phase 前的调用不校验。
// 返回违规列表（空 = 完全一致）。配置不可用或阶段缺失时跳过该段。
func validatePlaysAgainstGate(plays []play, mode string) []string {
	gc := loadGateConfig(mode)
	if gc == nil {
		return nil
	}
	current := ""
	known := map[string]bool{}
	for _, ph := range gc.Phases {
		known[ph.Name] = true
	}
	var warnings []string
	for _, p := range plays {
		if p.tool == "set_phase" {
			// 解析目标阶段
			phase := strings.Trim(strings.TrimSpace(p.args), `{}"`)
			for _, kv := range strings.Split(phase, ",") {
				parts := strings.SplitN(kv, ":", 2)
				if len(parts) == 2 && strings.Trim(parts[0], `"`) == "phase" {
					phase = strings.Trim(parts[1], `"`)
					break
				}
			}
			if known[phase] {
				current = phase
			}
			continue
		}
		if current == "" {
			continue // 首个 set_phase 前：阶段未知，跳过
		}
		ph := gc.phase(current)
		if ph == nil {
			continue
		}
		allowed := false
		for _, t := range ph.Tools {
			if t == p.tool {
				allowed = true
				break
			}
		}
		if !allowed {
			warnings = append(warnings, fmt.Sprintf("阶段 [%s] 工具 [%s] 不在白名单", current, p.tool))
		}
	}
	return warnings
}

// GateConfigLoaded 返回已加载的门禁配置文件路径（空 = 未加载，模拟器用 legacy 硬编码表）。
func GateConfigLoaded() string {
	loadGateConfig("single")
	return phaseConfigPath()
}

// ValidatePlays 校验场景 plays 与门禁配置白名单的一致性（导出，CLI 诊断用）。
// 单章场景用 gateScript（完整一章流程），批量场景用 batchAsIs。
func ValidatePlays(g, q, b int, mode string) []string {
	if b > 0 {
		return validatePlaysAgainstGate(batchAsIs(b), "batch")
	}
	if g > 0 {
		return validatePlaysAgainstGate(gateScript(0), "single")
	}
	return nil
}
