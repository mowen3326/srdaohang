package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"daohang/model"
	"daohang/store"
)

// 备份文件格式版本，用于后续兼容升级。
const backupVersion = 1

// BackupHandler 处理分类与网址的导入导出。
type BackupHandler struct {
	store store.Store
}

func NewBackup(s store.Store) *BackupHandler {
	return &BackupHandler{store: s}
}

// ExportCategory 导出文件中的分类。
type ExportCategory struct {
	Name      string        `json:"name"`
	Icon      string        `json:"icon"`
	SortOrder int           `json:"sort_order"`
	Links     []ExportLink  `json:"links"`
}

// ExportLink 导出文件中的网址。
type ExportLink struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

// ExportData 导出的完整数据。
type ExportData struct {
	Version    int              `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Categories []ExportCategory `json:"categories"`
}

// BackupPage 渲染数据管理页面。
func (h *BackupHandler) BackupPage(c *echo.Context) error {
	ctx := c.Request().Context()

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

	return c.Render(http.StatusOK, "admin_backup.html", map[string]any{
		"Title":        "数据管理",
		"IsAdmin":      true,
		"CatCount":     catCount,
		"LinkCount":    linkCount,
		"SettingsOpen": true,
		"ActiveTab":    "backup",
		"SiteName":     name,
		"Logo":         logoLight,
		"LogoLight":    logoLight,
		"LogoDark":     logoDark,
		"ImportMessage": c.QueryParam("msg"),
		"ImportError":  c.QueryParam("error"),
	})
}

// Export 导出分类与网址为专用 JSON 文件下载。
func (h *BackupHandler) Export(c *echo.Context) error {
	ctx := c.Request().Context()

	cats, err := h.store.ListCategories(ctx)
	if err != nil {
		return err
	}
	links, err := h.store.ListLinks(ctx)
	if err != nil {
		return err
	}

	// 按分类组装链接
	linkMap := map[int64][]ExportLink{}
	for _, l := range links {
		linkMap[l.CategoryID] = append(linkMap[l.CategoryID], ExportLink{
			Title:       l.Title,
			URL:         l.URL,
			Icon:        l.Icon,
			Description: l.Description,
			SortOrder:   l.SortOrder,
		})
	}

	data := ExportData{
		Version:    backupVersion,
		ExportedAt: time.Now().Format(time.RFC3339),
	}
	for _, cat := range cats {
		ec := ExportCategory{
			Name:      cat.Name,
			Icon:      cat.Icon,
			SortOrder: cat.SortOrder,
			Links:     linkMap[cat.ID],
		}
		if ec.Links == nil {
			ec.Links = []ExportLink{}
		}
		data.Categories = append(data.Categories, ec)
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	filename := "daohang-backup-" + time.Now().Format("20060102-150405") + ".mo"
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	return c.Blob(http.StatusOK, "application/json", raw)
}

// Import 导入专用 JSON 备份文件，追加到现有数据（同名分类复用，网址按 URL 去重）。
func (h *BackupHandler) Import(c *echo.Context) error {
	ctx := c.Request().Context()

	file, err := c.FormFile("backup")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择备份文件")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "读取文件失败")
	}
	defer src.Close()

	// 限制文件大小
	const maxSize = 20 << 20 // 20 MB
	buf := make([]byte, maxSize+1)
	n, err := io.ReadFull(src, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return echo.NewHTTPError(http.StatusBadRequest, "读取文件失败")
	}
	if n > maxSize {
		return echo.NewHTTPError(http.StatusBadRequest, "文件过大")
	}

	var data ExportData
	if err := json.Unmarshal(buf[:n], &data); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "文件格式无效，请确认是系统导出的备份文件")
	}
	if data.Version != backupVersion {
		return echo.NewHTTPError(http.StatusBadRequest, "备份文件版本不匹配")
	}

	// 现有分类名 -> ID 映射（同名复用）
	cats, err := h.store.ListCategories(ctx)
	if err != nil {
		return err
	}
	catByName := map[string]int64{}
	maxCatOrder := 0
	for _, c := range cats {
		catByName[c.Name] = c.ID
		if c.SortOrder > maxCatOrder {
			maxCatOrder = c.SortOrder
		}
	}

	// 现有链接 URL 集合（用于去重）
	links, err := h.store.ListLinks(ctx)
	if err != nil {
		return err
	}
	existingURLs := map[string]bool{}
	for _, l := range links {
		existingURLs[l.URL] = true
	}

	importedCats := 0
	importedLinks := 0

	for _, ec := range data.Categories {
		catID, ok := catByName[ec.Name]
		if !ok {
			// 新建分类
			maxCatOrder++
			nc := &model.Category{
				Name:      ec.Name,
				Icon:      ec.Icon,
				SortOrder: maxCatOrder,
			}
			if err := h.store.CreateCategory(ctx, nc); err != nil {
				return err
			}
			catID = nc.ID
			catByName[ec.Name] = catID
			importedCats++
		}

		// 计算该分类下现有最大 sort_order
		maxOrder := 0
		for _, l := range links {
			if l.CategoryID == catID && l.SortOrder > maxOrder {
				maxOrder = l.SortOrder
			}
		}

		for _, el := range ec.Links {
			if existingURLs[el.URL] {
				continue // 跳过已存在的网址，避免重复
			}
			maxOrder++
			nl := &model.Link{
				CategoryID:  catID,
				Title:       el.Title,
				URL:         el.URL,
				Icon:        el.Icon,
				Description: el.Description,
				SortOrder:   maxOrder,
			}
			if err := h.store.CreateLink(ctx, nl); err != nil {
				return err
			}
			existingURLs[el.URL] = true
			importedLinks++
		}
	}

	msg := "导入完成：新增分类 " + strconv.Itoa(importedCats) + " 个，新增网址 " + strconv.Itoa(importedLinks) + " 条"
	return c.Redirect(http.StatusSeeOther, AdminURL("/backup?msg="+url.QueryEscape(msg)))
}
