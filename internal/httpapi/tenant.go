package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

// newAPIKey 生成并保存一个开放接口密钥。明文只在这一个响应里出现。
func (s *Server) newAPIKey(ctx context.Context, appID int64, name, scopes string) (keyID, secret string, err error) {
	keyRaw := make([]byte, 12)
	secretRaw := make([]byte, 32)
	if _, err = rand.Read(keyRaw); err != nil {
		return "", "", err
	}
	if _, err = rand.Read(secretRaw); err != nil {
		return "", "", err
	}
	keyID = "ak_" + hex.EncodeToString(keyRaw)
	secret = base64.RawURLEncoding.EncodeToString(secretRaw)
	if _, err = s.st.CreateAPIKey(ctx, store.APIKey{
		AppID: appID, Name: name, Scopes: scopes, KeyID: keyID, Secret: secret,
		Status: "active", CreatedAt: s.now().Unix(),
	}); err != nil {
		return "", "", err
	}
	return keyID, secret, nil
}

// createTenant 一键开通一个开发者：应用 + 绑定该应用的管理员 + 开放接口密钥。
// 只给全局管理员用；密码、恢复码只在这里返回一次，API 密钥之后可在对接信息里再查。
func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		Scopes   string `json:"scopes"`
	}
	if !decode(w, r, &req) {
		return
	}
	code := strings.ToLower(strings.TrimSpace(req.Code))
	name := strings.TrimSpace(req.Name)
	username := strings.TrimSpace(req.Username)
	switch {
	case !appCodeRe.MatchString(code):
		writeErr(w, http.StatusBadRequest, CodeBadInput, "code 需为 2-32 位小写字母/数字/下划线/短横线")
		return
	case name == "":
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用名称必填")
		return
	case username == "":
		writeErr(w, http.StatusBadRequest, CodeBadInput, "管理员用户名必填")
		return
	}

	// 先把冲突查出来，避免建了应用却卡在账号上
	if _, err := s.st.AppByCode(ctx, code); err == nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "应用 code 已存在")
		return
	}
	if _, err := s.st.AdminByUsername(ctx, username); err == nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "用户名已存在")
		return
	}

	password := req.Password
	if password == "" {
		p, err := randomCode(16)
		if err != nil {
			internalErr(w, s.log, "生成密码失败", err)
			return
		}
		password = p
	}
	if len(password) < auth.MinPasswordLen {
		writeErr(w, http.StatusBadRequest, CodeBadInput,
			fmt.Sprintf("密码至少 %d 位，或留空自动生成", auth.MinPasswordLen))
		return
	}
	scopes := strings.TrimSpace(req.Scopes)
	if scopes == "" {
		scopes = "cards:read,cards:write,stats:read"
	}
	for _, sc := range strings.Split(scopes, ",") {
		switch strings.TrimSpace(sc) {
		case "cards:write", "cards:read", "stats:read":
		default:
			writeErr(w, http.StatusBadRequest, CodeBadInput, "未知权限 "+sc)
			return
		}
	}

	now := s.now().Unix()
	appID, err := s.st.CreateApp(ctx, code, name, now)
	if err != nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "应用 code 已存在")
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}
	adminID, err := s.st.CreateAdmin(ctx, username, hash, "admin", "", appID, now)
	if err != nil {
		internalErr(w, s.log, "建管理员失败（应用已建好）", err)
		return
	}
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		internalErr(w, s.log, "生成恢复码失败", err)
		return
	}
	if err := s.st.AddRecoveryCodes(ctx, adminID, hashes); err != nil {
		internalErr(w, s.log, "保存恢复码失败", err)
		return
	}
	keyID, secret, err := s.newAPIKey(ctx, appID, name+" 发货密钥", scopes)
	if err != nil {
		internalErr(w, s.log, "生成开放接口密钥失败", err)
		return
	}

	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "tenant.create", fmt.Sprintf("app:%d", appID), "", fmt.Sprintf(
		`{"code":%q,"username":%q,"api_key":%q}`, code, username, keyID), clientIP(r))

	writeData(w, map[string]any{
		"app":     map[string]any{"id": appID, "code": code, "name": name},
		"admin":   map[string]any{"username": username, "password": password, "recovery_codes": plain},
		"api_key": map[string]any{"key_id": keyID, "secret": secret, "scopes": scopes},
		"login":   "/login",
		"hint":    "密码和恢复码只显示这一次；API 密钥「对接信息 → 开放接口」里随时能再看",
	})
}
