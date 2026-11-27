package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// /v1/meta 免登录，后台改公告/变量后立刻反映。
func TestMetaReflectsAdminContent(t *testing.T) {
	e := setup(t)
	anon := &http.Client{}

	code, body := e.do(t, anon, http.MethodGet, "/v1/meta", "", "", "")
	if code != http.StatusOK {
		t.Fatalf("meta = %d %s", code, body)
	}
	if !strings.Contains(body, `"app":"devour"`) || !strings.Contains(body, `"maintenance":false`) {
		t.Fatalf("meta 基础字段不对: %s", body)
	}
	if !strings.Contains(body, "ticket_pubkey") {
		t.Fatalf("缺少票据公钥: %s", body)
	}

	c, csrf := e.login(t, "admin", "3.3.3.3")

	if code, b := e.do(t, c, http.MethodPost, "/admin/api/notices", csrf, "3.3.3.3",
		`{"kind":"list","title":"维护通知","body":"今晚 23:00 维护"}`); code != http.StatusOK {
		t.Fatalf("建公告: %d %s", code, b)
	}
	if code, b := e.do(t, c, http.MethodPut, "/admin/api/flags", csrf, "3.3.3.3",
		`{"key":"maintenance","value":"1"}`); code != http.StatusOK {
		t.Fatalf("设变量: %d %s", code, b)
	}
	if code, b := e.do(t, c, http.MethodPost, "/admin/api/releases", csrf, "3.3.3.3",
		`{"version":"2.0.1","info":"修复登录","must":true,"rollout_bp":10000}`); code != http.StatusOK {
		t.Fatalf("建版本: %d %s", code, b)
	}

	_, body = e.do(t, anon, http.MethodGet, "/v1/meta?fp=device-abc", "", "", "")
	if !strings.Contains(body, "今晚 23:00 维护") {
		t.Fatalf("公告未下发: %s", body)
	}
	if !strings.Contains(body, `"maintenance":true`) {
		t.Fatalf("维护开关未生效: %s", body)
	}
	if !strings.Contains(body, `"version":"2.0.1"`) || !strings.Contains(body, `"must":true`) {
		t.Fatalf("版本未下发: %s", body)
	}
}

func TestMetaRolloutFiltering(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "4.4.4.4")

	// 灰度 0%：没有设备指纹时不下发
	if code, b := e.do(t, c, http.MethodPost, "/admin/api/releases", csrf, "4.4.4.4",
		`{"version":"3.0.0","rollout_bp":1}`); code != http.StatusOK {
		t.Fatalf("建版本: %d %s", code, b)
	}
	_, body := e.do(t, &http.Client{}, http.MethodGet, "/v1/meta", "", "", "")
	if strings.Contains(body, `"version":"3.0.0"`) {
		t.Fatalf("无指纹时不该下发灰版本: %s", body)
	}
}

// 离线票据需要 access token，未带时 401。
func TestOfflineTicketRequiresToken(t *testing.T) {
	e := setup(t)
	code, _ := e.do(t, &http.Client{}, http.MethodPost, "/v1/offline-ticket", "", "", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401，得到 %d", code)
	}
}
