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

// ScenarioResult 单个场景的结果（now/legacy/clean 三协议对照）。
type ScenarioResult struct {
	Name       string  `json:"name"`
	NowHit     int64   `json:"now_hit"`
	NowMiss    int64   `json:"now_miss"`
	LegacyHit  int64   `json:"legacy_hit"`
	LegacyMiss int64   `json:"legacy_miss"`
	// clean：NS 落库 + 工具结果清理（read/read_required 的 skill 全文 → 占位符）
	CleanHit  int64 `json:"clean_hit"`
	CleanMiss int64 `json:"clean_miss"`
	// LLM 输出 token（assistant 消息字节，含 reasoning_content）
	NowOutput    int64 `json:"now_output"`
	LegacyOutput int64 `json:"legacy_output"`
	CleanOutput  int64 `json:"clean_output"`
	// miss 构成（按消息来源分类，诊断/成本明细用）
	NowMissByCat    map[string]int64 `json:"now_miss_by_cat,omitempty"`
	LegacyMissByCat map[string]int64 `json:"legacy_miss_by_cat,omitempty"`
	CleanMissByCat  map[string]int64 `json:"clean_miss_by_cat,omitempty"`
}

// Result 模拟总结果。
type Result struct {
	Scenarios []ScenarioResult `json:"scenarios"`
	TotalNowHit   int64 `json:"total_now_hit"`
	TotalNowMiss  int64 `json:"total_now_miss"`
	TotalLegacyHit  int64 `json:"total_legacy_hit"`
	TotalLegacyMiss int64 `json:"total_legacy_miss"`
	TotalCleanHit  int64 `json:"total_clean_hit"`
	TotalCleanMiss int64 `json:"total_clean_miss"`
	TotalNowOutput    int64 `json:"total_now_output"`
	TotalLegacyOutput int64 `json:"total_legacy_output"`
	TotalCleanOutput  int64 `json:"total_clean_output"`
	// miss 构成（now 协议，按消息来源分类）
	TotalNowMissByCat map[string]int64 `json:"total_now_miss_by_cat,omitempty"`
}

// Run 执行缓存模拟，三方对照：
//   legacy：NS 不落库（修复前基线）
//   now：NS 落库（当前优化，缓存前缀连续）
//   clean：NS 落库 + 工具结果清理（read/read_required 的 skill 全文 → 占位符，防注意力漂移）
// gateRounds 控制单章创作轮数（0=不跑单章）；shortQARounds 控制短对话穿插轮数（0=不穿插）；
// batchChapters 控制批量创作章数（0=不跑批量）；cleanRetain 控制清理保留窗口（<0 = 不清理）。
// 所有轮次发生在同一个对话窗口：init → 短对话 → 单章 → 短对话 → 批量 → 短对话，一条历史贯穿。
func Run(gateRounds, shortQARounds, batchChapters, cleanRetain int) (*Result, error) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	if gateRounds < 0 {
		gateRounds = 5
	}
	if shortQARounds < 0 {
		shortQARounds = 0
	}
	if batchChapters < 0 {
		batchChapters = 0
	}

	res := &Result{}

// 混合对话窗口场景（真实使用方式）
	// auto = 当前自动注入行为, now = 旧 read_required 行为（两者都有 NS 缓存，差异仅 auto-inject）
	sc := runTriple(fmt.Sprintf("对话窗口（单章 %d 轮 · 短对话 %d 轮 · 批量 %d 章）", gateRounds, shortQARounds, batchChapters), cleanRetain, func(mode string, c *TokenCache) [][2]int64 {
		simMode := mode
		if mode == "now" {
			simMode = "auto" // 当前版：auto-inject
		}
		if mode == "legacy" {
			simMode = "now" // 旧版：read_required（NS 缓存，无 auto-inject）
		}
		return buildMixedSession(simMode, c, gateRounds, shortQARounds, batchChapters)
	})
	res.Scenarios = append(res.Scenarios, sc)

	for _, s := range res.Scenarios {
		res.TotalNowHit += s.NowHit
		res.TotalNowMiss += s.NowMiss
		res.TotalLegacyHit += s.LegacyHit
		res.TotalLegacyMiss += s.LegacyMiss
		res.TotalCleanHit += s.CleanHit
		res.TotalCleanMiss += s.CleanMiss
		res.TotalNowOutput += s.NowOutput
		res.TotalLegacyOutput += s.LegacyOutput
		res.TotalCleanOutput += s.CleanOutput
		if res.TotalNowMissByCat == nil {
			res.TotalNowMissByCat = map[string]int64{}
		}
		for k, v := range s.NowMissByCat {
			res.TotalNowMissByCat[k] += v
		}
	}
	return res, nil
}

// MatrixCell 矩阵单元格：某会话结构 × 某保留窗口的结果。
type MatrixCell struct {
	Retain       int     // 保留窗口（-1 = now 不清理，>=0 = clean 保留 N 条）
	TotalIn      int64   // 总输入 token（hit+miss，真实成本口径）
	Miss         int64   // 未命中 token（全价计费）
	HitRate      float64 // 缓存命中率（%）
	SaveVsNowPct float64 // 相对 now 的总输入降幅（%）
	Cost         float64 // 估算成本（元）：hit×0.02 + miss×1（DeepSeek V4-Flash/mimo 同价）
	CostSavePct  float64 // 相对 now 的成本降幅（%）
}

// MatrixRow 矩阵行：一个会话结构在各保留窗口下的结果。
type MatrixRow struct {
	Name  string
	Cells []MatrixCell
}

// MatrixResult 边界矩阵总结果。
type MatrixResult struct {
	Rows []MatrixRow
}

// ModeCompare 创作模式对比单元格（同章数,不同模式 × 是否清理）。
type ModeCompare struct {
	Name      string  // 模式描述
	TotalIn   int64   // 总输入 token
	Miss      int64   // 未命中 token
	HitRate   float64 // 缓存命中率 %
	Cost      float64 // 成本（hit×0.02 + miss×1,真实价）
	CostSave  float64 // 相对"单章不清理"的成本降幅 %
}

// CompareResult 模式对比总结果。
type CompareResult struct {
	Modes []ModeCompare
}

