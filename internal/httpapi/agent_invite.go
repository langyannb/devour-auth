package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/auth"
)

// newAgentSlug 生成专属登录链接的路径段（16 位十六进制）。
func newAgentSlug() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// agentInviteLink 拼出下级的专属登录链接。
func (s *Server) agentInviteLink(slug string) string {
	if slug == "" {
		return ""
	}
	if s.agentHost == "" {
		return "/" + slug
	}
	return "https://" + s.agentHost + "/" + slug
}

// agentInviteInfo 专属链接打开时读下级信息（免登录，凭 slug）。
func (s *Server) agentInviteInfo(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("code")
	if len(slug) < 8 {
		writeErr(w, http.StatusNotFound, CodeBadInput, "链接无效")
		return
	}
	ip := clientIP(r)
	if !s.lim.Allow("agent:invite:ip:"+ip, 20, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁")
		return
	}
	a, err := s.st.AgentBySlug(r.Context(), slug)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "链接无效")
		return
	}
	writeData(w, map[string]any{
		"name":      a.Name,
		"level":     a.Level,
		"activated": a.PasswordSet == 1,
		"status":    a.Status,
	})
}

// agentInviteActivate 下级通过链接首次设置密码，成功后直接登录。
func (s *Server) agentInviteActivate(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("code")
	var req struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.Password) < auth.MinPasswordLen {
		writeErr(w, http.StatusBadRequest, CodeBadInput,
			fmt.Sprintf("密码至少 %d 位", auth.MinPasswordLen))
		return
	}
	ip := clientIP(r)
	if !s.lim.Allow("agent:invite:ip:"+ip, 10, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁")
		return
	}
	a, err := s.st.AgentBySlug(r.Context(), slug)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "链接无效")
		return
	}
	if a.Status != "active" {
		writeErr(w, http.StatusForbidden, CodeUnauthorized, "账号已被冻结")
		return
	}
	if a.PasswordSet == 1 {
		writeErr(w, http.StatusConflict, CodeBadInput, "这个链接已经设置过密码，请直接在登录页登录")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}
	if err := s.st.ActivateAgentBySlug(r.Context(), slug, hash); err != nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "这个链接已经设置过密码，请直接在登录页登录")
		return
	}
	res, err := s.agents.Login(r.Context(), a.Name, req.Password, ip, truncate(r.UserAgent(), 200))
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	s.setAgentCookie(w, res.Token, res.ExpiresAt)
	writeData(w, map[string]any{
		"csrf_token":  res.CSRF,
		"expires_at":  res.ExpiresAt,
		"id":          res.Agent.ID,
		"name":        res.Agent.Name,
		"level":       res.Agent.Level,
		"discount_bp": res.Agent.DiscountBP,
		"rebate_bp":   res.Agent.RebateBP,
		"server_time": s.now().Unix(),
	})
}

// ensureAgentLink 给还没有 slug 的代理补一个专属链接（老数据或管理员手动补）。
func (s *Server) ensureAgentLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	ctx := r.Context()
	a, err := s.st.AgentByID(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "代理不存在")
		return
	}
	slug := a.LoginSlug
	if slug == "" {
		slug, err = newAgentSlug()
		if err != nil {
			internalErr(w, s.log, "生成链接失败", err)
			return
		}
		if err := s.st.SetAgentLoginSlug(ctx, id, slug); err != nil {
			internalErr(w, s.log, "保存链接失败", err)
			return
		}
	}
	writeData(w, map[string]any{"id": id, "login_link": s.agentInviteLink(slug)})
}
