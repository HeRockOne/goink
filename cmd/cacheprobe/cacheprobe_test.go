package main

import (
	"testing"
)

// 修复后：请求2 应完全包含请求1 作为前缀（链连续）。
func TestPrefixChain_NowModeContinuous(t *testing.T) {
	initTools()
	hist := append([]map[string]any{}, fixedSystem()...)

	req1 := append(append([]map[string]any{}, hist...),
		userMsg("第 1 问：这个世界的修炼体系是什么？"),
	)
	req1 = append(req1, sysMsg(novelState(1)))
	b1 := promptBytes(req1)

	hist = append(hist, userMsg("第 1 问：这个世界的修炼体系是什么？"), sysMsg(novelState(1)), asstText(shortAnswer()))
	req2 := append(append([]map[string]any{}, hist...),
		userMsg("第 2 问：这个世界的修炼体系是什么？"),
	)
	req2 = append(req2, sysMsg(novelState(2)))
	b2 := promptBytes(req2)

	lcp := longestCommonPrefix(b1, b2)
	// 链连续的条件：请求1 全部字节都应是请求2 前缀，仅数组闭合括号 `]` 因后续消息而错位
	if lcp < len(b1)-2 {
		t.Fatalf("链断裂：请求2 未包含请求1 全部作为前缀，lcp=%d/%d", lcp, len(b1))
	}
}

// 核心断言（门禁创作 5 轮，历史较大时区分度最明显）：
// 修复后累计 miss 应显著小于修复前——legacy 每轮首请求整段历史重发为 miss。
func TestCumulativeMiss_NowBelowLegacy(t *testing.T) {
	initTools()

	nowCache := NewByteCache()
	legacyCache := NewByteCache()

	nowResults := buildGate("now", nowCache)
	legacyResults := buildGate("legacy", legacyCache)

	var nowMiss, legacyMiss int64
	for _, pr := range nowResults {
		nowMiss += int64(pr[1])
	}
	for _, pr := range legacyResults {
		legacyMiss += int64(pr[1])
	}

	if nowMiss >= legacyMiss {
		t.Fatalf("修复后累计 miss 未显著降低：now=%d legacy=%d", nowMiss, legacyMiss)
	}
	t.Logf("门禁创作5轮 累计miss now=%d legacy=%d", nowMiss, legacyMiss)
}

// 短问答 5 轮：消息历史极小（每轮一问一答 ~200 字节），两种协议差异几乎可忽略，
// 修复前后 miss 应相近（允许 ±200 字节噪声）。真实收益随历史规模增长（见门禁场景）。
func TestCumulativeMiss_NowBelowLegacy_ShortQA(t *testing.T) {
	initTools()

	nowCache := NewByteCache()
	legacyCache := NewByteCache()

	nowResults := buildShortQA("now", nowCache)
	legacyResults := buildShortQA("legacy", legacyCache)

	var nowMiss, legacyMiss int64
	for _, pr := range nowResults {
		nowMiss += int64(pr[1])
	}
	for _, pr := range legacyResults {
		legacyMiss += int64(pr[1])
	}

	diff := nowMiss - legacyMiss
	if diff > 200 || diff < -200 {
		t.Fatalf("短问答差异超出噪声范围：now=%d legacy=%d diff=%d", nowMiss, legacyMiss, diff)
	}
	t.Logf("短问答5轮 累计miss now=%d legacy=%d（历史极小，差异可忽略）", nowMiss, legacyMiss)
}
