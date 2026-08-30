package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"daohang/config"
	"daohang/model"
	"daohang/store"
)

// 背景图开关在 settings 表中的键。
const settingBackgroundEnabled = "background_enabled"

// 前台显示设置在 settings 表中的键。
// 三个开关值 "1" 表示显示，"0" 表示隐藏。
const (
	settingShowLinkURL  = "display_show_link_url"  // 是否显示网址链接文字
	settingShowLinkIcon = "display_show_link_icon" // 是否显示网址图标
	settingShowCatIcon  = "display_show_cat_icon"  // 是否显示分类图标
)

// 前台显示偏好在 settings 表中的键。
const (
	settingDefaultTheme = "display_default_theme" // 默认主题：light / dark / auto
	settingDefaultView  = "display_default_view"  // 默认显示模式：list / grid / icon
	settingDefaultCols  = "display_default_cols"  // 默认每行列数：0 表示自适应，否则 1-12
)

// 站点 Logo 在 settings 表中的键。值为上传到 uploads 目录的文件名，空表示使用默认 Logo。
const settingSiteLogo = "site_logo" // 旧键（浅色主题），保留兼容

// 区分主题的 Logo 键。
const (
	settingSiteLogoLight = "site_logo_light" // 浅色主题 Logo
	settingSiteLogoDark  = "site_logo_dark"  // 暗黑主题 Logo
)

// 站点名称在 settings 表中的键。空表示使用默认名称「Sr网址导航」。
const settingSiteName = "site_name"

// 站点 Favicon 在 settings 表中的键。值为上传到 uploads 目录的文件名，空表示不显示站点图标。
const settingSiteFavicon = "site_favicon"

// 前台访问是否需要登录的键。值为 "1" 时前台需登录，否则（空/"0"）公开访问。
const settingRequireLogin = "require_login"

// 后台访问路径前缀的键。值为 "admin"（默认）或自定义名，如 "panel"。
// 修改后需重启服务生效。
const settingAdminPath = "admin_path"

// defaultSiteName 默认站点名称。
const defaultSiteName = "Sr网址导航"

// 允许上传的背景图 MIME 类型。
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// SettingsHandler 处理后台设置：修改密码与背景图管理。
type SettingsHandler struct {
	cfg        *config.Config
	store      store.Store
	uploadRoot string // 背景图落盘目录的绝对路径
}

func NewSettings(cfg *config.Config, s store.Store, uploadRoot string) *SettingsHandler {
	return &SettingsHandler{cfg: cfg, store: s, uploadRoot: uploadRoot}
}

// ---------- 修改密码（独立页面） ----------

// siteLogos 读取浅色/暗黑主题的 Logo 文件名。
// 归一化规则：只上传一个时，两个主题共用同一个；都未上传时均返回空（使用默认 SVG）。
// 兼容旧键 settingSiteLogo（视为浅色 Logo）。
func siteLogos(ctx context.Context, s store.Store) (light, dark string, err error) {
	light, err = s.GetSetting(ctx, settingSiteLogoLight)
	if err != nil {
		return "", "", err
	}
	dark, err = s.GetSetting(ctx, settingSiteLogoDark)
	if err != nil {
		return "", "", err
	}

	// 兼容旧键：浅色未设置时回退到旧键
	if light == "" {
		if old, e := s.GetSetting(ctx, settingSiteLogo); e == nil && old != "" {
			light = old
		}
	}

	// 归一化：只上传一个时，另一个主题共用
	if light == "" && dark != "" {
		light = dark
	}
	if dark == "" && light != "" {
		dark = light
	}
	return light, dark, nil
}

// siteLogo 读取浅色主题 Logo 文件名，空表示使用默认 Logo。
func siteLogo(ctx context.Context, s store.Store) (string, error) {
	light, _, err := siteLogos(ctx, s)
	return light, err
}

// siteName 读取站点名称，空表示使用默认名称。
func siteName(ctx context.Context, s store.Store) (string, error) {
	name, err := s.GetSetting(ctx, settingSiteName)
	if err != nil {
		return "", err
	}
	if name == "" {
		return defaultSiteName, nil
	}
	return name, nil
}

