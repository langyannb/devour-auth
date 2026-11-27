package httpapi

import (
	"net/http"
	"time"

	"github.com/mojocn/base64Captcha"
)

// queryCaptcha 签发查卡图形验证码：4 位数字、2 分钟有效、一次验证即失效。
// 30 次/分/IP，避免验证码接口自身被刷。
func (s *Server) queryCaptcha(w http.ResponseWriter, r *http.Request) {
	if !s.lim.Allow("query:captcha:ip:"+clientIP(r), 30, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "请求过于频繁")
		return
	}
	id, img := s.captchaNew()
	if id == "" || img == "" {
		writeErr(w, http.StatusInternalServerError, CodeInternal, "验证码生成失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeData(w, map[string]any{"id": id, "image": img})
}

// defaultQueryCaptcha 内置实现：库自带的内存 store，2 分钟过期。
func defaultQueryCaptcha() (func() (id, img string), func(id, code string) bool) {
	c := base64Captcha.NewCaptcha(
		base64Captcha.NewDriverDigit(100, 320, 4, 0.2, 5),
		base64Captcha.NewMemoryStore(base64Captcha.GCLimitNumber, 2*time.Minute),
	)
	newFn := func() (string, string) {
		id, b64, _, err := c.Generate()
		if err != nil {
			return "", ""
		}
		// 库返回的就是 data:image/png;base64,... 完整串。
		return id, b64
	}
	verifyFn := func(id, code string) bool {
		// clear=true：无论对错，该 id 只能验一次。
		return c.Verify(id, code, true)
	}
	return newFn, verifyFn
}
