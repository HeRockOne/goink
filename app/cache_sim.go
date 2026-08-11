package app

import (
	"fmt"
	"sync"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel/internal/cacheprobe"
	"novel/internal/config"
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
	// 上下文窗口刻度（单窗口成本曲线）
	Marks          []WindowSimMark `json:"marks"`
	FinalTotal     int64           `json:"final_total"` // 终点历史大小（token）
	FinalReqs      int             `json:"final_reqs"`
	FinalCost      float64         `json:"final_cost"`
	FinalHitRate   float64         `json:"final_hit_rate"`
	BestInterval   string          `json:"best_interval"` // 最省区间标签（如 "256K→512K"）
	BestPerChapter float64         `json:"best_per_chapter"`
}

// WindowSimMark 窗口刻度快照（单窗口成本曲线打点）。
type WindowSimMark struct {
	Threshold        int64   `json:"threshold"` // 刻度（token）：128K/256K/512K/1024K
	Reached          bool    `json:"reached"`
	Hit              int64   `json:"hit"`
	Miss             int64   `json:"miss"`
	Out              int64   `json:"out"`
	Requests         int     `json:"requests"`
	Chapter          int     `json:"chapter"` // 到达时写到的章节号
	Cost             float64 `json:"cost"`    // 到达时累计成本（元）
	HitRate          float64 `json:"hit_rate"`
	IntervalChapters int     `json:"interval_chapters"` // 距上一刻度的章节数
	IntervalCost     float64 `json:"interval_cost"`     // 区间增量成本
	IntervalPerCh    float64 `json:"interval_per_chapter"`
}

// StartCacheSimulation 异步启动写书成本模拟（用户手动触发）。
// 立即返回，模拟在后台 goroutine 运行，完成后推送事件 cachesim:done。
// mode: single=单章逐章累积 / batch=批量批次循环 / mixed=混合会话（批量可多轮）。
func (a *App) StartCacheSimulation(mode string, gateRounds int, shortQARounds int, batchChapters int, batchRounds int) error {
	go func() {
		simMu.Lock()
		defer simMu.Unlock()
		res := a.runCacheSimulationSync(mode, gateRounds, shortQARounds, batchChapters, batchRounds)
		wails.EventsEmit(a.ctx, "cachesim:done", res)
	}()
	return nil
}

// runCacheSimulationSync 同步执行模拟并附带价格估算与窗口刻度（供 StartCacheSimulation 后台调用）。
func (a *App) runCacheSimulationSync(mode string, gateRounds int, shortQARounds int, batchChapters int, batchRounds int) *CacheSimResult {
	raw, err := cacheprobe.RunWindowMode(mode, gateRounds, shortQARounds, batchChapters, batchRounds)
	if err != nil {
		return &CacheSimResult{Cost: -1} // 失败标记
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

	// 窗口刻度：打点快照 + 区间每章成本 + 最省区间
	res.FinalTotal = raw.FinalTotal
	res.FinalReqs = raw.FinalReqs
	res.FinalCost = res.Cost
	res.FinalHitRate = res.HitRate
	prevCost := 0.0
	prevCh := 0
	prevTh := int64(0)
	prevReached := false
	bestInterval := ""
	bestPerCh := 1e18
	for _, mk := range raw.Marks {
		m := WindowSimMark{
			Threshold: mk.Threshold,
			Reached:   mk.Reached,
			Hit:       mk.Hit,
			Miss:      mk.Miss,
			Out:       mk.Out,
			Requests:  mk.Requests,
			Chapter:   mk.Chapter,
			Cost:      costOf(mk.Hit, mk.Miss, mk.Out, cachePrice, inputPrice, outputPrice),
			HitRate:   hitRate(mk.Hit, mk.Miss),
		}
		if prevReached && mk.Reached {
			m.IntervalChapters = mk.Chapter - prevCh
			m.IntervalCost = m.Cost - prevCost
			if m.IntervalChapters > 0 {
				m.IntervalPerCh = m.IntervalCost / float64(m.IntervalChapters)
			}
			if m.IntervalPerCh > 0 && m.IntervalPerCh < bestPerCh {
				bestPerCh = m.IntervalPerCh
				bestInterval = fmt.Sprintf("%dK→%dK", prevTh/1024, mk.Threshold/1024)
			}
		}
		prevCost = m.Cost
		prevCh = mk.Chapter
		prevTh = mk.Threshold
		prevReached = mk.Reached
		res.Marks = append(res.Marks, m)
	}
	res.BestInterval = bestInterval
	res.BestPerChapter = bestPerCh
	return res
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