// PasswordPage 渲染修改密码页面。
func (h *SettingsHandler) PasswordPage(c *echo.Context) error {
	ctx := c.Request().Context()
	catCount, linkCount, err := h.store.Count(ctx)
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

	return c.Render(http.StatusOK, "admin_password.html", map[string]any{
		"Title":     "修改密码",
		"IsAdmin":   true,
		"CatCount":     catCount,
		"LinkCount":    linkCount,
		"SettingsOpen": true,
		"ActiveTab":    "password",
		"Logo":         logoLight,
		"LogoLight":    logoLight,
		"LogoDark":     logoDark,
		"SiteName":      name,
		"MinLen":       config.MinPasswordLen,
		"PWError":      c.QueryParam("error"),
		"PWOK":         c.QueryParam("ok") == "1",
	})
}

// ChangePassword 校验旧密码并写入新密码。
func (h *SettingsHandler) ChangePassword(c *echo.Context) error {
	ctx := c.Request().Context()

	oldPass := c.FormValue("old_password")
	newPass := c.FormValue("new_password")
	confirm := c.FormValue("confirm_password")

	code := ""
	switch {
	case oldPass == "":
		code = "old_required"
	case len(newPass) < config.MinPasswordLen:
		code = "new_short"
	case newPass != confirm:
		code = "new_mismatch"
	}
	if code != "" {
		return c.Redirect(http.StatusSeeOther, AdminURL("/password?error="+code))
	}

	storedHash, err := h.store.GetSetting(ctx, settingHash)
	if err != nil {
		return err
	}
	if !verifyPassword(oldPass, storedHash) {
		return c.Redirect(http.StatusSeeOther, AdminURL("/password?error=old_wrong"))
	}

	hash, err := hashPassword(newPass)
	if err != nil {
		return err
	}
	if err := h.store.SetSetting(ctx, settingHash, hash); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, AdminURL("/password?ok=1"))
}

// ---------- 背景图管理（独立页面） ----------

// BackgroundsPage 渲染背景图管理页面。
func (h *SettingsHandler) BackgroundsPage(c *echo.Context) error {
	ctx := c.Request().Context()

	bgs, err := h.store.ListBackgrounds(ctx)
	if err != nil {
		return err
	}
	enabled, err := h.store.GetSetting(ctx, settingBackgroundEnabled)
	if err != nil {
		return err
	}
	catCount, linkCount, err := h.store.Count(ctx)
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

	return c.Render(http.StatusOK, "admin_backgrounds.html", map[string]any{
		"Title":       "背景图",
		"IsAdmin":     true,
		"CatCount":     catCount,
		"LinkCount":    linkCount,
		"SettingsOpen": true,
		"ActiveTab":    "backgrounds",
		"BGCount":      len(bgs),
		"Backgrounds":  bgs,
		"Enabled":      enabled == "1",
		"Logo":         logoLight,
		"LogoLight":    logoLight,
		"LogoDark":     logoDark,
		"SiteName":     name,
	})
}

// UploadBackground 接收并保存背景图，然后设为当前生效图。
func (h *SettingsHandler) UploadBackground(c *echo.Context) error {
	file, err := c.FormFile("background")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择要上传的图片")
	}

	fname, err := h.saveImage(file)
	if err != nil {
		return err
	}

	bg := &model.Background{
		Filename: fname,
		Original: filepath.Base(file.Filename),
	}
	if err := h.store.CreateBackground(c.Request().Context(), bg); err != nil {
		return err
	}
	// 上传成功后立即切换为该图生效
	if err := h.store.SetActiveBackground(c.Request().Context(), bg.ID); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, AdminURL("/backgrounds"))
}

// SwitchBackground 切换生效的背景图。
func (h *SettingsHandler) SwitchBackground(c *echo.Context) error {
	id, err := echo.PathParamOr[int64](c, "id", 0)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的背景图 ID")
	}
	if err := h.store.SetActiveBackground(c.Request().Context(), id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, AdminURL("/backgrounds"))
}

// ToggleBackground 开启或关闭背景图显示。
func (h *SettingsHandler) ToggleBackground(c *echo.Context) error {
	ctx := c.Request().Context()
	enable := c.FormValue("enable") == "1"

	val := "0"
	if enable {
		val = "1"
	}
	if err := h.store.SetSetting(ctx, settingBackgroundEnabled, val); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, AdminURL("/backgrounds"))
}

