package app

import (
	"fmt"
	"strings"
	"sync"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel/internal/cacheprobe"
	"novel/internal/config"
	"novel/internal/llm"
)

// simMu 串行化模拟器调用（cacheprobe 包级状态单线程设计，并行会串扰）。
var simMu sync.Mutex

// CacheSimResult 写书成本模拟结果（前端展示用）。
// 一个模拟 = 一个真实对话窗口（模式可配：单章 / 批量 / 混合），
// 除成本对照外，附带上下文窗口刻度快照（历史增长到 128K/256K/512K/1024K
// 时的累计成本与区间每章成本，找最省区间）。
type CacheSimResult struct {
	Mode  string `json:"mode"`  // single / batch / mixed
	Label string `json:"label"` // 展示名
	// 汇总（now 协议）
	TotalHit  int64   `json:"total_hit"`
	TotalMiss int64   `json:"total_miss"`
	TotalOut  int64   `json:"total_out"`
	HitRate   float64 `json:"hit_rate"`
	Cost      float64 `json:"cost"`
	// 终点汇总（mixed 阶段表终点行用）
	FinalTotal   int64   `json:"final_total"`
	FinalReqs    int     `json:"final_reqs"`
	FinalCost    float64 `json:"final_cost"`
	FinalHitRate float64 `json:"final_hit_rate"`
	// 阶段打点（mixed 模式：开书/短对话/单章轮/批量轮每阶段结束的成本快照）
	Stages []CacheSimStage `json:"stages"`
	// 上下文压缩触发次数（真机 threshold×窗口建模；>0 说明长窗口跑到了压缩点）
	Compresses int `json:"compresses"`
	// 压缩阈值（0.7=70%×窗口触发；读设置自定义值，前端提示用）
	CompressThreshold float64 `json:"compress_threshold"`
	// miss 构成（按消息来源分类：thinking/技能注入/工具结果/正文等，对比表用）
	MissByCat map[string]int64 `json:"miss_by_cat,omitempty"`
}

// SimScenarioReq 场景对比请求：一个可自定义场景的参数（前端场景编辑器）。
type SimScenarioReq struct {
	Name           string `json:"name"`
	GateRounds     int    `json:"gate_rounds"`     // 单章轮数（single/mixed 用）
	ShortQARounds  int    `json:"short_qa_rounds"` // 短对话轮数（mixed 用）
	BatchChapters  int    `json:"batch_chapters"`  // 批量章数（batch/mixed 用）
	BatchRounds    int    `json:"batch_rounds"`    // 批量轮数（mixed 用）
	ContextWindow  int    `json:"context_window"`  // 模拟窗口 token，0=按设置选中模型
}

// CacheSimStage 阶段打点快照（mixed 模式，按创作阶段边界记录）。
type CacheSimStage struct {
	Stage      string  `json:"stage"` // "开书完成" / "短对话 N" / "单章 N" / "批量轮 N"
	Chapter    int     `json:"chapter"`
	Total      int64   `json:"total"` // 该阶段结束时的历史大小
	Requests   int     `json:"requests"`
	Cost       float64 `json:"cost"` // 累计成本（元）
	HitRate    float64 `json:"hit_rate"`
	IntervalCost  float64 `json:"interval_cost"`  // 距上一阶段的增量成本
	IntervalCh    int     `json:"interval_chapters"` // 距上一阶段的章数增量
	IntervalPerCh float64 `json:"interval_per_chapter"`
}

// StartCacheSimulation 异步启动写书成本模拟（用户手动触发）。
// 立即返回，模拟在后台 goroutine 运行，完成后推送事件 cachesim:done。
// mode: single=单章逐章累积 / batch=批量批次循环 / mixed=混合会话（批量可多轮）。
// contextWindow: 模拟的模型上下文窗口（token），<=0 时按设置选中模型推断（默认 1M）。
func (a *App) StartCacheSimulation(mode string, gateRounds int, shortQARounds int, batchChapters int, batchRounds int, contextWindow int) error {
	go func() {
		simMu.Lock()
		defer simMu.Unlock()
		res := a.runCacheSimulationSync(mode, gateRounds, shortQARounds, batchChapters, batchRounds, contextWindow)
		wails.EventsEmit(a.ctx, "cachesim:done", res)
	}()
	return nil
}

