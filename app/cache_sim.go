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
// 一个模拟 = 一个真实对话窗口：短对话与单章/批量创作交替在一条历史里。
// 除了三方协议（now/legacy/clean）对照，还附带上下文窗口刻度快照
// （历史增长到 128K/256K/512K/1024K 时的累计成本，找最省区间）。
type CacheSimResult struct {
	Scenarios []CacheSimScenario `json:"scenarios"`
	// 汇总
	TotalNowHit     int64 `json:"total_now_hit"`
	TotalNowMiss    int64 `json:"total_now_miss"`
	TotalLegacyHit  int64 `json:"total_legacy_hit"`
	TotalLegacyMiss int64 `json:"total_legacy_miss"`
	// LLM 输出 token（assistant 消息字节，含 reasoning_content）
	TotalNowOutput    int64 `json:"total_now_output"`
	TotalLegacyOutput int64 `json:"total_legacy_output"`
	// 成本估算（按设置页配置的价格，元）
	NowCost       float64 `json:"now_cost"`
	LegacyCost    float64 `json:"legacy_cost"`
	NowHitRate    float64 `json:"now_hit_rate"`
	LegacyHitRate float64 `json:"legacy_hit_rate"`
	MissSavePct   float64 `json:"miss_save_pct"`
	// 上下文窗口刻度（now 协议，单窗口成本曲线）
	Marks          []WindowSimMark `json:"marks"`
	FinalTotal     int64           `json:"final_total"` // 终点历史大小（token）
	FinalReqs      int             `json:"final_reqs"`
	FinalCost      float64         `json:"final_cost"`
	FinalHitRate   float64         `json:"final_hit_rate"`
	BestInterval   string          `json:"best_interval"` // 最省区间标签（如 "256K→512K"）
	BestPerChapter float64         `json:"best_per_chapter"`
}

// CacheSimScenario 单场景结果。
type CacheSimScenario struct {
	Name          string  `json:"name"`
	NowHit        int64   `json:"now_hit"`
	NowMiss       int64   `json:"now_miss"`
	LegacyHit     int64   `json:"legacy_hit"`
	LegacyMiss    int64   `json:"legacy_miss"`
	NowHitRate    float64 `json:"now_hit_rate"`
	LegacyHitRate float64 `json:"legacy_hit_rate"`
	MissSavePct   float64 `json:"miss_save_pct"`
	// LLM 输出 token（assistant 消息字节，含 reasoning_content）
	NowOutput    int64 `json:"now_output"`
	LegacyOutput int64 `json:"legacy_output"`
	// 该场景的估算费用（元，按设置页价格）
	NowCost    float64 `json:"now_cost"`
	LegacyCost float64 `json:"legacy_cost"`
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
// 立即返回，模拟在后台 goroutine 运行（耗时 1-6 分钟），完成后推送事件 cachesim:done。
// gateRounds: 单章门禁创作轮数；shortQARounds: 短对话穿插轮数（0=不穿插）；
// batchChapters: 批量创作章数（0=不跑批量场景）。三者可自由组合——真实窗口
// 可能只跑单章、只跑批量、或混合。
func (a *App) StartCacheSimulation(gateRounds int, shortQARounds int, batchChapters int) error {
	go func() {
		simMu.Lock()
		defer simMu.Unlock()
		res := a.runCacheSimulationSync(gateRounds, shortQARounds, batchChapters)
		wails.EventsEmit(a.ctx, "cachesim:done", res)
	}()
	return nil
}

// runCacheSimulationSync 同步执行模拟并附带价格估算与窗口刻度（供 StartCacheSimulation 后台调用）。
func (a *App) runCacheSimulationSync(gateRounds int, shortQARounds int, batchChapters int) *CacheSimResult {
	raw, err := cacheprobe.Run(gateRounds, shortQARounds, batchChapters, 0) // clean 保留窗口默认 0（全清理）
	if err != nil {
		return &CacheSimResult{NowCost: -1, LegacyCost: -1} // 失败标记
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

	res := &CacheSimResult{}
	toScenario := func(name string, nH, nM, lH, lM int64, nOut, lOut int64) CacheSimScenario {
		return CacheSimScenario{
			Name:          name,
			NowHit:        nH, NowMiss: nM,
			LegacyHit:     lH, LegacyMiss: lM,
			NowHitRate:    hitRate(nH, nM),
			LegacyHitRate: hitRate(lH, lM),
			MissSavePct:   missSave(nM, lM),
			NowOutput:     nOut, LegacyOutput: lOut,
			NowCost:       costOf(nH, nM, nOut, cachePrice, inputPrice, outputPrice),
			LegacyCost:    costOf(lH, lM, lOut, cachePrice, inputPrice, outputPrice),
		}
	}
	for _, s := range raw.Scenarios {
		res.Scenarios = append(res.Scenarios, toScenario(s.Name, s.NowHit, s.NowMiss, s.LegacyHit, s.LegacyMiss, s.NowOutput, s.LegacyOutput))
	}
	res.TotalNowHit = raw.TotalNowHit
	res.TotalNowMiss = raw.TotalNowMiss
	res.TotalLegacyHit = raw.TotalLegacyHit
	res.TotalLegacyMiss = raw.TotalLegacyMiss
	res.TotalNowOutput = raw.TotalNowOutput
	res.TotalLegacyOutput = raw.TotalLegacyOutput
	res.NowHitRate = hitRate(raw.TotalNowHit, raw.TotalNowMiss)
	res.LegacyHitRate = hitRate(raw.TotalLegacyHit, raw.TotalLegacyMiss)
	res.MissSavePct = missSave(raw.TotalNowMiss, raw.TotalLegacyMiss)

	// 成本估算：hit×cache价 + miss×input价 + 输出（模拟器累计的 assistant 消息字节，含正文/思考）
	res.LegacyCost = costOf(res.TotalLegacyHit, res.TotalLegacyMiss, raw.TotalLegacyOutput, cachePrice, inputPrice, outputPrice)
	res.NowCost = costOf(res.TotalNowHit, res.TotalNowMiss, raw.TotalNowOutput, cachePrice, inputPrice, outputPrice)

	// 窗口刻度：打点快照 + 区间每章成本 + 最省区间
	res.FinalTotal = raw.FinalTotal
	prevCost := 0.0
	prevCh := 0
	prevTh := int64(0)
	prevReached := false
	bestInterval := ""
	bestPerCh := 1e18
	for _, mk := range raw.NowMarks {
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
	res.FinalReqs = raw.FinalReqs
	res.FinalCost = res.NowCost
	res.FinalHitRate = res.NowHitRate
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

// missSave 计算 miss 降幅百分比（legacy 为基准）。
func missSave(nowMiss, legacyMiss int64) float64 {
	if legacyMiss <= 0 {
		return 0
	}
	return 100 * float64(legacyMiss-nowMiss) / float64(legacyMiss)
}

// costOf 按价格计算总成本（元）。价格单位：元/百万 token。
func costOf(hit, miss, out int64, cachePrice, inputPrice, outputPrice float64) float64 {
	hitCost := float64(hit) * cachePrice / 1_000_000
	missCost := float64(miss) * inputPrice / 1_000_000
	outCost := float64(out) * outputPrice / 1_000_000
	return hitCost + missCost + outCost
}
