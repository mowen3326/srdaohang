package model

import (
	"hash/fnv"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Category 网址分类，Links 为组装后的子链接（不映射到数据库字段）。
type Category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
	Links     []Link `json:"links,omitempty"`
}

// Background 一张前台背景图记录。
type Background struct {
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	Original string `json:"original"`
	Active   bool   `json:"active"`
}

// LoginLog 一条登录审计日志记录。
type LoginLog struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// Link 一条网址记录。
type Link struct {
	ID          int64  `json:"id"`
	CategoryID  int64  `json:"category_id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
}

// 图标类型常量
const (
	IconKindURL  = "url"  // 图片地址
	IconKindText = "text" // Emoji 或短文字
	IconKindAuto = "auto" // 留空，自动取站点 favicon（分类无 favicon，留空即不显示）
	IconKindFA   = "fa"   // Font Awesome 图标（class 名，如 fa-solid fa-house）
)

// iconKind 判断图标字段的类型：
//   - 留空 → auto
//   - http(s):// 或 / 开头 → url（图片地址）
//   - fa- 开头（Font Awesome class）→ fa
//   - 其他 → text（Emoji 或短文字）
func iconKind(icon string) string {
	ic := strings.TrimSpace(icon)
	switch {
	case ic == "":
		return IconKindAuto
	case strings.HasPrefix(ic, "http://"), strings.HasPrefix(ic, "https://"), strings.HasPrefix(ic, "/"):
		return IconKindURL
	case isFontAwesome(ic):
		return IconKindFA
	default:
		return IconKindText
	}
}

// isFontAwesome 判断是否为 Font Awesome class（以 fa- 开头，如 fa-solid、fa-brands、fa-regular、fa-house）。
func isFontAwesome(ic string) bool {
	return strings.HasPrefix(ic, "fa-") ||
		strings.HasPrefix(ic, "fa-solid ") ||
		strings.HasPrefix(ic, "fa-brands ") ||
		strings.HasPrefix(ic, "fa-regular ")
}

// IconKind 分类图标类型。
func (c Category) IconKind() string { return iconKind(c.Icon) }

// IconText 分类图标的文字内容（Emoji）。
func (c Category) IconText() string { return strings.TrimSpace(c.Icon) }

// IconSrc 分类图标的图片地址；仅当为图片地址类型时返回，否则为空。
func (c Category) IconSrc() string {
	ic := strings.TrimSpace(c.Icon)
	if ic != "" && (strings.HasPrefix(ic, "http://") || strings.HasPrefix(ic, "https://") || strings.HasPrefix(ic, "/")) {
		return ic
	}
	return ""
}

// IconFA 分类图标的 Font Awesome class；仅当为 FA 类型时返回，否则为空。
func (c Category) IconFA() string {
	ic := strings.TrimSpace(c.Icon)
	if isFontAwesome(ic) {
		return ic
	}
	return ""
}

// IconKind 链接图标类型。
func (l Link) IconKind() string { return iconKind(l.Icon) }

// IconText 链接图标的文字内容（Emoji）。
func (l Link) IconText() string { return strings.TrimSpace(l.Icon) }

// IconFA 链接图标的 Font Awesome class；仅当为 FA 类型时返回，否则为空。
func (l Link) IconFA() string {
	ic := strings.TrimSpace(l.Icon)
	if isFontAwesome(ic) {
		return ic
	}
	return ""
}

// IconSrc 返回可直接用于 <img src> 的地址：优先自定义图标，否则回退站点 favicon。
func (l Link) IconSrc() string {
	ic := strings.TrimSpace(l.Icon)
	if ic != "" && (strings.HasPrefix(ic, "http://") || strings.HasPrefix(ic, "https://") || strings.HasPrefix(ic, "/")) {
		return ic
	}
	return l.Favicon()
}

// Favicon 由网址推导出站点 favicon 地址，解析失败返回空串。
func (l Link) Favicon() string {
	u, err := url.Parse(strings.TrimSpace(l.URL))
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Host + "/favicon.ico"
}

// Initial 取标题首字符，用于图标加载失败时的降级色块。
func (l Link) Initial() string {
	t := strings.TrimSpace(l.Title)
	if t == "" {
		return "#"
	}
	r, _ := utf8.DecodeRuneInString(t)
	return strings.ToUpper(string(r))
}

// Hue 根据标题与网址生成稳定的色相值（0-359），保证每次渲染颜色一致。
func (l Link) Hue() int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(l.Title + "|" + l.URL))
	return int(h.Sum32() % 360)
}

// Host 返回网址的主机名，用于卡片副标题展示。
func (l Link) Host() string {
	u, err := url.Parse(strings.TrimSpace(l.URL))
	if err != nil {
		return ""
	}
	return u.Host
}
