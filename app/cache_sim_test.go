package app

import (
	"testing"

	"novel/internal/cacheprobe"
)

func TestApplyHitAdjust(t *testing.T) {
	raw := &cacheprobe.WindowCostResult{
		FinalHit:  97_000_000,
		FinalMiss: 3_000_000,
		MissByCat: map[string]int64{"thinking": 2_000_000, "body": 1_000_000},
		Marks: []cacheprobe.WindowMark{
			{Threshold: 128 * 1024, Reached: true, Hit: 30_000_000, Miss: 1_000_000},
			{Threshold: 256 * 1024, Reached: true, Hit: 60_000_000, Miss: 2_000_000},
		},
		StageMarks: []cacheprobe.StageMark{
			{Stage: "开书完成", Hit: 5_000_000, Miss: 200_000},
		},
	}
	applyHitAdjust(raw, 0.95)

	total := raw.FinalHit + raw.FinalMiss
	if total != 100_000_000 {
		t.Fatalf("total changed: %d", total)
	}
	rate := float64(raw.FinalHit) / float64(total)
	if rate > 0.922 || rate < 0.920 {
		t.Fatalf("expected rate ~0.9215, got %f", rate)
	}
	// MissByCat 缩放保持比例
	if raw.MissByCat["thinking"]+raw.MissByCat["body"] != raw.FinalMiss {
		t.Fatalf("miss cat sum %d != final miss %d", raw.MissByCat["thinking"]+raw.MissByCat["body"], raw.FinalMiss)
	}
	if raw.Marks[1].Hit+raw.Marks[1].Miss != 62_000_000 {
		t.Fatalf("mark total changed: %d", raw.Marks[1].Hit+raw.Marks[1].Miss)
	}
	if raw.StageMarks[0].Hit+raw.StageMarks[0].Miss != 5_200_000 {
		t.Fatalf("stage total changed: %d", raw.StageMarks[0].Hit+raw.StageMarks[0].Miss)
	}
	t.Logf("hit=%d miss=%d rate=%.4f", raw.FinalHit, raw.FinalMiss, rate)
}
