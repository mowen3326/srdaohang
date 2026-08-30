package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"daohang/config"
	"daohang/handler"
	"daohang/store"
	"daohang/view"
)

//go:embed all:view/templates
var tmplFS embed.FS

//go:embed all:static
var staticFS embed.FS

func main() {
	cfg := config.Load()

	db, err := store.OpenSQLite(cfg.DBPath)
	if err != nil {
		slog.Error("数据库初始化失败", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 读取后台访问路径前缀（默认 "admin"，可在后台设置自定义）
	adminPath := "admin"
	if v, err := db.GetSetting(context.Background(), "admin_path"); err == nil && v != "" {
		adminPath = v
	}
	handler.SetAdminBase(adminPath)

	// 注册进程自重启回调：后台路径修改后自动重启生效
	handler.SetRestartHook(restartProcess)

	// DEV=1 时从磁盘读取模板与静态资源，便于开发期热更新
	dev := os.Getenv("DEV") == "1"

	var tmplRoot fs.FS
	if dev {
		tmplRoot = os.DirFS("view/templates")
	} else {
		if tmplRoot, err = fs.Sub(tmplFS, "view/templates"); err != nil {
			slog.Error("模板目录解析失败", "error", err)
			os.Exit(1)
		}
	}

	renderer, err := view.New(tmplRoot)
	if err != nil {
		slog.Error("模板解析失败", "error", err)
		os.Exit(1)
	}
	// 注入站点数据提供者，让所有页面统一携带自定义 favicon 与站点名
	renderer.SetSiteData(func() (string, string) {
		return handler.SitePageData(context.Background(), db)
	})

	e := echo.New()
	e.Renderer = renderer
	e.HTTPErrorHandler = errorHandler(renderer, db)

	// 中间件
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.Gzip())

	// 静态资源
	var staticRoot fs.FS
	if dev {
		staticRoot = os.DirFS("static")
	} else {
		staticRoot = echo.MustSubFS(staticFS, "static")
	}
	e.StaticFS("/static", staticRoot, cacheControl)

	// 前台
	site := handler.NewSite(cfg, db)
	e.GET("/", site.Index)

	// 后台访问路径前缀（可自定义，默认 /admin）
	adminBase := handler.AdminBase()

	// setup / login / logout 三个路由必须注册在鉴权中间件之外，否则会重定向死循环
	auth := handler.NewAuth(cfg, db)
	e.GET(adminBase+"/setup", auth.SetupPage)
	e.POST(adminBase+"/setup", auth.Setup)
	e.GET(adminBase+"/login", auth.LoginPage)
	e.POST(adminBase+"/login", auth.Login)
	e.POST(adminBase+"/logout", auth.Logout)

	// 背景图上传目录（与数据库同目录下的 uploads）
	uploadRoot := filepath.Join(filepath.Dir(cfg.DBPath), "uploads")
	e.Static("/uploads", uploadRoot)

	// 后台（整体鉴权）
	adminH := handler.NewAdmin(db)
	settingsH := handler.NewSettings(cfg, db, uploadRoot)
	admin := e.Group(adminBase)
	admin.Use(auth.RequireAuth())
	admin.Use(auth.RequireCSRF())
	admin.GET("", adminH.Dashboard)
	admin.POST("/categories", adminH.CreateCategory)
	admin.POST("/categories/:id", adminH.UpdateCategory)
	admin.POST("/categories/:id/delete", adminH.DeleteCategory)
	admin.POST("/links", adminH.CreateLink)
	admin.POST("/links/:id", adminH.UpdateLink)
	admin.POST("/links/:id/delete", adminH.DeleteLink)
	admin.POST("/links/delete", adminH.DeleteLinks)
	admin.GET("/password", settingsH.PasswordPage)
	admin.POST("/password", settingsH.ChangePassword)
	admin.GET("/loginlogs", settingsH.LoginLogsPage)
	admin.POST("/loginlogs/clear", settingsH.ClearLoginLogs)
	admin.GET("/backgrounds", settingsH.BackgroundsPage)
	admin.POST("/backgrounds", settingsH.UploadBackground)
	admin.POST("/backgrounds/:id/activate", settingsH.SwitchBackground)
	admin.POST("/backgrounds/:id/delete", settingsH.RemoveBackground)
	admin.POST("/backgrounds/toggle", settingsH.ToggleBackground)
	admin.GET("/display", settingsH.DisplayPage)
	admin.POST("/display", settingsH.SaveDisplay)
	admin.POST("/display/logo", settingsH.UploadLogo)
	admin.POST("/display/logo/clear", settingsH.ClearLogo)
	admin.POST("/display/favicon", settingsH.UploadFavicon)
	admin.POST("/display/favicon/clear", settingsH.ClearFavicon)

	importH := handler.NewImport(db)
	admin.GET("/import", importH.ImportPage)
	admin.POST("/import", importH.UploadImport)
	admin.POST("/import/save", importH.SaveImport)

	backupH := handler.NewBackup(db)
	admin.GET("/backup", backupH.BackupPage)
	admin.GET("/backup/export", backupH.Export)
	admin.POST("/backup/import", backupH.Import)

	// 启动并等待中断信号，收到后优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 未创建管理员时给出明确的引导入口
	if inited, err := db.IsInitialized(context.Background()); err == nil && !inited {
		slog.Info("首次使用：请访问 " + adminBase + "/setup 创建管理员账号")
	}

	// 启动日志里的站点名取后台设置，未设置时用默认名称
	siteName := "Sr网址导航"
	if v, err := db.GetSetting(context.Background(), "site_name"); err == nil && v != "" {
		siteName = v
	}

	slog.Info(siteName+" 已启动",
		"addr", cfg.Addr,
		"db", cfg.DBPath,
		"dev", dev,
		"session_days", int(cfg.SessionTTL.Hours()/24),
	)

	sc := echo.StartConfig{Address: cfg.Addr}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("服务异常退出", "error", err)
		os.Exit(1)
	}
	slog.Info("服务已停止")
}

