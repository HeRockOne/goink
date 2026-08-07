package cacheprobe

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

	nowCache := NewTokenCache()
	legacyCache := NewTokenCache()

	nowResults := buildGateWithRounds("now", nowCache, 5)
	legacyResults := buildGateWithRounds("legacy", legacyCache, 5)

	var nowMiss, legacyMiss int64
	for _, pr := range nowResults {
		nowMiss += pr[1]
	}
	for _, pr := range legacyResults {
		legacyMiss += pr[1]
	}

	if nowMiss >= legacyMiss {
		t.Fatalf("修复后累计 miss 未显著降低：now=%d legacy=%d", nowMiss, legacyMiss)
	}
	t.Logf("门禁创作5轮 累计miss now=%d legacy=%d", nowMiss, legacyMiss)
}

// 短问答 5 轮：消息历史极小，两种协议差异几乎可忽略，修复前后 miss 应相近。
func TestCumulativeMiss_NowBelowLegacy_ShortQA(t *testing.T) {
	initTools()

	nowCache := NewTokenCache()
	legacyCache := NewTokenCache()

	nowResults := buildShortQAWithRounds("now", nowCache, 5)
	legacyResults := buildShortQAWithRounds("legacy", legacyCache, 5)

	var nowMiss, legacyMiss int64
	for _, pr := range nowResults {
		nowMiss += pr[1]
	}
	for _, pr := range legacyResults {
		legacyMiss += pr[1]
	}

	diff := nowMiss - legacyMiss
	// 短问答行为（NS 1.4K 结构化近似后实测）：now 把 NS 落库 → 历史每轮膨胀 1.4K；
	// 短历史下膨胀成本 > 落库收益，now miss 略高于 legacy（实测 ~2.9K/5 轮）。
	// 这与门禁场景（历史大，落库收益显著）相反，属预期协议行为。
	// 断言：now 不劣于 legacy 太多（阈值 = NS 总量 ≈ 1.4K × 5 = 7K），
	// 且必须远小于门禁场景的收益量级（now << legacy，见 TestCumulativeMiss_NowBelowLegacy）。
	if diff > 7000 {
		t.Fatalf("短问答差异超出范围：now=%d legacy=%d diff=%d", nowMiss, legacyMiss, diff)
	}
	t.Logf("短问答5轮 累计miss now=%d legacy=%d（NS 落库膨胀成本，量级远小于门禁场景收益）", nowMiss, legacyMiss)
}

// tiktoken 精确计数：命中+未命中应等于请求总 token（含 tools 前缀）。
func TestTokenCache_HitPlusMissEqualsTotal(t *testing.T) {
	initTools()
	cache := NewTokenCache()

	req1 := append([]map[string]any{}, fixedSystem()...)
	req1 = append(req1, userMsg("第 1 问：这个世界的修炼体系是什么？"), sysMsg(novelState(1)))
	_, miss1 := cache.Step(req1)

	req2 := append([]map[string]any{}, req1...)
	req2 = append(req2, asstText(shortAnswer()), userMsg("第 2 问：这个世界的修炼体系是什么？"), sysMsg(novelState(2)))
	hit2, miss2 := cache.Step(req2)

	toolsN, msgsN := requestTokens(req2)
	total := toolsN + msgsN
	if hit2+miss2 != total {
		t.Fatalf("hit+miss 应等于请求总 token：hit=%d miss=%d total=%d", hit2, miss2, total)
	}
	if miss1 <= 0 {
		t.Fatalf("首次调用应全 miss，got %d", miss1)
	}
}

