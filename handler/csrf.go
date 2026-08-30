package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/labstack/echo/v5"
)

const csrfCookieName = "daohang_csrf"

// csrfToken 的格式：base64(随机值).base64(HMAC 签名)。
// 随机值每次登录后重新生成，签名用会话密钥，防止伪造。

// newCSRFToken 生成新的 CSRF token。
func newCSRFToken(secret []byte) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(raw)
	sig := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(raw) + "." +
		base64.RawURLEncoding.EncodeToString(sig), nil
}

// verifyCSRFToken 校验 token 是否由当前密钥签发且未被篡改。
func verifyCSRFToken(token string, secret []byte) bool {
	if token == "" {
		return false
	}
	parts := splitToken(token)
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
	return subtle.ConstantTimeCompare(sig, mac.Sum(nil)) == 1
}

func splitToken(t string) []string {
	// 简单按点切分，避免引入 strings 依赖
	var parts []string
	last := 0
	for i := 0; i < len(t); i++ {
		if t[i] == '.' {
			parts = append(parts, t[last:i])
			last = i + 1
		}
	}
	parts = append(parts, t[last:])
	return parts
}

// setCSRFCookie 下发 CSRF Cookie。
func (h *AuthHandler) setCSRFCookie(c *echo.Context) {
	token, err := newCSRFToken(h.cfg.SessionKey)
	if err != nil {
		return
	}
	c.SetCookie(&http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(h.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cfg.Secure,
	})
}

// RequireCSRF 校验写请求（POST 等）携带的 CSRF token 是否与 Cookie 中的一致。
// GET 等安全方法直接放行。
func (h *AuthHandler) RequireCSRF() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !isUnsafeMethod(c.Request().Method) {
				return next(c)
			}

			cookie, err := c.Cookie(csrfCookieName)
			if err != nil || cookie.Value == "" {
				// 会话有效但缺少 CSRF Cookie（如功能上线前遗留的旧会话）：
				// 此时无法校验，返回 403 并提示重新登录，避免误判为可直接放行。
				return echo.NewHTTPError(http.StatusForbidden, "登录状态已过期，请重新登录后重试")
			}

			formToken := c.FormValue("_csrf")
			if formToken == "" {
				// 兼容 JSON/其他 body：从 header 读取
				formToken = c.Request().Header.Get("X-CSRF-Token")
			}

			if formToken == "" ||
				!verifyCSRFToken(formToken, h.cfg.SessionKey) ||
				subtle.ConstantTimeCompare([]byte(formToken), []byte(cookie.Value)) != 1 {
				return echo.NewHTTPError(http.StatusForbidden, "CSRF 校验失败，请刷新页面后重试")
			}

			return next(c)
		}
	}
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
