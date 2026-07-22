package storage

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

// PatchAndSave 读取实体、用 input 的非零值字段覆盖、然后 Save。
// Save 会触发 GORM 回调（包括 operation_log），解决了 Model().Where().Updates() 不走回调的问题。
//
// 原理：input 的 omitempty tag 使 json.Marshal 只序列化非零字段，
// json.Unmarshal 到 entity 时只覆盖这些字段，未出现的保持 DB 原值——恰好等价于 PATCH 语义。
//
// 【重要约束】input 结构体的所有字段必须带 json:"...,omitempty" tag。
// 否则 json.Marshal 会序列化零值，json.Unmarshal 会覆盖 entity 的原有字段为零值。
// CI 脚本 scripts/check_omitempty 会自动检查所有 Update*Input 结构体。
// 不走 PatchAndSave 的 Input 可加 //nolint:omitempty 注释跳过检查。
//
// 【已知限制】omitempty 导致无法将 string 清空为 ""、将 int 清零。
// 需要清空字段时用指针类型（如 *string、*int64）或显式 ClearXxx bool flag。
//
// 用法：
//
//	var ch character.Character
//	if err := PatchAndSave(db, charID, novelID, &input, &ch); err != nil { ... }
func PatchAndSave[T any](db *gorm.DB, id int64, novelID int64, input any, entity *T) error {
	// 1. 加载当前行
	if err := db.Where("id = ? AND novel_id = ?", id, novelID).First(entity).Error; err != nil {
		return fmt.Errorf("patch: load entity: %w", err)
	}
	// 2. input → JSON → entity（只覆盖有值的字段）
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("patch: marshal input: %w", err)
	}
	if err := json.Unmarshal(data, entity); err != nil {
		return fmt.Errorf("patch: unmarshal to entity: %w", err)
	}
	// 3. Save 触发 GORM 回调
	if err := db.Save(entity).Error; err != nil {
		return fmt.Errorf("patch: save entity: %w", err)
	}
	return nil
}

// PatchAndSaveNoNovel 与 PatchAndSave 相同，但不要求 novel_id 条件。
// 用于没有 novel_id 字段的实体（如全局偏好）。
func PatchAndSaveNoNovel[T any](db *gorm.DB, id int64, input any, entity *T) error {
	if err := db.First(entity, id).Error; err != nil {
		return fmt.Errorf("patch: load entity: %w", err)
	}
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("patch: marshal input: %w", err)
	}
	if err := json.Unmarshal(data, entity); err != nil {
		return fmt.Errorf("patch: unmarshal to entity: %w", err)
	}
	if err := db.Save(entity).Error; err != nil {
		return fmt.Errorf("patch: save entity: %w", err)
	}
	return nil
}