// RemoveBackground 移除背景图并删除磁盘文件（若为生效图则一并清除生效状态）。
func (h *SettingsHandler) RemoveBackground(c *echo.Context) error {
	id, err := echo.PathParamOr[int64](c, "id", 0)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的背景图 ID")
	}

	ctx := c.Request().Context()
	bgs, err := h.store.ListBackgrounds(ctx)
	if err != nil {
		return err
	}

	var target *model.Background
	for i := range bgs {
		if bgs[i].ID == id {
			target = &bgs[i]
			break
		}
	}
	if target == nil {
		return echo.NewHTTPError(http.StatusNotFound, "背景图不存在")
	}

	if err := h.store.DeleteBackground(ctx, id); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(h.uploadRoot, target.Filename))

	return c.Redirect(http.StatusSeeOther, AdminURL("/backgrounds"))
}

// saveImage 校验并保存上传的图片，返回磁盘存储文件名。
func (h *SettingsHandler) saveImage(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, "读取上传文件失败")
	}
	defer src.Close()

	// 读取前 512 字节嗅探真实类型
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", echo.NewHTTPError(http.StatusBadRequest, "读取上传文件失败")
	}
	head = head[:n]

	ct := http.DetectContentType(head)
	if !allowedImageTypes[ct] {
		return "", echo.NewHTTPError(http.StatusBadRequest, "仅支持 JPG / PNG / GIF / WebP 图片")
	}

	// 由 MIME 类型推导扩展名
	ext := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
	}[ct]

	if err := os.MkdirAll(h.uploadRoot, 0o755); err != nil {
		return "", fmt.Errorf("创建上传目录失败: %w", err)
	}

	// 生成随机文件名，避免路径穿越与重名覆盖
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成文件名失败: %w", err)
	}
	fname := hex.EncodeToString(buf) + ext

	dst, err := os.Create(filepath.Join(h.uploadRoot, fname))
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// 写回嗅探头再拷贝剩余内容
	if _, err := dst.Write(head); err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return fname, nil
}

// ---------- 显示设置（独立页面） ----------

// DisplaySettings 表示前台的显示配置。
type DisplaySettings struct {
	ShowLinkURL  bool   // 显示网址链接文字
	ShowLinkIcon bool   // 显示网址图标
	ShowCatIcon  bool   // 显示分类图标
	DefaultTheme string // 默认主题：light / dark / auto
	DefaultView  string // 默认显示模式：list / grid / icon
	DefaultCols  int    // 默认每行列数：0 表示自适应，否则 1-12
	RequireLogin bool   // 前台访问是否需要登录
}

// loadDisplaySettings 读取前台的显示配置。未设置时使用默认值。
func (h *SettingsHandler) loadDisplaySettings(ctx context.Context) (DisplaySettings, error) {
	get := func(key string) (bool, error) {
		v, err := h.store.GetSetting(ctx, key)
		if err != nil {
			return false, err
		}
		// 未设置时默认为显示（值为空或 "1" 均视为显示）
		return v == "" || v == "1", nil
	}
	getStr := func(key string) (string, error) {
		return h.store.GetSetting(ctx, key)
	}

	ds := DisplaySettings{}
	var err error
	if ds.ShowLinkURL, err = get(settingShowLinkURL); err != nil {
		return ds, err
	}
	if ds.ShowLinkIcon, err = get(settingShowLinkIcon); err != nil {
		return ds, err
	}
	if ds.ShowCatIcon, err = get(settingShowCatIcon); err != nil {
		return ds, err
	}

	// 默认主题，未设置时浅色
	ds.DefaultTheme, err = getStr(settingDefaultTheme)
	if err != nil {
		return ds, err
	}
	if ds.DefaultTheme == "" {
		ds.DefaultTheme = "light"
	}

	// 默认显示模式，未设置时图标列表
	ds.DefaultView, err = getStr(settingDefaultView)
	if err != nil {
		return ds, err
	}
	if ds.DefaultView == "" {
		ds.DefaultView = "grid"
	}

	// 默认列数，未设置或非数字时自适应（0）
	if colsStr, err := getStr(settingDefaultCols); err != nil {
		return ds, err
	} else if colsStr != "" {
		if n, e := strconv.Atoi(colsStr); e == nil && n >= 1 && n <= 12 {
			ds.DefaultCols = n
		}
	}

	// 前台是否需要登录，默认否（公开访问）：仅 "1" 视为需要登录
	if v, err := getStr(settingRequireLogin); err != nil {
		return ds, err
	} else {
		ds.RequireLogin = v == "1"
	}

	return ds, nil
}

