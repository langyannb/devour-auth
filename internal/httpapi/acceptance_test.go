package httpapi_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

// loginAgent 建一个代理账号并登录，返回带会话的客户端。
func (e env) loginAgent(t *testing.T, name string) *http.Client {
	t.Helper()
	hash, err := auth.HashPassword(testPass)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateAgent(context.Background(), store.Agent{
		Name: name, Level: 1, QuotaCents: -1, PassHash: hash, Status: "active", CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	body := fmt.Sprintf(`{"name":%q,"password":%q}`, name, testPass)
	resp, err := c.Post(e.ts.URL+"/agent/api/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("代理登录 = %d", resp.StatusCode)
	}
	return c
}

// 验收项 10：代理会话调管理端接口 → 403，不能串门。
func TestAgentSessionCannotUseAdminRoutes(t *testing.T) {
	e := setup(t)
	c := e.loginAgent(t, "ag403")

	if code, _ := e.do(t, c, http.MethodGet, "/admin/api/me", "", "4.4.4.4", ""); code != http.StatusForbidden {
		t.Fatalf("代理调管理端应 403，得到 %d", code)
	}
}

// 验收项 11：代理改自己的折扣/额度 → 403，且数据不变。
func TestAgentCannotPatchOwnDiscount(t *testing.T) {
	e := setup(t)
	c := e.loginAgent(t, "agpatch")
	before := 0
	if err := e.st.DB().QueryRow(`SELECT discount_bp FROM agents WHERE name = 'agpatch'`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	code, body := e.do(t, c, http.MethodPatch, "/admin/api/agents/1", "", "4.4.4.4", `{"discount_bp":1}`)
	if code != http.StatusForbidden {
		t.Fatalf("代理改自己折扣应 403，得到 %d %s", code, body)
	}
	after := 0
	if err := e.st.DB().QueryRow(`SELECT discount_bp FROM agents WHERE name = 'agpatch'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("discount 被改动: %d -> %d", before, after)
	}
}

// 验收项 12：直接 UPDATE/DELETE audit_log 被触发器拒绝。
func TestAuditLogIsAppendOnly(t *testing.T) {
	e := setup(t)
	db := e.st.DB()
	if _, err := db.Exec(`INSERT INTO audit_log(ts, actor, action, target, prev_hash, hash) VALUES(1,'x','y','z','','')`); err != nil {
		t.Fatalf("插入审计: %v", err)
	}
	if _, err := db.Exec(`UPDATE audit_log SET action = 'tampered'`); err == nil {
		t.Fatal("UPDATE audit_log 应被拒绝")
	}
	if _, err := db.Exec(`DELETE FROM audit_log`); err == nil {
		t.Fatal("DELETE audit_log 应被拒绝")
	}
}

// 验收项 15：超过 reauth 窗口的敏感操作 → 403。
func TestReauthRequiredForSensitiveOps(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "5.5.5.5")
	// 把会话的 reauth 窗口置为过期
	if _, err := e.st.DB().Exec(`UPDATE admin_sessions SET reauth_until = 0`); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(t, c, http.MethodPost, "/admin/api/agents", csrf, "5.5.5.5",
		`{"name":"nope","password":"a-very-long-password"}`)
	if code != http.StatusForbidden {
		t.Fatalf("未 reauth 建代理应 403，得到 %d %s", code, body)
	}
}

// 公开查卡页要能显示卡多长（天卡/周卡/月卡）。
func TestClientQueryReturnsDuration(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "6.6.6.6")
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/cards/quick?app_id=1", csrf, "6.6.6.6",
		`{"kind":"week","qty":1,"multiplier":1,"device_limit":1}`); code != http.StatusOK {
		t.Fatalf("发卡失败: %d %s", code, body)
	}
	var kami string
	if err := e.st.DB().QueryRow(`SELECT kami FROM cards ORDER BY id DESC LIMIT 1`).Scan(&kami); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(t, &http.Client{}, http.MethodPost, "/v1/query", "", "6.6.6.6",
		fmt.Sprintf(`{"kami":%q}`, kami))
	if code != http.StatusOK {
		t.Fatalf("查卡失败: %d %s", code, body)
	}
	if !strings.Contains(body, `"duration_seconds":604800`) {
		t.Fatalf("查卡没返回时长: %s", body)
	}
}

// 卡密导出要拉全量，不能被单页 200 条截断。
func TestCardsExportAllPages(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "7.7.7.7")
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/cards/quick?app_id=1", csrf, "7.7.7.7",
		`{"kind":"day","qty":201,"multiplier":1,"device_limit":1}`); code != http.StatusOK {
		t.Fatalf("发卡失败: %d %s", code, body)
	}

	code, body := e.do(t, c, http.MethodGet, "/admin/api/cards/export", "", "7.7.7.7", "")
	if code != http.StatusOK {
		t.Fatalf("导出 = %d %s", code, body)
	}
	rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// 201 行数据 + 表头
	if len(rows) != 202 {
		t.Fatalf("导出行数 = %d want 202", len(rows))
	}
	if !strings.HasSuffix(rows[0][0], "id") || rows[0][4] != "duration_seconds" {
		t.Fatalf("表头不对: %v", rows[0])
	}
	if rows[1][4] != "86400" {
		t.Fatalf("duration_seconds = %q want 86400", rows[1][4])
	}
}
