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
	gate.OnToolCall("get_chapter_list", true, "")
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

// 批量 write 章边界：上一章 miniMaintain 六件套未完成时拒绝声明下一章。
func TestBatchChapterBoundaryRequiresMiniMaintain(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
mode: batch
phase: write
tools: read, edit, check, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot, set_phase
require: edit, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
next: review
loop: true
-->
`, "batch")

	// 模拟上一章只做了 2 件：edit + create_scene
	gate.OnToolCall("edit", true, "")
	gate.OnToolCall("create_scene", true, "")
	ok, warning := gate.SetPhase("write")
	if ok {
		t.Fatal("chapter boundary should be rejected when miniMaintain incomplete")
	}
	if !strings.Contains(warning, "迷你维护") {
		t.Errorf("warning should mention miniMaintain, got: %s", warning)
	}

	// 补齐剩余 4 件 + 章级一致性核对后章边界通过
	for _, tool := range []string{"update_character", "create_timeline_entry", "update_timeline_entry", "create_item_occurrence", "update_writing_snapshot", "check_story_consistency"} {
		gate.OnToolCall(tool, true, "")
	}
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("chapter boundary should pass after miniMaintain complete")
	}
}

// 章边界缺一致性核对：白名单含 check 时必须先核对再声明下一章（写时把关）。
func TestBatchChapterBoundaryRequiresConsistencyCheck(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
mode: batch
phase: write
tools: read, edit, check, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot, set_phase
require: edit, get_chapter_list, read, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot, check_story_consistency
next: review
loop: true
-->
`, "batch")

	// 六件套完成但未调 check_story_consistency → 章边界拦截
	for _, tool := range []string{"edit", "create_scene", "update_character", "create_timeline_entry", "update_timeline_entry", "create_item_occurrence", "update_writing_snapshot"} {
		gate.OnToolCall(tool, true, "")
	}
	ok, warning := gate.SetPhase("write")
	if ok {
		t.Fatal("chapter boundary should be rejected without per-chapter consistency check")
	}
	if !strings.Contains(warning, "check_story_consistency") {
		t.Errorf("warning should mention consistency check, got: %s", warning)
	}
	// 核对后放行
	gate.OnToolCall("check_story_consistency", true, "✅ 一致性检查通过")
	if ok, _ := gate.SetPhase("write"); !ok {
		t.Fatal("chapter boundary should pass after consistency check")
	}
}

// 用户自定义配置未放行 check_story_consistency 时，章边界不得被焊死。
func TestBatchChapterBoundarySkipsCheckWhenToolUnavailable(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
mode: batch
phase: write
tools: read, edit, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot, set_phase
require: edit, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
next: review
loop: true
-->
`, "batch")

	for _, tool := range []string{"edit", "create_scene", "update_character", "create_timeline_entry", "update_timeline_entry", "create_item_occurrence", "update_writing_snapshot"} {
		gate.OnToolCall(tool, true, "")
	}
	if ok, _ := gate.SetPhase("write"); !ok {
		t.Fatal("chapter boundary should pass when check tool not in whitelist")
	}
}

// 章边界通过后 ResetPhaseCounts：require 计数清零，下章必须重新结算。
func TestBatchChapterBoundaryResetCounts(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
mode: batch
phase: write
tools: read, edit, check, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot, set_phase
require: edit, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
next: review
loop: true
-->
`, "batch")

	// 上一章完成全部六件套 + 章级一致性核对
	for _, tool := range []string{"edit", "create_scene", "update_character", "create_timeline_entry", "update_timeline_entry", "create_item_occurrence", "update_writing_snapshot", "check_story_consistency"} {
		gate.OnToolCall(tool, true, "")
	}
	gate.SetWordCountOK(true)
	// 章边界通过
	if ok, _ := gate.SetPhase("write"); !ok {
		t.Fatal("chapter boundary should pass")
	}
	// 模拟 agent 章边界重置
	gate.ResetPhaseCounts()
	// 重置后：字数状态清空、require 计数清零
	if gate.WordCountCheck() != nil {
		t.Error("wordCountOK should be reset to nil")
	}
	if missing := gate.missingMiniMaintain(); len(missing) != len(miniMaintainTools) {
		t.Errorf("all miniMaintain tools should be missing after reset, got %v", missing)
	}
	// 下章转出（write→review 非同阶段）应被拦：六件套未完成
	ok, _ := gate.SetPhase("review")
	if ok {
		t.Error("exiting write without chapter miniMaintain should be blocked")
	}
}

