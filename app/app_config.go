package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"novel/internal/config"
	"novel/internal/storage"

	"golang.org/x/image/font/sfnt"
)

func fontFamilyFromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	font, err := sfnt.Parse(f)
	if err != nil {
		return ""
	}
	name, err := font.Name(nil, sfnt.NameIDFamily)
	if err != nil {
		return ""
	}
	return name
}

func (a *App) GetAppConfig() map[string]any {
	if a.cfg == nil {
		return map[string]any{"initialized": false}
	}
	return map[string]any{
		"initialized": true,
		"data_dir":    config.DataDirPath(),
	}
}

func (a *App) UpdateDataDir(newPath string) error {
	if newPath == "" {
		return fmt.Errorf("数据目录路径不能为空")
	}
	if err := config.Save(newPath); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	if a.db != nil {
		if err := storage.Close(a.db); err != nil {
			return fmt.Errorf("关闭旧数据库失败: %w", err)
		}
		a.db = nil
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载新配置失败: %w", err)
	}
	a.initWithConfig(cfg)
	a.logger.Info("数据目录已更改", "data_dir", config.DataDirPath())
	return nil
}

func (a *App) GetSystemFonts() []string {
	fontDirs := []string{
		filepath.Join(os.Getenv("WINDIR"), "Fonts"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"),
	}
	seen := map[string]bool{}
	var result []string
	for _, dir := range fontDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
				continue
			}
			family := fontFamilyFromFile(path)
			if family == "" {
				family = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
				family = strings.ReplaceAll(family, "-", " ")
				family = strings.ReplaceAll(family, "_", " ")
				family = strings.TrimSpace(family)
			}
			if family == "" || seen[family] {
				continue
			}
			seen[family] = true
			result = append(result, family)
		}
	}
	sort.Strings(result)
	if len(result) == 0 {
		result = []string{"KaiTi", "SimSun", "SimHei", "Microsoft YaHei", "FangSong"}
	}
	return result
}

func (a *App) GetPlatform() map[string]any {
	return map[string]any{
		"os":          runtime.GOOS,
		"defaultPath": config.DataDirPath(),
	}
}