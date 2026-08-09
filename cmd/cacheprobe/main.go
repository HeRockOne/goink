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

func pct(hit, miss int64) float64 {
	if hit+miss == 0 {
		return 0
	}
	return 100 * float64(hit) / float64(hit+miss)
}
