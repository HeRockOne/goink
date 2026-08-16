package cacheprobe

import (
	"fmt"
	"strings"
	"testing"

	"novel/internal/llm"
)

// 按门禁阶段统计 skill 全文的累计 token 贡献。
// 口径:某阶段的 read/read_required 结果进入历史后,在"之后每次请求"中都被重发。
// 累计贡献 = Σ(该阶段每条 skill 全文 token × 它之后发生的请求数)。
// 这直接回答:清理掉该阶段的 skill,能省多少累计输入。
func TestDiagSkillTokensByPhase(t *testing.T) {
	initTools()

	type phaseStat struct {
		name    string
		reads   int
		single  int64 // 该阶段 skill 全文单次 token 和(一次性进入历史)
		carry   int64 // 该阶段 skill 全文后续累计贡献(single × 之后请求数)
	}
	phases := []phaseStat{
		{name: "init"}, {name: "prepare"}, {name: "outline"},
		{name: "write"}, {name: "review"}, {name: "maintain"},
	}
	phaseIdx := func(name string) int {
		for i, p := range phases {
			if p.name == name {
				return i
			}
		}
		return -1
	}

	// 阶段归属:按 play 里的 set_phase 切换点切分
	// init 之后每个 play 属于"上一个 set_phase 之后的阶段"
	curPhase := "init"
	carryPending := make([]int64, len(phases)) // 已进入历史、等待累加进后续请求的 skill token(按阶段)

	cache := NewTokenCache()
	history := append([]map[string]any{}, fixedSystem()...)

	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))

	trackRequest := func() {
		// 本次请求:所有历史里的 skill 全文都被重发一次 → 各阶段 carry 累加
		for i := range phases {
			phases[i].carry += carryPending[i]
		}
	}
	trackRead := func(phase string, results []string) {
		idx := phaseIdx(phase)
		var n int64
		for _, r := range results {
			if tok, err := llm.CountTokens(r); err == nil {
				n += int64(tok)
			}
		}
		phases[idx].reads++
		phases[idx].single += n
		carryPending[idx] += n
	}

	step := func() {
		req := append(append([]map[string]any{}, history...), cur...)
		cache.Step(req)
		trackRequest()
	}
	playStep := func(p play) {
		step()
		cur = append(cur, asstToolCall("call_x", p.tool, p.args), toolMsg("call_x", p.tool, p.result))
		if p.tool == "auto_skill_injection" || p.tool == "read_required" || p.tool == "read" {
			trackRead(curPhase, []string{p.result})
		}
		if p.tool == "set_phase" {
			// 从 {"phase":"xxx"} 提取目标阶段
			curPhase = phaseFromArgs(p.args)
		}
	}

	// init
	for _, p := range initScript() {
		playStep(p)
	}
	step()
	commitCur("now", &history, cur)

	// gate 一轮
	plays := gateScript(1)
	for _, p := range plays {
		playStep(p)
	}
	step()

	fmt.Printf("\n%-10s %6s %10s %14s %12s\n", "阶段", "read数", "单次token", "后续累计", "占比%")
	fmt.Println(strings.Repeat("-", 56))
	var total int64
	for _, p := range phases {
		total += p.carry
	}
	for _, p := range phases {
		pct := 0.0
		if total > 0 {
			pct = 100 * float64(p.carry) / float64(total)
		}
		fmt.Printf("%-10s %6d %10d %14d %10.1f%%\n", p.name, p.reads, p.single, p.carry, pct)
	}
	fmt.Printf("%-10s %6s %10s %14d %10.1f%%\n", "总计", "", "", total, 100.0)
}

// phaseFromArgs 从 set_phase 的 JSON args 提取阶段名。
func phaseFromArgs(args string) string {
	idx := strings.Index(args, `"phase":"`)
	if idx < 0 {
		return args
	}
	rest := args[idx+len(`"phase":"`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return rest
	}
	return rest[:end]
}
