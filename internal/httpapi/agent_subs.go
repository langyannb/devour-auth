package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

// agentAudit 代理侧操作写审计（actor 是 agent:id）。
func (s *Server) agentAudit(ctx context.Context, agentID int64, action, target, after, ip string) {
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if err := audit.Append(ctx, tx, audit.Entry{
		TS: s.now().Unix(), Actor: fmt.Sprintf("agent:%d", agentID),
		Action: action, Target: target, After: after, IP: ip,
	}); err != nil {
		return
	}
	_ = tx.Commit()
}

// planKinds 代理侧只认这几种卡型。
var planKinds = []string{"day", "week", "month", "forever"}

func planKindName(kind string) string {
	switch kind {
	case "day":
		return "天卡"
	case "week":
		return "周卡"
	case "month":
		return "月卡"
	case "forever":
		return "永久卡"
	}
	return kind
}

// agentPlans 列出本应用的模板，供一级代理给下级挑。
func (s *Server) agentPlans(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	list, err := s.st.ListAgentPlans(r.Context(), c.Agent.AppID)
	if err != nil {
		internalErr(w, s.log, "查模板失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		if p.Active != 1 {
			continue
		}
		if strings.HasPrefix(p.Name, "sub_") { // 别的下级专属模板
			continue
		}
		items := make([]map[string]any, 0, len(p.Items))
		for _, k := range planKinds {
			if price, ok := p.Items[k]; ok && price > 0 {
				items = append(items, map[string]any{"kind": k, "name": planKindName(k), "price_cents": price})
			}
		}
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "items": items})
	}
	writeData(w, out)
}

// subAgent 取当前代理的下级，校验归属。
func (s *Server) subAgent(w http.ResponseWriter, r *http.Request) (store.Agent, store.Agent, bool) {
	c := agentFrom(r)
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return c.Agent, store.Agent{}, false
	}
	sub, err := s.st.AgentByID(r.Context(), id)
	if err != nil || !sub.ParentID.Valid || sub.ParentID.Int64 != c.Agent.ID || sub.AppID != c.Agent.AppID {
		writeErr(w, http.StatusNotFound, CodeBadInput, "下级不存在")
		return c.Agent, store.Agent{}, false
	}
	return c.Agent, sub, true
}

// agentSubPlan 一级代理给下级设价格模板：每一档都不能低于自己的成本。
func (s *Server) agentSubPlan(w http.ResponseWriter, r *http.Request) {
	parent, sub, ok := s.subAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		PlanID int64            `json:"plan_id"`
		Items  map[string]int64 `json:"items"` // 自定义价格（分），优先于 plan_id
	}
	if !decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	_, parentPlanID, err := s.st.AgentPermissions(ctx, parent.ID)
	if err != nil {
		internalErr(w, s.log, "查上级模板失败", err)
		return
	}
	if parentPlanID == 0 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "先给自己分配价格模板，再给下级设")
		return
	}
	parentPlan, err := s.st.AgentPlanByID(ctx, parentPlanID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "上级模板不存在")
		return
	}
	var childPlan store.AgentPlan
	if len(req.Items) > 0 {
		items := map[string]int64{}
		for _, kind := range planKinds {
			if price := req.Items[kind]; price > 0 {
				items[kind] = price
			}
		}
		if len(items) == 0 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "至少要设置一种卡的价格")
			return
		}
		name := fmt.Sprintf("sub_%d", sub.ID)
		id := int64(0)
		if old, err := s.st.AgentPlanByName(ctx, parent.AppID, name); err == nil {
			id = old.ID
		}
		pid, err := s.st.SaveAgentPlan(ctx, store.AgentPlan{
			ID: id, AppID: parent.AppID, Name: name, Active: 1, Items: items,
		}, s.now().Unix())
		if err != nil {
			internalErr(w, s.log, "保存下级模板失败", err)
			return
		}
		childPlan, err = s.st.AgentPlanByID(ctx, pid)
		if err != nil {
			internalErr(w, s.log, "读下级模板失败", err)
			return
		}
	} else {
		var err error
		childPlan, err = s.st.AgentPlanByID(ctx, req.PlanID)
		if err != nil || childPlan.AppID != parent.AppID || childPlan.Active != 1 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "模板不存在")
			return
		}
	}
	for _, kind := range planKinds {
		childPrice, hasChild := childPlan.Items[kind]
		if !hasChild || childPrice <= 0 {
			continue
		}
		base, hasParent := parentPlan.Items[kind]
		if !hasParent {
			continue // 自己没有这一档，不设下限，由代理自己定价
		}
		if childPrice < base {
			writeErr(w, http.StatusBadRequest, CodeBadInput,
				fmt.Sprintf("「%s」下级的价（%d 分）不能低于你的成本（%d 分）", planKindName(kind), childPrice, base))
			return
		}
	}

	// 保留下级原有权限，只换模板
	perms, _, err := s.st.AgentPermissions(ctx, sub.ID)
	if err != nil {
		internalErr(w, s.log, "查下级权限失败", err)
		return
	}
	if err := s.st.SetAgentPlanAndPerms(ctx, sub.ID, childPlan.ID, perms); err != nil {
		internalErr(w, s.log, "设置下级模板失败", err)
		return
	}
	s.agentAudit(r.Context(), parent.ID, "agent.sub_plan", fmt.Sprintf("agent:%d", sub.ID), fmt.Sprintf(`{"plan_id":%d}`, childPlan.ID), clientIP(r))
	writeData(w, map[string]any{"id": sub.ID, "plan_id": childPlan.ID, "plan_name": childPlan.Name})
}

