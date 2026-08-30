package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"daohang/config"
	"daohang/model"
	"daohang/store"
)

const cookieName = "daohang_session"

// 管理员凭据在数据库中的存储键
const (
	settingUser = "admin_user"
	settingHash = "admin_hash"
)

// 登录限流参数
const (
	maxLoginFails    = 3               // 连续失败次数上限
	loginLockoutTime = 15 * time.Minute // 达到上限后的锁定时长
)

// clientIP 提取客户端 IP。
// 仅当配置了 TRUST_PROXY（部署在反向代理后）时才信任 X-Forwarded-For 首项，
// 否则一律使用 TCP 对端地址 RemoteAddr，防止攻击者伪造该头绕过限流。
func (h *AuthHandler) clientIP(c *echo.Context) string {
	if h.cfg.TrustProxy {
		if xff := c.Request().Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i > 0 {
				xff = xff[:i]
			}
			if ip := strings.TrimSpace(xff); ip != "" {
				return ip
			}
		}
	}
	return stripPort(c.Request().RemoteAddr)
}

// stripPort 去掉 "IP:端口" 中的端口部分，仅保留 IP。
// 用于限流与日志记录，避免每次连接随机端口导致同源请求被当成不同来源。
func stripPort(addr string) string {
	if addr == "" {
		return ""
	}
	// IPv6 形如 "[::1]:8080"，需特殊处理
	if addr[0] == '[' {
		if i := strings.LastIndexByte(addr, ']'); i > 0 {
			return addr[1:i]
		}
		return addr
	}
	// IPv4 形如 "127.0.0.1:8080"
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		return addr[:i]
	}
	return addr
}

// lockKey 由用户名与 IP 组合成限流标识，避免单 IP 误伤他人，也防止单账号被全网锁定。
func lockKey(user, ip string) string {
	return user + "|" + ip
}

// AuthHandler 负责初始化、登录、登出与鉴权。
type AuthHandler struct {
	cfg   *config.Config
	store store.Store
}

func NewAuth(cfg *config.Config, s store.Store) *AuthHandler {
	return &AuthHandler{cfg: cfg, store: s}
}

// issueToken 生成 HMAC 签名的会话令牌，格式为 base64(过期时间戳).base64(签名)。
// 令牌自身携带过期时间，即使客户端篡改 Cookie 的 MaxAge 也无法延长有效期。
func issueToken(secret []byte, ttl time.Duration) string {
	payload := strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

// verifyToken 校验签名并检查是否过期，两步均通过才返回 true。
func verifyToken(token string, secret []byte) bool {
	if token == "" {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(raw)
	if subtle.ConstantTimeCompare(sig, mac.Sum(nil)) != 1 {
		return false
	}

	exp, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

func readSessionCookie(c *echo.Context) string {
	if ck, err := c.Cookie(cookieName); err == nil {
		return ck.Value
	}
	return ""
}

// setSession 下发会话 Cookie。
func (h *AuthHandler) setSession(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    issueToken(h.cfg.SessionKey, h.cfg.SessionTTL),
		Path:     "/",
		MaxAge:   int(h.cfg.SessionTTL.Seconds()), // 默认 7 天
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cfg.Secure,
	})
}

// ---------- 首次初始化 ----------

// SetupPage 展示创建管理员账号的页面。
// 已初始化则跳转登录页，避免重复创建覆盖已有账号。
// 初始化对所有来源开放（安装后应尽快完成，避免被他人抢先创建）。
func (h *AuthHandler) SetupPage(c *echo.Context) error {
	ctx := c.Request().Context()

	inited, err := h.store.IsInitialized(ctx)
	if err != nil {
		return err
	}
	if inited {
		return c.Redirect(http.StatusSeeOther, AdminURL("/login"))
	}

	name, err := siteName(c.Request().Context(), h.store)
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "admin_setup.html", map[string]any{
		"Title":        "创建管理员",
		"IsAdmin":      true,
		"HideSettings": true,
		"MinLen":       config.MinPasswordLen,
		"Error":        c.QueryParam("error"),
		"SiteName":     name,
	})
}

