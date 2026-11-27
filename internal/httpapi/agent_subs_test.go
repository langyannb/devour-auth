package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// 一级代理可以设置下级：模板（不低于自己成本）、额度划转、冻结。
func TestAgentManagesSubAgent(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "5.1.1.1")

	mkPlan := func(name string, day int64) int64 {
		body := fmt.Sprintf(`{"name":%q,"items":{"day":%d}}`, name, day)
		if code, resp := e.do(t, c, http.MethodPost, "/admin/api/plans?app_id=1", csrf, "5.1.1.1", body); code != http.StatusOK {
			t.Fatalf("建模板失败: %d %s", code, resp)
		}
		var id int64
		if err := e.st.DB().QueryRow(`SELECT id FROM agent_plans WHERE name = ?`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	cheap := mkPlan("cheap", 200)
	mine := mkPlan("mine", 300)
	pricey := mkPlan("pricey", 500)

	// 一级代理，额度 1000 分
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/agents?app_id=1", csrf, "5.1.1.1",
		`{"name":"top","password":"agent-pass-123456","monthly_quota_cents":1000}`); code != http.StatusOK {
		t.Fatalf("建代理失败: %d %s", code, body)
	}
	var topID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM agents WHERE name='top'`).Scan(&topID); err != nil {
		t.Fatal(err)
	}
	if code, body := e.do(t, c, http.MethodPut, fmt.Sprintf("/admin/api/agents/%d/plan?app_id=1", topID), csrf, "5.1.1.1",
		fmt.Sprintf(`{"plan_id":%d,"max_qty":10,"set_device_limit":true,"custom_kami":true}`, mine)); code != http.StatusOK {
		t.Fatalf("绑模板失败: %d %s", code, body)
	}

	ac, acsrf := e.loginAgentWithPassword(t, "top", "agent-pass-123456")
	if code, body := e.do(t, ac, http.MethodPost, "/agent/api/subagents", acsrf, "",
		`{"name":"sub1","password":"sub-pass-123456"}`); code != http.StatusOK {
		t.Fatalf("建下级失败: %d %s", code, body)
	}
	var subID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM agents WHERE name='sub1'`).Scan(&subID); err != nil {
		t.Fatal(err)
	}
	// 下级继承同一个应用
	var subApp int64
	if err := e.st.DB().QueryRow(`SELECT app_id FROM agents WHERE id=?`, subID).Scan(&subApp); err != nil {
		t.Fatal(err)
	}
	if subApp != 1 {
		t.Fatalf("下级应用 = %d want 1", subApp)
	}

	// 低于成本的价格被拒
	code, body := e.do(t, ac, http.MethodPut, fmt.Sprintf("/agent/api/subagents/%d/plan", subID), acsrf, "",
		fmt.Sprintf(`{"plan_id":%d}`, cheap))
	if code != http.StatusBadRequest || !strings.Contains(body, "不能低于你的成本") {
		t.Fatalf("低价模板应被拒: %d %s", code, body)
	}
	// 不低于成本的可以
	code, body = e.do(t, ac, http.MethodPut, fmt.Sprintf("/agent/api/subagents/%d/plan", subID), acsrf, "",
		fmt.Sprintf(`{"plan_id":%d}`, pricey))
	if code != http.StatusOK {
		t.Fatalf("设模板失败: %d %s", code, body)
	}

	// 划额度 400 分
	if code, body = e.do(t, ac, http.MethodPost, fmt.Sprintf("/agent/api/subagents/%d/quota", subID), acsrf, "",
		`{"delta_cents":400}`); code != http.StatusOK {
		t.Fatalf("划额度失败: %d %s", code, body)
	}
	var topQuota, subQuota int64
	_ = e.st.DB().QueryRow(`SELECT quota_cents FROM agents WHERE id=?`, topID).Scan(&topQuota)
	_ = e.st.DB().QueryRow(`SELECT quota_cents FROM agents WHERE id=?`, subID).Scan(&subQuota)
	if topQuota != 600 || subQuota != 400 {
		t.Fatalf("额度不对: top=%d sub=%d", topQuota, subQuota)
	}
	// 余额不足
	if code, _ = e.do(t, ac, http.MethodPost, fmt.Sprintf("/agent/api/subagents/%d/quota", subID), acsrf, "",
		`{"delta_cents":99999}`); code != http.StatusBadRequest {
		t.Fatalf("超额划转应 400，得到 %d", code)
	}

	// 冻结下级
	if code, body = e.do(t, ac, http.MethodPost, fmt.Sprintf("/agent/api/subagents/%d/freeze", subID), acsrf, "", ""); code != http.StatusOK {
		t.Fatalf("冻结失败: %d %s", code, body)
	}
	var status string
	_ = e.st.DB().QueryRow(`SELECT status FROM agents WHERE id=?`, subID).Scan(&status)
	if status != "frozen" {
		t.Fatalf("状态 = %s", status)
	}
}

