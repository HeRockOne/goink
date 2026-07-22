package storage

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
)

// TestPatchEntity 用于测试 PatchAndSave 的实体
type TestPatchEntity struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID     int64  `gorm:"column:novel_id;not null" json:"novel_id"`
	Name        string `gorm:"column:name" json:"name"`
	Description string `gorm:"column:description" json:"description"`
	Age         int    `gorm:"column:age" json:"age"`
	Status      string `gorm:"column:status" json:"status"`
}

func (TestPatchEntity) TableName() string { return "test_patch_entities" }

// TestPatchInput 模拟 UpdateXxxInput，omitempty 跳过零值
type TestPatchInput struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Age         int    `json:"age,omitempty"`
	Status      string `json:"status,omitempty"`
}

func setupPatchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: glog.Default.LogMode(glog.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&TestPatchEntity{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestPatchAndSave_PartialUpdate 验证只传部分字段时，其他字段保持不变
func TestPatchAndSave_PartialUpdate(t *testing.T) {
	db := setupPatchTestDB(t)

	// 创建初始实体
	original := TestPatchEntity{
		NovelID:     1,
		Name:        "alice",
		Description: "original desc",
		Age:         25,
		Status:      "active",
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 只更新 Name，其他字段不传
	input := TestPatchInput{Name: "alice_updated"}
	var loaded TestPatchEntity
	if err := PatchAndSave(db, original.ID, original.NovelID, &input, &loaded); err != nil {
		t.Fatalf("PatchAndSave: %v", err)
	}

	// 验证 Name 被更新
	if loaded.Name != "alice_updated" {
		t.Errorf("Name: expected 'alice_updated', got %q", loaded.Name)
	}
	// 验证其他字段保持不变
	if loaded.Description != "original desc" {
		t.Errorf("Description: expected 'original desc', got %q", loaded.Description)
	}
	if loaded.Age != 25 {
		t.Errorf("Age: expected 25, got %d", loaded.Age)
	}
	if loaded.Status != "active" {
		t.Errorf("Status: expected 'active', got %q", loaded.Status)
	}
	if loaded.NovelID != 1 {
		t.Errorf("NovelID: expected 1, got %d", loaded.NovelID)
	}
	if loaded.ID != original.ID {
		t.Errorf("ID: expected %d, got %d", original.ID, loaded.ID)
	}
}

// TestPatchAndSave_MultipleFields 验证同时更新多个字段
func TestPatchAndSave_MultipleFields(t *testing.T) {
	db := setupPatchTestDB(t)

	original := TestPatchEntity{
		NovelID:     1,
		Name:        "bob",
		Description: "desc1",
		Age:         30,
		Status:      "active",
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	input := TestPatchInput{
		Name:   "bob_new",
		Age:    35,
		Status: "inactive",
	}
	var loaded TestPatchEntity
	if err := PatchAndSave(db, original.ID, original.NovelID, &input, &loaded); err != nil {
		t.Fatalf("PatchAndSave: %v", err)
	}

	if loaded.Name != "bob_new" {
		t.Errorf("Name: expected 'bob_new', got %q", loaded.Name)
	}
	if loaded.Age != 35 {
		t.Errorf("Age: expected 35, got %d", loaded.Age)
	}
	if loaded.Status != "inactive" {
		t.Errorf("Status: expected 'inactive', got %q", loaded.Status)
	}
	// Description 未传，应保持原值
	if loaded.Description != "desc1" {
		t.Errorf("Description: expected 'desc1' (unchanged), got %q", loaded.Description)
	}
}

// TestPatchAndSave_OmitemptyLimitation 验证 omitempty 无法清空 string 字段
func TestPatchAndSave_OmitemptyLimitation(t *testing.T) {
	db := setupPatchTestDB(t)

	original := TestPatchEntity{
		NovelID:     1,
		Name:        "charlie",
		Description: "has desc",
		Age:         20,
		Status:      "active",
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 尝试清空 Description（传空字符串）
	input := TestPatchInput{Description: ""}
	var loaded TestPatchEntity
	if err := PatchAndSave(db, original.ID, original.NovelID, &input, &loaded); err != nil {
		t.Fatalf("PatchAndSave: %v", err)
	}

	// omitempty 导致空字符串被跳过，Description 保持原值
	if loaded.Description != "has desc" {
		t.Errorf("Description: expected 'has desc' (omitempty skips empty), got %q", loaded.Description)
	}
}

// TestPatchAndSave_EquivalentToUpdates 验证 PatchAndSave 与 Updates 行为等价
func TestPatchAndSave_EquivalentToUpdates(t *testing.T) {
	db := setupPatchTestDB(t)

	// 创建两个相同的实体
	entityA := TestPatchEntity{NovelID: 1, Name: "dave", Description: "desc", Age: 40, Status: "active"}
	entityB := TestPatchEntity{NovelID: 1, Name: "dave", Description: "desc", Age: 40, Status: "active"}
	if err := db.Create(&entityA).Error; err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := db.Create(&entityB).Error; err != nil {
		t.Fatalf("create B: %v", err)
	}

	input := TestPatchInput{Name: "dave_new", Age: 45}

	// A 用 PatchAndSave
	var loadedA TestPatchEntity
	if err := PatchAndSave(db, entityA.ID, entityA.NovelID, &input, &loadedA); err != nil {
		t.Fatalf("PatchAndSave A: %v", err)
	}

	// B 用原来的 Updates 模式
	if err := db.Model(&TestPatchEntity{}).
		Where("id = ? AND novel_id = ?", entityB.ID, entityB.NovelID).
		Updates(&input).Error; err != nil {
		t.Fatalf("Updates B: %v", err)
	}
	var loadedB TestPatchEntity
	db.First(&loadedB, entityB.ID)

	// 验证两者结果一致（除 ID 外）
	if loadedA.Name != loadedB.Name {
		t.Errorf("Name: PatchAndSave=%q, Updates=%q", loadedA.Name, loadedB.Name)
	}
	if loadedA.Description != loadedB.Description {
		t.Errorf("Description: PatchAndSave=%q, Updates=%q", loadedA.Description, loadedB.Description)
	}
	if loadedA.Age != loadedB.Age {
		t.Errorf("Age: PatchAndSave=%d, Updates=%d", loadedA.Age, loadedB.Age)
	}
	if loadedA.Status != loadedB.Status {
		t.Errorf("Status: PatchAndSave=%q, Updates=%q", loadedA.Status, loadedB.Status)
	}
}

// TestPatchAndSave_NoNovel 验证 PatchAndSaveNoNovel
func TestPatchAndSave_NoNovel(t *testing.T) {
	db := setupPatchTestDB(t)

	original := TestPatchEntity{
		NovelID:     1,
		Name:        "eve",
		Description: "desc",
		Age:         50,
		Status:      "active",
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	input := TestPatchInput{Name: "eve_new"}
	var loaded TestPatchEntity
	// 注意：PatchAndSaveNoNovel 不检查 novel_id，只用 id
	if err := PatchAndSaveNoNovel(db, original.ID, &input, &loaded); err != nil {
		t.Fatalf("PatchAndSaveNoNovel: %v", err)
	}

	if loaded.Name != "eve_new" {
		t.Errorf("Name: expected 'eve_new', got %q", loaded.Name)
	}
	if loaded.Description != "desc" {
		t.Errorf("Description: expected 'desc', got %q", loaded.Description)
	}
}

// TestPatchAndSave_NotFound 验证实体不存在时返回错误
func TestPatchAndSave_NotFound(t *testing.T) {
	db := setupPatchTestDB(t)

	input := TestPatchInput{Name: "ghost"}
	var loaded TestPatchEntity
	err := PatchAndSave(db, 99999, 1, &input, &loaded)
	if err == nil {
		t.Error("expected error for non-existent entity")
	}
}
