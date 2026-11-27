package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/langyannb/devour-auth/internal/httpapi"
	"github.com/langyannb/devour-auth/internal/store"
)

type captchaResp struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}

// fakeCaptcha 用钩子绕过 OCR：每次签发固定答案 1234，验证一次即失效。
func fakeCaptcha() (func() (string, string), func(string, string) bool) {
	var mu sync.Mutex
	codes := map[string]string{}
	n := 0
	newFn := func() (string, string) {
		mu.Lock()
		defer mu.Unlock()
		n++
		id := fmt.Sprintf("cap-%d", n)
		codes[id] = "1234"
		return id, "data:image/png;base64,FAKE"
	}
	verifyFn := func(id, code string) bool {
		mu.Lock()
		defer mu.Unlock()
		want, ok := codes[id]
		delete(codes, id)
		return ok && want == code
	}
	return newFn, verifyFn
}

func TestQueryCaptchaDefaultImplementation(t *testing.T) {
	e := setup(t)
	code, body := e.do(t, &http.Client{}, http.MethodGet, "/v1/query/captcha", "", "9.9.9.6", "")
	if code != http.StatusOK {
		t.Fatalf("默认验证码接口应 200，得到 %d %s", code, body)
	}
	var got struct {
		Data captchaResp `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("解析失败: %v %s", err, body)
	}
	if got.Data.ID == "" {
		t.Fatalf("缺少 id: %s", body)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got.Data.Image, "data:image/png;base64,"))
	if err != nil || !bytes.HasPrefix(raw, []byte("\x89PNG")) {
		t.Fatalf("内置实现未返回 PNG: %v", err)
	}
}

func TestQueryCaptchaIssued(t *testing.T) {
	newFn, verifyFn := fakeCaptcha()
	e := setup(t, func(o *httpapi.Options) {
		o.CaptchaNew, o.CaptchaVerify = newFn, verifyFn
	})

	code, body := e.do(t, &http.Client{}, http.MethodGet, "/v1/query/captcha", "", "9.9.9.1", "")
	if code != http.StatusOK {
		t.Fatalf("验证码接口应 200，得到 %d %s", code, body)
	}
	var got struct {
		Data captchaResp `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("解析失败: %v %s", err, body)
	}
	if got.Data.ID == "" {
		t.Fatalf("缺少 id: %s", body)
	}
	if !strings.HasPrefix(got.Data.Image, "data:image/png;base64,") {
		t.Fatalf("图片不是 data URI: %s", got.Data.Image)
	}
}

func TestQueryCaptchaRequired(t *testing.T) {
	newFn, verifyFn := fakeCaptcha()
	e := setup(t, func(o *httpapi.Options) {
		o.CaptchaNew, o.CaptchaVerify = newFn, verifyFn
	})
	ctx := context.Background()

	pid, err := e.st.CreatePolicy(ctx, store.Policy{
		AppID: 1, Code: "qcap", Name: "查卡测试", Kind: store.KindDuration,
		DurationDays: 1, DeviceLimit: 1, ConcurrentLimit: 1, HeartbeatSec: 600,
		Active: 1, PriceCents: 100, CreatedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后为 no-op
	if _, err := e.st.InsertCard(ctx, tx, store.Card{
		AppID: 1, Kami: "DV-CAP-TEST", PolicyID: pid, Kind: store.KindDuration,
		Status: store.CardActive, DeviceLimit: 1,
		ExpireAt: time.Now().Add(time.Hour).Unix(), SnapshotJSON: "{}", CreatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	ask := func(ip, body string) (int, string) {
		return e.do(t, &http.Client{}, http.MethodPost, "/v1/query", "", ip, body)
	}

	// 不带验证码
	code, body := ask("9.9.9.2", `{"kami":"DV-CAP-TEST"}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "验证码错误") {
		t.Fatalf("缺验证码应 400，得到 %d %s", code, body)
	}

	// 带 id、错码
	if code, body = ask("9.9.9.3", `{"kami":"DV-CAP-TEST","captcha_id":"cap-x","captcha_code":"0000"}`); code != http.StatusBadRequest {
		t.Fatalf("错码应 400，得到 %d %s", code, body)
	}

	// 正常签发 → 对码，应 200
	_, raw := e.do(t, &http.Client{}, http.MethodGet, "/v1/query/captcha", "", "9.9.9.4", "")
	var issued struct {
		Data captchaResp `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &issued); err != nil {
		t.Fatal(err)
	}
	okBody := fmt.Sprintf(`{"kami":"DV-CAP-TEST","captcha_id":%q,"captcha_code":"1234"}`, issued.Data.ID)
	if code, body = ask("9.9.9.4", okBody); code != http.StatusOK {
		t.Fatalf("对码应 200，得到 %d %s", code, body)
	}

	// 一次有效：同一 id 再验应 400
	if code, body = ask("9.9.9.5", okBody); code != http.StatusBadRequest {
		t.Fatalf("验证码应一次性，得到 %d %s", code, body)
	}
}
