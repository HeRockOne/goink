// cacheprobe 缓存命中率探针 CLI（薄壳，核心逻辑在 internal/cacheprobe 库）。
//
// 无网络、无 LLM 调用、无需 API Key 的缓存命中率模拟工具。三方协议对照：
//   legacy：NS 不落库（修复前基线）
//   now：NS 落库（当前优化，缓存前缀连续）
//   clean：NS 落库 + 工具结果清理（读过的 skill 全文 → 占位符，防注意力漂移）
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"novel/internal/cacheprobe"
)

func main() {
	gateRounds, shortQA, batchChapters, cleanRetain := 5, 5, 5, 0
	cachePrice := flag.Float64("cache", 0.02, "缓存命中价格（元/百万 token）")
	inputPrice := flag.Float64("input", 1.0, "输入（未命中）价格（元/百万 token）")
	outputPrice := flag.Float64("output", 2.0, "输出价格（元/百万 token）")
	flag.Parse()

	args := flag.Args()
	if len(args) > 0 && args[0] == "matrix" {
		runMatrix(*cachePrice, *inputPrice)
		return
	}
	if len(args) > 0 && args[0] == "compare" {
		runCompare(*cachePrice, *inputPrice, *outputPrice)
		return
	}
	if len(args) > 0 && args[0] == "nsondemand" {
		runNSOnDemand(*cachePrice, *inputPrice, *outputPrice)
		return
	}
	if len(args) > 0 && args[0] == "skilldedup" {
		runSkillDedup(*cachePrice, *inputPrice, *outputPrice)
		return
	}
	if len(args) > 0 && args[0] == "window" {
		runWindow(*cachePrice, *inputPrice, *outputPrice)
		return
	}
	if len(args) > 0 && args[0] == "table" {
		runTable(*cachePrice, *inputPrice, *outputPrice)
		return
	}
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &gateRounds)
	}
	if len(args) > 1 {
		fmt.Sscanf(args[1], "%d", &shortQA)
	}
	if len(args) > 2 {
		fmt.Sscanf(args[2], "%d", &batchChapters)
	}
	if len(args) > 3 {
		fmt.Sscanf(args[3], "%d", &cleanRetain)
	}

	res, err := cacheprobe.Run(gateRounds, shortQA, batchChapters, cleanRetain)
	if err != nil {
		fmt.Fprintln(os.Stderr, "模拟失败:", err)
		os.Exit(1)
	}

	fmt.Println("================================================================")
	fmt.Println(" cacheprobe：缓存命中对照（消息级前缀模拟，tiktoken 精确计数）")
	fmt.Println(" 三协议：legacy(NS不落库) / now(NS落库) / clean(NS落库+skill全文清理)")
	fmt.Println(" 场景：一个真实对话窗口——短对话与单章/批量创作交替，一条历史贯穿")
	fmt.Printf(" 价格：缓存 ¥%.3f/M · 输入 ¥%.3f/M · 输出 ¥%.3f/M（-cache/-input/-output 可改）\n", *cachePrice, *inputPrice, *outputPrice)
	fmt.Println("================================================================")

	for _, s := range res.Scenarios {
		fmt.Printf("\n=== %s ===\n", s.Name)
		fmt.Printf("  legacy  hit=%12d miss=%10d out=%10d 命中率=%5.1f%% 成本¥%.4f\n", s.LegacyHit, s.LegacyMiss, s.LegacyOutput, pct(s.LegacyHit, s.LegacyMiss), cost(s.LegacyHit, s.LegacyMiss, s.LegacyOutput, *cachePrice, *inputPrice, *outputPrice))
		fmt.Printf("  now     hit=%12d miss=%10d out=%10d 命中率=%5.1f%% 成本¥%.4f\n", s.NowHit, s.NowMiss, s.NowOutput, pct(s.NowHit, s.NowMiss), cost(s.NowHit, s.NowMiss, s.NowOutput, *cachePrice, *inputPrice, *outputPrice))
		fmt.Printf("  clean   hit=%12d miss=%10d out=%10d 命中率=%5.1f%% 成本¥%.4f\n", s.CleanHit, s.CleanMiss, s.CleanOutput, pct(s.CleanHit, s.CleanMiss), cost(s.CleanHit, s.CleanMiss, s.CleanOutput, *cachePrice, *inputPrice, *outputPrice))
		// now vs clean：总输入 token（真实成本口径）
		nowTotal := s.NowHit + s.NowMiss
		cleanTotal := s.CleanHit + s.CleanMiss
		save := 0.0
		if nowTotal > 0 {
			save = float64(nowTotal-cleanTotal) / float64(nowTotal) * 100
		}
		fmt.Printf("  clean 总输入降幅 = %.1f%%（now %d → clean %d；清理的历史不再随每请求重发）\n", save, nowTotal, cleanTotal)
	}

	fmt.Printf("\n=== 汇总 ===\n")
	fmt.Printf("  legacy  hit=%12d miss=%10d out=%10d 命中率=%5.1f%% 成本¥%.4f\n", res.TotalLegacyHit, res.TotalLegacyMiss, res.TotalLegacyOutput, pct(res.TotalLegacyHit, res.TotalLegacyMiss), cost(res.TotalLegacyHit, res.TotalLegacyMiss, res.TotalLegacyOutput, *cachePrice, *inputPrice, *outputPrice))
	fmt.Printf("  now     hit=%12d miss=%10d out=%10d 命中率=%5.1f%% 成本¥%.4f\n", res.TotalNowHit, res.TotalNowMiss, res.TotalNowOutput, pct(res.TotalNowHit, res.TotalNowMiss), cost(res.TotalNowHit, res.TotalNowMiss, res.TotalNowOutput, *cachePrice, *inputPrice, *outputPrice))
	fmt.Printf("  clean   hit=%12d miss=%10d out=%10d 命中率=%5.1f%% 成本¥%.4f\n", res.TotalCleanHit, res.TotalCleanMiss, res.TotalCleanOutput, pct(res.TotalCleanHit, res.TotalCleanMiss), cost(res.TotalCleanHit, res.TotalCleanMiss, res.TotalCleanOutput, *cachePrice, *inputPrice, *outputPrice))
	nowTotal := res.TotalNowHit + res.TotalNowMiss
	cleanTotal := res.TotalCleanHit + res.TotalCleanMiss
	if nowTotal > 0 {
		fmt.Printf("  clean 总输入降幅 = %.1f%%（now %d → clean %d）\n", float64(nowTotal-cleanTotal)/float64(nowTotal)*100, nowTotal, cleanTotal)
	}
}

