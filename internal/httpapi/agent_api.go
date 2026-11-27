package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/langyannb/devour-auth/internal/agent"
	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

const agentIdleTimeout = 30 * time.Minute

type agentCtx struct {
	Agent   store.Agent
	Session store.AgentSession
}

type agentCtxKey struct{}

func agentFrom(r *http.Request) agentCtx {
	return r.Context().Value(agentCtxKey{}).(agentCtx)
}

// requireAgent 只认代理会话，和管理端 cookie 完全隔离（design.md 第 5 节）。
func (s *Server) requireAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		cookie, err := r.Cookie(s.agentCookieName())
		if err != nil {
			if _, aerr := r.Cookie(s.adminCookieName()); aerr == nil {
				writeErr(w, http.StatusForbidden, CodeUnauthorized, "管理端会话不能访问代理端")
				return
			}
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "未登录")
			return
		}
		sess, err := s.st.AgentSessionByTokenHash(ctx, auth.HashToken(cookie.Value))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "会话已失效")
			return
		}
		now := s.now().Unix()
		if sess.ExpiresAt <= now || sess.LastSeen+int64(agentIdleTimeout/time.Second) <= now {
			_ = s.st.RevokeAgentSession(ctx, sess.ID, now)
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
		if sess.IP != "" && !sameSubnet(sess.IP, clientIP(r)) {
			_ = s.st.RevokeAgentSession(ctx, sess.ID, now)
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "登录 IP 已变更，请重新登录")
			return
		}
		a, err := s.st.AgentByID(ctx, sess.AgentID)
		if err != nil || a.Status != "active" {
			writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "账号不可用")
			return
		}
		_ = s.st.TouchAgentSession(ctx, sess.ID, now)
		next(w, r.WithContext(context.WithValue(r.Context(), agentCtxKey{}, agentCtx{Agent: a, Session: sess})))
	}
}

func (s *Server) agentCookieName() string {
	if s.env == "prod" {
		return "__Host-devour_agent"
	}
	return "devour_agent"
}

func (s *Server) setAgentCookie(w http.ResponseWriter, token string, expires int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.agentCookieName(),
		Value:    token,
		Path:     "/",
		Expires:  time.Unix(expires, 0),
		MaxAge:   int(agent.SessionTTL / time.Second),
		HttpOnly: true,
		Secure:   s.env == "prod",
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearAgentCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.agentCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.env == "prod",
		SameSite: http.SameSiteStrictMode,
	})
}

// ---------- 登录 ----------

type agentLoginReq struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (s *Server) agentLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.lim.Allow("agent:login:ip:"+ip, 5, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁")
		return
	}
	var req agentLoginReq
	if !decode(w, r, &req) {
		return
	}
	if !s.lim.Allow("agent:login:acct:"+req.Name, 5, 15*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁")
		return
	}
	res, err := s.agents.Login(r.Context(), req.Name, req.Password, ip, truncate(r.UserAgent(), 200))
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	s.lim.Reset("agent:login:acct:" + req.Name)
	s.setAgentCookie(w, res.Token, res.ExpiresAt)
	writeData(w, map[string]any{
		"csrf_token":  res.CSRF,
		"expires_at":  res.ExpiresAt,
		"id":          res.Agent.ID,
		"name":        res.Agent.Name,
		"level":       res.Agent.Level,
		"discount_bp": res.Agent.DiscountBP,
		"server_time": s.now().Unix(),
	})
}

func (s *Server) agentLogout(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	if err := s.agents.Logout(r.Context(), c.Session.ID); err != nil {
		s.log.Error("代理登出失败", "err", err)
	}
	s.clearAgentCookie(w)
	writeData(w, map[string]any{"logged_out": true})
}

func (s *Server) agentMe(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	ctx := r.Context()
	ids, err := s.agents.VisibleAgentIDs(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查可见范围失败", err)
		return
	}
	subs, err := s.st.ListAgents(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查下级失败", err)
		return
	}
	writeData(w, map[string]any{
		"id":                  c.Agent.ID,
		"name":                c.Agent.Name,
		"level":               c.Agent.Level,
		"discount_bp":         c.Agent.DiscountBP,
		"monthly_quota_cents": c.Agent.MonthlyQuotaCents,
		"visible_agent_ids":   ids,
		"subagents":           len(subs),
		"server_time":         s.now().Unix(),
	})
}