// Setup 创建管理员账号，成功后直接登录并进入后台。
func (h *AuthHandler) Setup(c *echo.Context) error {
	ctx := c.Request().Context()

	inited, err := h.store.IsInitialized(ctx)
	if err != nil {
		return err
	}
	if inited {
		return c.Redirect(http.StatusSeeOther, AdminURL("/login"))
	}

	user := strings.TrimSpace(c.FormValue("username"))
	pass := c.FormValue("password")
	confirm := c.FormValue("password_confirm")

	// 校验并回显对应的错误提示
	code := ""
	switch {
	case len([]rune(user)) < 2:
		code = "user_short"
	case len(pass) < config.MinPasswordLen:
		code = "pass_short"
	case pass != confirm:
		code = "pass_mismatch"
	}
	if code != "" {
		return c.Redirect(http.StatusSeeOther, AdminURL("/setup?error="+code))
	}

	hash, err := hashPassword(pass)
	if err != nil {
		return err
	}
	if err := h.store.SetSetting(ctx, settingUser, user); err != nil {
		return err
	}
	if err := h.store.SetSetting(ctx, settingHash, hash); err != nil {
		// 用户名已写入但哈希失败，回滚以保持状态一致
		_ = h.store.SetSetting(ctx, settingUser, "")
		return err
	}

	// 创建完成即视为已登录，免去再输一次
	h.setSession(c)
	h.setCSRFCookie(c)
	return c.Redirect(http.StatusSeeOther, AdminURL(""))
}

// ---------- 登录 / 登出 ----------

// LoginPage 展示登录页。
// 未初始化时引导到创建账号页；已登录则直接进入后台。
func (h *AuthHandler) LoginPage(c *echo.Context) error {
	ctx := c.Request().Context()

	inited, err := h.store.IsInitialized(ctx)
	if err != nil {
		return err
	}
	if !inited {
		return c.Redirect(http.StatusSeeOther, AdminURL("/setup"))
	}

	if verifyToken(readSessionCookie(c), h.cfg.SessionKey) {
		return c.Redirect(http.StatusSeeOther, AdminURL(""))
	}

	name, err := siteName(ctx, h.store)
	if err != nil {
		return err
	}
	logoLight, logoDark, err := siteLogos(ctx, h.store)
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "admin_login.html", map[string]any{
		"Title":        "登录",
		"Error":        c.QueryParam("error"),
		"Failed":       c.QueryParam("error") == "1",
		"Locked":       c.QueryParam("error") == "locked",
		"IsAdmin":      true,
		"HideSettings": true,
		"SiteName":     name,
		"Logo":         logoLight,
		"LogoLight":    logoLight,
		"LogoDark":     logoDark,
		"Next":         c.QueryParam("next"),
	})
}

