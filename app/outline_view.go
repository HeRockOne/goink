package app

import (
	"fmt"

	"novel/internal/outline"
	"novel/internal/volume"
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
	Theme           string `json:"theme"`
	WordCountPlan   int    `json:"word_count_plan"`
}

// SaveOutline 保存全书总纲（创建或更新）。
func (a *App) SaveOutline(novelID int64, input SaveOutlineInput) (*outline.Outline, error) {
	o := &outline.Outline{
		NovelID:         novelID,
		CoreConflict:    input.CoreConflict,
		GrowthArc:       input.GrowthArc,
		EndingDirection: input.EndingDirection,
		Theme:           input.Theme,
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

// ── Volume（卷纲）CRUD ──────────────────────────────────

// GetVolumes 获取指定小说的所有卷（按排序）。
func (a *App) GetVolumes(novelID int64) ([]volume.Volume, error) {
	store := volume.NewStore(a.db)
	volumes, err := store.ListByNovel(a.ctx, novelID)
	if err != nil {
		return nil, err
	}
	if volumes == nil {
		return []volume.Volume{}, nil
	}
	return volumes, nil
}

// SaveVolumeInput 是 SaveVolume 的参数。
type SaveVolumeInput struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	StartChapter int    `json:"start_chapter"`
	EndChapter   int    `json:"end_chapter"`
	DetailJSON   string `json:"detail_json"`
	SortOrder    int    `json:"sort_order"`
}

// SaveVolume 创建或更新一卷（id=0 为创建）。
func (a *App) SaveVolume(novelID int64, input SaveVolumeInput) (*volume.Volume, error) {
	store := volume.NewStore(a.db)
	if input.ID == 0 {
		// 创建
		v := volume.Volume{
			NovelID:      novelID,
			Name:         input.Name,
			Description:  input.Description,
			StartChapter: input.StartChapter,
			EndChapter:   input.EndChapter,
			DetailJSON:   input.DetailJSON,
			SortOrder:    input.SortOrder,
		}
		if err := store.Create(a.ctx, &v); err != nil {
			return nil, fmt.Errorf("create volume: %w", err)
		}
		return &v, nil
	}
	// 更新
	v, err := store.GetByID(a.ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("volume not found: %d", input.ID)
	}
	if input.Name != "" {
		v.Name = input.Name
	}
	if input.Description != "" {
		v.Description = input.Description
	}
	if input.StartChapter > 0 {
		v.StartChapter = input.StartChapter
	}
	if input.EndChapter > 0 {
		v.EndChapter = input.EndChapter
	}
	if input.DetailJSON != "" {
		v.DetailJSON = input.DetailJSON
	}
	if input.SortOrder > 0 {
		v.SortOrder = input.SortOrder
	}
	if err := store.Update(a.ctx, v); err != nil {
		return nil, fmt.Errorf("update volume: %w", err)
	}
	return v, nil
}

// DeleteVolume 删除一卷。
func (a *App) DeleteVolume(volumeID int64) error {
	if volumeID == 0 {
		return fmt.Errorf("卷 ID 不能为空")
	}
	store := volume.NewStore(a.db)
	return store.Delete(a.ctx, volumeID)
}
