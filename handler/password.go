package handler

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// 密码哈希参数。PBKDF2-HMAC-SHA256 的迭代次数参照 OWASP 建议取值，
// 单次运算约耗时数百毫秒，对个人站点而言安全性与响应速度均可接受。
const (
	pbkdf2Iter   = 600000 // 迭代次数
	pbkdf2KeyLen = 32     // 派生密钥长度（字节）
	pbkdf2Salt   = 16     // 盐长度（字节）
)

// hashPassword 生成密码哈希，格式：
//
//	pbkdf2$<迭代次数>$<base64(salt)>$<base64(hash)>
//
// 把参数一并存入，便于日后安全升级迭代次数时兼容旧哈希。
func hashPassword(pass string) (string, error) {
	salt := make([]byte, pbkdf2Salt)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成盐失败: %w", err)
	}

	// 标准库签名：Key(h func() Hash, password string, salt []byte, iter, keyLength int) ([]byte, error)
	dk, err := pbkdf2.Key(sha256.New, pass, salt, pbkdf2Iter, pbkdf2KeyLen)
	if err != nil {
		return "", fmt.Errorf("派生密钥失败: %w", err)
	}

	return fmt.Sprintf("pbkdf2$%d$%s$%s",
		pbkdf2Iter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk),
	), nil
}

// verifyPassword 校验明文密码是否匹配已存储的哈希。
func verifyPassword(pass, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}

	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}

	got, err := pbkdf2.Key(sha256.New, pass, salt, iter, len(want))
	if err != nil {
		return false
	}
	// 恒定时间比较，避免通过耗时差异逐字节推测哈希
	return subtle.ConstantTimeCompare(got, want) == 1
}
