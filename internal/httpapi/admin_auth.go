package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

const (
	adminIdleTimeout = 30 * time.Minute
	adminAbsTimeout  = 12 * time.Hour
	reauthWindow     = 5 * time.Minute

	loginPerIP       = 5
	loginPerAccount  = 5
	loginAccountSpan = 15 * time.Minute
)

// dummyHash 用于账号不存在时消耗同样的 argon2 时间，避免用户名枚举。
var dummyHash = func() string {
	h, err := auth.HashPassword("devour-timing-equalizer")
	if err != nil {
		panic(err)
	}
	return h
}()

type adminCtx struct {
	Admin   store.Admin
	Session store.AdminSession
}

type adminCtxKey struct{}

func adminFrom(r *http.Request) adminCtx {
	return r.Context().Value(adminCtxKey{}).(adminCtx)
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTP     string `json:"totp"`
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ip := clientIP(r)

	if !s.lim.Allow("admin:login:ip:"+ip, loginPerIP, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁，稍后再试")
		return
	}

	var req loginReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "请求格式错误")
		return
	}
	if !s.lim.Allow("admin:login:acct:"+req.Username, loginPerAccount, loginAccountSpan) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁，稍后再试")
		return
	}

	now := s.now()
	a, err := s.st.AdminByUsername(ctx, req.Username)
	passOK := false
	if errors.Is(err, store.ErrNotFound) {
		auth.VerifyPassword(req.Password, dummyHash)
	} else if err != nil {
		s.log.Error("登录查询失败", "err", err)
		writeErr(w, http.StatusInternalServerError, CodeInternal, "服务内部错误")
		return
	} else {
		passOK = auth.VerifyPassword(req.Password, a.PassHash)
	}
	if !passOK || a.ID == 0 || a.LockedUntil > now.Unix() {
		writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "账号或密码错误")
		return
	}

	if a.TotpSecret != "" {
		secret, err := auth.DecodeTOTPSecret(a.TotpSecret)
		if err != nil || !auth.ValidateTOTP(secret, req.TOTP, now) {
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "动态验证码错误")
			return
		}
	}

	token, err := auth.NewToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, CodeInternal, "服务内部错误")
		return
	}
	csrf, err := auth.NewCSRF()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, CodeInternal, "服务内部错误")
		return
	}
	expires := now.Add(adminAbsTimeout).Unix()
	sessID, err := s.st.CreateAdminSession(ctx, a.ID, auth.HashToken(token), csrf,
		ip, truncate(r.UserAgent(), 200), now.Unix(), expires)
	if err != nil {
		s.log.Error("创建会话失败", "err", err)
		writeErr(w, http.StatusInternalServerError, CodeInternal, "服务内部错误")
		return
	}
	// 新登录踢旧会话
	if err := s.st.RevokeOtherAdminSessions(ctx, a.ID, sessID, now.Unix()); err != nil {
		s.log.Error("踢旧会话失败", "err", err)
	}

	s.lim.Reset("admin:login:acct:" + req.Username)
	s.setAdminCookie(w, token, expires)
	writeData(w, map[string]any{
		"csrf_token":  csrf,
		"expires_at":  expires,
		"admin":       map[string]any{"id": a.ID, "username": a.Username, "role": a.Role},
		"server_time": now.Unix(),
	})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	c := adminFrom(r)
	if err := s.st.RevokeAdminSession(r.Context(), c.Session.ID, s.now().Unix()); err != nil {
		s.log.Error("吊销会话失败", "err", err)
	}
	s.clearAdminCookie(w)
	writeData(w, map[string]any{"logged_out": true})
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	c := adminFrom(r)
	writeData(w, map[string]any{
		"id":           c.Admin.ID,
		"username":     c.Admin.Username,
		"role":         c.Admin.Role,
		"reauth_until": c.Session.ReauthUntil,
		"server_time":  s.now().Unix(),
	})
}

type reauthReq struct {
	Password string `json:"password"`
	TOTP     string `json:"totp"`
}

