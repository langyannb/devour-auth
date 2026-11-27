package httpapi

import (
	"crypto/ed25519"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/audit"
)

// adminStats 概览页的汇总数字。
func (s *Server) adminStats(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	appID := queryInt64(r, "app_id")
	st, err := s.st.Stats(r.Context(), appID, dayStart, now.Unix())
	if err != nil {
		internalErr(w, s.log, "统计失败", err)
		return
	}
	writeData(w, map[string]any{
		"cards_total":     st.CardsTotal,
		"cards_by_status": st.CardsByStatus,
		"devices_bound":   st.DevicesBound,
		"agents_active":   st.AgentsActive,
		"activations_day": st.ActivationsDay,
		"online_sessions": st.OnlineSessions,
		"server_time":     now.Unix(),
	})
}

// statsTrend 最近 N 天的每日汇总（默认 14 天）。
func (s *Server) statsTrend(w http.ResponseWriter, r *http.Request) {
	days := int(queryInt64(r, "days"))
	if days <= 0 || days > 90 {
		days = 14
	}
	since := s.now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	list, err := s.st.ListDailyStats(r.Context(), since)
	if err != nil {
		internalErr(w, s.log, "查趋势失败", err)
		return
	}
	writeData(w, list)
}

// verifyAuditChain 重算整链并校验每日锚点（后台「校验」按钮）。
func (s *Server) verifyAuditChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := audit.Verify(ctx, s.st.DB())
	if err != nil {
		writeData(w, map[string]any{"ok_chain": false, "entries": n, "error": err.Error()})
		return
	}
	m, err := audit.VerifyAnchors(ctx, s.st.DB(), s.ticketKey.Public().(ed25519.PublicKey))
	if err != nil {
		writeData(w, map[string]any{"ok_chain": false, "entries": n, "anchors": m, "error": err.Error()})
		return
	}
	writeData(w, map[string]any{"ok_chain": true, "entries": n, "anchors": m})
}
