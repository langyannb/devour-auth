package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("a-very-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("格式不对: %s", h)
	}
	if !VerifyPassword("a-very-long-password", h) {
		t.Fatal("正确密码应通过")
	}
	if VerifyPassword("a-very-long-passworD", h) {
		t.Fatal("错误密码不应通过")
	}
	if VerifyPassword("a-very-long-password", "$argon2id$broken") {
		t.Fatal("坏哈希不应通过")
	}
	// 同一密码两次哈希不同（盐不同）
	h2, _ := HashPassword("a-very-long-password")
	if h == h2 {
		t.Fatal("盐应当随机")
	}
}

func TestToken(t *testing.T) {
	a, _ := NewToken()
	b, _ := NewToken()
	if a == b || len(a) < 40 {
		t.Fatalf("令牌应随机且够长: %q %q", a, b)
	}
	if HashToken(a) == a || len(HashToken(a)) != 64 {
		t.Fatal("库里存的应是 sha256 十六进制")
	}
}

func TestTOTPVectors(t *testing.T) {
	secret, err := DecodeTOTPSecret("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ") // "12345678901234567890"
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1234567890, "005924"},
	} {
		got := TOTP(secret, time.Unix(tc.unix, 0))
		if got != tc.want {
			t.Fatalf("T=%d: got %s want %s", tc.unix, got, tc.want)
		}
	}
	if !ValidateTOTP(secret, TOTP(secret, time.Unix(59, 0)), time.Unix(59, 0)) {
		t.Fatal("当前码应通过")
	}
	if ValidateTOTP(secret, TOTP(secret, time.Unix(59, 0)), time.Unix(59+300, 0)) {
		t.Fatal("5 分钟前的码不应通过")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter()
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }

	if !l.Allow("ip", 2, time.Minute) || !l.Allow("ip", 2, time.Minute) {
		t.Fatal("前两次应放行")
	}
	if l.Allow("ip", 2, time.Minute) {
		t.Fatal("第三次应拒绝")
	}
	if !l.Allow("other", 2, time.Minute) {
		t.Fatal("不同 key 互不影响")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("ip", 2, time.Minute) {
		t.Fatal("窗口过后应恢复")
	}
	l.Reset("ip")
	if !l.Allow("ip", 2, time.Minute) || !l.Allow("ip", 2, time.Minute) {
		t.Fatal("Reset 后应重新计数")
	}
}
