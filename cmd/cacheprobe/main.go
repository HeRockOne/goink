// cacheprobe 缓存命中率探针 CLI（薄壳，核心逻辑在 internal/cacheprobe 库）。
//
// 无网络、无 LLM 调用、无需 API Key 的缓存命中率模拟工具。验证 NovelState 落库协议
// （P1）对 DeepSeek/商汤前缀缓存的收益。
package main

import (
	"fmt"
	"os"

	"novel/internal/cacheprobe"
)

func main() {
	gateRounds := 5
	shortQA := 5
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &gateRounds)
	}
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &shortQA)
	}

	res, err := cacheprobe.Run(gateRounds, shortQA)
	if err != nil {
		fmt.Fprintln(os.Stderr, "模拟失败:", err)
		os.Exit(1)
	}

	fmt.Println("================================================================")
	fmt.Println(" cacheprobe：缓存命中对照（消息级前缀模拟，tiktoken 精确计数）")
	fmt.Println(" 对比：修复前（NS 不落库） vs 修复后（NS 落库）")
	fmt.Println("================================================================")

	for _, s := range res.Scenarios {
		fmt.Printf("\n=== %s ===\n", s.Name)
		fmt.Printf("  修复前  hit=%12d miss=%12d 命中率=%5.1f%%\n", s.LegacyHit, s.LegacyMiss, pct(s.LegacyHit, s.LegacyMiss))
		fmt.Printf("  修复后  hit=%12d miss=%12d 命中率=%5.1f%%\n", s.NowHit, s.NowMiss, pct(s.NowHit, s.NowMiss))
		missSave := 0.0
		if s.LegacyMiss > 0 {
			missSave = float64(s.LegacyMiss-s.NowMiss) / float64(s.LegacyMiss) * 100
		}
		fmt.Printf("  miss 降幅 = %.1f%%（未命中的 token 直接按全价计费，此项即真实成本节约）\n", missSave)
	}

	fmt.Printf("\n=== 汇总 ===\n")
	fmt.Printf("  修复前  hit=%12d miss=%12d 命中率=%5.1f%%\n", res.TotalLegacyHit, res.TotalLegacyMiss, pct(res.TotalLegacyHit, res.TotalLegacyMiss))
	fmt.Printf("  修复后  hit=%12d miss=%12d 命中率=%5.1f%%\n", res.TotalNowHit, res.TotalNowMiss, pct(res.TotalNowHit, res.TotalNowMiss))
}

func pct(hit, miss int64) float64 {
	if hit+miss == 0 {
		return 0
	}
	return 100 * float64(hit) / float64(hit+miss)
}