// Login 校验账号密码并下发会话 Cookie，含失败限流与审计日志。
func (h *AuthHandler) Login(c *echo.Context) error {
	ctx := c.Request().Context()

	user := strings.TrimSpace(c.FormValue("username"))
	pass := c.FormValue("password")
	ip := h.clientIP(c)

	// 记录日志（成功/失败统一落库）
	logEntry := func(success bool, reason string) {
		_ = h.store.AddLoginLog(ctx, &model.LoginLog{
			Username: user,
			IP:       ip,
			Success:  success,
			Reason:   reason,
		})
	}

	storedUser, err := h.store.GetSetting(ctx, settingUser)
	if err != nil {
		return err
	}
	storedHash, err := h.store.GetSetting(ctx, settingHash)
	if err != nil {
		return err
	}

	key := lockKey(user, ip)

	// 已处于锁定期则直接拒绝，不再校验密码
	if until, err := h.store.GetLoginLockUntil(ctx, key); err != nil {
		return err
	} else if until > time.Now().Unix() {
		logEntry(false, "locked")
		return c.Redirect(http.StatusSeeOther, AdminURL("/login?error=locked"))
	}

	// 两处均用恒定时间比较，避免通过响应耗时推断账号或密码
	okUser := subtle.ConstantTimeCompare([]byte(user), []byte(storedUser)) == 1
	okPass := verifyPassword(pass, storedHash)

	if !okUser || !okPass || storedHash == "" {
		// 失败计数 +1，达到上限则锁定 15 分钟
		fails, err := h.store.GetLoginFailCount(ctx, key)
		if err != nil {
			return err
		}
		fails++
		if err := h.store.SetLoginFailCount(ctx, key, fails); err != nil {
			return err
		}

		if fails >= maxLoginFails {
			if err := h.store.SetLoginLockUntil(ctx, key, time.Now().Add(loginLockoutTime).Unix()); err != nil {
				return err
			}
			// 锁定后重置计数，避免解锁后下一击又立刻触发锁定
			_ = h.store.SetLoginFailCount(ctx, key, 0)
			logEntry(false, "locked")
			return c.Redirect(http.StatusSeeOther, AdminURL("/login?error=locked"))
		}

		logEntry(false, "bad_credentials")
		return c.Redirect(http.StatusSeeOther, AdminURL("/login?error=1"))
	}

	// 登录成功：清除失败计数与锁定状态
	if err := h.store.DeleteLoginFail(ctx, key); err != nil {
		return err
	}
	logEntry(true, "")

	// 登录成功后重新生成会话与 CSRF token，防止会话固定攻击
	h.setSession(c)
	h.setCSRFCookie(c)

	// 回跳：仅允许站内相对路径，防止开放重定向
	if next := c.FormValue("next"); isSafeNext(next) {
		return c.Redirect(http.StatusSeeOther, next)
	}
	return c.Redirect(http.StatusSeeOther, AdminURL(""))
}

// isSafeNext 校验回跳地址，仅允许安全的站内相对路径。
// 拦截协议相对地址（//）、反斜杠（\）及 "/\" 等浏览器会归一化后逃逸出站外的写法，
// 防止开放重定向。
func isSafeNext(next string) bool {
	if next == "" || next[0] != '/' {
		return false
	}
	// 协议相对地址：//example.com
	if strings.HasPrefix(next, "//") {
		return false
	}
	// 反斜杠写法：/\example.com 或 \example.com（部分浏览器将 \ 归一化为 /）
	if strings.Contains(next, "\\") {
		return false
	}
	return true
}

// Logout 通过覆写过期 Cookie 清除会话。
func (h *AuthHandler) Logout(c *echo.Context) error {
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cfg.Secure,
	})
	return c.Redirect(http.StatusSeeOther, AdminURL("/login"))
}

// ---------- 鉴权中间件 ----------

// RequireAuth 保护后台路由。
//
// 分流逻辑：未初始化 → 创建账号页；未登录 → 登录页；已登录 → 放行。
// 注意 setup / login / logout 三个路由必须注册在此中间件之外，否则会重定向死循环。
func (h *AuthHandler) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := c.Request().Context()

			inited, err := h.store.IsInitialized(ctx)
			if err != nil {
				return err
			}
			if !inited {
				return c.Redirect(http.StatusSeeOther, AdminURL("/setup"))
			}

			if !verifyToken(readSessionCookie(c), h.cfg.SessionKey) {
				return c.Redirect(http.StatusSeeOther, AdminURL("/login"))
			}

			// 会话有效但缺少 CSRF Cookie 时（如功能上线前遗留的旧会话），
			// 自动补发，避免后续 POST 因缺少 token 而被 403 拦截。
			if _, err := c.Cookie(csrfCookieName); err != nil {
				h.setCSRFCookie(c)
			}

			return next(c)
		}
	}
}
