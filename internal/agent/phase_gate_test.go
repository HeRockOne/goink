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
`)
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
`)
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
`)
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
`)
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
`)
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
