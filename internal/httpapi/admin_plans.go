package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/store"
)

func planJSON(p store.AgentPlan) map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Name, "active": p.Active,
		"items": p.Items, "created_at": p.CreatedAt,
	}
}

func (s *Server) listPlans(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	list, err := s.st.ListAgentPlans(r.Context(), app.ID)
	if err != nil {
		internalErr(w, s.log, "查价格模板失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, planJSON(p))
	}
	writeData(w, out)
}

type planReq struct {
	ID     int64            `json:"id"`
	Name   string           `json:"name"`
	Active int64            `json:"active"`
	Items  map[string]int64 `json:"items"`
}

func (s *Server) savePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	var req planReq
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "模板名必填")
		return
	}
	items := map[string]int64{}
	for kind, price := range req.Items {
		switch kind {
		case "day", "week", "month", "forever": // 自定义时间不给代理
			if price > 0 {
				items[kind] = price
			}
		}
	}
	if len(items) == 0 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "至少要设置一个卡类型的价格")
		return
	}
	active := req.Active
	if req.ID == 0 {
		active = 1
	}
	id, err := s.st.SaveAgentPlan(ctx, store.AgentPlan{
		ID: req.ID, AppID: app.ID, Name: req.Name, Active: active, Items: items,
	}, s.now().Unix())
	if err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, CodeBadInput, "模板名已存在")
			return
		}
		internalErr(w, s.log, "保存模板失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "plan.save", fmt.Sprintf("plan:%d", id), "",
		fmt.Sprintf(`{"name":%q,"items":%d}`, req.Name, len(items)), clientIP(r))
	got, _ := s.st.AgentPlanByID(ctx, id)
	writeData(w, planJSON(got))
}

// setAgentPlan 给代理绑定模板并设置权限。
func (s *Server) setAgentPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	if _, err := s.st.AgentByID(ctx, id); err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "代理不存在")
		return
	}
	var req struct {
		PlanID         int64 `json:"plan_id"`
		SetDeviceLimit bool  `json:"set_device_limit"`
		CustomKami     bool  `json:"custom_kami"`
		MaxQty         int64 `json:"max_qty"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.MaxQty <= 0 {
		req.MaxQty = 10
	}
	if err := s.st.SetAgentPlanAndPerms(ctx, id, req.PlanID, store.AgentPermissions{
		SetDeviceLimit: req.SetDeviceLimit,
		CustomKami:     req.CustomKami,
		MaxQty:         req.MaxQty,
	}); err != nil {
		internalErr(w, s.log, "设置代理模板失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "agent.set_plan", fmt.Sprintf("agent:%d", id), "",
		fmt.Sprintf(`{"plan_id":%d,"set_device_limit":%t,"custom_kami":%t,"max_qty":%d}`,
			req.PlanID, req.SetDeviceLimit, req.CustomKami, req.MaxQty), clientIP(r))
	writeData(w, map[string]any{"id": id, "updated": true})
}

// getAgentPlan 读代理当前模板与权限。
func (s *Server) getAgentPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	perms, planID, err := s.st.AgentPermissions(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "代理不存在")
		return
	}
	var plan any
	if planID > 0 {
		if p, err := s.st.AgentPlanByID(ctx, planID); err == nil {
			plan = planJSON(p)
		}
	}
	writeData(w, map[string]any{"plan_id": planID, "plan": plan, "permissions": perms})
}
