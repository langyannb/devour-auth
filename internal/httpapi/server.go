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
	Env        string
	Static     fs.FS
	TicketKey  ed25519.PrivateKey
	AdminHost  string
	AgentHost  string
	NetHost    string
	UnbindHost string
	BackupDir  string
	DBPath     string
	ListenAddr string
	Notifier   card.Notifier
	// CaptchaNew / CaptchaVerify 可选钩子：测试注入用，nil 时走内置图形验证码。
	CaptchaNew    func() (id, img string)
	CaptchaVerify func(id, code string) bool
}

type Server struct {
	st            *store.Store
	cards         *card.Service
	clients       *clientauth.Service
	agents        *agent.Service
	static        fs.FS
	ticketKey     ed25519.PrivateKey
	log           *slog.Logger
	env           string
	adminHost     string
	agentHost     string
	netHost       string
	unbindHost    string
	backupDir     string
	dbPath        string
	listenAddr    string
	lim           *auth.Limiter
	openNonces    *auth.Limiter
	captchaNew    func() (id, img string)
	captchaVerify func(id, code string) bool
	now           func() time.Time
}

func New(st *store.Store, log *slog.Logger, opt Options) *Server {
	cardSvc := card.New(st)
	if opt.Notifier != nil {
		cardSvc.SetNotifier(opt.Notifier)
	}
	captchaNew, captchaVerify := opt.CaptchaNew, opt.CaptchaVerify
	if captchaNew == nil || captchaVerify == nil {
		newFn, verifyFn := defaultQueryCaptcha()
		if captchaNew == nil {
			captchaNew = newFn
		}
		if captchaVerify == nil {
			captchaVerify = verifyFn
		}
	}
	return &Server{
		st:            st,
		cards:         cardSvc,
		clients:       clientauth.New(st),
		agents:        agent.New(st),
		static:        opt.Static,
		ticketKey:     opt.TicketKey,
		log:           log,
		env:           opt.Env,
		adminHost:     opt.AdminHost,
		agentHost:     opt.AgentHost,
		netHost:       opt.NetHost,
		unbindHost:    opt.UnbindHost,
		backupDir:     opt.BackupDir,
		dbPath:        opt.DBPath,
		listenAddr:    opt.ListenAddr,
		lim:           auth.NewLimiter(),
		openNonces:    auth.NewLimiter(),
		captchaNew:    captchaNew,
		captchaVerify: captchaVerify,
		now:           time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)

	// 管理端会话
	mux.HandleFunc("POST /admin/api/login", s.adminLogin)
	mux.HandleFunc("POST /admin/api/logout", s.requireAdmin(s.adminLogout))
	mux.HandleFunc("GET /admin/api/me", s.requireAdmin(s.adminMe))
	mux.HandleFunc("GET /admin/api/accounts", s.requireAdmin(s.requireRole("admin", s.listAccounts)))
	mux.HandleFunc("POST /admin/api/accounts", s.requireAdmin(s.requireRole("admin", s.createAccount)))
	mux.HandleFunc("POST /admin/api/accounts/{id}/reset-password", s.requireAdmin(s.requireRole("admin", s.resetAccountPassword)))
	mux.HandleFunc("PATCH /admin/api/accounts/{id}/ip", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.setAccountIPAllow))))
	mux.HandleFunc("POST /admin/api/reauth", s.requireAdmin(s.adminReauth))
	mux.HandleFunc("POST /admin/api/me/totp/setup", s.requireAdmin(s.adminTOTPSetup))
	mux.HandleFunc("POST /admin/api/me/totp/enable", s.requireAdmin(s.adminTOTPEnable))
	mux.HandleFunc("POST /admin/api/me/totp/disable", s.requireAdmin(s.adminTOTPDisable))

	// 应用、卡类型、权限（改价/改策略需 reauth）
	mux.HandleFunc("GET /admin/api/apps", s.requireAdmin(s.listApps))
	mux.HandleFunc("POST /admin/api/apps", s.requireAdmin(s.requireRole("admin", s.createApp)))
	mux.HandleFunc("PATCH /admin/api/apps/{id}", s.requireAdmin(s.requireRole("admin", s.updateApp)))
	mux.HandleFunc("GET /admin/api/policies", s.requireAdmin(s.listPolicies))
	mux.HandleFunc("POST /admin/api/policies", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.createPolicy))))
	mux.HandleFunc("PATCH /admin/api/policies/{id}", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.updatePolicy))))
	mux.HandleFunc("GET /admin/api/entitlements", s.requireAdmin(s.listEntitlements))
	mux.HandleFunc("POST /admin/api/entitlements", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.createEntitlement))))

	// 卡密
	mux.HandleFunc("POST /admin/api/cards/generate", s.requireAdmin(s.requireRole("admin", s.generateCards)))
	mux.HandleFunc("POST /admin/api/cards/quick", s.requireAdmin(s.requireRole("admin", s.quickGenerate)))
	mux.HandleFunc("GET /admin/api/cards", s.requireAdmin(s.listCards))
	mux.HandleFunc("GET /admin/api/cards/export", s.requireAdmin(s.exportCards))
	mux.HandleFunc("POST /admin/api/cards/batch", s.requireAdmin(s.batchCards))
	mux.HandleFunc("GET /admin/api/cards/bulk-preview", s.requireAdmin(s.bulkPreview))
	mux.HandleFunc("POST /admin/api/cards/bulk", s.requireAdmin(s.requireRole("admin", s.requireReauth(s.bulkCards))))
	mux.HandleFunc("PATCH /admin/api/cards/{id}", s.requireAdmin(s.patchCard))
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
	mux.HandleFunc("GET /admin/api/stats/trend", s.requireAdmin(s.statsTrend))
	mux.HandleFunc("GET /admin/api/analytics", s.requireAdmin(s.adminAnalytics))
	mux.HandleFunc("GET /admin/api/health-check", s.requireAdmin(s.healthCheck))

	// 黑名单 / 事件 / 财务 / 批次 / 系统状态
	mux.HandleFunc("GET /admin/api/blacklist", s.requireAdmin(s.listBlacklist))
	mux.HandleFunc("POST /admin/api/blacklist", s.requireAdmin(s.addBlacklist))
	mux.HandleFunc("DELETE /admin/api/blacklist/{id}", s.requireAdmin(s.removeBlacklist))
	mux.HandleFunc("GET /admin/api/events", s.requireAdmin(s.listEvents))
	mux.HandleFunc("GET /admin/api/finance/ledger", s.requireAdmin(s.listFinanceLedger))
	mux.HandleFunc("GET /admin/api/finance/bills", s.requireAdmin(s.listFinanceBills))
	mux.HandleFunc("GET /admin/api/batches", s.requireAdmin(s.listBatches))
	mux.HandleFunc("GET /admin/api/system/status", s.requireAdmin(s.systemStatus))

	// 设备与在线会话
	mux.HandleFunc("GET /admin/api/devices", s.requireAdmin(s.listDevices))
	mux.HandleFunc("POST /admin/api/devices/{id}/unbind", s.requireAdmin(s.deviceAction("unbind")))
	mux.HandleFunc("POST /admin/api/devices/{id}/ban", s.requireAdmin(s.deviceAction("ban")))
	mux.HandleFunc("POST /admin/api/devices/{id}/unban", s.requireAdmin(s.deviceAction("unban")))
	mux.HandleFunc("GET /admin/api/sessions", s.requireAdmin(s.listSessions))
	mux.HandleFunc("POST /admin/api/sessions/{id}/revoke", s.requireAdmin(s.revokeSession))
	mux.HandleFunc("GET /admin/api/cards/{id}/devices", s.requireAdmin(s.listCardDevices))
	mux.HandleFunc("GET /admin/api/cards/{id}/sessions", s.requireAdmin(s.listCardSessions))

	// 代理管理（管理员）
	mux.HandleFunc("GET /admin/api/agents", s.requireAdmin(s.listAgents))
	mux.HandleFunc("POST /admin/api/agents", s.requireAdmin(s.requireReauth(s.createAgent)))
	mux.HandleFunc("PATCH /admin/api/agents/{id}", s.requireAdmin(s.requireReauth(s.updateAgent)))
	mux.HandleFunc("POST /admin/api/agents/{id}/freeze", s.requireAdmin(s.setAgentFrozen(true)))
	mux.HandleFunc("GET /admin/api/plans", s.requireAdmin(s.listPlans))
	mux.HandleFunc("POST /admin/api/plans", s.requireAdmin(s.requireRole("admin", s.savePlan)))
	mux.HandleFunc("GET /admin/api/agents/{id}/plan", s.requireAdmin(s.getAgentPlan))
	mux.HandleFunc("PUT /admin/api/agents/{id}/plan", s.requireAdmin(s.requireRole("admin", s.setAgentPlan)))
	mux.HandleFunc("GET /admin/api/agents/{id}/prices", s.requireAdmin(s.listAgentPrices))
	mux.HandleFunc("PUT /admin/api/agents/{id}/prices", s.requireAdmin(s.requireRole("admin", s.setAgentPrices)))
	mux.HandleFunc("POST /admin/api/agents/{id}/unfreeze", s.requireAdmin(s.setAgentFrozen(false)))
	mux.HandleFunc("GET /admin/api/agents/{id}/bills", s.requireAdmin(s.agentBillsAdmin))
	mux.HandleFunc("POST /admin/api/agents/{id}/quota", s.requireAdmin(s.agentQuotaTopup))
	mux.HandleFunc("POST /admin/api/agents/{id}/reset-password", s.requireAdmin(s.requireReauth(s.resetAgentPassword)))
	mux.HandleFunc("POST /admin/api/agents/{id}/link", s.requireAdmin(s.ensureAgentLink))
	mux.HandleFunc("POST /admin/api/bills/{billId}/settle", s.requireAdmin(s.settleBill))

	// 代理端（独立会话域）
	mux.HandleFunc("POST /agent/api/login", s.agentLogin)
	mux.HandleFunc("GET /agent/api/invite/{code}", s.agentInviteInfo)
	mux.HandleFunc("POST /agent/api/invite/{code}", s.agentInviteActivate)
	mux.HandleFunc("POST /agent/api/logout", s.requireAgent(s.agentLogout))
	mux.HandleFunc("GET /agent/api/me", s.requireAgent(s.agentMe))
	mux.HandleFunc("GET /agent/api/policies", s.requireAgent(s.agentPolicies))
	mux.HandleFunc("GET /agent/api/plan", s.requireAgent(s.agentPlan))
	mux.HandleFunc("POST /agent/api/issue", s.requireAgent(s.agentIssue))
	mux.HandleFunc("GET /agent/api/cards", s.requireAgent(s.agentCards))
	mux.HandleFunc("GET /agent/api/ledger", s.requireAgent(s.agentLedger))
	mux.HandleFunc("GET /agent/api/bills", s.requireAgent(s.agentBills))
	mux.HandleFunc("GET /agent/api/subagents", s.requireAgent(s.agentSubagents))
	mux.HandleFunc("GET /agent/api/summary", s.requireAgent(s.agentSummary))
	mux.HandleFunc("GET /agent/api/analytics", s.requireAgent(s.agentAnalytics))
	mux.HandleFunc("GET /agent/api/notices", s.requireAgent(s.agentNotices))
	mux.HandleFunc("POST /agent/api/cards/{id}/ban", s.requireAgent(s.agentCardAction("ban", s.cards.Ban)))
	mux.HandleFunc("POST /agent/api/cards/{id}/unban", s.requireAgent(s.agentCardAction("unban", s.cards.Unban)))
	mux.HandleFunc("POST /agent/api/cards/{id}/unbind", s.requireAgent(s.agentCardAction("unbind", s.cards.Unbind)))
	mux.HandleFunc("POST /agent/api/subagents", s.requireAgent(s.agentCreateSub))
	mux.HandleFunc("GET /agent/api/plans", s.requireAgent(s.agentPlans))
	mux.HandleFunc("PUT /agent/api/subagents/{id}/plan", s.requireAgent(s.agentSubPlan))
	mux.HandleFunc("POST /agent/api/subagents/{id}/quota", s.requireAgent(s.agentSubQuota))
	mux.HandleFunc("POST /agent/api/subagents/{id}/freeze", s.requireAgent(s.agentSubFrozen(true)))
	mux.HandleFunc("POST /agent/api/subagents/{id}/unfreeze", s.requireAgent(s.agentSubFrozen(false)))
	mux.HandleFunc("PUT /agent/api/subagents/{id}/perms", s.requireAgent(s.agentSubPerms))
	mux.HandleFunc("POST /agent/api/subagents/{id}/link", s.requireAgent(s.agentSubLink))
	mux.HandleFunc("POST /agent/api/password", s.requireAgent(s.agentChangePassword))
	mux.HandleFunc("GET /agent/api/ledger/export", s.requireAgent(s.agentBillsExport))

	// 运营内容
	mux.HandleFunc("GET /admin/api/releases", s.requireAdmin(s.listReleases))
	mux.HandleFunc("POST /admin/api/releases", s.requireAdmin(s.createRelease))
	mux.HandleFunc("PUT /admin/api/releases/{id}", s.requireAdmin(s.updateRelease))
	mux.HandleFunc("DELETE /admin/api/releases/{id}", s.requireAdmin(s.deleteRelease))
	mux.HandleFunc("POST /admin/api/releases/{id}/toggle", s.requireAdmin(s.toggleRelease))
	mux.HandleFunc("GET /admin/api/notices", s.requireAdmin(s.listNoticesAdmin))
	mux.HandleFunc("POST /admin/api/notices", s.requireAdmin(s.createNotice))
	mux.HandleFunc("PUT /admin/api/notices/{id}", s.requireAdmin(s.updateNotice))
	mux.HandleFunc("DELETE /admin/api/notices/{id}", s.requireAdmin(s.deleteNotice))
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
	mux.HandleFunc("POST /admin/api/open/debug", s.requireAdmin(s.openDebug))
	mux.HandleFunc("GET /admin/api/webhooks", s.requireAdmin(s.listWebhooks))
	mux.HandleFunc("POST /admin/api/webhooks", s.requireAdmin(s.createWebhook))
	mux.HandleFunc("POST /admin/api/webhooks/{id}/toggle", s.requireAdmin(s.toggleWebhook))
	mux.HandleFunc("DELETE /admin/api/webhooks/{id}", s.requireAdmin(s.deleteWebhook))
	mux.HandleFunc("GET /admin/api/webhooks/deliveries", s.requireAdmin(s.listWebhookDeliveries))
	mux.HandleFunc("GET /admin/api/ops/status", s.requireAdmin(s.opsStatus))
	mux.HandleFunc("POST /admin/api/open/keys/{id}/disable", s.requireAdmin(s.setAPIKeyStatus(false)))
	mux.HandleFunc("POST /admin/api/open/keys/{id}/enable", s.requireAdmin(s.setAPIKeyStatus(true)))

	// 客户端协议（design.md 第 12 节）
	mux.HandleFunc("POST /v1/auth/login", s.clientLogin)
	mux.HandleFunc("POST /v1/auth/refresh", s.clientRefresh)
	mux.HandleFunc("POST /v1/auth/logout", s.clientLogout)
	mux.HandleFunc("GET /v1/session", s.clientSession)
	mux.HandleFunc("POST /v1/consume", s.clientConsume)
	mux.HandleFunc("POST /v1/pause", s.clientPause)

	mux.HandleFunc("GET /v1/query/captcha", s.queryCaptcha)
	mux.HandleFunc("POST /v1/query", s.clientQuery)
	mux.HandleFunc("POST /v1/query/unbind", s.clientQueryUnbind)

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
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelDebug
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelInfo
		}
		s.log.Log(r.Context(), level, "http",
			"method", r.Method, "path", r.URL.Path, "status", status,
			"dur_ms", time.Since(start).Milliseconds())
	})
}

// statusWriter 记录实际写出的状态码，便于排查 401/429。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}
