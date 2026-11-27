package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/langyannb/devour-auth/internal/httpapi"
)

func captchaSetup(t *testing.T) env {
	t.Helper()
	return setup(t, func(o *httpapi.Options) {
		o.CaptchaNew = func() (string, string) { return "cid", "data:image/png;base64,AA" }
		o.CaptchaVerify = func(id, code string) bool { return id == "cid" && code == "1234" }
	})
}

// quickCard 发一张天卡并返回卡密。
func (e env) quickCard(t *testing.T, c *http.Client, csrf, ip string) string {
	t.Helper()
	return e.quickCardLimit(t, c, csrf, ip, 2)
}

func (e env) quickCardLimit(t *testing.T, c *http.Client, csrf, ip string, limit int) string {
	t.Helper()
	body := fmt.Sprintf(`{"kind":"day","qty":1,"multiplier":1,"device_limit":%d}`, limit)
	if code, resp := e.do(t, c, http.MethodPost, "/admin/api/cards/quick?app_id=1", csrf, ip, body); code != http.StatusOK {
		t.Fatalf("发卡失败: %d %s", code, resp)
	}
	var kami string
	if err := e.st.DB().QueryRow(`SELECT kami FROM cards ORDER BY id DESC LIMIT 1`).Scan(&kami); err != nil {
		t.Fatal(err)
	}
	return kami
}

func (e env) clientLogin(t *testing.T, kami, fp, ip string) {
	t.Helper()
	body := fmt.Sprintf(`{"kami":%q,"device":{"fingerprint":%q,"model":"pixel","brand":"google","os":"14","app_version":"1.0"},"ts":%d,"nonce":%q}`,
		kami, fp, time.Now().Unix(), fmt.Sprintf("nonce-%d-%s", time.Now().UnixNano(), fp))
	code, resp := e.do(t, &http.Client{}, http.MethodPost, "/v1/auth/login", "", ip, body)
	if code != http.StatusOK {
		t.Fatalf("客户端登录失败: %d %s", code, resp)
	}
}

func (e env) setFlag(t *testing.T, c *http.Client, csrf, ip, key, value string) {
	t.Helper()
	body := fmt.Sprintf(`{"key":%q,"value":%q}`, key, value)
	if code, resp := e.do(t, c, http.MethodPut, "/admin/api/flags?app_id=1", csrf, ip, body); code != http.StatusOK {
		t.Fatalf("设开关失败: %d %s", code, resp)
	}
}

