package httpapi_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/httpapi"
	"github.com/langyannb/devour-auth/internal/store"
)

const testPass = "a-very-long-test-password"

type env struct {
	ts *httptest.Server
	st *store.Store
}

func setup(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	hash, err := auth.HashPassword(testPass)
	if err != nil {
		t.Fatal(err)
	}
	// 测试账号不带 TOTP，登录只要密码
	if _, err := st.CreateAdmin(ctx, "admin", hash, "admin", "", 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAdmin(ctx, "op", hash, "operator", "", 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureApp(ctx, "devour", "Devour", 1000); err != nil {
		t.Fatal(err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv := httpapi.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), httpapi.Options{
		Env:       "dev",
		Static:    fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>test</title>")}},
		TicketKey: priv,
		AdminHost: "admin.test",
		AgentHost: "agent.test",
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return env{ts: ts, st: st}
}

type loginResp struct {
	OK   bool `json:"ok"`
	Data struct {
		CSRFToken string `json:"csrf_token"`
	} `json:"data"`
}

func (e env) login(t *testing.T, user, xff string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Jar: jar}
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, user, testPass)
	req, _ := http.NewRequest(http.MethodPost, e.ts.URL+"/admin/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", xff)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out loginResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatalf("%s 登录失败: %d", user, resp.StatusCode)
	}
	return c, out.Data.CSRFToken
}

func (e env) do(t *testing.T, c *http.Client, method, path, csrf, xff, body string) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestAdminRequiresSession(t *testing.T) {
	e := setup(t)
	c := &http.Client{}
	if code, _ := e.do(t, c, http.MethodGet, "/admin/api/me", "", "7.7.7.7", ""); code != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，得到 %d", code)
	}
}

func TestCSRFRequiredForWrites(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "1.1.1.1")

	if code, _ := e.do(t, c, http.MethodPost, "/admin/api/logout", "", "1.1.1.1", ""); code != http.StatusForbidden {
		t.Fatalf("缺 CSRF 应 403，得到 %d", code)
	}
	if code, _ := e.do(t, c, http.MethodGet, "/admin/api/me", "", "1.1.1.1", ""); code != http.StatusOK {
		t.Fatalf("GET 不需要 CSRF，得到 %d", code)
	}
	if code, _ := e.do(t, c, http.MethodPost, "/admin/api/logout", csrf, "1.1.1.1", ""); code != http.StatusOK {
		t.Fatalf("带 CSRF 应 200，得到 %d", code)
	}
}

// 验收项 9：operator 会话调 admin 专属接口 → 403。
func TestOperatorCannotAdminOnlyRoutes(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "op", "2.2.2.2")

	code, body := e.do(t, c, http.MethodPost, "/admin/api/policies?app_id=1", csrf, "2.2.2.2",
		`{"code":"x","name":"x","kind":"forever"}`)
	if code != http.StatusForbidden {
		t.Fatalf("operator 改策略应 403，得到 %d %s", code, body)
	}
	// 只读接口允许
	if code, _ := e.do(t, c, http.MethodGet, "/admin/api/cards", "", "2.2.2.2", ""); code != http.StatusOK {
		t.Fatalf("operator 查卡应 200，得到 %d", code)
	}
}

// 验收项 14：伪造会话 cookie 换 IP → 会话失效。
func TestAdminSessionBoundToIP(t *testing.T) {
	e := setup(t)
	c, _ := e.login(t, "admin", "1.1.1.1")
	if code, _ := e.do(t, c, http.MethodGet, "/admin/api/me", "", "1.1.1.1", ""); code != http.StatusOK {
		t.Fatalf("同 IP 应 200，得到 %d", code)
	}
	if code, _ := e.do(t, c, http.MethodGet, "/admin/api/me", "", "9.9.9.9", ""); code != http.StatusUnauthorized {
		t.Fatalf("换 IP 应 401，得到 %d", code)
	}
	// 换回去也应失效（会话已吊销）
	if code, _ := e.do(t, c, http.MethodGet, "/admin/api/me", "", "1.1.1.1", ""); code != http.StatusUnauthorized {
		t.Fatalf("吊销后原 IP 也应 401，得到 %d", code)
	}
}

func TestHealthz(t *testing.T) {
	e := setup(t)
	resp, err := http.Get(e.ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["ok"] != true {
		t.Fatalf("healthz 内容: %v", out)
	}
}