// batch+loop 阶段（write）require 满足后不应自动推进，需 LLM 手动 set_phase。
func TestBatchLoopPhaseNoAutoAdvance(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
mode: batch
phase: write
tools: read, edit, check, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot, set_phase
require: edit, create_scene, update_character, create_timeline_entry, update_timeline_entry, create_item_occurrence, update_writing_snapshot
next: review
loop: true
-->
`, "batch")

	// 模拟完成全部 require 工具 + 章级一致性核对
	for _, tool := range []string{"edit", "create_scene", "update_character", "create_timeline_entry", "update_timeline_entry", "create_item_occurrence", "update_writing_snapshot", "check_story_consistency"} {
		gate.OnToolCall(tool, true, "")
	}
	gate.SetWordCountOK(true)

	// CheckTransitionReady 应该返回 ready（require 已满足）
	if ready, next := gate.CheckTransitionReady(); !ready || next != "review" {
		t.Errorf("CheckTransitionReady should return ready=true, next=review, got ready=%v next=%s", ready, next)
	}

	// ShouldAutoAdvance 应该返回 false（batch+loop 不自动推进）
	if ready, _ := gate.ShouldAutoAdvance(); ready {
		t.Error("ShouldAutoAdvance should return false for batch+loop phase")
	}

	// 同阶段切换 set_phase("write") 应该成功（章边界）
	if ok, _ := gate.SetPhase("write"); !ok {
		t.Error("same-phase set_phase('write') should succeed for chapter boundary")
	}

	// 模拟 agent 侧 handleBatchChapterBoundary 调用 ResetPhaseCounts 重置计数
	gate.ResetPhaseCounts()

	// 切换后 require 计数应被重置
	if missing := gate.missingMiniMaintain(); len(missing) != len(miniMaintainTools) {
		t.Errorf("all miniMaintain tools should be missing after reset, got %v", missing)
	}
}

// 单章模式 write 阶段（无 loop）ShouldAutoAdvance 应正常工作。
func TestSinglePhaseShouldAutoAdvance(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
mode: single
phase: write
tools: read, edit, get_chapter_list, check_story_consistency, set_phase
require: edit, get_chapter_list, read, check_story_consistency
next: review
-->
`, "single")

	// 模拟完成全部 require 工具
	for _, tool := range []string{"edit", "get_chapter_list", "read", "check_story_consistency"} {
		gate.OnToolCall(tool, true, "")
	}
	gate.SetWordCountOK(true)

	// CheckTransitionReady 应该返回 ready
	if ready, next := gate.CheckTransitionReady(); !ready || next != "review" {
		t.Errorf("CheckTransitionReady should return ready=true, next=review, got ready=%v next=%s", ready, next)
	}

	// ShouldAutoAdvance 也应该返回 true（单章无 loop，正常自动推进）
	if ready, next := gate.ShouldAutoAdvance(); !ready || next != "review" {
		t.Errorf("ShouldAutoAdvance should return ready=true for single phase, got ready=%v next=%s", ready, next)
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
	gate.OnToolCall("edit", true, "")
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
	nilGate.OnToolCall("read", true, "")
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
		gate.OnToolCall(tr[1], true, "")
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
	gate.OnToolCall("get_characters", true, "")
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
	gate.OnToolCall("get_characters", true, "")
	walkFullCycle(t, gate)

	// 第二轮：prepare → outline 正常推进（走 next）。
	// 阶段切换后工具计数已重置，本阶段 require（get_characters）须在本阶段内满足。
	gate.OnToolCall("get_characters", true, "")
	gate.OnToolCall("edit", true, "")
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
	gate.OnToolCall("get_characters", true, "")
	gate.OnToolCall("edit", true, "")
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline should succeed")
	}
	gate.OnToolCall("edit", true, "")
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write should succeed")
	}

	// write 回退到已访问的 outline：合法（单轮内回退修正）。
	// 回退前本阶段 require（edit）须已满足——阶段切换后计数从零计。
	gate.OnToolCall("edit", true, "")
	gate.SetWordCountOK(true) // write 转出需字数达标
	ok, _ = gate.SetPhase("outline")
	if !ok {
		t.Error("write->outline fallback within cycle should succeed")
	}
}