func queryCard(t *testing.T, e env, kami, ip string) map[string]any {
	t.Helper()
	code, body := e.do(t, &http.Client{}, http.MethodPost, "/v1/query", "", ip,
		fmt.Sprintf(`{"kami":%q,"captcha_id":"cid","captcha_code":"1234"}`, kami))
	if code != http.StatusOK {
		t.Fatalf("查卡失败: %d %s", code, body)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

// 解绑页流程：查卡拿到设备和 ticket，按设备解绑。
func TestQueryUnbindSelectedDevice(t *testing.T) {
	e := captchaSetup(t)
	c, csrf := e.login(t, "admin", "8.8.1.1")
	kami := e.quickCard(t, c, csrf, "8.8.1.1")
	e.clientLogin(t, kami, "fp-one", "8.8.1.1")
	e.clientLogin(t, kami, "fp-two", "8.8.1.1")

	d := queryCard(t, e, kami, "8.8.1.1")
	devices, _ := d["devices"].([]any)
	if len(devices) != 2 {
		t.Fatalf("设备数 = %d want 2", len(devices))
	}
	ticket, _ := d["unbind_ticket"].(string)
	if ticket == "" {
		t.Fatal("没有 unbind_ticket")
	}
	var wantID int64
	for _, raw := range devices {
		m := raw.(map[string]any)
		if fp, _ := m["fingerprint"].(string); !strings.HasPrefix(fp, "****") {
			t.Fatalf("指纹未脱敏: %s", fp)
		}
		if wantID == 0 {
			wantID = int64(m["id"].(float64))
		}
	}

	code, body := e.do(t, &http.Client{}, http.MethodPost, "/v1/query/unbind", "", "8.8.1.1",
		fmt.Sprintf(`{"kami":%q,"ticket":%q,"device_ids":[%d]}`, kami, ticket, wantID))
	if code != http.StatusOK {
		t.Fatalf("解绑失败: %d %s", code, body)
	}
	d = queryCard(t, e, kami, "8.8.1.1")
	if d["bound_devices"].(float64) != 1 {
		t.Fatalf("解绑后绑定数 = %v want 1", d["bound_devices"])
	}
}

// 免费模式：没有免费次数时拒绝，不扣时。
func TestQueryUnbindRejectWhenNoFree(t *testing.T) {
	e := captchaSetup(t)
	c, csrf := e.login(t, "admin", "8.8.2.1")
	kami := e.quickCard(t, c, csrf, "8.8.2.1")
	e.clientLogin(t, kami, "fp-a", "8.8.2.1")
	e.setFlag(t, c, csrf, "8.8.2.1", "unbind_free", "0")
	e.setFlag(t, c, csrf, "8.8.2.1", "unbind_mode", "reject")
	e.setFlag(t, c, csrf, "8.8.2.1", "unbind_cost_hours", "5")

	ticket := queryCard(t, e, kami, "8.8.2.1")["unbind_ticket"].(string)
	code, body := e.do(t, &http.Client{}, http.MethodPost, "/v1/query/unbind", "", "8.8.2.1",
		fmt.Sprintf(`{"kami":%q,"ticket":%q}`, kami, ticket))
	if code != http.StatusConflict {
		t.Fatalf("应拒绝，得到 %d %s", code, body)
	}
	if !strings.Contains(body, "免费换绑次数已用完") {
		t.Fatalf("提示不对: %s", body)
	}
}

// 收费模式：超出免费次数按小时扣时长。
func TestQueryUnbindChargeHours(t *testing.T) {
	e := captchaSetup(t)
	c, csrf := e.login(t, "admin", "8.8.3.1")
	kami := e.quickCard(t, c, csrf, "8.8.3.1")
	e.clientLogin(t, kami, "fp-b", "8.8.3.1") // 激活时长卡
	e.setFlag(t, c, csrf, "8.8.3.1", "unbind_free", "0")
	e.setFlag(t, c, csrf, "8.8.3.1", "unbind_mode", "charge")
	e.setFlag(t, c, csrf, "8.8.3.1", "unbind_cost_hours", "1")

	before := queryCard(t, e, kami, "8.8.3.1")["expire_at"].(float64)
	ticket := queryCard(t, e, kami, "8.8.3.1")["unbind_ticket"].(string)
	code, body := e.do(t, &http.Client{}, http.MethodPost, "/v1/query/unbind", "", "8.8.3.1",
		fmt.Sprintf(`{"kami":%q,"ticket":%q}`, kami, ticket))
	if code != http.StatusOK {
		t.Fatalf("解绑失败: %d %s", code, body)
	}
	if !strings.Contains(body, `"charged_hours":1`) {
		t.Fatalf("应扣 1 小时: %s", body)
	}
	after := queryCard(t, e, kami, "8.8.3.1")["expire_at"].(float64)
	if before-after != 3600 {
		t.Fatalf("到期应减 3600 秒，差 %v", before-after)
	}
}

// ticket 必须和卡密对得上。
func TestQueryUnbindWrongTicket(t *testing.T) {
	e := captchaSetup(t)
	c, csrf := e.login(t, "admin", "8.8.4.1")
	kami := e.quickCard(t, c, csrf, "8.8.4.1")
	e.clientLogin(t, kami, "fp-c", "8.8.4.1")
	code, _ := e.do(t, &http.Client{}, http.MethodPost, "/v1/query/unbind", "", "8.8.4.1",
		fmt.Sprintf(`{"kami":%q,"ticket":"bad.ticket"}`, kami))
	if code != http.StatusBadRequest {
		t.Fatalf("错误 ticket 应 400，得到 %d", code)
	}
}

// 设备验证关掉后不再拦设备上限；IP 验证打开时换网段记一条事件。
func TestDeviceAndIPVerifyFlags(t *testing.T) {
	e := captchaSetup(t)
	c, csrf := e.login(t, "admin", "8.8.5.1")
	kami := e.quickCardLimit(t, c, csrf, "8.8.5.1", 1)
	var cardID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM cards WHERE kami = ?`, kami).Scan(&cardID); err != nil {
		t.Fatal(err)
	}

	// 关闭设备验证前：第二个设备应被拒
	e.clientLogin(t, kami, "fp-1", "8.8.5.1")
	body := fmt.Sprintf(`{"kami":%q,"device":{"fingerprint":"fp-2"},"ts":%d,"nonce":%q}`,
		kami, time.Now().Unix(), fmt.Sprintf("nonce-x-%d", time.Now().UnixNano()))
	if code, _ := e.do(t, &http.Client{}, http.MethodPost, "/v1/auth/login", "", "8.8.5.1", body); code != http.StatusForbidden {
		t.Fatalf("设备上限应 403，得到 %d", code)
	}

	e.setFlag(t, c, csrf, "8.8.5.1", "verify_device", "0")
	e.clientLogin(t, kami, "fp-2", "8.8.5.1")

	e.setFlag(t, c, csrf, "8.8.5.1", "verify_ip", "1")
	e.clientLogin(t, kami, "fp-2", "9.9.5.9")
	var n int
	if err := e.st.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE card_id = ? AND kind = 'ip_anomaly'`, cardID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("换 IP 应记录 ip_anomaly 事件")
	}
}

// 新卡类型只允许时长/永久。
func TestPolicyKindRestricted(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "8.8.6.1")
	code, body := e.do(t, c, http.MethodPost, "/admin/api/policies?app_id=1", csrf, "8.8.6.1",
		`{"code":"count-new","name":"次数卡","kind":"count","device_limit":1,"initial_times":10}`)
	if code != http.StatusBadRequest {
		t.Fatalf("次数卡应被拒，得到 %d %s", code, body)
	}
	if code, body = e.do(t, c, http.MethodPost, "/admin/api/policies?app_id=1", csrf, "8.8.6.1",
		`{"code":"day-new","name":"天卡","kind":"duration","duration_days":1,"device_limit":1}`); code != http.StatusOK {
		t.Fatalf("时长卡应允许，得到 %d %s", code, body)
	}
}
