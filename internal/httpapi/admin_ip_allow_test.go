package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/langyannb/devour-auth/internal/auth"
)

// loginCode 登录并返回状态码与响应体（不因失败而终止测试）。
func (e env) loginCode(t *testing.T, user, xff string) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, user, testPass)
	return e.do(t, &http.Client{}, http.MethodPost, "/admin/api/login", "", xff, body)
}

func createIPAdmin(t *testing.T, e env, username, ipAllow string) int64 {
	t.Helper()
	hash, err := auth.HashPassword(testPass)
	if err != nil {
		t.Fatal(err)
	}
	id, err := e.st.CreateAdmin(context.Background(), username, hash, "admin", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if ipAllow != "" {
		if err := e.st.SetAdminIPAllow(context.Background(), id, ipAllow); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// 白名单内的 IP / CIDR 能登录；同 /24 的其它 IP 也不行（不做网段容忍）。
func TestAdminIPAllowLogin(t *testing.T) {
	e := setup(t)
	createIPAdmin(t, e, "ipadmin", "10.0.0.5 10.1.0.0/16")

	if code, body := e.loginCode(t, "ipadmin", "10.0.0.5"); code != http.StatusOK {
		t.Fatalf("名单内 IP 登录应 200，得到 %d %s", code, body)
	}
	if code, body := e.loginCode(t, "ipadmin", "10.1.2.3"); code != http.StatusOK {
		t.Fatalf("名单内 CIDR 登录应 200，得到 %d %s", code, body)
	}
	code, body := e.loginCode(t, "ipadmin", "10.0.0.6")
	if code != http.StatusUnauthorized {
		t.Fatalf("同 /24 但不在名单应 401，得到 %d %s", code, body)
	}
	if !strings.Contains(body, "该网络不允许登录管理端") {
		t.Fatalf("拒绝应返回中文提示，得到 %s", body)
	}
	if strings.Contains(body, "10.1.0.0") || strings.Contains(body, "10.0.0.5") {
		t.Fatalf("拒绝响应泄漏了白名单内容: %s", body)
	}
}

// 白名单为空时不限制，清空后立即恢复不限。
func TestAdminIPAllowEmptyMeansUnrestricted(t *testing.T) {
	e := setup(t)
	id := createIPAdmin(t, e, "ipempty", "1.2.3.4")

	if code, body := e.loginCode(t, "ipempty", "9.9.9.9"); code != http.StatusUnauthorized {
		t.Fatalf("有名单时其它 IP 应 401，得到 %d %s", code, body)
	}
	if err := e.st.SetAdminIPAllow(context.Background(), id, ""); err != nil {
		t.Fatal(err)
	}
	if code, body := e.loginCode(t, "ipempty", "9.9.9.9"); code != http.StatusOK {
		t.Fatalf("清空白名单后应不限，得到 %d %s", code, body)
	}
}

// PATCH：非法值 400；合法值写入并回显；不能把自己锁死。
func TestAdminSetIPAllow(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "1.1.1.1")

	code, body := e.do(t, c, http.MethodPatch, "/admin/api/accounts/2/ip", csrf, "1.1.1.1",
		`{"ip_allow":"1.2.3.4, not-an-ip"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("非法白名单应 400，得到 %d %s", code, body)
	}

	code, body = e.do(t, c, http.MethodPatch, "/admin/api/accounts/2/ip", csrf, "1.1.1.1",
		`{"ip_allow":"1.2.3.4 10.0.0.0/8"}`)
	if code != http.StatusOK {
		t.Fatalf("合法白名单应 200，得到 %d %s", code, body)
	}
	var resp struct {
		Data struct {
			ID      int64  `json:"id"`
			IPAllow string `json:"ip_allow"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ID != 2 || resp.Data.IPAllow != "1.2.3.4,10.0.0.0/8" {
		t.Fatalf("返回账号行不对: %s", body)
	}

	// 列表接口要带上 ip_allow
	_, listBody := e.do(t, c, http.MethodGet, "/admin/api/accounts", "", "1.1.1.1", "")
	if !strings.Contains(listBody, `"ip_allow":"1.2.3.4,10.0.0.0/8"`) {
		t.Fatalf("账号列表缺少 ip_allow: %s", listBody)
	}

	// 把自己锁死：当前 IP 不在新名单内 → 拒绝
	code, body = e.do(t, c, http.MethodPatch, "/admin/api/accounts/1/ip", csrf, "1.1.1.1",
		`{"ip_allow":"2.2.2.2"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("自锁白名单应 400，得到 %d %s", code, body)
	}
	if !strings.Contains(body, "当前登录 IP") {
		t.Fatalf("自锁拒绝应说明原因，得到 %s", body)
	}

	// 包含当前 IP 的 CIDR 可以保存
	code, body = e.do(t, c, http.MethodPatch, "/admin/api/accounts/1/ip", csrf, "1.1.1.1",
		`{"ip_allow":"1.1.1.0/24"}`)
	if code != http.StatusOK {
		t.Fatalf("包含当前 IP 应允许，得到 %d %s", code, body)
	}

	var action string
	if err := e.st.DB().QueryRow(
		`SELECT action FROM audit_log WHERE action = 'admin.set_ip_allow' ORDER BY id DESC LIMIT 1`).
		Scan(&action); err != nil {
		t.Fatalf("应写入审计: %v", err)
	}
}
