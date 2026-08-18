package app

import (
	"fmt"

	"novel/internal/outline"
)

// ── Outline（全书总纲）CRUD ─────────────────────────────

// GetOutline 获取指定小说的总纲（不存在则返回 nil）。
func (a *App) GetOutline(novelID int64) (*outline.Outline, error) {
	o, err := a.outline.GetByNovelID(a.ctx, novelID)
	if err != nil {
		return nil, nil // 不存在时返回 nil，不报错
	}
	return o, nil
}

// SaveOutlineInput 是 SaveOutline 的参数。
type SaveOutlineInput struct {
	CoreConflict    string `json:"core_conflict"`
	GrowthArc       string `json:"growth_arc"`
	EndingDirection string `json:"ending_direction"`
	WordCountPlan   int    `json:"word_count_plan"`
}

// SaveOutline 保存全书总纲（创建或更新）。
func (a *App) SaveOutline(novelID int64, input SaveOutlineInput) (*outline.Outline, error) {
	o := &outline.Outline{
		NovelID:         novelID,
		CoreConflict:    input.CoreConflict,
		GrowthArc:       input.GrowthArc,
		EndingDirection: input.EndingDirection,
		WordCountPlan:   input.WordCountPlan,
	}
	if err := a.outline.Upsert(a.ctx, o); err != nil {
		return nil, fmt.Errorf("save outline: %w", err)
	}
	return o, nil
}

// ── OutlineBeat（大爽点）CRUD ───────────────────────────

// GetOutlineBeats 获取指定小说的所有大爽点（按章号排序）。
func (a *App) GetOutlineBeats(novelID int64) ([]outline.OutlineBeat, error) {
	beats, err := a.outline.ListBeats(a.ctx, novelID)
	if err != nil {
		return nil, err
	}
	if beats == nil {
		return []outline.OutlineBeat{}, nil
	}
	return beats, nil
}

// CreateOutlineBeatInput 是 CreateOutlineBeat 的参数。
type CreateOutlineBeatInput struct {
	Chapter     int    `json:"chapter"`
	Description string `json:"description"`
	BeatType    string `json:"beat_type"`
	Importance  int    `json:"importance"`
}

// CreateOutlineBeat 创建一条大爽点。
func (a *App) CreateOutlineBeat(novelID int64, input CreateOutlineBeatInput) (*outline.OutlineBeat, error) {
	if input.Description == "" {
		return nil, fmt.Errorf("爽点描述不能为空")
	}
	b := outline.OutlineBeat{
		NovelID:     novelID,
		Chapter:     input.Chapter,
		Description: input.Description,
		BeatType:    input.BeatType,
		Importance:  input.Importance,
	}
	if b.BeatType == "" {
		b.BeatType = "shuangdian"
	}
	if b.Importance == 0 {
		b.Importance = 3
	}
	if err := a.outline.CreateBeat(a.ctx, &b); err != nil {
		return nil, fmt.Errorf("create outline beat: %w", err)
	}
	return &b, nil
}

// UpdateOutlineBeatInput 是 UpdateOutlineBeat 的参数。
type UpdateOutlineBeatInput struct {
	ID          int64  `json:"id"`
	Chapter     int    `json:"chapter"`
	Description string `json:"description"`
	BeatType    string `json:"beat_type"`
	Importance  int    `json:"importance"`
}

// UpdateOutlineBeat 更新一条大爽点。
func (a *App) UpdateOutlineBeat(novelID int64, input UpdateOutlineBeatInput) (*outline.OutlineBeat, error) {
	if input.ID == 0 {
		return nil, fmt.Errorf("爽点 ID 不能为空")
	}
	b := outline.OutlineBeat{
		ID:          input.ID,
		NovelID:     novelID,
		Chapter:     input.Chapter,
		Description: input.Description,
		BeatType:    input.BeatType,
		Importance:  input.Importance,
	}
	if b.BeatType == "" {
		b.BeatType = "shuangdian"
	}
	if err := a.outline.UpdateBeat(a.ctx, &b); err != nil {
		return nil, fmt.Errorf("update outline beat: %w", err)
	}
	return &b, nil
}

// DeleteOutlineBeat 删除一条大爽点。
func (a *App) DeleteOutlineBeat(beatID int64) error {
	if beatID == 0 {
		return fmt.Errorf("爽点 ID 不能为空")
	}
	return a.outline.DeleteBeat(a.ctx, beatID)
}
