package agent

import (
	"testing"
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
