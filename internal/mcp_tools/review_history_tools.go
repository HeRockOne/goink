package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"novel/internal/review"
)

// ── get_review_history ───────────────────────────────────

// GetReviewHistoryArgs 是 get_review_history 的参数。
type GetReviewHistoryArgs struct {
	Chapter       *int `json:"chapter" jsonschema:"description=按章节过滤，返回审读范围覆盖该章的记录,default="                              validate:"omitempty,gte=1"`
	Limit         int  `json:"limit" jsonschema:"description=最多返回条数,default=10,maximum=50" validate:"omitempty,min=1,max=50"`
	IncludeReport bool `json:"include_report" jsonschema:"description=是否附带完整报告原文（较长，默认只返回评分摘要）,default=false"`
}

// GetReviewHistoryTool 查询历史审稿记录（只读）。
type GetReviewHistoryTool struct{}

func (t *GetReviewHistoryTool) Name() string           { return "get_review_history" }
func (t *GetReviewHistoryTool) Description() string    { return getReviewHistoryDescription }
func (t *GetReviewHistoryTool) Category() ToolCategory { return CategoryConsistencyCheck }
func (t *GetReviewHistoryTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetReviewHistoryArgs{})
}
func (t *GetReviewHistoryTool) ExposeToLLM() bool { return true }
func (t *GetReviewHistoryTool) NewArgs() any      { return &GetReviewHistoryArgs{} }

func (t *GetReviewHistoryTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetReviewHistoryArgs)
	if tc.DB == nil {
		return nil, fmt.Errorf("数据库未配置")
	}
	limit := a.Limit
	if limit < 1 {
		limit = 10
	}
	q := tc.DB.Where("novel_id = ?", tc.NovelID)
	if a.Chapter != nil {
		ch := *a.Chapter
		q = q.Where("chapter_start <= ? AND chapter_end >= ?", ch, ch)
	}
	var records []review.ReviewRecord
	if err := q.Order("created_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return &ToolResult{Success: false, Error: "查询审稿记录失败: " + err.Error()}, nil
	}

	type summary struct {
		ID           int64   `json:"id"`
		ChapterStart int     `json:"chapter_start"`
		ChapterEnd   int     `json:"chapter_end"`
		TotalScore   float64 `json:"total_score"`
		Verdict      string  `json:"verdict"`
		FatalCount   int     `json:"fatal_count"`
		DimStructure float64 `json:"dim_structure"`
		DimCharacter float64 `json:"dim_character"`
		DimPacing    float64 `json:"dim_pacing"`
		DimProse     float64 `json:"dim_prose"`
		DimScene     float64 `json:"dim_scene"`
		CreatedAt    string  `json:"created_at"`
		Report       string  `json:"report,omitempty"`
		Instruction  string  `json:"instruction,omitempty"`
	}
	out := make([]summary, 0, len(records))
	for _, r := range records {
		s := summary{
			ID: r.ID, ChapterStart: r.ChapterStart, ChapterEnd: r.ChapterEnd,
			TotalScore: r.TotalScore, Verdict: r.Verdict, FatalCount: r.FatalCount,
			DimStructure: r.DimStructure, DimCharacter: r.DimCharacter, DimPacing: r.DimPacing,
			DimProse: r.DimProse, DimScene: r.DimScene,
			CreatedAt: r.CreatedAt.Format("2006-01-02 15:04"),
		}
		if a.IncludeReport {
			s.Report = r.Report
			s.Instruction = r.Instruction
		} else if r.Instruction != "" && len(r.Instruction) > 80 {
			s.Instruction = r.Instruction[:80] + "..."
		} else {
			s.Instruction = r.Instruction
		}
		out = append(out, s)
	}
	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"records": out,
			"count":   len(out),
		},
	}, nil
}

const getReviewHistoryDescription = `查询本小说的历史审稿记录（只读）。每次 review 子代理完成审稿后系统自动存档，此处可回查。

用途：
1. 修订章节时找回此前审稿提出的问题清单，逐条核对是否已修复
2. 查看分数趋势与反复出现的问题类型（如连续多章"角色深度"偏低）
3. 上下文压缩后找回原报告细节，无需重跑子代理

返回字段：章节范围、总分、判定（pass/revise/fail）、致命问题数、5 维度分、时间。include_report=true 时附完整报告原文。`

// ── 注册 ──────────────────────────────────────────────────

// RegisterReviewTools 注册审稿记录工具。
func RegisterReviewTools(r *Registry) {
	r.Register(&GetReviewHistoryTool{})
}
