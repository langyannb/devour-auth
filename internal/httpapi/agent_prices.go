package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/agent"
)

// listAgentPrices 返回该代理对每个卡类型的实际单价（含未单独定价的默认价）。
func (s *Server) listAgentPrices(w http.ResponseWriter, r *http.Request) {
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
	apps, err := s.st.ListApps(ctx)
	if err != nil || len(apps) == 0 {
		writeData(w, []map[string]any{})
		return
	}
	appID := effectiveAppID(r)
	if appID <= 0 {
		appID = apps[0].ID
	}
	policies, err := s.st.ListPolicies(ctx, appID)
	if err != nil {
		internalErr(w, s.log, "查卡类型失败", err)
		return
	}
	custom, err := s.st.ListAgentPrices(ctx, id)
	if err != nil {
		internalErr(w, s.log, "查代理价格失败", err)
		return
	}

	out := make([]map[string]any, 0, len(policies))
	for _, p := range policies {
		if p.Active != 1 || isInternalPolicy(p.Code) {
			continue
		}
		def := agent.PayableCents(p.PriceCents, a.DiscountBP, 1) // 售价 × 折扣
		price := def
		if v, ok := custom[p.ID]; ok {
			price = v
		}
		out = append(out, map[string]any{
			"policy_id":        p.ID,
			"code":             p.Code,
			"name":             p.Name,
			"kind":             p.Kind,
			"base_price_cents": p.PriceCents,
			"discount_price":   def,
			"price_cents":      price,
			"custom":           func() bool { _, ok := custom[p.ID]; return ok }(),
		})
	}
	writeData(w, out)
}

// setAgentPrices 批量设置代理价（price_cents=0 表示删除该条，回退到折扣价）。
func (s *Server) setAgentPrices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	if a, err := s.st.AgentByID(ctx, id); err != nil || (adminScopeID(r) > 0 && a.AppID != adminScopeID(r)) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "代理不存在")
		return
	}
	var req struct {
		Items []struct {
			PolicyID   int64 `json:"policy_id"`
			PriceCents int64 `json:"price_cents"`
		} `json:"items"`
	}
	if !decode(w, r, &req) {
		return
	}
	now := s.now().Unix()
	for _, it := range req.Items {
		if it.PolicyID <= 0 {
			continue
		}
		if err := s.st.SetAgentPrice(ctx, id, it.PolicyID, it.PriceCents, now); err != nil {
			internalErr(w, s.log, "保存代理价格失败", err)
			return
		}
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "agent.set_prices", fmt.Sprintf("agent:%d", id), "",
		fmt.Sprintf(`{"items":%d}`, len(req.Items)), clientIP(r))
	writeData(w, map[string]any{"updated": len(req.Items)})
}
