package agent

import "testing"

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
