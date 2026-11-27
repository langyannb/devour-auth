package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// 防护演练：nonce 重放必须拒绝，审计链被改过必须校验失败。
func TestSecurityDrill(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "10.1.1.1")

	// 先发一张卡用来登录
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/cards/quick?app_id=1", csrf, "10.1.1.1",
		`{"kind":"week","qty":1,"multiplier":1,"device_limit":1}`); code != http.StatusOK {
		t.Fatalf("发卡失败: %d %s", code, body)
	}
	var kami string
	if err := e.st.DB().QueryRow(`SELECT kami FROM cards ORDER BY id DESC LIMIT 1`).Scan(&kami); err != nil {
		t.Fatal(err)
	}

	nonce := fmt.Sprintf("drill-nonce-%d", time.Now().UnixNano())
	login := func(n string) int {
		body := fmt.Sprintf(`{"app":"devour","kami":%q,"device":{"fingerprint":"drill-fp","model":"pixel","brand":"google","os":"14"},"ts":%d,"nonce":%q}`,
			kami, time.Now().Unix(), n)
		code, _ := e.do(t, &http.Client{}, http.MethodPost, "/v1/auth/login", "", "10.1.1.2", body)
		return code
	}
	if code := login(nonce); code != http.StatusOK {
		t.Fatalf("首次登录应成功，得到 %d", code)
	}
	if code := login(nonce); code != http.StatusBadRequest {
		t.Fatalf("重放同一个 nonce 应被拒绝，得到 %d", code)
	}

	// 审计链：改一条记录后校验必须失败
	if code, body := e.do(t, c, http.MethodPut, "/admin/api/flags?app_id=1", csrf, "10.1.1.1",
		`{"key":"drill","value":"1"}`); code != http.StatusOK {
		t.Fatalf("写变量失败: %d %s", code, body)
	}
	if code, body := e.do(t, c, http.MethodGet, "/admin/api/audit/verify", "", "10.1.1.1", ""); code != http.StatusOK {
		t.Fatalf("校验失败: %d %s", code, body)
	}
	// 触发器会挡住 UPDATE，先删掉触发器模拟「有人绕过应用直接改库」
	if _, err := e.st.DB().Exec(`DROP TRIGGER audit_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.DB().Exec(`UPDATE audit_log SET after = '{"tampered":1}' WHERE id = (SELECT MIN(id) FROM audit_log)`); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(t, c, http.MethodGet, "/admin/api/audit/verify", "", "10.1.1.1", "")
	if code != http.StatusOK || !strings.Contains(body, `"ok_chain":false`) {
		t.Fatalf("被改过的审计链应校验失败: %d %s", code, body)
	}
}
