package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func (e env) loginAgentWithPassword(t *testing.T, name, pass string) (*http.Client, string) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	body := fmt.Sprintf(`{"name":%q,"password":%q}`, name, pass)
	resp, err := c.Post(e.ts.URL+"/agent/api/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var lr struct {
		OK   bool `json:"ok"`
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil || !lr.OK {
		t.Fatalf("代理登录失败: %d", resp.StatusCode)
	}
	return c, lr.Data.CSRF
}

// 切换应用必须完全隔离：代理、开放密钥、卡密、公告都不能串。
func TestAppIsolation(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "6.1.1.1")

	// 新建应用 tool2
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/apps", csrf, "6.1.1.1",
		`{"code":"tool2","name":"工具二"}`); code != http.StatusOK {
		t.Fatalf("建应用失败: %d %s", code, body)
	}
	var app2 int64
	if err := e.st.DB().QueryRow(`SELECT id FROM apps WHERE code = 'tool2'`).Scan(&app2); err != nil {
		t.Fatal(err)
	}

	// 两个应用各建一个代理
	mk := func(appID int64, name string) {
		url := fmt.Sprintf("/admin/api/agents?app_id=%d", appID)
		if code, body := e.do(t, c, http.MethodPost, url, csrf, "6.1.1.1",
			fmt.Sprintf(`{"name":%q,"password":"agent-pass-123456","monthly_quota_cents":10000}`, name)); code != http.StatusOK {
			t.Fatalf("建代理失败: %d %s", code, body)
		}
	}
	mk(1, "only-devour")
	mk(app2, "only-tool2")

	list := func(appID int64) string {
		_, body := e.do(t, c, http.MethodGet, fmt.Sprintf("/admin/api/agents?app_id=%d", appID), "", "6.1.1.1", "")
		return body
	}
	if b := list(1); !strings.Contains(b, "only-devour") || strings.Contains(b, "only-tool2") {
		t.Fatalf("app1 代理列表串了: %s", b)
	}
	if b := list(app2); !strings.Contains(b, "only-tool2") || strings.Contains(b, "only-devour") {
		t.Fatalf("app2 代理列表串了: %s", b)
	}

	// 开放密钥按应用隔离
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/open/keys?app_id=1", csrf, "6.1.1.1",
		`{"name":"key-devour","scopes":"cards:read"}`); code != http.StatusOK {
		t.Fatalf("建密钥失败: %d %s", code, body)
	}
	keys := func(appID int64) string {
		_, body := e.do(t, c, http.MethodGet, fmt.Sprintf("/admin/api/open/keys?app_id=%d", appID), "", "6.1.1.1", "")
		return body
	}
	if b := keys(1); !strings.Contains(b, "key-devour") {
		t.Fatalf("app1 应有密钥: %s", b)
	}
	if b := keys(app2); strings.Contains(b, "key-devour") {
		t.Fatalf("app2 看到了 app1 的密钥: %s", b)
	}

	// 代理不能给别的应用出卡
	var agentID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM agents WHERE name = 'only-devour'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	var planID int64
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/plans?app_id=1", csrf, "6.1.1.1",
		`{"name":"p1","items":{"day":300}}`); code != http.StatusOK {
		t.Fatalf("建模板失败: %d %s", code, body)
	}
	if err := e.st.DB().QueryRow(`SELECT id FROM agent_plans WHERE name = 'p1'`).Scan(&planID); err != nil {
		t.Fatal(err)
	}
	if code, body := e.do(t, c, http.MethodPut, fmt.Sprintf("/admin/api/agents/%d/plan?app_id=1", agentID), csrf, "6.1.1.1",
		fmt.Sprintf(`{"plan_id":%d,"max_qty":10}`, planID)); code != http.StatusOK {
		t.Fatalf("绑模板失败: %d %s", code, body)
	}

	ac, acsrf := e.loginAgentWithPassword(t, "only-devour", "agent-pass-123456")

	code, body := e.do(t, ac, http.MethodPost, fmt.Sprintf("/agent/api/issue?app_id=%d", app2), acsrf, "",
		`{"kind":"day","qty":1,"multiplier":1,"device_limit":1}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "该代理不属于这个应用") {
		t.Fatalf("跨应用出卡应被拒: %d %s", code, body)
	}
}
