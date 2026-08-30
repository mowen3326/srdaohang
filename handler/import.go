package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"daohang/model"
	"daohang/store"
)

// Bookmark 一条解析出的浏览器收藏。
type Bookmark struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Folder string `json:"folder"` // 所属文件夹路径（" / " 连接），空表示未分类
	Index  int    `json:"index"`  // 在原始列表中的索引，用于勾选导入时定位
}

// BookmarkGroup 按文件夹分组后的书签集合。
type BookmarkGroup struct {
	Folder    string     // 文件夹名，空表示「未分类」
	Bookmarks []Bookmark
}

// ImportHandler 处理浏览器收藏夹导入。
type ImportHandler struct {
	store store.Store
}

func NewImport(s store.Store) *ImportHandler {
	return &ImportHandler{store: s}
}

// 统一匹配 <H3>文件夹名</H3> 或 <A HREF="url" ...>标题</A> 或 </DL>（忽略大小写，允许属性换行）
var bookmarkRe = regexp.MustCompile(`(?is)<H3[^>]*>(.*?)</H3>|<A\s+[^>]*HREF\s*=\s*["']([^"']+)["'][^>]*>(.*?)</A>|</DL>`)

// stripTags 去掉标题中的 HTML 标签与实体占位。
var tagRe = regexp.MustCompile(`(?is)<[^>]+>`)

// parseBookmarks 解析浏览器导出的 Netscape Bookmark HTML 文件，按文件夹归类返回书签列表。
// 文件夹名支持嵌套，用栈维护层级；书签的 Folder 字段记录其所在文件夹路径（用 " / " 连接）。
func parseBookmarks(data []byte) []Bookmark {
	var out []Bookmark
	var stack []string // 文件夹层级栈

	for _, m := range bookmarkRe.FindAllSubmatch(data, -1) {
		raw := string(m[0])
		lower := strings.ToLower(raw)

		switch {
		case strings.HasPrefix(lower, "<h3"):
			// 进入文件夹
			folder := strings.TrimSpace(decodeHTMLEntities(tagRe.ReplaceAllString(string(m[1]), "")))
			if folder != "" {
				stack = append(stack, folder)
			}
		case strings.HasPrefix(lower, "</dl"):
			// 退出当前文件夹层级
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case strings.HasPrefix(lower, "<a"):
			// 书签：m[2]=url, m[3]=title
			url := strings.TrimSpace(string(m[2]))
			if url == "" || isUnsafeURL(url) {
				continue
			}
			title := tagRe.ReplaceAllString(string(m[3]), "")
			title = strings.TrimSpace(decodeHTMLEntities(title))
			if title == "" {
				title = url
			}
			folder := ""
			if len(stack) > 0 {
				folder = strings.Join(stack, " / ")
			}
			out = append(out, Bookmark{Title: title, URL: url, Folder: folder, Index: len(out)})
		}
	}
	return out
}

