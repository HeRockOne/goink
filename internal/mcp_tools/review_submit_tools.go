package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"novel/internal/review"
)

// ── submit_review ─────────────────────────────────────────
//
// 审稿评分的结构化提交通道：模型只交各维度分数与致命数，
// 加权总分与结论由代码按固定规则计算（唯一真相），
// 返回规范结论行供门控确定性匹配。本工具不写库——
// 落库由 RunSubAgent 扫描本调用参数后统一完成。

// SubmitReviewArgs 是 submit_review 的参数。
type SubmitReviewArgs struct {
	ChapterStart int     `json:"chapter_start" jsonschema:"required,description=审读起始章节号" validate:"required,min=1"`
	ChapterEnd   int     `json:"chapter_end"   jsonschema:"required,description=审读结束章节号（单章与 chapter_start 相同）" validate:"required,min=1"`
	DimStructure float64 `json:"dim_structure" jsonschema:"required,description=故事结构得分（0-10）：事实错误/逻辑漏洞/时间线/因果链/爽点兑现/卷纲范围/类型契合" validate:"required"`
	DimCharacter float64 `json:"dim_character" jsonschema:"required,description=角色深度得分（0-10）：OOC/行为动机/成长弧线/关系一致性" validate:"required"`
	DimPacing    float64 `json:"dim_pacing"    jsonschema:"required,description=节奏与爽点得分（0-10）：节奏拖沓/同质场景/爽点密度/钩子强度" validate:"required"`
	DimProse     float64 `json:"dim_prose"     jsonschema:"required,description=散文工艺得分（0-10）：AI味/重复句式/旁白越界/信息密度" validate:"required"`
	DimScene     float64 `json:"dim_scene"     jsonschema:"required,description=场景工程得分（0-10）：视角纯度/五感描写/场景必要性/衔接" validate:"required"`
	FatalCount   int     `json:"fatal_count"   jsonschema:"required,description=致命问题数（0 表示无致命）。致命=事实错误/OOC/设定矛盾等一票否决项"`
}

// SubmitReviewTool 提交审稿评分，代码计算加权总分并推导结论。
type SubmitReviewTool struct{}

func (t *SubmitReviewTool) Name() string           { return "submit_review" }
func (t *SubmitReviewTool) Description() string    { return submitReviewDescription }
func (t *SubmitReviewTool) Category() ToolCategory { return CategoryConsistencyCheck }

func (t *SubmitReviewTool) JSONSchema() json.RawMessage { return SchemaOf(SubmitReviewArgs{}) }
func (t *SubmitReviewTool) ExposeToLLM() bool           { return true }
func (t *SubmitReviewTool) NewArgs() any                { return &SubmitReviewArgs{} }

func (t *SubmitReviewTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*SubmitReviewArgs)

	if a.ChapterEnd < a.ChapterStart {
		return &ToolResult{Success: false, Error: fmt.Sprintf("chapter_end(%d) 不能小于 chapter_start(%d)", a.ChapterEnd, a.ChapterStart)}, nil
	}
	dims := map[string]float64{
		"dim_structure": a.DimStructure,
		"dim_character": a.DimCharacter,
		"dim_pacing":    a.DimPacing,
		"dim_prose":     a.DimProse,
		"dim_scene":     a.DimScene,
	}
	for name, v := range dims {
		if v < 0 || v > 10 {
			return &ToolResult{Success: false, Error: fmt.Sprintf("%s=%v 越界，必须在 0-10 之间", name, v)}, nil
		}
	}
	if a.FatalCount < 0 {
		return &ToolResult{Success: false, Error: "fatal_count 不能为负数"}, nil
	}

	total := review.ComputeTotalScore(a.DimStructure, a.DimCharacter, a.DimPacing, a.DimProse, a.DimScene)
	verdict := review.DeriveVerdict(total, a.FatalCount)
	verdictCN := map[string]string{review.VerdictPass: "通过", review.VerdictRevise: "需修改", review.VerdictFail: "不通过"}[verdict]
	canonical := fmt.Sprintf("[审稿结论] 总分：%s/10（%s）", strconv.FormatFloat(total, 'f', -1, 64), verdictCN)

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"content":       canonical,
			"chapter_start": a.ChapterStart,
			"chapter_end":   a.ChapterEnd,
			"total_score":   total,
			"verdict":       verdict,
			"fatal_count":   a.FatalCount,
			"dim_structure": a.DimStructure,
			"dim_character": a.DimCharacter,
			"dim_pacing":    a.DimPacing,
			"dim_prose":     a.DimProse,
			"dim_scene":     a.DimScene,
		},
	}, nil
}

const submitReviewDescription = `提交审稿评分（审稿完成后、输出报告正文前必须先调用本工具）。
只交各维度原始分与致命问题数；加权总分和结论（通过/需修改/不通过）由系统按固定权重（结构30%/角色25%/节奏20%/散文15%/场景10%）与规则（无致命且≥9.0=通过；<7.0或含致命=不通过；其余=需修改）计算，无需也不要在报告中自行计算。
返回的 [审稿结论] 行是最终结论，报告正文须与其一致。`

// ── 注册 ──────────────────────────────────────────────────

// RegisterReviewSubmitTools 注册审稿评分提交工具。
func RegisterReviewSubmitTools(r *Registry) {
	r.Register(&SubmitReviewTool{})
}
