package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

// agentCreateSub 一级代理开通二级代理；折扣不能低于自己的成本。
func (s *Server) agentCreateSub(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	if c.Agent.Level != 1 {
		writeErr(w, http.StatusForbidden, CodeUnauthorized, "只有一级代理能开通下级")
		return
	}
	var req struct {
		Name       string `json:"name"`
		Password   string `json:"password"`
		DiscountBP int64  `json:"discount_bp"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" || len(req.Password) < auth.MinPasswordLen {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "代理名必填，密码至少 12 位")
		return
	}
	if req.DiscountBP < c.Agent.DiscountBP || req.DiscountBP > 10000 {
		writeErr(w, http.StatusBadRequest, CodeBadInput,
			fmt.Sprintf("下级折扣不能低于你的成本（%d 基点）", c.Agent.DiscountBP))
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}
	id, err := s.st.CreateAgent(r.Context(), store.Agent{
		Name: req.Name, ParentID: store.NullInt64(c.Agent.ID), Level: 2,
		DiscountBP: req.DiscountBP, RebateBP: 0, MonthlyQuotaCents: 0,
		PassHash: hash, Status: "active", CreatedAt: s.now().Unix(),
	})
	if err == store.ErrConflict {
		writeErr(w, http.StatusConflict, CodeBadInput, "代理名已存在")
		return
	}
	if err != nil {
		internalErr(w, s.log, "开通下级失败", err)
		return
	}
	_ = s.writeAudit(r.Context(), adminCtx{
		Admin: store.Admin{ID: c.Agent.ID, Username: c.Agent.Name, Role: "agent"},
	}, "agent.create_sub", fmt.Sprintf("agent:%d", id), "",
		fmt.Sprintf(`{"name":%q,"discount_bp":%d}`, req.Name, req.DiscountBP), clientIP(r))
	writeData(w, map[string]any{"id": id, "name": req.Name, "level": 2, "discount_bp": req.DiscountBP})
}

// agentChangePassword 代理改自己的密码。
func (s *Server) agentChangePassword(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !auth.VerifyPassword(req.OldPassword, c.Agent.PassHash) {
		writeErr(w, http.StatusForbidden, CodeUnauthorized, "原密码错误")
		return
	}
	if len(req.NewPassword) < auth.MinPasswordLen {
		writeErr(w, http.StatusBadRequest, CodeBadInput,
			fmt.Sprintf("新密码至少 %d 位", auth.MinPasswordLen))
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		internalErr(w, s.log, "哈希失败", err)
		return
	}
	if err := s.st.SetAgentPassword(r.Context(), c.Agent.ID, hash); err != nil {
		internalErr(w, s.log, "改密码失败", err)
		return
	}
	writeData(w, map[string]any{"changed": true})
}

// agentBillsExport 代理导出自己的流水 CSV。
func (s *Server) agentBillsExport(w http.ResponseWriter, r *http.Request) {
	c := agentFrom(r)
	rows, _, err := s.st.ListAgentLedger(r.Context(), c.Agent.ID, 200, 0)
	if err != nil {
		internalErr(w, s.log, "导出失败", err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="ledger.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte("id,kind,amount_cents,ref,created_at\n"))
	for _, x := range rows {
		_, _ = fmt.Fprintf(w, "%d,%s,%d,%s,%d\n", x.ID, x.Kind, x.AmountCents, x.Ref, x.CreatedAt)
	}
}
