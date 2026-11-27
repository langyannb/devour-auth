package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

// ---------- 通用解析 ----------

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "请求格式错误")
		return false
	}
	return true
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func queryInt64(r *http.Request, key string) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return v
}

// writeAudit 记录非卡操作的管理动作（策略、权限等）。
func (s *Server) writeAudit(ctx context.Context, c adminCtx, action, target, before, after, ip string) error {
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     s.now().Unix(),
		Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
		Action: action,
		Target: target,
		Before: before,
		After:  after,
		IP:     ip,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func internalErr(w http.ResponseWriter, log *slog.Logger, msg string, err error) {
	log.Error(msg, "err", err)
	writeErr(w, http.StatusInternalServerError, CodeInternal, "服务内部错误")
}

// ---------- 应用 ----------

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.st.ListApps(r.Context())
	if err != nil {
		internalErr(w, s.log, "查应用失败", err)
		return
	}
	out := make([]map[string]any, 0, len(apps))
	for _, a := range apps {
		out = append(out, appJSON(a))
	}
	writeData(w, out)
}

func appJSON(a store.App) map[string]any {
	return map[string]any{"id": a.ID, "code": a.Code, "name": a.Name, "active": a.Active}
}

// ---------- 卡类型 ----------

func policyJSON(p store.Policy) map[string]any {
	return map[string]any{
		"id": p.ID, "app_id": p.AppID, "code": p.Code, "name": p.Name, "kind": p.Kind,
		"duration_days": p.DurationDays, "duration_hours": p.DurationHours, "duration_seconds": p.DurationSeconds,
		"initial_times": p.InitialTimes, "initial_points": p.InitialPoints,
		"device_limit": p.DeviceLimit, "concurrent_limit": p.ConcurrentLimit,
		"heartbeat_sec": p.HeartbeatSec, "offline_hours": p.OfflineHours, "offline_starts": p.OfflineStarts,
		"rebind_free": p.RebindFree, "rebind_cooldown_days": p.RebindCooldownDays, "rebind_cost_hours": p.RebindCostHours,
		"renewable": p.Renewable, "trial_per_device": p.TrialPerDevice,
		"price_cents": p.PriceCents, "active": p.Active, "created_at": p.CreatedAt,
	}
}

type policyReq struct {
	Code               *string   `json:"code"`
	Name               *string   `json:"name"`
	Kind               *string   `json:"kind"`
	DurationDays       *int64    `json:"duration_days"`
	DurationHours      *int64    `json:"duration_hours"`
	DurationSeconds    *int64    `json:"duration_seconds"`
	InitialTimes       *int64    `json:"initial_times"`
	InitialPoints      *int64    `json:"initial_points"`
	DeviceLimit        *int64    `json:"device_limit"`
	ConcurrentLimit    *int64    `json:"concurrent_limit"`
	HeartbeatSec       *int64    `json:"heartbeat_sec"`
	OfflineHours       *int64    `json:"offline_hours"`
	OfflineStarts      *int64    `json:"offline_starts"`
	RebindFree         *int64    `json:"rebind_free"`
	RebindCooldownDays *int64    `json:"rebind_cooldown_days"`
	RebindCostHours    *int64    `json:"rebind_cost_hours"`
	Renewable          *int64    `json:"renewable"`
	TrialPerDevice     *int64    `json:"trial_per_device"`
	PriceCents         *int64    `json:"price_cents"`
	Active             *int64    `json:"active"`
	Entitlements       *[]string `json:"entitlements"`
}

func (req policyReq) apply(p *store.Policy) {
	set := func(dst *int64, src *int64) {
		if src != nil {
			*dst = *src
		}
	}
	set(&p.DurationDays, req.DurationDays)
	set(&p.DurationHours, req.DurationHours)
	set(&p.DurationSeconds, req.DurationSeconds)
	set(&p.InitialTimes, req.InitialTimes)
	set(&p.InitialPoints, req.InitialPoints)
	set(&p.DeviceLimit, req.DeviceLimit)
	set(&p.ConcurrentLimit, req.ConcurrentLimit)
	set(&p.HeartbeatSec, req.HeartbeatSec)
	set(&p.OfflineHours, req.OfflineHours)
	set(&p.OfflineStarts, req.OfflineStarts)
	set(&p.RebindFree, req.RebindFree)
	set(&p.RebindCooldownDays, req.RebindCooldownDays)
	set(&p.RebindCostHours, req.RebindCostHours)
	set(&p.Renewable, req.Renewable)
	set(&p.TrialPerDevice, req.TrialPerDevice)
	set(&p.PriceCents, req.PriceCents)
	set(&p.Active, req.Active)
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Kind != nil {
		p.Kind = *req.Kind
	}
}

