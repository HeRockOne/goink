package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// AppDir 返回当前可执行文件所在的目录。
func AppDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("platform: 获取可执行文件路径失败: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("platform: 解析可执行文件符号链接失败: %w", err)
	}
	return filepath.Dir(exe), nil
}

// ResolveGit 返回 git 可执行文件的路径。
// 搜索顺序: app 自带 runtime/git/ → 用户数据目录 runtime/git/ → 系统 PATH。
// 每个候选路径都会验证可执行性（git --version），不可用则跳过继续 fallback。
//
// 当环境变量 GOINK_TESTING=1 时，仅从 DataDir 的 bundled 路径查找，
// 不 fallback 到系统 PATH，找不到直接报错。用于 E2E 测试确保使用 bundled git。
func ResolveGit() (string, error) {
	// GOINK_TESTING 模式：只查 DataDir bundled 路径，不做任何 fallback
	if os.Getenv("GOINK_TESTING") != "" {
		path := dataDirBundledGitPath()
		if verifyGit(path) == nil {
			return path, nil
		}
		return "", fmt.Errorf("git: GOINK_TESTING 模式下未找到 bundled git (%s)", path)
	}

	// 1. app 自带的 bundled git
	if appDir, err := AppDir(); err == nil {
		if path := bundledGitPath(appDir); verifyGit(path) == nil {
			return path, nil
		}
	}

	// 2. 用户数据目录下的 runtime/git/ (开发模式或手动安装)
	dataGit := filepath.Join(DataDir(), "runtime", "git", gitBinName())
	if verifyGit(dataGit) == nil {
		return dataGit, nil
	}

	// 3. 系统 PATH
	if path, err := exec.LookPath("git"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("git: 找不到可用的 git 可执行文件，请安装 Git")
}

// dataDirBundledGitPath 返回 DataDir 下 bundled git 的路径，
// 使用与生产环境相同的目录结构（Windows 为 MinGit 的 mingw64/bin/git.exe）。
func dataDirBundledGitPath() string {
	dataDir := DataDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(dataDir, "runtime", "git", "mingw64", "bin", "git.exe")
	default:
		return filepath.Join(dataDir, "runtime", "git", "git")
	}
}

// verifyGit 验证 git 可执行文件是否存在且能正常运行。
func verifyGit(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	cmd := exec.Command(path, "--version")
	SetPlatformAttr(cmd)
	return cmd.Run()
}

// gitBinName 返回当前平台 git 二进制文件名。
func gitBinName() string {
	if runtime.GOOS == "windows" {
		return "git.exe"
	}
	return "git"
}

// ResolveOnnxLib 返回 ONNX Runtime 动态库的路径。
// 优先 app 自带的 runtime/，然后用户数据目录 runtime/，最后系统路径。
//
// 当环境变量 GOINK_TESTING=1 时，仅从 DataDir 的 runtime/ 查找，
// 不 fallback 到系统路径，找不到直接报错。用于 E2E 测试确保使用 bundled ONNX。
func ResolveOnnxLib() (string, error) {
	libName := onnxLibName()

	// GOINK_TESTING 模式：只查 DataDir runtime 路径，不做任何 fallback
	if os.Getenv("GOINK_TESTING") != "" {
		p := filepath.Join(DataDir(), "runtime", libName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("platform: GOINK_TESTING 模式下未找到 ONNX Runtime (%s)", p)
	}

	appDir, err := AppDir()
	if err == nil {
		for _, dir := range bundledRuntimeDirs(appDir) {
			p := filepath.Join(dir, libName)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	dataRuntime := filepath.Join(DataDir(), "runtime", libName)
	if _, err := os.Stat(dataRuntime); err == nil {
		return dataRuntime, nil
	}

	for _, p := range systemOnnxPaths(libName) {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("platform: ONNX Runtime 库未找到（%s）", libName)
}

// DataDir 返回应用数据目录（绝对路径）。
// Windows 返回可执行文件所在目录（单目录安装模式），其他平台返回 ~/Goink/。
// 开发模式下 exe 位于临时目录时，所有平台统一返回 ~/Goink/。
// 环境变量 GOINK_DATA_DIR 可覆盖以上逻辑，用于集成测试。
func DataDir() string {
	if dir := os.Getenv("GOINK_DATA_DIR"); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		if dir, err := AppDir(); err == nil {
			tmp := os.TempDir()
			if !strings.HasPrefix(strings.ToLower(dir), strings.ToLower(tmp)) {
				return dir
			}
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Goink")
}

// bundledGitPath 返回自带的 git 完整路径。
func bundledGitPath(appDir string) string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(appDir, "runtime", "git", "mingw64", "bin", "git.exe")
	case "darwin":
		// macOS .app bundle 中 runtime 在 Contents/Resources/ 下
		return filepath.Join(appDir, "..", "Resources", "runtime", "git", "git")
	default:
		return filepath.Join(appDir, "runtime", "git", "git")
	}
}

// bundledRuntimeDirs 返回自带的 runtime 目录列表，按优先级排列。
func bundledRuntimeDirs(appDir string) []string {
	switch runtime.GOOS {
	case "darwin":
		// macOS .app bundle: runtime 在 Contents/Resources/，
		// AppDir 返回 Contents/MacOS/，所以用 ../Resources/runtime/
		return []string{
			filepath.Join(appDir, "..", "Resources", "runtime"),
			filepath.Join(appDir, "runtime"),
		}
	default:
		return []string{filepath.Join(appDir, "runtime")}
	}
}

// BundledModelsDir 返回打包自带的模型目录路径（绝对路径）。
func BundledModelsDir(appDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(appDir, "..", "Resources", "runtime", "models")
	default:
		return filepath.Join(appDir, "runtime", "models")
	}
}

func onnxLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "libonnxruntime.dylib"
	default:
		return "libonnxruntime.so"
	}
}

func systemOnnxPaths(lib string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/usr/local/lib/" + lib,
			"/usr/lib/" + lib,
		}
	default:
		return []string{
			"/usr/lib/" + lib,
			"/usr/local/lib/" + lib,
		}
	}
}