// errorHandler 统一渲染错误页面。
func errorHandler(r *view.Renderer, db store.Store) echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		code := http.StatusInternalServerError
		msg := "服务器开小差了，请稍后再试"

		// 优先通过 HTTPStatusCoder 接口提取状态码，兼容 Echo v5 的 *HTTPError
		// 与私有 *httpError（如路由未匹配的 ErrNotFound），避免 404 被误判成 500。
		if sc := echo.StatusCode(err); sc != 0 {
			code = sc
		}
		var he *echo.HTTPError
		if errors.As(err, &he) && he.Message != "" {
			msg = he.Message
		}

		if code >= 500 {
			c.Logger().Error("请求处理失败", "error", err, "path", c.Request().URL.Path)
		}

		// 响应已提交则无法再渲染页面。
		// v5 中 c.Response() 返回 http.ResponseWriter，需解包为 *echo.Response 才能读取提交状态
		if resp, uerr := echo.UnwrapResponse(c.Response()); uerr == nil && resp.Committed {
			return
		}

		// 显式写入状态码，避免错误被渲染成 200 导致客户端误判
		if resp, uerr := echo.UnwrapResponse(c.Response()); uerr == nil {
			resp.Status = code
		}

		// 注入站点级数据（favicon、站点名），让错误页也能展示自定义图标与标题
		favicon, siteName := handler.SitePageData(c.Request().Context(), db)

		data := map[string]any{
			"Code":     code,
			"Message":  msg,
			"Favicon":  favicon,
			"SiteName": siteName,
		}
		if renderErr := r.Render(c, c.Response(), "error.html", data); renderErr != nil {
			_ = c.String(code, msg)
		}
	}
}

// cacheControl 为静态资源设置缓存头。
func cacheControl(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "public, max-age=86400")
		return next(c)
	}
}

// restartProcess 自重启：重新拉起自身进程并退出当前进程。
//
// 用于后台路径修改后自动生效。以相同的工作目录、环境变量与参数重新启动
// 同一个可执行文件，之后优雅退出当前进程，由新进程接管端口。
//
// 注意：Windows 下使用 exec.Command 启动新进程（旧进程退出后新进程独立运行）；
// Linux 下同样走 exec.Command，兼容 systemd 与手动启动两种方式。
func restartProcess() {
	exe, err := os.Executable()
	if err != nil {
		slog.Error("重启失败：无法定位可执行文件", "error", err)
		return
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	// 独立于父进程，避免父进程退出时被一并终止（Windows 关键）
	cmd.SysProcAttr = restartSysProcAttr()

	slog.Info("正在重启服务以应用新配置...", "exe", exe)
	if err := cmd.Start(); err != nil {
		slog.Error("重启失败：无法启动新进程", "error", err)
		return
	}

	// 新进程已启动，优雅退出当前进程。
	// 直接 os.Exit(0) 跳过 defer 清理，因为新进程会重新初始化。
	os.Exit(0)
}