// DisplayPage 渲染显示设置页面。
func (h *SettingsHandler) DisplayPage(c *echo.Context) error {
	ctx := c.Request().Context()

	ds, err := h.loadDisplaySettings(ctx)
	if err != nil {
		return err
	}
	catCount, linkCount, err := h.store.Count(ctx)
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
	favicon, err := siteFavicon(ctx, h.store)
	if err != nil {
		return err
	}

	// 后台路径前缀（仅用于回显，当前值不含前导斜杠）
	adminPath, err := h.store.GetSetting(ctx, settingAdminPath)
	if err != nil {
		return err
	}
	if adminPath == "" {
		adminPath = "admin"
	}

	return c.Render(http.StatusOK, "admin_display.html", map[string]any{
		"Title":        "网站设置",
		"IsAdmin":      true,
		"CatCount":     catCount,
		"LinkCount":    linkCount,
		"SettingsOpen": true,
		"ActiveTab":    "display",
		"ShowLinkURL":  ds.ShowLinkURL,
		"ShowLinkIcon": ds.ShowLinkIcon,
		"ShowCatIcon":  ds.ShowCatIcon,
		"DefaultTheme": ds.DefaultTheme,
		"DefaultView":  ds.DefaultView,
		"DefaultCols":  ds.DefaultCols,
		"RequireLogin": ds.RequireLogin,
		"Logo":         logoLight,
		"LogoLight":    logoLight,
		"LogoDark":     logoDark,
		"Favicon":      favicon,
		"SiteName":     name,
		"AdminPath":    adminPath,
	})
}

// SaveDisplay 保存显示设置。复选框未勾选时不会出现在表单里，据此判定开关。
func (h *SettingsHandler) SaveDisplay(c *echo.Context) error {
	ctx := c.Request().Context()

	// 表单中只有勾选的复选框才会提交值，未勾选视为隐藏
	set := func(key, formName string) error {
		val := "0"
		if c.FormValue(formName) == "on" {
			val = "1"
		}
		return h.store.SetSetting(ctx, key, val)
	}

	if err := set(settingShowLinkURL, "show_link_url"); err != nil {
		return err
	}
	if err := set(settingShowLinkIcon, "show_link_icon"); err != nil {
		return err
	}
	if err := set(settingShowCatIcon, "show_cat_icon"); err != nil {
		return err
	}

	// 默认主题（校验取值）
	theme := c.FormValue("default_theme")
	if theme != "light" && theme != "dark" && theme != "auto" {
		theme = "light"
	}
	if err := h.store.SetSetting(ctx, settingDefaultTheme, theme); err != nil {
		return err
	}

	// 默认显示模式（校验取值）
	view := c.FormValue("default_view")
	if view != "list" && view != "grid" && view != "icon" {
		view = "grid"
	}
	if err := h.store.SetSetting(ctx, settingDefaultView, view); err != nil {
		return err
	}

	// 默认每行列数（0 或空 = 自适应）
	cols := 0
	if v := c.FormValue("default_cols"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 1 && n <= 12 {
			cols = n
		}
	}
	if err := h.store.SetSetting(ctx, settingDefaultCols, strconv.Itoa(cols)); err != nil {
		return err
	}

	// 站点名称（去除首尾空白；空则恢复默认）
	name := strings.TrimSpace(c.FormValue("site_name"))
	if err := h.store.SetSetting(ctx, settingSiteName, name); err != nil {
		return err
	}

	// 前台是否需要登录（复选框，勾选才提交 on）
	requireLogin := "0"
	if c.FormValue("require_login") == "on" {
		requireLogin = "1"
	}
	if err := h.store.SetSetting(ctx, settingRequireLogin, requireLogin); err != nil {
		return err
	}

	// 后台访问路径前缀（仅允许字母/数字/下划线/连字符，空则恢复默认 "admin"）
	adminPath := strings.TrimSpace(c.FormValue("admin_path"))
	adminPath = sanitizeAdminPath(adminPath)

	oldAdminPath, _ := h.store.GetSetting(ctx, settingAdminPath)
	if oldAdminPath == "" {
		oldAdminPath = "admin"
	}

	pathChanged := adminPath != oldAdminPath
	if err := h.store.SetSetting(ctx, settingAdminPath, adminPath); err != nil {
		return err
	}

	// 路径变化：延迟重启以应用新前缀，并返回过渡页提示用户跳转到新地址
	if pathChanged {
		ScheduleRestart(1200 * time.Millisecond)
		newBase := "/" + adminPath
		return c.Render(http.StatusOK, "admin_restart.html", map[string]any{
			"Title":     "正在应用新路径",
			"IsAdmin":   true,
			"AdminBase": newBase,
			"NewBase":   newBase,
		})
	}

	return c.Redirect(http.StatusSeeOther, AdminURL("/display"))
}

