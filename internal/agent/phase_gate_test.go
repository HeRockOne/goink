package agent

import (
	"log/slog"
	"strings"
	"testing"

	"novel/internal/skill"
)

func TestParsePhaseGateConfig(t *testing.T) {
	markdown := `
<!-- phase-gate-config
phase: prepare
tools: get_chapter_list, read, get_characters
require: get_chapter_list, get_characters
next: outline
-->

<!-- phase-gate-config
phase: outline
tools: read, edit
require: edit
next: write
-->
`
	gate := ParsePhaseGateConfig(markdown, "single")
	if gate == nil {
		t.Fatal("expected non-nil PhaseGate")
	}
	if !gate.Active() {
		t.Fatal("expected Active() == true")
	}
	if gate.CurrentPhase() != "prepare" {
		t.Errorf("expected initial phase 'prepare', got '%s'", gate.CurrentPhase())
	}
}

func TestPhaseGateSetPhaseRequiresMet(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: get_chapter_list, read
require: get_chapter_list
next: outline
-->

<!-- phase-gate-config
phase: outline
tools: read, edit
require: edit
next: write
-->
`, "single")
	// require 未满足 → 阻塞
	ok, warning := gate.SetPhase("outline")
	if ok {
		t.Error("should BLOCK when require not met")
	}
	if warning == "" {
		t.Error("expected non-empty warning")
	}

	// 满足 require 后 → 允许
	gate.OnToolCall("get_chapter_list", true)
	ok, _ = gate.SetPhase("outline")
	if !ok {
		t.Error("should allow transition after require met")
	}
	if gate.CurrentPhase() != "outline" {
		t.Errorf("expected 'outline', got '%s'", gate.CurrentPhase())
	}
}

func TestPhaseGateSetPhaseSamePhase(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: read
require: read
next: outline
-->
`, "single")
	// 同阶段切换直接成功
	ok, _ := gate.SetPhase("prepare")
	if !ok {
		t.Error("same phase switch should succeed")
	}
}

func TestPhaseGateSetPhaseUnknown(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: read
require: read
next: outline
-->
`, "single")
	ok, warning := gate.SetPhase("nonexistent")
	if ok {
		t.Error("should not allow nonexistent phase")
	}
	if warning == "" {
		t.Error("expected non-empty warning")
	}
}

func TestPhaseGateToolBlocked(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: read, get_characters
require: get_characters
next: outline
-->

<!-- phase-gate-config
phase: outline
tools: read, edit
require: edit
next: write
-->
`, "single")
	// prepare 阶段不允许 edit
	allowed, _ := gate.CheckToolAllowed("edit")
	if allowed {
		t.Error("edit should NOT be allowed in prepare phase")
	}

	// prepare 阶段允许 read
	allowed, _ = gate.CheckToolAllowed("read")
	if !allowed {
		t.Error("read should be allowed in prepare phase")
	}

	// set_phase 始终允许
	allowed, _ = gate.CheckToolAllowed("set_phase")
	if !allowed {
		t.Error("set_phase should always be allowed")
	}
}