// cost 按价格计算总成本（元）。价格单位：元/百万 token。
func cost(hit, miss, out int64, cachePrice, inputPrice, outputPrice float64) float64 {
	return float64(hit)*cachePrice/1e6 + float64(miss)*inputPrice/1e6 + float64(out)*outputPrice/1e6
}

// runMatrix 边界矩阵：会话结构 × 保留窗口，找 clean 的收益拐点。
func runMatrix(cachePrice, inputPrice float64) {
	res, err := cacheprobe.RunMatrix()
	if err != nil {
		fmt.Fprintln(os.Stderr, "矩阵模拟失败:", err)
		os.Exit(1)
	}
	fmt.Println("================================================================")
	fmt.Println(" clean 边界矩阵：会话结构 × 保留窗口（-1=now 不清理，0=全清，N=保留N条）")
	fmt.Printf(" 指标：总输入降幅%% / 命中率%% / 成本¥（hit×%.2f + miss×%.2f，输入口径）\n", cachePrice, inputPrice)
	fmt.Println("================================================================")

	fmt.Printf("\n%-22s %-24s %-24s %-24s %-24s %-24s\n", "会话结构", "retain=-1(now)", "retain=0", "retain=1", "retain=3", "retain=5")
	fmt.Println(strings.Repeat("-", 150))
	for _, row := range res.Rows {
		fmt.Printf("%-22s", row.Name)
		for _, cell := range row.Cells {
			label := fmt.Sprintf("↓%.1f%%/%.1f%%/¥%.4f", cell.SaveVsNowPct, cell.HitRate, cell.Cost)
			fmt.Printf("%-24s", label)
		}
		fmt.Println()
	}
}