func (s *Server) agentPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apps, err := s.st.ListApps(ctx)
	if err != nil || len(apps) == 0 {
		writeData(w, []map[string]any{})
		return
	}
	policies, err := s.st.ListPolicies(r.Context(), apps[0].ID)
	if err != nil {
		internalErr(w, s.log, "查卡类型失败", err)
		return
	}
	c := agentFrom(r)
	prices, err := s.st.ListAgentPrices(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查代理价格失败", err)
		return
	}
	out := make([]map[string]any, 0, len(policies))
	for _, p := range policies {
		if p.Active != 1 || isInternalPolicy(p.Code) {
			continue
		}
		m := policyJSON(p)
		price := agent.PayableCents(p.PriceCents, c.Agent.DiscountBP, 1)
		if v, ok := prices[p.ID]; ok {
			price = v
		}
		m["agent_price_cents"] = price
		m["has_custom_price"] = func() bool { _, ok := prices[p.ID]; return ok }()
		out = append(out, m)
	}
	writeData(w, out)
}

type agentIssueReq struct {
	// 新模型：按价格模板出卡
	Kind        string   `json:"kind"`
	Multiplier  int64    `json:"multiplier"`
	Value       int64    `json:"value"`
	Unit        string   `json:"unit"`
	DeviceLimit int64    `json:"device_limit"`
	Kamis       []string `json:"kamis"`
	// 旧模型
	PolicyID int64  `json:"policy_id"`
	Qty      int    `json:"qty"`
	Prefix   string `json:"prefix"`
}

// agentPlan 返回代理商绑定的价格模板与权限（出卡页用）。
func (s *Server) agentPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := agentFrom(r)
	perms, planID, err := s.st.AgentPermissions(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查权限失败", err)
		return
	}
	if planID == 0 {
		writeData(w, map[string]any{"plan": nil, "permissions": perms})
		return
	}
	plan, err := s.st.AgentPlanByID(ctx, planID)
	if err != nil {
		writeData(w, map[string]any{"plan": nil, "permissions": perms})
		return
	}
	items := make([]map[string]any, 0, len(plan.Items))
	for kind, price := range plan.Items {
		name := map[string]string{"day": "天卡", "week": "周卡", "month": "月卡", "forever": "永久卡", "custom": "自定义时间"}[kind]
		items = append(items, map[string]any{"kind": kind, "name": name, "price_cents": price})
	}
	writeData(w, map[string]any{
		"plan":        map[string]any{"id": plan.ID, "name": plan.Name, "items": items},
		"permissions": perms,
	})
}

func (s *Server) agentIssue(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	var req agentIssueReq
	if !decode(w, r, &req) {
		return
	}

	// 新模型：按价格模板出卡
	if req.Kind != "" {
		app, err := s.appFromQuery(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
			return
		}
		res, err := s.agents.IssueByPlan(r.Context(), agent.PlanIssueReq{
			AgentID: c.Agent.ID, AppID: app.ID,
			Kind: req.Kind, Multiplier: req.Multiplier, Value: req.Value, Unit: req.Unit,
			Qty: req.Qty, DeviceLimit: req.DeviceLimit, Kamis: req.Kamis,
			IP: clientIP(r),
		})
		if err != nil {
			if errors.Is(err, agent.ErrQuota) {
				writeErr(w, http.StatusForbidden, CodeBadInput, err.Error())
				return
			}
			s.writeAPIError(w, err)
			return
		}
		writeData(w, map[string]any{
			"batch_id": res.BatchID, "kamis": res.Kamis, "payable": res.Payable, "rebate": res.Rebate,
		})
		return
	}

	res, err := s.agents.Issue(r.Context(), agent.IssueReq{
		AgentID:  c.Agent.ID,
		PolicyID: req.PolicyID,
		Qty:      req.Qty,
		Prefix:   req.Prefix,
		IP:       clientIP(r),
	})
	if err != nil {
		if errors.Is(err, agent.ErrQuota) {
			writeErr(w, http.StatusForbidden, CodeBadInput, err.Error())
			return
		}
		s.writeAPIError(w, err)
		return
	}
	writeData(w, map[string]any{
		"batch_id": res.BatchID,
		"kamis":    res.Kamis,
		"payable":  res.Payable,
		"rebate":   res.Rebate,
	})
}