func TestPhaseGateEditPath(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: outline
tools: read, edit
require: edit
next: write
edit_paths: outlines/*, goink.md
-->

<!-- phase-gate-config
phase: write
tools: read, edit
require: edit
next: review
edit_paths: chapters/*
-->
`, "single")
	// outline: 只能编辑 outlines 和 goink
	allowed, _ := gate.CheckEditPath("outlines/001.md")
	if !allowed {
		t.Error("outlines/001.md should be allowed in outline")
	}
	allowed, _ = gate.CheckEditPath("goink.md")
	if !allowed {
		t.Error("goink.md should be allowed in outline")
	}
	allowed, _ = gate.CheckEditPath("chapters/001.md")
	if allowed {
		t.Error("chapters/001.md should NOT be allowed in outline")
	}

	// write: 只能编辑 chapters
	gate.SetPhase("outline")
	gate.OnToolCall("edit", true)
	gate.SetPhase("write")
	allowed, _ = gate.CheckEditPath("chapters/001.md")
	if !allowed {
		t.Error("chapters/001.md should be allowed in write")
	}
	allowed, _ = gate.CheckEditPath("outlines/001.md")
	if allowed {
		t.Error("outlines/001.md should NOT be allowed in write")
	}
}

func TestPhaseGateNilSafe(t *testing.T) {
	var nilGate *PhaseGate
	allowed, _ := nilGate.CheckToolAllowed("read")
	if !allowed {
		t.Error("nil gate should allow all tools")
	}
	nilGate.OnToolCall("read", true)
}

func fullFlowGate(t *testing.T) *PhaseGate {
	return ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: read, get_characters
require: get_characters
next: outline
-->

<!-- phase-gate-config
phase: outline
tools: read, edit
require: edit
next: write
-->

<!-- phase-gate-config
phase: write
tools: read, edit
require: edit
next: review
-->

<!-- phase-gate-config
phase: review
tools: read, edit
require: edit
next: maintain
-->

<!-- phase-gate-config
phase: maintain
tools: read, edit
require: edit
next: prepare
-->
`, "single")
}

// 走完一轮完整流程：prepare → outline → write → review → maintain → prepare
func walkFullCycle(t *testing.T, gate *PhaseGate) {
	t.Helper()
	transitions := [][2]string{
		{"outline", "edit"},
		{"write", "edit"},
		{"review", "edit"},
		{"maintain", "edit"},
		{"prepare", "edit"}, // maintain.next = prepare，回到起点开始第二轮
	}
	for _, tr := range transitions {
		gate.OnToolCall(tr[1], true)
		// write 转出需字数校验通过
		if tr[0] == "review" {
			gate.SetWordCountOK(true)
		}
		ok, _ := gate.SetPhase(tr[0])
		if !ok {
			t.Fatalf("expected forward transition to %s to succeed", tr[0])
		}
	}
}

// Bug 回归测试：完成一轮完整流程（回到起点 prepare）后，visited 已重置，
// 第二轮不能再任意跳转阶段。
func TestPhaseGateNoArbitraryJumpAfterFullCycle(t *testing.T) {
	gate := fullFlowGate(t)
	gate.OnToolCall("get_characters", true)
	walkFullCycle(t, gate)
	// 回到 prepare（第二轮开始），visited 已重置为 [prepare]

	// 第二轮：prepare 阶段不能直接跳 write（write 不在本轮 visited）
	ok, warning := gate.SetPhase("write")
	if ok {
		t.Error("after full cycle reset, jumping prepare->write should be BLOCKED (bug regression)")
	}
	if warning == "" {
		t.Error("expected non-empty warning for cross-level jump")
	}
	if gate.CurrentPhase() != "prepare" {
		t.Errorf("phase should stay prepare, got %s", gate.CurrentPhase())
	}
}

// 修复后：合法推进仍可用（第二轮正常进行）
func TestPhaseGateSecondCycleForwardStillWorks(t *testing.T) {
	gate := fullFlowGate(t)
	gate.OnToolCall("get_characters", true)
	walkFullCycle(t, gate)

	// 第二轮：prepare → outline 正常推进（走 next）
	gate.OnToolCall("edit", true)
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Error("second cycle forward transition prepare->outline should succeed")
	}
	if gate.CurrentPhase() != "outline" {
		t.Errorf("expected outline, got %s", gate.CurrentPhase())
	}
}

// 单轮内回退修正仍允许：write 发现大纲问题回 outline
func TestPhaseGateInCycleFallbackStillWorks(t *testing.T) {
	gate := fullFlowGate(t)
	gate.OnToolCall("get_characters", true)
	gate.OnToolCall("edit", true)
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline should succeed")
	}
	gate.OnToolCall("edit", true)
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write should succeed")
	}

	// write 回退到已访问的 outline：合法（单轮内回退修正）
	gate.SetWordCountOK(true) // write 转出需字数达标
	ok, _ = gate.SetPhase("outline")
	if !ok {
		t.Error("write->outline fallback within cycle should succeed")
	}
}

// 进入 write 阶段重置字数校验：上一章的字数结果不能带到本章（回归测试）
func TestPhaseGateEnterWriteResetsWordCount(t *testing.T) {
	gate := fullFlowGate(t)
	gate.OnToolCall("get_characters", true)
	gate.OnToolCall("edit", true)
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline should succeed")
	}
	gate.OnToolCall("edit", true)
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write should succeed")
	}

	// 进入 write 后字数状态必须重置为未检查：上一章检查的 true 不能放行本章
	if gate.WordCountCheck() != nil {
		t.Fatal("wordCountOK should reset to nil after entering write")
	}

	// 未重新检查字数时，write 转出必须被阻塞
	ok, msg := gate.SetPhase("review")
	if ok {
		t.Fatal("write->review should be blocked when word count not re-checked: " + msg)
	}

	// 重新检查本章字数（不达标）仍然阻塞
	gate.SetWordCountOK(false)
	ok, _ = gate.SetPhase("review")
	if ok {
		t.Fatal("write->review should be blocked when word count below minimum")
	}

	// 达标后放行
	gate.SetWordCountOK(true)
	ok, _ = gate.SetPhase("review")
	if !ok {
		t.Fatal("write->review should succeed when word count ok")
	}
}

// 单轮内回退到更早的已访问阶段仍允许（write 回 prepare 重新准备）
func TestPhaseGateInCycleFallbackToStartAllowed(t *testing.T) {
	gate := fullFlowGate(t)
	gate.OnToolCall("get_characters", true)
	gate.OnToolCall("edit", true)
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline should succeed")
	}
	gate.OnToolCall("edit", true)
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write should succeed")
	}

	// write 回退到已访问的 prepare：单轮内仍允许（prepare 在本轮 visited 中）
	gate.SetWordCountOK(true) // write 转出需字数达标
	ok, _ = gate.SetPhase("prepare")
	if !ok {
		t.Error("write->prepare fallback within cycle should succeed (prepare was visited)")
	}
}

// batch 模式回归：写多章时 write→review→maintain→done→prepare 是合法循环，
// 第二轮不能任意跳转；但 batch 的 done→prepare 应触发重置。
func TestBatchModeCycleReset(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: init
tools: read
require: read
next: prepare
-->

<!-- phase-gate-config
phase: prepare
tools: read
require: read
next: outline
-->

<!-- phase-gate-config
phase: outline
tools: read, edit
require: edit
next: write
-->

<!-- phase-gate-config
phase: write
tools: read, edit
require: edit
next: review
-->

<!-- phase-gate-config
phase: review
tools: read, edit
require: edit
next: maintain
-->

<!-- phase-gate-config
phase: maintain
tools: read, edit
require: edit
next: done
-->

<!-- phase-gate-config
phase: done
tools: read
next: prepare
-->
`, "batch")

	// 走一轮：init→prepare→outline→write→review→maintain→done→prepare
	transitions := []string{"prepare", "outline", "write", "review", "maintain", "done", "prepare"}
	for _, target := range transitions {
		gate.OnToolCall("read", true)
		gate.OnToolCall("edit", true)
		if target == "review" {
			gate.SetWordCountOK(true)
		}
		ok, warning := gate.SetPhase(target)
		if !ok {
			t.Fatalf("batch transition to %s failed: %s", target, warning)
		}
	}
	// 回到 prepare（第二轮开始），visited 应已重置

	// prepare 不能直接跳 write
	ok, _ := gate.SetPhase("write")
	if ok {
		t.Error("BUG: batch mode after full cycle, prepare->write should be BLOCKED")
	}
	if gate.CurrentPhase() != "prepare" {
		t.Errorf("phase should stay prepare, got %s", gate.CurrentPhase())
	}
}

// 单轮内 write 回退到 outline（已访问）不应误触发重置
func TestPhaseGateFallbackDoesNotReset(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: read
require: read
next: outline
-->

<!-- phase-gate-config
phase: outline
tools: read, edit
require: edit
next: write
-->

<!-- phase-gate-config
phase: write
tools: read, edit
require: edit
next: review
-->
`, "single")

	gate.OnToolCall("read", true)
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline failed")
	}
	gate.OnToolCall("edit", true)
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write failed")
	}

	// write 回退到 outline：合法，且 visited 不应被重置（仍含 prepare）
	gate.SetWordCountOK(true)
	ok, _ = gate.SetPhase("outline")
	if !ok {
		t.Fatal("write->outline fallback failed")
	}
	if len(gate.visited) < 2 {
		t.Errorf("visited should not be reset on fallback, got len=%d (want >=2)", len(gate.visited))
	}
}

// maintain 阶段 require 含全部状态查询（宁可多调用不要漏维护）：
// 每章必须查询 场景/物品流转/角色关系，确认无遗漏后才能切出。
func TestMaintainRequireIncludesStateChecks(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: read, get_characters
require: get_characters
next: maintain
-->

<!-- phase-gate-config
phase: maintain
tools: read, edit, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
next: prepare
-->
`, "single")

	// 先满足 prepare require 进入 maintain
	gate.OnToolCall("get_characters", true)
	ok, _ := gate.SetPhase("maintain")
	if !ok {
		t.Fatal("prepare->maintain should succeed")
	}

	// 没查完 → 切出被阻塞
	gate.OnToolCall("edit", true)
	gate.OnToolCall("update_chapter_plan", true)
	gate.OnToolCall("update_chapter_meta", true)
	gate.OnToolCall("update_writing_snapshot", true)
	gate.OnToolCall("search_lore", true)
	gate.OnToolCall("search_items", true)
	gate.OnToolCall("get_characters", true)
	gate.OnToolCall("get_timeline", true)
	gate.OnToolCall("get_story_arcs", true)
	gate.OnToolCall("get_reader_perspective", true)
	// 缺 get_scenes / get_item_occurrences / get_character_relations
	ok, warning := gate.SetPhase("prepare")
	if ok {
		t.Error("should BLOCK: missing state checks (get_scenes/item_occurrences/character_relations)")
	}
	if warning == "" {
		t.Error("expected warning listing missing state checks")
	}

	// 查完 3 个新增查询 → 允许切出
	gate.OnToolCall("get_scenes", true)
	gate.OnToolCall("get_item_occurrences", true)
	gate.OnToolCall("get_character_relations", true)
	ok, warning = gate.SetPhase("prepare")
	if !ok {
		t.Errorf("should allow transition after all 13 requires met, got warning: %s", warning)
	}
}

// TestRequireReadsPerPhase 验证 require_reads 的阶段内强制语义：
// 技能必须在当前阶段内用 read_required 读取，跨阶段读取不满足。
func TestRequireReadsPerPhase(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: outline
tools: read, read_required, edit
require: edit
require_reads: main-tech-chapter-hook-enhanced
next: write
-->

<!-- phase-gate-config
phase: write
tools: read, read_required, edit
require: edit
require_reads: main-tech-show-dont-tell, main-tech-anti-ai-writing
next: done
-->

<!-- phase-gate-config
phase: done
tools: read
next: prepare
-->
`, "single")
	if gate == nil {
		t.Fatal("ParsePhaseGateConfig returned nil")
	}

	// outline 阶段：未读技能时切换被拦
	gate.OnToolCall("edit", true)
	ok, warning := gate.SetPhase("write")
	if ok {
		t.Error("should BLOCK: outline require_reads not met")
	}
	if warning == "" {
		t.Error("expected warning listing missing skill")
	}

	// 读 outline 必读技能后放行
	gate.OnReadRequired("main-tech-chapter-hook-enhanced")
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Error("should allow: outline require_reads met")
	}

	// write 阶段：即使前面读过其他技能，本阶段必读未读仍被拦
	gate.OnToolCall("edit", true)
	ok, warning = gate.SetPhase("done")
	if ok {
		t.Error("should BLOCK: write require_reads not met")
	}

	// 读 write 必读技能（跨阶段读的 outline 技能不算）
	gate.OnReadRequired("main-tech-show-dont-tell")
	ok, warning = gate.SetPhase("done")
	if ok {
		t.Error("should BLOCK: anti-ai-writing still missing")
	}
	gate.OnReadRequired("main-tech-anti-ai-writing")
	gate.OnToolCall("get_chapter_list", true)
	gate.SetWordCountOK(true)
	ok, _ = gate.SetPhase("done")
	if !ok {
		t.Error("should allow: write require_reads met")
	}
}

// TestRequireReadsBeforeCreation 验证事前技能强制：
// 必读技能未加载前，创作/维护动作（edit/update/create/run_subagent）被直接拦截，
// 而不是等 set_phase 时才补读——技能是创作指导，不是切换手续。
func TestRequireReadsBeforeCreation(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: write
tools: read, read_required, edit, create_scene, run_subagent, get_characters, set_phase
require: edit
require_reads: main-tech-show-dont-tell
next: done
-->
`, "single")
	if gate == nil {
		t.Fatal("ParsePhaseGateConfig returned nil")
	}

	// 未读技能时：edit 被事前拦截
	allowed, warning := gate.CheckToolAllowed("edit")
	if allowed {
		t.Error("should BLOCK: edit before required skill loaded")
	}
	if warning == "" || !strings.Contains(warning, "main-tech-show-dont-tell") {
		t.Errorf("warning should name missing skill, got: %q", warning)
	}

	// 未读技能时：维护类工具同样被拦
	if allowed, _ := gate.CheckToolAllowed("create_scene"); allowed {
		t.Error("should BLOCK: create_scene before required skill loaded")
	}
	if allowed, _ := gate.CheckToolAllowed("run_subagent"); allowed {
		t.Error("should BLOCK: run_subagent before required skill loaded")
	}

	// 未读技能时：只读/查询/管理工具放行（read_required 是加载入口）
	if allowed, _ := gate.CheckToolAllowed("read_required"); !allowed {
		t.Error("should allow: read_required is the loading entry")
	}
	if allowed, _ := gate.CheckToolAllowed("read"); !allowed {
		t.Error("should allow: read")
	}
	if allowed, _ := gate.CheckToolAllowed("get_characters"); !allowed {
		t.Error("should allow: get_characters")
	}
	if allowed, _ := gate.CheckToolAllowed("set_phase"); !allowed {
		t.Error("should allow: set_phase")
	}

	// 读技能后：edit 放行
	gate.OnReadRequired("main-tech-show-dont-tell")
	if allowed, _ := gate.CheckToolAllowed("edit"); !allowed {
		t.Error("should allow: edit after required skill loaded")
	}
	if allowed, _ := gate.CheckToolAllowed("create_scene"); !allowed {
		t.Error("should allow: create_scene after required skill loaded")
	}
}


// review 自动注入所有 sub-*（含三层查找），不硬编码技能名；无 sub- 时返回空。
func TestBuildSubagentSkills(t *testing.T) {
	store, err := skill.NewStore(slog.Default(), "")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	a := &Agent{skillStore: store}

	out := a.buildSubagentSkills(0)
	if out == "" {
		t.Fatal("expected sub-* skills injected (builtin has sub-tech-review-standards + sub-tech-anti-ai-grade)")
	}
	// 必须包含两个现有 sub- 技能（按内容特征而非技能名）
	if !strings.Contains(out, "22 项硬伤检查") {
		t.Error("expected sub-tech-review-standards content injected")
	}
	if !strings.Contains(out, "T1 出现即换") {
		t.Error("expected sub-tech-anti-ai-grade content injected")
	}
	// 不得包含 main- 技能
	if strings.Contains(out, "main-tech-show-dont-tell") {
		t.Error("main- skill should NOT be injected into subagent")
	}
}
