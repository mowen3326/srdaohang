//go:build !windows

package main

import "syscall"

// restartSysProcAttr Linux/macOS 等平台使用默认属性，进程由 systemd 或 shell 管理。
func restartSysProcAttr() *syscall.SysProcAttr {
	return nil
}