// sanitizeAdminPath 清洗后台路径前缀：
// 仅保留字母、数字、下划线、连字符，去除前导斜杠，空值返回 "admin"。
// 防止注入特殊字符破坏路由或造成路径歧义。
func sanitizeAdminPath(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "admin"
	}
	return out
}

// ---------- 站点 Logo ----------

// logoKeyByTheme 根据主题返回对应的 Logo 设置键。
func logoKeyByTheme(theme string) string {
	if theme == "dark" {
		return settingSiteLogoDark
	}
	return settingSiteLogoLight
}

// UploadLogo 上传指定主题的自定义 Logo，覆盖旧的，保存到 uploads 目录并记录文件名。
func (h *SettingsHandler) UploadLogo(c *echo.Context) error {
	ctx := c.Request().Context()

	theme := c.FormValue("theme") // light / dark
	if theme != "dark" {
		theme = "light"
	}
	key := logoKeyByTheme(theme)

	file, err := c.FormFile("logo")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择要上传的 Logo 图片")
	}

	fname, err := h.saveImage(file)
	if err != nil {
		return err
	}

	// 若该主题已有自定义 Logo，删除旧文件
	if old, err := h.store.GetSetting(ctx, key); err == nil && old != "" {
		_ = os.Remove(filepath.Join(h.uploadRoot, old))
	}

	if err := h.store.SetSetting(ctx, key, fname); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, AdminURL("/display"))
}

// ClearLogo 清空指定主题的自定义 Logo，恢复默认。
func (h *SettingsHandler) ClearLogo(c *echo.Context) error {
	ctx := c.Request().Context()

	theme := c.FormValue("theme") // light / dark
	if theme != "dark" {
		theme = "light"
	}
	key := logoKeyByTheme(theme)

	if old, err := h.store.GetSetting(ctx, key); err == nil && old != "" {
		_ = os.Remove(filepath.Join(h.uploadRoot, old))
	}
	if err := h.store.SetSetting(ctx, key, ""); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, AdminURL("/display"))
}

// ---------- 站点 Favicon ----------

// siteFavicon 读取自定义 Favicon 文件名，空表示不显示站点图标。
func siteFavicon(ctx context.Context, s store.Store) (string, error) {
	return s.GetSetting(ctx, settingSiteFavicon)
}

// SitePageData 返回站点级页面数据（Favicon 文件名与站点名），
// 供 main 包的错误页等非 handler 场景复用，避免硬编码 setting 键名。
func SitePageData(ctx context.Context, s store.Store) (favicon, name string) {
	if v, err := s.GetSetting(ctx, settingSiteFavicon); err == nil {
		favicon = v
	}
	if v, err := s.GetSetting(ctx, settingSiteName); err == nil {
		name = v
	}
	return favicon, name
}

