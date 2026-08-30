package handler

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"daohang/config"
	"daohang/model"
	"daohang/store"
)

// SiteHandler 处理前台页面。
type SiteHandler struct {
	cfg   *config.Config
	store store.Store
}

func NewSite(cfg *config.Config, s store.Store) *SiteHandler {
	return &SiteHandler{cfg: cfg, store: s}
}

// Index 渲染首页：查询分类与链接并按分类组装。
func (h *SiteHandler) Index(c *echo.Context) error {
	ctx := c.Request().Context()

	// 尚未创建管理员时，直接引导到初始化页面。
	// 此时前台必然是空的，展示空状态再让用户点按钮属于多余操作。
	inited, err := h.store.IsInitialized(ctx)
	if err != nil {
		return err
	}
	if !inited {
		return c.Redirect(http.StatusSeeOther, AdminURL("/setup"))
	}

	// 前台需要登录时：未登录则跳转到登录页，登录后回跳前台。
	requireLogin, err := h.store.GetSetting(ctx, settingRequireLogin)
	if err != nil {
		return err
	}
	if requireLogin == "1" && !verifyToken(readSessionCookie(c), h.cfg.SessionKey) {
		return c.Redirect(http.StatusSeeOther, AdminURL("/login?next=/"))
	}

	cats, err := h.store.ListCategories(ctx)
	if err != nil {
		return err
	}
	links, err := h.store.ListLinks(ctx)
	if err != nil {
		return err
	}

	// 按 category_id 归组，保持链接自身排序
	grouped := make(map[int64][]model.Link, len(cats))
	for _, l := range links {
		grouped[l.CategoryID] = append(grouped[l.CategoryID], l)
	}

	// 只保留含有网址的分类：空分类在前台没有展示意义，不渲染。
	//
	// 这里复用 cats 的底层数组做原地过滤（visible 的写入位置始终不超前于
	// 读取位置 i），因此不会覆盖尚未遍历的元素，且不产生额外分配。
	visible := cats[:0]
	for i := range cats {
		cats[i].Links = grouped[cats[i].ID]
		if len(cats[i].Links) > 0 {
			visible = append(visible, cats[i])
		}
	}

	// 背景图：仅在「已开启」且存在生效图时才展示
	bg, err := h.store.ActiveBackground(ctx)
	if err != nil {
		return err
	}
	if enabled, err := h.store.GetSetting(ctx, settingBackgroundEnabled); err == nil && enabled != "1" {
		bg = nil
	} else if err != nil {
		return err
	}

	// 前台显示设置（网址文字 / 网址图标 / 分类图标）
	showLinkURL, err := h.store.GetSetting(ctx, settingShowLinkURL)
	if err != nil {
		return err
	}
	showLinkIcon, err := h.store.GetSetting(ctx, settingShowLinkIcon)
	if err != nil {
		return err
	}
	showCatIcon, err := h.store.GetSetting(ctx, settingShowCatIcon)
	if err != nil {
		return err
	}
	// 未设置时默认显示（值为空或 "1" 视为显示）
	display := map[string]bool{
		"ShowLinkURL":  showLinkURL == "" || showLinkURL == "1",
		"ShowLinkIcon": showLinkIcon == "" || showLinkIcon == "1",
		"ShowCatIcon":  showCatIcon == "" || showCatIcon == "1",
	}

	// 前台显示偏好（默认主题 / 显示模式 / 列数），后台设置、数据库存储
	defaultTheme, err := h.store.GetSetting(ctx, settingDefaultTheme)
	if err != nil {
		return err
	}
	if defaultTheme == "" {
		defaultTheme = "light"
	}
	defaultView, err := h.store.GetSetting(ctx, settingDefaultView)
	if err != nil {
		return err
	}
	if defaultView == "" {
		defaultView = "grid"
	}
	defaultCols, err := h.store.GetSetting(ctx, settingDefaultCols)
	if err != nil {
		return err
	}
	if defaultCols == "" {
		defaultCols = "0"
	}

	// 站点 Logo（空表示使用默认 SVG Logo）
	logoLight, logoDark, err := siteLogos(ctx, h.store)
	if err != nil {
		return err
	}
	// 站点名称
	name, err := siteName(ctx, h.store)
	if err != nil {
		return err
	}
	// 站点 Favicon
	favicon, err := siteFavicon(ctx, h.store)
	if err != nil {
		return err
	}

	totalLinks := len(links)
	return c.Render(http.StatusOK, "index.html", map[string]any{
		"Categories": visible,
		"TotalLinks": totalLinks,
		// 数据库中是否存在分类（含空分类），用于空状态给出更准确的提示
		"HasCategories":  len(cats) > 0,
		"Background":     bg,
		"Display":        display,
		"DefaultTheme":   defaultTheme,
		"DefaultView":    defaultView,
		"DefaultCols":    defaultCols,
		"Logo":           logoLight,
		"LogoLight":      logoLight,
		"LogoDark":       logoDark,
		"Favicon":        favicon,
		"SiteName":       name,
		// 当前访问者是否已登录（用于控制是否展示后台入口，避免泄露后台路径）。
		// 注意：不能用 IsAdmin（那会触发前台加载 admin.css，破坏前台顶栏样式），
		// 单独用 IsLoggedIn 标志。
		"IsLoggedIn": verifyToken(readSessionCookie(c), h.cfg.SessionKey),
	})
}