// CompareModes 同章数(4 章)不同创作模式的真实成本对比：
// 单章 4 轮 vs 批量 2批×2章,各 × 无清理/clean(全清)。
// 回答：批量模式(轮边界少)与 clean(历史瘦身)谁更省,能否叠加。
func CompareModes() (*CompareResult, error) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	const cachePrice, missPrice = 0.02, 1.0
	cost := func(h, m int64) float64 {
		return float64(h)*cachePrice/1e6 + float64(m)*missPrice/1e6
	}
	run := func(name string, c *TokenCache, fn func(string, *TokenCache) [][2]int64) ModeCompare {
		r := fn("clean", c)
		var h, m int64
		for _, pr := range r {
			h += pr[0]
			m += pr[1]
		}
		return ModeCompare{
			Name:    name,
			TotalIn: h + m,
			Miss:    m,
			HitRate: 100 * float64(h) / float64(h+m),
			Cost:    cost(h, m),
		}
	}

	// 基准：单章 4 轮,不清理
	base := run("单章 4 轮(不清理)", NewTokenCache(), func(mode string, c *TokenCache) [][2]int64 {
		return buildGateWithRounds(mode, c, 4)
	})
	base.CostSave = 0

	modes := []ModeCompare{base}
	appendMode := func(name string, c *TokenCache, fn func(string, *TokenCache) [][2]int64) {
		mc := run(name, c, fn)
		mc.CostSave = 0
		if base.Cost > 0 {
			mc.CostSave = (base.Cost - mc.Cost) / base.Cost * 100
		}
		modes = append(modes, mc)
	}

	appendMode("批量 2批×2章(不清理)", NewTokenCache(), func(mode string, c *TokenCache) [][2]int64 {
		return buildBatchWithRounds(mode, c, 2)
	})
	appendMode("单章 4 轮 + clean(全清)", NewCleanCache(0), func(mode string, c *TokenCache) [][2]int64 {
		return buildGateWithRounds(mode, c, 4)
	})
	appendMode("批量 2批×2章 + clean(全清)", NewCleanCache(0), func(mode string, c *TokenCache) [][2]int64 {
		return buildBatchWithRounds(mode, c, 2)
	})
	// 阶段切换清理（set_phase 时清上一阶段 read，当前阶段保留全文）
	appendMode("单章 4 轮 + 阶段清理", NewPhaseCleanCache(), func(mode string, c *TokenCache) [][2]int64 {
		return buildGatePhaseClean(c, 4)
	})

	return &CompareResult{Modes: modes}, nil
}

