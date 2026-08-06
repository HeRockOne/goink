package app

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"novel/internal/config"
	"novel/internal/storage"
)

// fontFamilyFromFile 读取字体文件元数据中的家族名（NameID=1），
// 优先 Windows 简体中文（zh-CN）名称，其次繁体/英文，支持 .ttc 集合。
// 解析失败返回空串。
func fontFamilyFromFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) < 4 {
		return ""
	}
	if string(data[:4]) == "ttcf" {
		if len(data) < 16 {
			return ""
		}
		numFonts := int(binary.BigEndian.Uint32(data[8:]))
		for i := 0; i < numFonts; i++ {
			off := int(binary.BigEndian.Uint32(data[12+i*4:]))
			if f := familyNameAt(data, off); f != "" {
				return f
			}
		}
		return ""
	}
	return familyNameAt(data, 0)
}

// familyNameAt 解析 sfnt 偏移处的字体的 name 表，取最佳家族名。
func familyNameAt(data []byte, off int) string {
	if off+12 > len(data) {
		return ""
	}
	numTables := int(binary.BigEndian.Uint16(data[off+4:]))
	var nameOff int
	for i := 0; i < numTables; i++ {
		rec := off + 12 + i*16
		if rec+16 > len(data) {
			return ""
		}
		if string(data[rec:rec+4]) == "name" {
			nameOff = int(binary.BigEndian.Uint32(data[rec+8:]))
			break
		}
	}
	if nameOff == 0 || nameOff+6 > len(data) {
		return ""
	}
	count := int(binary.BigEndian.Uint16(data[nameOff+2:]))
	strOff := nameOff + int(binary.BigEndian.Uint16(data[nameOff+4:]))
	var zhTW, enUS, other string
	for i := 0; i < count; i++ {
		e := nameOff + 6 + i*12
		if e+12 > len(data) {
			break
		}
		platform := binary.BigEndian.Uint16(data[e:])
		encoding := binary.BigEndian.Uint16(data[e+2:])
		lang := binary.BigEndian.Uint16(data[e+4:])
		if binary.BigEndian.Uint16(data[e+6:]) != 1 { // 仅家族名
			continue
		}
		length := int(binary.BigEndian.Uint16(data[e+8:]))
		offset := int(binary.BigEndian.Uint16(data[e+10:]))
		start, end := strOff+offset, strOff+offset+length
		if start < 0 || end > len(data) {
			continue
		}
		var val string
		switch platform {
		case 3: // Windows: UCS-2 / UTF-16BE
			if encoding > 10 {
				continue
			}
			val = decodeUCS2(data[start:end])
		case 1: // Macintosh: 仅全 ASCII 时可用
			if isASCII(data[start:end]) {
				val = string(data[start:end])
			}
		default:
			continue
		}
		if val == "" {
			continue
		}
		if platform == 3 && lang == 0x0804 { // 简体中文优先
			return val
		}
		if platform == 3 && lang == 0x0404 && zhTW == "" {
			zhTW = val
		} else if platform == 3 && lang == 0x0409 && enUS == "" {
			enUS = val
		} else if platform == 3 && other == "" {
			other = val
		}
	}
	if zhTW != "" {
		return zhTW
	}
	if enUS != "" {
		return enUS
	}
	return other
}

func decodeUCS2(b []byte) string {
	if len(b)&1 != 0 {
		return ""
	}
	r := make([]rune, len(b)/2)
	for i := range r {
		r[i] = rune(binary.BigEndian.Uint16(b[i*2:]))
	}
	return string(r)
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
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
