package config

import (
	"log/slog"
	"os"
	"time"
)

// Config 保存应用运行所需的全部配置，全部可通过环境变量覆盖。
//
// 管理员账号不在此处配置：首次启动后访问 /admin/setup 创建，
// 凭据以 PBKDF2 哈希形式存于数据库，因此不存在任何默认密码。
type Config struct {
	Addr       string        // 监听地址，默认 :8080
	DBPath     string        // SQLite 文件路径
	SessionKey []byte        // 会话 Cookie 的 HMAC 签名密钥
	SessionTTL time.Duration // 登录有效期，默认 7 天
	Secure     bool          // 仅 HTTPS 部署时置为 true
	TrustProxy bool          // 部署在反向代理后时置为 true，才信任 X-Forwarded-For
}

// MinPasswordLen 创建管理员密码时的最小长度。
const MinPasswordLen = 6

// Load 读取环境变量生成配置，缺失项使用安全默认值。
func Load() *Config {
	cfg := &Config{
		Addr:       envOr("ADDR", ":8080"),
		DBPath:     envOr("DB_PATH", "data/daohang.db"),
		SessionTTL: 7 * 24 * time.Hour,
		Secure:     envBool("SECURE_COOKIE"),
		TrustProxy: envBool("TRUST_PROXY"),
	}

	// 会话密钥用于签名 Cookie。
	// 优先读取环境变量 SESSION_SECRET；未设置时回退到内置默认密钥，
	// 保证服务重启后登录态不失效。生产环境务必通过环境变量覆盖默认值。
	if key := os.Getenv("SESSION_SECRET"); key != "" {
		cfg.SessionKey = []byte(key)
	} else {
		cfg.SessionKey = []byte(defaultSessionSecret)
		slog.Warn("未设置 SESSION_SECRET，使用内置默认密钥；生产环境请务必设置环境变量以覆盖")
	}

	return cfg
}

// defaultSessionSecret 内置默认会话密钥。
// 仅用于未配置 SESSION_SECRET 时的开发/兜底场景，保证重启不掉登录态；
// 生产环境应通过环境变量 SESSION_SECRET 覆盖，避免密钥泄露导致会话被伪造。
const defaultSessionSecret = "daohang-default-session-secret-change-me"

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	}
	return false
}
