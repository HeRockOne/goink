package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetReaderPerspectives_Empty(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	assert.Empty(t, perspectives)
}

func TestCreateReaderPerspective(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	p, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:           "known",
		Content:        "The protagonist is an orphan",
		PlantedChapter: 1,
		RelatedTruth:   "Parents are alive and in hiding",
	})
	require.NoError(t, err)
	assert.Equal(t, "known", p.Type)
	assert.Equal(t, "The protagonist is an orphan", p.Content)
	assert.Equal(t, 1, p.PlantedChapter)
	assert.Equal(t, "Parents are alive and in hiding", p.RelatedTruth)
	assert.Equal(t, novelID, p.NovelID)
}

func TestCreateReaderPerspective_EmptyFields(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:    "",
		Content: "Some content",
	})
	assert.Error(t, err)

	_, err = app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:    "suspense",
		Content: "",
	})
	assert.Error(t, err)
}

func TestGetReaderPerspectives_AfterCreate(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	_, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:           "known",
		Content:        "Reader knows the hero's name",
		PlantedChapter: 1,
	})
	require.NoError(t, err)

	_, err = app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:           "suspense",
		Content:        "Who is the traitor?",
		PlantedChapter: 3,
	})
	require.NoError(t, err)

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	assert.Len(t, perspectives, 2)
}

func TestUpdateReaderPerspective(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	p, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:           "suspense",
		Content:        "Who killed the king?",
		PlantedChapter: 2,
	})
	require.NoError(t, err)

	err = app.UpdateReaderPerspective(p.ID, novelID, UpdateReaderPerspectiveInput{
		RevealedChapter: 8,
		Content:         "Who killed the king? (resolved)",
	})
	require.NoError(t, err)

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	require.Len(t, perspectives, 1)
	assert.Equal(t, 8, perspectives[0].RevealedChapter)
	assert.Equal(t, "Who killed the king? (resolved)", perspectives[0].Content)
}

func TestDeleteReaderPerspective(t *testing.T) {
	app := setupTestApp(t)
	novel := createTestNovel(t, app)
	novelID := novel.ID

	p, err := app.CreateReaderPerspective(novelID, CreateReaderPerspectiveInput{
		Type:           "misconception",
		Content:        "Reader thinks the mentor is good",
		PlantedChapter: 1,
	})
	require.NoError(t, err)

	err = app.DeleteReaderPerspective(p.ID, novelID)
	require.NoError(t, err)

	perspectives, err := app.GetReaderPerspectives(novelID)
	require.NoError(t, err)
	assert.Empty(t, perspectives)
}
