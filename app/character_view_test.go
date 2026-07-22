package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCharacters_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	chars, err := app.GetCharacters(novelID)
	require.NoError(t, err)
	assert.Empty(t, chars)
}

func TestCreateCharacter(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	char, err := app.CreateCharacter(novelID, CreateCharacterInput{
		Name:        "张三",
		Description: "主角",
		Personality: `{"traits":["勇敢","冲动"]}`,
		Abilities:   `["剑术","隐身"]`,
	})
	require.NoError(t, err)
	assert.NotZero(t, char.ID)
	assert.Equal(t, novelID, char.NovelID)
	assert.Equal(t, "张三", char.Name)
	assert.Equal(t, "主角", char.Description)
	assert.Equal(t, `{"traits":["勇敢","冲动"]}`, char.Personality)
	assert.Equal(t, `["剑术","隐身"]`, char.Abilities)
}

func TestCreateCharacter_EmptyName(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateCharacter(novelID, CreateCharacterInput{
		Name: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "角色名称不能为空")
}

func TestGetCharacters_AfterCreate(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateCharacter(novelID, CreateCharacterInput{Name: "张三"})
	require.NoError(t, err)
	_, err = app.CreateCharacter(novelID, CreateCharacterInput{Name: "李四"})
	require.NoError(t, err)

	chars, err := app.GetCharacters(novelID)
	require.NoError(t, err)
	assert.Len(t, chars, 2)

	names := map[string]bool{}
	for _, c := range chars {
		names[c.Name] = true
	}
	assert.True(t, names["张三"])
	assert.True(t, names["李四"])
}

func TestUpdateCharacter(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	char, err := app.CreateCharacter(novelID, CreateCharacterInput{
		Name:        "旧名",
		Description: "旧描述",
	})
	require.NoError(t, err)

	err = app.UpdateCharacter(novelID, char.ID, UpdateCharacterInput{
		Name:        "新名",
		Description: "新描述",
	})
	require.NoError(t, err)

	chars, err := app.GetCharacters(novelID)
	require.NoError(t, err)
	require.Len(t, chars, 1)
	assert.Equal(t, "新名", chars[0].Name)
	assert.Equal(t, "新描述", chars[0].Description)
}

func TestDeleteCharacter(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	char, err := app.CreateCharacter(novelID, CreateCharacterInput{Name: "张三"})
	require.NoError(t, err)

	err = app.DeleteCharacter(novelID, char.ID)
	require.NoError(t, err)

	chars, err := app.GetCharacters(novelID)
	require.NoError(t, err)
	assert.Empty(t, chars)
}

func TestGetCharacterRelations(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	rels, err := app.GetCharacterRelations(novelID)
	require.NoError(t, err)
	assert.Empty(t, rels)
}
