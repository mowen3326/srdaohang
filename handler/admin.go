package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"daohang/model"
	"daohang/store"
)

// 后台标签页锚点。
//
// 增删改完成后重定向到带锚点的地址，前端会据此切回对应标签，
// 避免每次操作都跳回概览页。锚点名与模板中的 data-panel 保持一致。
// 由于后台路径前缀可自定义，这里用函数动态拼接。
func anchorCategories() string { return AdminURL("#categories") }
func anchorLinks() string      { return AdminURL("#links") }

// anchorLinksFiltered 构造带筛选状态的网址管理锚点地址。
// 保留分类筛选（link_cat）与搜索关键词（link_q），使删除后停留在原筛选视图。
func anchorLinksFiltered(c *echo.Context) string {
	q := url.Values{}
	if cat := strings.TrimSpace(c.FormValue("link_cat")); cat != "" {
		q.Set("link_cat", cat)
	}
	if kw := strings.TrimSpace(c.FormValue("link_q")); kw != "" {
		q.Set("link_q", kw)
	}
	if raw := q.Encode(); raw != "" {
		return AdminURL("?" + raw + "#links")
	}
	return anchorLinks()
}

// AdminHandler 处理后台的增删改查。
type AdminHandler struct {
	store store.Store
}

func NewAdmin(s store.Store) *AdminHandler {
	return &AdminHandler{store: s}
}

// 后台列表每页条数。
const pageSize = 20

// Dashboard 后台主页，同时提供分类与链接列表（分页）。
func (h *AdminHandler) Dashboard(c *echo.Context) error {
	ctx := c.Request().Context()

	catPage := queryInt(c, "cat_page", 1)
	linkPage := queryInt(c, "link_page", 1)
	catQ := c.QueryParam("cat_q")
	linkQ := c.QueryParam("link_q")
	linkCat := queryInt64(c, "link_cat", 0)

	// 分类：分页列表（表格用）+ 全量列表（网址弹窗下拉、分类名映射用）
	cats, catTotal, err := h.store.ListCategoriesPaged(ctx, catQ, catPage, pageSize)
	if err != nil {
		return err
	}
	allCats, err := h.store.ListCategories(ctx)
	if err != nil {
		return err
	}

	// 网址：分页列表（支持按分类筛选）
	links, linkTotal, err := h.store.ListLinksPaged(ctx, linkQ, linkCat, linkPage, pageSize)
	if err != nil {
		return err
	}

	catCount, linkCount, err := h.store.Count(ctx)
	if err != nil {
		return err
	}

	bgs, err := h.store.ListBackgrounds(ctx)
	if err != nil {
		return err
	}

	logoLight, logoDark, err := siteLogos(ctx, h.store)
	if err != nil {
		return err
	}
	name, err := siteName(ctx, h.store)
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "admin_dashboard.html", map[string]any{
		"Title":      "后台管理",
		"IsAdmin":    true,
		"Categories": cats,
		"AllCategories": allCats,
		"Links":      links,
		"CatCount":   catCount,
		"LinkCount":  linkCount,
		"BGCount":    len(bgs),
		"Logo":       logoLight,
		"LogoLight":  logoLight,
		"LogoDark":   logoDark,
		"SiteName":   name,
		// 分页信息
		"CatPage":     catPage,
		"CatTotal":    catTotal,
		"CatPages":    (catTotal + pageSize - 1) / pageSize,
		"CatQ":        catQ,
		"LinkPage":     linkPage,
		"LinkTotal":    linkTotal,
		"LinkPages":    (linkTotal + pageSize - 1) / pageSize,
		"LinkQ":        linkQ,
		"LinkCat":      linkCat,
		"LinkPageSize": len(links),
	})
}

// queryInt 读取查询参数中的整数，解析失败或缺省时返回 def。
func queryInt(c *echo.Context, key string, def int) int {
	v := c.QueryParam(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// queryInt64 读取查询参数中的 64 位整数，解析失败或缺省时返回 def。
func queryInt64(c *echo.Context, key string, def int64) int64 {
	v := c.QueryParam(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

// ---------- 分类 ----------

func (h *AdminHandler) CreateCategory(c *echo.Context) error {
	name, _ := echo.FormValueOr[string](c, "name", "")
	name = strings.TrimSpace(name)
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "分类名称不能为空")
	}

	icon, _ := echo.FormValueOr[string](c, "icon", "")
	sortOrder, ok := formInt(c, "sort_order")
	if !ok || sortOrder == 0 {
		sortOrder = h.nextCategorySort(c)
	}

	if err := h.store.CreateCategory(c.Request().Context(), &model.Category{
		Name:      name,
		Icon:      strings.TrimSpace(icon),
		SortOrder: sortOrder,
	}); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, anchorCategories())
}

func (h *AdminHandler) UpdateCategory(c *echo.Context) error {
	id, err := echo.PathParamOr[int64](c, "id", 0)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的分类 ID")
	}

	ctx := c.Request().Context()
	cat, err := h.store.GetCategory(ctx, id)
	if err != nil {
		return err
	}
	if cat == nil {
		return echo.NewHTTPError(http.StatusNotFound, "分类不存在")
	}

	name, _ := echo.FormValueOr[string](c, "name", "")
	name = strings.TrimSpace(name)
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "分类名称不能为空")
	}

	icon, _ := echo.FormValueOr[string](c, "icon", "")
	sortOrder, _ := echo.FormValueOr[int](c, "sort_order", cat.SortOrder)

	cat.Name = name
	cat.Icon = strings.TrimSpace(icon)
	cat.SortOrder = sortOrder

	if err := h.store.UpdateCategory(ctx, cat); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, anchorCategories())
}

