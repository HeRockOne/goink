//go:build windows

package platform

import (
	"os/exec"
	"syscall"
)

// SetPlatformAttr 为子进程设置平台相关属性（Windows 上隐藏控制台窗口）。
func SetPlatformAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
}
