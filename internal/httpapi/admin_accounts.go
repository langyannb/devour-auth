package httpapi

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(out), nil
}

// newRecoveryCodes 生成 10 个一次性的「XXXX-XXXX」恢复码，返回明文与哈希。
func newRecoveryCodes() (plain []string, hashes []string, err error) {
	for i := 0; i < 10; i++ {
		raw, err := randomCode(8)
		if err != nil {
			return nil, nil, err
		}
		code := raw[:4] + "-" + raw[4:]
		plain = append(plain, code)
		hashes = append(hashes, auth.HashToken(code))
	}
	return plain, hashes, nil
}

func accountRow(a store.AdminAccount) map[string]any {
	return map[string]any{
		"id": a.ID, "username": a.Username, "role": a.Role,
		"totp_enabled": a.TotpEnabled, "ip_allow": a.IPAllow,
		"locked_until": a.LockedUntil,
		"created_at":   a.CreatedAt, "unused_recovery_codes": a.UnusedCodes,
	}
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListAdminAccounts(r.Context())
	if err != nil {
		internalErr(w, s.log, "查账号失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, a := range list {
		out = append(out, accountRow(a))
	}
	writeData(w, out)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Username == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "用户名必填")
		return
	}
	if req.Role != "admin" && req.Role != "operator" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "role 需为 admin 或 operator")
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
		writeErr(w, http.StatusBadRequest, CodeBadInput, fmt.Sprintf("密码至少 %d 位", auth.MinPasswordLen))
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}
	id, err := s.st.CreateAdmin(ctx, req.Username, hash, req.Role, "", s.now().Unix())
	if err != nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "用户名已存在")
		return
	}
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		internalErr(w, s.log, "生成恢复码失败", err)
		return
	}
	if err := s.st.AddRecoveryCodes(ctx, id, hashes); err != nil {
		internalErr(w, s.log, "保存恢复码失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "admin.create", fmt.Sprintf("admin:%d", id), "",
		fmt.Sprintf(`{"username":%q,"role":%q}`, req.Username, req.Role), clientIP(r))

	// 密码与恢复码只在这里返回一次
	writeData(w, map[string]any{
		"id": id, "username": req.Username, "role": req.Role,
		"password": password, "recovery_codes": plain,
		"hint": "密码和恢复码只显示这一次",
	})
}

func (s *Server) resetAccountPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	a, err := s.st.AdminByID(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "账号不存在")
		return
	}
	password, err := randomCode(16)
	if err != nil {
		internalErr(w, s.log, "生成密码失败", err)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}
	if err := s.st.SetAdminCredentials(ctx, id, a.Username, hash); err != nil {
		internalErr(w, s.log, "更新密码失败", err)
		return
	}
	// 重置密码同时换掉恢复码
	_ = s.st.ClearRecoveryCodes(ctx, id)
	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		internalErr(w, s.log, "生成恢复码失败", err)
		return
	}
	if err := s.st.AddRecoveryCodes(ctx, id, hashes); err != nil {
		internalErr(w, s.log, "保存恢复码失败", err)
		return
	}
	// 踢掉该账号的旧会话
	n, _ := s.st.RevokeAllAdminSessions(ctx, id, s.now().Unix())

	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "admin.reset_password", fmt.Sprintf("admin:%d", id), "", `{}`, clientIP(r))
	writeData(w, map[string]any{
		"username": a.Username, "password": password,
		"recovery_codes": plain, "revoked_sessions": n,
	})
}
