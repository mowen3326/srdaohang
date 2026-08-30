package handler

import "strings"

// 后台访问路径前缀。
//
// 默认 "/admin"，可在后台「网站设置」中自定义（存于 settings 表的 admin_path 键），
// 修改后需重启服务生效。main.go 启动时通过 SetAdminBase 设置。
//
// 由于该值在服务运行期间保持不变（改路径需重启），使用包级变量即可安全地
// 被所有 handler 的跳转逻辑与模板渲染读取，无需为每个请求传递。
var adminBase = "/admin"

// SetAdminBase 设置后台路径前缀。传入值应为不含末尾斜杠的 "/xxx" 形式，
// 未设置或非法时回退到默认 "/admin"。
func SetAdminBase(p string) {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		adminBase = "/admin"
		return
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	adminBase = strings.TrimRight(p, "/")
}

// AdminBase 返回当前后台路径前缀（如 "/admin"）。
func AdminBase() string { return adminBase }

// AdminURL 拼接后台路径，返回完整路径（如 "/admin/login"）。
// suffix 需以 "/" 开头，如 "/login"、"/categories"；传空串返回前缀本身。
func AdminURL(suffix string) string {
	if suffix == "" {
		return adminBase
	}
	return adminBase + suffix
}
