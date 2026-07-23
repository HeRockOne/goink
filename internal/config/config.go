package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"novel/internal/platform"
)

var (
	globalCfg *AppConfig
	cfgMu     sync.RWMutex
)

// Set 设置全局配置单例，InitWithConfig 成功后调用。
func Set(cfg *AppConfig) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	globalCfg = cfg
}

// Get 返回全局配置单例，未初始化时返回 nil。
func Get() *AppConfig {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return globalCfg
}

// AppConfig 保留用于未来扩展。
type AppConfig struct {
	DataDir string `json:"data_dir"` // 保留字段，数据目录由 exe 位置决定
}

// DataDirPath 返回数据根目录（即 exe 所在目录）。
func DataDirPath() string {
	return platform.DataDir()
}

// GlobalDBPath 返回全局数据库路径。
func GlobalDBPath() string {
	return filepath.Join(platform.DataDir(), "novel-agent.db")
}

// NovelDirPath 返回指定小说的 Git 仓库根目录。
func NovelDirPath(novelID int64) string {
	return filepath.Join(platform.DataDir(), "novels", fmt.Sprintf("%d", novelID))
}

// LLMConfigPath 返回 LLM 加密配置文件的固定路径 ~/.goink/llm_config.enc。
func LLMConfigPath() string {
	dir, _ := configDir()
	return filepath.Join(dir, "llm_config.enc")
}

// UserSkillsDir 返回用户级 skill 目录 ~/.goink/skills/。
func UserSkillsDir() string {
	dir, _ := configDir()
	return filepath.Join(dir, "skills")
}

// NovelSkillsDir 返回指定小说的 skill 目录。
func NovelSkillsDir(novelID int64) string {
	return filepath.Join(NovelDirPath(novelID), "skills")
}

// StyleSamplesDir 返回全局风格素材目录 ~/.goink/style_samples/。
func StyleSamplesDir() string {
	dir, _ := configDir()
	return filepath.Join(dir, "style_samples")
}

// ModelsDir 返回 ONNX 模型目录路径。
// 优先查安装包自带的 runtime/models/，找不到再 fallback 到用户数据目录。
func ModelsDir() string {
	appDir, err := platform.AppDir()
	if err == nil {
		bundled := platform.BundledModelsDir(appDir)
		if _, err := os.Stat(filepath.Join(bundled, "model.onnx")); err == nil {
			return bundled
		}
	}
	return filepath.Join(platform.DataDir(), "models")
}

// configDir 返回用户级配置目录 ~/.goink。
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(home, ".goink"), nil
}

// Load 返回空配置。数据目录由 exe 位置决定，不需要 config.json。
func Load() (*AppConfig, error) {
	dataDir := platform.DataDir()
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("创建数据目录 %s 失败: %w", dataDir, err)
	}
	return &AppConfig{}, nil
}

// Save 保留接口兼容性，不再写入 config.json。
func Save(dataDir string) error {
	return nil
}