// 进入 write 阶段重置字数校验：上一章的字数结果不能带到本章（回归测试）
func TestPhaseGateEnterWriteResetsWordCount(t *testing.T) {
	gate := fullFlowGate(t)
	gate.OnToolCall("get_characters", true, "")
	gate.OnToolCall("edit", true, "")
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline should succeed")
	}
	gate.OnToolCall("edit", true, "")
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write should succeed")
	}

	// 进入 write 后字数状态必须重置为未检查：上一章检查的 true 不能放行本章
	if gate.WordCountCheck() != nil {
		t.Fatal("wordCountOK should reset to nil after entering write")
	}

	// 本阶段正文写入（阶段切换后 require edit 从零计）
	gate.OnToolCall("edit", true, "")

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
	gate.OnToolCall("get_characters", true, "")
	gate.OnToolCall("edit", true, "")
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline should succeed")
	}
	gate.OnToolCall("edit", true, "")
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write should succeed")
	}

	// write 回退到已访问的 prepare：单轮内仍允许（prepare 在本轮 visited 中）。
	// 回退前本阶段 require（edit）须已满足——阶段切换后计数从零计。
	gate.OnToolCall("edit", true, "")
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
		gate.OnToolCall("read", true, "")
		gate.OnToolCall("edit", true, "")
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

	gate.OnToolCall("read", true, "")
	ok, _ := gate.SetPhase("outline")
	if !ok {
		t.Fatal("prepare->outline failed")
	}
	gate.OnToolCall("edit", true, "")
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Fatal("outline->write failed")
	}

	// write 回退到 outline：合法，且 visited 不应被重置（仍含 prepare）。
	// 回退前本阶段 require（edit）须已满足——阶段切换后计数从零计。
	gate.OnToolCall("edit", true, "")
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
	gate.OnToolCall("get_characters", true, "")
	ok, _ := gate.SetPhase("maintain")
	if !ok {
		t.Fatal("prepare->maintain should succeed")
	}

	// 没查完 → 切出被阻塞
	gate.OnToolCall("edit", true, "")
	gate.OnToolCall("update_chapter_plan", true, "")
	gate.OnToolCall("update_chapter_meta", true, "")
	gate.OnToolCall("update_writing_snapshot", true, "")
	gate.OnToolCall("search_lore", true, "")
	gate.OnToolCall("search_items", true, "")
	gate.OnToolCall("get_characters", true, "")
	gate.OnToolCall("get_timeline", true, "")
	gate.OnToolCall("get_story_arcs", true, "")
	gate.OnToolCall("get_reader_perspective", true, "")
	// 缺 get_scenes / get_item_occurrences / get_character_relations
	ok, warning := gate.SetPhase("prepare")
	if ok {
		t.Error("should BLOCK: missing state checks (get_scenes/item_occurrences/character_relations)")
	}
	if warning == "" {
		t.Error("expected warning listing missing state checks")
	}

	// 查完 3 个新增查询 → 允许切出
	gate.OnToolCall("get_scenes", true, "")
	gate.OnToolCall("get_item_occurrences", true, "")
	gate.OnToolCall("get_character_relations", true, "")
	ok, warning = gate.SetPhase("prepare")
	if !ok {
		t.Errorf("should allow transition after all 13 requires met, got warning: %s", warning)
	}
}

