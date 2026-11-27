package httpapi_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// 版本与公告要能改、能删，正文支持多行。
func TestEditAndDeleteContent(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "7.8.1.1")

	// ---------- 版本 ----------
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/releases?app_id=1", csrf, "7.8.1.1",
		`{"version":"1.0.0","info":"第一行\n第二行","url":"https://example.com/a","must":false,"rollout_bp":10000}`); code != http.StatusOK {
		t.Fatalf("发版本失败: %d %s", code, body)
	}
	var relID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM releases WHERE version = '1.0.0'`).Scan(&relID); err != nil {
		t.Fatalf("查版本失败: %v", err)
	}
	var info string
	_ = e.st.DB().QueryRow(`SELECT info FROM releases WHERE id = ?`, relID).Scan(&info)
	if !strings.Contains(info, "\n") {
		t.Fatalf("正文应保留换行: %q", info)
	}

	code, body := e.do(t, c, http.MethodPut, "/admin/api/releases/"+strconv.FormatInt(relID, 10), csrf, "7.8.1.1",
		`{"version":"1.0.1","info":"改过的\n两行","url":"https://example.com/b","must":true,"rollout_bp":5000}`)
	if code != http.StatusOK {
		t.Fatalf("改版本失败: %d %s", code, body)
	}
	var version, newInfo string
	var must, rollout int64
	_ = e.st.DB().QueryRow(`SELECT version, info, must, rollout_bp FROM releases WHERE id = ?`, relID).
		Scan(&version, &newInfo, &must, &rollout)
	if version != "1.0.1" || must != 1 || rollout != 5000 || !strings.Contains(newInfo, "\n") {
		t.Fatalf("版本没改对: %s %d %d %q", version, must, rollout, newInfo)
	}

	if code, body := e.do(t, c, http.MethodDelete, "/admin/api/releases/"+strconv.FormatInt(relID, 10), csrf, "7.8.1.1", ""); code != http.StatusOK {
		t.Fatalf("删版本失败: %d %s", code, body)
	}
	var n int64
	_ = e.st.DB().QueryRow(`SELECT COUNT(*) FROM releases WHERE id = ?`, relID).Scan(&n)
	if n != 0 {
		t.Fatal("版本没删掉")
	}

	// ---------- 公告 ----------
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/notices?app_id=1", csrf, "7.8.1.1",
		`{"kind":"popup","title":"维护通知","body":"今晚 23:00 维护\n预计 30 分钟"}`); code != http.StatusOK {
		t.Fatalf("发公告失败: %d %s", code, body)
	}
	var noticeID int64
	_ = e.st.DB().QueryRow(`SELECT id FROM notices WHERE title = '维护通知'`).Scan(&noticeID)

	code, body = e.do(t, c, http.MethodPut, "/admin/api/notices/"+strconv.FormatInt(noticeID, 10), csrf, "7.8.1.1",
		`{"kind":"list","title":"维护通知（已改）","body":"改过的正文\n第二行"}`)
	if code != http.StatusOK {
		t.Fatalf("改公告失败: %d %s", code, body)
	}
	var title, nb string
	_ = e.st.DB().QueryRow(`SELECT title, body FROM notices WHERE id = ?`, noticeID).Scan(&title, &nb)
	if title != "维护通知（已改）" || !strings.Contains(nb, "\n") {
		t.Fatalf("公告没改对: %q %q", title, nb)
	}

	if code, body := e.do(t, c, http.MethodDelete, "/admin/api/notices/"+strconv.FormatInt(noticeID, 10), csrf, "7.8.1.1", ""); code != http.StatusOK {
		t.Fatalf("删公告失败: %d %s", code, body)
	}
	_ = e.st.DB().QueryRow(`SELECT COUNT(*) FROM notices WHERE id = ?`, noticeID).Scan(&n)
	if n != 0 {
		t.Fatal("公告没删掉")
	}

	// 删不存在的要 404，而不是静默成功
	if code, _ := e.do(t, c, http.MethodDelete, "/admin/api/releases/999999", csrf, "7.8.1.1", ""); code != http.StatusNotFound {
		t.Fatalf("删不存在的版本应 404，得到 %d", code)
	}
}