// runNSOnDemand NS 按需注入对照（非写章轮跳过重复 NS，所有模型通用收益）。
func runNSOnDemand(cachePrice, inputPrice, outputPrice float64) {
	res, err := cacheprobe.RunNSOnDemand()
	if err != nil {
		fmt.Fprintln(os.Stderr, "NS 按需注入模拟失败:", err)
		os.Exit(1)
	}
	fmt.Println("================================================================")
	fmt.Printf(" NS 按需注入对照（%s）\n", res.Scenario)
	fmt.Printf(" 价格：缓存 ¥%.3f/M · 输入 ¥%.3f/M\n", cachePrice, inputPrice)
	fmt.Println(" 差异唯一：非写章轮 NS 字节不变时，现状每轮重复注入 vs 按需跳过")
	fmt.Println("================================================================")
	fmt.Printf(" 现状     hit=%12d miss=%10d 请求=%d  命中率=%5.1f%%  成本¥%.4f\n",
		res.NowHit, res.NowMiss, res.NowRequests, res.NowHitRate(), cost(res.NowHit, res.NowMiss, 0, cachePrice, inputPrice, outputPrice))
	fmt.Printf(" 按需注入 hit=%12d miss=%10d 请求=%d  命中率=%5.1f%%  成本¥%.4f\n",
		res.LayeredHit, res.LayeredMiss, res.LayeredRequests, res.LayeredHitRate(), cost(res.LayeredHit, res.LayeredMiss, 0, cachePrice, inputPrice, outputPrice))
	fmt.Printf("  miss 降幅 = %.1f%%（现状 %d → 按需 %d）\n",
		res.MissSavePct(), res.NowMiss, res.LayeredMiss)
}

// runSkillDedup 技能注入去重对照（同阶段技能全文只注入一次）。
func runSkillDedup(cachePrice, inputPrice, outputPrice float64) {
	res, err := cacheprobe.RunSkillDedup()
	if err != nil {
		fmt.Fprintln(os.Stderr, "技能去重模拟失败:", err)
		os.Exit(1)
	}
	fmt.Println("================================================================")
	fmt.Printf(" 技能注入去重对照（%s）\n", res.Scenario)
	fmt.Printf(" 价格：缓存 ¥%.3f/M · 输入 ¥%.3f/M\n", cachePrice, inputPrice)
	fmt.Println(" 差异唯一：同阶段技能全文是否重复注入（历史中已有则跳过）")
	fmt.Println("================================================================")
	fmt.Printf(" 现状     hit=%12d miss=%10d 请求=%d  命中率=%5.1f%%  成本¥%.4f\n",
		res.NowHit, res.NowMiss, res.NowRequests, res.NowHitRate(), cost(res.NowHit, res.NowMiss, 0, cachePrice, inputPrice, outputPrice))
	fmt.Printf(" 技能去重 hit=%12d miss=%10d 请求=%d  命中率=%5.1f%%  成本¥%.4f\n",
		res.LayeredHit, res.LayeredMiss, res.LayeredRequests, res.LayeredHitRate(), cost(res.LayeredHit, res.LayeredMiss, 0, cachePrice, inputPrice, outputPrice))
	fmt.Printf("  miss 降幅 = %.1f%%（现状 %d → 去重 %d）\n",
		res.MissSavePct(), res.NowMiss, res.LayeredMiss)
}

// modelPrice 模型价格档（元/百万 token）：cache hit / input miss / output。
type modelPrice struct {
	name        string
	cache, in, out float64
	note        string
}

