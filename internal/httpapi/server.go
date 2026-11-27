package httpapi

import (
	"crypto/ed25519"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/agent"
	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/clientauth"
	"github.com/langyannb/devour-auth/internal/store"
)

// Options 服务端启动参数，集中传避免构造函数参数爆炸。
type Options struct {
	Env       string
	Static    fs.FS
	TicketKey ed25519.PrivateKey
	AdminHost string
	AgentHost string
	NetHost   string
}

type Server struct {
	st         *store.Store
	cards      *card.Service
	clients    *clientauth.Service
	agents     *agent.Service
	static     fs.FS
	ticketKey  ed25519.PrivateKey
	log        *slog.Logger
	env        string
	adminHost  string
	agentHost  string
	netHost    string
	lim        *auth.Limiter
	openNonces *auth.Limiter
	now        func() time.Time
}

func New(st *store.Store, log *slog.Logger, opt Options) *Server {
	return &Server{
		st:         st,
		cards:      card.New(st),
		clients:    clientauth.New(st),
		agents:     agent.New(st),
		static:     opt.Static,
		ticketKey:  opt.TicketKey,
		log:        log,
		env:        opt.Env,
		adminHost:  opt.AdminHost,
		agentHost:  opt.AgentHost,
		netHost:    opt.NetHost,
		lim:        auth.NewLimiter(),
		openNonces: auth.NewLimiter(),
		now:        time.Now,
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
	mux.HandleFunc("GET /admin/api/audit/verify", s.requireAdmin(s.verifyAuditChain))
	mux.HandleFunc("GET /admin/api/stats", s.requireAdmin(s.adminStats))

	// 代理管理（管理员）
	mux.HandleFunc("GET /admin/api/agents", s.requireAdmin(s.listAgents))
	mux.HandleFunc("POST /admin/api/agents", s.requireAdmin(s.requireReauth(s.createAgent)))
	mux.HandleFunc("PATCH /admin/api/agents/{id}", s.requireAdmin(s.requireReauth(s.updateAgent)))
	mux.HandleFunc("POST /admin/api/agents/{id}/freeze", s.requireAdmin(s.setAgentFrozen(true)))
	mux.HandleFunc("POST /admin/api/agents/{id}/unfreeze", s.requireAdmin(s.setAgentFrozen(false)))
	mux.HandleFunc("GET /admin/api/agents/{id}/bills", s.requireAdmin(s.agentBillsAdmin))
	mux.HandleFunc("POST /admin/api/bills/{billId}/settle", s.requireAdmin(s.settleBill))

	// 代理端（独立会话域）
	mux.HandleFunc("POST /agent/api/login", s.agentLogin)
	mux.HandleFunc("POST /agent/api/logout", s.requireAgent(s.agentLogout))
	mux.HandleFunc("GET /agent/api/me", s.requireAgent(s.agentMe))
	mux.HandleFunc("GET /agent/api/policies", s.requireAgent(s.agentPolicies))
	mux.HandleFunc("POST /agent/api/issue", s.requireAgent(s.agentIssue))
	mux.HandleFunc("GET /agent/api/cards", s.requireAgent(s.agentCards))
	mux.HandleFunc("GET /agent/api/ledger", s.requireAgent(s.agentLedger))
	mux.HandleFunc("GET /agent/api/bills", s.requireAgent(s.agentBills))

	// 运营内容
	mux.HandleFunc("GET /admin/api/releases", s.requireAdmin(s.listReleases))
	mux.HandleFunc("POST /admin/api/releases", s.requireAdmin(s.createRelease))
	mux.HandleFunc("POST /admin/api/releases/{id}/toggle", s.requireAdmin(s.toggleRelease))
	mux.HandleFunc("GET /admin/api/notices", s.requireAdmin(s.listNoticesAdmin))
	mux.HandleFunc("POST /admin/api/notices", s.requireAdmin(s.createNotice))
	mux.HandleFunc("POST /admin/api/notices/{id}/toggle", s.requireAdmin(s.toggleNotice))
	mux.HandleFunc("GET /admin/api/flags", s.requireAdmin(s.listFlagsAdmin))
	mux.HandleFunc("PUT /admin/api/flags", s.requireAdmin(s.setFlag))

	// 客户端元信息与离线票据
	mux.HandleFunc("GET /v1/meta", s.meta)
	mux.HandleFunc("POST /v1/offline-ticket", s.offlineTicket)

	// 开放 API（HMAC 签名）
	mux.HandleFunc("POST /open/v1/cards/generate", s.requireAPIKey("cards:write", s.openGenerate))
	mux.HandleFunc("POST /open/v1/cards/query", s.requireAPIKey("cards:read", s.openQuery))
	mux.HandleFunc("POST /open/v1/cards/extend", s.requireAPIKey("cards:write", s.openExtend))
	mux.HandleFunc("POST /open/v1/cards/unbind", s.requireAPIKey("cards:write", s.openUnbind))
	mux.HandleFunc("POST /open/v1/cards/entitlements/grant", s.requireAPIKey("cards:write", s.openGrant))
	mux.HandleFunc("GET /open/v1/stats/online", s.requireAPIKey("stats:read", s.openOnline))

	// 开放 API 密钥管理
	mux.HandleFunc("GET /admin/api/open/keys", s.requireAdmin(s.listAPIKeys))
	mux.HandleFunc("POST /admin/api/open/keys", s.requireAdmin(s.createAPIKey))
	mux.HandleFunc("POST /admin/api/open/keys/{id}/disable", s.requireAdmin(s.setAPIKeyStatus(false)))
	mux.HandleFunc("POST /admin/api/open/keys/{id}/enable", s.requireAdmin(s.setAPIKeyStatus(true)))

	// 客户端协议（design.md 第 12 节）
	mux.HandleFunc("POST /v1/auth/login", s.clientLogin)
	mux.HandleFunc("POST /v1/auth/refresh", s.clientRefresh)
	mux.HandleFunc("POST /v1/auth/logout", s.clientLogout)
	mux.HandleFunc("GET /v1/session", s.clientSession)
	mux.HandleFunc("POST /v1/consume", s.clientConsume)
	mux.HandleFunc("POST /v1/pause", s.clientPause)

	mux.HandleFunc("POST /v1/query", s.clientQuery)

	// 未匹配路径：API 前缀返回 JSON 错误，其余交给前端路由。
	mux.HandleFunc("GET /", s.spa)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "接口不存在")
	})
	return s.withLogging(s.hostGuard(mux))
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
