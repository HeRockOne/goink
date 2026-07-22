package app

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"novel/internal/config"
)

// BackupData 创建数据备份 zip 文件。
// 返回备份文件路径。
func (a *App) BackupData() (string, error) {
	dataDir := config.DataDirPath()
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	backupPath := filepath.Join(dataDir, fmt.Sprintf("goink-backup-%s.zip", timestamp))

	zipFile, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("创建备份文件失败: %w", err)
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	// 1. 备份数据库
	dbPath := config.GlobalDBPath()
	if err := addFileToZip(w, dbPath, "novel-agent.db", dataDir); err != nil {
		a.logger.Warn("备份数据库失败", "err", err)
	}

	// 2. 备份 novels 目录
	novelsDir := filepath.Join(dataDir, "novels")
	if err := addDirToZip(w, novelsDir, "novels", dataDir); err != nil {
		a.logger.Warn("备份 novels 目录失败", "err", err)
	}

	// 3. 备份用户技能目录
	skillsDir := config.UserSkillsDir()
	if _, err := os.Stat(skillsDir); err == nil {
		if err := addDirToZip(w, skillsDir, "skills", dataDir); err != nil {
			a.logger.Warn("备份用户技能失败", "err", err)
		}
	}

	// 4. 备份 config.json
	configPath := filepath.Join(dataDir, "..", ".config", "goink", "config.json")
	if _, err := os.Stat(configPath); err == nil {
		if err := addFileToZip(w, configPath, "config.json", filepath.Dir(configPath)); err != nil {
			a.logger.Warn("备份 config.json 失败", "err", err)
		}
	}

	if err := w.Close(); err != nil {
		return "", fmt.Errorf("完成备份失败: %w", err)
	}

	info, _ := os.Stat(backupPath)
	sizeMB := float64(0)
	if info != nil {
		sizeMB = float64(info.Size()) / 1024 / 1024
	}

	a.logger.Info("备份完成", "path", backupPath, "size_mb", fmt.Sprintf("%.1f", sizeMB))
	return backupPath, nil
}

// RestoreData 从 zip 文件恢复数据。
// 会先备份当前数据，然后恢复。
func (a *App) RestoreData(backupPath string) error {
	// 检查备份文件存在
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("备份文件不存在: %s", backupPath)
	}

	dataDir := config.DataDirPath()

	// 先备份当前数据（安全网）
	safetyBackup := filepath.Join(dataDir, fmt.Sprintf("goink-pre-restore-%s.zip", time.Now().Format("2006-01-02T15-04-05")))
	zipFile, err := os.Create(safetyBackup)
	if err == nil {
		w := zip.NewWriter(zipFile)
		dbPath := config.GlobalDBPath()
		addFileToZip(w, dbPath, "novel-agent.db", dataDir)
		addDirToZip(w, filepath.Join(dataDir, "novels"), "novels", dataDir)
		w.Close()
		zipFile.Close()
		a.logger.Info("恢复前安全备份", "path", safetyBackup)
	}

	// 打开备份 zip
	r, err := zip.OpenReader(backupPath)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer r.Close()

	// 逐个解压
	for _, f := range r.File {
		// 安全检查：防止路径穿越
		if strings.Contains(f.Name, "..") {
			continue
		}

		targetPath := filepath.Join(dataDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}

		// 确保父目录存在
		os.MkdirAll(filepath.Dir(targetPath), 0755)

		// 解压文件
		outFile, err := os.Create(targetPath)
		if err != nil {
			a.logger.Warn("创建文件失败", "path", targetPath, "err", err)
			continue
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			continue
		}

		io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
	}

	a.logger.Info("恢复完成", "from", backupPath)
	return nil
}

// GetBackupList 列出所有备份文件。
func (a *App) GetBackupList() []map[string]any {
	dataDir := config.DataDirPath()
	var backups []map[string]any

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return backups
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "goink-backup-") && strings.HasSuffix(name, ".zip") {
			info, _ := entry.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			backups = append(backups, map[string]any{
				"name": name,
				"path": filepath.Join(dataDir, name),
				"size": size,
				"time": entry.Name()[len("goink-backup-") : len(name)-4],
			})
		}
	}

	return backups
}

// WriteTempFile 将前端上传的文件写入临时目录，返回路径。
func (a *App) WriteTempFile(filename string, data []byte) (string, error) {
	dataDir := config.DataDirPath()
	tmpDir := filepath.Join(dataDir, "tmp")
	os.MkdirAll(tmpDir, 0755)

	tmpPath := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	return tmpPath, nil
}

func addFileToZip(w *zip.Writer, filePath, zipPath, baseDir string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	writer, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	return err
}

func addDirToZip(w *zip.Writer, dirPath, zipPrefix, baseDir string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// 跳过 runtime 目录（可重新下载）
		rel, _ := filepath.Rel(baseDir, path)
		if strings.HasPrefix(rel, "runtime") || strings.HasPrefix(rel, "cache") {
			return filepath.SkipDir
		}

		// 跳过备份文件本身
		if strings.HasSuffix(rel, ".zip") && strings.HasPrefix(filepath.Base(rel), "goink-") {
			return nil
		}

		zipPath := filepath.Join(zipPrefix, rel)
		zipPath = filepath.ToSlash(zipPath)

		if info.IsDir() {
			header := &zip.FileHeader{
				Name:   zipPath + "/",
				Method: zip.Store,
			}
			w.CreateHeader(header)
			return nil
		}

		return addFileToZip(w, path, zipPath, baseDir)
	})
}
