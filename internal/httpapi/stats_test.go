package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestStatsAndAuditVerify(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "6.6.6.6")

	code, body := e.do(t, c, http.MethodGet, "/admin/api/stats", "", "6.6.6.6", "")
	if code != http.StatusOK {
		t.Fatalf("stats: %d %s", code, body)
	}
	if !strings.Contains(body, `"cards_total":0`) || !strings.Contains(body, `"online_sessions":0`) {
		t.Fatalf("stats 内容不对: %s", body)
	}

	// 先产生一条审计，再校验链
	if code, b := e.do(t, c, http.MethodPut, "/admin/api/flags", csrf, "6.6.6.6",
		`{"key":"maintenance","value":"0"}`); code != http.StatusOK {
		t.Fatalf("写变量: %d %s", code, b)
	}
	code, body = e.do(t, c, http.MethodGet, "/admin/api/audit/verify", "", "6.6.6.6", "")
	if code != http.StatusOK || !strings.Contains(body, `"ok_chain":true`) {
		t.Fatalf("verify: %d %s", code, body)
	}
	if !strings.Contains(body, `"entries":1`) {
		t.Fatalf("应有 1 条审计: %s", body)
	}
}

func TestHostGuard(t *testing.T) {
	e := setup(t)
	// setup 里 AdminHost=admin.test、AgentHost=agent.test、NetHost 为空
	send := func(host, path string) int {
		req, _ := http.NewRequest(http.MethodGet, e.ts.URL+path, nil)
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := send("admin.test", "/v1/meta"); code != http.StatusNotFound {
		t.Fatalf("admin 域访问 /v1/meta 应 404，得到 %d", code)
	}
	if code := send("admin.test", "/admin/api/me"); code == http.StatusNotFound {
		t.Fatal("admin 域访问自己的接口不该 404")
	}
	if code := send("agent.test", "/admin/api/me"); code != http.StatusNotFound {
		t.Fatalf("agent 域访问 /admin 应 404，得到 %d", code)
	}
	if code := send("agent.test", "/agent/api/me"); code == http.StatusNotFound {
		t.Fatal("agent 域访问自己的接口不该 404")
	}
}
