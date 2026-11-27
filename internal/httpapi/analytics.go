package httpapi

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/langyannb/devour-auth/internal/geo"
)

// analyticsDays 数据大屏的时间窗口：7–90 天，默认 30。
func analyticsDays(r *http.Request) int {
	days := int(queryInt64(r, "days"))
	if days < 7 || days > 90 {
		if days > 90 {
			return 90
		}
		return 30
	}
	return days
}

// analyticsSince 窗口起始那天的本地零点。
func analyticsSince(now time.Time, days int) (string, int64) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -(days - 1))
	return start.Format("2006-01-02"), start.Unix()
}

// adminAnalytics 管理端数据大屏：趋势、卡类型、状态、代理排行。
func (s *Server) adminAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	appID := queryInt64(r, "app_id")
	days := analyticsDays(r)
	now := s.now()
	sinceDay, since := analyticsSince(now, days)

	trend, err := s.st.ListDailyStats(ctx, sinceDay)
	if err != nil {
		internalErr(w, s.log, "查趋势失败", err)
		return
	}
	kinds, err := s.st.CardsByKind(ctx, appID)
	if err != nil {
		internalErr(w, s.log, "查卡类型失败", err)
		return
	}
	top, err := s.st.TopAgentsByCards(ctx, appID, 6)
	if err != nil {
		internalErr(w, s.log, "查代理排行失败", err)
		return
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	st, err := s.st.Stats(ctx, appID, dayStart, now.Unix())
	if err != nil {
		internalErr(w, s.log, "统计失败", err)
		return
	}
	activeDevices, err := s.st.ActiveDevices(ctx, appID, since)
	if err != nil {
		internalErr(w, s.log, "查活跃设备失败", err)
		return
	}
	geoStats, err := s.provinceStats(ctx, appID, since)
	if err != nil {
		internalErr(w, s.log, "查省份分布失败", err)
		return
	}
	writeData(w, map[string]any{
		"days":            days,
		"trend":           trend,
		"by_kind":         kinds,
		"top_agents":      top,
		"cards_total":     st.CardsTotal,
		"cards_by_status": st.CardsByStatus,
		"devices_bound":   st.DevicesBound,
		"agents_active":   st.AgentsActive,
		"online_sessions": st.OnlineSessions,
		"activations_day": st.ActivationsDay,
		"active_devices":  activeDevices,
		"geo":             geoStats,
		"server_time":     now.Unix(),
	})
}

// ProvinceStat 一个省份的登录与 IP 数。
type ProvinceStat struct {
	Province string `json:"province"`
	Logins   int64  `json:"logins"`
	IPs      int64  `json:"ips"`
	Abroad   bool   `json:"abroad"`
}

// provinceStats 把来源 IP 解析成省份（离线库，查不到归「未知」）。
func (s *Server) provinceStats(ctx context.Context, appID, since int64) ([]ProvinceStat, error) {
	ips, err := s.st.LoginIPCounts(ctx, appID, since)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*ProvinceStat{}
	for _, x := range ips {
		province, _, abroad := geo.LookupFull(x.IP)
		name := geo.ShortProvince(province)
		switch {
		case province == "":
			name = "未知"
		case name == "":
			name = province
		}
		st := byKey[name]
		if st == nil {
			st = &ProvinceStat{Province: name, Abroad: abroad}
			byKey[name] = st
		}
		st.Logins += x.Logins
		st.IPs++
	}
	out := make([]ProvinceStat, 0, len(byKey))
	for _, st := range byKey {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Logins != out[j].Logins {
			return out[i].Logins > out[j].Logins
		}
		return out[i].Province < out[j].Province
	})
	return out, nil
}

// agentAnalytics 代理端数据：自己与下级范围内的出卡趋势与分布。
func (s *Server) agentAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := agentFrom(r)
	days := analyticsDays(r)
	now := s.now()
	_, since := analyticsSince(now, days)

	ids, err := s.agents.VisibleAgentIDs(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查可见范围失败", err)
		return
	}
	kinds, err := s.st.AgentCardsBy(ctx, ids, "kind")
	if err != nil {
		internalErr(w, s.log, "查卡类型失败", err)
		return
	}
	statuses, err := s.st.AgentCardsBy(ctx, ids, "status")
	if err != nil {
		internalErr(w, s.log, "查卡状态失败", err)
		return
	}
	daily, err := s.st.AgentDailyCards(ctx, ids, since)
	if err != nil {
		internalErr(w, s.log, "查出卡趋势失败", err)
		return
	}
	activeDevices, err := s.st.AgentActiveDevices(ctx, ids, since)
	if err != nil {
		internalErr(w, s.log, "查活跃设备失败", err)
		return
	}
	writeData(w, map[string]any{
		"days":           days,
		"trend":          daily,
		"by_kind":        kinds,
		"by_status":      statuses,
		"active_devices": activeDevices,
		"quota_cents":    c.Agent.QuotaCents,
		"level":          c.Agent.Level,
		"server_time":    now.Unix(),
	})
}