func validKind(k string) bool {
	switch k {
	case store.KindDuration, store.KindForever, store.KindCount, store.KindCredit, store.KindTimed, store.KindTrial:
		return true
	}
	return false
}

// resolveEntitlements 把权限 code 列表换成 id，任一缺失即报错。
func (s *Server) resolveEntitlements(ctx context.Context, appID int64, codes []string) ([]int64, error) {
	ids := make([]int64, 0, len(codes))
	for _, code := range codes {
		e, err := s.st.EntitlementByCode(ctx, appID, code)
		if err != nil {
			return nil, fmt.Errorf("权限 %s 不存在", code)
		}
		ids = append(ids, e.ID)
	}
	return ids, nil
}

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request) {
	appID := queryInt64(r, "app_id")
	policies, err := s.st.ListPolicies(r.Context(), appID)
	if err != nil {
		internalErr(w, s.log, "查卡类型失败", err)
		return
	}
	out := make([]map[string]any, 0, len(policies))
	for _, p := range policies {
		out = append(out, policyJSON(p))
	}
	writeData(w, out)
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req policyReq
	if !decode(w, r, &req) {
		return
	}
	if req.Code == nil || *req.Code == "" || req.Name == nil || req.Kind == nil || !validKind(*req.Kind) {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "code / name / kind 必填且 kind 合法")
		return
	}
	appID := queryInt64(r, "app_id")
	if appID <= 0 {
		apps, err := s.st.ListApps(ctx)
		if err != nil || len(apps) == 0 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "缺少 app_id")
			return
		}
		appID = apps[0].ID
	}

	p := store.Policy{
		AppID: appID, Code: *req.Code, Kind: *req.Kind, Name: *req.Name,
		DeviceLimit: 1, ConcurrentLimit: 1, HeartbeatSec: 600,
		OfflineHours: 24, OfflineStarts: 20, RebindFree: 1,
		RebindCooldownDays: 30, Renewable: 1, TrialPerDevice: 1, Active: 1,
		CreatedAt: s.now().Unix(),
	}
	req.apply(&p)
	if p.DeviceLimit < 1 {
		p.DeviceLimit = 1
	}
	id, err := s.st.CreatePolicy(ctx, p)
	if err != nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "卡类型已存在或参数冲突")
		return
	}
	if req.Entitlements != nil {
		ids, err := s.resolveEntitlements(ctx, appID, *req.Entitlements)
		if err != nil {
			writeErr(w, http.StatusBadRequest, CodeBadInput, err.Error())
			return
		}
		if err := s.st.SetPolicyEntitlements(ctx, id, ids); err != nil {
			internalErr(w, s.log, "绑定权限失败", err)
			return
		}
	}
	p.ID = id
	if err := s.writeAudit(ctx, adminFrom(r), "policy.create",
		fmt.Sprintf("policy:%d", id), "", jsonOf(policyJSON(p)), clientIP(r)); err != nil {
		internalErr(w, s.log, "写审计失败", err)
		return
	}
	writeData(w, policyJSON(p))
}

