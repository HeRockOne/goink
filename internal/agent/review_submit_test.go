package agent

import (
	"strings"
	"testing"
)

func TestFindSubmitReviewArgs(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "审读第22章"},
		{"role": "assistant", "content": ""},
		// 无 tool_calls 的 assistant 应跳过
		{"role": "tool", "content": "ok"},
		{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"function": map[string]any{
						"name": "submit_review",
						"arguments": `{"chapter_start":22,"chapter_end":22,"dim_structure":8,"dim_character":9,` +
							`"dim_pacing":7,"dim_prose":5,"dim_scene":9,"fatal_count":0}`,
					},
				},
				map[string]any{
					"function": map[string]any{
						"name": "read",
						"arguments": `{"path":"chapters/022.md"}`,
					},
				},
			},
		},
	}

	args, ok := findSubmitReviewArgs(msgs)
	if !ok {
		t.Fatal("findSubmitReviewArgs 未找到 submit_review 调用")
	}
	if v, _ := args["dim_prose"].(float64); v != 5 {
		t.Errorf("dim_prose = %v, want 5", v)
	}
	if v, _ := args["fatal_count"].(float64); v != 0 {
		t.Errorf("fatal_count = %v, want 0", v)
	}
}

func TestFindSubmitReviewArgs_LastWins(t *testing.T) {
	msgs := []map[string]any{
		{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{"function": map[string]any{"name": "submit_review",
					"arguments": `{"chapter_start":22,"dim_structure":5}`}},
			},
		},
		{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{"function": map[string]any{"name": "submit_review",
					"arguments": `{"chapter_start":22,"dim_structure":8}`}},
			},
		},
	}
	args, ok := findSubmitReviewArgs(msgs)
	if !ok {
		t.Fatal("未找到调用")
	}
	if v, _ := args["dim_structure"].(float64); v != 8 {
		t.Errorf("应取最后一次调用 dim_structure = %v, want 8", v)
	}
}

func TestFindSubmitReviewArgs_None(t *testing.T) {
	msgs := []map[string]any{{"role": "assistant", "content": "纯文本报告"}}
	if _, ok := findSubmitReviewArgs(msgs); ok {
		t.Error("无 submit_review 调用时不应返回 ok")
	}
}

func TestReviewRecordFromSubmit(t *testing.T) {
	args := map[string]any{
		"chapter_start": float64(22),
		"chapter_end":   float64(22),
		"dim_structure": float64(8),
		"dim_character": float64(9),
		"dim_pacing":    float64(7),
		"dim_prose":     float64(5),
		"dim_scene":     float64(9),
		"fatal_count":   float64(0),
	}
	rec := reviewRecordFromSubmit(args, "报告正文", "审读第22章")

	if rec.TotalScore != 7.7 {
		t.Errorf("TotalScore = %v, want 7.7（代码计算，非模型提交）", rec.TotalScore)
	}
	if rec.Verdict != "revise" {
		t.Errorf("Verdict = %q, want revise", rec.Verdict)
	}
	if rec.ChapterStart != 22 || rec.ChapterEnd != 22 {
		t.Errorf("chapters = %d-%d, want 22-22（显式参数，非正则解析）", rec.ChapterStart, rec.ChapterEnd)
	}
	if rec.DimProse != 5 || rec.FatalCount != 0 {
		t.Errorf("dims/fatal 未正确透传")
	}
	canonical, _ := args["canonical"].(string)
	if !strings.Contains(canonical, "[审稿结论] 总分：7.7/10（需修改）") {
		t.Errorf("规范结论行 = %q", canonical)
	}
}
