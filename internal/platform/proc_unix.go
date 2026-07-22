//go:build !windows

package platform

import "os/exec"

// SetPlatformAttr 为子进程设置平台相关属性（非 Windows 平台为空操作）。
func SetPlatformAttr(*exec.Cmd) {}
