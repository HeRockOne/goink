package cacheprobe

import (
	"testing"
)

// 首轮固定前缀被动缓存建模：SimFirstHitRatio>0 时首轮 fixed 类消息与 tools 部分命中，
// hit/miss 返回值与 MissByCat 分类同步修正；=0 时保持全 miss 保守口径。
func TestFirstRoundHitRatio(t *testing.T) {
	old := SimFirstHitRatio
	defer func() { SimFirstHitRatio = old }()
	initTools()

	msgs := append([]map[string]any{}, fixedSystem()...)
	msgs = append(msgs, userMsg("测试首轮命中建模"))

	// 基线：ratio=0 → 首轮全 miss
	SimFirstHitRatio = 0
	c1 := NewTokenCache()
	c1.SetMissCat(missCatOf)
	h1, m1 := c1.Step(msgs)
	if h1 != 0 {
		t.Fatalf("ratio=0 时首轮应全 miss，hit=%d", h1)
	}
	if m1 <= 0 {
		t.Fatalf("ratio=0 时首轮 miss 应为正，miss=%d", m1)
	}
	fixedFull := c1.MissByCat["fixed"]

	// ratio=0.84 → 首轮 fixed 部分按比例命中
	SimFirstHitRatio = 0.84
	c2 := NewTokenCache()
	c2.SetMissCat(missCatOf)
	h2, m2 := c2.Step(msgs)
	if h2 <= 0 || m2 <= 0 {
		t.Fatalf("ratio=0.84 时首轮应部分命中，hit=%d miss=%d", h2, m2)
	}
	if h2+m2 != h1+m1 {
		t.Errorf("首轮总输入应不变，hit+miss=%d vs 基线 %d", h2+m2, h1+m1)
	}
	// fixed 类 miss 应约为全量的 (1-0.84)
	fixedHit := float64(h2) / float64(fixedFull) // fixedFull 含 tools
	if fixedHit < 0.5 || fixedHit > 1 {
		t.Errorf("首轮 fixed 命中占比异常: %v（期望 ~0.84）", fixedHit)
	}
	// MissByCat 的 fixed 应同步缩水
	if c2.MissByCat["fixed"] >= c1.MissByCat["fixed"] {
		t.Errorf("fixed miss 应随首轮命中缩小: %d vs 基线 %d", c2.MissByCat["fixed"], c1.MissByCat["fixed"])
	}
	// 累计口径一致
	if c2.hit != h2 || c2.miss != m2 {
		t.Errorf("累计与返回值不一致: hit %d/%d miss %d/%d", c2.hit, h2, c2.miss, m2)
	}
}