// runWindow 上下文规模刻度测试：单窗口内历史增长到 128K/256K/512K/1024K 时的成本快照。
// 矩阵：模型（价格档）× 模式（批量/单章）× 刻度。区间成本 = 相邻刻度累计成本之差，找最省区间。
func runWindow(cachePrice, inputPrice, outputPrice float64) {
	models := []modelPrice{
		{"deepseek-v4-flash", 0.02, 1.0, 2.0, "1M 窗口"},
		{"deepseek-v4-pro", 0.026, 3.13, 6.26, "1M 窗口"},
	}
	modes := []struct {
		key, name string
		chapters  int
	}{
		// 单章：每章一轮完整门禁，~40K/章 → 1M 窗口约 25 章
		{"single", "单章模式（每章全门禁流程，1M 窗口约 25 章）", 26},
		// 批量：每批 6 章完整批量门禁，批次循环，~50K/批 → 1M 窗口约 20 批（120 章）
		{"batch", "批量模式（每批 6 章完整批量门禁，批次循环）", 120},
	}

	fmt.Println("================================================================")
	fmt.Println(" 单窗口上下文规模刻度（不压缩理想模型，0.7×窗口为真机压缩参考线）")
	fmt.Println(" 刻度 = 单窗口内历史（请求输入）增长到 128K/256K/512K/1024K 的时刻")
	fmt.Println("================================================================")

	for _, m := range models {
		fmt.Printf("\n### 模型 %s（hit ¥%.3f/M · miss ¥%.3f/M · out ¥%.3f/M，%s）\n\n", m.name, m.cache, m.in, m.out, m.note)
		for _, md := range modes {
			fmt.Printf("**%s**\n\n", md.name)
			res, err := cacheprobe.RunWindowCost(md.key, md.chapters)
			if err != nil {
				fmt.Fprintln(os.Stderr, "窗口刻度模拟失败:", err)
				os.Exit(1)
			}
			fmt.Printf("| 刻度 | 到达时 | 累计成本¥ | 累计 miss | 累计 hit | 请求数 | 命中率 |\n")
			fmt.Printf("|------|--------|----------|-----------|----------|--------|--------|\n")
			prevCost := 0.0
			prevMiss := int64(0)
			prevCh := 0
			prevTh := int64(0)
			prevReached := false
			bestName := ""
			bestPerCh := 1e18
			for _, mk := range res.Marks {
				label := fmt.Sprintf("%dK", mk.Threshold/1024)
				if !mk.Reached {
					fmt.Printf("| %s | 未到达 | - | - | - | - | - |\n", label)
					continue
				}
				c := cost(mk.Hit, mk.Miss, mk.Out, m.cache, m.in, m.out)
				rate := 100 * float64(mk.Hit) / float64(mk.Hit+mk.Miss)
				fmt.Printf("| %s | 第 %d 章 | %.4f | %dK | %dK | %d | %.1f%% |\n",
					label, mk.Chapter, c, mk.Miss/1000, mk.Hit/1000, mk.Requests, rate)
				if prevReached {
					intervalCost := c - prevCost
					perCh := 0.0
					if mk.Chapter-prevCh > 0 {
						perCh = intervalCost / float64(mk.Chapter-prevCh)
					}
					if perCh < bestPerCh {
						bestPerCh = perCh
						bestName = fmt.Sprintf("%dK→%dK", prevTh/1024, mk.Threshold/1024)
					}
					fmt.Printf("| 区间 %dK→%dK | %d 章 | +%.4f | +%dK | - | - | 每章 ¥%.4f |\n",
						prevTh/1024, mk.Threshold/1024, mk.Chapter-prevCh, intervalCost, (mk.Miss-prevMiss)/1000, perCh)
				}
				prevCost = c
				prevMiss = mk.Miss
				prevCh = mk.Chapter
				prevTh = mk.Threshold
				prevReached = true
			}
			if bestName != "" {
				fmt.Printf("> 最省区间：%s（每章 ¥%.4f）\n\n", bestName, bestPerCh)
			}
			// 终点（最终历史大小）
			fc := cost(res.FinalHit, res.FinalMiss, res.FinalOut, m.cache, m.in, m.out)
			frate := 100 * float64(res.FinalHit) / float64(res.FinalHit+res.FinalMiss)
			fmt.Printf("| 终点（历史 %dK） | %d 请求 | %.4f | %dK | %dK | - | %.1f%% |\n",
				res.FinalTotal/1000, res.FinalReqs, fc, res.FinalMiss/1000, res.FinalHit/1000, frate)
			fmt.Println()
		}
	}
}

// runCompare 同章数不同模式对比：单章/批量 × 是否清理。
func runCompare(cachePrice, inputPrice, outputPrice float64) {
	res, err := cacheprobe.CompareModes()
	if err != nil {
		fmt.Fprintln(os.Stderr, "对比模拟失败:", err)
		os.Exit(1)
	}
	fmt.Println("================================================================")
	fmt.Printf(" 创作模式对比（同 4 章产出,成本口径 hit×%.2f + miss×%.2f，输入）\n", cachePrice, inputPrice)
	fmt.Println("================================================================")
	fmt.Printf("\n%-28s %12s %10s %9s %10s %10s\n", "模式", "总输入", "miss", "命中率", "成本¥", "降幅")
	fmt.Println(strings.Repeat("-", 82))
	for _, m := range res.Modes {
		fmt.Printf("%-28s %10dK %9dK %8.1f%% %9.4f %9.1f%%\n",
			m.Name, m.TotalIn/1000, m.Miss/1000, m.HitRate, m.Cost, m.CostSave)
	}
}

// tableScenario 表格场景：单章轮数 / 短对话轮数 / 批量章数 / 展示名。
type tableScenario struct {
	g, q, b int
	name    string
}