func (s *Server) agentCards(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	ctx := r.Context()
	ids, err := s.agents.VisibleAgentIDs(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查可见范围失败", err)
		return
	}
	cards, total, err := s.st.ListCards(ctx, store.CardFilter{
		AgentIDs: ids,
		Status:   r.URL.Query().Get("status"),
		Kami:     r.URL.Query().Get("kami"),
		Limit:    int(queryInt64(r, "limit")),
		Offset:   int(queryInt64(r, "offset")),
	})
	if err != nil {
		internalErr(w, s.log, "查卡失败", err)
		return
	}
	out := make([]map[string]any, 0, len(cards))
	for _, cd := range cards {
		out = append(out, cardJSON(cd))
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

// agentSubagents 代理可见的下级列表（含自己）。
func (s *Server) agentSubagents(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	ctx := r.Context()
	subs, err := s.st.ListAgents(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查下级失败", err)
		return
	}
	out := make([]map[string]any, 0, len(subs)+1)
	me := agentJSON(c.Agent)
	me["is_self"] = true
	out = append(out, me)
	for _, a := range subs {
		out = append(out, agentJSON(a))
	}
	writeData(w, out)
}

func (s *Server) agentLedger(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	rows, total, err := s.st.ListAgentLedger(r.Context(), c.Agent.ID,
		int(queryInt64(r, "limit")), int(queryInt64(r, "offset")))
	if err != nil {
		internalErr(w, s.log, "查流水失败", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		out = append(out, map[string]any{
			"id": x.ID, "kind": x.Kind, "amount_cents": x.AmountCents,
			"ref": x.Ref, "created_at": x.CreatedAt,
		})
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

func (s *Server) agentBills(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	bills, err := s.st.ListAgentBills(r.Context(), c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查账单失败", err)
		return
	}
	out := make([]map[string]any, 0, len(bills))
	for _, b := range bills {
		out = append(out, billJSON(b))
	}
	writeData(w, out)
}

func billJSON(b store.AgentBill) map[string]any {
	var settled any
	if b.SettledAt.Valid {
		settled = b.SettledAt.Int64
	}
	return map[string]any{
		"id": b.ID, "agent_id": b.AgentID, "month": b.Month,
		"amount_cents": b.AmountCents, "status": b.Status,
		"settled_at": settled, "note": b.Note,
	}
}

func agentJSON(a store.Agent) map[string]any {
	var parent, frozen any
	if a.ParentID.Valid {
		parent = a.ParentID.Int64
	}
	if a.FrozenAt.Valid {
		frozen = a.FrozenAt.Int64
	}
	return map[string]any{
		"id": a.ID, "name": a.Name, "parent_id": parent, "level": a.Level,
		"discount_bp": a.DiscountBP, "rebate_bp": a.RebateBP,
		"quota_cents": a.QuotaCents, "monthly_quota_cents": a.MonthlyQuotaCents, "status": a.Status,
		"created_at": a.CreatedAt, "frozen_at": frozen,
	}
}

// ---------- 管理员侧 ----------

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListAgents(r.Context(), queryInt64(r, "parent_id"))
	if err != nil {
		internalErr(w, s.log, "查代理失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, a := range list {
		out = append(out, agentJSON(a))
	}
	writeData(w, out)
}

type createAgentReq struct {
	Name              string `json:"name"`
	Password          string `json:"password"`
	ParentID          int64  `json:"parent_id"`
	DiscountBP        int64  `json:"discount_bp"`
	RebateBP          int64  `json:"rebate_bp"`
	MonthlyQuotaCents int64  `json:"monthly_quota_cents"`
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createAgentReq
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" || len(req.Password) < auth.MinPasswordLen {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "代理名必填，密码至少 12 位")
		return
	}
	if req.DiscountBP <= 0 {
		req.DiscountBP = 10000 // 折扣已废弃，价格由模板决定
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}

	a := store.Agent{
		Name: req.Name, Level: 1, DiscountBP: req.DiscountBP,
		RebateBP:          req.RebateBP,
		MonthlyQuotaCents: req.MonthlyQuotaCents,
		QuotaCents:        req.MonthlyQuotaCents,
		PassHash:          hash, Status: "active", CreatedAt: s.now().Unix(),
	}
	if req.ParentID > 0 {
		parent, err := s.st.AgentByID(ctx, req.ParentID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "上级代理不存在")
			return
		}
		if parent.Level != 1 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "最多两级：上级必须是 level=1")
			return
		}
		a.ParentID = store.NullInt64(req.ParentID)
		a.Level = 2
	}
	id, err := s.st.CreateAgent(ctx, a)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, CodeBadInput, "代理名已存在")
			return
		}
		internalErr(w, s.log, "创建代理失败", err)
		return
	}
	a.ID = id
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "agent.create", fmt.Sprintf("agent:%d", id), "", jsonOf(agentJSON(a)), clientIP(r))
	writeData(w, agentJSON(a))
}

