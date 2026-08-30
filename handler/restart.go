package handler

import (
	"sync"
	"time"
)

// OnRestart 由 main 包注册的重启回调。保存后台路径变化后调用，
// 触发进程自重启（延迟执行，给当前响应留出返回时间）。
var (
	restartMu  sync.Mutex
	onRestart  func()
	restartOnce sync.Once
)

// SetRestartHook 注册重启回调（main 包启动时调用）。
func SetRestartHook(fn func()) {
	restartMu.Lock()
	onRestart = fn
	restartMu.Unlock()
}

// ScheduleRestart 延迟 delay 后触发一次重启。
// 用于后台路径修改后自动生效：先返回响应给浏览器，再重启进程。
func ScheduleRestart(delay time.Duration) {
	restartMu.Lock()
	fn := onRestart
	restartMu.Unlock()
	if fn == nil {
		return
	}

	// 用 sync.Once 保证同一进程内只触发一次重启，避免连续保存重复拉起多个进程
	restartOnce.Do(func() {
		go func() {
			time.Sleep(delay)
			fn()
		}()
	})
}
