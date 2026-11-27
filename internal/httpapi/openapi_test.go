package httpapi_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func hmacSign(secret, method, path, ts, nonce, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(method + "\n" + path + "\n" + ts + "\n" + nonce + "\n"))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

type openCaller struct {
	ts     *httptest.Server
	keyID  string
	secret string
	nonce  int
}

func (o *openCaller) call(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	o.nonce++
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	nonce := fmt.Sprintf("n-%d-%d", now.UnixNano(), o.nonce)
	req, err := http.NewRequest(method, o.ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", o.keyID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", hmacSign(o.secret, method, path, ts, nonce, body))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func newOpenCaller(t *testing.T, e env, scopes string) *openCaller {
	t.Helper()
	c, csrf := e.login(t, "admin", "5.5.5.5")
	code, body := e.do(t, c, http.MethodPost, "/admin/api/open/keys", csrf, "5.5.5.5",
		fmt.Sprintf(`{"name":"发货系统","scopes":%q}`, scopes))
	if code != http.StatusOK {
		t.Fatalf("建密钥: %d %s", code, body)
	}
	var out struct {
		Data struct {
			KeyID  string `json:"key_id"`
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Secret == "" || out.Data.KeyID == "" {
		t.Fatalf("没拿到 secret: %s", body)
	}
	return &openCaller{ts: e.ts, keyID: out.Data.KeyID, secret: out.Data.Secret}
}

func TestOpenAPISignature(t *testing.T) {
	e := setup(t)
	o := newOpenCaller(t, e, "cards:read,cards:write")

	// 正确签名：卡不存在说明已通过验签
	code, body := o.call(t, http.MethodPost, "/open/v1/cards/query", `{"kami":"NOPE-0000"}`)
	if code != http.StatusNotFound || !strings.Contains(body, "CARD_NOT_FOUND") {
		t.Fatalf("正确签名应进入业务层: %d %s", code, body)
	}

	// 错 secret
	bad := &openCaller{ts: e.ts, keyID: o.keyID, secret: "wrong-secret"}
	if code, _ := bad.call(t, http.MethodPost, "/open/v1/cards/query", `{"kami":"X"}`); code != http.StatusUnauthorized {
		t.Fatalf("错 secret 应 401，得到 %d", code)
	}

	// 时间偏差 10 分钟
	now := time.Now()
	req, _ := http.NewRequest(http.MethodPost, e.ts.URL+"/open/v1/cards/query", strings.NewReader(`{"kami":"X"}`))
	ts := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	nonce := "skew-nonce-1"
	req.Header.Set("X-Api-Key", o.keyID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", hmacSign(o.secret, http.MethodPost, "/open/v1/cards/query", ts, nonce, `{"kami":"X"}`))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("时间偏差应 401，得到 %d", resp.StatusCode)
	}
}

func TestOpenAPINonceReplay(t *testing.T) {
	e := setup(t)
	o := newOpenCaller(t, e, "cards:read")

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "replay-nonce-123456"
	body := `{"kami":"X"}`
	sig := hmacSign(o.secret, http.MethodPost, "/open/v1/cards/query", ts, nonce, body)

	send := func() int {
		req, _ := http.NewRequest(http.MethodPost, e.ts.URL+"/open/v1/cards/query", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", o.keyID)
		req.Header.Set("X-Timestamp", ts)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", sig)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := send(); code == http.StatusUnauthorized {
		t.Fatalf("首次请求不该被拒: %d", code)
	}
	if code := send(); code != http.StatusUnauthorized {
		t.Fatalf("重放 nonce 应 401，得到 %d", code)
	}
}

func TestOpenAPIScopeEnforced(t *testing.T) {
	e := setup(t)
	o := newOpenCaller(t, e, "cards:read")

	code, body := o.call(t, http.MethodPost, "/open/v1/cards/generate", `{"policy_id":1,"qty":1}`)
	if code != http.StatusForbidden {
		t.Fatalf("缺 cards:write 应 403，得到 %d %s", code, body)
	}
	// stats:read 也没有
	if code, _ := o.call(t, http.MethodGet, "/open/v1/stats/online", ""); code != http.StatusForbidden {
		t.Fatalf("缺 stats:read 应 403，得到 %d", code)
	}
}

func TestOpenAPIStatsOnline(t *testing.T) {
	e := setup(t)
	o := newOpenCaller(t, e, "stats:read")
	code, body := o.call(t, http.MethodGet, "/open/v1/stats/online", "")
	if code != http.StatusOK {
		t.Fatalf("stats/online: %d %s", code, body)
	}
	if !strings.Contains(body, `"online":0`) {
		t.Fatalf("在线数应为 0: %s", body)
	}
}

// 开放接口按类型发卡：不需要 policy_id，和发卡页一致。
func TestOpenAPIQuickIssue(t *testing.T) {
	e := setup(t)
	o := newOpenCaller(t, e, "cards:write")

	code, body := o.call(t, http.MethodPost, "/open/v1/cards/quick",
		`{"kind":"week","qty":2,"multiplier":1,"device_limit":2,"prefix":"FZ"}`)
	if code != http.StatusOK || !strings.Contains(body, `"qty":2`) {
		t.Fatalf("开放接口发卡失败: %d %s", code, body)
	}
	var n int64
	if err := e.st.DB().QueryRow(`SELECT COUNT(*) FROM cards WHERE app_id = 1 AND kami LIKE 'FZ-%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应生成 2 张，实际 %d", n)
	}
	// 卡密能在客户端协议里用（同一应用）
	var kami string
	_ = e.st.DB().QueryRow(`SELECT kami FROM cards WHERE kami LIKE 'FZ-%' LIMIT 1`).Scan(&kami)
	loginBody := fmt.Sprintf(`{"app":"devour","kami":%q,"device":{"fingerprint":"open-fp"},"ts":%d,"nonce":"open-quick-nonce-1"}`,
		kami, time.Now().Unix())
	if code, body := e.do(t, &http.Client{}, http.MethodPost, "/v1/auth/login", "", "5.9.9.9", loginBody); code != http.StatusOK {
		t.Fatalf("开放接口发的卡应能登录: %d %s", code, body)
	}
}

// 开放接口：永久卡续期要回 400 而不是 500；授权接口不能在事务里把自己锁死。
func TestOpenAPIErrorsAndGrant(t *testing.T) {
	e := setup(t)
	o := newOpenCaller(t, e, "cards:write,cards:read")

	// 发一张永久卡
	if code, body := o.call(t, http.MethodPost, "/open/v1/cards/quick", `{"kind":"forever","qty":1}`); code != http.StatusOK {
		t.Fatalf("发永久卡失败: %d %s", code, body)
	}
	var kami string
	_ = e.st.DB().QueryRow(`SELECT kami FROM cards WHERE kind = 'forever' ORDER BY id DESC LIMIT 1`).Scan(&kami)

	code, body := o.call(t, http.MethodPost, "/open/v1/cards/extend", fmt.Sprintf(`{"kami":%q,"days":3}`, kami))
	if code != http.StatusBadRequest || !strings.Contains(body, "永久卡") {
		t.Fatalf("永久卡续期应 400 并说明原因: %d %s", code, body)
	}

	// 授权：先建一个权限，再通过开放接口附加（这里曾经因为事务+单连接死锁）
	if _, err := e.st.DB().Exec(`INSERT INTO entitlements(app_id, code, name, cost_points, active, created_at)
		VALUES(1, 'vip', '会员', 0, 1, 1000)`); err != nil {
		t.Fatal(err)
	}
	code, body = o.call(t, http.MethodPost, "/open/v1/cards/entitlements/grant",
		fmt.Sprintf(`{"kami":%q,"code":"vip"}`, kami))
	if code != http.StatusOK {
		t.Fatalf("授权应成功: %d %s", code, body)
	}
}
