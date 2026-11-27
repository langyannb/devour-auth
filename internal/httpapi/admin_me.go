package httpapi

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/langyannb/devour-auth/internal/auth"
)

// newTOTPSecret 20 字节随机 → base32（无填充），与验证器兼容。
func newTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func otpauthURL(issuer, account, secret string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + q.Encode()
}

type totpSetupResp struct {
	Secret     string `json:"secret"`
	OtpauthURL string `json:"otpauth_url"`
	Account    string `json:"account"`
	Issuer     string `json:"issuer"`
}

// adminTOTPSetup 生成待绑定的秘钥；不落库，等 enable 验证通过才写入。
func (s *Server) adminTOTPSetup(w http.ResponseWriter, r *http.Request) {
	c := adminFrom(r)
	secret, err := newTOTPSecret()
	if err != nil {
		internalErr(w, s.log, "生成秘钥失败", err)
		return
	}
	writeData(w, totpSetupResp{
		Secret:     secret,
		OtpauthURL: otpauthURL("Devour", c.Admin.Username, secret),
		Account:    c.Admin.Username,
		Issuer:     "Devour",
	})
}

type totpEnableReq struct {
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

func (s *Server) adminTOTPEnable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := adminFrom(r)
	var req totpEnableReq
	if !decode(w, r, &req) {
		return
	}
	secret := strings.ToUpper(strings.TrimSpace(req.Secret))
	key, err := auth.DecodeTOTPSecret(secret)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "秘钥格式不对")
		return
	}
	if !auth.ValidateTOTP(key, req.Code, s.now()) {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "动态验证码不正确")
		return
	}
	if err := s.st.SetAdminTOTP(ctx, c.Admin.ID, secret); err != nil {
		internalErr(w, s.log, "保存秘钥失败", err)
		return
	}
	if err := s.writeAudit(ctx, c, "admin.totp_enable",
		fmt.Sprintf("admin:%d", c.Admin.ID), "", `{"totp":"enabled"}`, clientIP(r)); err != nil {
		internalErr(w, s.log, "写审计失败", err)
		return
	}
	writeData(w, map[string]any{"totp_enabled": true})
}

type totpDisableReq struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

// adminTOTPDisable 关闭 TOTP 需要密码；已启用时还要当前动态码。
func (s *Server) adminTOTPDisable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := adminFrom(r)
	var req totpDisableReq
	if !decode(w, r, &req) {
		return
	}
	if !auth.VerifyPassword(req.Password, c.Admin.PassHash) {
		writeErr(w, http.StatusForbidden, CodeUnauthorized, "密码错误")
		return
	}
	if c.Admin.TotpSecret != "" {
		key, err := auth.DecodeTOTPSecret(c.Admin.TotpSecret)
		if err != nil || !auth.ValidateTOTP(key, req.Code, s.now()) {
			writeErr(w, http.StatusForbidden, CodeUnauthorized, "动态验证码不正确")
			return
		}
	}
	if err := s.st.SetAdminTOTP(ctx, c.Admin.ID, ""); err != nil {
		internalErr(w, s.log, "清空秘钥失败", err)
		return
	}
	if err := s.writeAudit(ctx, c, "admin.totp_disable",
		fmt.Sprintf("admin:%d", c.Admin.ID), `{"totp":"enabled"}`, `{"totp":"disabled"}`, clientIP(r)); err != nil {
		internalErr(w, s.log, "写审计失败", err)
		return
	}
	writeData(w, map[string]any{"totp_enabled": false})
}
