package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// 列表要能看出卡多长、绑了几台、在线几台（代理反馈「只显示时长卡」）。
func TestCardListShowsDurationDevicesOnline(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "3.3.3.3")

	code, body := e.do(t, c, http.MethodPost, "/admin/api/cards/quick?app_id=1", csrf, "3.3.3.3",
		`{"kind":"week","qty":1,"multiplier":1,"device_limit":2}`)
	if code != http.StatusOK {
		t.Fatalf("发卡失败: %d %s", code, body)
	}

	var cardID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM cards ORDER BY id DESC LIMIT 1`).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := e.st.DB().Exec(
		`INSERT INTO devices(card_id, fingerprint, first_seen, last_seen, banned) VALUES(?,?,?,?,0)`,
		cardID, "fp-1", now, now); err != nil {
		t.Fatal(err)
	}
	var devID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM devices WHERE card_id = ?`, cardID).Scan(&devID); err != nil {
		t.Fatal(err)
	}
	// 在线 = access token 未过期且会话未吊销
	if _, err := e.st.DB().Exec(
		`INSERT INTO sessions(card_id, device_id, access_hash, refresh_hash, access_exp, refresh_exp, last_heartbeat, created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		cardID, devID, "ah-1", "rh-1", now+600, now+86400, now, now); err != nil {
		t.Fatal(err)
	}

	code, body = e.do(t, c, http.MethodGet, "/admin/api/cards", "", "3.3.3.3", "")
	if code != http.StatusOK {
		t.Fatalf("查卡失败: %d %s", code, body)
	}
	var out struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data.Items) == 0 {
		t.Fatal("列表为空")
	}
	row := out.Data.Items[0]
	if row["duration_seconds"] != float64(7*86400) {
		t.Errorf("duration_seconds = %v want 604800", row["duration_seconds"])
	}
	if row["bound_devices"] != float64(1) || row["online_devices"] != float64(1) {
		t.Errorf("devices/online = %v/%v want 1/1", row["bound_devices"], row["online_devices"])
	}
	if fmt.Sprint(row["device_limit"]) != "2" {
		t.Errorf("device_limit = %v want 2", row["device_limit"])
	}
}
