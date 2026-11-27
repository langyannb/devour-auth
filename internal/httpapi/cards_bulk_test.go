package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// 按范围批量调时：只动有时长的卡，扣到 0 置为已过期，清理只回收已过期卡。
func TestBulkCardTimeAndRecycle(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "7.1.1.1")
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/cards/quick?app_id=1", csrf, "7.1.1.1",
		`{"kind":"day","qty":3,"multiplier":1,"device_limit":1}`); code != http.StatusOK {
		t.Fatalf("发卡失败: %d %s", code, body)
	}

	var ids []int64
	rows, err := e.st.DB().Query(`SELECT id FROM cards ORDER BY id LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 3 {
		t.Fatalf("卡数 = %d want 3", len(ids))
	}
	// 卡 1 已激活、卡 2 已过期、卡 3 未激活（无时长）
	if err := execSQL(e, `UPDATE cards SET status='active', expire_at=unixepoch()+3600 WHERE id=?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(e, `UPDATE cards SET status='expired', expire_at=unixepoch()-100 WHERE id=?`, ids[1]); err != nil {
		t.Fatal(err)
	}

	// 预览：已激活 1 张可调时
	code, body := e.do(t, c, http.MethodGet, "/admin/api/cards/bulk-preview?app_id=1&scope=active", "", "7.1.1.1", "")
	if code != http.StatusOK || !strings.Contains(body, `"total":1`) || !strings.Contains(body, `"time_cards":1`) {
		t.Fatalf("预览不对: %d %s", code, body)
	}

	// 加 2 小时
	code, body = e.do(t, c, http.MethodPost, "/admin/api/cards/bulk?app_id=1", csrf, "7.1.1.1",
		`{"scope":"active","action":"add","seconds":7200}`)
	if code != http.StatusOK || !strings.Contains(body, `"affected":1`) {
		t.Fatalf("加时失败: %d %s", code, body)
	}
	var before int64
	if err := e.st.DB().QueryRow(`SELECT expire_at FROM cards WHERE id=?`, ids[0]).Scan(&before); err != nil {
		t.Fatal(err)
	}

	// 扣 100 小时：超过剩余，卡变已过期
	code, body = e.do(t, c, http.MethodPost, "/admin/api/cards/bulk?app_id=1", csrf, "7.1.1.1",
		`{"scope":"active","action":"subtract","seconds":360000}`)
	if code != http.StatusOK {
		t.Fatalf("扣时失败: %d %s", code, body)
	}
	var status string
	var expire int64
	if err := e.st.DB().QueryRow(`SELECT status, expire_at FROM cards WHERE id=?`, ids[0]).Scan(&status, &expire); err != nil {
		t.Fatal(err)
	}
	if status != "expired" || expire > before {
		t.Fatalf("扣光后应过期，得到 %s %d", status, expire)
	}

	// 清理已过期：卡 1 + 卡 2
	code, body = e.do(t, c, http.MethodPost, "/admin/api/cards/bulk?app_id=1", csrf, "7.1.1.1",
		`{"scope":"expired","action":"recycle"}`)
	if code != http.StatusOK || !strings.Contains(body, `"affected":2`) {
		t.Fatalf("清理失败: %d %s", code, body)
	}
	var recycled int
	if err := e.st.DB().QueryRow(`SELECT COUNT(*) FROM cards WHERE status='recycled'`).Scan(&recycled); err != nil {
		t.Fatal(err)
	}
	if recycled != 2 {
		t.Fatalf("回收数 = %d want 2", recycled)
	}
	var action string
	if err := e.st.DB().QueryRow(`SELECT action FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != "card.bulk_recycle" {
		t.Fatalf("审计 = %s", action)
	}
}

func execSQL(e env, q string, args ...any) error {
	_, err := e.st.DB().Exec(fmt.Sprint(q), args...)
	return err
}
