package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// TOTPPeriod 标准 30 秒步长。
const TOTPPeriod = 30 * time.Second

// DecodeTOTPSecret 解析 base32 秘钥（大小写、空格、填充容错）。
func DecodeTOTPSecret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	s = strings.TrimRight(s, "=")
	if s == "" {
		return nil, fmt.Errorf("空秘钥")
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
}

// TOTP 按 RFC 6238 生成 6 位码。
func TOTP(secret []byte, t time.Time) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(t.Unix())/uint64(TOTPPeriod/time.Second))
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}

// ValidateTOTP 接受前后各一个步长，抵御轻微时钟漂移。
func ValidateTOTP(secret []byte, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for _, d := range []time.Duration{-TOTPPeriod, 0, TOTPPeriod} {
		if subtle.ConstantTimeCompare([]byte(TOTP(secret, t.Add(d))), []byte(code)) == 1 {
			return true
		}
	}
	return false
}