// RunMatrix 边界矩阵实验：多组会话结构 × 保留窗口（-1/0/1/3/5），
// 定位 clean 方案的收益边界与拐点（保留窗口多大时收益消失/劣化）。
func RunMatrix() (*MatrixResult, error) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	structs := []struct {
		name string
		g, q, b int
	}{
		{"单章1轮", 1, 0, 0},
		{"批量1批2章", 0, 0, 2},
		{"混合(1+1+2)", 1, 1, 2},
		{"长会话(5+3+5)", 5, 3, 5},
	}
	retains := []int{-1, 0, 1, 3, 5}

	res := &MatrixResult{}
	// 真实价格（DeepSeek V4-Flash / mimo 同价，元/百万 token）
	const cachePrice, missPrice = 0.02, 1.0
	cost := func(h, m int64) float64 {
		return float64(h)*cachePrice/1e6 + float64(m)*missPrice/1e6
	}
	for _, st := range structs {
		row := MatrixRow{Name: st.name}
		// now 基线（retain=-1）：单独的 TokenCache 跑一遍，作为降幅基准
		var nowTotal int64
		var nowCost float64
		{
			c := NewTokenCache()
			r := buildMixedSession("now", c, st.g, st.q, st.b)
			var h, m int64
			for _, pr := range r {
				h += pr[0]
				m += pr[1]
			}
			nowTotal = h + m
			nowCost = cost(h, m)
		}
		for _, rt := range retains {
			var c *TokenCache
			if rt < 0 {
				c = NewTokenCache()
			} else {
				c = NewCleanCache(rt)
			}
			r := buildMixedSession("clean", c, st.g, st.q, st.b)
			var h, m int64
			for _, pr := range r {
				h += pr[0]
				m += pr[1]
			}
			total := h + m
			save := 0.0
			if nowTotal > 0 {
				save = float64(nowTotal-total) / float64(nowTotal) * 100
			}
			cst := cost(h, m)
			costSave := 0.0
			if nowCost > 0 {
				costSave = (nowCost - cst) / nowCost * 100
			}
			row.Cells = append(row.Cells, MatrixCell{
				Retain:       rt,
				TotalIn:      total,
				Miss:         m,
				HitRate:      100 * float64(h) / float64(h+m),
				SaveVsNowPct: save,
				Cost:         cst,
				CostSavePct:  costSave,
			})
		}
		res.Rows = append(res.Rows, row)
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

// runTriple 跑 now/legacy/clean(retain) 三种协议并汇总（clean 用发送前变换的 TokenCache）。
func runTriple(name string, retain int, fn func(string, *TokenCache) [][2]int64) ScenarioResult {
	nowCache := NewTokenCache()
	nowCache.SetMissCat(missCatOf)
	legacyCache := NewTokenCache()
	legacyCache.SetMissCat(missCatOf)
	cleanCache := NewCleanCache(retain)
	cleanCache.SetMissCat(missCatOf)
	nowR := fn("now", nowCache)
	legR := fn("legacy", legacyCache)
	cleanR := fn("clean", cleanCache)

	var nH, nM, lH, lM, cH, cM int64
	for _, pr := range nowR {
		nH += pr[0]
		nM += pr[1]
	}
	for _, pr := range legR {
		lH += pr[0]
		lM += pr[1]
	}
	for _, pr := range cleanR {
		cH += pr[0]
		cM += pr[1]
	}
	return ScenarioResult{
		Name: name, NowHit: nH, NowMiss: nM, LegacyHit: lH, LegacyMiss: lM, CleanHit: cH, CleanMiss: cM,
		NowOutput: nowCache.Output(), LegacyOutput: legacyCache.Output(), CleanOutput: cleanCache.Output(),
		NowMissByCat: nowCache.MissByCat, LegacyMissByCat: legacyCache.MissByCat, CleanMissByCat: cleanCache.MissByCat,
	}
}

// appendPhase 模拟 set_phase 后的 system-reminder 注入（与真机 agent.go 对齐：
// 成功注入静态确认"已切换到 [X] 阶段"（600aa8c 起静态化，禁止动态 StatusString——
// 动态内容注入历史中段会让整条前缀缓存失效）；失败分支保留"缺什么"信息。
func appendPhase(cur []map[string]any, phase string, ok bool) []map[string]any {
	if !ok {
		return append(cur, userMsg("<system-reminder>\n{\"success\":false,\"error\":\"require 未满足\",\"current_phase\":\""+phase+"\"}\n</system-reminder>"))
	}
	return append(cur, userMsg(fmt.Sprintf("<system-reminder>\n已切换到 [%s] 阶段，继续执行该阶段任务。\n</system-reminder>", phase)))
}

// buildGatePhaseClean 单章门禁 + 阶段切换清理：
// prepare → outline → write → review → maintain 各阶段结束时（set_phase 切换点），
// 把上一阶段 read/read_required 的调用 ID 标记为可清（cache.MarkCleared），
// 发送前 transform 替换为占位符。当前阶段 read 保留全文（写正文时技能完整可用）。
// 与"每请求 keep 窗口清理"的关键差异：阶段内永不清理，语义安全。
func buildGatePhaseClean(cache *TokenCache, rounds int) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))
	for i, p := range initScript() {
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		if p.tool == "set_phase" {
			simPhase = p.args
			cur = appendPhase(cur, p.args, true)
		}
		cur = append(cur,
			asstToolCall(fmt.Sprintf("call_init_p%d", i), p.tool, p.args),
			toolMsg(fmt.Sprintf("call_init_p%d", i), p.tool, playResult(p)),
		)
	}
	cur = append(cur, asstText("开书完成：世界观、角色、总纲、第一卷弧线已建立，进入第一章创作。"))
	req := append(append([]map[string]any{}, history...), cur...)
	hit, miss := cache.Step(req)
	results = append(results, [2]int64{hit, miss})
	history = append(history, cur...)

	for turn := 1; turn <= rounds; turn++ {
		cur := []map[string]any{userMsg(fmt.Sprintf("请创作第 %d 章，继续推进剧情。", turn+1))}
		cur = append(cur, sysMsg(novelState(turn)))

		// 阶段追踪：当前阶段内收集的 read 调用 ID，遇到 set_phase 时标记上一阶段
		pendingReads := make([]string, 0)
		markPhase := func() {
			if len(pendingReads) > 0 {
				cache.MarkCleared(pendingReads...)
				pendingReads = pendingReads[:0]
			}
		}

		plays := gateScript(turn)
		cur = runPlays(cache, history, cur, plays, &results,
			func(subCur []map[string]any) [][2]int64 {
				return simulateSubagent(cache, history, subCur, turn)
			},
			func(id string) { pendingReads = append(pendingReads, id) },
			func(c []map[string]any, phase string) []map[string]any {
				markPhase()
				return c
			},
		)
		cur = append(cur, asstText(finalAssistant(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		history = append(history, cur...)
	}
	return results
}

// buildGateWithRounds 按指定轮数跑门禁场景。
func buildGateWithRounds(mode string, cache *TokenCache, rounds int) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))
	if mode == "auto" {
		cur = append(cur, sysMsg(initInject)) // auto：init 必读技能直接注入
	}
	for i, p := range initScript() {
		if mode == "auto" && (p.tool == "auto_skill_injection" || p.tool == "read_required") {
			continue // auto：init 技能已系统注入，跳过技能读取工具
		}
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		if p.tool == "set_phase" {
			simPhase = p.args
			if mode == "auto" {
				if sk, ok := phaseInjectSkills[p.args]; ok && sk != "" {
					cur = append(cur, sysMsg(sk))
				}
			}
			cur = appendPhase(cur, p.args, true)
		}
		cur = append(cur,
			asstToolCall(fmt.Sprintf("call_init_p%d", i), p.tool, p.args),
			toolMsg(fmt.Sprintf("call_init_p%d", i), p.tool, playResult(p)),
		)
	}
	cur = append(cur, asstText("开书完成：世界观、角色、总纲、第一卷弧线已建立，进入第一章创作。"))
	req := append(append([]map[string]any{}, history...), cur...)
	hit, miss := cache.Step(req)
	results = append(results, [2]int64{hit, miss})
	if mode == "auto" {
		history = append(history, cur...)
	} else if mode == "now" {
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
			if mode == "auto" && (p.tool == "auto_skill_injection" || p.tool == "read_required") {
				continue // auto：技能在进入阶段时系统注入，跳过技能读取工具
			}
			req := append(append([]map[string]any{}, history...), cur...)
			hit, miss := cache.Step(req)
			results = append(results, [2]int64{hit, miss})
			id := fmt.Sprintf("call_t%d_p%d", turn, i)
			if p.tool == "set_phase" {
				simPhase = p.args
				if mode == "auto" {
					if sk, ok := phaseInjectSkills[p.args]; ok && sk != "" {
						cur = append(cur, sysMsg(sk))
					}
				}
				cur = appendPhase(cur, p.args, true)
			}
			cur = append(cur, asstToolCall(id, p.tool, p.args))
			if p.tool == "run_subagent" {
				subResults := simulateSubagent(cache, history, cur, turn)
				results = append(results, subResults...)
			}
			cur = append(cur, toolMsg(id, p.tool, playResult(p)))
		}
		cur = append(cur, asstText(finalAssistant(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})

		if mode == "auto" {
			history = append(history, cur...)
		} else if mode == "now" {
			history = append(history, cur...)
		} else {
			legacyCur := append([]map[string]any{}, cur[0])
			legacyCur = append(legacyCur, cur[2:]...)
			history = append(history, legacyCur...)
		}
	}
	return results
}

// isOptMode 判断是否"优化模式"（auto-inject + 自动推进 + 去提醒）
func isOptMode(mode string) bool { return mode == "opt" }

// skipReadRequired 判断是否跳过 read_required（auto 和 opt 模式）
func skipReadRequired(mode string) bool { return mode == "auto" || mode == "opt" }

// buildGateWithRoundsOpt 优化模式：auto-inject + set_phase 自动推进（不生成消息）+ 去掉成功提醒。
func buildGateWithRoundsOpt(cache *TokenCache, rounds int) [][2]int64 {
	return buildOptWithPrefix(cache, rounds, fixedSystem())
}

