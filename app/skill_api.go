package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"novel/internal/config"
	"novel/internal/git"
	"novel/internal/skill"
)

// ListSkillsInput 是 ListSkills 的入参。
type ListSkillsInput struct {
	NovelID int64 `json:"novel_id"`
}

// ListSkills 返回所有可用 skill 的元数据（同名覆盖：novel > user > builtin）。
func (a *App) ListSkills(input ListSkillsInput) []skill.SkillMeta {
	if a.skill == nil {
		return nil
	}
	return a.skill.ListMeta(input.NovelID)
}

// DeleteSkillInput 是 DeleteSkill 的入参。
type DeleteSkillInput struct {
	NovelID int64  `json:"novel_id"`
	Name    string `json:"name"`
	Source  string `json:"source"` // "novel" | "user"
}

// DeleteSkill 删除用户级或小说级技能文件。内置技能不可删除。
func (a *App) DeleteSkill(input DeleteSkillInput) error {
	if a.skill == nil {
		return fmt.Errorf("skill store 未初始化")
	}
	if input.Name == "" {
		return fmt.Errorf("技能名称不能为空")
	}
	name := strings.TrimSuffix(filepath.Base(input.Name), ".md")
	if name == "" || name != input.Name {
		return fmt.Errorf("技能名称非法")
	}

	source := input.Source
	if source != "novel" && source != "user" {
		return fmt.Errorf("只能删除用户级或小说级技能")
	}

	var filePath string
	switch source {
	case "novel":
		if input.NovelID <= 0 {
			return fmt.Errorf("小说 ID 无效")
		}
		var err error
		filePath, err = git.SafePath(config.NovelSkillsDir(input.NovelID), name+".md")
		if err != nil {
			return fmt.Errorf("技能名称非法: %w", err)
		}
	case "user":
		var err error
		filePath, err = git.SafePath(config.UserSkillsDir(), name+".md")
		if err != nil {
			return fmt.Errorf("技能名称非法: %w", err)
		}
	}

	if err := os.Remove(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("技能文件不存在: %s", name)
		}
		return fmt.Errorf("删除技能文件失败: %w", err)
	}

	// 重新加载对应层级
	switch source {
	case "novel":
		if err := a.skill.ReloadNovel(input.NovelID, config.NovelSkillsDir(input.NovelID)); err != nil {
			a.logger.Warn("删除技能后重新加载小说级技能失败", "name", name, "err", err)
		}
	case "user":
		if err := a.skill.ReloadUser(config.UserSkillsDir()); err != nil {
			a.logger.Warn("删除技能后重新加载用户级技能失败", "name", name, "err", err)
		}
	}

	return nil
}