type updateAgentReq struct {
	DiscountBP        *int64 `json:"discount_bp"`
	RebateBP          *int64 `json:"rebate_bp"`
	MonthlyQuotaCents *int64 `json:"monthly_quota_cents"`
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	old, err := s.st.AgentByID(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "代理不存在")
		return
	}
	var req updateAgentReq
	if !decode(w, r, &req) {
		return
	}
	discount, rebate, quota := old.DiscountBP, old.RebateBP, old.MonthlyQuotaCents
	if req.DiscountBP != nil {
		if *req.DiscountBP <= 0 || *req.DiscountBP > 10000 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "折扣基点需在 1..10000")
			return
		}
		discount = *req.DiscountBP
	}
	if req.RebateBP != nil {
		rebate = *req.RebateBP
	}
	if req.MonthlyQuotaCents != nil {
		quota = *req.MonthlyQuotaCents
	}
	if err := s.st.UpdateAgentPricing(ctx, id, discount, rebate, quota); err != nil {
		internalErr(w, s.log, "更新代理失败", err)
		return
	}
	updated, _ := s.st.AgentByID(ctx, id)
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "agent.update", fmt.Sprintf("agent:%d", id),
		jsonOf(agentJSON(old)), jsonOf(agentJSON(updated)), clientIP(r))
	writeData(w, agentJSON(updated))
}

func (s *Server) setAgentFrozen(frozen bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, ok := pathID(r)
		if !ok {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
			return
		}
		status := "active"
		frozenAt := store.NullInt64(0)
		if frozen {
			status = "frozen"
			frozenAt = store.NullInt64(s.now().Unix())
		}
		if err := s.st.SetAgentStatus(ctx, id, status, frozenAt); err != nil {
			internalErr(w, s.log, "更新状态失败", err)
			return
		}
		action := "agent.unfreeze"
		if frozen {
			action = "agent.freeze"
		}
		c := adminFrom(r)
		_ = s.writeAudit(ctx, c, action, fmt.Sprintf("agent:%d", id), "", fmt.Sprintf(`{"status":%q}`, status), clientIP(r))
		writeData(w, map[string]any{"id": id, "status": status})
	}
}

func (s *Server) agentBillsAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	bills, err := s.st.ListAgentBills(r.Context(), id)
	if err != nil {
		internalErr(w, s.log, "查账单失败", err)
		return
	}
	out := make([]map[string]any, 0, len(bills))
	for _, b := range bills {
		out = append(out, billJSON(b))
	}
	writeData(w, out)
}

func (s *Server) settleBill(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("billId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "billId 不合法")
		return
	}
	if err := s.st.SettleAgentBill(r.Context(), id, s.now().Unix(), ""); err != nil {
		internalErr(w, s.log, "结算失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "agent_bill.settle", fmt.Sprintf("bill:%d", id), "", `{"status":"settled"}`, clientIP(r))
	writeData(w, map[string]any{"id": id, "status": "settled"})
}

// agentQuotaTopup 给代理加/扣额度并记流水。
func (s *Server) agentQuotaTopup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	a, err := s.st.AgentByID(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "代理不存在")
		return
	}
	var req struct {
		DeltaCents int64  `json:"delta_cents"`
		Note       string `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.DeltaCents == 0 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "金额不能为 0")
		return
	}
	if a.QuotaCents < 0 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "该代理是不限额，先把额度设成具体数字再加减")
		return
	}
	newQuota, err := s.st.AddAgentQuota(ctx, id, req.DeltaCents)
	if err != nil {
		internalErr(w, s.log, "调整额度失败", err)
		return
	}
	if newQuota < 0 {
		// 不允许扣成负数，回滚
		_, _ = s.st.AddAgentQuota(ctx, id, -req.DeltaCents)
		writeErr(w, http.StatusBadRequest, CodeBadInput, "扣减后额度会变成负数")
		return
	}
	kind := "topup"
	if req.DeltaCents < 0 {
		kind = "adjust"
	}
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err == nil {
		_ = s.st.AddAgentLedgerTx(ctx, tx, id, kind, -req.DeltaCents, req.Note, s.now().Unix())
		_ = tx.Commit()
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "agent.quota_topup", fmt.Sprintf("agent:%d", id),
		fmt.Sprintf(`{"quota":%d}`, a.QuotaCents), fmt.Sprintf(`{"quota":%d,"note":%q}`, newQuota, req.Note), clientIP(r))
	writeData(w, map[string]any{"id": id, "quota_cents": newQuota})
}