// buildGateWithRoundsOptNoCat 优化模式 + 去掉 catalog（auto 技能改 manual，不进 catalog）。
func buildGateWithRoundsOptNoCat(cache *TokenCache, rounds int) [][2]int64 {
	return buildOptWithPrefix(cache, rounds, fixedSystemNoCat())
}

// buildOptWithPrefix 优化模式核心：auto-inject + 自动推进 + 去提醒，prefix 可自定义（含/不含 catalog）。
func buildOptWithPrefix(cache *TokenCache, rounds int, prefix []map[string]any) [][2]int64 {
	return buildOptWithPrefixAndSub(cache, rounds, prefix, false)
}

// buildOptWithPrefixTrimmedSub 同 buildOptWithPrefix，但子代理用精简历史。
func buildOptWithPrefixTrimmedSub(cache *TokenCache, rounds int, prefix []map[string]any) [][2]int64 {
	return buildOptWithPrefixAndSub(cache, rounds, prefix, true)
}

// buildOptWithPrefixAndSub 优化模式核心 + 可选精简子代理。
func buildOptWithPrefixAndSub(cache *TokenCache, rounds int, prefix []map[string]any, trimmedSub bool) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, prefix...)

	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))
	cur = append(cur, sysMsg(initInject))
	for _, p := range initScript() {
		if p.tool == "auto_skill_injection" || p.tool == "read_required" {
			continue
		}
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		if p.tool == "set_phase" {
			simPhase = p.args
			if sk, ok := phaseInjectSkills[p.args]; ok && sk != "" {
				cur = append(cur, sysMsg(sk))
			}
			continue
		}
		cur = append(cur, asstToolCall("init", p.tool, p.args), toolMsg("init", p.tool, playResult(p)))
	}
	cur = append(cur, asstText("开书完成"))
	req := append(append([]map[string]any{}, history...), cur...)
	hit, miss := cache.Step(req)
	results = append(results, [2]int64{hit, miss})
	history = append(history, cur...)

	for turn := 1; turn <= rounds; turn++ {
		cur := []map[string]any{userMsg(fmt.Sprintf("请创作第 %d 章。", turn+1))}
		cur = append(cur, sysMsg(novelState(turn)))
		plays := gateScript(turn)
		for _, p := range plays {
			if p.tool == "auto_skill_injection" || p.tool == "read_required" {
				continue
			}
			req := append(append([]map[string]any{}, history...), cur...)
			hit, miss := cache.Step(req)
			results = append(results, [2]int64{hit, miss})
			if p.tool == "set_phase" {
				simPhase = p.args
				if sk, ok := phaseInjectSkills[p.args]; ok && sk != "" {
					cur = append(cur, sysMsg(sk))
				}
				continue
			}
			cur = append(cur, asstToolCall(fmt.Sprintf("t%d", turn), p.tool, p.args))
			if p.tool == "run_subagent" {
				var subResults [][2]int64
				if trimmedSub {
					subResults = simulateSubagentTrimmed(cache, history, cur, turn)
				} else {
					subResults = simulateSubagent(cache, history, cur, turn)
				}
				results = append(results, subResults...)
			}
			cur = append(cur, toolMsg(fmt.Sprintf("t%d", turn), p.tool, playResult(p)))
		}
		cur = append(cur, asstText(finalAssistant(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		history = append(history, cur...)
	}
	return results
}
// init → prepare（一次）→ outline（N 章大纲一次出）→ write（循环 N 章）→ review（统一）→ maintain（统一）→ done。
// 与单章连续 N 轮不同：轮边界只在批次间出现（整批只有一次 prepare/review/maintain），
// 历史跨章连续累积，NS 落库收益在批次边界放大——legacy 在批次边界把整批历史重发为 miss。
func buildBatchWithRounds(mode string, cache *TokenCache, chapters int) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	for batch := 0; batch < 2; batch++ {
		cur := []map[string]any{userMsg(fmt.Sprintf("请批量创作 %d 章：先出全部大纲，再逐章写正文，全部完成后统一审稿与维护。", chapters))}
		cur = append(cur, sysMsg(novelState(batch * chapters)))
		if batch == 0 {
			cur = runPlays(cache, history, cur, filterReadRequired(initScript(), mode), &results, nil, nil, nil)
		} else {
			req := append(append([]map[string]any{}, history...), cur...)
			hit, miss := cache.Step(req)
			results = append(results, [2]int64{hit, miss})
		}

		plays := batchAsIs(chapters)
		cur = runPlays(cache, history, cur, filterReadRequired(plays, mode), &results,
			func(subCur []map[string]any) [][2]int64 {
				return simulateSubagent(cache, history, subCur, batch*chapters+chapters)
			}, nil, nil)
		cur = append(cur, asstText(fmt.Sprintf("批量创作完成：%d 章正文已写入，审稿与维护已统一完成。", chapters)))
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

// buildBatchLoop 用户定义的批量模式：单窗口内，每 perBatch 章走一次完整批量门禁
// （首批 init + 每批 prepare→outline(perBatch 章大纲)→write→review→maintain），
// 批次循环、上下文累积，直到刻度/窗口上限。章号按批偏移，不覆盖前批。
func buildBatchLoop(mode string, cache *TokenCache, perBatch, batches int) [][2]int64 {
	return buildBatchLoopCompress(mode, cache, perBatch, batches, 0, 0.7)
}

// buildBatchLoopCompress 带上下文压缩建模的批量循环（contextWindow>0 时每批开始检查）。
func buildBatchLoopCompress(mode string, cache *TokenCache, perBatch, batches, contextWindow int, threshold float64) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	if contextWindow > 0 {
		cache.EnableCompression(contextWindow, threshold, func() {
			simCompress(cache, &history, simCurrentChapter)
		})
	}

	for b := 0; b < batches; b++ {
		base := b * perBatch
		if contextWindow > 0 {
			cache.MaybeCompress()
		}
		cur := []map[string]any{userMsg(fmt.Sprintf("请批量创作 %d 章：先出全部大纲，再逐章写正文，全部完成后统一审稿与维护。", perBatch))}
		cur = append(cur, sysMsg(novelState(base)))
		if b == 0 {
			cur = runPlays(cache, history, cur, filterReadRequired(initScript(), mode), &results, nil, nil, nil)
		} else {
			req := append(append([]map[string]any{}, history...), cur...)
			hit, miss := cache.Step(req)
			results = append(results, [2]int64{hit, miss})
		}

		plays := batchAsIsBase(perBatch, base)
		cur = runPlays(cache, history, cur, filterReadRequired(plays, mode), &results,
			func(subCur []map[string]any) [][2]int64 {
				return simulateSubagent(cache, history, subCur, base+perBatch)
			}, nil, nil)
		cur = append(cur, asstText(fmt.Sprintf("批量创作完成：%d 章正文已写入，审稿与维护已统一完成。", perBatch)))
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

// ---- 混合会话（真实对话窗口） ----

// 短对话轮：用户查设定/改设定，AI 调工具并回答（每轮 2 问、3-4 次工具调用）。
// 返回 (plays 之外的 cur 增量部分, 本轮工具调用序列)。
// 与单章/批量共用一条 history：短对话穿插在创作轮之间，不独占对话窗口。
func qaPlay() []play {
	return []play{
		{tool: "get_writing_context", args: `{"current_chapter":1}`, result: longContext(1)},
		{tool: "get_characters", args: `{}`, result: `{"characters":[{"id":1,"name":"陈昊","desc":"主角","location_id":3},{"id":2,"name":"林雪","desc":"师姐","location_id":3}]}`},
	}
}

// qaRound 一轮短对话：user 查设定 → AI 查工具 → AI 回答 → user 改设定 → AI 更新 → AI 回答。
// 追加到 cur，并执行所有请求（cache.Step）。
// nsOnDemandSim=true 时模拟"NS 按需注入"（RunNSOnDemand 对照用）：非写章轮进度指纹未变，
// 历史中已有同字节 NS，跳过重复注入（新消息即使字节重复也计入 miss 尾部）。
func qaRound(cache *TokenCache, history, cur []map[string]any, results *[][2]int64, turn int) []map[string]any {
	step := func() {
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		*results = append(*results, [2]int64{hit, miss})
	}

	ns := func() string {
		if nsOnDemandSim {
			return ""
		}
		return novelState(turn)
	}

	// 第 1 问：查看设定
	cur = append(cur, userMsg(fmt.Sprintf("第 %d 轮：帮我看看现在的主角设定和世界观。", turn)))
	if s := ns(); s != "" {
		cur = append(cur, sysMsg(s))
	}
	step()
	cur = append(cur,
		asstToolCall(fmt.Sprintf("call_qa%d_p0", turn), "get_writing_context", `{"current_chapter":1}`),
		toolMsg(fmt.Sprintf("call_qa%d_p0", turn), "get_writing_context", realToolResult("get_writing_context", `{"current_chapter":1}`, longContext(1))),
	)
	step()
	cur = append(cur,
		asstToolCall(fmt.Sprintf("call_qa%d_p1", turn), "get_characters", `{}`),
		toolMsg(fmt.Sprintf("call_qa%d_p1", turn), "get_characters", realToolResult("get_characters", `{}`, `{"characters":[{"id":1,"name":"陈昊","desc":"主角","location_id":3}]}`)),
	)
	step()
	cur = append(cur, asstText("当前设定：主角陈昊，青云宗弟子；世界观为东方玄幻，灵气修炼体系。"))

	// 第 2 问：修改设定
	cur = append(cur, userMsg("把主角的兵器改成墨玉剑，性格再沉稳一点。"))
	if s := ns(); s != "" {
		cur = append(cur, sysMsg(s))
	}
	step()
	cur = append(cur,
		asstToolCall(fmt.Sprintf("call_qa%d_p2", turn), "update_item", `{"item_id":1,"name":"墨玉剑","owner_id":1}`),
		toolMsg(fmt.Sprintf("call_qa%d_p2", turn), "update_item", wrapResult("已更新")),
		asstToolCall(fmt.Sprintf("call_qa%d_p3", turn), "update_character", `{"character_id":1,"personality":"沉稳内敛"}`),
		toolMsg(fmt.Sprintf("call_qa%d_p3", turn), "update_character", wrapResult("已更新")),
	)
	step()
	cur = append(cur, asstText("已更新：主角兵器改为墨玉剑，性格改为沉稳内敛。"))
	return cur
}

// commitCur 把本轮 cur 追加进 history（now 保留 NS；legacy 剔除 NS 位置）。
// clean 模式的发送前变换由 TokenCache.transform 处理，历史与 now 相同（全文落库）。
func commitCur(mode string, history *[]map[string]any, cur []map[string]any) {
	if mode == "now" || mode == "clean" {
		*history = append(*history, cur...)
	} else {
		legacyCur := append([]map[string]any{}, cur[0])
		legacyCur = append(legacyCur, cur[2:]...)
		*history = append(*history, legacyCur...)
	}
}

// simCompress 模拟真机上下文压缩（internal/agent/compress.go Compress 对齐）：
// 1. ResetChain：前缀链重置（下轮首请求全 miss）
// 2. 重建消息：fixedSystem + 新 NS + compressionSummary reminder + 保留最近 maxSimRetain 条消息
//    （真机保留最近 15 条 user 消息及之后内容，模拟简化保留历史尾部）
// 3. 技能全文不在重建后的历史中 → 下次 set_phase 时 visibleIn 全文比对失败 → 自动重新注入全文
//    （与真机压缩后技能衰减恢复一致，无需清注入记录）
const maxSimRetain = 15

func simCompress(cache *TokenCache, history *[]map[string]any, turn int) {
	cache.ResetChain()
	cache.lastTotal = 0 // 压缩后新历史从 0 起（否则下次 MaybeCompress 误判旧阈值再触发）
	// 保留尾部最近 maxSimRetain 条消息（对齐真机 retainMessages 的 user 消息窗口近似）
	retained := []map[string]any{}
	if len(*history) > 0 {
		start := len(*history) - maxSimRetain
		if start < 0 {
			start = 0
		}
		retained = append(retained, (*history)[start:]...)
	}
	// 重建：fixedSystem + 新 NS + 摘要提醒 + 保留尾部
	newHistory := append([]map[string]any{}, fixedSystem()...)
	newHistory = append(newHistory, sysMsg(novelState(turn)))
	newHistory = append(newHistory, userMsg(compressionSummary()))
	newHistory = append(newHistory, retained...)
	*history = newHistory
}
// cleanVersion 发送前变换：把 read/read_required 的 skill 全文替换为占位符，
// 仅保留最近 retain 条完整结果（从消息序列尾部向前数）。返回新切片，不改原消息。
// 占位符替换使该条消息与上一请求的字节不同 → 前缀在"滑出窗口"时断裂一次，
// 之后连续——这是 clean 的缓存代价，收益是历史大幅缩小（O(N²)→O(N)）。
// 对齐 JetBrains observation masking：工具结果清理是主要机制。
func cleanVersion(messages []map[string]any, retain int) []map[string]any {
	// 首轮保护：历史过短（init 阶段刚读完技能）不清理，对齐真实实现 minClearableMsgs。
	if len(messages) < 20 {
		return messages
	}
	// 收集所有 read/read_required 的 tool 消息下标（从后向前保留 retain 条）
	idx := make([]int, 0)
	for i, m := range messages {
		role, _ := m["role"].(string)
		if role != "tool" {
			continue
		}
		name, _ := m["name"].(string)
		if name == "auto_skill_injection" || name == "read_required" || name == "read" {
			idx = append(idx, i)
		}
	}
	keepFrom := 0
	if len(idx) > retain {
		keepFrom = len(idx) - retain
	}
	if keepFrom == 0 {
		return messages // 全部保留，无变化
	}

	out := make([]map[string]any, len(messages))
	copy(out, messages)
	for k := 0; k < keepFrom; k++ {
		i := idx[k]
		dup := make(map[string]any, len(messages[i]))
		for key, v := range messages[i] {
			dup[key] = v
		}
		name, _ := dup["name"].(string)
		dup["content"] = "[已读技能内容已清理: " + name + "]"
		out[i] = dup
	}
	return out
}

// LayeredResult 对照结果：now（现状）vs 优化（按需注入等）。
type LayeredResult struct {
	Scenario        string `json:"scenario"`
	NowHit          int64  `json:"now_hit"`
	NowMiss         int64  `json:"now_miss"`
	LayeredHit      int64  `json:"layered_hit"`
	LayeredMiss     int64  `json:"layered_miss"`
	NowRequests     int    `json:"now_requests"`
	LayeredRequests int    `json:"layered_requests"`
}

// NowHitRate 现状命中率（%）。
func (r *LayeredResult) NowHitRate() float64 {
	if r.NowHit+r.NowMiss == 0 {
		return 0
	}
	return 100 * float64(r.NowHit) / float64(r.NowHit+r.NowMiss)
}

// LayeredHitRate 优化后命中率（%）。
func (r *LayeredResult) LayeredHitRate() float64 {
	if r.LayeredHit+r.LayeredMiss == 0 {
		return 0
	}
	return 100 * float64(r.LayeredHit) / float64(r.LayeredHit+r.LayeredMiss)
}

// MissSavePct 优化相对现状的 miss 降幅（%）。
func (r *LayeredResult) MissSavePct() float64 {
	if r.NowMiss == 0 {
		return 0
	}
	return 100 * float64(r.NowMiss-r.LayeredMiss) / float64(r.NowMiss)
}

// nsOnDemandSim 包级开关（单线程模拟器）：true 时 qaRound 跳过 NS 注入，
// 模拟"NS 按需注入"（非写章轮 NS 不变不重复注入），RunNSOnDemand 对照用。
var nsOnDemandSim bool

// RunNSOnDemand NS 按需注入对照：同一混合会话（批量 5 章 + 短对话 2），
// now = 每轮注入 NS（现状）；ondemand = 非写章轮跳过重复 NS。
// 收益来源（所有 OpenAI 兼容模型通用）：新消息即使字节与历史重复也计入 miss 尾部，
// 跳过注入则本轮只 miss 新 user 消息本身。
func RunNSOnDemand() (*LayeredResult, error) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	cNow := NewTokenCache()
	resNow := buildMixedSession("auto", cNow, 0, 2, 5)

	nsOnDemandSim = true
	defer func() { nsOnDemandSim = false }()
	cLay := NewTokenCache()
	resLay := buildMixedSession("auto", cLay, 0, 2, 5)

	r := &LayeredResult{
		Scenario:        "NS 按需注入对照（批量 5 章 + 短对话 2）",
		NowRequests:     len(resNow),
		LayeredRequests: len(resLay),
	}
	for _, p := range resNow {
		r.NowHit += p[0]
		r.NowMiss += p[1]
	}
	for _, p := range resLay {
		r.LayeredHit += p[0]
		r.LayeredMiss += p[1]
	}
	return r, nil
}

// RunSkillDedup 技能注入去重对照：模型视野（history+cur）中已有同阶段技能全文则跳过。
// 真机收益：技能注入占 miss 构成 26%（单章模式每章 5 次真切换重复注入全文）。
// 判定与真机一致：全文比对（visibleIn），压缩/清理后全文消失会自动重新注入。
func RunSkillDedup() (*LayeredResult, error) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	// 现状：每阶段切换注入技能全文（不查重）
	cNow := NewTokenCache()
	resNow := buildMixedSession("auto", cNow, 5, 0, 0)

	// 去重：模型视野中已有相同全文则跳过
	skillDedupSim = true
	defer func() { skillDedupSim = false }()
	cLay := NewTokenCache()
	resLay := buildMixedSession("auto", cLay, 5, 0, 0)

	r := &LayeredResult{
		Scenario:        "技能注入去重对照（单章 5 轮）",
		NowRequests:     len(resNow),
		LayeredRequests: len(resLay),
	}
	for _, p := range resNow {
		r.NowHit += p[0]
		r.NowMiss += p[1]
	}
	for _, p := range resLay {
		r.LayeredHit += p[0]
		r.LayeredMiss += p[1]
	}
	return r, nil
}

// WindowCostResult 上下文刻度对照结果。
type WindowCostResult struct {
	Marks      []WindowMark // 各刻度到达时的累计快照（single/batch 模式）
	StageMarks []StageMark  // 阶段打点（mixed 模式，按创作阶段边界）
	Compresses int          // 上下文压缩触发次数（contextWindow>0 时）
	FinalHit   int64
	FinalMiss  int64
	FinalOut   int64
	FinalReqs  int
	FinalTotal int64          // 最后请求的输入大小（历史到达的最终规模）
	MissByCat  map[string]int64 // miss 构成（按消息来源分类，GUI 对比表用）
	Threshold  float64        // 压缩阈值（0.7=70%×窗口触发；用户自定义值，GUI 展示用）
}

// RunWindowCost 单窗口"上下文规模刻度"打点：
// mode=single：一个窗口内每章走完整单章门禁（gateRounds 轮），上下文累积；
// mode=batch：一个窗口内每 perBatch 章走完整批量门禁（buildBatchLoop 批次循环），上下文累积。
// 历史增长过程中首次跨过 128K/256K/512K/1024K 时记录累计 hit/miss/out/请求数/章数，
// 回答"单窗口内到达各上下文大小时的成本"与"最省区间"。章数上限由调用方控制（1M 窗口内）。
func RunWindowCost(mode string, chapters int) (*WindowCostResult, error) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	simWindowThresholds = []int64{128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024}
	simCurrentChapter = 0
	c := NewTokenCache()
	c.marks = make([]WindowMark, len(simWindowThresholds))

	var res [][2]int64
	if mode == "single" {
		res = buildMixedSession("auto", c, chapters, 0, 0)
	} else {
		// 批量：每批 6 章，批次循环直到章数上限（每批 ~40-60K 历史，1M 需 ~20 批）
		perBatch := 6
		batches := chapters / perBatch
		if batches < 1 {
			batches = 1
		}
		res = buildBatchLoop("now", c, perBatch, batches)
	}

	r := &WindowCostResult{
		Marks:     c.marks,
		FinalHit:  c.hit,
		FinalMiss: c.miss,
		FinalOut:  c.output,
		FinalReqs: c.reqCount,
	}
	if len(res) > 0 {
		last := res[len(res)-1]
		r.FinalTotal = last[0] + last[1]
	}
	// 清理包级状态，避免污染后续 Run
	simWindowThresholds = nil
	simCurrentChapter = 0
	return r, nil
}

// RunWindowMode 单窗口"写书成本 + 上下文刻度"模拟（GUI 设置页用，now 协议）：
//   single：一个窗口内每章走完整单章门禁逐章累积（~40K/章，1M 窗口约 25 章）
//   batch：一个窗口内每 perBatch 章走完整批量门禁批次循环（~50K/批，1M 约 20 批）
//   mixed：混合会话（开书 → 短对话 → 单章 → 短对话 → 批量 → 短对话），
//          批量部分按 batchRounds 批次循环（章号偏移，不覆盖前批），
//          按用户配置的轮数跑（窗口大小由输入决定，刻度能打到哪算哪）
// contextWindow>0 时启用上下文压缩建模（对齐真机 agent.go:434-455：每轮开始
// 检查 token 预算，超 threshold×窗口触发压缩重建——链重置 + fixedSystem/NS/摘要/保留尾部）。
// threshold<=0 时默认 0.7（真机默认压缩阈值；用户可自定义，GUI 从设置传入）。
// 历史增长过程中首次跨过 128K/256K/512K/1024K 时记录累计 hit/miss/out/请求数/章数。
// 返回：刻度快照 + 最终汇总（成本由调用方按设置页价格估算）。
func RunWindowMode(mode string, gateRounds, shortQARounds, batchChapters, batchRounds, contextWindow int, threshold float64) (*WindowCostResult, error) {
	if threshold <= 0 {
		threshold = 0.7
	}
	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()
	// 门禁拦截建模：默认按真机 set_phase 失败率 25% 注入拦截（require 未满足），
	// 失败消息全 miss 且带动重试——让成本口径贴近真机（真机日志 28 次 set_phase 7 次失败）。
	simGateBlockRate = 0.25
	defer func() { simGateBlockRate = 0 }()

	simWindowThresholds = []int64{128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024}
	simCurrentChapter = 0
	c := NewTokenCache()
	c.SetMissCat(missCatOf)
	c.marks = make([]WindowMark, len(simWindowThresholds))
	// 预填刻度值：未到达的刻度也要有名字（前端不再显示 0K）
	for i, th := range simWindowThresholds {
		c.marks[i].Threshold = th
	}
	defer func() { simWindowThresholds = nil; simCurrentChapter = 0 }()

	var res [][2]int64
	switch mode {
	case "single":
		if gateRounds <= 0 {
			gateRounds = 26 // 1M 窗口约 25 章
		}
		res = buildMixedSessionCompress("auto", c, gateRounds, 0, 0, 1, contextWindow, threshold)
	case "batch":
		if batchChapters <= 0 {
			batchChapters = 120
		}
		perBatch := 6
		batches := batchChapters / perBatch
		if batches < 1 {
			batches = 1
		}
		res = buildBatchLoopCompress("now", c, perBatch, batches, contextWindow, threshold)
	default: // mixed
		if gateRounds <= 0 && shortQARounds <= 0 && batchChapters <= 0 {
			gateRounds, shortQARounds, batchChapters = 3, 5, 5
		}
		if batchRounds < 1 {
			batchRounds = 1
		}
		// 阶段打点（混合模式专用）：按创作阶段边界记录，替代阈值刻度
		c.stageMarks = make([]StageMark, 0, 12)
		res = buildMixedSessionCompress("auto", c, gateRounds, shortQARounds, batchChapters, batchRounds, contextWindow, threshold)
	}
	_ = res

	r := &WindowCostResult{
		Marks:      c.marks,
		StageMarks: c.stageMarks,
		FinalHit:   c.hit,
		FinalMiss:  c.miss,
		FinalOut:   c.output,
		FinalReqs:  c.reqCount,
		Compresses: c.compressCount,
		MissByCat:  c.MissByCat,
		Threshold:  threshold,
	}
	// 终点历史用峰值：压缩重建后 lastTotal 缩回骨架，峰值才反映窗口真实增长上限
	r.FinalTotal = c.peakTotal
	if r.FinalTotal <= 0 {
		r.FinalTotal = c.lastTotal
	}
	return r, nil
}

// buildMixedSession 模拟一个真实对话窗口：开书 → 短对话（查/改设定）→ 单章创作
// → 短对话 → 批量创作 → 短对话……全部发生在同一条历史里，短对话穿插在创作轮之间。
// 这比"短对话独立场景"更贴近真实使用：用户不会开一个窗口纯聊天，再开一个窗口纯创作。
func buildMixedSession(mode string, cache *TokenCache, gateRounds, qaRounds, batchChapters int) [][2]int64 {
	return buildMixedSessionCore(mode, cache, gateRounds, qaRounds, batchChapters, 1, 0, 0.7)
}

// buildMixedSessionLoop 同 buildMixedSession，但批量部分批次循环 batchRounds 次
// （每批 batchChapters 章，章号按批偏移，不覆盖前批）——混合会话也能跑到 1M 窗口。
func buildMixedSessionLoop(mode string, cache *TokenCache, gateRounds, qaRounds, batchChapters, batchRounds int) [][2]int64 {
	return buildMixedSessionCore(mode, cache, gateRounds, qaRounds, batchChapters, batchRounds, 0, 0.7)
}

// buildMixedSessionCompress 带上下文压缩建模的混合会话（contextWindow>0 时启用）。
func buildMixedSessionCompress(mode string, cache *TokenCache, gateRounds, qaRounds, batchChapters, batchRounds, contextWindow int, threshold float64) [][2]int64 {
	return buildMixedSessionCore(mode, cache, gateRounds, qaRounds, batchChapters, batchRounds, contextWindow, threshold)
}

func buildMixedSessionCore(mode string, cache *TokenCache, gateRounds, qaRounds, batchChapters, batchRounds, contextWindow int, threshold float64) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	// 上下文压缩建模：每轮开始时检查 token 预算（对齐真机 agent.go:434-455），
	// 超 threshold×窗口触发压缩重建（ResetChain + 重建消息序列）
	if contextWindow > 0 {
		cache.EnableCompression(contextWindow, threshold, func() {
			simCompress(cache, &history, simCurrentChapter)
		})
	}
	// 压缩检查辅助：在每轮请求前调用（本轮首个请求全 miss 或复用重建链）
	checkCompress := func() {
		if cache.MaybeCompress() {
			// 压缩后首个请求全 miss（链已重置），由后续 Step 自然体现
		}
	}

	// auto 模式：init 技能直接注入
	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))
	if mode == "auto" {
		cur = append(cur, sysMsg(initInject))
	}
	cur = runPlays(cache, history, cur, filterReadRequired(initScript(), mode), &results,
		nil, nil, func(c []map[string]any, phase string) []map[string]any {
			return injectPhaseOn(mode, history, c, phase)
		})
	cur = append(cur, asstText("开书完成：世界观、角色、总纲、第一卷弧线已建立，进入第一章创作。"))
	req := append(append([]map[string]any{}, history...), cur...)
	hit, miss := cache.Step(req)
	results = append(results, [2]int64{hit, miss})
	commitCur(mode, &history, cur)
	cache.RecordStage("开书完成")

	// 短对话配额：穿插在各创作轮之间（开头 1 轮 + 每轮单章后 1 轮 + 批量前/后）
	qaBudget := qaRounds
	qaNo := 0
	doQA := func(turn int) bool {
		if qaBudget <= 0 {
			return false
		}
		checkCompress()
		cur := qaRound(cache, history, []map[string]any{}, &results, turn)
		commitCur(mode, &history, cur)
		qaBudget--
		qaNo++
		cache.RecordStage(fmt.Sprintf("短对话 %d", qaNo))
		return true
	}

	// 开头短对话（查设定）
	if qaRounds > 0 {
		doQA(0)
	}

	// 单章创作轮（每轮之间穿插短对话）
	for turn := 1; turn <= gateRounds; turn++ {
		checkCompress()
		cur := []map[string]any{userMsg(fmt.Sprintf("请创作第 %d 章，继续推进剧情。", turn+1))}
		cur = append(cur, sysMsg(novelState(turn)))
		cur = runPlays(cache, history, cur, filterReadRequired(gateScript(turn), mode), &results,
			func(subCur []map[string]any) [][2]int64 {
				return simulateSubagent(cache, history, subCur, turn)
			},
			nil, func(c []map[string]any, phase string) []map[string]any {
				return injectPhaseOn(mode, history, c, phase)
			})
		cur = append(cur, asstText(finalAssistant(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		commitCur(mode, &history, cur)
		cache.RecordStage(fmt.Sprintf("单章 %d", turn))

		// 单章完成后穿插短对话（查看/微调）
		doQA(turn)
	}

	// 批量创作（短对话过渡 → 批量批次循环 → 短对话收尾）。
	// 章号连续性：单章轮 gateScript(turn) 写第 turn+1 章（2..gateRounds+1），
	// 批量每批从 base+1 起写——base 必须接在单章轮之后（base0 = gateRounds），
	// 否则批量又从第 1 章重写，章号重叠、刻度打点全挤在单章轮（混合模式"看起来"没批量）。
	if batchChapters > 0 {
		if batchRounds < 1 {
			batchRounds = 1
		}
		for r := 0; r < batchRounds; r++ {
			// 单章轮 gateScript(turn) 写第 turn+1 章（2..gateRounds+1），
			// 批量接在后面：base+1 = gateRounds+2 起，章号连续不重叠
			base := gateRounds + 1 + r*batchChapters
			checkCompress()
			doQA(gateRounds + r + 1)
			cur := []map[string]any{userMsg(fmt.Sprintf("请批量创作 %d 章：先出全部大纲，再逐章写正文，全部完成后统一审稿与维护。", batchChapters))}
			cur = append(cur, sysMsg(novelState(base)))
			cur = runPlays(cache, history, cur, filterReadRequired(batchAsIsBase(batchChapters, base), mode), &results,
				func(subCur []map[string]any) [][2]int64 {
					return simulateSubagent(cache, history, subCur, base+batchChapters)
				},
				nil, func(c []map[string]any, phase string) []map[string]any {
					return injectPhaseOn(mode, history, c, phase)
				})
			cur = append(cur, asstText(fmt.Sprintf("批量创作完成：%d 章正文已写入，审稿与维护已统一完成。", batchChapters)))
			req := append(append([]map[string]any{}, history...), cur...)
			hit, miss := cache.Step(req)
			results = append(results, [2]int64{hit, miss})
			commitCur(mode, &history, cur)
			cache.RecordStage(fmt.Sprintf("批量轮 %d", r+1))
		}
		doQA(gateRounds + batchRounds + 1)
	}

	return results
}