func (s *Server) adminReauth(w http.ResponseWriter, r *http.Request) {
	c := adminFrom(r)
	var req reauthReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "请求格式错误")
		return
	}
	now := s.now()
	if !auth.VerifyPassword(req.Password, c.Admin.PassHash) {
		writeErr(w, http.StatusForbidden, CodeUnauthorized, "密码错误")
		return
	}
	if c.Admin.TotpSecret != "" {
		secret, err := auth.DecodeTOTPSecret(c.Admin.TotpSecret)
		if err != nil || !auth.ValidateTOTP(secret, req.TOTP, now) {
			writeErr(w, http.StatusForbidden, CodeUnauthorized, "动态验证码错误")
			return
		}
	}
	until := now.Add(reauthWindow).Unix()
	if err := s.st.SetAdminReauth(r.Context(), c.Session.ID, until); err != nil {
		writeErr(w, http.StatusInternalServerError, CodeInternal, "服务内部错误")
		return
	}
	writeData(w, map[string]any{"reauth_until": until})
}

// requireAdmin 校验管理端会话：空闲 30 分钟 / 绝对 12 小时，写请求校验 CSRF。
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		cookie, err := r.Cookie(s.adminCookieName())
		if err != nil {
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "未登录")
			return
		}
		sess, err := s.st.AdminSessionByTokenHash(ctx, auth.HashToken(cookie.Value))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "会话已失效")
			return
		}
		now := s.now().Unix()
		if sess.ExpiresAt <= now || sess.LastSeen+int64(adminIdleTimeout/time.Second) <= now {
			_ = s.st.RevokeAdminSession(ctx, sess.ID, now)
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "会话已失效")
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(sess.CSRF)) != 1 {
				writeErr(w, http.StatusForbidden, CodeUnauthorized, "CSRF 校验失败")
				return
			}
		}
		// 管理端会话绑定 IP：cookie 被搬到别的网络立即失效（验收 14）。
		if sess.IP != "" && sess.IP != clientIP(r) {
			_ = s.st.RevokeAdminSession(ctx, sess.ID, now)
			_, _ = s.st.DB().ExecContext(ctx,
				`INSERT INTO events(ts, kind, ip, ua, code) VALUES(?,?,?,?,?)`,
				now, "admin_session_ip_change", clientIP(r), truncate(r.UserAgent(), 200), CodeUnauthorized)
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "登录 IP 已变更，请重新登录")
			return
		}
		admin, err := s.st.AdminByID(ctx, sess.AdminID)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "会话已失效")
			return
		}
		_ = s.st.TouchAdminSession(ctx, sess.ID, now)
		next(w, r.WithContext(context.WithValue(r.Context(), adminCtxKey{}, adminCtx{Admin: admin, Session: sess})))
	}
}

// requireRole 在 requireAdmin 之后使用：operator 不能改价、改策略、发卡、碰账号。
func (s *Server) requireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if adminFrom(r).Admin.Role != role {
			writeErr(w, http.StatusForbidden, CodeUnauthorized, "权限不足")
			return
		}
		next(w, r)
	}
}

// requireReauth 敏感操作：5 分钟内做过一次密码 + TOTP 校验。
func (s *Server) requireReauth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if adminFrom(r).Session.ReauthUntil <= s.now().Unix() {
			writeErr(w, http.StatusForbidden, CodeUnauthorized, "需要重新验证身份")
			return
		}
		next(w, r)
	}
}

func (s *Server) adminCookieName() string {
	if s.env == "prod" {
		return "__Host-devour_admin"
	}
	return "devour_admin"
}

func (s *Server) setAdminCookie(w http.ResponseWriter, token string, expires int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.adminCookieName(),
		Value:    token,
		Path:     "/",
		Expires:  time.Unix(expires, 0),
		MaxAge:   int(adminAbsTimeout / time.Second),
		HttpOnly: true,
		Secure:   s.env == "prod",
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearAdminCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.adminCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.env == "prod",
		SameSite: http.SameSiteStrictMode,
	})
}

// clientIP 只在本机反代（loopback）时信任 X-Forwarded-For / X-Real-IP。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			first, _, _ := strings.Cut(v, ",")
			if first = strings.TrimSpace(first); first != "" {
				return first
			}
		}
		if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
			return v
		}
	}
	return host
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