// runTable 跑一组常用工作负载场景，输出 Markdown 表格（now 协议，含输出计费 + miss 构成）。
// 含"上下文规模维度"：批量 5/10/20/30/40/50/60 章，观察每章成本随对话窗口累积的变化。
// 标注 1M 窗口 × 0.7 压缩阈值 ≈ 700K token 的位置（模拟器不建模压缩，阈值后真机每章成本会跳升）。
func runTable(cachePrice, inputPrice, outputPrice float64) {
	scenarios := []tableScenario{
		{1, 0, 0, "单章 1 轮"},
		{3, 0, 0, "单章 3 轮"},
		{5, 0, 0, "单章 5 轮"},
		{5, 3, 0, "单章 5 轮 + 短对话 3"},
		{0, 0, 5, "批量 5 章"},
		{0, 2, 5, "批量 5 章 + 短对话 2"},
		{3, 2, 3, "混合 3+2+3"},
		{5, 5, 5, "混合 5+5+5"},
		{0, 0, 10, "批量 10 章"},
		{0, 0, 20, "批量 20 章"},
		{0, 0, 30, "批量 30 章"},
		{0, 0, 40, "批量 40 章（≈1M×0.7 压缩阈值边缘）"},
		{0, 0, 50, "批量 50 章（超阈值，真机需压缩）"},
		{0, 0, 60, "批量 60 章（超阈值，真机需压缩）"},
	}

	type row struct {
		sc   tableScenario
		h, m int64
		out  int64
		rate float64
		cost float64
		cat  map[string]int64
	}
	rows := make([]row, 0, len(scenarios))

	fmt.Printf("# cacheprobe 成本模拟表（now 协议）\n\n")
	fmt.Printf("价格：缓存 ¥%.3f/M · 输入 ¥%.3f/M · 输出 ¥%.3f/M　正文与思考按真实分布生成\n\n", cachePrice, inputPrice, outputPrice)
	fmt.Printf("| 场景 | 单章 | 短对话 | 批量 | 输入 hit | 输入 miss | 输出 out | 命中率 | 成本 ¥ | 每章 ¥ |\n")
	fmt.Printf("|------|-----|-------|------|---------|----------|---------|--------|--------|--------|\n")

	for _, sc := range scenarios {
		res, err := cacheprobe.Run(sc.g, sc.q, sc.b, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "场景 %s 模拟失败: %v\n", sc.name, err)
			continue
		}
		h, m, out := res.TotalNowHit, res.TotalNowMiss, res.TotalNowOutput
		rate := pct(h, m)
		c := cost(h, m, out, cachePrice, inputPrice, outputPrice)
		chapters := sc.g + sc.b
		if chapters == 0 {
			chapters = 1
		}
		fmt.Printf("| %s | %d | %d | %d | %d | %d | %d | %.1f%% | %.4f | %.4f |\n",
			sc.name, sc.g, sc.q, sc.b, h, m, out, rate, c, c/float64(chapters))
		rows = append(rows, row{sc: sc, h: h, m: m, out: out, rate: rate, cost: c, cat: res.TotalNowMissByCat})
	}

	// miss 构成表（now 协议，按消息来源分类）
	fmt.Printf("\n## miss 构成（now 协议，按消息来源分类）\n\n")
	fmt.Printf("| 场景 | miss 总计 | thinking | 技能注入 | 工具结果 | 查询 | 固定/NS | 正文 | 大纲 | 其他 |\n")
	fmt.Printf("|------|----------|----------|----------|----------|------|---------|------|------|------|\n")
	for _, r := range rows {
		get := func(k string) int64 { return r.cat[k] }
		fmt.Printf("| %s | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			r.sc.name, r.m,
			get("thinking"), get("skill_inject"), get("update"), get("query"), get("fixed"), get("body"), get("outline"), get("other"))
	}

	// 门禁配置一致性校验（plays 工具调用 vs 门禁配置白名单）
	fmt.Printf("\n## 门禁配置一致性（plays vs 门禁配置示例.md 白名单）\n\n")
	gc := cacheprobe.GateConfigLoaded()
	if gc == "" {
		fmt.Println("门禁配置未加载（未找到 门禁配置示例.md / GOINK_PHASE_CONFIG），跳过校验")
		return
	}
	fmt.Printf("门禁配置来源: %s\n", gc)
	for _, sc := range scenarios {
		mode := "single"
		if sc.b > 0 {
			mode = "batch"
		}
		warns := cacheprobe.ValidatePlays(sc.g, sc.q, sc.b, mode)
		if len(warns) == 0 {
			fmt.Printf("- %s：✓ 全部工具调用在阶段白名单内\n", sc.name)
			continue
		}
		fmt.Printf("- %s：⚠ %d 处不一致\n", sc.name, len(warns))
		for _, w := range warns {
			fmt.Printf("    %s\n", w)
		}
	}
}

func pct(hit, miss int64) float64 {
	if hit+miss == 0 {
		return 0
	}
	return 100 * float64(hit) / float64(hit+miss)
}