// Bug 回归测试（真机阻塞复盘）：write 阶段调过的 edit 不能预填 maintain 的 require。
// 进入 maintain 后 require 从零计——否则轮末自动推进在 LLM 补做 goink.md 指纹（edit）前
// 就把阶段推到 done，done 白名单无 edit，指纹被冻结；set_phase("maintain") 回退后
// require 仍满又被立即推进，形成死循环。
func TestMaintainRequireNotPrefilledByWritePhase(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: prepare
tools: read, edit
require: edit
next: maintain
-->
<!-- phase-gate-config
phase: maintain
tools: read, edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
require: edit, update_chapter_plan, update_chapter_meta, update_writing_snapshot, search_lore, search_items, get_characters, get_timeline, get_story_arcs, get_reader_perspective, get_scenes, get_item_occurrences, get_character_relations
next: done
-->
<!-- phase-gate-config
phase: done
tools: get_chapter_list, get_writing_snapshot, get_phase_gate_config, set_phase
-->
`, "single")
	if gate == nil {
		t.Fatal("ParsePhaseGateConfig returned nil")
	}

	// 模拟 write 阶段：edit 已成功（正文写入）
	gate.OnToolCall("edit", true, "")
	ok, warning := gate.SetPhase("maintain")
	if !ok {
		t.Fatalf("prepare->maintain should succeed: %s", warning)
	}

	// 进入 maintain 后，write 阶段的 edit 不能算数：require 未满足，不得自动推进
	if ready, _ := gate.CheckTransitionReady(); ready {
		t.Fatal("maintain should NOT be transition-ready right after entry (edit from write phase must not prefill)")
	}

	// maintain 阶段 edit 仍可用（白名单含 edit），但 done 阶段没有 edit
	if allowed, _ := gate.CheckToolAllowed("edit"); !allowed {
		t.Fatal("edit should be allowed in maintain phase")
	}

	// 做完全部 12 项非 edit 维护动作后仍不得推进（缺 edit）
	for _, tool := range []string{"update_chapter_plan", "update_chapter_meta", "update_writing_snapshot",
		"search_lore", "search_items", "get_characters", "get_timeline", "get_story_arcs",
		"get_reader_perspective", "get_scenes", "get_item_occurrences", "get_character_relations"} {
		gate.OnToolCall(tool, true, "")
	}
	if ready, _ := gate.CheckTransitionReady(); ready {
		t.Fatal("maintain should NOT be transition-ready without edit (goink.md fingerprint)")
	}

	// 补做 edit（goink.md 指纹）后才允许推进 done
	gate.OnToolCall("edit", true, "")
	ready, next := gate.CheckTransitionReady()
	if !ready || next != "done" {
		t.Fatalf("maintain should be transition-ready to done after all 13 requires met in-phase, got ready=%v next=%q", ready, next)
	}
	ok, _ = gate.SetPhase("done")
	if !ok {
		t.Fatal("maintain->done should succeed")
	}
	// done 阶段 edit 被冻结（指纹在 maintain 阶段内已完成，此冻结是正确行为）
	if allowed, _ := gate.CheckToolAllowed("edit"); allowed {
		t.Error("edit should be blocked in done phase")
	}
}

// TestAutoSkillInjectionPerPhase 验证 auto_skill_injection 的阶段内强制语义：
// 技能必须在当前阶段内用 auto_skill_injection 读取，跨阶段读取不满足。
func TestAutoSkillInjectionPerPhase(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: outline
tools: read, auto_skill_injection, edit
require: edit
auto_skill_injection: main-tech-chapter-hook-enhanced
next: write
-->

<!-- phase-gate-config
phase: write
tools: read, auto_skill_injection, edit
require: edit
auto_skill_injection: main-tech-show-dont-tell, main-tech-anti-ai-writing
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
	gate.OnToolCall("edit", true, "")
	ok, warning := gate.SetPhase("write")
	if ok {
		t.Error("should BLOCK: outline auto_skill_injection not met")
	}
	if warning == "" {
		t.Error("expected warning listing missing skill")
	}

	// 读 outline 必读技能后放行
	gate.OnSkillInjected("main-tech-chapter-hook-enhanced")
	ok, _ = gate.SetPhase("write")
	if !ok {
		t.Error("should allow: outline auto_skill_injection met")
	}

	// write 阶段：即使前面读过其他技能，本阶段必读未读仍被拦
	gate.OnToolCall("edit", true, "")
	ok, warning = gate.SetPhase("done")
	if ok {
		t.Error("should BLOCK: write auto_skill_injection not met")
	}

	// 读 write 必读技能（跨阶段读的 outline 技能不算）
	gate.OnSkillInjected("main-tech-show-dont-tell")
	ok, warning = gate.SetPhase("done")
	if ok {
		t.Error("should BLOCK: anti-ai-writing still missing")
	}
	gate.OnSkillInjected("main-tech-anti-ai-writing")
	gate.OnToolCall("get_chapter_list", true, "")
	gate.SetWordCountOK(true)
	ok, _ = gate.SetPhase("done")
	if !ok {
		t.Error("should allow: write auto_skill_injection met")
	}
}

// TestAutoSkillInjectionBeforeCreation 验证事前技能强制：
// 必读技能未加载前，创作/维护动作（edit/update/create/run_subagent）被直接拦截，
// 而不是等 set_phase 时才补读——技能是创作指导，不是切换手续。
func TestAutoSkillInjectionBeforeCreation(t *testing.T) {
	gate := ParsePhaseGateConfig(`
<!-- phase-gate-config
phase: write
tools: read, auto_skill_injection, edit, create_scene, run_subagent, get_characters, set_phase
require: edit
auto_skill_injection: main-tech-show-dont-tell
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

	// 未读技能时：只读/查询/管理工具放行（auto_skill_injection 是加载入口）
	if allowed, _ := gate.CheckToolAllowed("auto_skill_injection"); !allowed {
		t.Error("should allow: auto_skill_injection is the loading entry")
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
	gate.OnSkillInjected("main-tech-show-dont-tell")
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
	if !strings.Contains(out, "27 项") && !strings.Contains(out, "27项") && !strings.Contains(out, "22 项") && !strings.Contains(out, "22项") {
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

// TestValidateGateConfig 验证配置校验：require 引用 tools 外工具报错、
// next 指向不存在阶段报错、技能不存在告警、edit 无 edit_paths 告警、有效配置零问题。
func TestValidateGateConfig(t *testing.T) {
	skills := []string{"main-tech-show-dont-tell", "main-tech-common-sense-logic"}

	// 坏配置：require 引用 tools 外工具 + next 指向不存在阶段 + 技能不存在 + edit 无路径限制
	bad := `
<!-- phase-gate-config
phase: prepare
tools: read, get_chapter_list
require: get_characters, missing_tool
auto_skill_injection: main-tech-no-such-skill
next: ghost
-->
<!-- phase-gate-config
phase: write
tools: edit, read
require: edit
next: prepare
-->`
	issues := ValidateGateConfig(bad, skills)
	if len(issues) == 0 {
		t.Fatal("expected issues for bad config")
	}
	var hasRequire, hasNext, hasSkill, hasEditPath bool
	for _, it := range issues {
		switch {
		case strings.Contains(it.Message, "require 引用了 tools 中没有"):
			hasRequire = true
		case strings.Contains(it.Message, "不存在"):
			if strings.Contains(it.Message, "阶段") {
				hasNext = true
			} else {
				hasSkill = true
			}
		case strings.Contains(it.Message, "edit_paths"):
			hasEditPath = true
		}
	}
	if !hasRequire {
		t.Error("expected require-not-in-tools error")
	}
	if !hasNext {
		t.Error("expected next-ghost error")
	}
	if !hasSkill {
		t.Error("expected unknown-skill warning")
	}
	if !hasEditPath {
		t.Error("expected edit-without-edit_paths warning")
	}

	// 好配置：零问题
	good := `
<!-- phase-gate-config
phase: prepare
tools: read, auto_skill_injection, get_characters, get_chapter_list
require: get_characters, get_chapter_list
auto_skill_injection: main-tech-common-sense-logic
next: write
-->
<!-- phase-gate-config
phase: write
tools: read, auto_skill_injection, edit
edit_paths: chapters/*
	require: edit
auto_skill_injection: main-tech-show-dont-tell
next: prepare
-->`
	if issues := ValidateGateConfig(good, skills); len(issues) != 0 {
		t.Errorf("expected no issues for good config, got %+v", issues)
	}
}

// ── 结果门控测试 ──────────────────────────────────────────

func TestResultGate_ErrorBlocksTransition(t *testing.T) {
	// 模拟：check_story_consistency 返回 [ERROR]，禁止推进
	config := `
<!-- phase-gate-config
mode: single
phase: review
tools: check_story_consistency
require: check_story_consistency
next: maintain
-->
<!-- phase-gate-config
mode: single
phase: maintain
tools: edit
require: edit
next: done
-->`
	gate := ParsePhaseGateConfig(config, "single")
	// 初始阶段是 review（配置中的第一个阶段）
	if gate.CurrentPhase() != "review" {
		t.Fatalf("expected initial phase 'review', got '%s'", gate.CurrentPhase())
	}

	// 调用 check_story_consistency，返回包含 [ERROR] 的结果
	gate.OnToolCall("check_story_consistency", true, "[ERROR] 死者复出：张三 状态为 dead")

	// 尝试推进到 maintain，应该被拒绝
	ok, reason := gate.SetPhase("maintain")
	if ok {
		t.Error("should block transition when check_story_consistency returns [ERROR]")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
	t.Logf("✓ 结果门控正确阻断： %s", reason)
}

func TestResultGate_WarningAllowsTransition(t *testing.T) {
	// 模拟：check_story_consistency 返回 [WARNING]，允许推进
	config := `
<!-- phase-gate-config
mode: single
phase: review
tools: check_story_consistency
require: check_story_consistency
next: maintain
-->
<!-- phase-gate-config
mode: single
phase: maintain
tools: edit
require: edit
next: done
-->`
	gate := ParsePhaseGateConfig(config, "single")

	// 调用 check_story_consistency，返回包含 [WARNING] 的结果
	gate.OnToolCall("check_story_consistency", true, "[WARNING] 角色出场断档：李四 近30章未出场")

	// 尝试推进到 maintain，应该允许
	ok, _ := gate.SetPhase("maintain")
	if !ok {
		t.Error("should allow transition when check_story_consistency returns only [WARNING]")
	}
	t.Logf("✓ [WARNING] 不阻断推进")
}

func TestResultGate_NoErrorAllowsTransition(t *testing.T) {
	// 模拟：check_story_consistency 返回通过，允许推进
	config := `
<!-- phase-gate-config
mode: single
phase: review
tools: check_story_consistency
require: check_story_consistency
next: maintain
-->
<!-- phase-gate-config
mode: single
phase: maintain
tools: edit
require: edit
next: done
-->`
	gate := ParsePhaseGateConfig(config, "single")

	// 调用 check_story_consistency，返回通过
	gate.OnToolCall("check_story_consistency", true, "✅ 一致性检查通过")

	// 尝试推进到 maintain，应该允许
	ok, _ := gate.SetPhase("maintain")
	if !ok {
		t.Error("should allow transition when check_story_consistency passes")
	}
	t.Logf("✓ 检查通过不阻断推进")
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSub(s, substr))
}

func containsSub(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
