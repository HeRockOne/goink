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
)

// GetAppConfig 返回当前运行时配置信息（供前端诊断）。
func (a *App) GetAppConfig() map[string]any {
	if a.cfg == nil {
		return map[string]any{"initialized": false}
	}
	return map[string]any{
		"initialized": true,
		"data_dir":    config.DataDirPath(),
	}
}

// UpdateDataDir 更改数据目录并重新初始化所有运行时模块。
//
// TODO: 实现数据迁移——更改目录时自动将旧目录中的数据文件移动到新目录。
// 同盘用 os.Rename（原子），跨盘用递归拷贝+进度回调，目标非空时弹确认框。
func (a *App) UpdateDataDir(newPath string) error {
	if newPath == "" {
		return fmt.Errorf("数据目录路径不能为空")
	}

	// 先保存新配置，失败时旧 DB 仍可用
	if err := config.Save(newPath); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	// 关闭旧数据库
	if a.db != nil {
		if err := storage.Close(a.db); err != nil {
			return fmt.Errorf("关闭旧数据库失败: %w", err)
		}
		a.db = nil
	}

	// 重新加载并初始化
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载新配置失败: %w", err)
	}

	a.initWithConfig(cfg)
	a.logger.Info("数据目录已更改", "data_dir", config.DataDirPath())
	return nil
}

// GetSystemFonts 返回系统已安装字体列表（Windows 读取 C:\Windows\Fonts）。
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
			name := e.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
				continue
			}
			// 从文件名推断字体族名：去掉扩展名，替换连字符/下划线为空格
			family := strings.TrimSuffix(name, filepath.Ext(name))
			family = strings.ReplaceAll(family, "-", " ")
			family = strings.ReplaceAll(family, "_", " ")
			family = strings.TrimSpace(family)
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

// GetPlatform 返回平台信息，供前端决定默认路径等行为。
func (a *App) GetPlatform() map[string]any {
	return map[string]any{
		"os":          runtime.GOOS,
		"defaultPath": config.DataDirPath(),
	}
}
