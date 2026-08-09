package app

import (
	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel/internal/cacheprobe"
	"novel/internal/config"
)

// CacheSimResult 缓存模拟结果（前端展示用）。
type CacheSimResult struct {
	Scenarios []CacheSimScenario `json:"scenarios"`
	// 汇总
	TotalNowHit    int64   `json:"total_now_hit"`
	TotalNowMiss   int64   `json:"total_now_miss"`
	TotalLegacyHit int64   `json:"total_legacy_hit"`
	TotalLegacyMiss int64  `json:"total_legacy_miss"`
	// LLM 输出 token（assistant 消息字节，含 reasoning_content）
	TotalNowOutput    int64 `json:"total_now_output"`
	TotalLegacyOutput int64 `json:"total_legacy_output"`
	// 成本估算（按设置页配置的价格，元）
	NowCost       float64 `json:"now_cost"`
	LegacyCost    float64 `json:"legacy_cost"`
	NowHitRate    float64 `json:"now_hit_rate"`
	LegacyHitRate float64 `json:"legacy_hit_rate"`
	MissSavePct   float64 `json:"miss_save_pct"`
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

// StartCacheSimulation 异步启动缓存命中模拟（用户手动触发）。
// 立即返回，模拟在后台 goroutine 运行（耗时 1-6 分钟），完成后推送事件 cachesim:done。
// gateRounds: 单章门禁创作轮数；shortQARounds: 短对话穿插轮数（0=不穿插）；
// batchChapters: 批量创作章数（0=不跑批量场景）。
func (a *App) StartCacheSimulation(gateRounds int, shortQARounds int, batchChapters int) error {
	go func() {
		res := a.runCacheSimulationSync(gateRounds, shortQARounds, batchChapters)
		wails.EventsEmit(a.ctx, "cachesim:done", res)
	}()
	return nil
}

// runCacheSimulationSync 同步执行模拟并附带价格估算（供 StartCacheSimulation 后台调用）。
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
