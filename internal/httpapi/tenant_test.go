package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

// 一键开通：应用 + 绑定应用的管理员 + 开放接口密钥，一次返回全部凭据。
func TestCreateTenant(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "11.1.1.1")

	code, body := e.do(t, c, http.MethodPost, "/admin/api/tenants", csrf, "11.1.1.1",
		`{"code":"tn","name":"测试开发者","username":"tn-dev"}`)
	if code != http.StatusOK {
		t.Fatalf("一键开通失败: %d %s", code, body)
	}
	var out struct {
		Data struct {
			App struct {
				ID   int64  `json:"id"`
				Code string `json:"code"`
			} `json:"app"`
			Admin struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"admin"`
			APIKey struct {
				KeyID  string `json:"key_id"`
				Secret string `json:"secret"`
			} `json:"api_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Admin.Password == "" || out.Data.APIKey.Secret == "" || out.Data.App.ID == 0 {
		t.Fatalf("凭据不完整: %s", body)
	}

	// 用返回的密码登录，账号应绑定到新应用
	jar, _ := cookiejar.New(nil)
	tc := &http.Client{Jar: jar}
	loginBody := fmt.Sprintf(`{"username":"tn-dev","password":%q}`, out.Data.Admin.Password)
	req, _ := http.NewRequest(http.MethodPost, e.ts.URL+"/admin/api/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "11.1.2.2")
	resp, err := tc.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("开通的账号应能登录: %v %d", err, resp.StatusCode)
	}
	var loginOut struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&loginOut)
	resp.Body.Close()
	if loginOut.Data.CSRF == "" {
		t.Fatal("登录没拿到 csrf")
	}

	// 开放接口密钥能直接用（卡不存在说明已通过验签与权限）
	o := &openCaller{ts: e.ts, keyID: out.Data.APIKey.KeyID, secret: out.Data.APIKey.Secret}
	if code, body := o.call(t, http.MethodPost, "/open/v1/cards/query", `{"kami":"NOPE"}`); code != http.StatusNotFound {
		t.Fatalf("开通的密钥应能调开放接口: %d %s", code, body)
	}

	// 同 code 再来一次要 409
	if code, _ := e.do(t, c, http.MethodPost, "/admin/api/tenants", csrf, "11.1.1.1",
		`{"code":"tn","name":"重复","username":"tn-dev2"}`); code != http.StatusConflict {
		t.Fatalf("重复 code 应 409，得到 %d", code)
	}

	// 绑定应用的账号不能再开新租户
	if code, _ := e.do(t, tc, http.MethodPost, "/admin/api/tenants", loginOut.Data.CSRF, "11.1.2.2",
		`{"code":"tn3","name":"x","username":"tn-dev3"}`); code != http.StatusForbidden {
		t.Fatalf("绑定应用的账号不该能开租户，得到 %d", code)
	}
}