// agentSubQuota 上级把额度划给下级。
func (s *Server) agentSubQuota(w http.ResponseWriter, r *http.Request) {
	parent, sub, ok := s.subAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		DeltaCents int64 `json:"delta_cents"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.DeltaCents <= 0 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "划转额度必须大于 0")
		return
	}
	ctx := r.Context()
	if err := s.st.TransferAgentQuota(ctx, parent.ID, sub.ID, req.DeltaCents, sub.Name, s.now().Unix()); err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, err.Error())
		return
	}
	got, _ := s.st.AgentByID(ctx, sub.ID)
	me, _ := s.st.AgentByID(ctx, parent.ID)
	writeData(w, map[string]any{
		"sub_quota_cents": got.QuotaCents, "my_quota_cents": me.QuotaCents,
	})
}

// agentSubFrozen 上级冻结/解冻下级。
func (s *Server) agentSubFrozen(frozen bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parent, sub, ok := s.subAgent(w, r)
		if !ok {
			return
		}
		ctx := r.Context()
		var at sql.NullInt64
		status := "active"
		if frozen {
			status = "frozen"
			at = store.NullInt64(s.now().Unix())
		}
		if err := s.st.SetAgentStatus(ctx, sub.ID, status, at); err != nil {
			internalErr(w, s.log, "改下级状态失败", err)
			return
		}
		s.agentAudit(ctx, parent.ID, "agent.sub_status", fmt.Sprintf("agent:%d", sub.ID), fmt.Sprintf(`{"status":%q}`, status), clientIP(r))
		writeData(w, map[string]any{"id": sub.ID, "status": status})
	}
}

// agentSubPerms 上级设置下级权限，不能超过自己。
func (s *Server) agentSubPerms(w http.ResponseWriter, r *http.Request) {
	parent, sub, ok := s.subAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		SetDeviceLimit bool  `json:"set_device_limit"`
		CustomKami     bool  `json:"custom_kami"`
		MaxQty         int64 `json:"max_qty"`
	}
	if !decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	myPerms, _, err := s.st.AgentPermissions(ctx, parent.ID)
	if err != nil {
		internalErr(w, s.log, "查权限失败", err)
		return
	}
	if req.SetDeviceLimit && !myPerms.SetDeviceLimit {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "你自己没有设置设备数量的权限")
		return
	}
	if req.CustomKami && !myPerms.CustomKami {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "你自己没有自定义卡密的权限")
		return
	}
	if req.MaxQty <= 0 {
		req.MaxQty = 1
	}
	if myPerms.MaxQty > 0 && req.MaxQty > myPerms.MaxQty {
		writeErr(w, http.StatusBadRequest, CodeBadInput, fmt.Sprintf("单批上限不能超过你自己的 %d", myPerms.MaxQty))
		return
	}
	_, subPlanID, err := s.st.AgentPermissions(ctx, sub.ID)
	if err != nil {
		internalErr(w, s.log, "查下级模板失败", err)
		return
	}
	if err := s.st.SetAgentPlanAndPerms(ctx, sub.ID, subPlanID, store.AgentPermissions{
		SetDeviceLimit: req.SetDeviceLimit, CustomKami: req.CustomKami, MaxQty: req.MaxQty,
	}); err != nil {
		internalErr(w, s.log, "设置下级权限失败", err)
		return
	}
	writeData(w, map[string]any{"id": sub.ID, "updated": true})
}

// agentSubLink 给下级补/取专属登录链接（老数据没有 slug）。
func (s *Server) agentSubLink(w http.ResponseWriter, r *http.Request) {
	_, sub, ok := s.subAgent(w, r)
	if !ok {
		return
	}
	slug := sub.LoginSlug
	if slug == "" {
		var err error
		slug, err = newAgentSlug()
		if err != nil {
			internalErr(w, s.log, "生成链接失败", err)
			return
		}
		if err := s.st.SetAgentLoginSlug(r.Context(), sub.ID, slug); err != nil {
			internalErr(w, s.log, "保存链接失败", err)
			return
		}
	}
	writeData(w, map[string]any{
		"id": sub.ID, "login_link": s.agentInviteLink(slug),
		"activated": sub.PasswordSet == 1,
	})
}

// resetAgentPassword 管理员重置代理密码。
func (s *Server) resetAgentPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.Password) < auth.MinPasswordLen {
		writeErr(w, http.StatusBadRequest, CodeBadInput, fmt.Sprintf("密码至少 %d 位", auth.MinPasswordLen))
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}
	if err := s.st.SetAgentPassword(r.Context(), id, hash); err != nil {
		internalErr(w, s.log, "重置密码失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "agent.reset_password", fmt.Sprintf("agent:%d", id), "", `{"changed":true}`, clientIP(r))
	writeData(w, map[string]any{"id": id, "changed": true})
}