// 下级通过专属链接首次设置密码，然后正常登录；链接只能用一次。
func TestSubAgentInviteLink(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "5.2.1.1")
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/agents?app_id=1", csrf, "5.2.1.1",
		`{"name":"inv-top","password":"agent-pass-123456","monthly_quota_cents":1000}`); code != http.StatusOK {
		t.Fatalf("建代理失败: %d %s", code, body)
	}
	ac, acsrf := e.loginAgentWithPassword(t, "inv-top", "agent-pass-123456")

	code, body := e.do(t, ac, http.MethodPost, "/agent/api/subagents", acsrf, "",
		`{"name":"inv-sub"}`)
	if code != http.StatusOK {
		t.Fatalf("建下级失败: %d %s", code, body)
	}
	var created struct {
		Data struct {
			ID        int64  `json:"id"`
			LoginLink string `json:"login_link"`
			Activated bool   `json:"activated"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.LoginLink == "" || created.Data.Activated {
		t.Fatalf("链接/激活状态不对: %s", body)
	}
	var slug string
	if err := e.st.DB().QueryRow(`SELECT login_slug FROM agents WHERE id=?`, created.Data.ID).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug == "" {
		t.Fatal("没有生成 slug")
	}

	// 打开链接：未激活
	code, body = e.do(t, &http.Client{}, http.MethodGet, "/agent/api/invite/"+slug, "", "", "")
	if code != http.StatusOK || !strings.Contains(body, `"activated":false`) {
		t.Fatalf("邀请信息不对: %d %s", code, body)
	}
	// 设置密码
	code, body = e.do(t, &http.Client{}, http.MethodPost, "/agent/api/invite/"+slug, "", "",
		`{"password":"sub-pass-123456"}`)
	if code != http.StatusOK || !strings.Contains(body, "csrf_token") {
		t.Fatalf("设置密码失败: %d %s", code, body)
	}
	// 能正常登录
	if _, csrf2 := e.loginAgentWithPassword(t, "inv-sub", "sub-pass-123456"); csrf2 == "" {
		t.Fatal("激活后应能登录")
	}
	// 再点链接：已激活
	code, body = e.do(t, &http.Client{}, http.MethodPost, "/agent/api/invite/"+slug, "", "",
		`{"password":"another-pass-123"}`)
	if code != http.StatusConflict {
		t.Fatalf("重复激活应 409: %d %s", code, body)
	}
}

func jsonUnmarshal(s string, v any) error {
	return json.NewDecoder(strings.NewReader(s)).Decode(v)
}

// 老下级没有 slug 时，点「登录链接」能补一个，并且可以正常打开。
func TestSubAgentEnsureLink(t *testing.T) {
	e := setup(t)
	c, csrf := e.login(t, "admin", "5.3.1.1")
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/agents?app_id=1", csrf, "5.3.1.1",
		`{"name":"old-top","password":"agent-pass-123456","monthly_quota_cents":1000}`); code != http.StatusOK {
		t.Fatalf("建代理失败: %d %s", code, body)
	}
	var topID int64
	_ = e.st.DB().QueryRow(`SELECT id FROM agents WHERE name='old-top'`).Scan(&topID)
	if code, body := e.do(t, c, http.MethodPost, "/admin/api/agents?app_id=1", csrf, "5.3.1.1",
		fmt.Sprintf(`{"name":"old-sub","password":"sub-pass-123456","parent_id":%d}`, topID)); code != http.StatusOK {
		t.Fatalf("建下级失败: %d %s", code, body)
	}
	var subID int64
	_ = e.st.DB().QueryRow(`SELECT id FROM agents WHERE name='old-sub'`).Scan(&subID)
	// 模拟老数据：没有 slug
	if _, err := e.st.DB().Exec(`UPDATE agents SET login_slug = NULL WHERE id = ?`, subID); err != nil {
		t.Fatal(err)
	}

	ac, acsrf := e.loginAgentWithPassword(t, "old-top", "agent-pass-123456")
	code, body := e.do(t, ac, http.MethodPost, fmt.Sprintf("/agent/api/subagents/%d/link", subID), acsrf, "", "")
	if code != http.StatusOK || !strings.Contains(body, "login_link") {
		t.Fatalf("补链接失败: %d %s", code, body)
	}
	var slug string
	_ = e.st.DB().QueryRow(`SELECT login_slug FROM agents WHERE id=?`, subID).Scan(&slug)
	if slug == "" {
		t.Fatal("slug 未写入")
	}
	// 再点一次返回同一个链接
	_, body2 := e.do(t, ac, http.MethodPost, fmt.Sprintf("/agent/api/subagents/%d/link", subID), acsrf, "", "")
	if !strings.Contains(body2, slug) {
		t.Fatalf("应返回同一个链接: %s", body2)
	}
}
