package httpapi_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/langyannb/devour-auth/internal/httpapi"
	"github.com/langyannb/devour-auth/internal/store"
)

// opsServer 用同一个库另起一个注入了备份目录的服务端。
func opsServer(t *testing.T, st *store.Store, backupDir string) env {
	t.Helper()
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
		BackupDir: backupDir,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return env{ts: ts, st: st}
}

func getOps(t *testing.T, e env) map[string]map[string]any {
	t.Helper()
	c, _ := e.login(t, "admin", "8.8.8.8")
	code, body := e.do(t, c, http.MethodGet, "/admin/api/ops/status", "", "8.8.8.8", "")
	if code != http.StatusOK {
		t.Fatalf("ops/status: %d %s", code, body)
	}
	var out struct {
		OK   bool                      `json:"ok"`
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("解析响应: %v %s", err, body)
	}
	if !out.OK {
		t.Fatalf("ok 应为 true: %s", body)
	}
	return out.Data
}

func requireKeys(t *testing.T, name string, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("%s 缺少字段 %s: %v", name, k, m)
		}
	}
}

func TestOpsStatusShape(t *testing.T) {
	base := setup(t)
	e := opsServer(t, base.st, t.TempDir())

	data := getOps(t, e)
	for _, name := range []string{"anchor", "rollup", "bills", "backup"} {
		m, ok := data[name]
		if !ok {
			t.Fatalf("缺少 %s: %v", name, data)
		}
		requireKeys(t, name, m, "last_at", "next_at")
	}
	requireKeys(t, "backup", data["backup"], "configured")

	// 空库：last 为 0，next 已按调度算出。
	if data["anchor"]["last_at"] != float64(0) || data["rollup"]["last_at"] != float64(0) {
		t.Fatalf("空库 last 应为 0: %v", data)
	}
	for _, name := range []string{"anchor", "rollup", "bills", "backup"} {
		if data[name]["next_at"].(float64) <= 0 {
			t.Fatalf("%s next_at 应大于 0: %v", name, data[name])
		}
	}
}

func TestOpsStatusLastTimes(t *testing.T) {
	base := setup(t)
	e := opsServer(t, base.st, t.TempDir())

	day := "2026-11-26"
	if _, err := base.st.DB().ExecContext(t.Context(),
		`INSERT INTO audit_anchor(day, last_id, last_hash, signature, created_at)
		 VALUES(?, 2, 'hash', 'sig', 1000)`, day); err != nil {
		t.Fatal(err)
	}
	// 只有锚点时，日统计的 last 不能被锚点的 day 串用。
	data := getOps(t, e)
	if data["rollup"]["last_at"] != float64(0) {
		t.Fatalf("日统计无记录时 last 应为 0: %v", data["rollup"])
	}
	if _, err := base.st.DB().ExecContext(t.Context(),
		`INSERT INTO daily_stats(day, activations, logins, unique_devices, errors, online_peak, issued_cents)
		 VALUES('2026-11-25', 1, 2, 3, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}

	data = getOps(t, e)
	wantAnchor := float64(time.Date(2026, 11, 26, 0, 0, 0, 0, time.Local).Unix())
	if got := data["anchor"]["last_at"]; got != wantAnchor {
		t.Fatalf("anchor last_at = %v，想要 %v", got, wantAnchor)
	}
	wantRollup := float64(time.Date(2026, 11, 25, 0, 0, 0, 0, time.Local).Unix())
	if got := data["rollup"]["last_at"]; got != wantRollup {
		t.Fatalf("rollup last_at = %v，想要 %v", got, wantRollup)
	}
}

func TestOpsStatusBackup(t *testing.T) {
	base := setup(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app-2026112700.db.gpg"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	data := getOps(t, opsServer(t, base.st, dir))
	if data["backup"]["configured"] != true {
		t.Fatalf("备份应已配置: %v", data["backup"])
	}
	if data["backup"]["last_at"].(float64) <= 0 {
		t.Fatalf("备份 last_at 应大于 0: %v", data["backup"])
	}

	// 未注入目录时视为未配置。
	data = getOps(t, opsServer(t, base.st, ""))
	if data["backup"]["configured"] != false {
		t.Fatalf("未注入备份目录应 configured=false: %v", data["backup"])
	}
	if data["backup"]["last_at"] != float64(0) || data["backup"]["next_at"] != float64(0) {
		t.Fatalf("未配置时 last/next 应为 0: %v", data["backup"])
	}
}
