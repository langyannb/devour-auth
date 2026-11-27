package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// 数据大屏接口：管理员看到全量分布，代理只看到自己范围内的。
func TestAnalyticsEndpoints(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "7.7.7.1")

	if code, body := e.do(t, c, http.MethodPost, "/admin/api/cards/quick?app_id=1", csrf, "7.7.7.1",
		`{"kind":"week","qty":1,"multiplier":1,"device_limit":2}`); code != http.StatusOK {
		t.Fatalf("发卡失败: %d %s", code, body)
	}

	code, body := e.do(t, c, http.MethodGet, "/admin/api/analytics?app_id=1&days=30", "", "7.7.7.1", "")
	if code != http.StatusOK {
		t.Fatalf("管理端 analytics: %d %s", code, body)
	}
	if !strings.Contains(body, `"cards_total":1`) || !strings.Contains(body, `"by_kind":[{"key":`) {
		t.Fatalf("管理端 analytics 内容不对: %s", body)
	}

	// 建一个代理（没发过卡），代理端只应看到空分布
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/agents?app_id=1", csrf, "7.7.7.1",
		`{"name":"ana-agent","password":"agent-pass-123456","monthly_quota_cents":1000}`); code != http.StatusOK {
		t.Fatalf("建代理失败: %d %s", code, body)
	}
	ac, acsrf := e.loginAgentWithPassword(t, "ana-agent", "agent-pass-123456")
	code, body = e.do(t, ac, http.MethodGet, "/agent/api/analytics?days=30", acsrf, "", "")
	if code != http.StatusOK {
		t.Fatalf("代理端 analytics: %d %s", code, body)
	}
	if !strings.Contains(body, `"by_kind":[]`) || !strings.Contains(body, `"trend":[]`) {
		t.Fatalf("代理不应看到别人的卡: %s", body)
	}
}