func (s *Server) updatePolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	old, err := s.st.PolicyByID(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡类型不存在")
		return
	}
	var req policyReq
	if !decode(w, r, &req) {
		return
	}
	p := old
	req.apply(&p)
	if !validKind(p.Kind) {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "kind 不合法")
		return
	}
	if err := s.st.UpdatePolicy(ctx, p); err != nil {
		internalErr(w, s.log, "更新卡类型失败", err)
		return
	}
	if req.Entitlements != nil {
		ids, err := s.resolveEntitlements(ctx, p.AppID, *req.Entitlements)
		if err != nil {
			writeErr(w, http.StatusBadRequest, CodeBadInput, err.Error())
			return
		}
		if err := s.st.SetPolicyEntitlements(ctx, p.ID, ids); err != nil {
			internalErr(w, s.log, "绑定权限失败", err)
			return
		}
	}
	if err := s.writeAudit(ctx, adminFrom(r), "policy.update",
		fmt.Sprintf("policy:%d", id), jsonOf(policyJSON(old)), jsonOf(policyJSON(p)), clientIP(r)); err != nil {
		internalErr(w, s.log, "写审计失败", err)
		return
	}
	writeData(w, policyJSON(p))
}

// ---------- 功能权限 ----------

func entJSON(e store.Entitlement) map[string]any {
	return map[string]any{
		"id": e.ID, "app_id": e.AppID, "code": e.Code, "name": e.Name,
		"cost_points": e.CostPoints, "cost_times": e.CostTimes,
		"active": e.Active, "created_at": e.CreatedAt,
	}
}

func (s *Server) listEntitlements(w http.ResponseWriter, r *http.Request) {
	es, err := s.st.ListEntitlements(r.Context(), queryInt64(r, "app_id"))
	if err != nil {
		internalErr(w, s.log, "查权限失败", err)
		return
	}
	out := make([]map[string]any, 0, len(es))
	for _, e := range es {
		out = append(out, entJSON(e))
	}
	writeData(w, out)
}

type entReq struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	CostPoints int64  `json:"cost_points"`
	CostTimes  int64  `json:"cost_times"`
}

func (s *Server) createEntitlement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req entReq
	if !decode(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" || req.CostPoints < 0 || req.CostTimes < 0 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "code / name 必填，成本不能为负")
		return
	}
	appID := queryInt64(r, "app_id")
	if appID <= 0 {
		apps, err := s.st.ListApps(ctx)
		if err != nil || len(apps) == 0 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "缺少 app_id")
			return
		}
		appID = apps[0].ID
	}
	e := store.Entitlement{
		AppID: appID, Code: req.Code, Name: req.Name,
		CostPoints: req.CostPoints, CostTimes: req.CostTimes,
		Active: 1, CreatedAt: s.now().Unix(),
	}
	id, err := s.st.CreateEntitlement(ctx, e)
	if err != nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "权限 code 已存在")
		return
	}
	e.ID = id
	if err := s.writeAudit(ctx, adminFrom(r), "entitlement.create",
		fmt.Sprintf("entitlement:%d", id), "", jsonOf(entJSON(e)), clientIP(r)); err != nil {
		internalErr(w, s.log, "写审计失败", err)
		return
	}
	writeData(w, entJSON(e))
}

// ---------- 卡密 ----------

func cardJSON(c store.Card) map[string]any {
	var batchID, agentID, activatedAt, bannedAt any
	if c.BatchID.Valid {
		batchID = c.BatchID.Int64
	}
	if c.AgentID.Valid {
		agentID = c.AgentID.Int64
	}
	if c.ActivatedAt.Valid {
		activatedAt = c.ActivatedAt.Int64
	}
	if c.BannedAt.Valid {
		bannedAt = c.BannedAt.Int64
	}
	return map[string]any{
		"id": c.ID, "app_id": c.AppID, "kami": c.Kami, "policy_id": c.PolicyID,
		"kind": c.Kind, "status": c.Status, "batch_id": batchID, "agent_id": agentID,
		"device_limit": c.DeviceLimit, "expire_at": c.ExpireAt,
		"remain_times": c.RemainTimes, "points": c.Points,
		"remaining_seconds": c.RemainingSeconds, "paused": c.Paused,
		"rebind_used": c.RebindUsed, "note": c.Note,
		"activated_at": activatedAt, "banned_at": bannedAt, "created_at": c.CreatedAt,
	}
}

type generateReq struct {
	PolicyID int64  `json:"policy_id"`
	Qty      int    `json:"qty"`
	Prefix   string `json:"prefix"`
	Note     string `json:"note"`
}