// saveFavicon 校验并保存上传的 favicon，支持 .ico 与常见图片格式，返回磁盘文件名。
func (h *SettingsHandler) saveFavicon(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, "读取上传文件失败")
	}
	defer src.Close()

	// 读取前 512 字节嗅探真实类型
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", echo.NewHTTPError(http.StatusBadRequest, "读取上传文件失败")
	}
	head = head[:n]

	ct := http.DetectContentType(head)
	// 原始文件扩展名（用于识别 .ico）
	origExt := strings.ToLower(filepath.Ext(file.Filename))

	// favicon 支持 .ico 或常见图片格式
	var ext string
	switch {
	case origExt == ".ico" || ct == "image/x-icon" || ct == "image/vnd.microsoft.icon":
		ext = ".ico"
	case ct == "image/png":
		ext = ".png"
	case ct == "image/jpeg":
		ext = ".jpg"
	case ct == "image/webp":
		ext = ".webp"
	case ct == "image/gif":
		ext = ".gif"
	default:
		return "", echo.NewHTTPError(http.StatusBadRequest, "仅支持 ICO / PNG / JPG / WebP / GIF 图片")
	}

	if err := os.MkdirAll(h.uploadRoot, 0o755); err != nil {
		return "", fmt.Errorf("创建上传目录失败: %w", err)
	}

	// 生成随机文件名
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成文件名失败: %w", err)
	}
	fname := hex.EncodeToString(buf) + ext

	dst, err := os.Create(filepath.Join(h.uploadRoot, fname))
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := dst.Write(head); err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return fname, nil
}

// UploadFavicon 上传自定义 Favicon，覆盖旧的。
func (h *SettingsHandler) UploadFavicon(c *echo.Context) error {
	ctx := c.Request().Context()

	file, err := c.FormFile("favicon")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择要上传的 favicon 图标")
	}

	fname, err := h.saveFavicon(file)
	if err != nil {
		return err
	}

	// 删除旧的 favicon 文件
	if old, err := h.store.GetSetting(ctx, settingSiteFavicon); err == nil && old != "" {
		_ = os.Remove(filepath.Join(h.uploadRoot, old))
	}

	if err := h.store.SetSetting(ctx, settingSiteFavicon, fname); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, AdminURL("/display"))
}

// ClearFavicon 清空自定义 Favicon（清空后不显示站点图标）。
func (h *SettingsHandler) ClearFavicon(c *echo.Context) error {
	ctx := c.Request().Context()

	if old, err := h.store.GetSetting(ctx, settingSiteFavicon); err == nil && old != "" {
		_ = os.Remove(filepath.Join(h.uploadRoot, old))
	}
	if err := h.store.SetSetting(ctx, settingSiteFavicon, ""); err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, AdminURL("/display"))
}

// ---------- 登录日志（独立页面） ----------

// loginLogsPageSize 登录日志每页条数。
const loginLogsPageSize = 20

// LoginLogsPage 渲染登录审计日志页面（分页）。
func (h *SettingsHandler) LoginLogsPage(c *echo.Context) error {
	ctx := c.Request().Context()

	catCount, linkCount, err := h.store.Count(ctx)
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

	page := queryInt(c, "page", 1)
	logs, total, err := h.store.ListLoginLogsPaged(ctx, page, loginLogsPageSize)
	if err != nil {
		return err
	}
	pages := (total + loginLogsPageSize - 1) / loginLogsPageSize
	if pages < 1 {
		pages = 1
	}

	return c.Render(http.StatusOK, "admin_loginlogs.html", map[string]any{
		"Title":        "登录日志",
		"IsAdmin":      true,
		"CatCount":     catCount,
		"LinkCount":    linkCount,
		"SettingsOpen": true,
		"ActiveTab":    "loginlogs",
		"Logo":         logoLight,
		"LogoLight":    logoLight,
		"LogoDark":     logoDark,
		"SiteName":     name,
		"LoginLogs":    logs,
		"LogPage":      page,
		"LogTotal":     total,
		"LogPages":     pages,
	})
}

// ClearLoginLogs 清空全部登录日志。
func (h *SettingsHandler) ClearLoginLogs(c *echo.Context) error {
	if err := h.store.ClearLoginLogs(c.Request().Context()); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, AdminURL("/loginlogs"))
}
