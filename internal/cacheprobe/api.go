package cacheprobe

import (
	"fmt"
	"log/slog"
)

// Options 是模拟参数。
type Options struct {
	GateRounds    int  // 门禁创作轮数（每轮 = 一章完整流程）
	ShortQARounds int  // 短对话穿插轮数（0 = 不穿插）
	MixShortQA    bool // 是否在门禁轮之间穿插短对话
}

// ScenarioResult 单个场景的结果（now/legacy 或单协议）。
type ScenarioResult struct {
	Name       string  `json:"name"`
	NowHit     int64   `json:"now_hit"`
	NowMiss    int64   `json:"now_miss"`
	LegacyHit  int64   `json:"legacy_hit"`
	LegacyMiss int64   `json:"legacy_miss"`
}

// Result 模拟总结果。
type Result struct {
	Scenarios []ScenarioResult `json:"scenarios"`
	TotalNowHit   int64 `json:"total_now_hit"`
	TotalNowMiss  int64 `json:"total_now_miss"`
	TotalLegacyHit  int64 `json:"total_legacy_hit"`
	TotalLegacyMiss int64 `json:"total_legacy_miss"`
}

// Run 执行缓存模拟，返回 now（NS 落库）vs legacy（NS 不落库）对照。
// rounds 控制门禁创作轮数；shortQA 控制穿插的短对话轮数（0 则不穿插）。
func Run(gateRounds, shortQARounds int) (*Result, error) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	if gateRounds <= 0 {
		gateRounds = 5
	}
	if shortQARounds < 0 {
		shortQARounds = 0
	}

	res := &Result{}

	// 短对话场景
	if shortQARounds > 0 {
		sc := runPair("短对话 "+fmt.Sprint(shortQARounds)+" 轮", func(mode string, c *TokenCache) [][2]int64 {
			return buildShortQAWithRounds(mode, c, shortQARounds)
		})
		res.Scenarios = append(res.Scenarios, sc)
	}

	// 门禁创作场景
	sc := runPair(fmt.Sprintf("门禁创作 %d 轮", gateRounds), func(mode string, c *TokenCache) [][2]int64 {
		return buildGateWithRounds(mode, c, gateRounds)
	})
	res.Scenarios = append(res.Scenarios, sc)

	for _, s := range res.Scenarios {
		res.TotalNowHit += s.NowHit
		res.TotalNowMiss += s.NowMiss
		res.TotalLegacyHit += s.LegacyHit
		res.TotalLegacyMiss += s.LegacyMiss
	}
	return res, nil
}

// runPair 跑 now/legacy 两种协议并汇总。
func runPair(name string, fn func(string, *TokenCache) [][2]int64) ScenarioResult {
	nowCache := NewTokenCache()
	legacyCache := NewTokenCache()
	nowR := fn("now", nowCache)
	legR := fn("legacy", legacyCache)

	var nH, nM, lH, lM int64
	for _, pr := range nowR {
		nH += pr[0]
		nM += pr[1]
	}
	for _, pr := range legR {
		lH += pr[0]
		lM += pr[1]
	}
	return ScenarioResult{Name: name, NowHit: nH, NowMiss: nM, LegacyHit: lH, LegacyMiss: lM}
}

// phaseReminder 模拟 set_phase 后的 system-reminder 注入（真实 agent.go:483：appendMsg("user", <system-reminder>结果JSON)）。
func phaseReminder(phase string, ok bool) map[string]any {
	if ok {
		return userMsg("<system-reminder>\n{\"success\":true,\"phase\":\"" + phase + "\",\"status\":\"active\"}\n</system-reminder>")
	}
	return userMsg("<system-reminder>\n{\"success\":false,\"error\":\"require 未满足\",\"current_phase\":\"" + phase + "\"}\n</system-reminder>")
}

// buildGateWithRounds 按指定轮数跑门禁场景。
func buildGateWithRounds(mode string, cache *TokenCache, rounds int) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))
	for i, p := range initScript() {
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		cur = append(cur,
			asstToolCall(fmt.Sprintf("call_init_p%d", i), p.tool, p.args),
			toolMsg(fmt.Sprintf("call_init_p%d", i), p.tool, p.result),
		)
		if p.tool == "set_phase" {
			cur = append(cur, phaseReminder(p.args, true))
		}
	}
	cur = append(cur, asstText("开书完成：世界观、角色、总纲、第一卷弧线已建立，进入第一章创作。"))
	req := append(append([]map[string]any{}, history...), cur...)
	hit, miss := cache.Step(req)
	results = append(results, [2]int64{hit, miss})
	if mode == "now" {
		history = append(history, cur...)
	} else {
		legacyCur := append([]map[string]any{}, cur[0])
		legacyCur = append(legacyCur, cur[2:]...)
		history = append(history, legacyCur...)
	}

	for turn := 1; turn <= rounds; turn++ {
		cur := []map[string]any{userMsg(fmt.Sprintf("请创作第 %d 章，继续推进剧情。", turn+1))}
		cur = append(cur, sysMsg(novelState(turn)))

		plays := gateScript(turn)
		for i, p := range plays {
			req := append(append([]map[string]any{}, history...), cur...)
			hit, miss := cache.Step(req)
			results = append(results, [2]int64{hit, miss})
			cur = append(cur, asstToolCall(fmt.Sprintf("call_t%d_p%d", turn, i), p.tool, p.args))
			if p.tool == "run_subagent" {
				subResults := simulateSubagent(cache, history, cur, turn)
				results = append(results, subResults...)
			}
			cur = append(cur, toolMsg(fmt.Sprintf("call_t%d_p%d", turn, i), p.tool, p.result))
			if p.tool == "set_phase" {
				cur = append(cur, phaseReminder(p.args, true))
			}
		}
		cur = append(cur, asstText(finalAssistant(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})

		if mode == "now" {
			history = append(history, cur...)
		} else {
			legacyCur := append([]map[string]any{}, cur[0])
			legacyCur = append(legacyCur, cur[2:]...)
			history = append(history, legacyCur...)
		}
	}
	return results
}

// buildShortQAWithRounds 按指定轮数跑短对话场景。
func buildShortQAWithRounds(mode string, cache *TokenCache, rounds int) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	for turn := 1; turn <= rounds; turn++ {
		cur := []map[string]any{userMsg(fmt.Sprintf("第 %d 问：这个世界的修炼体系是什么？", turn))}
		cur = append(cur, sysMsg(novelState(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})

		if mode == "now" {
			history = append(history, cur...)
		} else {
			history = append(history, cur[:1]...)
		}
		history = append(history, asstText(shortAnswer()))
	}
	return results
}
