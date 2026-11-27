package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/clientauth"
	"github.com/langyannb/devour-auth/internal/store"
)

type Server struct {
	st      *store.Store
	cards   *card.Service
	clients *clientauth.Service
	log     *slog.Logger
	env     string
	lim     *auth.Limiter
	now     func() time.Time
}

func New(st *store.Store, log *slog.Logger, env string) *Server {
	return &Server{
		st:      st,
		cards:   card.New(st),
		clients: clientauth.New(st),
		log:     log,
		env:     env,
		lim:     auth.NewLimiter(),
		now:     time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)

	// 管理端会话
	mux.HandleFunc("POST /admin/api/login", s.adminLogin)
	mux.HandleFunc("POST /admin/api/logout", s.requireAdmin(s.adminLogout))
	mux.HandleFunc("GET /admin/api/me", s.requireAdmin(s.adminMe))
	mux.HandleFunc("POST /admin/api/reauth", s.requireAdmin(s.adminReauth))
	mux.HandleFunc("POST /admin/api/me/totp/setup", s.requireAdmin(s.adminTOTPSetup))
	mux.HandleFunc("POST /admin/api/me/totp/enable", s.requireAdmin(s.adminTOTPEnable))
	mux.HandleFunc("POST /admin/api/me/totp/disable", s.requireAdmin(s.adminTOTPDisable))

	// 应用、卡类型、权限（改价/改策略需 reauth）
	mux.HandleFunc("GET /admin/api/apps", s.requireAdmin(s.listApps))
	mux.HandleFunc("GET /admin/api/policies", s.requireAdmin(s.listPolicies))
	mux.HandleFunc("POST /admin/api/policies", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.createPolicy))))
	mux.HandleFunc("PATCH /admin/api/policies/{id}", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.updatePolicy))))
	mux.HandleFunc("GET /admin/api/entitlements", s.requireAdmin(s.listEntitlements))
	mux.HandleFunc("POST /admin/api/entitlements", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.createEntitlement))))

	// 卡密
	mux.HandleFunc("POST /admin/api/cards/generate", s.requireAdmin(s.requireRole("admin", s.generateCards)))
	mux.HandleFunc("GET /admin/api/cards", s.requireAdmin(s.listCards))
	mux.HandleFunc("GET /admin/api/cards/{id}", s.requireAdmin(s.getCard))
	mux.HandleFunc("POST /admin/api/cards/{id}/ban", s.requireAdmin(s.cardStatusAction("ban", s.cards.Ban)))
	mux.HandleFunc("POST /admin/api/cards/{id}/unban", s.requireAdmin(s.cardStatusAction("unban", s.cards.Unban)))
	mux.HandleFunc("POST /admin/api/cards/{id}/recycle", s.requireAdmin(s.cardStatusAction("recycle", s.cards.Recycle)))
	mux.HandleFunc("POST /admin/api/cards/{id}/unbind", s.requireAdmin(s.cardStatusAction("unbind", s.cards.Unbind)))
	mux.HandleFunc("POST /admin/api/cards/{id}/reset-rebind", s.requireAdmin(s.cardStatusAction("reset_rebind", s.cards.ResetRebind)))
	mux.HandleFunc("POST /admin/api/cards/{id}/extend", s.requireAdmin(s.extendCard))
	mux.HandleFunc("POST /admin/api/cards/{id}/entitlements/grant", s.requireAdmin(s.grantCardEntitlement))
	mux.HandleFunc("POST /admin/api/cards/{id}/entitlements/revoke", s.requireAdmin(s.revokeCardEntitlement))

	// 审计
	mux.HandleFunc("GET /admin/api/audit", s.requireAdmin(s.listAudit))

	// 客户端协议（design.md 第 12 节）
	mux.HandleFunc("POST /v1/auth/login", s.clientLogin)
	mux.HandleFunc("POST /v1/auth/refresh", s.clientRefresh)
	mux.HandleFunc("POST /v1/auth/logout", s.clientLogout)
	mux.HandleFunc("GET /v1/session", s.clientSession)
	mux.HandleFunc("POST /v1/consume", s.clientConsume)
	mux.HandleFunc("POST /v1/pause", s.clientPause)

	// 未匹配路径也走统一错误体（JSON 端点，不返回 HTML）。
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "接口不存在")
	})
	return s.withLogging(mux)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DB().PingContext(r.Context()); err != nil {
		s.log.Error("healthz: 数据库不可用", "err", err)
		writeErr(w, http.StatusServiceUnavailable, CodeInternal, "数据库不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"server_time": s.now().Unix(),
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path,
			"dur_ms", time.Since(start).Milliseconds())
	})
}
