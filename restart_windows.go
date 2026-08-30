//go:build windows

package main

import "syscall"

// restartSysProcAttr Windows 下新进程使用独立控制台并脱离父进程，
// 避免父进程退出时子进程被一并关闭。
func restartSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}