func (h *AdminHandler) DeleteCategory(c *echo.Context) error {
	id, err := echo.PathParamOr[int64](c, "id", 0)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的分类 ID")
	}
	if err := h.store.DeleteCategory(c.Request().Context(), id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, anchorCategories())
}

// ---------- 网址 ----------

func (h *AdminHandler) CreateLink(c *echo.Context) error {
	title, _ := echo.FormValueOr[string](c, "title", "")
	title = strings.TrimSpace(title)
	if title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "网址名称不能为空")
	}

	rawURL, _ := echo.FormValueOr[string](c, "url", "")
	rawURL = normalizeURL(rawURL)
	if rawURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "网址不能为空")
	}

	catID, _ := echo.FormValueOr[int64](c, "category_id", 0)
	if catID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择所属分类")
	}

	icon, _ := echo.FormValueOr[string](c, "icon", "")
	desc, _ := echo.FormValueOr[string](c, "description", "")
	sortOrder, ok := formInt(c, "sort_order")
	if !ok || sortOrder == 0 {
		sortOrder = h.nextLinkSort(c, catID)
	}

	if err := h.store.CreateLink(c.Request().Context(), &model.Link{
		CategoryID:  catID,
		Title:       title,
		URL:         rawURL,
		Icon:        strings.TrimSpace(icon),
		Description: strings.TrimSpace(desc),
		SortOrder:   sortOrder,
	}); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, anchorLinks())
}

func (h *AdminHandler) UpdateLink(c *echo.Context) error {
	id, err := echo.PathParamOr[int64](c, "id", 0)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的网址 ID")
	}

	ctx := c.Request().Context()
	link, err := h.store.GetLink(ctx, id)
	if err != nil {
		return err
	}
	if link == nil {
		return echo.NewHTTPError(http.StatusNotFound, "网址不存在")
	}

	title, _ := echo.FormValueOr[string](c, "title", "")
	title = strings.TrimSpace(title)
	if title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "网址名称不能为空")
	}

	rawURL, _ := echo.FormValueOr[string](c, "url", "")
	rawURL = normalizeURL(rawURL)
	if rawURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "网址不能为空")
	}

	catID, _ := echo.FormValueOr[int64](c, "category_id", link.CategoryID)
	if catID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择所属分类")
	}

	icon, _ := echo.FormValueOr[string](c, "icon", "")
	desc, _ := echo.FormValueOr[string](c, "description", "")
	sortOrder, _ := echo.FormValueOr[int](c, "sort_order", link.SortOrder)

	link.CategoryID = catID
	link.Title = title
	link.URL = rawURL
	link.Icon = strings.TrimSpace(icon)
	link.Description = strings.TrimSpace(desc)
	link.SortOrder = sortOrder

	if err := h.store.UpdateLink(ctx, link); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, anchorLinks())
}

func (h *AdminHandler) DeleteLink(c *echo.Context) error {
	id, err := echo.PathParamOr[int64](c, "id", 0)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的网址 ID")
	}
	if err := h.store.DeleteLink(c.Request().Context(), id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, anchorLinksFiltered(c))
}

// DeleteLinks 批量删除选中的网址。表单字段 ids 为逗号分隔的 ID 列表。
func (h *AdminHandler) DeleteLinks(c *echo.Context) error {
	raw := strings.TrimSpace(c.FormValue("ids"))
	if raw == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择要删除的网址")
	}

	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "无效的网址 ID")
		}
		ids = append(ids, n)
	}
	if len(ids) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择要删除的网址")
	}

	if err := h.store.DeleteLinks(c.Request().Context(), ids); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, anchorLinksFiltered(c))
}

// ---------- 辅助函数 ----------

// nextCategorySort 返回新分类应使用的排序号（当前最大值 + 10），使其排在末尾。
func (h *AdminHandler) nextCategorySort(c *echo.Context) int {
	cats, err := h.store.ListCategories(c.Request().Context())
	if err != nil {
		return 10
	}
	max := 0
	for _, ct := range cats {
		if ct.SortOrder > max {
			max = ct.SortOrder
		}
	}
	return max + 10
}

// nextLinkSort 返回新链接在指定分类内应使用的排序号。
func (h *AdminHandler) nextLinkSort(c *echo.Context, catID int64) int {
	links, err := h.store.ListLinks(c.Request().Context())
	if err != nil {
		return 10
	}
	max := 0
	for _, l := range links {
		if l.CategoryID == catID && l.SortOrder > max {
			max = l.SortOrder
		}
	}
	return max + 10
}

// formInt 读取整数字段，解析失败返回 (0, false)。
func formInt(c *echo.Context, key string) (int, bool) {
	v, err := echo.FormValueOr[int](c, key, 0)
	if err != nil {
		return 0, false
	}
	return v, true
}

// normalizeURL 补全协议前缀，形如 "example.com" 会被补成 "https://example.com"。
func normalizeURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	if _, err := url.Parse(u); err != nil {
		return ""
	}
	return u
}