// StartCacheSimScenarios 批量场景对比：一次跑多个可自定义场景，完成后推送事件 cachesim:batch-done。
// 场景类型按参数推断：批量章数>0 且无单章轮数 → batch；单章轮数>0 且批量章数=0 → single；都有 → mixed。
func (a *App) StartCacheSimScenarios(scenarios []SimScenarioReq) error {
	go func() {
		simMu.Lock()
		defer simMu.Unlock()
		results := make([]CacheSimResult, 0, len(scenarios))
		for _, sc := range scenarios {
			mode := "mixed"
			switch {
			case sc.BatchChapters > 0 && sc.GateRounds == 0:
				mode = "batch"
			case sc.GateRounds > 0 && sc.BatchChapters == 0:
				mode = "single"
			}
			if sc.BatchRounds < 1 {
				sc.BatchRounds = 1
			}
			r := a.runCacheSimulationSync(mode, sc.GateRounds, sc.ShortQARounds, sc.BatchChapters, sc.BatchRounds, sc.ContextWindow)
			if sc.Name != "" {
				r.Label = sc.Name
			}
			results = append(results, *r)
		}
		wails.EventsEmit(a.ctx, "cachesim:batch-done", results)
	}()
	return nil
}

// runCacheSimulationSync 同步执行模拟并附带价格估算与窗口刻度（供 StartCacheSimulation 后台调用）。
func (a *App) runCacheSimulationSync(mode string, gateRounds int, shortQARounds int, batchChapters int, batchRounds int, contextWindow int) *CacheSimResult {
	// 上下文窗口：前端传入 >0 用传入值；否则按设置选中模型推断
	// （SelectedModelKey 格式 provider/modelID，解析后匹配内置定义），
	// 仍匹配不到默认 1M（DeepSeek）。
	compressThreshold := 0.7
	if contextWindow <= 0 {
		contextWindow = 1_000_000
		if s, err := config.LoadSettings(a.db); err == nil && s.SelectedModelKey != "" {
			if w := modelContextWindow(s.SelectedModelKey); w > 0 {
				contextWindow = w
			}
		}
	}
	// 压缩阈值：读设置自定义值（对齐真机压缩触发口径），默认 0.7
	simHitAdjust := 1.0
	if s, err := config.LoadSettings(a.db); err == nil {
		if s.CompressionThreshold > 0 {
			compressThreshold = s.CompressionThreshold
		}
		// 命中率校准：真机命中率（89-93%）低于模拟器（96-97%），按系数下调（1=不校准）
		if s.SimHitRateAdjust > 0 && s.SimHitRateAdjust <= 1 {
			simHitAdjust = s.SimHitRateAdjust
		}
	}
	raw, err := cacheprobe.RunWindowMode(mode, gateRounds, shortQARounds, batchChapters, batchRounds, contextWindow, compressThreshold)
	if err != nil {
		a.Logger().Error("cachesim failed", "mode", mode, "window", contextWindow, "err", err.Error())
		return &CacheSimResult{Cost: -1} // 失败标记
	}
	// 校准命中率：输入总量不变，把部分 hit 转为 miss（成本随 miss 比例上升）
	if simHitAdjust < 1 {
		applyHitAdjust(raw, simHitAdjust)
	}

	// 价格：从设置读（ContextRing 同源），默认 DeepSeek 价（0.02/1/2，元/百万 token）
	inputPrice, outputPrice, cachePrice := 1.0, 2.0, 0.02
	if s, err := config.LoadSettings(a.db); err == nil {
		if s.PriceInput > 0 {
			inputPrice = s.PriceInput
		}
		if s.PriceOutput > 0 {
			outputPrice = s.PriceOutput
		}
		if s.CachePrice > 0 {
			cachePrice = s.CachePrice
		}
	}

	label := "混合会话"
	switch mode {
	case "single":
		label = fmt.Sprintf("单章模式（%d 章逐章完整门禁）", gateRounds)
	case "batch":
		label = fmt.Sprintf("批量模式（%d 章，每批 6 章批次循环）", batchChapters)
	default:
		label = fmt.Sprintf("混合会话（单章 %d 轮 · 短对话 %d 轮 · 批量 %d 章 × %d 轮）", gateRounds, shortQARounds, batchChapters, batchRounds)
	}

	res := &CacheSimResult{
		Mode:  mode,
		Label: label,
	}
	res.TotalHit = raw.FinalHit
	res.TotalMiss = raw.FinalMiss
	res.TotalOut = raw.FinalOut
	res.HitRate = hitRate(raw.FinalHit, raw.FinalMiss)
	res.Cost = costOf(raw.FinalHit, raw.FinalMiss, raw.FinalOut, cachePrice, inputPrice, outputPrice)
	res.Compresses = raw.Compresses
	res.CompressThreshold = raw.Threshold
	res.MissByCat = raw.MissByCat

	// 终点汇总（mixed 阶段表终点行用）
	res.FinalTotal = raw.FinalTotal
	res.FinalReqs = raw.FinalReqs
	res.FinalCost = res.Cost
	res.FinalHitRate = res.HitRate

	// 阶段打点（mixed 模式）：每阶段累计成本 + 区间增量 + 每章成本
	if mode == "mixed" {
		prevCost := 0.0
		prevCh := 0
		for _, sm := range raw.StageMarks {
			s := CacheSimStage{
				Stage:    sm.Stage,
				Chapter:  sm.Chapter,
				Total:    sm.Total,
				Requests: sm.Requests,
				Cost:     costOf(sm.Hit, sm.Miss, sm.Out, cachePrice, inputPrice, outputPrice),
				HitRate:  hitRate(sm.Hit, sm.Miss),
			}
			s.IntervalCost = s.Cost - prevCost
			s.IntervalCh = s.Chapter - prevCh
			if s.IntervalCh > 0 {
				s.IntervalPerCh = s.IntervalCost / float64(s.IntervalCh)
			}
			prevCost = s.Cost
			prevCh = s.Chapter
			res.Stages = append(res.Stages, s)
		}
	}

	// 模拟结果写日志（goink.log），供排查 token/压缩/成本问题
	a.Logger().Info("cachesim done",
		"mode", mode,
		"window", contextWindow,
		"label", res.Label,
		"hit_rate", res.HitRate,
		"cost", res.Cost,
		"total_hit", res.TotalHit,
		"total_miss", res.TotalMiss,
		"total_out", res.TotalOut,
		"compresses", res.Compresses,
	)
	return res
}