func (s *Server) generateCards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req generateReq
	if !decode(w, r, &req) {
		return
	}
	p, err := s.st.PolicyByID(ctx, req.PolicyID)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡类型不存在")
		return
	}
	c := adminFrom(r)
	res, err := s.cards.Generate(ctx, card.GenerateReq{
		AppID: p.AppID, PolicyID: p.ID, Qty: req.Qty, Prefix: req.Prefix,
		CreatedBy: c.Admin.ID,
		Actor:     fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
		IP:        clientIP(r),
		Note:      req.Note,
	})
	if err != nil {
		if errors.Is(err, card.ErrInvalid) {
			writeErr(w, http.StatusBadRequest, CodeBadInput, err.Error())
			return
		}
		internalErr(w, s.log, "发卡失败", err)
		return
	}
	writeData(w, map[string]any{"batch_id": res.BatchID, "kamis": res.Kamis, "qty": len(res.Kamis)})
}

func (s *Server) listCards(w http.ResponseWriter, r *http.Request) {
	cards, total, err := s.st.ListCards(r.Context(), store.CardFilter{
		AppID:    queryInt64(r, "app_id"),
		Status:   r.URL.Query().Get("status"),
		PolicyID: queryInt64(r, "policy_id"),
		Kami:     r.URL.Query().Get("kami"),
		Limit:    int(queryInt64(r, "limit")),
		Offset:   int(queryInt64(r, "offset")),
	})
	if err != nil {
		internalErr(w, s.log, "查卡失败", err)
		return
	}
	out := make([]map[string]any, 0, len(cards))
	for _, c := range cards {
		out = append(out, cardJSON(c))
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

func (s *Server) getCard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	c, err := s.st.CardByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
		return
	}
	bound, err := s.st.CountBoundDevices(r.Context(), id)
	if err != nil {
		internalErr(w, s.log, "查设备失败", err)
		return
	}
	// snapshot 是出卡时冻结的策略副本，详情页要展示
	var snap any
	_ = json.Unmarshal([]byte(c.SnapshotJSON), &snap)
	out := cardJSON(c)
	out["bound_devices"] = bound
	out["snapshot"] = snap
	writeData(w, out)
}

func (s *Server) cardStatusAction(action string, fn func(context.Context, int64, card.StatusChange) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(r)
		if !ok {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
			return
		}
		c := adminFrom(r)
		ch := card.StatusChange{
			Action: "card." + action,
			Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
			IP:     clientIP(r),
		}
		if err := fn(r.Context(), id, ch); err != nil {
			switch {
			case errors.Is(err, store.ErrNotFound):
				writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
			case errors.Is(err, card.ErrState), errors.Is(err, card.ErrInvalid):
				writeErr(w, http.StatusConflict, CodeBadInput, err.Error())
			default:
				internalErr(w, s.log, "操作失败", err)
			}
			return
		}
		writeData(w, map[string]any{"id": id, "updated": true})
	}
}

type extendReq struct {
	Days    int64 `json:"days"`
	Times   int64 `json:"times"`
	Points  int64 `json:"points"`
	Seconds int64 `json:"seconds"`
}

func (s *Server) extendCard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req extendReq
	if !decode(w, r, &req) {
		return
	}
	c := adminFrom(r)
	err := s.cards.Extend(r.Context(), id, card.ExtendAdd{
		Days: req.Days, Times: req.Times, Points: req.Points, Seconds: req.Seconds,
	}, card.StatusChange{
		Action: "card.extend",
		Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
		IP:     clientIP(r),
	})
	switch {
	case err == nil:
		writeData(w, map[string]any{"id": id, "updated": true})
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
	case errors.Is(err, card.ErrState), errors.Is(err, card.ErrInvalid):
		writeErr(w, http.StatusConflict, CodeBadInput, err.Error())
	default:
		internalErr(w, s.log, "续期失败", err)
	}
}

// ---------- 审计 ----------

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	rows, total, err := s.st.ListAudit(r.Context(),
		int(queryInt64(r, "limit")), int(queryInt64(r, "offset")))
	if err != nil {
		internalErr(w, s.log, "查审计失败", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		out = append(out, map[string]any{
			"id": a.ID, "ts": a.TS, "actor": a.Actor, "action": a.Action,
			"target": a.Target, "before": a.Before, "after": a.After, "ip": a.IP,
		})
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
