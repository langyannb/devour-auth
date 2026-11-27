package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// 绑定应用的账号只能看自己的应用；客户端登录也必须带对应用。
func TestAdminAppScope(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "9.1.1.1")

	// 建第二个应用
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/apps", csrf, "9.1.1.1",
		`{"code":"friend","name":"朋友的应用"}`); code != http.StatusOK {
		t.Fatalf("建应用失败: %d %s", code, body)
	}
	var friendApp int64
	if err := e.st.DB().QueryRow(`SELECT id FROM apps WHERE code = 'friend'`).Scan(&friendApp); err != nil {
		t.Fatal(err)
	}

	// 两个应用各发一张卡
	issue := func(appID int64) string {
		code, body := e.do(t, c, http.MethodPost, fmt.Sprintf("/admin/api/cards/quick?app_id=%d", appID), csrf, "9.1.1.1",
			`{"kind":"week","qty":1,"multiplier":1,"device_limit":1}`)
		if code != http.StatusOK {
			t.Fatalf("发卡失败(app %d): %d %s", appID, code, body)
		}
		var kami string
		if err := e.st.DB().QueryRow(`SELECT kami FROM cards WHERE app_id = ? ORDER BY id DESC LIMIT 1`, appID).Scan(&kami); err != nil {
			t.Fatal(err)
		}
		return kami
	}
	mineKami := issue(1)
	friendKami := issue(friendApp)

	// 建一个只能管 friend 的账号
	code, body := e.do(t, c, http.MethodPost, "/admin/api/accounts", csrf, "9.1.1.1",
		fmt.Sprintf(`{"username":"friend-dev","password":"`+testPass+`","role":"admin","app_id":%d}`, friendApp))
	if code != http.StatusOK {
		t.Fatalf("建账号失败: %d %s", code, body)
	}
	fc, fcsrf := e.login(t, "friend-dev", "9.1.2.2")

	// 故意传 app_id=1，服务端也只会返回他自己应用的数据
	code, body = e.do(t, fc, http.MethodGet, "/admin/api/cards?app_id=1", "", "9.1.2.2", "")
	if code != http.StatusOK {
		t.Fatalf("查卡失败: %d %s", code, body)
	}
	if !strings.Contains(body, friendKami) || strings.Contains(body, mineKami) {
		t.Fatalf("绑定账号看到了别人的卡: %s", body)
	}

	// 按 id 直接访问别的应用的卡也要 404
	var myCardID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM cards WHERE kami = ?`, mineKami).Scan(&myCardID); err != nil {
		t.Fatal(err)
	}
	if code, _ := e.do(t, fc, http.MethodGet, fmt.Sprintf("/admin/api/cards/%d", myCardID), "", "9.1.2.2", ""); code != http.StatusNotFound {
		t.Fatalf("别人的卡按 id 应该 404，得到 %d", code)
	}
	if code, _ := e.do(t, fc, http.MethodPost, fmt.Sprintf("/admin/api/cards/%d/ban", myCardID), fcsrf, "9.1.2.2", `{"reason":"x"}`); code != http.StatusNotFound {
		t.Fatalf("别人的卡不该能封禁，得到 %d", code)
	}

	// 只有最高权限能管账号：绑定应用的账号一律 403
	if code, _ := e.do(t, fc, http.MethodGet, "/admin/api/accounts", "", "9.1.2.2", ""); code != http.StatusForbidden {
		t.Fatalf("绑定账号不该能看账号列表，得到 %d", code)
	}
	if code, _ := e.do(t, fc, http.MethodPost, "/admin/api/accounts", fcsrf, "9.1.2.2",
		`{"username":"friend-op","password":"`+testPass+`","role":"operator"}`); code != http.StatusForbidden {
		t.Fatalf("绑定账号不该能建账号，得到 %d", code)
	}
	// 审计可以看（只含本应用的 actor），实例级体检仍然拒绝
	if code, _ := e.do(t, fc, http.MethodGet, "/admin/api/audit", "", "9.1.2.2", ""); code != http.StatusOK {
		t.Fatalf("绑定账号应能看自己应用的审计，得到 %d", code)
	}
	if code, _ := e.do(t, fc, http.MethodGet, "/admin/api/health-check", "", "9.1.2.2", ""); code != http.StatusForbidden {
		t.Fatalf("体检是实例级的，应拒绝，得到 %d", code)
	}

	// 应用列表只剩自己的
	code, body = e.do(t, fc, http.MethodGet, "/admin/api/apps", "", "9.1.2.2", "")
	if code != http.StatusOK || strings.Contains(body, `"code":"devour"`) || !strings.Contains(body, `"code":"friend"`) {
		t.Fatalf("应用列表应只剩 friend: %d %s", code, body)
	}

	// 客户端登录：app 必须和卡的应用一致
	login := func(app, kami string) int {
		payload := fmt.Sprintf(`{"app":%q,"kami":%q,"device":{"fingerprint":"fp-1","model":"pixel","brand":"google","os":"14","app_version":"1.0"},"ts":%d,"nonce":%q}`,
			app, kami, time.Now().Unix(), fmt.Sprintf("nonce-%d-%s", time.Now().UnixNano(), app))
		code, _ := e.do(t, &http.Client{}, http.MethodPost, "/v1/auth/login", "", "9.1.3.3", payload)
		return code
	}
	if code := login("friend", friendKami); code != http.StatusOK {
		t.Fatalf("对的 app 应能登录，得到 %d", code)
	}
	if code := login("devour", friendKami); code != http.StatusNotFound {
		t.Fatalf("错的 app 不该认这张卡，得到 %d", code)
	}
	if code := login("friend", mineKami); code != http.StatusNotFound {
		t.Fatalf("别的应用的卡不该能登录，得到 %d", code)
	}

	// 不带 app：只认第一个应用，朋友应用的卡登不了
	payload := fmt.Sprintf(`{"kami":%q,"device":{"fingerprint":"fp-2","model":"pixel","brand":"google","os":"14"},"ts":%d,"nonce":"nonce-default-app-123"}`,
		friendKami, time.Now().Unix())
	if code, _ := e.do(t, &http.Client{}, http.MethodPost, "/v1/auth/login", "", "9.1.3.4", payload); code != http.StatusNotFound {
		t.Fatalf("不带 app 时不应认朋友应用的卡，得到 %d", code)
	}
}
