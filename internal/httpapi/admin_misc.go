package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/store"
)

// ---------- 黑名单 ----------

func (s *Server) listBlacklist(w http.ResponseWriter, r *http.Request) {
	app, _ := s.appFromQuery(r)
	list, err := s.st.ListBlacklist(r.Context(), app.ID)
	if err != nil {
		internalErr(w, s.log, "查黑名单失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, b := range list {
		out = append(out, map[string]any{
			"id": b.ID, "app_id": b.AppID, "kind": b.Kind, "value": b.Value,
			"reason": b.Reason, "expires_at": b.ExpiresAt, "created_at": b.CreatedAt,
		})
	}
	writeData(w, out)
}

func (s *Server) addBlacklist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, _ := s.appFromQuery(r)
	var req struct {
		Kind      string `json:"kind"`
		Value     string `json:"value"`
		Reason    string `json:"reason"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if !decode(w, r, &req) {
		return
	}
	switch req.Kind {
	case "ip", "device", "card":
	default:
		writeErr(w, http.StatusBadRequest, CodeBadInput, "kind 需为 ip/device/card")
		return
	}
	if req.Value == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "value 必填")
		return
	}
	c := adminFrom(r)
	id, err := s.st.AddBlacklist(ctx, store.BlacklistEntry{
		AppID: app.ID, Kind: req.Kind, Value: req.Value, Reason: req.Reason,
		ExpiresAt: req.ExpiresAt, CreatedAt: s.now().Unix(),
	}, c.Admin.ID)
	if err != nil {
		internalErr(w, s.log, "写黑名单失败", err)
		return
	}
	_ = s.writeAudit(ctx, c, "blacklist.add", fmt.Sprintf("blacklist:%d", id), "",
		fmt.Sprintf(`{"kind":%q,"value":%q}`, req.Kind, req.Value), clientIP(r))
	writeData(w, map[string]any{"id": id, "ok": true})
}

func (s *Server) removeBlacklist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	ctx := r.Context()
	if err := s.st.RemoveBlacklist(ctx, id); err != nil {
		internalErr(w, s.log, "删除黑名单失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "blacklist.remove", fmt.Sprintf("blacklist:%d", id), "", `{}`, clientIP(r))
	writeData(w, map[string]any{"id": id, "removed": true})
}

// ---------- 事件日志 ----------

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	list, total, err := s.st.ListEvents(r.Context(),
		adminScopeID(r), queryInt64(r, "card_id"), r.URL.Query().Get("kind"),
		int(queryInt64(r, "limit")), int(queryInt64(r, "offset")))
	if err != nil {
		internalErr(w, s.log, "查事件失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		out = append(out, map[string]any{
			"ts": e.TS, "kind": e.Kind, "card_id": e.CardID, "device_id": e.DeviceID,
			"ip": e.IP, "ua": e.UA, "code": e.Code, "detail": e.Detail,
		})
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

// ---------- 财务 ----------

func (s *Server) listFinanceLedger(w http.ResponseWriter, r *http.Request) {
	rows, total, err := s.st.ListAllLedger(r.Context(),
		effectiveAppID(r), queryInt64(r, "agent_id"), int(queryInt64(r, "limit")), int(queryInt64(r, "offset")))
	if err != nil {
		internalErr(w, s.log, "查流水失败", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		out = append(out, map[string]any{
			"id": x.ID, "agent_id": x.AgentID, "kind": x.Kind,
			"amount_cents": x.AmountCents, "ref": x.Ref, "created_at": x.CreatedAt,
		})
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

func (s *Server) listFinanceBills(w http.ResponseWriter, r *http.Request) {
	bills, err := s.st.ListAllBills(r.Context(), effectiveAppID(r), r.URL.Query().Get("status"))
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

// ---------- 批次 ----------

func (s *Server) listBatches(w http.ResponseWriter, r *http.Request) {
	app, _ := s.appFromQuery(r)
	list, err := s.st.ListBatches(r.Context(), app.ID)
	if err != nil {
		internalErr(w, s.log, "查批次失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, b := range list {
		var agentID any
		if b.AgentID.Valid {
			agentID = b.AgentID.Int64
		}
		out = append(out, map[string]any{
			"id": b.ID, "policy_id": b.PolicyID, "agent_id": agentID,
			"qty": b.Qty, "created_by": b.CreatedBy, "created_at": b.CreatedAt,
		})
	}
	writeData(w, out)
}

// ---------- 系统状态 ----------

func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{
		"server_time": s.now().Unix(),
		"env":         s.env,
		"admin_host":  s.adminHost,
		"agent_host":  s.agentHost,
		"net_host":    s.netHost,
	}
	var n int64
	_ = s.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM cards`).Scan(&n)
	out["cards"] = n
	_ = s.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE revoked_at IS NULL AND refresh_exp > ?`, s.now().Unix()).Scan(&n)
	out["online_sessions"] = n
	_ = s.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&n)
	out["events"] = n
	_ = s.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&n)
	out["audit_entries"] = n
	_ = s.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE status = 'active'`).Scan(&n)
	out["active_api_keys"] = n
	out["uptime_note"] = "服务重启后计数归零，进程启动时间见 systemd"
	_ = time.Now()
	writeData(w, out)
}