// isUnsafeURL 判断导入的书签 URL 是否属于危险协议。
// 拒绝 javascript:/vbscript:/data: 等可执行或非导航协议，仅放行 http/https 及站内相对地址。
func isUnsafeURL(u string) bool {
	lower := strings.ToLower(u)
	for _, p := range []string{"javascript:", "vbscript:", "data:", "file:", "about:"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// decodeHTMLEntities 解码常见 HTML 实体。
func decodeHTMLEntities(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
		"&#39;", "'", "&nbsp;", " ",
	)
	return r.Replace(s)
}

// importErrMsg 将导入错误码转为可读提示。
func importErrMsg(code string) string {
	switch code {
	case "empty":
		return "请至少勾选一条收藏后再导入"
	case "":
		return ""
	default:
		return "导入失败，请重试"
	}
}

// ImportPage 渲染导入收藏页面。
func (h *ImportHandler) ImportPage(c *echo.Context) error {
	return h.renderImport(c, nil, "", 0, importErrMsg(c.QueryParam("error")))
}

// renderImport 渲染导入页面。bookmarks 为剩余书签（nil 表示尚未解析），
// msg 为成功提示，okCount 为本次导入条数，errMsg 为错误提示。
func (h *ImportHandler) renderImport(c *echo.Context, bookmarks []Bookmark, msg string, okCount int, errMsg string) error {
	ctx := c.Request().Context()

	cats, err := h.store.ListCategories(ctx)
	if err != nil {
		return err
	}
	catCount, linkCount, err := h.store.Count(ctx)
	if err != nil {
		return err
	}
	name, err := siteName(ctx, h.store)
	if err != nil {
		return err
	}
	logoLight, logoDark, err := siteLogos(ctx, h.store)
	if err != nil {
		return err
	}

	data := map[string]any{
		"Title":         "导入收藏",
		"IsAdmin":       true,
		"CatCount":      catCount,
		"LinkCount":     linkCount,
		"SettingsOpen":  true,
		"ActiveTab":     "import",
		"SiteName":      name,
		"Logo":          logoLight,
		"LogoLight":     logoLight,
		"LogoDark":      logoDark,
		"Categories":    cats,
		"ImportMessage": msg,
		"OKCount":       okCount,
		"ImportError":   errMsg,
	}

	if bookmarks != nil {
		raw, err := json.Marshal(bookmarks)
		if err != nil {
			return err
		}
		data["Bookmarks"] = bookmarks
		data["BookmarksRaw"] = string(raw)
		data["BookmarkCount"] = len(bookmarks)
		data["BookmarkGroups"] = groupBookmarks(bookmarks)
	}

	return c.Render(http.StatusOK, "admin_import.html", data)
}

// groupBookmarks 按 Folder 分组，保持原始顺序；未分类的归入「未分类」。
func groupBookmarks(bookmarks []Bookmark) []BookmarkGroup {
	idx := map[string]int{} // 文件夹名 -> groups 下标
	groups := []BookmarkGroup{}

	for _, b := range bookmarks {
		key := b.Folder
		gi, ok := idx[key]
		if !ok {
			gi = len(groups)
			idx[key] = gi
			groups = append(groups, BookmarkGroup{Folder: key})
		}
		groups[gi].Bookmarks = append(groups[gi].Bookmarks, b)
	}

	return groups
}

// UploadImport 接收上传的 HTML 文件，解析后渲染带书签列表的页面。
func (h *ImportHandler) UploadImport(c *echo.Context) error {
	file, err := c.FormFile("bookmarks")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择浏览器导出的 HTML 收藏文件")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "读取文件失败")
	}
	defer src.Close()

	// 限制文件大小，避免异常大文件占用内存
	const maxSize = 10 << 20 // 10 MB
	buf := make([]byte, maxSize+1)
	n, err := io.ReadFull(src, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return echo.NewHTTPError(http.StatusBadRequest, "读取文件失败")
	}
	if n > maxSize {
		return echo.NewHTTPError(http.StatusBadRequest, "文件过大（超过 10MB）")
	}

	bookmarks := parseBookmarks(buf[:n])
	if len(bookmarks) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "未解析到任何书签，请确认文件是浏览器导出的 HTML 收藏夹")
	}

	return h.renderImport(c, bookmarks, "", 0, "")
}

// SaveImport 将选中的书签批量导入到目标分类，成功后保留剩余书签。
func (h *ImportHandler) SaveImport(c *echo.Context) error {
	ctx := c.Request().Context()

	catID, err := strconv.ParseInt(c.FormValue("category_id"), 10, 64)
	if err != nil || catID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择目标分类")
	}

	if err := c.Request().ParseForm(); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "表单解析失败")
	}
	// 选中的书签索引（多个同名 checkbox）
	selVals := c.Request().Form["selected"]
	if len(selVals) == 0 {
		return c.Redirect(http.StatusSeeOther, AdminURL("/import?error=empty"))
	}

	raw := c.FormValue("bookmarks")
	var bookmarks []Bookmark
	if err := json.Unmarshal([]byte(raw), &bookmarks); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "书签数据无效，请重新上传")
	}

	// 解析选中索引
	idxSet := map[int]bool{}
	for _, p := range selVals {
		if i, e := strconv.Atoi(strings.TrimSpace(p)); e == nil {
			idxSet[i] = true
		}
	}

	// 计算目标分类下现有链接的最大 sort_order，接着递增
	maxOrder := 0
	links, err := h.store.ListLinks(ctx)
	if err != nil {
		return err
	}
	for _, l := range links {
		if l.CategoryID == catID && l.SortOrder > maxOrder {
			maxOrder = l.SortOrder
		}
	}

	imported := 0
	var remaining []Bookmark
	for i, b := range bookmarks {
		if idxSet[i] {
			maxOrder++
			l := &model.Link{
				CategoryID: catID,
				Title:      b.Title,
				URL:        b.URL,
				Icon:       "", // 留空，前台自动取站点 favicon
				SortOrder:  maxOrder,
			}
			if err := h.store.CreateLink(ctx, l); err != nil {
				return err
			}
			imported++
		} else {
			// 重新编号，使 Index 始终等于剩余列表的下标
			b.Index = len(remaining)
			remaining = append(remaining, b)
		}
	}

	msg := "成功导入 " + strconv.Itoa(imported) + " 条收藏"
	if len(remaining) == 0 {
		msg += "，全部收藏已导入完成"
		return h.renderImport(c, nil, msg, imported, "")
	}
	return h.renderImport(c, remaining, msg, imported, "")
}