// modelContextWindow 按模型 key 查内置定义的上下文窗口（token）。
// SelectedModelKey 格式为 "provider/modelID"（如 "deepseek/deepseek-v4-flash"），
// 解析出 modelID 后匹配 llm.Builtin 各 provider 的模型表；未找到返回 0。
func modelContextWindow(modelKey string) int {
	modelID := modelKey
	if idx := strings.IndexByte(modelKey, '/'); idx > 0 {
		modelID = modelKey[idx+1:]
	}
	for _, p := range llm.Builtin {
		for _, m := range p.Models {
			if m.ID == modelID {
				return m.ContextWindow
			}
		}
	}
	return 0
}

// hitRate 计算命中率（0-100）。
func hitRate(hit, miss int64) float64 {
	if hit+miss == 0 {
		return 0
	}
	return 100 * float64(hit) / float64(hit+miss)
}

// costOf 按价格计算总成本（元）。价格单位：元/百万 token。
func costOf(hit, miss, out int64, cachePrice, inputPrice, outputPrice float64) float64 {
	hitCost := float64(hit) * cachePrice / 1_000_000
	missCost := float64(miss) * inputPrice / 1_000_000
	outCost := float64(out) * outputPrice / 1_000_000
	return hitCost + missCost + outCost
}

// applyHitAdjust 按校准系数下调命中率（真机 89-93% vs 模拟 96-97%）：
// 输入总量不变，把部分 hit 转成 miss（按各快照的当前命中率等比下调），
// 成本随 miss 比例自然上升。MissByCat 按 miss 增幅等比缩放保持分类占比。
func applyHitAdjust(raw *cacheprobe.WindowCostResult, adjust float64) {
	total := raw.FinalHit + raw.FinalMiss
	if total <= 0 {
		return
	}
	oldMiss := raw.FinalMiss
	rate := float64(raw.FinalHit) / float64(total)
	newMiss := int64(float64(total) * (1 - rate*adjust))
	if newMiss > total {
		newMiss = total
	}
	raw.FinalHit = total - newMiss
	raw.FinalMiss = newMiss
	missScale := 1.0
	if oldMiss > 0 {
		missScale = float64(newMiss) / float64(oldMiss)
	}
	if raw.MissByCat != nil {
		for k, v := range raw.MissByCat {
			raw.MissByCat[k] = int64(float64(v) * missScale)
		}
		// 四舍五入误差补到第一个分类，保证分类总和 = newMiss
		sum := int64(0)
		for _, v := range raw.MissByCat {
			sum += v
		}
		for k := range raw.MissByCat {
			raw.MissByCat[k] += newMiss - sum
			break
		}
	}
	adjustPair := func(hit, miss int64) (int64, int64) {
		t := hit + miss
		if t <= 0 {
			return hit, miss
		}
		r := float64(hit) / float64(t)
		nm := int64(float64(t) * (1 - r*adjust))
		if nm > t {
			nm = t
		}
		return t - nm, nm
	}
	for i := range raw.StageMarks {
		raw.StageMarks[i].Hit, raw.StageMarks[i].Miss = adjustPair(raw.StageMarks[i].Hit, raw.StageMarks[i].Miss)
	}
}
